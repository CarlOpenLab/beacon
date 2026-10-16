package main

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// The rail's selection is the id of the shell on screen, prefixed so that one
// field can carry what the column of shells asked for.
const (
	railTabPrefix = "tab:"

	// sidebarWidth fits a shell's label and where it is working.
	sidebarWidth = 216
)

// sidebar draws the column of shells, the one place a shell is chosen. It is
// rebuilt from the window's own state on every frame, so a shell that appears —
// started here, or taken over from another machine later — shows up as soon as
// its tab does.
func (w *window) sidebar(c *ui.Context, labels []string, mode string) {
	t := c.Theme()
	col := shellColorsFor(t)

	// The column draws with the window's selection field and that field is never
	// seeded here: the click that changes the selection is written into the very
	// pointer the column was registered with, so overwriting it before the click
	// is read back would swallow the click. When the field names a shell that is
	// still open it *is* the window's idea of the shell on screen.
	if w.indexOfTab(strings.TrimPrefix(w.rail, railTabPrefix)) < 0 {
		w.rail = ""
		if id := w.activeID(); id != "" {
			w.rail = railTabPrefix + id
		}
	}

	rail := ui.Sidebar(c, &w.rail, func() {
		ui.SidebarSection(c, "会话", nil, func() {
			if len(labels) == 0 {
				ui.Text(c, "还没有终端").FontSize(12).TextColor(t.TextMuted)
				return
			}
			shown := 0
			for _, id := range w.order {
				if w.tabs[id] == nil {
					continue
				}
				label := id
				if shown < len(labels) {
					label = labels[shown]
				}
				shown++
				ui.SidebarItem(c, railTabPrefix+id, iconTerminal, label)
			}
		})
	}).
		Width(sidebarWidth).FillHeight().
		Background(col.pane).
		BorderWidth(0, 1, 0, 0).BorderColor(col.hairline)

	switch {
	case rail.Changed():
		w.railAction(c, mode)
	case w.rail != w.selectionID():
		// The shell on screen changed somewhere else — one was opened or closed, or
		// a shell was taken over — so the column follows it on the next frame.
		w.rail = w.selectionID()
	}
}

// selectionID is the column's id for the shell on screen, or "" for none.
func (w *window) selectionID() string {
	if id := w.activeID(); id != "" {
		return railTabPrefix + id
	}
	return ""
}

// railAction runs what the column asked for. A click was written into the
// selection field while the column was being drawn, and the column reports it in
// the same frame, so the choice takes effect at once.
func (w *window) railAction(c *ui.Context, mode string) {
	if !strings.HasPrefix(w.rail, railTabPrefix) {
		return
	}
	if i := w.indexOfTab(strings.TrimPrefix(w.rail, railTabPrefix)); i >= 0 {
		w.selected = i
	}
}

// activeID is the session id of the shell on screen, or "" when there is none.
func (w *window) activeID() string {
	if w.selected < 0 || w.selected >= len(w.order) {
		return ""
	}
	return w.order[w.selected]
}

// indexOfTab is where a session sits in the tab order, or -1.
func (w *window) indexOfTab(id string) int {
	for i, other := range w.order {
		if other == id {
			return i
		}
	}
	return -1
}
