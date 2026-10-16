package main

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// The rail's selection is an id rather than a tab index: shells come and go
// underneath it, and the appearance and settings entries are ids as well, so
// one string carries the whole selection.
const (
	railTabPrefix = "tab:"
	railTheme     = "action:appearance"
	railSettings  = "action:settings"

	// railWidth leaves room for a shell title and the appearance entry.
	railWidth = 200
)

// sidebar draws the navigation rail: the shells this window has open, the
// appearance switch and the way into the settings window. It is rebuilt from
// the window's own state on every frame, so a shell that appears — started
// here, or taken over from another machine later — shows up as soon as its tab
// does.
func (w *window) sidebar(c *ui.Context, labels []string, mode string) {
	// Seed the selection with the shell that is on screen. A click lands in the
	// same field while the rail is being drawn, which is why the field is window
	// state and not a local: railAction below reads it back in the same frame.
	w.rail = ""
	if id := w.activeID(); id != "" {
		w.rail = railTabPrefix + id
	}

	rail := ui.Sidebar(c, &w.rail, func() {
		if len(labels) > 0 {
			ui.SidebarSection(c, "会话", nil, func() {
				for i, id := range w.order {
					ui.SidebarItem(c, railTabPrefix+id, nil, labels[i])
				}
			})
		}
		ui.SidebarSection(c, "外观", nil, func() {
			ui.SidebarItem(c, railTheme, nil, themeActionLabel(mode, c.Theme().Dark))
		})
		ui.SidebarSection(c, "应用", nil, func() {
			ui.SidebarItem(c, railSettings, nil, "设置")
		})
	}).Width(railWidth)

	if rail.Changed() {
		w.railAction(c, mode)
	}
}

// railAction runs what the rail just asked for. It only runs on the frame that
// changed the selection, which is also the frame the click set w.rail.
func (w *window) railAction(c *ui.Context, mode string) {
	switch {
	case w.rail == railSettings:
		go w.openSettings()
	case w.rail == railTheme:
		w.setTheme(flipTheme(mode, c.Theme().Dark))
	case strings.HasPrefix(w.rail, railTabPrefix):
		if i := w.indexOfTab(strings.TrimPrefix(w.rail, railTabPrefix)); i >= 0 {
			w.selected = i
		}
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
