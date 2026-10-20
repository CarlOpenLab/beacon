package main

import (
	"strings"
	"testing"

	"github.com/CarlOpenLab/term-fleet/internal/protocol"
	"github.com/CarlOpenLab/term-fleet/internal/settings"
	"github.com/CarlOpenLab/term-fleet/locales"
)

// notifyWindow is a window whose notifications are collected instead of shown:
// the decision is what these tests are about, and a test has no window server to
// put a banner on.
func notifyWindow(t *testing.T) (*window, *[]string) {
	t.Helper()
	w, _ := newTestShellWindow(t, "one")
	got := &[]string{}
	w.onNotify = func(title, body string) {
		*got = append(*got, title+"｜"+body)
	}
	return w, got
}

func state(status, kind, message string, unread bool) []protocol.Summary {
	return []protocol.Summary{{
		SessionID: "s1", Title: "review -- fix login", Status: status,
		Kind: kind, Message: message, Unread: unread, Alive: true,
	}}
}

// A notification is about a change: the first list a window sees describes what
// was already true when it opened, and saying it would be news about the past.
func TestTheFirstListSaysNothing(t *testing.T) {
	w, got := notifyWindow(t)
	w.reportStatuses(machineLocal, state("waiting", "permission", "Approve?", true))
	if len(*got) != 0 {
		t.Fatalf("the first list notified: %v", *got)
	}
}

// Working → waiting is the notification this fleet exists for, and the message
// the program gave is its body.
func TestComingToNeedYouNotifies(t *testing.T) {
	w, got := notifyWindow(t)
	w.reportStatuses(machineLocal, state("working", "", "", false))
	w.reportStatuses(machineLocal, state("waiting", "permission", "Approve the deploy?", true))
	if len(*got) != 1 {
		t.Fatalf("got %d notifications, want one: %v", len(*got), *got)
	}
	if !strings.Contains((*got)[0], locales.NotifyNeedsYouTitle("")) || !strings.Contains((*got)[0], "Approve the deploy?") {
		t.Fatalf("the notification reads %q", (*got)[0])
	}

	// The same state again, with a new message, is not a new event.
	w.reportStatuses(machineLocal, state("waiting", "permission", "Approve the deploy?", true))
	if len(*got) != 1 {
		t.Fatalf("a repeated waiting notified again: %v", *got)
	}
}

// A kind with no message says what the kind means.
func TestWaitingWithoutAMessageNamesItsKind(t *testing.T) {
	w, got := notifyWindow(t)
	w.reportStatuses(machineLocal, state("working", "", "", false))
	w.reportStatuses(machineLocal, state("waiting", "question", "", true))
	if len(*got) != 1 || !strings.Contains((*got)[0], locales.StatusHasQuestion()) {
		t.Fatalf("notifications = %v", *got)
	}
}

// Failed and finished are the other two: an error the program reported, and an
// agent that stopped working while the result went unseen.
func TestFailingAndFinishingNotify(t *testing.T) {
	w, got := notifyWindow(t)
	w.reportStatuses(machineLocal, state("working", "", "", false))
	w.reportStatuses(machineLocal, state("error", "", "build failed", true))
	if len(*got) != 1 || !strings.Contains((*got)[0], locales.NotifyFailedTitle("")) {
		t.Fatalf("a failure did not notify: %v", *got)
	}
	w.reportStatuses(machineLocal, state("working", "", "", false))
	w.reportStatuses(machineLocal, state("idle", "", "", true))
	if len(*got) != 2 || !strings.Contains((*got)[1], locales.NotifyDoneTitle("")) {
		t.Fatalf("finishing did not notify: %v", *got)
	}
	// Idle again, still unread: nothing new to say.
	w.reportStatuses(machineLocal, state("idle", "", "", true))
	if len(*got) != 2 {
		t.Fatalf("idle repeated notified: %v", *got)
	}
}

// Each of the three can be turned off on its own, and the Dock badge with them.
func TestTheSwitchesSilenceTheirOwnEvent(t *testing.T) {
	for _, tc := range []struct {
		name string
		off  func(*settings.Settings)
		from string
		to   string
	}{
		{"waiting", func(s *settings.Settings) { s.NotifyWaitingOff = true }, "working", "waiting"},
		{"error", func(s *settings.Settings) { s.NotifyErrorOff = true }, "working", "error"},
		{"done", func(s *settings.Settings) { s.NotifyDoneOff = true }, "working", "idle"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, got := notifyWindow(t)
			if _, err := w.store.Update(tc.off); err != nil {
				t.Fatal(err)
			}
			w.reportStatuses(machineLocal, state(tc.from, "", "", false))
			w.reportStatuses(machineLocal, state(tc.to, "", "", true))
			if len(*got) != 0 {
				t.Fatalf("%s off still notified: %v", tc.name, *got)
			}
		})
	}
}
