//go:build !windows

package main

// The blank pane this logging and these fixes were added for, deterministically:
// a tab on another machine's shell is closed, and the same shell is opened
// again. The second tab must paint the shell's screen.
//
// It used to be white forever. The link keeps one session object per session id,
// and closing a tab never took its object out. The new tab then registered
// itself behind the old one — which the link quietly refused, because a second
// watch of the same id kept the first — so the hub's replay was handed to a
// terminal whose tab no longer existed. Nothing errored, and the pane that was
// opened looked exactly like a machine that had nothing to say.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CarlOpenLab/beacon/internal/agent"
	"github.com/CarlOpenLab/beacon/internal/daemon"
	"github.com/CarlOpenLab/beacon/internal/hub"
	"github.com/CarlOpenLab/beacon/internal/protocol"
	"github.com/CarlOpenLab/beacon/internal/relay"
	"github.com/CarlOpenLab/beacon/internal/settings"
)

func TestReopeningAClosedRemoteTabRendersTheShellAgain(t *testing.T) {
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

	// The other machine's window opens a shell and holds it.
	owner.send(protocol.Msg{T: protocol.CTCreate, Cols: 80, Rows: 24, Shell: "/bin/sh"})
	created := owner.waitFor("created", func(m protocol.Msg) bool { return m.T == protocol.STCreated })
	shell := created.SessionID
	owner.waitFor("the owner holds the lease", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleController
	})
	owner.send(protocol.Msg{T: protocol.CTInput, SessionID: shell, Data: []byte("printf 'REOPEN_BANNER\\n'\n")})

	// This window opens a tab on it and sees the shell.
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
		t.Fatal("no tab for the remote shell")
	}
	waitFor(t, "the tab to render the shell", 20*time.Second, func() bool {
		w.drain()
		return strings.Contains(tb.term.Text(), "REOPEN_BANNER")
	})

	// The view is detached without ending the shell (the cleanup path, not the
	// user-facing close), and the session is opened again from the panel a moment
	// later.
	w.removeTab(tb)
	tb2 := w.openTab("member-2", shell)
	if tb2 == nil {
		t.Fatal("no tab for the reopened shell")
	}
	if tb2 == tb {
		t.Fatal("the reopened tab is the tab that was closed")
	}
	waitFor(t, "the reopened tab to render the shell's replay", 20*time.Second, func() bool {
		w.drain()
		return strings.Contains(tb2.term.Text(), "REOPEN_BANNER")
	})

	// And it follows the live shell, not only the replay: output produced after
	// the reopen reaches the new tab.
	owner.send(protocol.Msg{T: protocol.CTInput, SessionID: shell, Data: []byte("printf 'REOPEN_LIVE\\n'\n")})
	waitFor(t, "the reopened tab to follow the live shell", 20*time.Second, func() bool {
		w.drain()
		return strings.Contains(tb2.term.Text(), "REOPEN_LIVE")
	})
}
