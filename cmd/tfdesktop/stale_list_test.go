package main

// A session list is a snapshot, and a snapshot can be older than a shell. The
// hub broadcasts one from every session's watcher, so a list taken while ⌘D's
// shell was still being created can land after the create answer has already
// named the waiting tab. The list does not name the fresh shell — it did not
// exist when the list was made — and the tab has no attach answer yet to say it
// is there. Reconcile used to believe the list and close the tab, which took a
// pane out of the split a moment after the person opened it.

import (
	"testing"

	"github.com/CarlOpenLab/beacon/internal/protocol"
)

// attach answers a tab's attach the way the hub does.
func attach(l *link, summary protocol.Summary) {
	l.dispatch(protocol.Msg{T: protocol.STAttached, SessionID: summary.SessionID, Session: &summary})
}

func TestASessionListOlderThanTheCreateKeepsTheFreshTab(t *testing.T) {
	w, _ := newTestShellWindow(t)
	l := newLink()
	l.name = machineLocal
	l.onSessions = w.sessionsArrived
	w.local = &localLink{link: l}

	// One running shell, with an attach behind it.
	first := w.openTab(machineLocal, "")
	l.dispatch(protocol.Msg{T: protocol.STCreated, SessionID: "s1"})
	w.drain()
	attach(l, protocol.Summary{SessionID: "s1", Title: "zsh", Alive: true, Cols: 80, Rows: 24})
	w.drain()
	if !first.everAttached() {
		t.Fatal("the first tab never took the hub's attach")
	}

	// ⌘D's fresh shell: the tab asked, and the create answer has named it, but
	// the attach has not been answered yet.
	fresh := w.openTab(machineLocal, "")
	l.dispatch(protocol.Msg{T: protocol.STCreated, SessionID: "s2"})
	w.drain()
	if fresh.id != "s2" {
		t.Fatalf("the fresh tab holds id %q, want s2", fresh.id)
	}
	if fresh.everAttached() {
		t.Fatal("the fresh tab was attached before the hub answered")
	}

	// The list that raced the attach: made before s2 existed, it names only s1.
	l.dispatch(protocol.Msg{T: protocol.STSessions, Sessions: []protocol.Summary{{
		SessionID: "s1", Title: "zsh", Alive: true, Cols: 80, Rows: 24,
	}}})
	w.drain()

	if !w.hasTab(fresh) {
		t.Fatal("the list older than the create closed the shell still being created")
	}
	if !w.hasTab(first) {
		t.Fatal("the list closed the shell it named")
	}
	if len(w.order) != 2 {
		t.Fatalf("the window holds %d tabs, want the shell and the one being created", len(w.order))
	}

	// The guard is about tabs with no attach behind them, not a way to keep
	// dead ones: once the hub has answered, a list that no longer names the tab
	// closes it as before.
	attach(l, protocol.Summary{SessionID: "s2", Alive: false})
	w.drain()
	l.dispatch(protocol.Msg{T: protocol.STSessions, Sessions: []protocol.Summary{{
		SessionID: "s1", Title: "zsh", Alive: true, Cols: 80, Rows: 24,
	}}})
	w.drain()
	if w.hasTab(fresh) {
		t.Fatal("a tab the hub had answered for outlived the list that dropped it")
	}
}
