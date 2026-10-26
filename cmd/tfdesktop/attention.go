package main

import (
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/beacon/internal/uitokens"
	"github.com/CarlOpenLab/beacon/locales"
)

// Attention: what needs the user, what is busy, and what finished while nobody
// was looking. The counts sit in the sidebar's foot; ⌘J and a click on a count
// walk the sessions that answer to them, wrapping, and expanding a folded group
// on the way so that the session it lands on is the session you can see.
//
// These are the three questions a fleet exists to answer, and they are answered
// from the status the daemon reports (internal/status), not guessed here.

func (w *window) allEntries() []sessionEntry {
	var out []sessionEntry
	for _, m := range w.groups() {
		out = append(out, w.entriesFor(m.ID)...)
	}
	return out
}

func (w *window) statusCounts() (waiting, working, done int) {
	for _, e := range w.allEntries() {
		switch {
		case e.status == "waiting":
			waiting++
		case e.status == "working":
			working++
		case e.status == "idle" && e.unread:
			done++
		}
	}
	return waiting, working, done
}

// matches says whether a row answers to a kind of attention.
func matches(e sessionEntry, kind string) bool {
	switch kind {
	case "waiting":
		return e.status == "waiting"
	case "working":
		return e.status == "working"
	case "idle":
		return e.status == "idle" && e.unread
	}
	return false
}

// jumpTo goes to the next session of a kind, after the one on screen, wrapping.
func (w *window) jumpTo(kind string) {
	entries := w.allEntries()
	if len(entries) == 0 {
		return
	}
	active := -1
	for i, e := range entries {
		if e.active {
			active = i
			break
		}
	}
	for off := 1; off <= len(entries); off++ {
		i := (active + off + len(entries)) % len(entries)
		if matches(entries[i], kind) {
			w.reveal(entries[i])
			return
		}
	}
	switch kind {
	case "waiting":
		w.setNotice(locales.SidebarNothingNeedsYou())
	case "working":
		w.setNotice(locales.SidebarNothingWorking())
	default:
		w.setNotice(locales.SidebarNothingDone())
	}
}

// jumpToAttention is ⌘J: the next session that needs the user, else the next
// one that finished unseen.
func (w *window) jumpToAttention() {
	if waiting, _, _ := w.statusCounts(); waiting == 0 {
		if _, _, done := w.statusCounts(); done == 0 {
			w.setNotice(locales.SidebarNothingNeedsYou())
			return
		}
		w.jumpTo("idle")
		return
	}
	w.jumpTo("waiting")
}

// reveal opens the session a row is about, on its machine, with its group
// unfolded: a jump that lands on a folded group has gone nowhere.
func (w *window) reveal(e sessionEntry) {
	delete(w.collapsed, e.machine)
	if w.selectedMachine() != e.machine {
		if m, ok := w.machineByID(e.machine); ok {
			w.selectMachine(m)
		}
	}
	e.pick()
	w.invalidate()
}

// attentionChip is what a session wants, in a card's header: the message the
// program gave, or what its kind of block means, or how far along it is.
func (w *window) attentionChip(c *ui.Context, pal *uitokens.Palette, tb *tab) {
	if tb == nil || tb.id == "" {
		return
	}
	sum, ok := w.summaryOf(tb.machine, tb.id)
	if !ok {
		return
	}
	switch sum.Status {
	case "waiting":
		msg := sum.Message
		if msg == "" {
			switch sum.Kind {
			case "permission":
				msg = locales.StatusNeedsPermission()
			case "question":
				msg = locales.StatusHasQuestion()
			case "auth":
				msg = locales.StatusNeedsSignIn()
			default:
				msg = locales.StatusWaiting()
			}
		}
		chipBox(c, pal, pal.AttnSoft, pal.Attention, func() {
			statusGlyph(c, pal, "waiting", true, -1, 11)
			ui.Text(c, msg).FontSize(12).FontWeight(500).SingleLine().Ellipsis("…").MaxWidth(320)
		})
	case "working":
		chipBox(c, pal, pal.Chip, pal.TextMuted, func() {
			statusGlyph(c, pal, "working", false, sum.Progress, 11)
			label := ago(sum.Since, c.Now())
			if sum.Progress >= 0 {
				label = itoa(sum.Progress) + "%"
			}
			if sum.Message != "" {
				label = sum.Message + " · " + label
			}
			ui.Text(c, label).FontSize(12).FontFeatures("tnum").SingleLine().Ellipsis("…").MaxWidth(320)
		})
	case "error":
		chipBox(c, pal, pal.Danger.Alpha(0.1), pal.Danger, func() {
			statusGlyph(c, pal, "error", true, -1, 11)
			msg := sum.Message
			if msg == "" {
				msg = locales.StatusFailed()
			}
			ui.Text(c, msg).FontSize(12).FontWeight(500).SingleLine().Ellipsis("…").MaxWidth(320)
		})
	}
}
