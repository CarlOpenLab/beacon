package main

// The settings dialog is a floating window, not a sheet over the window's wash:
// the page stays in sight behind it, and the pointer drags it aside by its
// header. These are the two things the report asked for.

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/beacon/internal/settings"
	"github.com/CarlOpenLab/beacon/locales"
)

// TestSettingsLeavesThePageInSight reads the backdrop off the pixels: with the
// dialog open, a corner outside it looks exactly as it does with it closed. The
// wash that used to cover the shells would have changed it. The acrylic window
// is translucent by default, and the tester composites a translucent page over
// its own canvas, so the page is made opaque here: the comparison then reads the
// page itself, whatever the compositor does underneath, and the acrylic blur
// stays on.
func TestSettingsLeavesThePageInSight(t *testing.T) {
	w, _ := newTestShellWindow(t)
	w.fleet = newTestFleet(t)
	if _, err := w.store.Update(func(s *settings.Settings) { s.BackgroundOpacity = 1 }); err != nil {
		t.Fatal(err)
	}

	const width, height = 1000, 700
	// A point in the window's bottom-left corner, clear of the centred dialog
	// (820 wide at this size, so it starts 90 in).
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
		t.Errorf("the dialog's backdrop changed the page: closed (%d,%d,%d), open (%d,%d,%d)",
			r1>>8, g1>>8, b1>>8, r2>>8, g2>>8, b2>>8)
	}
}

// TestTheSettingsDialogFollowsItsHeader drags the dialog by its title: it moves
// on the window, and the window it belongs to does not.
func TestTheSettingsDialogFollowsItsHeader(t *testing.T) {
	w, _ := newTestShellWindow(t)
	w.fleet = newTestFleet(t)

	tt := ui.NewTester(w.view, 1000, 700)
	tt.Frame()
	if err := tt.Click(locales.ActionSettings()); err != nil {
		t.Fatalf("Click(%s): %v", locales.ActionSettings(), err)
	}
	tt.Frame()

	// The dialog's own × names the header it sits in; the title text alone is
	// ambiguous, as the sidebar's settings button says the same word.
	closeBtn, ok := tt.Find(locales.SettingsClose())
	if !ok {
		t.Fatalf("the dialog's close button is not on screen; texts = %v", tt.Texts())
	}
	// The nav's first page sits in the dialog, so its left edge names where the
	// dialog starts: press the header's padding there, clear of its buttons.
	page, ok := tt.Find(locales.SettingsPaneTerminal())
	if !ok {
		t.Fatalf("the dialog's pages are not on screen; texts = %v", tt.Texts())
	}
	x, y := page.X-4, closeBtn.Y+closeBtn.H/2
	tt.Press(x, y)
	tt.Frame()
	tt.Move(x+60, y+20)
	tt.Frame()
	tt.Release(x+60, y+20)
	tt.Frame()

	if !w.settingsOpen {
		t.Fatal("the drag closed the dialog")
	}
	if w.settingsDrag.X < 40 || w.settingsDrag.X > 80 || w.settingsDrag.Y < 10 || w.settingsDrag.Y > 30 {
		t.Fatalf("the dialog moved to %+v after a 60,20 drag, want about 60,20", w.settingsDrag)
	}
}
