//go:build !windows

package main

// The reported failure: a machine whose shell is busy and held by that
// machine's own window (the Mac running an AI session), viewed from another
// machine through the relay. The viewer attaches and must be caught up by the
// ring replay — the tab renders the shell's screen, not a blank pane.

import (
	"context"
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

func TestViewerSeesBusyHeldRemoteShell(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// The other machine: its daemon, its hub, and its own window's link.
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

	// This machine's window, reaching the fleet the way the app does.
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

	// The other machine's window opens a shell and holds it: the machine's own
	// shell, held by the machine's own client — the AI session on the Mac.
	owner.send(protocol.Msg{T: protocol.CTCreate, Cols: 80, Rows: 24, Shell: "/bin/sh"})
	created := owner.waitFor("created", func(m protocol.Msg) bool { return m.T == protocol.STCreated })
	shell := created.SessionID
	owner.waitFor("the owner holds the lease", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleController
	})

	// The shell prints a banner and then keeps producing output without pause,
	// the way a session running an AI does.
	owner.send(protocol.Msg{T: protocol.CTInput, SessionID: shell,
		Data: []byte("printf 'BUSY_BANNER\\n'; while true; do printf 'TICK\\n'; sleep 0.02; done\n")})
	waitFor(t, "the banner in the ring", 10*time.Second, func() bool {
		s, ok := other.Hub().Session(shell)
		if !ok {
			return false
		}
		data, _, _ := s.Ring().ReadFrom(0)
		return strings.Contains(string(data), "BUSY_BANNER")
	})

	// This window looks at the machine and opens a tab on the busy shell. The
	// owner holds the lease, so the tab arrives as a viewer.
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
	if tb == nil {
		t.Fatal("no tab for the busy remote shell")
	}
	if tb.held() {
		t.Error("the viewer was handed the lease of a shell the machine's own window holds")
	}

	// The tab renders the shell's screen: the replayed banner, and the ticks
	// that keep arriving after the attach.
	waitFor(t, "the viewer's terminal to render the replay", 20*time.Second, func() bool {
		w.drain()
		return strings.Contains(tb.term.Text(), "BUSY_BANNER")
	})
	waitFor(t, "the viewer's terminal to render live output", 20*time.Second, func() bool {
		w.drain()
		return strings.Count(tb.term.Text(), "TICK") > 3
	})
}
