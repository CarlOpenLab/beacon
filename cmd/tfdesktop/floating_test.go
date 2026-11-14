package main

// Settings is a window of its own now — the way Gloss's settings window is:
// the page stays in sight behind it, because the page is not under it at all,
// and the header carries the window's own controls. These are the two things
// the report asked for.

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/beacon/internal/settings"
	"github.com/CarlOpenLab/beacon/locales"
)

// TestSettingsLeavesThePageInSight reads the pixels: with settings open, the
// main window looks exactly as it does closed. When settings was a card of the
// page, this guarded against a wash over the shells; now that it is a window
// of its own, the page has nothing of settings in it at all — the test keeps
// standing so a future frame cannot quietly draw settings back into the page.
func TestSettingsLeavesThePageInSight(t *testing.T) {
	w, _ := newTestShellWindow(t)
	w.fleet = newTestFleet(t)
	if _, err := w.store.Update(func(s *settings.Settings) { s.BackgroundOpacity = 1 }); err != nil {
		t.Fatal(err)
	}

	const width, height = 1000, 700
	// A point in the window's bottom-left corner, clear of everything.
	const probeX, probeY = 30, 650

	closed := ui.NewTester(w.view, width, height)
	closed.Frame()
	closed.Frame()
	r1, g1, b1, _ := closed.Image().At(probeX, probeY).RGBA()

	if err := closed.Click(locales.ActionSettings()); err != nil {
		t.Fatalf("Click(%s): %v", locales.ActionSettings(), err)
	}
	if !w.settingsOpen {
		t.Fatal("settings did not open")
	}
	closed.Frame()
	closed.Frame()
	r2, g2, b2, _ := closed.Image().At(probeX, probeY).RGBA()

	if r1 != r2 || g1 != g2 || b1 != b2 {
		t.Errorf("settings changed the page: closed (%d,%d,%d), open (%d,%d,%d)",
			r1>>8, g1>>8, b1>>8, r2>>8, g2>>8, b2>>8)
	}
}

// TestTheSettingsWindowCarriesItsControls drives the window controls in the
// header: they act on the settings window's own frame — minimize, maximize,
// close — and closing from there is closing settings, which is what the
// system's close (the same frame, from outside the view) funnels into.
func TestTheSettingsWindowCarriesItsControls(t *testing.T) {
	w, _ := newTestShellWindow(t)
	w.fleet = newTestFleet(t)
	w.openSettings()
	sw := w.settingsContent
	frame := &fakeWindowFrame{}
	sw.frame = frame

	tt := ui.NewTester(sw.view, 820, 620)
	tt.Frame()
	for _, label := range []string{locales.WindowMinimize(), locales.WindowMaximize(), locales.WindowClose()} {
		if _, ok := tt.Find(label); !ok {
			t.Fatalf("the header does not carry the window's %s control; texts = %v", label, tt.Texts())
		}
	}
	if err := tt.Click(locales.WindowClose()); err != nil {
		t.Fatalf("Click(%s): %v", locales.WindowClose(), err)
	}
	if frame.closed == 0 {
		t.Error("the header's close did not close the settings window's frame")
	}
}

// TestEscapeClosesTheSettingsWindow answers the window from inside it: Escape
// puts settings away, the same answer the system's close gives from outside.
func TestEscapeClosesTheSettingsWindow(t *testing.T) {
	w, _ := newTestShellWindow(t)
	w.fleet = newTestFleet(t)
	w.openSettings()
	if !w.settingsOpen {
		t.Fatal("settings did not open")
	}
	sw := w.settingsContent
	tt := ui.NewTester(sw.view, 820, 620)
	tt.Frame()

	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if w.settingsOpen {
		t.Error("Escape did not close settings")
	}
}
