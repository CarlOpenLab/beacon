package main

import (
	"testing"

	"github.com/CarlOpenLab/term-fleet/internal/uitokens"
)

// The card's corner follows the window's: the system draws the outer arc, the
// card sits one frame inside it, and the two are the same width apart all the
// way round when the inner radius is the outer one less the gap.
//
// A floor keeps the card from being squared off where that arithmetic lands on
// zero: Windows rounds its windows at 8 and the frame is 8, so the parallel
// answer is a square card inside a round window — which reads as no card at
// all. Eight is the window's own curvature, so the card stays as round as the
// window even where it cannot be parallel to it.
func TestCardRadiusFollowsTheWindow(t *testing.T) {
	cases := []struct {
		goos string
		mac  int
		want float32
	}{
		{"darwin", 26, 20}, // macOS 26 (Tahoe): 28 - 8
		{"darwin", 15, 8},  // macOS 15 and earlier: 10 - 8, then the floor
		{"windows", 0, 8},  // 8 - 8 would be 0; the floor keeps the corner
		{"linux", 0, 8},    // 12 - 8 is 4; the floor lifts it to 8
	}
	for _, tc := range cases {
		if got := cardRadiusForVersion(tc.goos, tc.mac); got != tc.want {
			t.Errorf("%s (mac %d): card radius = %.0f, want %.0f", tc.goos, tc.mac, got, tc.want)
		}
	}
}

// No platform squares the card off, and none drifts further from the window
// than the floor allows.
func TestNoPlatformSquaresTheCard(t *testing.T) {
	for _, goos := range []string{"darwin", "windows", "linux"} {
		card := cardRadiusFor(goos)
		if card < minCardRadius {
			t.Errorf("%s: card radius %.0f is below the %.0f floor", goos, card, minCardRadius)
		}
		parallel := windowRadiusFor(goos) - float32(uitokens.CardGap)
		if parallel > minCardRadius && card != parallel {
			t.Errorf("%s: card radius %.0f lost the concentric value %.0f", goos, card, parallel)
		}
	}
}
