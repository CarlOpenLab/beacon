package main

import (
	"testing"

	"github.com/CarlOpenLab/term-fleet/internal/uitokens"
)

// TestCardRadiusIsConcentric holds the rule the card's corner follows: it is
// the window's radius minus the frame around it, so the two arcs share a centre
// and the 8px wash keeps the same width at the corner as along the edges.
//
// The rule is pinned rather than a number: the window's corner belongs to the
// system, and macOS 26 made it much rounder than macOS 15. A card that keeps a
// literal radius drifts out of that relationship silently — only the gap at the
// corner changes, by a few pixels, which reads as two unrelated cards.
func TestCardRadiusIsConcentric(t *testing.T) {
	for _, goos := range []string{"darwin", "windows", "linux"} {
		win, card := windowRadiusFor(goos), cardRadiusFor(goos)
		if want := max(0, win-float32(uitokens.CardGap)); card != want {
			t.Errorf("%s: card radius %.0f != window radius %.0f - gap %d", goos, card, win, uitokens.CardGap)
		}
		if card < 0 {
			t.Errorf("%s: card radius is negative (%.0f)", goos, card)
		}
	}
}

// The machine this is built and used on is macOS 26, where the window corner is
// 28: a card that still used the old 14 would pinch the frame to five pixels at
// the corner. Twenty is the number that keeps it round and parallel.
func TestDarwinCardStaysRound(t *testing.T) {
	if got := cardRadiusFor("darwin"); got != 20 {
		t.Fatalf("darwin card radius = %.0f, want 20 (window 28 - gap 8)", got)
	}
}
