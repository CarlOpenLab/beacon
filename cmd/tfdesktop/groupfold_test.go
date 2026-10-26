package main

import (
	"testing"
	"time"

	"github.com/CarlOpenLab/beacon/internal/tunnel"
)

// A group the person folds by hand takes its rows with it: the group below
// slides up over the fold instead of the rows vanishing between two frames.
func TestAMachineGroupFoldsItsRowsWithAnAnimation(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	w.fleet = newTestFleet(t)
	w.collapsed = make(map[string]bool)
	w.foldByHand = make(map[string]bool)
	w.roster = []tunnel.Machine{{ID: "m2", Name: "second"}}

	tt := drawNav(t, w)
	tt.Frame()
	start, ok := tt.Find("second")
	if !ok {
		t.Fatalf("the second machine is not in the column; texts = %v", tt.Texts())
	}

	// The person clicked the first group's header: its rows leave, and the
	// second group is on its way up when the next frame lands.
	w.foldByHand[machineLocal] = true
	w.collapsed[machineLocal] = true
	tt.Frame()
	tt.Advance(60 * time.Millisecond)
	mid, ok := tt.Find("second")
	if !ok {
		t.Fatalf("the second machine left the column while the first folded")
	}
	if mid.Y >= start.Y {
		t.Fatalf("60ms into the fold the second group is at y=%.1f, want above %.1f", mid.Y, start.Y)
	}

	tt.Advance(300 * time.Millisecond)
	end, ok := tt.Find("second")
	if !ok {
		t.Fatalf("the second machine left the column")
	}
	if end.Y >= mid.Y {
		t.Fatalf("the fold stopped at y=%.1f and rests at %.1f", mid.Y, end.Y)
	}

	// A group the window opens for the person — a jump into one of its
	// sessions — is there on the next frame, with no fold to watch: the column
	// the person turned back to is already the one they expect.
	w.collapsed[machineLocal] = false
	tt.Frame()
	tt.Advance(20 * time.Millisecond)
	now, ok := tt.Find("second")
	if !ok {
		t.Fatalf("the second machine left the column")
	}
	if now.Y != start.Y {
		t.Fatalf("the group opened to y=%.1f, want it back at %.1f on the same frame", now.Y, start.Y)
	}
}
