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
