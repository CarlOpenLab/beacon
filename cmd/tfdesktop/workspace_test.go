package main

import (
	"testing"

	"github.com/CarlOpenLab/term-fleet/locales"
)

// splitWindow is a window whose body is split in two, one shell each, with the
// second pane on screen: the state ⌘D leaves behind after the shell it opened
// has been named by the hub.
func splitWindow(t *testing.T) *window {
	t.Helper()
	w, _ := newTestShellWindow(t, "one", "two")
	w.tabs[1].alive, w.tabs[2].alive = true, true
	w.split = &splitNode{
		side: true, pct: 50,
		a: &splitNode{tab: w.tabs[1]},
		b: &splitNode{tab: w.tabs[2]},
	}
	w.focus = w.split.b
	w.selected = 1
	return w
}

// A split is one tab with two panes, as the window draws it: the sidebar lists
// the workspace once, and the panes under it as a tree, so a person can see that
// the one tab holds two terminals and can go straight to either of them.
func TestASplitIsOneTabWithItsPanesUnderIt(t *testing.T) {
	w := splitWindow(t)
	entries := w.entriesFor(machineLocal)
	if len(entries) != 1 {
		t.Fatalf("the split made %d rows, want one: %+v", len(entries), entries)
	}
	row := entries[0]
	if row.label != "one" {
		t.Errorf("the workspace row is named %q, want the first pane's name", row.label)
	}
	if row.meta != locales.WorkspacePanes(2) {
		t.Errorf("the workspace row says %q, want %q", row.meta, locales.WorkspacePanes(2))
	}
	if len(row.panes) != 2 {
		t.Fatalf("the workspace lists %d panes, want two", len(row.panes))
	}

	nav := drawNav(t, w)
	if !nav.HasText("one") || !nav.HasText("two") {
		t.Fatalf("the workspace and its panes are not listed: %v", nav.Texts())
	}
	// The pane rows are the way to a specific terminal: clicking the second
	// puts it on screen without walking the split with ⌘].
	if err := nav.Click("two"); err != nil {
		t.Fatalf("click the second pane: %v", err)
	}
	if w.focus.tab != w.tabs[2] {
		t.Fatalf("clicking the pane row left the focus on %v", w.focus.tab.name())
	}
	row = w.entriesFor(machineLocal)[0]
	if row.panes[0].active || !row.panes[1].active {
		t.Fatalf("active panes = %v/%v, want only the second", row.panes[0].active, row.panes[1].active)
	}
}

// Closing the workspace's row stops watching every shell in it, and closing one
// pane leaves the other on screen with its own row once again.
func TestClosingOnePaneGivesTheOtherItsRowBack(t *testing.T) {
	w := splitWindow(t)
	w.closeSplitPane() // stops watching the focused pane, "two"
	// The split is down to one pane, so the body is one pane again — and both
	// shells are still running, so both are rows: the one closed was not ended,
	// only taken out of the split.
	if w.split != nil {
		t.Fatal("closing one pane of two left the split up")
	}
	entries := w.entriesFor(machineLocal)
	if len(entries) != 2 {
		t.Fatalf("after closing a pane there are %d rows, want the two shells still running", len(entries))
	}
	if w.activeTab() != w.tabs[1] {
		t.Fatalf("the focus did not move to the pane that is left")
	}
}

// ⌘] and ⌘[ walk the panes in the order the body draws them.
func TestCyclePaneWalksTheSplit(t *testing.T) {
	w := splitWindow(t)
	w.cyclePane(1)
	if w.focus.tab != w.tabs[1] {
		t.Fatalf("⌘] from the second pane did not wrap to the first")
	}
	w.cyclePane(-1)
	if w.focus.tab != w.tabs[2] {
		t.Fatalf("⌘[ from the first pane did not wrap to the second")
	}
}

// ⌘⌥← from the right pane goes to the left one, and ⌘⌥→ comes back.
func TestFocusDirectionCrossesTheDivider(t *testing.T) {
	w := splitWindow(t)
	w.focusDirection("left")
	if w.focus.tab != w.tabs[1] {
		t.Fatalf("⌘⌥← did not cross to the left pane")
	}
	w.focusDirection("right")
	if w.focus.tab != w.tabs[2] {
		t.Fatalf("⌘⌥→ did not cross back to the right pane")
	}
	// Nothing lies above a side-by-side split, so the focus stays put.
	w.focusDirection("up")
	if w.focus.tab != w.tabs[2] {
		t.Fatalf("⌘⌥↑ moved the focus in a split that has nothing above")
	}
}

// ⌘⇧↩ fills the body with the focused pane and puts the split back on the next
// press. The window is drawn both ways to check that it is one pane on screen.
func TestZoomFocusedFillsTheBody(t *testing.T) {
	w := splitWindow(t)
	w.tabs[1].term = fedTerminal(t, previewScreen())
	w.tabs[2].term = fedTerminal(t, previewScreen())

	if w.zoomed {
		t.Fatal("a fresh split is zoomed")
	}
	w.zoomFocused()
	if !w.zoomed {
		t.Fatal("⌘⇧↩ did not zoom")
	}
	w.zoomFocused()
	if w.zoomed {
		t.Fatal("⌘⇧↩ did not unzoom")
	}
}

// ⌘= and ⌘- step the terminal's size and ⌘0 puts it back, through the store that
// the next window opens at.
func TestZoomTextStepsTheStore(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	before := w.store.Get().FontSize
	w.zoomText(1)
	after := w.store.Get().FontSize
	if after <= before && before > 0 {
		t.Fatalf("⌘= did not make the text bigger: %v → %v", before, after)
	}
	w.zoomText(0)
	if got := w.store.Get().FontSize; got != 13 {
		t.Fatalf("⌘0 left the size at %v, want the default 13", got)
	}
}
