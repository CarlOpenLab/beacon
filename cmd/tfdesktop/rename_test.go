package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/beacon/locales"
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
	settleName(t, w, tb)
	before := tb.name()
	// The name before a rename is whatever the shell's terminal called itself;
	// on Windows that is cmd's "Administrator: C:\\Windows\\SYSTEM32\\cmd.exe".
	// Escape and an untouched field must leave both that name and the hub's
	// copy of it alone.
	beforeHub := hubTitleNow(f, tb.id)

	tt := drawNav(t, w)
	tt.Frame()
	w.beginRename(tb.machine, tb.id, before)
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
	if named := hubTitle(t, f, tb.id, beforeHub); named != beforeHub {
		t.Fatalf("the hub was told a name the person did not commit: %q", named)
	}
}

// A press anywhere else commits the name, the way a blur does in the browser
// client: the field does not outstay its welcome on the row. The press that
// opened the field is not one of those — it landed on the row a frame before
// the field existed — so the field survives its own opening.
func TestAPressElsewhereCommitsTheName(t *testing.T) {
	w, f := newSplitWindow(t)
	tb := openShell(t, w, f, testDir(t))

	tt := drawNav(t, w)
	tt.Frame()
	where, ok := tt.Find(tb.name())
	if !ok {
		t.Fatalf("the 会话 panel does not list %q; texts = %v", tb.name(), tt.Texts())
	}
	x, y := where.X+where.W/2, where.Y+where.H/2
	tt.ClickAt(x, y)
	tt.Frame()
	tt.ClickAt(x, y)
	tt.Frame()
	if !w.renamingIs(tb.machine, tb.id) {
		t.Fatal("a double-click on the row did not put its name into a field")
	}

	tt.Type("build box")
	// A press on the row's own edge — beside the name, not in the field — is a
	// press elsewhere as far as the field is concerned.
	tt.ClickAt(where.X+2, y)
	tt.Frame()
	drainFor(t, w, func() bool { return w.renaming == nil })

	if named := hubTitle(t, f, tb.id, "build box"); named != "build box" {
		t.Fatalf("the hub names the shell %q, want the name that was typed", named)
	}
	// And the tab reads it, which is the person who typed it seeing it land.
	if got := tb.name(); got != "build box" {
		t.Fatalf("the tab reads %q, want the name it was given", got)
	}
}

// A field nobody typed into names nothing: the press that lets go of it leaves
// the shell with the name it had, rather than pinning the name the row showed
// as a title of its own.
func TestAnUntouchedFieldNamesNothing(t *testing.T) {
	w, f := newSplitWindow(t)
	tb := openShell(t, w, f, testDir(t))
	settleName(t, w, tb)
	before := tb.name()
	beforeHub := hubTitleNow(f, tb.id)

	tt := drawNav(t, w)
	tt.Frame()
	where, ok := tt.Find(before)
	if !ok {
		t.Fatalf("the 会话 panel does not list %q; texts = %v", before, tt.Texts())
	}
	x, y := where.X+where.W/2, where.Y+where.H/2
	tt.ClickAt(x, y)
	tt.Frame()
	tt.ClickAt(x, y)
	tt.Frame()
	if !w.renamingIs(tb.machine, tb.id) {
		t.Fatal("a double-click on the row did not put its name into a field")
	}

	tt.ClickAt(where.X+2, y)
	tt.Frame()
	drainFor(t, w, func() bool { return w.renaming == nil })

	if got := tb.name(); got != before {
		t.Fatalf("the shell is named %q, want the name it had, %q", got, before)
	}
	// Nothing was sent: the hub's list still names the shell the way it
	// already did.
	if named := hubTitle(t, f, tb.id, beforeHub); named != beforeHub {
		t.Fatalf("the hub was told a name nobody typed: %q", named)
	}
}

// settleName waits until the shell stops naming itself. Windows' cmd sets its
// console title a moment after it starts, and a test that captures the name
// before that frame would see the name change under it for a reason that has
// nothing to do with a rename.
func settleName(t *testing.T, w *window, tb *tab) {
	t.Helper()
	last := tb.name()
	stable := time.Now()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		w.drain()
		time.Sleep(20 * time.Millisecond)
		if now := tb.name(); now != last {
			last = now
			stable = time.Now()
		}
		if time.Since(stable) >= 250*time.Millisecond {
			return
		}
	}
}

// hubTitleNow is the title the hub gives a session right now, for a test that
// compares against what it already was rather than against empty.
func hubTitleNow(f *fleet, id string) string {
	for _, s := range f.Hub().Sessions() {
		if s.SessionID == id {
			return s.Title
		}
	}
	return ""
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

	w.beginRename(tb.machine, tb.id, tb.name())
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
