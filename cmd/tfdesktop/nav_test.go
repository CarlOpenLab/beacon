package main

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/settings"
)

// newTestShellWindow is a shell window with shells open and no hub behind it:
// the rail reads the tab order and the labels, and nothing else.
func newTestShellWindow(t *testing.T, titles ...string) (*window, string) {
	t.Helper()
	dir := t.TempDir()
	w := &window{store: settings.Open(dir), tabs: make(map[string]*tab)}
	for i, title := range titles {
		id := fmt.Sprintf("s%d", i+1)
		w.tabs[id] = &tab{id: id, title: title, ordinal: i + 1}
		w.order = append(w.order, id)
	}
	return w, dir
}

// drawNav draws the window's navigation the way its view draws it: the strip of
// controls, and the column of shells beside it. The appearance comes from the
// store and the labels from the shells that are open.
func drawNav(t *testing.T, w *window) *ui.Tester {
	t.Helper()
	return ui.NewTester(func(c *ui.Context) {
		mode := w.store.Get().Theme
		applyTheme(c, mode)
		labels := make([]string, 0, len(w.order))
		for _, id := range w.order {
			if tb := w.tabs[id]; tb != nil {
				labels = append(labels, tb.label())
			}
		}
		w.activityRail(c, mode)
		w.sidebar(c, labels, mode)
	}, activityWidth+sidebarWidth, 420)
}

func TestSidebarListsTheOpenShellsAndPicksOne(t *testing.T) {
	w, _ := newTestShellWindow(t, "one", "two")
	tt := drawNav(t, w)

	for _, want := range []string{"one", "two"} {
		if !tt.HasText(want) {
			t.Fatalf("the column does not show %q; it shows %v", want, tt.Texts())
		}
	}
	if _, ok := tt.Find("设置"); !ok {
		t.Fatalf("the strip has no settings entry")
	}

	if err := tt.Click("two"); err != nil {
		t.Fatalf("click the second shell: %v", err)
	}
	if w.selected != 1 {
		t.Fatalf("selected = %d, want the second shell", w.selected)
	}
	if err := tt.Click("one"); err != nil {
		t.Fatalf("click the first shell: %v", err)
	}
	if w.selected != 0 {
		t.Fatalf("selected = %d, want the first shell", w.selected)
	}
}

func TestSidebarAppearanceSwitchesTheThemeAndSavesIt(t *testing.T) {
	w, dir := newTestShellWindow(t)
	tt := drawNav(t, w)
	tt.SetDark(false) // the system is showing its light appearance

	light := tt.Image().RGBAAt(2, 2)
	if _, ok := tt.Find("切换到暗黑"); !ok {
		t.Fatalf("a light shell should offer the dark switch; it shows %v", tt.Texts())
	}
	if err := tt.Click("切换到暗黑"); err != nil {
		t.Fatalf("click the appearance entry: %v", err)
	}
	if got := w.store.Get().Theme; got != themeDark {
		t.Fatalf("appearance = %q, want %q", got, themeDark)
	}
	if got := settings.Load(dir).Theme; got != themeDark {
		t.Fatalf("appearance on disk = %q, want %q", got, themeDark)
	}

	// The next frame is drawn in the appearance that was just chosen, and it
	// offers the way back.
	tt.Frame()
	if dark := tt.Image().RGBAAt(2, 2); dark == light {
		t.Fatalf("the frame kept the old appearance: %v", dark)
	}
	if _, ok := tt.Find("切换到白天"); !ok {
		t.Fatalf("a dark shell should offer the light switch; it shows %v", tt.Texts())
	}
	if err := tt.Click("切换到白天"); err != nil {
		t.Fatalf("click the appearance entry: %v", err)
	}
	if got := w.store.Get().Theme; got != themeLight {
		t.Fatalf("appearance = %q, want %q", got, themeLight)
	}
}

func TestFlipTheme(t *testing.T) {
	cases := []struct {
		mode       string
		systemDark bool
		want       string
	}{
		{themeSystem, false, themeDark}, // the first click always changes something
		{themeSystem, true, themeLight},
		{themeLight, false, themeDark},
		{themeDark, false, themeLight},
		{"Nonsense", false, themeDark}, // an unreadable file still leaves a way out
	}
	for _, tc := range cases {
		if got := flipTheme(tc.mode, tc.systemDark); got != tc.want {
			t.Errorf("flipTheme(%q, %v) = %q, want %q", tc.mode, tc.systemDark, got, tc.want)
		}
	}
}

func TestThemeForFollowsTheSystemUnlessAsked(t *testing.T) {
	if themeFor(themeSystem) != nil {
		t.Error("the system default should leave the ui's own theme alone")
	}
	if !themeFor(themeDark).Dark {
		t.Error("the dark appearance is not dark")
	}
	if themeFor(themeLight).Dark {
		t.Error("the light appearance is dark")
	}
}

func TestAppearancePaneOffersAllThreeModes(t *testing.T) {
	f := newTestFleet(t)
	store := settings.Open(t.TempDir())
	// The desk is the window the choice is saved for: the pane goes through it,
	// the way it does in the shell.
	sw := newSettingsWindow(&window{store: store}, store, f)
	tt := ui.NewTester(func(c *ui.Context) { sw.view(c) }, 560, 460)
	if err := tt.Click("外观"); err != nil {
		t.Fatalf("open the appearance pane: %v", err)
	}
	for _, want := range []string{"跟随系统", "白天", "暗黑"} {
		if !tt.HasText(want) {
			t.Fatalf("the appearance pane does not offer %q; it shows %v", want, tt.Texts())
		}
	}
	if !tt.HasText("当前是「跟随系统」") {
		t.Fatalf("the pane does not say which appearance is on: %v", tt.Texts())
	}
	if err := tt.Click("暗黑"); err != nil {
		t.Fatalf("choose the dark appearance: %v", err)
	}
	if got := store.Get().Theme; got != themeDark {
		t.Fatalf("appearance = %q, want %q", got, themeDark)
	}
}
