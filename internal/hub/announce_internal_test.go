//go:build !windows

package hub

// A create is answered on the asking client's own connection, but the session
// list is broadcast by every watcher in the process: a session another shell's
// status change puts on the wire can carry the new shell before the asking
// client is told which one it got. That client then mounts the shell the list
// names while its waiting tab is still unnamed, and the created answer binds
// the same shell again — two tabs for one shell, one of them blank.
//
// The fix is the invariant these tests hold the hub to: a session is in no list
// until its create has been answered on the wire.

import (
	"testing"
	"time"

	"github.com/CarlOpenLab/beacon/internal/protocol"
)

func newBareHub(t *testing.T) *Hub {
	t.Helper()
	h := New(Options{Host: protocol.Host{Hostname: "test", Platform: "test", Arch: "test"}})
	t.Cleanup(func() { h.Manager().Shutdown() })
	return h
}

func TestASessionIsListedOnlyAfterItsCreateIsAnnounced(t *testing.T) {
	h := newBareHub(t)

	s, reused, err := h.create("", 80, 24, "", "/bin/sh", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if reused {
		t.Fatal("a fresh id reused a live session")
	}

	// The shell exists, and no list may name it yet: the asking client has not
	// been told its id.
	if got := h.listSummaries(); len(got) != 0 {
		t.Fatalf("the list named %d sessions before the created answer: %v", len(got), got)
	}

	h.announce(s.ID)
	got := h.listSummaries()
	if len(got) != 1 || got[0].SessionID != s.ID {
		t.Fatalf("the announced session is not listed: %v", got)
	}
}

// An ended session leaves the announced set with the manager entry: a later
// shell made under the same id must wait for its own answer like any other.
func TestAnEndedSessionIsForgottenByTheList(t *testing.T) {
	h := newBareHub(t)

	s, _, err := h.create("s1", 80, 24, "", "/bin/sh", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	h.announce(s.ID)

	// The real path: the watcher finalises the session when its process ends.
	go h.watchSession(s)
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, ok := h.Manager().Get("s1"); !ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the hub never forgot the ended session")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := h.listSummaries(); len(got) != 0 {
		t.Fatalf("the list still names the ended session: %v", got)
	}

	again, reused, err := h.create("s1", 80, 24, "", "/bin/sh", "")
	if err != nil {
		t.Fatalf("create again: %v", err)
	}
	if reused {
		t.Fatal("the ended session was reused")
	}
	if got := h.listSummaries(); len(got) != 0 {
		t.Fatalf("a reused id is listed before its create is announced: %v", got)
	}
	h.announce(again.ID)
	if got := h.listSummaries(); len(got) != 1 || got[0].SessionID != again.ID {
		t.Fatalf("the re-made session is not listed after its announce: %v", got)
	}
}
