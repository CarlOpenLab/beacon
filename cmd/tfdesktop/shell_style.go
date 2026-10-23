package main

// The window's corner, and the card's corner inside it.
//
// The window's rounding is the system's — we do not draw it — but the card sits
// a fixed gap inside it, so the card has to know how round the window is. The
// relationship is the whole point: for the 8px frame to read as one width all
// the way round, the inner corner must be the outer radius minus the gap, with
// both arcs sharing a centre. A number written down on its own drifts away from
// the window's as soon as the system changes (macOS 26 made its corners much
// rounder than macOS 15), and the gap pinches at the corner before anyone
// notices why.

import (
	"runtime"

	"github.com/CarlOpenLab/term-fleet/internal/uitokens"
)

// windowRadiusFor is the radius the system draws. These three numbers belong to
// the operating system, not to this app's radius scale: they are facts to be
// followed, not tokens to be chosen.
func windowRadiusFor(goos string) float32 {
	switch goos {
	case "darwin":
		// macOS 26 (Tahoe) rounds its windows at roughly 28; macOS 15 and
		// earlier used 10. This is the same measurement the Gloss shell makes,
		// and it is the macOS this app is built and used on.
		return 28
	case "windows":
		// Windows 11's DWM corner.
		return 8
	default:
		// The common Linux CSD corner (GNOME's Adwaita).
		return 12
	}
}

// minCardRadius is the smallest corner a card keeps, and it exists because the
// arithmetic alone gives Windows 0: an 8px window corner less an 8px frame is a
// square card. Perfectly parallel, and it reads as no card at all — a square
// panel inside a rounded window pinches the wash at the corner instead of
// widening it, and the window's own rounding calls attention to it. Eight is
// the window's own curvature, so the card is as round as the window even where
// it cannot be parallel to it; the frame widens by a little over a pixel at the
// corner, which the eye reads as a frame rather than as a mistake.
const minCardRadius float32 = 8

// cardRadiusForVersion is the card's corner: the window's, less the frame
// around it, and never below minCardRadius. The version matters only on macOS,
// where the window corner moved from 10 to 28 in 26.
func cardRadiusForVersion(goos string, macMajor int) float32 {
	win := windowRadiusFor(goos)
	if goos == "darwin" && macMajor < 26 {
		win = 10
	}
	return max(minCardRadius, win-float32(uitokens.CardGap))
}

// cardRadiusFor is the card's corner for a platform at its current version.
func cardRadiusFor(goos string) float32 { return cardRadiusForVersion(goos, 26) }

// cardRadius is the workspace card's corner on this machine.
func cardRadius() float32 { return cardRadiusForVersion(runtime.GOOS, macMajorVersion()) }
