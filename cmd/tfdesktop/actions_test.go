package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/settings"
	"github.com/CarlOpenLab/term-fleet/locales"
)

// Accelerators are written one way in a settings file and drawn another on
// screen; the round trip is what keeps a rebinding from changing what it means.
func TestAcceleratorsRoundTrip(t *testing.T) {
	for _, accel := range []string{
		"cmd+t", "cmd+shift+d", "cmd+alt+left", "cmd+bracketright",
		"ctrl+shift+tab", "cmd+comma", "f5", "cmd+shift+enter", "cmd+0",
	} {
		mods, key, ok := parseAccel(accel)
		if !ok {
			t.Fatalf("%q does not parse", accel)
		}
		if got := formatAccel(mods, key); got != accel {
			t.Errorf("%q round-trips to %q", accel, got)
		}
	}
	for _, bad := range []string{"", "cmd+", "cmd+nope", "hyper+t"} {
		if _, _, ok := parseAccel(bad); ok {
			t.Errorf("%q parsed and should not have", bad)
		}
	}
}

// Every action's default is a real accelerator, and no two of them are the same
// key: a list that binds ⌘D twice would run whichever came first.
func TestTheActionsHaveDistinctKeys(t *testing.T) {
	seen := map[string]string{}
	for _, a := range appActions {
		if _, _, ok := parseAccel(a.def); !ok {
			t.Errorf("%s has an accelerator that does not parse: %q", a.id, a.def)
			continue
		}
		if other, ok := seen[a.def]; ok {
			t.Errorf("%s and %s both answer to %s", other, a.id, a.def)
		}
		seen[a.def] = a.id
		if a.title() == "" {
			t.Errorf("%s has no title", a.id)
		}
	}
}

// The keyboard is read from the list, so rebinding an action in Settings is the
// key the window answers to — and the old one stops working.
func TestARebindingMovesTheShortcut(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	if _, err := w.store.Update(func(s *settings.Settings) {
		s.Keybinds = map[string]string{"sidebar.toggle": "cmd+k"}
	}); err != nil {
		t.Fatal(err)
	}
	press := func(mods ui.Modifiers, key ui.Key) {
		t.Helper()
		tt := ui.NewTester(w.view, 900, 600)
		tt.Frame()
		tt.Key(mods, key)
		tt.Frame()
	}

	press(ui.Cmd, ui.KeyK)
	if !w.sidebarHidden {
		t.Fatal("⌘K did not toggle the sidebar after the rebinding")
	}
	w.sidebarHidden = false
	press(ui.Cmd, ui.KeyB)
	if w.sidebarHidden {
		t.Fatal("⌘B still toggles the sidebar after it was rebound away")
	}
}

// An action can be unbound: an empty accelerator means the key is nobody's.
func TestAnUnboundActionDoesNothing(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	if _, err := w.store.Update(func(s *settings.Settings) {
		s.Keybinds = map[string]string{"sidebar.toggle": ""}
	}); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(w.view, 900, 600)
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyB)
	tt.Frame()
	if w.sidebarHidden {
		t.Fatal("an unbound action still fired")
	}
}

// The keyboard page lists the actions with the keys in force, and recording a
// new one writes it: that is the whole point of the page.
func TestTheKeyboardPageRecordsAShortcut(t *testing.T) {
	sw, store := newTestSettingsWindow(t)
	sw.pane = paneKeyboard
	tt := ui.NewTester(sw.view, 760, 600)
	tt.Frame()
	if !tt.HasText(glyphAccel("cmd+t")) {
		t.Fatalf("the page does not show 新建终端's keys: %v", tt.Texts())
	}

	newTab, _ := actionByID("session.new")
	if !sw.recordKey(newTab, ui.Cmd|ui.Shift, ui.KeyT) {
		t.Fatal("the recording did not take the keys")
	}
	if got := store.Get().Keybinds["session.new"]; got != "cmd+shift+t" {
		t.Fatalf("recorded %q, want cmd+shift+t", got)
	}
	if sw.recording != "" {
		t.Fatal("the page is still recording after the keys were taken")
	}

	// The same page now shows the new keys.
	tt.Frame()
	if !tt.HasText(glyphAccel("cmd+shift+t")) {
		t.Fatalf("the page does not show the new keys: %v", tt.Texts())
	}
}

// Two actions on one key is a mistake the page names rather than hides.
func TestRecordingAKeyInUseSaysSo(t *testing.T) {
	sw, _ := newTestSettingsWindow(t)
	sw.pane = paneKeyboard
	dupe, _ := actionByID("session.new")
	sw.recordKey(dupe, ui.Cmd, ui.KeyD) // ⌘D already splits the pane
	if !strings.Contains(sw.keyNotice, locales.ActionSplitRight()) {
		t.Fatalf("the conflict notice is %q", sw.keyNotice)
	}
}

// Clearing a shortcut leaves it unbound rather than putting the default back:
// the default is what 「恢复默认」 is for.
func TestClearingAShortcutUnbindsIt(t *testing.T) {
	sw, store := newTestSettingsWindow(t)
	newTab, _ := actionByID("session.new")
	sw.setKeys(newTab, "")
	if got, ok := store.Get().Keybinds["session.new"]; !ok || got != "" {
		t.Fatalf("clearing wrote %q (present=%v), want an empty override", got, ok)
	}
	if sw.keysFor(newTab) != "" {
		t.Fatal("the page still shows a shortcut for the cleared action")
	}
}

// The page's own path: clicking a shortcut chip takes the keyboard, and the
// next keys typed become the shortcut. This is what a person does, and it is
// what the unit test above cannot prove.
func TestRecordingThroughThePage(t *testing.T) {
	sw, store := newTestSettingsWindow(t)
	sw.pane = paneKeyboard
	tt := ui.NewTester(sw.view, 760, 600)
	tt.Frame()

	rect, ok := tt.Find(glyphAccel("cmd+t"))
	if !ok {
		t.Fatalf("the chip showing ⌘T is not on the page: %v", tt.Texts())
	}
	tt.ClickAt(rect.X+rect.W/2, rect.Y+rect.H/2)
	tt.Frame()
	if sw.recording != "session.new" {
		t.Fatalf("clicking the chip started recording %q", sw.recording)
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyT)
	tt.Frame()
	if got := store.Get().Keybinds["session.new"]; got != "cmd+shift+t" {
		t.Fatalf("the keys typed did not become the shortcut: %q", got)
	}
	if sw.recording != "" {
		t.Fatal("the page is still recording")
	}
}
