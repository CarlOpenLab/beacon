package main

// Naming a shell.
//
// A shell is named after its program and its number until somebody names it:
// the hub keeps a title per session, every client's tab and row reads it, and
// an empty title gives the shell its program's name back. The gesture is a
// double-click on the shell's row in the 会话 panel, the way every list on a Mac
// renames what it lists, with the same act in the row's menu for anybody who
// does not double-click. The field commits on Enter and on a press anywhere
// else, and lets go on Escape.
//
// The lease is not consulted: a title is a label on a shell everybody watching
// sees, not the keyboard. A phone reading a shell may name what it is reading,
// and the machine's own window may name it back — which is the whole of the
// authority a label needs.

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/beacon/locales"
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
// as the name the row shows, so editing is editing rather than typing it again.
// The name the draft starts as is kept as the seed, which is what tells a field
// nobody touched from a renamed one.
func (w *window) beginRename(machine, session, label string) {
	if session == "" {
		return
	}
	w.renaming = &renameTarget{machine: machine, session: session, tab: w.tabOf(machine, session)}
	w.renameDraft = label
	w.renameSeed = label
	w.renameArmed = false
	w.invalidate()
}

// renamingIs reports whether the shell being named is this one, which is what
// the row asks to know whether to draw a field instead of its name.
func (w *window) renamingIs(machine, session string) bool {
	return w.renaming != nil && w.renaming.machine == machine && w.renaming.session == session
}

// commitRename sends the draft to the hub. The row's own name moves at once —
// the person who typed it should see it — and the session list that follows
// confirms it for every client watching the shell, this window included. A
// draft nobody touched names nothing: the shell keeps the name it had, derived
// or titled, rather than having the name the row showed pinned to it as one.
func (w *window) commitRename() {
	at := w.renaming
	w.renaming = nil
	w.renameArmed = false
	if at == nil {
		return
	}
	title := strings.TrimSpace(w.renameDraft)
	seed := w.renameSeed
	w.renameDraft = ""
	w.renameSeed = ""
	if title == seed {
		w.invalidate()
		return
	}
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
	w.renameSeed = ""
	w.renameArmed = false
	w.invalidate()
}

// renameField is the row's name while it is being edited: a field in the name's
// place, Enter committing, a press anywhere else committing and Escape letting
// go. It takes the keyboard focus the moment it appears, with the whole name
// selected, so typing replaces it the way renaming does everywhere else — and
// editing it is still there for anybody who wants to change one letter.
//
// The field wears the row's own typography on the panel's raised field ground,
// at a height that sits inside the row rather than filling it: the name becoming
// editable is the row changing state, not a box landing on it. The border is the
// panel's hairline until the caret is in the field, which is the moment it
// appears, and the accent while it is.
func (w *window) renameField(c *ui.Context) {
	col := shellColorsFor(c.Theme())
	box := ui.TextInput(c, &w.renameDraft).
		Fill().
		Height(22).
		AutoFocus().
		SetTextSelection(0, len([]rune(w.renameDraft))).
		PaddingX(8).PaddingY(2).
		Radius(radiusSmall).
		FontSize(12).
		Background(col.field).
		BorderWidth(1).
		BorderColor(col.line).
		Placeholder(locales.RenamePlaceholder())
	if box.Submitted() {
		w.commitRename()
		return
	}
	if box.Shortcut(0, ui.KeyEscape) {
		w.cancelRename()
		return
	}
	// A press anywhere but the field is the field losing the floor, the way a
	// blur is in the browser client: the name is kept. The press that opened the
	// field is not one of those — it landed on the row a frame before the field
	// existed — so the field only starts listening once it has been on screen
	// for a frame.
	if w.renameArmed && box.PressedOutside() {
		w.commitRename()
		return
	}
	w.renameArmed = true
}
