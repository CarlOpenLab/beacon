//go:build !windows

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/agent"
	"github.com/CarlOpenLab/term-fleet/internal/daemon"
	"github.com/CarlOpenLab/term-fleet/internal/hub"
	"github.com/CarlOpenLab/term-fleet/internal/protocol"
	"github.com/CarlOpenLab/term-fleet/internal/relay"
	"github.com/CarlOpenLab/term-fleet/internal/settings"
)

// The whole reach, over real sockets: a relay, another machine's daemon dialled
// into it as a member, and the window's own client link. If the tunnel could not
// carry a session under the hub's arbitration, this could not pass — which is
// the point, because taking over a shell on another machine is the same act as
// taking one back on this one, and both are proved the same way.
func TestWindowTakesOverAnotherMachinesShell(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// The other machine: its own daemon, its own hub, its own PTY.
	other, err := daemon.New(daemon.Options{Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("daemon: %v", err)
	}
	go func() { _ = other.Serve(ctx) }()
	t.Cleanup(func() { _ = other.Close() })

	rl, err := relay.New(relay.Config{
		Devices:     map[string]string{"member-2": "membersec", "desk-1": "desksec"},
		ClientToken: "fleettoken",
	})
	if err != nil {
		t.Fatalf("relay: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/relay/agent", rl.ServeAgent)
	mux.HandleFunc("/relay/client", rl.ServeClient)
	mux.HandleFunc("/relay/machines", rl.Machines)
	mux.HandleFunc("/relay/control-token", rl.ControlToken)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	ag, err := agent.New(agent.Config{
		URL:         "ws" + strings.TrimPrefix(ts.URL, "http") + "/relay/agent",
		DeviceID:    "member-2",
		Secret:      "membersec",
		MachineName: "other box",
		Hub:         other.Hub(),
		MinBackoff:  10 * time.Millisecond,
		MaxBackoff:  50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("agent: %v", err)
	}
	go func() { _ = ag.Run(ctx) }()

	// The window's machine, and the window's client link to the relay — set up
	// the way a person sets it up, through the 远程 pane: the address, the fleet's
	// client token in its field, 应用. Nothing here reaches past what the window
	// itself can be told to do, which is the point: the pair of machines this
	// test stands for is two desktops and a relay.
	f := newTestFleet(t)
	store := settings.Open(t.TempDir())
	w := newWindow(f, store, "", "")
	sw := newSettingsWindow(w, store, f)
	tt := ui.NewTester(sw.view, 640, 640)
	tt.Frame()
	if err := tt.Click("远程"); err != nil {
		t.Fatalf("Click(远程): %v", err)
	}
	tt.Frame()
	sw.relay, sw.device, sw.secret = "ws"+strings.TrimPrefix(ts.URL, "http")+"/relay/agent", "desk-1", "desksec"
	sw.relayOn = true
	tt.Frame()
	if err := tt.Click("应用"); err != nil {
		t.Fatalf("Click(应用): %v", err)
	}
	tt.Frame()
	if f.RelayClient() == nil {
		t.Fatalf("applying the relay did not build a client link: %s", f.RelayClientErr())
	}

	// The roster arrives, and the other machine is on it, online.
	waitFor(t, "the roster", 20*time.Second, func() bool {
		w.drain()
		for _, m := range w.roster {
			if m.ID == "member-2" && m.Online {
				return true
			}
		}
		return false
	})

	// A shell on the other machine, started from there.
	remote, err := other.Hub().Create("", 80, 24, "", "/bin/sh")
	if err != nil {
		t.Fatalf("remote shell: %v", err)
	}

	// The window looks at the machine, and its hub reports what it runs.
	w.lookAtMachine("member-2")
	waitFor(t, "the other machine's session list", 20*time.Second, func() bool {
		w.drain()
		return len(w.remoteSessions["member-2"]) == 1
	})

	// The window opens a tab on that shell, and drives it: reaching a machine is
	// the consent, so the tab comes with the lease and the shell's own machine
	// goes read-only.
	l := w.remoteLink("member-2")
	if l == nil {
		t.Fatal("no link to the other machine")
	}
	tb := w.openTab("member-2", remote.ID)

	// The attach is answered with the lease: the first watcher of an unclaimed
	// shell holds it, so the window may type into another machine's shell under
	// that machine's arbitration.
	waitFor(t, "the lease on the remote shell", 20*time.Second, func() bool {
		w.drain()
		return tb.held()
	})
	tb.conn.Write([]byte("printf 'FROM_DESKTOP\\n' 1\n"))
	// The terminal's own screen is what proves it: the conn's buffer is drained
	// by the view as fast as it fills, so only the rendered text is left to read.
	waitFor(t, "the remote shell echoing", 10*time.Second, func() bool {
		return strings.Contains(tb.term.Text(), "FROM_DESKTOP")
	})
}

// pipeClient drives the window's end of an in-process pipe with the same
// helpers a socket client has: it stands in for the other machine's own window.
type pipeClient struct {
	t    *testing.T
	pipe *hub.Pipe
}

func newPipeClient(t *testing.T, p *hub.Pipe) *pipeClient {
	t.Helper()
	return &pipeClient{t: t, pipe: p}
}

func (c *pipeClient) send(m protocol.Msg) {
	c.t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		c.t.Fatalf("marshal: %v", err)
	}
	if err := c.pipe.Send(b); err != nil {
		c.t.Fatalf("send: %v", err)
	}
}

func (c *pipeClient) recv() protocol.Msg {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	b, err := c.pipe.Recv(ctx)
	if err != nil {
		c.t.Fatalf("recv: %v", err)
	}
	var m protocol.Msg
	if err := json.Unmarshal(b, &m); err != nil {
		c.t.Fatalf("unmarshal: %v", err)
	}
	return m
}

func (c *pipeClient) waitFor(what string, pred func(protocol.Msg) bool) protocol.Msg {
	c.t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if m := c.recv(); pred(m) {
			return m
		}
	}
	c.t.Fatalf("timeout waiting for %s", what)
	return protocol.Msg{}
}

// waitFor polls until cond holds or the deadline passes, applying whatever the
// links have posted each time: the window's state is written by their reader
// goroutines and read here, and drain is the edge between the two.
func waitFor(t *testing.T, what string, d time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

// The takeover, end to end and across machines: the other machine's own window
// holds its shell, this window arrives as a viewer with the glow showing, takes
// the lease by stealing, and the other machine's window takes it back — which it
// may always do, because it is the machine's shell.
func TestTakeoverAcrossMachines(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// The other machine: its daemon, its hub, and its own window's link — the
	// machine's own client, which the hub marks as the owner.
	other, err := daemon.New(daemon.Options{Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("daemon: %v", err)
	}
	go func() { _ = other.Serve(ctx) }()
	t.Cleanup(func() { _ = other.Close() })
	ownerPipe := hub.NewPipe()
	defer ownerPipe.Close()
	go other.Hub().ServeLocal(ownerPipe.Local())
	owner := newPipeClient(t, ownerPipe)

	rl, err := relay.New(relay.Config{
		Devices:     map[string]string{"member-2": "membersec", "desk-1": "desksec"},
		ClientToken: "fleettoken",
	})
	if err != nil {
		t.Fatalf("relay: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/relay/agent", rl.ServeAgent)
	mux.HandleFunc("/relay/client", rl.ServeClient)
	mux.HandleFunc("/relay/machines", rl.Machines)
	mux.HandleFunc("/relay/control-token", rl.ControlToken)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	ag, err := agent.New(agent.Config{
		URL:         "ws" + strings.TrimPrefix(ts.URL, "http") + "/relay/agent",
		DeviceID:    "member-2",
		Secret:      "membersec",
		MachineName: "other box",
		Hub:         other.Hub(),
		MinBackoff:  10 * time.Millisecond,
		MaxBackoff:  50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("agent: %v", err)
	}
	go func() { _ = ag.Run(ctx) }()

	// This machine's window, reaching the fleet the way the app does: join it as
	// a member, then ask the relay for a credential to drive the rest of it.
	f := newTestFleet(t)
	store := settings.Open(t.TempDir())
	relayURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/relay/agent"
	if err := f.SetRelay(relayURL, "desk-1", "desksec", "", true); err != nil {
		t.Fatalf("SetRelay: %v", err)
	}
	if err := f.Reach(); err != nil {
		t.Fatalf("Reach: %v", err)
	}
	w := newWindow(f, store, "", "")
	waitFor(t, "the roster", 20*time.Second, func() bool {
		w.drain()
		for _, m := range w.roster {
			if m.ID == "member-2" && m.Online {
				return true
			}
		}
		return false
	})

	// The other machine's window opens a shell and holds it.
	owner.send(protocol.Msg{T: protocol.CTCreate, Cols: 80, Rows: 24, Shell: "/bin/sh"})
	created := owner.waitFor("created", func(m protocol.Msg) bool { return m.T == protocol.STCreated })
	shell := created.SessionID
	owner.waitFor("the owner holds the lease", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleController
	})

	// This window opens a tab on that shell, and the tab drives it: the lease
	// comes with the tab, so the machine whose shell it is is the one that ends
	// up read-only, with the glow and 拿回控制权 on its side.
	w.lookAtMachine("member-2")
	waitFor(t, "the other machine's session list", 20*time.Second, func() bool {
		w.drain()
		for _, s := range w.remoteSessions["member-2"] {
			if s.SessionID == shell {
				return true
			}
		}
		return false
	})
	tb := w.openTab("member-2", shell)

	// The tab drives the shell: the lease comes with it, so the machine whose
	// shell it is is told it is not the one typing any more.
	waitFor(t, "the lease on the shell this window opened", 20*time.Second, func() bool {
		w.drain()
		return tb.held()
	})
	owner.waitFor("the owner is demoted", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleViewer && m.Controller != ""
	})

	// The keys of the window that holds the lease reach the shell — on another
	// machine, under that machine's arbitration.
	tb.conn.Write([]byte("printf 'DRIVEN_FROM_HERE\n' 1\n"))
	waitFor(t, "the remote shell echoing its driver", 20*time.Second, func() bool {
		w.drain()
		return strings.Contains(tb.term.Text(), "DRIVEN_FROM_HERE")
	})

	// The machine whose shell it is takes it back, with no confirmation: the hub
	// honours its own window's claim over anyone else's. Taking a shell back ends
	// the driver's connection to that machine — the tab goes, the link goes, the
	// panels come home, and the window says who did it.
	owner.send(protocol.Msg{T: protocol.CTClaim, SessionID: shell})
	waitFor(t, "the tab to go when the machine takes its shell back", 20*time.Second, func() bool {
		w.drain()
		return w.tabOf("member-2", shell) == nil
	})
	if _, open := w.remote["member-2"]; open {
		t.Error("the link to the machine that took its shell back is still open")
	}
	if got := w.selectedMachine(); got != machineLocal {
		t.Errorf("the panels are on %q after being disconnected, want this machine", got)
	}
	if notice := w.currentNotice(); !strings.Contains(notice, w.machineName("member-2")) {
		t.Errorf("notice = %q, want it to name the machine that took its shell back", notice)
	}
}

// Two desktops, one relay: the pair the report was about. Each machine is a
// window with its own shells, its own member link and its own client link, set
// up the way a person sets one up — the address and the client token on the 远程
// pane, then 应用 — and each drives a shell on the other.
//
// The window that opens a shell on another machine drives it — reaching a machine
// is the consent — so the machine whose shell it is goes read-only behind the
// glow and takes it back with 拿回控制权: the hub always honours the machine the
// shell runs on, and taking it back ends the driver's connection to that machine.
func TestTwoDesktopsDriveEachOther(t *testing.T) {
	url, alphaID, alphaSecret, betaID, betaSecret := twoDeskRelay(t)
	alpha, alphaDesk := deskFleet(t, url, alphaID, alphaSecret)
	beta, betaDesk := deskFleet(t, url, betaID, betaSecret)

	// Each window sees the other on the relay.
	for _, side := range []struct {
		w     *window
		other string
	}{{alphaDesk, betaID}, {betaDesk, alphaID}} {
		w, other := side.w, side.other
		waitFor(t, "the roster to name "+other, 20*time.Second, func() bool {
			w.drain()
			for _, m := range w.roster {
				if m.ID == other && m.Online {
					return true
				}
			}
			return false
		})
	}

	// A shell on each machine, and the other desktop drives it.
	for _, side := range []struct {
		who     string
		w       *window
		target  string
		host    *fleet
		waiting *window
	}{
		{"alpha drives beta", alphaDesk, betaID, beta, betaDesk},
		{"beta drives alpha", betaDesk, alphaID, alpha, alphaDesk},
	} {
		s, err := side.host.Hub().Create("", 80, 24, "", "/bin/sh")
		if err != nil {
			t.Fatalf("shell on %s: %v", side.target, err)
		}
		// The machine's own window takes its own shell: it is watching the shell
		// on screen, and the first watcher of an unclaimed shell holds it.
		waitFor(t, side.target+"'s own window to hold it", 20*time.Second, func() bool {
			side.waiting.drain()
			tb := side.waiting.tabOf(machineLocal, s.ID)
			return tb != nil && tb.held()
		})

		// The other desktop picks that machine in the panel and opens the shell.
		// Opening it takes the lease, so what is left behind is a machine whose
		// own window can only watch.
		entry, ok := side.w.machineByID(side.target)
		if !ok {
			t.Fatalf("%s does not know %s", side.who, side.target)
		}
		side.w.selectMachine(entry)
		if got := side.w.selectedMachine(); got != side.target {
			t.Fatalf("%s picked %s, want %s", side.who, got, side.target)
		}
		waitFor(t, "the session list from "+side.target, 20*time.Second, func() bool {
			side.w.drain()
			return len(side.w.remoteSessions[side.target]) == 1
		})
		tb := side.w.openTab(side.target, s.ID)
		if tb == nil {
			t.Fatalf("%s: no tab for the shell on %s", side.who, side.target)
		}

		// The far machine is told in the same frame, and the glow and the popover
		// that say "not yours right now" are what it gets.
		waitFor(t, side.who+": the lease", 20*time.Second, func() bool {
			side.w.drain()
			return tb.held()
		})
		waitFor(t, side.target+" to be told it lost the shell", 20*time.Second, func() bool {
			side.waiting.drain()
			other := side.waiting.tabOf(machineLocal, s.ID)
			return other != nil && !other.held() && other.holder() != ""
		})
		if !side.waiting.leaseOpen {
			t.Errorf("%s: the machine that lost the shell did not open the panel that says so", side.target)
		}
		// The glow is drawn exactly while someone else holds the shell, and only
		// on the machine being driven: the far machine's pane is the one that
		// breathes, and the window driving it — which holds the lease — is not.
		if other := side.waiting.tabOf(machineLocal, s.ID); other.holder() == "" {
			t.Errorf("%s: the driven machine has no holder to glow about", side.target)
		}
		if tb.holder() != "" {
			t.Errorf("%s: the window driving the shell thinks someone else holds it", side.who)
		}

		// The machine takes its shell back — the person sitting at it presses
		// 拿回控制权 — and the driver's connection to that machine ends: the tab
		// goes, the link goes, the panels return to its own machine, and it says
		// who did it.
		side.waiting.reclaim(side.waiting.tabOf(machineLocal, s.ID))
		waitFor(t, side.who+" to be disconnected from "+side.target, 20*time.Second, func() bool {
			side.w.drain()
			return side.w.tabOf(side.target, s.ID) == nil
		})
		if _, open := side.w.remote[side.target]; open {
			t.Errorf("%s: the link to %s is still open after it took its shell back", side.who, side.target)
		}
		if got := side.w.selectedMachine(); got != machineLocal {
			t.Errorf("%s: the panels are on %q after being disconnected, want this machine", side.who, got)
		}
		if notice := side.w.currentNotice(); !strings.Contains(notice, side.w.machineName(side.target)) {
			t.Errorf("%s: notice = %q, want it to name the machine that took its shell back", side.who, notice)
		}
	}
}

// twoDeskRelay stands up the pair the report was about: one relay with two
// machines enrolled, and the member endpoint both desks dial as their own.
func twoDeskRelay(t *testing.T) (url, alphaID, alphaSecret, betaID, betaSecret string) {
	t.Helper()
	reg, err := relay.OpenRegistry(filepath.Join(t.TempDir(), "devices.json"))
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	if alphaID, alphaSecret, err = reg.Enroll("alpha", ""); err != nil {
		t.Fatalf("enroll alpha: %v", err)
	}
	if betaID, betaSecret, err = reg.Enroll("beta", ""); err != nil {
		t.Fatalf("enroll beta: %v", err)
	}
	rl, err := relay.New(relay.Config{Registry: reg, ClientToken: "fleettoken"})
	if err != nil {
		t.Fatalf("relay: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/relay/agent", rl.ServeAgent)
	mux.HandleFunc("/relay/client", rl.ServeClient)
	mux.HandleFunc("/relay/control-token", rl.ControlToken)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return "ws" + strings.TrimPrefix(ts.URL, "http") + "/relay/agent", alphaID, alphaSecret, betaID, betaSecret
}

// deskFleet is one desktop, set up the way a person sets one up: a fleet with
// shells of its own, and a 远程 pane holding the address, this machine's device
// credential, the fleet's client token and the 接入 switch. Nothing here reaches
// past the window's own controls, which is the point — the pair of machines the
// tests below stand for is two desktops and a relay.
func deskFleet(t *testing.T, url, deviceID, secret string) (*fleet, *window) {
	t.Helper()
	f := newTestFleet(t)
	store := settings.Open(t.TempDir())
	w := newWindow(f, store, "", "")
	sw := newSettingsWindow(w, store, f)
	tt := ui.NewTester(sw.view, 640, 640)
	tt.Frame()
	if err := tt.Click("远程"); err != nil {
		t.Fatalf("Click(远程): %v", err)
	}
	tt.Frame()
	sw.relay, sw.device, sw.secret, sw.relayOn = url, deviceID, secret, true
	tt.Frame()
	if err := tt.Click("应用"); err != nil {
		t.Fatalf("Click(应用): %v", err)
	}
	tt.Frame()
	if !f.RelayOn() {
		t.Fatalf("%s is not on the relay after 应用", deviceID)
	}
	if f.RelayClient() == nil {
		t.Fatalf("%s has no client link: %s", deviceID, f.RelayClientErr())
	}
	return f, w
}

// Starting a terminal on another machine, from here: the way in when the far
// machine has nothing running. A panel that can only open shells somebody
// already started over there is not control of a machine you are not sitting at.
func TestDesktopStartsATerminalOnTheOtherMachine(t *testing.T) {
	url, alphaID, alphaSecret, betaID, betaSecret := twoDeskRelay(t)
	_, alphaDesk := deskFleet(t, url, alphaID, alphaSecret)
	beta, _ := deskFleet(t, url, betaID, betaSecret)

	waitFor(t, "the roster to name the other machine", 20*time.Second, func() bool {
		alphaDesk.drain()
		for _, m := range alphaDesk.roster {
			if m.ID == betaID && m.Online {
				return true
			}
		}
		return false
	})
	if len(beta.Hub().Sessions()) != 0 {
		t.Fatal("the far machine already has a shell")
	}

	// The click a person makes: pick the machine in the panel, then press its ＋.
	// No link to it exists yet, and the create waits for the relay to route the
	// stream rather than being dropped.
	entry, ok := alphaDesk.machineByID(betaID)
	if !ok {
		t.Fatal("the roster does not hold the other machine")
	}
	alphaDesk.selectMachine(entry)
	alphaDesk.newTab()

	var id string
	waitFor(t, "a shell on the far machine", 20*time.Second, func() bool {
		live := beta.Hub().Sessions()
		if len(live) != 1 {
			return false
		}
		id = live[0].SessionID
		return true
	})

	// The tab opened here is the tab watching that shell, and it holds it: the
	// far hub names the shell, and its `created` frame adopts the waiting tab.
	var tb *tab
	waitFor(t, "the tab to hold the far shell", 20*time.Second, func() bool {
		alphaDesk.drain()
		tb = alphaDesk.tabOf(betaID, id)
		return tb != nil && tb.held()
	})

	// And it is a real shell over there: what this window types is echoed by the
	// PTY on the far machine, which is the round trip in one line.
	waitFor(t, "the far shell's prompt", 20*time.Second, func() bool {
		return strings.TrimSpace(tb.term.Text()) != ""
	})
	if _, err := tb.conn.Write([]byte("printf 'FROM_ALPHA\\n' 1\n")); err != nil {
		t.Fatalf("type into the far shell: %v", err)
	}
	waitFor(t, "the far machine's echo", 20*time.Second, func() bool {
		return strings.Contains(tb.term.Text(), "FROM_ALPHA")
	})
}

// The lease follows the shell on screen. A window that reached another machine
// and is driving one of its shells lets go the moment the screen moves on — so
// 切到本机 does not leave the far machine sitting behind 正在被控制 with a
// keyboard that no longer works — and takes it again when the shell comes back
// on screen. The connection to that machine is untouched: it is the lease that
// moves, not the link.
func TestTheLeaseFollowsTheShellOnScreen(t *testing.T) {
	url, alphaID, alphaSecret, betaID, betaSecret := twoDeskRelay(t)
	alpha, alphaDesk := deskFleet(t, url, alphaID, alphaSecret)
	beta, betaDesk := deskFleet(t, url, betaID, betaSecret)

	waitFor(t, "the roster to name the other machine", 20*time.Second, func() bool {
		alphaDesk.drain()
		for _, m := range alphaDesk.roster {
			if m.ID == betaID && m.Online {
				return true
			}
		}
		return false
	})

	// A shell of this machine's own, so 切到本机 has something to go back to.
	own, err := alpha.Hub().Create("", 80, 24, "", "/bin/sh")
	if err != nil {
		t.Fatalf("shell on this machine: %v", err)
	}
	var ownTab *tab
	waitFor(t, "this machine's own window to hold its shell", 20*time.Second, func() bool {
		alphaDesk.drain()
		ownTab = alphaDesk.tabOf(machineLocal, own.ID)
		return ownTab != nil && ownTab.held()
	})

	// A shell on the other machine, held by that machine's own window.
	far, err := beta.Hub().Create("", 80, 24, "", "/bin/sh")
	if err != nil {
		t.Fatalf("shell on the far machine: %v", err)
	}
	var farTab *tab
	waitFor(t, "the far machine's window to hold its shell", 20*time.Second, func() bool {
		betaDesk.drain()
		farTab = betaDesk.tabOf(machineLocal, far.ID)
		return farTab != nil && farTab.held()
	})

	// This window reaches that machine and opens the shell: it drives it, so the
	// far machine's own window is left read-only and told so.
	entry, ok := alphaDesk.machineByID(betaID)
	if !ok {
		t.Fatal("the roster does not hold the other machine")
	}
	alphaDesk.selectMachine(entry)
	waitFor(t, "the far machine's session list", 20*time.Second, func() bool {
		alphaDesk.drain()
		return len(alphaDesk.remoteSessions[betaID]) == 1
	})
	mine := alphaDesk.openTab(betaID, far.ID)
	if mine == nil {
		t.Fatal("no tab for the far machine's shell")
	}
	waitFor(t, "the lease on the far shell", 20*time.Second, func() bool {
		alphaDesk.drain()
		return mine.held()
	})
	waitFor(t, "the far machine to be told it lost its shell", 20*time.Second, func() bool {
		betaDesk.drain()
		return farTab != nil && !farTab.held() && farTab.holder() != ""
	})

	// 切到本机: the click a person makes on this machine's row in the panel. The
	// screen follows it, and the far shell is let go — the far machine takes its
	// own shell back, so 正在被控制 ends there without anyone clicking anything.
	local, ok := alphaDesk.machineByID(machineLocal)
	if !ok {
		t.Fatal("the window does not know this machine")
	}
	alphaDesk.selectMachine(local)
	alphaDesk.frame(t)
	waitFor(t, "the far machine to have its shell back", 20*time.Second, func() bool {
		alphaDesk.drain()
		betaDesk.drain()
		return farTab.held() && !mine.held()
	})

	// The connection to that machine is still there: this is a lease moving, not
	// a link being dropped, and nothing on screen says otherwise.
	if alphaDesk.remote[betaID] == nil {
		t.Fatal("the link to the far machine was dropped when the lease moved")
	}
	if notice := alphaDesk.currentNotice(); strings.Contains(notice, alphaDesk.machineName(betaID)) {
		t.Errorf("notice = %q, want nothing about the far machine taking anything back", notice)
	}

	// And going back to that shell drives it again, without a click.
	alphaDesk.selectTab(mine)
	alphaDesk.frame(t)
	waitFor(t, "the lease on the far shell again", 20*time.Second, func() bool {
		alphaDesk.drain()
		return mine.held()
	})
	waitFor(t, "the far machine read-only again", 20*time.Second, func() bool {
		betaDesk.drain()
		return !farTab.held() && farTab.holder() != ""
	})
}

// frame draws one frame of the window, the way the app's own loop does. The
// lease follows the screen from there, and only from there: a window that never
// draws is a window with nothing on screen.
func (w *window) frame(t *testing.T) {
	t.Helper()
	tt := ui.NewTester(func(c *ui.Context) { w.view(c) }, 900, 600)
	tt.Frame()
}
