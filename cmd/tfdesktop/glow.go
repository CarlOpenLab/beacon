package main

// What a shell looks like when someone else is typing into it.
//
// The lease already tells every client who holds a shell, so the "someone else
// has this" state needs no new channel: the tab's role frame carries it. This
// file draws it — a glow along the terminal's edge, breathing through the
// palette, and a popover naming the holder with the way back.
//
// The shape follows mygo's chat-glow example: colours moving about the pane, a
// crisp line with softer layers over it. The pinned ui strokes a rounded rect
// in one colour at a time and has no conic gradient to sweep around one, so the
// sweep is the ring's colour cycling rather than a wheel of colour turning:
// the same read — this pane is not yours right now — for a dozen strokes a
// frame instead of a shader.
//
// Three rules keep it honest:
//
//   - it is drawn only while another client holds the lease, so a shell this
//     window controls is drawn exactly as it was before;
//   - it is an overlay the pointer goes through, along the pane's edge rather
//     than across the text: no re-layout, no re-shaping of the grid;
//   - the breathing stands still when the desktop asks for less motion, and the
//     fade is an Animate, which asks for us.

import (
	"math"
	"runtime"
	"time"

	"github.com/egoist/mygo/ui"
)

// glowCycle is how long the ring takes to breathe through the palette.
const glowCycle = 7 * time.Second

// glowFade is how long the glow takes to come up and go away. The state itself
// is a step — a shell is either this window's or it is not — so this is only the
// eye's transition, and a desktop asking for less motion skips it.
const glowFade = 260 * time.Millisecond

// glowBands are the glow's layers along the pane's edge: this wide, this
// transparent, the innermost the brightest, as the chat-glow example stacks
// them.
var glowBands = []struct {
	width float32
	alpha float32
}{{10, 0.08}, {6, 0.14}, {3.5, 0.24}, {1.5, 0.9}}

// leaseGlow draws the glow for a pane whose lease another client holds, as an
// overlay the pointer goes through. strength is 0 for no glow and 1 for a full
// one; still is what the desktop asked for about motion.
func leaseGlow(c *ui.Context, strength float64, still bool) {
	if strength <= 0.001 {
		return
	}
	t := c.Theme()
	color := glowColor(t, glowPhase(c.Now(), still))
	corner := float32(10)
	if runtime.GOOS == "darwin" {
		corner = 16
	}
	ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().
		Opacity(float32(strength)).
		Draw(func(p *ui.Painter, r ui.Rect) {
			for _, band := range glowBands {
				p.Stroke(r, color.Alpha(band.alpha), corner, band.width)
			}
		})
}

// glowColor is the cycle's colour at a point in it, 0 to 1: the accent that
// says "chosen", the warning that says "busy", and back through the green that
// says "live".
func glowColor(t *ui.Theme, at float64) ui.Color {
	at = math.Mod(at, 1)
	switch {
	case at < 0.25:
		return mixOklab(t.Accent, t.Warning, at*4)
	case at < 0.5:
		return mixOklab(t.Warning, t.Success, (at-0.25)*4)
	case at < 0.75:
		return mixOklab(t.Success, t.Warning, (at-0.5)*4)
	default:
		return mixOklab(t.Warning, t.Accent, (at-0.75)*4)
	}
}

// glowPhase is where the cycle is, 0 to 1. It stands still where the desktop
// asks for less motion — the phase is then always the start of the cycle,
// whatever the clock says.
func glowPhase(now time.Time, still bool) float64 {
	if still {
		return 0
	}
	return math.Mod(now.Sub(glowEpoch).Seconds()/glowCycle.Seconds(), 1)
}

// glowEpoch is the clock the cycle is measured from, set at the first frame
// that draws a glow so the phase does not jump when a shell is taken over.
var glowEpoch = time.Now()

// leasePopover is the panel under the glow: who holds the shell, and the way
// back. anchor is the pane it hangs from; open is the window's own state, so
// the panel stays where the user left it.
func leasePopover(c *ui.Context, anchor ui.Element, open *bool, holder string, steal bool, reclaim func()) {
	t := c.Theme()
	ui.Popover(c, anchor, open, func() {
		ui.Column(c).Gap(8).Padding(12).Children(func() {
			ui.Text(c, "这个终端正被 "+holder+" 控制").FontSize(13)
			ui.Text(c, "你现在的输入不会送进 shell；拿回控制权后恢复。").
				FontSize(11).TextColor(t.TextMuted)
			if steal {
				ui.Text(c, "这是另一台机器上的 shell，拿回时会通知对方。").
					FontSize(11).TextColor(t.TextMuted)
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "保持只读").Clicked() {
					*open = false
				}
				if ui.PrimaryButton(c, "拿回控制权").Clicked() {
					reclaim()
					*open = false
				}
			})
		})
	})
}

// leaseTabMark is the small mark on a tab whose lease another client holds. The
// tab strip has no room for a sentence; the glow and this are enough to say
// which shells are being driven from elsewhere.
func leaseTabMark(c *ui.Context, holder string) {
	t := c.Theme()
	ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(7, 7).Radius(3.5).Background(t.Warning)
		ui.Text(c, holder).FontSize(10).TextColor(t.Warning)
	})
}

// mixOklab blends two colours, as the ui's own gradients mix them, so the
// cycle's midpoints keep their brightness instead of going muddy.
func mixOklab(from, to ui.Color, at float64) ui.Color {
	return from.Mix(to, float32(math.Min(math.Max(at, 0), 1)))
}
