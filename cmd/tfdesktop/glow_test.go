package main

import (
	"testing"

	"github.com/CarlOpenLab/term-fleet/internal/protocol"
)

// The glow is the one piece of chrome that says a shell is being driven from
// elsewhere, so it has to be on screen exactly when the lease says so — and
// absent otherwise, because a window that glows at its own shells is a window
// nobody can read.
func TestLeaseChromeFollowsTheLease(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	w.fleet = newTestFleet(t)

	// Held by this window: no glow, no popover, no mark on the tab.
	tb := w.tabs[1]
	tb.setLease(protocol.RoleController, "", 3)
	if tb.holder() != "" || !tb.held() {
		t.Fatalf("a controller should read as holding the lease: holder=%q held=%v", tb.holder(), tb.held())
	}

	// Held by someone else: the holder is named, and this window does not hold.
	tb.setLease(protocol.RoleViewer, "c9", 4)
	if got := tb.holder(); got != "c9" {
		t.Fatalf("holder = %q, want c9", got)
	}
	if tb.held() {
		t.Fatal("a viewer reads as holding the lease")
	}

	// The mark is drawn on the tab's row.
	tt := drawNav(t, w)
	if !tt.HasText("c9") {
		t.Fatalf("the tab row does not mark the holder; texts = %v", tt.Texts())
	}

	// Taking it back clears all of it.
	tb.setLease(protocol.RoleController, "", 5)
	if tb.holder() != "" {
		t.Fatalf("holder after reclaim = %q, want none", tb.holder())
	}
}

// The glow's colour moves through the palette over one cycle, and stands still
// where the desktop asks for less motion: the phase is a function of the clock
// only when motion is allowed.
func TestGlowCycleMovesAndStandsStill(t *testing.T) {
	theme := darkTheme()
	if glowColor(theme, 0) != theme.Accent {
		t.Error("the cycle does not start at the accent")
	}
	if glowColor(theme, 0.5) != theme.Success {
		t.Error("the cycle does not reach the live colour at its middle")
	}
	// One full cycle later it is back where it started.
	if glowColor(theme, 1) != glowColor(theme, 0) {
		t.Error("the cycle does not close")
	}
}

// The phase moves with the clock and stands still where the desktop asks for
// less motion: the same clock, two answers.
func TestGlowPhaseMovesAndStandsStill(t *testing.T) {
	now := glowEpoch.Add(glowCycle / 2) // half a cycle in
	if got := glowPhase(now, false); got < 0.45 || got > 0.55 {
		t.Fatalf("half a cycle in, the phase is %v, want about 0.5", got)
	}
	if got := glowPhase(now, true); got != 0 {
		t.Fatalf("with less motion asked for, the phase is %v, want 0", got)
	}
}
