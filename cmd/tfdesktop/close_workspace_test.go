package main

// What the window shows once a tab goes. Closing the tab on screen must leave
// the person looking at the same thing a pick of the next tab would show — a
// workspace is a layout, and a pane of it drawn alone under a sidebar row that
// says 分屏 is two answers to "what is on screen".

import (
	"testing"
)

// A workspace put away is not a session to draw alone: closing the last tab of
// the workspace on screen hands the body to the next tab, and that tab's
// workspace comes back with it.
func TestClosingTheLastTabBringsTheNextTabsWorkspaceBack(t *testing.T) {
	w, _ := newTestShellWindow(t, "solo", "one", "two")
	ws := &workspace{
		split: &splitNode{
			side: true, pct: 50,
			a: &splitNode{tab: w.tabs[2]},
			b: &splitNode{tab: w.tabs[3]},
		},
	}
	ws.focus = ws.split.a
	w.tabs[2].ws, w.tabs[3].ws = ws, ws
	w.selected = 0 // "solo" is the tab on screen

	w.closeTab(w.tabs[1])

	if w.split != ws.split {
		t.Fatal("closing the last tab put a lone pane on screen instead of its workspace")
	}
	if w.ws != ws {
		t.Fatal("the workspace on screen is not the one the body draws")
	}
	if got := w.activeTab(); got != w.tabs[2] {
		t.Fatalf("the body shows %v, want the workspace's focused pane", got)
	}
	if got := w.entriesFor(machineLocal); len(got) != 1 || !got[0].active || len(got[0].panes) != 2 {
		t.Fatalf("the sidebar does not list the restored workspace as the one on screen: %+v", got)
	}
}

// Closing a tab above the one on screen keeps the shell on screen. The strip is
// an order, not a position: an earlier tab going must not move the person to
// the next shell — which, when that shell belongs to a workspace, is how a
// close used to land on a session of a workspace nobody had switched to.
func TestClosingATabAboveTheOneOnScreenKeepsItOnScreen(t *testing.T) {
	w, _ := newTestShellWindow(t, "a", "b", "c")
	w.selected = 1 // "b" is the tab on screen
	on := w.activeTab()

	w.closeTab(w.tabs[1]) // "a", above it

	if got := w.activeTab(); got != on {
		t.Fatalf("closing a tab above the one on screen moved to %v, want %v", got, on)
	}
	if w.selected != 0 {
		t.Fatalf("the strip points at %d, want the surviving tab at 0", w.selected)
	}
}
