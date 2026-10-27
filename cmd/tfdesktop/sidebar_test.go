package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/beacon/internal/protocol"
	"github.com/CarlOpenLab/beacon/locales"
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
	// The refresh waits for the pointer — one mark at rest is what a 268-point
	// header can carry — while the + that opens a shell stays out, because it is
	// the group's usual action and not its repair.
	if tt.HasText(locales.SessionRefreshMachine()) {
		t.Fatalf("the group shows its refresh at rest; texts = %v", tt.Texts())
	}
	if !tt.HasText(locales.SessionNewOnMachine()) {
		t.Fatalf("the group has no new-terminal button at rest; texts = %v", tt.Texts())
	}
	name := w.machineDisplayName(machineLocal)
	header, ok := tt.Find(name)
	if !ok {
		t.Fatalf("this machine's group is missing; texts = %v", tt.Texts())
	}
	tt.Move(header.X+4, header.Y+4)
	tt.Frame()
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
	if w.collapsed[machineLocal] {
		t.Error("clicking refresh folded the machine's group")
	}

	// The button answers the click on the row itself: it turns while the list is
	// on its way, so a click that was taken is not confused with one that did
	// nothing.
	if !w.refreshing(machineLocal) {
		t.Fatal("the refresh did not turn after the click")
	}
	tt.Frame()
	if !w.refreshing(machineLocal) {
		t.Error("a frame stopped the refresh before the list arrived")
	}

	// The list landing is what stops it — not a fixed while, and not a second
	// click.
	w.sessionsArrived(nil)
	w.drain()
	if w.refreshing(machineLocal) {
		t.Error("the refresh kept turning after the machine's list arrived")
	}

	// A machine that never answers stops turning too: the mark has a lifetime, so
	// an unreachable machine cannot leave a ring turning for the rest of the
	// session.
	w.refreshRequested(machineLocal)
	w.refreshAt[machineLocal] = time.Now().Add(-refreshSpin - time.Second)
	if w.refreshing(machineLocal) {
		t.Error("the refresh is still turning past its lifetime")
	}
}

// A tab's row keeps its state at rest — and its close under the pointer — and carries no clock:
// the time since a state began is a fact for the tooltip, not a column of the
// sidebar. The state is a dot or the working ring, never a checkmark.
func TestATabRowKeepsItsCloseAndStateWithoutAClock(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	w.tabs[1].alive = true
	w.live = []protocol.Summary{{
		SessionID: "s1", Title: "one", Alive: true, Status: "waiting",
		Since: time.Now().Add(-5 * time.Minute).UnixMilli(),
	}}

	nav := drawNav(t, w)
	if !nav.HasText("needs you") {
		t.Fatalf("a waiting tab has no needs-you mark: %v", nav.Texts())
	}
	if _, ok := nav.Find("5m"); ok {
		t.Errorf("the tab row still shows its age: %v", nav.Texts())
	}
	// The close is the one act that ends a running shell, so it waits for the
	// pointer; the row keeps its square, so the name does not move when it comes.
	if _, ok := nav.Find(locales.TabCloseNamed("one")); ok {
		t.Errorf("the tab row shows its close at rest: %v", nav.Texts())
	}
	row, ok := nav.Find("one")
	if !ok {
		t.Fatalf("the tab's row is missing; texts = %v", nav.Texts())
	}
	nav.Move(row.X+4, row.Y+4)
	nav.Frame()
	if _, ok := nav.Find(locales.TabCloseNamed("one")); !ok {
		t.Errorf("the tab row has no close under the pointer: %v", nav.Texts())
	}

	// Finished, unseen: a green dot (the done mark). Seen: nothing, not a check.
	w.live[0].Status, w.live[0].Unread = "idle", true
	nav = drawNav(t, w)
	if !nav.HasText("done") {
		t.Fatalf("a finished, unseen tab has no done mark: %v", nav.Texts())
	}
	w.live[0].Unread = false
	nav = drawNav(t, w)
	if nav.HasText("done") || nav.HasText("needs you") {
		t.Errorf("a tab at rest still wears a state mark: %v", nav.Texts())
	}
}
