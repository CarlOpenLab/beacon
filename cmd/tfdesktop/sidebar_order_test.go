package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// dragRow drags from one point to another the way a pointer does: a press, a
// few moves so the framework sees a drag rather than a click, and the release.
func dragRow(tt *ui.Tester, fromX, fromY, toX, toY float32) {
	tt.Press(fromX, fromY)
	for k := 1; k <= 4; k++ {
		f := float32(k) / 4
		tt.Move(fromX+(toX-fromX)*f, fromY+(toY-fromY)*f)
	}
	tt.Release(toX, toY)
}

// A saved order is what the group draws: the hub's list stays oldest-first for
// everybody else, and this is the window's own arrangement.
func TestTheSidebarDrawsTheDraggedOrder(t *testing.T) {
	w, _ := newTestShellWindow(t, "one", "two", "three")
	w.sessionOrder = map[string][]string{machineLocal: {"s3", "s1", "s2"}}

	entries := w.entriesFor(machineLocal)
	got := []string{entries[0].label, entries[1].label, entries[2].label}
	want := []string{"three", "one", "two"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("rows = %q, want %q", got, want)
		}
	}

	// A row the saved order has never seen joins at the end instead of jumping
	// into the middle of an arrangement it knows nothing about.
	w.tabs[99] = &tab{id: "s99", machine: machineLocal, title: "four"}
	w.order = append(w.order, 99)
	entries = w.entriesFor(machineLocal)
	if last := entries[len(entries)-1].label; last != "four" {
		t.Fatalf("a new shell landed at %q, want it at the end", last)
	}
}

// Dragging the first row to the bottom of the third puts it after the third,
// and the arrangement survives in the settings file for the next launch.
func TestDraggingASessionRowReordersAndSaves(t *testing.T) {
	w, _ := newTestShellWindow(t, "one", "two", "three")
	tt := drawNav(t, w)
	tt.Frame()

	first, ok := tt.Find("one")
	if !ok {
		t.Fatalf("row one is missing; texts = %v", tt.Texts())
	}
	last, ok := tt.Find("three")
	if !ok {
		t.Fatalf("row three is missing; texts = %v", tt.Texts())
	}
	dragRow(tt, first.X+first.W/2, first.Y+first.H/2, last.X+last.W/2, last.Y+last.H-2)
	tt.Frame()

	want := []string{"s2", "s3", "s1"}
	got := w.sessionOrder[machineLocal]
	if len(got) != len(want) {
		t.Fatalf("saved order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("saved order = %v, want %v", got, want)
		}
	}
	if saved := w.store.Get().SessionOrder[machineLocal]; len(saved) != len(want) {
		t.Fatalf("settings hold %v, want the dragged order", saved)
	}

	entries := w.entriesFor(machineLocal)
	if entries[2].label != "one" {
		t.Fatalf("after the drag the rows read %q", []string{entries[0].label, entries[1].label, entries[2].label})
	}
}

// While a row is dragged the list is a preview: the row is already drawn where
// it would land, under the other rows it moved past, and nothing is written
// until the pointer lets go.
func TestDraggingShowsWhereTheRowWillLand(t *testing.T) {
	w, _ := newTestShellWindow(t, "one", "two", "three")
	tt := drawNav(t, w)
	tt.Frame()

	first, ok := tt.Find("one")
	if !ok {
		t.Fatalf("row one is missing; texts = %v", tt.Texts())
	}
	third, ok := tt.Find("three")
	if !ok {
		t.Fatalf("row three is missing; texts = %v", tt.Texts())
	}
	tt.Press(first.X+first.W/2, first.Y+first.H/2)
	for k := 1; k <= 4; k++ {
		f := float32(k) / 4
		tt.Move(first.X+first.W/2, first.Y+first.H/2+(third.Y+third.H-4-(first.Y+first.H/2))*f)
		tt.Frame()
	}

	// The row is drawn at the bottom, below the two it passed, though the
	// saved order has not changed: this is the preview.
	moved, _ := tt.Find("one")
	second, _ := tt.Find("two")
	if !(moved.Y > second.Y) {
		t.Fatalf("dragged row is at %.1f, want it below the rows it passed (two at %.1f)", moved.Y, second.Y)
	}
	if w.sessionOrder[machineLocal] != nil {
		t.Fatalf("a drag in flight wrote the order: %v", w.sessionOrder[machineLocal])
	}

	tt.Release(first.X+first.W/2, third.Y+third.H-4)
	tt.Frame()
	if got := w.sessionOrder[machineLocal]; len(got) != 3 || got[2] != "s1" {
		t.Fatalf("order after the drop = %v, want one last", got)
	}
}

// Escape gives up a drag, and the list goes back to the order it had.
func TestEscapePutsADraggedRowBack(t *testing.T) {
	w, _ := newTestShellWindow(t, "one", "two", "three")
	tt := drawNav(t, w)
	tt.Frame()

	first, _ := tt.Find("one")
	third, _ := tt.Find("three")
	tt.Press(first.X+first.W/2, first.Y+first.H/2)
	for k := 1; k <= 4; k++ {
		f := float32(k) / 4
		tt.Move(first.X+first.W/2, first.Y+first.H/2+(third.Y+third.H-4-(first.Y+first.H/2))*f)
		tt.Frame()
	}
	if moved, _ := tt.Find("one"); moved.Y < third.Y-1 {
		t.Fatalf("preview did not move the row: one at %.1f, three at %.1f", moved.Y, third.Y)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	tt.Release(first.X+first.W/2, third.Y+third.H-4)
	tt.Frame()

	if w.sessionOrder[machineLocal] != nil {
		t.Fatalf("Escape saved the order: %v", w.sessionOrder[machineLocal])
	}
	back, _ := tt.Find("one")
	if back.Y > secondRowY(t, tt) {
		t.Fatalf("after Escape row one is at %.1f, want it back at the top", back.Y)
	}
}

// Letting go somewhere the rows did not take the drag — over empty sidebar, or
// another machine — leaves the arrangement alone.
func TestLettingGoOutsideTheRowsLeavesTheOrderAlone(t *testing.T) {
	w, _ := newTestShellWindow(t, "one", "two", "three")
	tt := drawNav(t, w)
	tt.Frame()

	first, _ := tt.Find("one")
	third, _ := tt.Find("three")
	tt.Press(first.X+first.W/2, first.Y+first.H/2)
	for k := 1; k <= 4; k++ {
		f := float32(k) / 4
		tt.Move(first.X+first.W/2, first.Y+first.H/2+(third.Y+third.H+80-(first.Y+first.H/2))*f)
		tt.Frame()
	}
	tt.Release(first.X+first.W/2, third.Y+third.H+80)
	tt.Frame()

	if w.sessionOrder[machineLocal] != nil {
		t.Fatalf("a drag let go outside the rows saved an order: %v", w.sessionOrder[machineLocal])
	}
	back, _ := tt.Find("one")
	if back.Y > secondRowY(t, tt) {
		t.Fatalf("row one is still at %.1f, want the list back as it was", back.Y)
	}
}

// secondRowY is where the second row of the list is drawn: the row a drag puts
// the first one back above.
func secondRowY(t *testing.T, tt *ui.Tester) float32 {
	t.Helper()
	second, ok := tt.Find("two")
	if !ok {
		t.Fatalf("row two is missing; texts = %v", tt.Texts())
	}
	return second.Y
}
