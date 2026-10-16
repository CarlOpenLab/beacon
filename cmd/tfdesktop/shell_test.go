package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// The shell is the part of the window that is not a terminal: the strip, the
// header, the empty state and the bottom line. These tests check that each of
// them still says and offers what it is there for; the pictures in
// TestRenderShellPreviews are what says whether they look right.

// The strip is the only way to change the window itself, so both of its controls
// have to be on it and both have to work.
func TestTheStripSwitchesTheAppearanceAndOpensSettings(t *testing.T) {
	w, _ := newTestShellWindow(t)
	tt := ui.NewTester(func(c *ui.Context) {
		mode := w.store.Get().Theme
		applyTheme(c, mode)
		w.activityRail(c, mode)
	}, activityWidth, 420)

	if _, ok := tt.Find("切换到暗黑"); !ok {
		t.Fatalf("a light shell should offer the dark switch")
	}
	tt.Click("切换到暗黑")
	tt.Frame()
	if got := w.store.Get().Theme; got != themeDark {
		t.Fatalf("the strip wrote %q to the settings, want %q", got, themeDark)
	}
	if _, ok := tt.Find("切换到白天"); !ok {
		t.Fatalf("a dark shell should offer the light switch")
	}
	if _, ok := tt.Find("设置"); !ok {
		t.Fatalf("the strip has no settings entry")
	}
}

// The strip's controls belong at the bottom, under the column of shells: the
// mark heads the strip, and what changes the window itself ends it. A control
// left beside the shells is one the user has to hunt for, and the empty middle
// of a top-loaded strip reads as a control that was forgotten.
func TestTheStripKeepsItsControlsAtTheBottom(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	tt := drawNav(t, w)

	settings, ok := tt.Find("设置")
	if !ok {
		t.Fatalf("the strip has no settings entry")
	}
	appearance, ok := tt.Find("切换到暗黑")
	if !ok {
		t.Fatalf("the strip has no appearance entry")
	}
	shell, ok := tt.Find("one")
	if !ok {
		t.Fatalf("the column lists no shell to place the strip against")
	}
	if settings.Y <= shell.Y {
		t.Fatalf("设置 sits at y=%v, not below the shells at y=%v", settings.Y, shell.Y)
	}
	if appearance.Y <= shell.Y {
		t.Fatalf("the appearance switch sits at y=%v, not below the shells at y=%v", appearance.Y, shell.Y)
	}
	if settings.Y <= appearance.Y {
		t.Fatalf("设置 (y=%v) sits above the appearance switch (y=%v); the controls should read appearance, then settings", settings.Y, appearance.Y)
	}
}

// The header answers "which shell is this" and nothing else: the controls a
// shell needs moved to the column, on the panel they act on.
func TestHeaderNamesTheShellOnScreen(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	tt := ui.NewTester(func(c *ui.Context) {
		w.header(c, []string{"one"})
	}, 700, 52)

	if !tt.HasText("one") {
		t.Fatalf("the header does not name the shell; it shows %v", tt.Texts())
	}
	if !tt.HasText(w.serviceLine()) {
		t.Fatalf("the header does not name the service; it shows %v", tt.Texts())
	}
}

// The bottom line is where the service's address lives, and the build with it.
func TestStatusBarNamesTheServiceAndTheBuild(t *testing.T) {
	w, _ := newTestShellWindow(t)
	tt := ui.NewTester(func(c *ui.Context) {
		w.statusBar(c)
	}, 600, 28)

	if !tt.HasText(w.serviceLine()) {
		t.Fatalf("the bottom line does not name the service; it shows %v", tt.Texts())
	}
	if !tt.HasText(buildLine()) {
		t.Fatalf("the bottom line does not name the build; it shows %v", tt.Texts())
	}
}

// A window with no shells open says what to do about it rather than showing an
// empty rectangle.
func TestEmptyStateInvitesTheFirstShell(t *testing.T) {
	w, _ := newTestShellWindow(t)
	tt := ui.NewTester(func(c *ui.Context) {
		w.emptyState(c)
	}, 600, 320)

	if !tt.HasText("还没有终端") {
		t.Fatalf("the empty window says nothing; it shows %v", tt.Texts())
	}
}

// The mark is embedded, so the window always has a picture of itself — the strip
// and the empty state both draw it.
func TestTheMarkIsEmbedded(t *testing.T) {
	if mark == nil {
		t.Fatal("the embedded mark did not decode")
	}
	if w, h := mark.Size(); w != 512 || h != 512 {
		t.Fatalf("the mark is %dx%d, want 512x512", w, h)
	}
}

// The shell brings its own palette: the two set appearances, a strip that sits
// off the window ground, and a terminal that is part of the window rather than a
// program that wandered into it.
func TestTheShellBringsItsOwnPalette(t *testing.T) {
	dark := themeFor(themeDark)
	if dark == nil || !dark.Dark {
		t.Fatal("the set dark appearance is not dark")
	}
	if got, want := dark.Background, ui.Hex("#0b0d15"); got != want {
		t.Fatalf("the dark window ground is %v, want %v", got, want)
	}
	if light := themeFor(themeLight); light == nil || light.Dark {
		t.Fatal("the set light appearance is not light")
	}
	if got := shellColorsFor(dark).rail; got == dark.Background {
		t.Fatal("the strip sits on the window ground, so it is invisible")
	}
	if got := terminalTheme(true).Background; got != dark.Background {
		t.Fatalf("the dark pane ground is %v, want the window's %v", got, dark.Background)
	}
	if got := terminalTheme(false).Background; got != themeFor(themeLight).Surface {
		t.Fatalf("the light pane ground is %v, want the window's paper", got)
	}
}
