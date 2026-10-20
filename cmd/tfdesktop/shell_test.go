package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/uitokens"
	"github.com/CarlOpenLab/term-fleet/locales"
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
	if _, ok := tt.Find(locales.ActionNewTerminalTip()); !ok {
		t.Fatal("missing new terminal action")
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

// The service and the fleet live in the sidebar's foot now: a dot for the local
// service and how many machines answer, with the address in its tooltip. It is
// the one line the old status bar was for, in the corner kero keeps it.
func TestTheSidebarFootNamesTheService(t *testing.T) {
	w, _ := newTestShellWindow(t)
	tt := ui.NewTester(func(c *ui.Context) {
		applyTheme(c, w.store.Get().Theme)
		w.sidebarFooter(c, paletteFor(c))
	}, 300, uitokens.FooterH)

	if !tt.HasText(w.serviceLine()) {
		t.Fatalf("the foot does not name the service; it shows %v", tt.Texts())
	}
	if _, ok := tt.Find(locales.ServiceOff()); !ok {
		t.Fatalf("the service line is not a chip a pointer can rest on; texts = %v", tt.Texts())
	}
}

// The mark is embedded, so the window always has a picture of itself — the
// brand in the header and the empty state both draw it.
func TestTheMarkIsEmbedded(t *testing.T) {
	if mark == nil {
		t.Fatal("the embedded mark did not decode")
	}
	if w, h := mark.Size(); w != 512 || h != 512 {
		t.Fatalf("the mark is %dx%d, want 512x512", w, h)
	}
}

// The shell brings its own palette: kero's light and dark sets, a card that is
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
