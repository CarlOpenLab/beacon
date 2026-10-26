//go:build !windows

package main

// A tab opened for a session that is already gone must not read as an exited
// shell. The panel draws the last list its machine pushed, so a click can reach
// a session the hub no longer has; the attach is answered no_session, and the
// tab — which never heard from the shell — used to show "会话已退出，最后几行仍可
// 读取" over a blank pane, offering a last line that does not exist. It is closed
// instead, the stale row goes with it, and the window says why.

import (
	"testing"

	"github.com/CarlOpenLab/beacon/internal/protocol"
	"github.com/CarlOpenLab/beacon/locales"
)

func TestATabOpenedOnAGoneSessionIsClosedNotMarkedExited(t *testing.T) {
	f := newTestFleet(t)
	w, _ := newTestShellWindow(t)
	w.fleet = f
	w.local = openLocalLink(f.Hub())
	w.local.onSessions = w.sessionsArrived
	w.local.onGone = w.linkGone

	// The row a stale list leaves behind: it says alive, and the hub has never
	// heard of the session.
	w.live = []protocol.Summary{{SessionID: "s9", Alive: true, Cols: 80, Rows: 24}}

	tb := w.openTab(machineLocal, "s9")
	if tb == nil {
		t.Fatal("no tab was opened for the stale row")
	}
	if tb.isEnded() {
		t.Fatal("a tab that never attached reads as an exited shell")
	}

	drainFor(t, w, func() bool { return len(w.order) == 0 })

	if got := w.notice; got != locales.AppSessionGone() {
		t.Errorf("notice = %q, want the sentence about a session that is gone", got)
	}
	for _, s := range w.live {
		if s.SessionID == "s9" {
			t.Fatal("the stale row is still listed after the hub said no_session")
		}
	}
}
