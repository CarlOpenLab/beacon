package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/locales"
)

// The column folds rather than disappearing: one frame after the toggle still
// shows part of it, and the cards beside it keep moving while it does.
func TestTheSidebarFoldsWithAnAnimation(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	w.fleet = newTestFleet(t)

	tt := ui.NewTester(w.view, 900, 600)
	tt.Frame()
	if got := w.chrome.shown; got != 1 {
		t.Fatalf("the open column is shown %.2f of its width, want 1", got)
	}

	w.sidebarHidden = true
	tt.Frame()
	tt.Advance(80 * time.Millisecond)
	if got := w.chrome.shown; got <= 0.01 || got >= 0.99 {
		t.Fatalf("80ms after folding, the column is shown %.2f of its width, want a frame in between", got)
	}

	tt.Advance(400 * time.Millisecond)
	if got := w.chrome.shown; got > 0.01 {
		t.Fatalf("the closed column is still shown %.2f of its width, want 0", got)
	}

	// And it comes back the same way.
	w.sidebarHidden = false
	tt.Frame()
	tt.Advance(80 * time.Millisecond)
	if got := w.chrome.shown; got <= 0.01 || got >= 0.99 {
		t.Fatalf("80ms after unfolding, the column is shown %.2f of its width, want a frame in between", got)
	}
}

// Every machine's group carries its own refresh, at the right of its header and
// left of the new-terminal button: the two actions of that row, in that order.
func TestAMachineGroupRefreshesItsSessions(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	w.fleet = newTestFleet(t)

	tt := drawNav(t, w)
	name := w.machineName(machineLocal)
	header, ok := tt.Find(name)
	if !ok {
		t.Fatalf("this machine's group is missing; texts = %v", tt.Texts())
	}
	tt.Move(header.X+4, header.Y+4)
	tt.Frame()

	if !tt.HasText(locales.SessionRefreshMachine()) {
		t.Fatalf("the group has no refresh button; texts = %v", tt.Texts())
	}
	refresh, ok := tt.Find(locales.SessionRefreshMachine())
	if !ok {
		t.Fatalf("the refresh button is not on screen; texts = %v", tt.Texts())
	}
	plus, ok := tt.Find(locales.SessionNewOnMachine())
	if !ok {
		t.Fatalf("the group has no new-terminal button; texts = %v", tt.Texts())
	}
	if refresh.X >= plus.X {
		t.Errorf("refresh is at x=%v, new terminal at x=%v, want refresh on the left", refresh.X, plus.X)
	}
	if err := tt.Click(locales.SessionRefreshMachine()); err != nil {
		t.Fatal(err)
	}
	if w.notice != locales.AppRefreshedLocal() {
		t.Errorf("notice = %q, want this machine's sessions refreshed", w.notice)
	}
}
