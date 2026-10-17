package main

// What a shell looks like when someone else is typing into it.
//
// The lease already tells every client who holds a shell, so the "someone else
// has this" state needs no new channel: the tab's role frame carries it. This
// file draws it — the glow along the terminal's edge, and a popover naming the
// holder with the way back.
//
// The glow is MyGo's shader: the ring from the chat-glow example, whose colours
// flow around the pane's edge, its line thin and its glow falling off outward.
// MyGo compiles that one shader for whatever draws the window — Metal, Direct3D
// 11, OpenGL or WebGL 2 — and interprets it on the CPU where none can, so there
// is no second, approximate drawing to keep in step: what a preview shows is
// what the window shows.
//
// Three rules keep it honest:
//
//   - it is drawn only while another client holds the lease, so a shell this
//     window controls is drawn exactly as it was before;
//   - it is an overlay the pointer goes through, along the pane's edge rather
//     than across the text: no re-layout, no re-shaping of the grid;
//   - the flow stands still when the desktop asks for less motion, and the fade
//     is an Animate, which asks for us.

import (
	_ "embed"
	"runtime"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"
)

// leaseRingSource is the chat-glow ring, kept as the example writes it: a Go
// file of package main whose Fragment MyGo compiles and runs.
//
//go:embed assets/lease-ring.kage
var leaseRingSource string

// leaseRing is the compiled shader. Its uniforms are looked up once — a lookup
// per frame would parse a name per frame — and the values are set per frame.
var (
	leaseRing       = shader.MustCompile("package main\n" + leaseRingSource)
	ringTime        = leaseRing.Uniform("Time")
	ringStrength    = leaseRing.Uniform("Strength")
	ringPad         = leaseRing.Uniform("Pad")
	ringRadius      = leaseRing.Uniform("Radius")
	leaseRingValues = leaseRing.NewValues()
)

// leaseRingPad is how far the ring sits inside the pane's edge, in DIPs: far
// enough in that the terminal's own rounded corner does not clip it.
const leaseRingPad = float32(7)

// glowFade is how long the glow takes to come up and go away. The state itself
// is a step — a shell is either this window's or it is not — so this is only the
// eye's transition, and a desktop asking for less motion skips it.
const glowFade = 260 * time.Millisecond

// glowEpoch is the clock the flow is measured from, so the colours do not jump
// when a shell is taken over: they are a function of how long a glow has been
// possible, not of when this pane was drawn.
var glowEpoch = time.Now()

// leaseGlow draws the glow for a pane whose lease another client holds, as an
// overlay the pointer goes through. strength is 0 for no glow and 1 for a full
// one; still is what the desktop asked for about motion.
func leaseGlow(c *ui.Context, strength float64, still bool) {
	if strength <= 0.001 {
		return
	}
	corner := paneCorner()
	ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().
		Opacity(float32(strength)).
		Draw(func(p *ui.Painter, r ui.Rect) {
			leaseRing.Draw(p, r, shader.Options{
				Radius: corner,
				Values: leaseRingValues.
					Float(ringTime, float32(glowFlow(c.Now(), still))).
					// Someone else holds the shell, so the ring is at its
					// brightest: coming and going is the element's opacity, which
					// is what the fade animates.
					Float(ringStrength, 1).
					Float(ringPad, leaseRingPad).
					Float(ringRadius, corner),
			})
		})
}

// glowFlow is the shader's Time uniform: seconds of glow, which the ring's
// colours flow with. It stands still where the desktop asks for less motion, so
// the pane still says who holds it without anything moving.
func glowFlow(now time.Time, still bool) float64 {
	if still {
		return 0
	}
	return now.Sub(glowEpoch).Seconds()
}

// paneCorner is the pane's own corner radius, which the ring follows so that it
// sits on the pane's edge rather than across it.
func paneCorner() float32 {
	if runtime.GOOS == "darwin" {
		return 16
	}
	return 10
}

// leasePopoverWidth is how wide the panel under the glow is allowed to be. The
// panel hangs from the top of a pane, and a pane can be narrower than a sentence:
// a panel that grew to fit its words would hang off the terminal it belongs to.
const leasePopoverWidth = 260

// leasePopover is the panel under the glow: who holds the shell, and the way
// back. anchor is the pane it hangs from; open is the window's own state, so
// the panel stays where the user left it. Its width is fixed rather than fitted,
// so it never runs past the pane it is about.
//
// The panel is about the lease and nothing else. Ending the shell and closing
// the tab are deliberately absent: one kills a process for every client and the
// other stops watching it, and neither is a question about who may type.
func leasePopover(c *ui.Context, anchor ui.Element, open *bool, holder string, steal bool, reclaim func()) {
	t := c.Theme()
	ui.Popover(c, anchor, open, func() {
		ui.Column(c).Gap(8).Padding(12).Width(leasePopoverWidth).Children(func() {
			ui.Text(c, holder+" 正在控制这个终端").FontSize(13)
			ui.Text(c, "你的输入不会送进 shell。").
				FontSize(11).TextColor(t.TextMuted)
			take := "拿回控制权"
			if steal {
				// Another machine's shell: this window never had it, so it is
				// taken rather than taken back, and the holder is told.
				take = "接管"
				ui.Text(c, "接管会通知对方，对方会变成只读。").
					FontSize(11).TextColor(t.TextMuted)
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "保持只读").Clicked() {
					*open = false
				}
				if ui.PrimaryButton(c, take).Tooltip("取得控制权（对方会变成只读）").Clicked() {
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
