package main

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/beacon/internal/settings"
	"github.com/CarlOpenLab/beacon/internal/uitokens"
)

// Drag the session list the way a sortable list moves: the row picked up
// follows the pointer as a copy, the list closes the hole it left and opens
// one where it would land, and the rows between slide out of the way. The
// arrangement is only written when the pointer lets go; until then the list
// on screen is a preview of it.

// rowDrag is one row being dragged in one machine's group: which row, and the
// place among the other rows it is being previewed at (0 is before the first
// of them, len(others) is after the last).
type rowDrag struct {
	machine string
	key     string
	at      int
	// active says a row of this machine reported the drag this frame. A drag
	// that stops reporting it without a drop was let go somewhere else, and
	// the list goes back to the order it had.
	active bool
}

// previewEntries draws a machine's rows with the dragged row already moved to
// the place it would land. It returns the order to draw, the dragged row's
// key, and where it sits among the rows after it was lifted out.
func (w *window) previewEntries(machine string, entries []sessionEntry) ([]sessionEntry, string, int) {
	d := w.rowDrag
	if d == nil || d.machine != machine || !d.active || len(entries) < 2 {
		return entries, "", -1
	}
	from := -1
	for i := range entries {
		if sessionKey(entries[i]) == d.key {
			from = i
			break
		}
	}
	if from < 0 {
		return entries, "", -1
	}
	rest := make([]sessionEntry, 0, len(entries)-1)
	rest = append(rest, entries[:from]...)
	rest = append(rest, entries[from+1:]...)
	at := min(max(d.at, 0), len(rest))
	out := make([]sessionEntry, 0, len(entries))
	out = append(out, rest[:at]...)
	out = append(out, entries[from])
	out = append(out, rest[at:]...)
	return out, d.key, at
}

// dropIndex is where the pointer says the dragged row would land: before the
// first row whose middle is below it, among the rows the drag took out. The
// dragged row's own box is skipped, since it moves with the preview and would
// otherwise answer for itself.
func dropIndex(pointerY float32, preview []sessionEntry, tops []float32, dragKey string) int {
	at := 0
	for i, e := range preview {
		if sessionKey(e) == dragKey {
			continue
		}
		if pointerY < tops[i]+uitokens.RowH/2 {
			return at
		}
		at++
	}
	return at
}

// moveSessionIndex records the dragged row's new place among the machine's
// rows: lifted out, and put back at index at. The arrangement is this window's,
// so it is saved beside the theme rather than pushed to the hub.
func (w *window) moveSessionIndex(machine, from string, at int) {
	entries := w.entriesFor(machine)
	keys := make([]string, 0, len(entries))
	for _, e := range entries {
		keys = append(keys, sessionKey(e))
	}
	idx := -1
	for i, key := range keys {
		if key == from {
			idx = i
			break
		}
	}
	if idx < 0 {
		// The row went away under the drag: draw the list as it is.
		w.invalidate()
		return
	}
	keys = append(keys[:idx], keys[idx+1:]...)
	at = min(max(at, 0), len(keys))
	keys = append(keys[:at], append([]string{from}, keys[at:]...)...)
	if w.sessionOrder == nil {
		w.sessionOrder = make(map[string][]string)
	}
	w.sessionOrder[machine] = keys
	if w.store != nil {
		order := keys
		_, _ = w.store.Update(func(s *settings.Settings) {
			if s.SessionOrder == nil {
				s.SessionOrder = make(map[string][]string)
			}
			s.SessionOrder[machine] = order
		})
	}
	// The frame drawing now still shows the preview, with the row under the
	// pointer left invisible: the next one draws the order as it now is.
	w.invalidate()
}

// rowSlide is how the rows a drag passed move out of its way: a short spring
// with no bounce. Bounce is what made the list wobble — a row that takes its
// place in an order should stay there, not lean past it and come back — while
// the spring's continuity is what keeps a second move on the way from starting
// over: the row carries on from where it is, to wherever it is now going.
var rowSlide = ui.Animation{Spring: ui.SpringOptions{Duration: 220 * time.Millisecond}}

// placeRow puts a row where the list draws it, and slides it there from where
// it was drawn last frame when the order moved it. This is what makes the rows
// between a dragged row and its landing place glide instead of jump.
func (w *window) placeRow(el ui.Element, machine, key string, top float32, animate bool) {
	id := machine + "\x00" + key
	if w.rowTop == nil {
		w.rowTop = make(map[string]float32)
		w.rowSettle = make(map[string]int)
	}
	prev, seen := w.rowTop[id]
	w.rowTop[id] = top
	if !animate {
		// The row under the pointer is the hole, not a row: its content is
		// invisible and the list draws the slot. There is nothing to slide; the
		// copy the framework paints on the pointer is what moves.
		w.rowSettle[id] = 0
		return
	}
	if seen {
		if delta := prev - top; math.Abs(float64(delta)) > 0.5 {
			el.AnimatePose(ui.Pose{Y: delta}, rowSlide)
			w.rowSettle[id] = 24
			return
		}
	}
	if frames := w.rowSettle[id]; frames > 0 {
		el.AnimatePose(ui.Pose{}, rowSlide)
		w.rowSettle[id] = frames - 1
	}
}
