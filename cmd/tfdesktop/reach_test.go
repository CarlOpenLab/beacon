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

	// The window opens a tab on that shell. It arrives as a viewer — the shell
	// is unclaimed, so the first watcher takes it — and its output shows.
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

	// This window opens a tab on that shell. It arrives as a viewer, and the
	// chrome says so: the holder is named, and this window does not hold.
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
	waitFor(t, "the lease as a viewer", 20*time.Second, func() bool {
		w.drain()
		return tb.holder() != "" && !tb.held()
	})

	// A viewer's keys go nowhere: the conn takes them and drops them, so the
	// shell never sees a keystroke it should not have.
	tb.conn.Write([]byte("printf 'VIEWER_TYPED\\n' 1\n"))
	time.Sleep(300 * time.Millisecond)
	if strings.Contains(tb.term.Text(), "VIEWER_TYPED") {
		t.Fatal("a viewer's keystrokes reached the shell")
	}

	// Stealing takes it, and the other machine's window is told it lost it.
	tb.conn.claiming(true)
	waitFor(t, "the lease after stealing", 20*time.Second, func() bool {
		w.drain()
		return tb.held()
	})
	owner.waitFor("the owner is demoted", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleViewer && m.Controller != ""
	})
	tb.conn.Write([]byte("printf 'STOLEN_AND_TYPED\\n' 1\n"))
	waitFor(t, "the shell echoing the new holder", 20*time.Second, func() bool {
		w.drain()
		return strings.Contains(tb.term.Text(), "STOLEN_AND_TYPED")
	})

	// And the machine whose shell it is takes it back, with no confirmation:
	// the hub honours its own window's claim over anyone else's.
	owner.send(protocol.Msg{T: protocol.CTClaim, SessionID: shell})
	waitFor(t, "the owner has it back", 20*time.Second, func() bool {
		w.drain()
		return tb.holder() != "" && !tb.held()
	})
	owner.waitFor("the owner holds it again", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleController
	})
}

// Two desktops, one relay: the pair the report was about. Each machine is a
// window with its own shells, its own member link and its own client link, set
// up the way a person sets one up — the address and the client token on the 远程
// pane, then 应用 — and each drives a shell on the other.
//
// The shell on a machine is its own window's to hold, so the far side arrives
// read-only and takes it back with 拿回控制权: the same steal a browser asks for,
// answered by the machine that owns the shell, which is what the glow and the
// popover are for.
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

		// The other desktop picks that machine in the panel and opens the shell:
		// read-only, because the shell is the far machine's own window's.
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

		// 拿回控制权: the far machine is told in the same frame, and the glow and
		// the popover that say "not yours right now" are what it gets.
		side.w.reclaim(tb)
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
