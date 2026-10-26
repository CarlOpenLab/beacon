package main

// Watching another machine's shell is a window onto a screen that already has a
// size. The fit shrinks the text until that whole screen is in the pane, so the
// viewer sees what the controller sees instead of the left edge of it.

import (
	"math"
	"testing"

	"github.com/egoist/mygo/plugins/terminal"

	"github.com/CarlOpenLab/beacon/internal/protocol"
	"github.com/CarlOpenLab/beacon/internal/settings"
)

func TestViewerFitScaleMakesTheWholeScreenFit(t *testing.T) {
	// A 120x30 shell in a pane that fits 80x24 at the settings' font: width is
	// the limit, so two thirds of the font fits all 120 columns.
	scale := viewerFitScale(80, 24, 120, 30, 1)
	if want := 80.0 / 120.0; math.Abs(scale-want) > 0.001 {
		t.Fatalf("scale = %v, want %v", scale, want)
	}

	// At that scale the pane fits 120x36, and the next frame's answer is the
	// same scale rather than a step further down or back up.
	if got := viewerFitScale(120, 36, 120, 30, scale); math.Abs(got-scale) > 0.001 {
		t.Errorf("the second frame moved from %v to %v", scale, got)
	}

	// A pane with room to spare does not enlarge the text: fitting is about
	// seeing the whole screen, not filling the window.
	if got := viewerFitScale(200, 60, 120, 30, 1); got != 1 {
		t.Errorf("a roomy pane scaled to %v, want 1", got)
	}

	// A screen that almost fits keeps the settings' font: losing one row is
	// not worth making every line smaller.
	if got := viewerFitScale(114, 30, 120, 30, 1); got != 1 {
		t.Errorf("a nearly fitting screen scaled to %v, want 1", got)
	}

	// Unlaid panes and unlaid screens have nothing to compare yet.
	for _, tc := range [][4]int{{0, 24, 120, 30}, {80, 0, 120, 30}, {80, 24, 0, 30}, {80, 24, 120, 0}} {
		if got := viewerFitScale(tc[0], tc[1], tc[2], tc[3], 1); got != 1 {
			t.Errorf("viewerFitScale%v = %v, want 1", tc, got)
		}
	}
}

func TestFitViewerFontsShrinksAWatchAndRestoresAController(t *testing.T) {
	w, _ := newTestShellWindow(t, "remote")
	w.font = terminal.Font{Size: settings.DefaultFontSize}
	w.store = settings.Open(t.TempDir())

	tb := w.tabs[1]
	tb.alive = true
	tb.machine = "win-2024"
	tb.term = fedTerminal(t, previewScreen())
	tb.term.Resize(120, 30)
	tb.conn = &termConn{lastCols: 80, lastRows: 24}
	tb.role, tb.controller, tb.epoch = protocol.RoleViewer, "mac-studio", 4
	w.tabs[1] = tb

	w.fitViewerFonts()
	if want := 80.0 / 120.0; math.Abs(tb.viewScale-want) > 0.01 {
		t.Fatalf("the watched pane scaled to %v, want %v", tb.viewScale, want)
	}

	// Taking the lease gives the shell this pane's size again, so the fit lets
	// go of the font: the controller is never drawn smaller than settings say.
	tb.setLease(protocol.RoleController, "", 5)
	w.fitViewerFonts()
	if tb.viewScale != 1 {
		t.Errorf("a controlled shell stayed scaled at %v", tb.viewScale)
	}

	// And the switch turns the whole thing off: a watched screen keeps the
	// settings' font even when it is wider than the pane.
	if _, err := w.store.Update(func(st *settings.Settings) { st.ViewerFitOff = true }); err != nil {
		t.Fatal(err)
	}
	tb.setLease(protocol.RoleViewer, "mac-studio", 6)
	tb.viewScale = 0.5
	w.fitViewerFonts()
	if tb.viewScale != 1 {
		t.Errorf("the fit was off but the shell stayed scaled at %v", tb.viewScale)
	}
}
