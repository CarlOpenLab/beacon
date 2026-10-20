package main

// Naming a shell.
//
// A shell is named after its program and its number until somebody names it:
// the hub keeps a title per session, every client's tab and row reads it, and
// an empty title gives the shell its program's name back. The gesture is a
// double-click on the shell's row in the 会话 panel, the way every list on a Mac
// renames what it lists, with the same act in the row's menu for anybody who
// does not double-click.
//
// The lease is not consulted: a title is a label on a shell everybody watching
// sees, not the keyboard. A phone reading a shell may name what it is reading,
// and the machine's own window may name it back — which is the whole of the
// authority a label needs.

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/locales"
)

// renameTarget is the shell being named: where it runs, which session it is,
// and the tab watching it when this window has one — the tab is what moves its
// label at once, before the hub's answer comes back.
type renameTarget struct {
	machine string
	session string
	tab     *tab
}

// beginRename puts a shell's name into the field on its row: the draft starts
// as the name it has now, so editing is editing rather than typing it again.
func (w *window) beginRename(machine, session string) {
	if session == "" {
		return
	}
	w.renaming = &renameTarget{machine: machine, session: session, tab: w.tabOf(machine, session)}
	w.renameDraft = w.renaming.name()
	w.invalidate()
}

// name is what the shell is called right now: its title when it has one, and
// the program-and-number name the window gives it when it does not.
func (r *renameTarget) name() string {
	if r == nil {
		return ""
	}
	if r.tab != nil {
		return r.tab.name()
	}
	return r.session
}

// renamingIs reports whether the shell being named is this one, which is what
// the row asks to know whether to draw a field instead of its name.
func (w *window) renamingIs(machine, session string) bool {
	return w.renaming != nil && w.renaming.machine == machine && w.renaming.session == session
}

// commitRename sends the draft to the hub. The row's own name moves at once —
// the person who typed it should see it — and the session list that follows
// confirms it for every client watching the shell, this window included.
func (w *window) commitRename() {
	at := w.renaming
	w.renaming = nil
	if at == nil {
		return
	}
	title := strings.TrimSpace(w.renameDraft)
	w.renameDraft = ""
	if at.tab != nil {
		at.tab.setTitle(title)
	}
	w.linkFor(at.machine).rename(at.session, title)
	w.invalidate()
}

// cancelRename leaves the shell named as it was.
func (w *window) cancelRename() {
	w.renaming = nil
	w.renameDraft = ""
	w.invalidate()
}

// renameField is the row's name while it is being edited: a field in the name's
// place, Enter committing and Escape letting go. It takes the keyboard focus
// the moment it appears, with the whole name selected, so typing replaces it
// the way renaming does everywhere else — and editing it is still there for
// anybody who wants to change one letter.
func (w *window) renameField(c *ui.Context) {
	t := c.Theme()
	box := ui.TextInput(c, &w.renameDraft).
		Fill().
		AutoFocus().
		SetTextSelection(0, len([]rune(w.renameDraft))).
		Radius(radiusSmall).
		Background(t.Background).
		BorderWidth(1).
		BorderColor(t.Accent).
		Placeholder(locales.RenamePlaceholder())
	if box.Submitted() {
		w.commitRename()
		return
	}
	if box.Shortcut(0, ui.KeyEscape) {
		w.cancelRename()
	}
}
