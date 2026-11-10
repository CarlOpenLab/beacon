package main

// The create answer and the session list both name a new shell. When the list
// comes first — an older hub can broadcast it from another session's watcher
// between making the shell and answering the create — the waiting tab has no id
// yet, so the list mounts the shell as a second tab. The created answer then
// binds the same shell to the waiting tab, and the link routes its frames to
// one of the two: the other is a blank pane that never fills.

import (
	"testing"

	"github.com/CarlOpenLab/beacon/internal/protocol"
)

func TestASessionListThatBeatTheCreateAnswerLeavesOneTab(t *testing.T) {
	w, _ := newTestShellWindow(t)
	l := newLink()
	l.name = machineLocal
	l.onSessions = w.sessionsArrived
	w.local = &localLink{link: l}

	waiting := w.openTab(machineLocal, "")
	if waiting == nil {
		t.Fatal("the window opened no waiting tab")
	}

	// The list that raced the answer: it names the shell before the hub has
	// told the waiting tab which shell it got.
	l.dispatch(protocol.Msg{T: protocol.STSessions, Sessions: []protocol.Summary{{
		SessionID: "s7", Title: "zsh", Alive: true, Cols: 80, Rows: 24,
	}}})
	w.drain()
	if len(w.order) != 2 {
		t.Fatalf("the raced list mounted %d tabs, want the second one to be the bug", len(w.order))
	}

	// The answer arrives and names the shell for the tab that asked.
	l.dispatch(protocol.Msg{T: protocol.STCreated, SessionID: "s7"})
	w.drain()

	if len(w.order) != 1 {
		t.Fatalf("one shell is in %d tabs, want one", len(w.order))
	}
	if got := w.tabs[w.order[0]]; got != waiting {
		t.Fatal("the tab kept is not the one that asked for the shell")
	}
	if w.activeTab() != waiting {
		t.Fatal("the tab that asked for the shell is not the one on screen")
	}
	if waiting.id != "s7" {
		t.Fatalf("the waiting tab holds id %q, want s7", waiting.id)
	}
	if got := l.session("s7"); got != waiting.sess {
		t.Fatal("the link routes s7 to a session no tab draws")
	}
}
