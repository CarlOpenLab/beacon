package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/locales"
)

// Naming a shell is the whole point of the gesture: a double-click on the
// shell's row puts its name into a field, Enter sends the name to the hub, and
// the hub's answer is what every client — this window included — reads
// afterwards. The shell is named by the title the hub keeps, so the row, the
// band and the inspector all move together.
func TestADoubleClickOnTheRowNamesTheShell(t *testing.T) {
	w, f := newSplitWindow(t)
	tb := openShell(t, w, f, testDir(t))
	if got := tb.name(); got == "" {
		t.Fatal("the shell has no name to edit")
	}

	tt := drawNav(t, w)
	tt.Frame()
	where, ok := tt.Find(tb.name())
	if !ok {
		t.Fatalf("the 会话 panel does not list %q; texts = %v", tb.name(), tt.Texts())
	}
	x, y := where.X+where.W/2, where.Y+where.H/2
	// A double-click is two presses in the same place, the way a pointer makes
	// one; the tester takes them a frame apart, which is what makes the second
	// one the second half of a double click.
	tt.ClickAt(x, y)
	tt.Frame()
	tt.ClickAt(x, y)
	tt.Frame()

	if !w.renamingIs(tb.machine, tb.id) {
		t.Fatal("a double-click on the row did not put its name into a field")
	}
	if w.renameDraft != tb.name() {
		t.Fatalf("the field holds %q, want the name the shell has now", w.renameDraft)
	}

	tt.Type("build box")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	drainFor(t, w, func() bool { return w.renaming == nil })

	// The hub keeps the name, trimmed, and says so to everybody. The frame is
	// answered on the hub's own goroutine, so the answer is waited for rather
	// than read once.
	if named := hubTitle(t, f, tb.id, "build box"); named != "build box" {
		t.Fatalf("the hub names the shell %q, want the name that was typed", named)
	}
	// And the tab reads it, which is the person who typed it seeing it land.
	if got := tb.name(); got != "build box" {
		t.Fatalf("the tab reads %q, want the name it was given", got)
	}
}

// Escape leaves the shell named as it was: a field that commits on every way
// out of it is a field nobody dares open.
func TestEscapeLeavesTheShellNamedAsItWas(t *testing.T) {
	w, f := newSplitWindow(t)
	tb := openShell(t, w, f, testDir(t))
	before := tb.name()

	tt := drawNav(t, w)
	tt.Frame()
	w.beginRename(tb.machine, tb.id)
	tt.Frame()
	tt.Type("a name that will not stick")
	tt.Key(0, ui.KeyEscape)
	tt.Frame()

	if w.renaming != nil {
		t.Fatal("Escape did not close the field")
	}
	if got := tb.name(); got != before {
		t.Fatalf("the shell is named %q, want the name it had, %q", got, before)
	}
	if named := hubTitle(t, f, tb.id, ""); named != "" {
		t.Fatalf("the hub was told a name the person did not commit: %q", named)
	}
}

// hubTitle is the title the hub gives a session, waited for until it says want:
// a rename is answered on the hub's own goroutine, so the frame that carries it
// lands a moment after the one that sent it.
func hubTitle(t *testing.T, f *fleet, id, want string) string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		named := ""
		for _, s := range f.Hub().Sessions() {
			if s.SessionID == id {
				named = s.Title
			}
		}
		if named == want {
			return named
		}
		if time.Now().After(deadline) {
			return named
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// An empty name gives the shell its program's name back: the title is cleared
// rather than kept as a blank, and the hub's own list says so.
func TestAnEmptyNameGivesTheShellItsProgramBack(t *testing.T) {
	w, f := newSplitWindow(t)
	tb := openShell(t, w, f, testDir(t))

	w.beginRename(tb.machine, tb.id)
	w.renameDraft = "  "
	w.commitRename()

	if tb.name() == "  " || tb.name() == "" {
		t.Fatalf("the shell is named %q, want its program's name", tb.name())
	}
	if named := hubTitle(t, f, tb.id, ""); named != "" {
		t.Fatalf("the hub still names the shell %q after the title was cleared", named)
	}
}

// The row's menu carries the same act as the double-click, for anybody who
// does not double-click.
func TestTheRowsMenuOffersTheRename(t *testing.T) {
	w, f := newSplitWindow(t)
	tb := openShell(t, w, f, testDir(t))
	tt := drawNav(t, w)
	tt.Frame()
	where, ok := tt.Find(tb.name())
	if !ok {
		t.Fatalf("the 会话 panel does not list %q; texts = %v", tb.name(), tt.Texts())
	}
	tt.RightClickAt(where.X+where.W/2, where.Y+where.H/2)
	tt.Frame()

	found := false
	for _, item := range tt.Menu() {
		if item == locales.TabRename() {
			found = true
		}
	}
	if !found {
		t.Fatalf("the row's menu does not offer the rename; menu = %v", tt.Menu())
	}
}
