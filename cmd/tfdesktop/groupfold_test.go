package main

import (
	"testing"
	"time"
)

// A group the person folds by hand takes its rows with it: they are clipped and
// faded over the fold rather than vanishing between two frames. A group the
// window opens for the person — a jump into one of its sessions — is there on
// the next frame, with no fold to watch: the column the person turned back to is
// already the one they expect.
func TestAMachineGroupFoldsItsRowsWithAnAnimation(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	w.fleet = newTestFleet(t)
	w.collapsed = make(map[string]bool)
	w.foldByHand = make(map[string]bool)

	tt := drawNav(t, w)
	tt.Frame()
	start, ok := tt.Find("one")
	if !ok {
		t.Fatalf("the shell is not in the column; texts = %v", tt.Texts())
	}

	// The person clicked the group's header: its rows leave over the fold.
	w.foldByHand[machineLocal] = true
	w.collapsed[machineLocal] = true
	tt.Frame()
	tt.Advance(60 * time.Millisecond)
	if _, ok := tt.Find("one"); !ok {
		t.Fatal("the rows left in one frame, with no fold to watch")
	}

	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	if _, ok := tt.Find("one"); ok {
		t.Fatalf("the rows stayed after the fold; texts = %v", tt.Texts())
	}

	w.collapsed[machineLocal] = false
	tt.Frame()
	now, ok := tt.Find("one")
	if !ok {
		t.Fatalf("the group did not open for the window; texts = %v", tt.Texts())
	}
	if now.Y != start.Y {
		t.Fatalf("the group opened to y=%.1f, want it back at %.1f on the same frame", now.Y, start.Y)
	}
}
