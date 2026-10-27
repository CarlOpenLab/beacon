package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/beacon/internal/uitokens"
	"github.com/CarlOpenLab/beacon/locales"
)

// The shell is the part of the window that is not a terminal: the band across
// the top, the strip of pills, the empty state and the bottom line. These tests
// check that each of them still says and offers what it is there for; the
// pictures in TestRenderShellPreviews are what says whether they look right.

// A card's header names the shell on screen, and where the window's own controls
// went: the shell's name is on the card, and 新建终端 and 设置 are in the
// sidebar's foot and top bar. The old band across the top, with a sentence that
// stitched the machine and the shell together, is gone.
func TestTheCardHeaderNamesTheActiveTerminal(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	tb := w.tabs[1]
	tb.alive = true
	tb.term = fedTerminal(t, previewScreen())
	tt := ui.NewTester(w.view, 900, 600)
	tt.Frame()
	if !tt.HasText("one") {
		t.Fatalf("missing the shell's name on its card: %v", tt.Texts())
	}
	if tt.HasText(locales.HeaderMachineShell(locales.MachineLocal(), "one")) {
		t.Fatal("the removed band across the top is still drawn")
	}
	if _, ok := tt.Find(locales.ActionNewTerminalTip()); ok {
		t.Fatal("the card header still carries the window's new-terminal button")
	}
	if _, ok := tt.Find(locales.ActionSettings()); !ok {
		t.Fatal("missing settings action")
	}
	if tt.HasText(locales.InspectTitle()) {
		t.Fatal("removed inspector is still visible")
	}
}

// A window with nothing open says what to do about it rather than showing an
// empty rectangle, and it offers the action it is talking about.
func TestEmptyStateInvitesTheFirstShell(t *testing.T) {
	w, _ := newTestShellWindow(t)
	tt := ui.NewTester(func(c *ui.Context) {
		applyTheme(c, w.store.Get().Theme)
		w.emptyState(c, paletteFor(c))
	}, 600, 320)

	if !tt.HasText(locales.EmptyTitle()) {
		t.Fatalf("the empty window says nothing; it shows %v", tt.Texts())
	}
	if _, ok := tt.Find(locales.ActionNewTerminal()); !ok {
		t.Fatalf("the empty window does not offer a way out; texts = %v", tt.Texts())
	}
}

// The palette and the settings live in the top bar, beside the button that
// folds the column: the rail at the bottom is machines only.
func TestTheSidebarTopBarHoldsThePaletteAndSettings(t *testing.T) {
	w, _ := newTestShellWindow(t)
	tt := ui.NewTester(func(c *ui.Context) {
		applyTheme(c, w.store.Get().Theme)
		w.sidebar(c, paletteFor(c))
	}, 300, 400)

	for _, want := range []string{locales.ActionPalette(), locales.ActionSettings()} {
		if _, ok := tt.Find(want); !ok {
			t.Fatalf("the rail does not hold %q; it shows %v", want, tt.Texts())
		}
	}
}

// The shell brings its own palette: the app's light and dark sets, a card that is
// not the ground it sits on, and a terminal whose colors match the card the
// window draws around it.
func TestTheShellBringsItsOwnPalette(t *testing.T) {
	dark := darkTheme()
	if !dark.Dark || dark.Background != uitokens.Dark.Sidebar {
		t.Fatal("wrong desktop ground")
	}
	if dark.Accent != uitokens.Dark.Accent {
		t.Fatal("chrome accent is not the palette's")
	}
	term := terminalTheme(true)
	if term.Background != uitokens.Dark.Card || term.Cursor != ui.Hex("#e6e6ea") {
		t.Fatal("terminal does not match the card")
	}
	if term.Palette[1] == term.Palette[2] || term.Palette[2] == term.Palette[4] {
		t.Fatal("ANSI colors lost their distinction")
	}

	// The light appearance is its own set, not the dark one inverted: its card is
	// white, its ink is near black, and its terminal reads on the white.
	light := lightTheme()
	if light.Dark || light.Background == dark.Background {
		t.Fatal("the light appearance is the dark one")
	}
	lightTerm := terminalTheme(false)
	if lightTerm.Background != uitokens.Light.Card || lightTerm.Foreground != ui.Hex("#1d1d1f") {
		t.Fatal("the light terminal does not read on its card")
	}
}
