//go:build !windows

package main

// The reported failure, deterministically: the relay link drops while a tab on
// another machine's shell is open (a relay restart, a slow-viewer eviction, a
// member flap — all of which end the stream), the machine comes back, and the
// person clicks the session row again. The tab must re-attach and follow the
// live shell; a tab orphaned on the dead link renders nothing new, forever.

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

func TestDeadRemoteTabReattachesOnClick(t *testing.T) {
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

	agentCtx, stopAgent := context.WithCancel(ctx)
	defer stopAgent()
	startAgent := func() {
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
		go func() { _ = ag.Run(agentCtx) }()
	}
	startAgent()

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
	// shell, held by the machine's own client.
	owner.send(protocol.Msg{T: protocol.CTCreate, Cols: 80, Rows: 24, Shell: "/bin/sh"})
	created := owner.waitFor("created", func(m protocol.Msg) bool { return m.T == protocol.STCreated })
	shell := created.SessionID
	owner.waitFor("the owner holds the lease", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleController
	})
	owner.send(protocol.Msg{T: protocol.CTInput, SessionID: shell, Data: []byte("printf 'FIRST_BANNER\\n'\n")})

	// The window looks at the machine and opens a tab on the shell.
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
		return strings.Contains(tb.term.Text(), "FIRST_BANNER")
	})

	// The relay goes away — a restart, or the slow-viewer eviction a busy
	// session provokes. Every stream ends; the member redials.
	rl.Shutdown("relay restarted")
	waitFor(t, "the window's link to the machine to be dropped", 20*time.Second, func() bool {
		w.drain()
		_, ok := w.remote["member-2"]
		return !ok
	})
	waitFor(t, "the machine to be back on the roster", 20*time.Second, func() bool {
		w.drain()
		for _, m := range w.roster {
			if m.ID == "member-2" && m.Online {
				return true
			}
		}
		return false
	})

	// The shell keeps running on its machine — the session outlived the
	// outage — and prints something new.
	owner.send(protocol.Msg{T: protocol.CTInput, SessionID: shell, Data: []byte("printf 'AFTER_RECONNECT\\n'\n")})

	// The person clicks the session row again: the tab that is already open is
	// the one watching that shell, so the click lands on it.
	w.openOrSelect("member-2", shell)

	// The tab follows the live shell again: the new output reaches it, which
	// only a re-attach can do — the link it was on is gone.
	waitFor(t, "the tab to render output produced after the outage", 20*time.Second, func() bool {
		w.drain()
		return strings.Contains(tb.term.Text(), "AFTER_RECONNECT")
	})
}

// The same outage, healed without a click: the window looks at the machine
// again — picking it in the panel, refreshing its sessions — and the tabs the
// dead link left behind come back on the new stream by themselves, the way the
// browser client's reconnect re-attaches its tabs.
func TestDeadRemoteTabReattachesWhenMachineIsLookedAt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

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

	agentCtx, stopAgent := context.WithCancel(ctx)
	defer stopAgent()
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
	go func() { _ = ag.Run(agentCtx) }()

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

	owner.send(protocol.Msg{T: protocol.CTCreate, Cols: 80, Rows: 24, Shell: "/bin/sh"})
	created := owner.waitFor("created", func(m protocol.Msg) bool { return m.T == protocol.STCreated })
	shell := created.SessionID
	owner.waitFor("the owner holds the lease", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleController
	})
	owner.send(protocol.Msg{T: protocol.CTInput, SessionID: shell, Data: []byte("printf 'FIRST_BANNER\\n'\n")})

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
		return strings.Contains(tb.term.Text(), "FIRST_BANNER")
	})

	// The relay goes away and comes back, the way a restart or a member flap
	// looks from here.
	rl.Shutdown("relay restarted")
	waitFor(t, "the window's link to the machine to be dropped", 20*time.Second, func() bool {
		w.drain()
		_, ok := w.remote["member-2"]
		return !ok
	})
	waitFor(t, "the machine to be back on the roster", 20*time.Second, func() bool {
		w.drain()
		for _, m := range w.roster {
			if m.ID == "member-2" && m.Online {
				return true
			}
		}
		return false
	})

	// Nobody clicks the session row. The window looks at the machine again —
	// what picking it in the 机器 panel does — and the tab is re-attached by
	// that alone.
	owner.send(protocol.Msg{T: protocol.CTInput, SessionID: shell, Data: []byte("printf 'HEALED_WITHOUT_CLICK\\n'\n")})
	w.ensureSessionList("member-2")
	waitFor(t, "the tab to render output produced after the outage", 20*time.Second, func() bool {
		w.drain()
		return strings.Contains(tb.term.Text(), "HEALED_WITHOUT_CLICK")
	})
}
