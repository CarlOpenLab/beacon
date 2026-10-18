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
func TestTheHeaderSwitchesTheAppearanceAndOpensSettings(t *testing.T) {
	w, _ := newTestShellWindow(t)
	tt := ui.NewTester(func(c *ui.Context) {
		mode := w.store.Get().Theme
		applyTheme(c, mode)
		w.header(c)
	}, 900, headerHeight)

	if _, ok := tt.Find("切换到暗黑"); !ok {
		t.Fatalf("a light shell should offer the dark switch")
	}
	tt.Click("切换到暗黑")
	tt.Frame()
	if got := w.store.Get().Theme; got != themeDark {
		t.Fatalf("the header wrote %q to the settings, want %q", got, themeDark)
	}
	if _, ok := tt.Find("切换到白天"); !ok {
		t.Fatalf("a dark shell should offer the light switch")
	}
	if _, ok := tt.Find("设置"); !ok {
		t.Fatalf("the header has no settings entry")
	}
	if _, ok := tt.Find("新建终端"); !ok {
		t.Fatalf("the header has no way to open a shell; texts = %v", tt.Texts())
	}
}

// The band names the product rather than the shell on screen, and it is the only
// place the window itself can be changed from. The shell on screen is named by
// the pill that carries it, the row that lists it and the line along the bottom;
// the band is for the one sentence the window has to say, and for the controls
// that are about the window.
func TestTheHeaderNamesTheProductAndCarriesTheWindowControls(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	tt := ui.NewTester(func(c *ui.Context) {
		applyTheme(c, w.store.Get().Theme)
		w.header(c)
	}, 900, headerHeight)

	if !tt.HasText("term-fleet") {
		t.Fatalf("the band does not name the product; it shows %v", tt.Texts())
	}
	for _, want := range []string{"会话检视", "设置", "新建终端"} {
		if _, ok := tt.Find(want); !ok {
			t.Fatalf("the band has no %q; texts = %v", want, tt.Texts())
		}
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
	dark := themeFor(themeDark)
	if dark == nil || !dark.Dark {
		t.Fatal("the set dark appearance is not dark")
	}
	if got, want := dark.Background, ui.Hex("#101014"); got != want {
		t.Fatalf("the dark window ground is %v, want %v", got, want)
	}
	if got, want := dark.Accent, ui.Hex("#a48bfa"); got != want {
		t.Fatalf("the dark accent is %v, want the design's violet %v", got, want)
	}
	lightTheme := themeFor(themeLight)
	if lightTheme == nil || lightTheme.Dark {
		t.Fatal("the set light appearance is not light")
	}
	if got, want := lightTheme.Background, ui.Hex("#f7f7f8"); got != want {
		t.Fatalf("the light window ground is %v, want %v", got, want)
	}
	if got, want := lightTheme.Accent, ui.Hex("#5f45e6"); got != want {
		t.Fatalf("the light accent is %v, want %v", got, want)
	}
	// The light appearance is not the dark one inverted: every surface has to be
	// its own answer, or the window ends up with a panel it cannot separate from
	// its ground.
	for _, pair := range [][2]ui.Color{
		{lightTheme.Background, lightTheme.Surface},
		{lightTheme.Background, lightTheme.SurfaceHover},
		{lightTheme.Surface, lightTheme.SurfaceHover},
	} {
		if pair[0] == pair[1] {
			t.Fatalf("the light appearance draws %v and %v the same", pair[0], pair[1])
		}
	}
	if got := shellColorsFor(dark).side; got == dark.Background {
		t.Fatal("the sidebar sits on the window ground, so it is invisible")
	}
	if got := shellColorsFor(lightTheme).side; got == lightTheme.Background {
		t.Fatal("the light sidebar sits on the window ground, so it is invisible")
	}
	// The terminal keeps its own dark ground in both appearances: the ANSI 16
	// are designed for a dark ground, and inverting them makes success and
	// failure read wrong. The window's chrome flips; the sheet does not.
	if got, want := terminalTheme(true).Background, ui.Hex("#0d0d12"); got != want {
		t.Fatalf("the dark pane ground is %v, want %v", got, want)
	}
	if got := terminalTheme(false).Background; got != terminalTheme(true).Background {
		t.Fatal("the light appearance gives the terminal a different ground")
	}
	// The accent is the one saturated colour in the chrome, and the terminal
	// shares it: a shell that wandered in would have brought its own.
	if got, want := terminalTheme(true).Cursor, ui.Hex("#b6a6ff"); got != want {
		t.Fatalf("the terminal cursor is %v, want the chrome's own accent %v", got, want)
	}
}
