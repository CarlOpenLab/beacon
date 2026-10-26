//go:build !windows

package main

// A tab closed somewhere else must leave no tab here. The exit frame that a
// close produces is shared by every client, and an ordinary exit is deliberately
// kept so its last lines can be read — so the frame has to say which of the two
// it is. This test drives the whole way: a second client on the hub claims the
// shell, closes it with dismiss, and the window that was watching drops its tab
// without being touched.

import (
	"testing"

	"github.com/CarlOpenLab/beacon/internal/hub"
	"github.com/CarlOpenLab/beacon/internal/protocol"
)

func TestACloseFromAnotherClientDropsThisWindowsTab(t *testing.T) {
	f := newTestFleet(t)
	w, _ := newTestShellWindow(t)
	w.fleet = f
	w.local = openLocalLink(f.Hub())
	w.local.onSessions = w.sessionsArrived
	w.local.onGone = w.linkGone

	tb := w.openTab(machineLocal, "")
	w.local.create(80, 24, "", "", "")
	waitSessions(f, 1)
	settleTabs(t, w, 1)
	w.drain()

	// The other client: a phone or another window, on the same hub.
	pipe := hub.NewPipe()
	defer pipe.Close()
	go f.Hub().ServeLocal(pipe.Local())
	other := newPipeClient(t, pipe)
	other.send(protocol.Msg{T: protocol.CTHello, V: protocol.Version})
	other.waitFor("hello", func(m protocol.Msg) bool { return m.T == protocol.STHello })
	other.send(protocol.Msg{T: protocol.CTAttach, SessionID: tb.id, Cursor: 0})
	other.waitFor("attached", func(m protocol.Msg) bool { return m.T == protocol.STAttached })

	// Ending needs the lease, so the viewer takes it first — what both clients
	// do before a close, and the steal is the part a person asked for.
	other.send(protocol.Msg{T: protocol.CTClaim, SessionID: tb.id, Steal: true})
	other.waitFor("the other client holds the shell", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleController
	})
	other.send(protocol.Msg{T: protocol.CTClose, SessionID: tb.id, Dismiss: true})

	drainFor(t, w, func() bool { return len(w.order) == 0 })

	if got := liveCount(f); got != 0 {
		t.Fatalf("the close left %d live shells, want the process gone", got)
	}
	// And the roster that follows must not put it back.
	w.reconcile(f.Hub().Sessions())
	if len(w.order) != 0 {
		t.Fatalf("a closed tab came back on the next roster: %d tabs", len(w.order))
	}
}
