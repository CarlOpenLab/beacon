//go:build !windows

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
		Devices:     map[string]string{"member-2": "membersec"},
		ClientToken: "fleettoken",
	})
	if err != nil {
		t.Fatalf("relay: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/relay/agent", rl.ServeAgent)
	mux.HandleFunc("/relay/client", rl.ServeClient)
	mux.HandleFunc("/relay/machines", rl.Machines)
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

	// The window's machine, and the window's client link to the relay.
	f := newTestFleet(t)
	store := settings.Open(t.TempDir())
	if _, err := store.Update(func(s *settings.Settings) {
		s.RelayURL = "ws" + strings.TrimPrefix(ts.URL, "http") + "/relay/agent"
		s.ClientToken = "fleettoken"
	}); err != nil {
		t.Fatalf("store: %v", err)
	}
	w := newWindow(f, store, "", "")
	// The window's own client link, as the app opens it at startup.
	if err := f.SetRelayClient(store.Get().RelayURL, store.Get().ClientToken, ""); err != nil {
		t.Fatalf("relay client: %v", err)
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
		Devices:     map[string]string{"member-2": "membersec"},
		ClientToken: "fleettoken",
	})
	if err != nil {
		t.Fatalf("relay: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/relay/agent", rl.ServeAgent)
	mux.HandleFunc("/relay/client", rl.ServeClient)
	mux.HandleFunc("/relay/machines", rl.Machines)
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

	// This machine's window, with its client link to the relay.
	f := newTestFleet(t)
	store := settings.Open(t.TempDir())
	if _, err := store.Update(func(s *settings.Settings) {
		s.RelayURL = "ws" + strings.TrimPrefix(ts.URL, "http") + "/relay/agent"
		s.ClientToken = "fleettoken"
	}); err != nil {
		t.Fatalf("store: %v", err)
	}
	w := newWindow(f, store, "", "")
	if err := f.SetRelayClient(store.Get().RelayURL, store.Get().ClientToken, ""); err != nil {
		t.Fatalf("relay client: %v", err)
	}
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
