package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// The shell is the part of the window that is not a terminal: the band across
// the top, the strip of pills, the empty state and the bottom line. These tests
// check that each of them still says and offers what it is there for; the
// pictures in TestRenderShellPreviews are what says whether they look right.

// The header is the only way to change the window itself, so both of its
// controls have to be on it and both have to work.
func TestTheHeaderNamesTheActiveTerminalAndOffersNewTerminal(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	w.tabs[1].alive = true
	tt := ui.NewTester(w.view, 900, 600)
	if !tt.HasText("本机 / one") {
		t.Fatalf("missing terminal title: %v", tt.Texts())
	}
	if _, ok := tt.Find("新建终端"); !ok {
		t.Fatal("missing new terminal action")
	}
	if _, ok := tt.Find("设置"); !ok {
		t.Fatal("missing settings action")
	}
	if tt.HasText("会话检视") {
		t.Fatal("removed inspector is still visible")
	}
}

// A window with nothing open says what to do about it rather than showing an
// empty rectangle, and it offers the action it is talking about.
func TestEmptyStateInvitesTheFirstShell(t *testing.T) {
	w, _ := newTestShellWindow(t)
	tt := ui.NewTester(func(c *ui.Context) {
		applyTheme(c, w.store.Get().Theme)
		w.emptyState(c)
	}, 600, 320)

	if !tt.HasText("还没有打开的终端") {
		t.Fatalf("the empty window says nothing; it shows %v", tt.Texts())
	}
	if _, ok := tt.Find("新建终端"); !ok {
		t.Fatalf("the empty window does not offer a way out; texts = %v", tt.Texts())
	}
}

// The bottom line is where the service's address lives and the machine the shell
// on screen runs on, with the shell itself at the end of it. The build is in the
// status chip's tooltip: the line already names the platform it is running on,
// and naming it a second time was the one thing on the line that said nothing.
func TestStatusBarNamesTheServiceAndTheMachine(t *testing.T) {
	w, _ := newTestShellWindow(t)
	tt := ui.NewTester(func(c *ui.Context) {
		applyTheme(c, w.store.Get().Theme)
		w.statusBar(c)
	}, 600, statusHeight)

	if !tt.HasText(w.serviceLine()) {
		t.Fatalf("the bottom line does not name the service; it shows %v", tt.Texts())
	}
	if _, ok := tt.Find("本机服务已关闭"); !ok {
		t.Fatalf("the status chip is not a chip a pointer can rest on; texts = %v", tt.Texts())
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

// The shell brings its own palette: the two set appearances, a sidebar that sits
// off the window ground, and a terminal that is part of the window rather than a
// program that wandered into it.
func TestTheShellBringsItsOwnPalette(t *testing.T) {
	dark := darkTheme()
	if !dark.Dark || dark.Background != ui.Hex("#111111") {
		t.Fatal("wrong desktop ground")
	}
	if dark.Accent != ui.Hex("#d0d0d0") {
		t.Fatal("chrome accent is not neutral")
	}
	term := terminalTheme(true)
	if term.Background != terminalGround || term.Cursor != ui.Hex("#dedede") {
		t.Fatal("terminal does not match desktop")
	}
	if term.Palette[1] == term.Palette[2] || term.Palette[2] == term.Palette[4] {
		t.Fatal("ANSI colors lost their distinction")
	}
}
