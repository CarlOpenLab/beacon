package main

// What a shell looks like when someone else is typing into it.
//
// The lease already tells every client who holds a shell, so the "someone else
// has this" state needs no new channel: the tab's role frame carries it. Two
// pieces of chrome say it, and each has a job the other cannot do:
//
//   - a banner in the card's header naming who holds the shell on screen — or
//     that nobody does — with the way back. It shares the header row rather
//     than taking a strip of its own, so taking or releasing the lease never
//     re-fits the terminal under it: the same shell stays the same rectangle.
//     Every other pane of a split says 只读 (or 已退出) in its own header instead;
//   - a panel under the top of the pane saying what holding it means, and that
//     pressing the button will end the other client's typing.
//
// And one says something the others cannot: the glow along the pane's edge. It
// is drawn for one state only — a shell this machine runs, being driven by
// another client — and that is the whole point of it. A person sits down at
// this machine, sees a terminal of theirs lit at the edge, and knows without
// reading anything that somebody else has their keyboard. On another machine's
// shell the same glow would mean nothing: nobody here owns that process, and a
// window that lit up for every shell someone else held would be lighting up for
// the ordinary case, which is how a signal stops being one.

import (
	_ "embed"
	"time"

	"github.com/CarlOpenLab/term-fleet/locales"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"
)

// leaseGlowSource is the shader below, kept as MyGo's examples write one: a Go
// file of package main whose Fragment MyGo compiles for whatever draws the
// window and runs everywhere else.
//
//go:embed assets/lease-glow.kage
var leaseGlowSource string

// leaseGlow is the compiled shader. Its uniforms are looked up once — a lookup
// per frame would parse a name per frame — and the values are set per frame.
var (
	leaseGlowShader = shader.MustCompile("package main\n" + leaseGlowSource)
	glowTime        = leaseGlowShader.Uniform("Time")
	glowStrength    = leaseGlowShader.Uniform("Strength")
	glowPadUniform  = leaseGlowShader.Uniform("Pad")
	glowRadius      = leaseGlowShader.Uniform("Radius")
	leaseGlowValues = leaseGlowShader.NewValues()
)

// leaseGlowPad is how far the overlay's own edge is from the light, in DIPs.
// The pane floats paneInset inside the body, so the light is centred on the
// pane's edge and half of it falls on the terminal, half on the ground the pane
// floats on. A light that could not reach the ground would be a coloured border
// painted on the pane rather than light along it.
const leaseGlowPad = float32(paneInset)

// leaseGlowRadius is the light's own corner: the sheet's radius, carried out to
// where the light runs. Read from the sheet rather than written twice, because a
// light drawn for a different corner than the pane has is a light that misses
// the corner.
const leaseGlowRadius = float32(0)

// glowFade is how long the light takes to come up and go away. The state itself
// is a step — a shell either has somebody else on it or it does not — so this is
// only the eye's transition, and a desktop asking for less motion skips it.
const glowFade = 320 * time.Millisecond

// glowEpoch is the clock the drift is measured from, so the colours do not jump
// when a shell is taken over: they are a function of how long there has been a
// light to show, not of when this pane was drawn.
var glowEpoch = time.Now()

// drivenElsewhere is whether the shell on screen is one this machine runs and
// another client is driving. It is the condition the glow is drawn under, and
// the only condition it is drawn under — see the note at the top of this file.
func (w *window) drivenElsewhere(tb *tab) bool {
	return tb != nil && tb.alive && tb.machine == machineLocal && tb.holder() != ""
}

// readOnly is whether the shell on screen is one this window may not type into:
// alive, with a lease that is somebody else's or nobody's. The pane's 只读 chip,
// the band above it and the panel over it are drawn from this answer and from
// nothing else, so the three can never disagree about it.
//
// Nobody's is the same answer as somebody else's here — a shell no one drives
// takes no typing either — and it is the state a remote shell lands in when its
// lease is let go with the machine that owns it not watching. Not knowing yet is
// a separate case: the word arrives in a role frame.
func readOnly(tb *tab) bool {
	return tb.alive && tb.known() && !tb.held()
}

// leaseGlow draws the light along the edge of a pane another client holds, as an
// overlay the pointer goes through. It covers the whole body rather than the
// pane, because that is the room the light needs; the pane's own edge is
// leaseGlowPad in from this box on every side.
//
// strength is 0 for no light and 1 for a full one; still is what the desktop
// asked for about motion.
func leaseGlow(c *ui.Context, strength float64, still bool) {
	if strength <= 0.001 {
		return
	}
	ui.Box(c).Absolute().Top(0).Start(0).End(0).Bottom(0).PassThrough().
		Opacity(float32(strength)).
		Draw(func(p *ui.Painter, r ui.Rect) {
			leaseGlowShader.Draw(p, r, shader.Options{
				Values: leaseGlowValues.
					Float(glowTime, float32(glowDrift(c.Now(), still))).
					// Somebody else holds the shell, so the light is at its
					// brightest: coming and going is the element's opacity, which
					// is what the fade animates.
					Float(glowStrength, 1).
					Float(glowPadUniform, leaseGlowPad).
					Float(glowRadius, leaseGlowRadius),
			})
		})
}

// glowDrift is the shader's Time uniform: seconds of light, which the colours
// drift with. It stands still where the desktop asks for less motion, so the
// pane still says who holds it without anything moving — the same light, in the
// same place, in colours that happen not to travel.
func glowDrift(now time.Time, still bool) float64 {
	if still {
		return 0
	}
	return now.Sub(glowEpoch).Seconds()
}

// leasePopoverWidth is how wide the panel over the pane's foot is allowed to be.
// The panel sits at the bottom center of the pane, and a pane can be narrower
// than a sentence: a panel that grew to fit its words would hang off the
// terminal it belongs to.
const leasePopoverWidth = 320

// leasePopover is the panel that names the holder of the shell on screen, and
// the way back. anchor is a point at the pane's bottom center, so the panel sits
// there rather than over the top-left corner of the terminal the person is
// reading. open is the window's own state, so the panel stays where the user
// left it. Its width is fixed rather than fitted, so two panels read the same
// and the lines do not reflow as the holder's name changes.
//
// It says three things and no more: who has the shell, what that means for the
// person typing here, and what pressing the button will do to the other side.
// The last one is the one an interface usually leaves out, and it is the one
// that decides whether the button gets pressed.
//
// It has a state for a holder that is nobody. The panel opens on the frame that
// takes a shell away from this window, and that frame can hand it to no one: a
// remote shell whose lease was let go with the machine that owns it not
// watching is free, and a panel that said " 正在控制这个终端" with a blank where
// a name goes would be a panel about nobody. The offer stands all the same — a
// claim on a free lease is honoured at once, and there is no one to steal from.
//
// Ending the shell and closing the tab are deliberately absent: one kills a
// process for every client and the other stops watching it, and neither is a
// question about who may type.
func leasePopover(c *ui.Context, anchor ui.Element, open *bool, holder string, steal bool, reclaim func()) {
	t := c.Theme()
	pal := paletteFor(c)

	// The word is about which machine the shell runs on: one this window never
	// drove is 接管, one it drove and lost is 拿回.
	take := locales.ActionTakeBack()
	if steal {
		take = locales.ActionTakeOver()
	}
	who := locales.LeaseWho(holder)
	outcome := locales.LeaseOutcomeBack(holder)
	if holder == "" {
		// Nobody's shell: the panel says so rather than leaving a blank where a
		// name goes, and the button promises nothing about a person who is not
		// there.
		who = locales.LeaseWhoNobody()
		outcome = locales.LeaseOutcomeFree()
	} else if steal {
		// Another machine's shell: this window never had it, so it is taken
		// rather than taken back, and the holder is told.
		outcome = locales.LeaseOutcomeOver(holder)
	}

	ui.PopoverBase(c, anchor, open, func(panel ui.Element) {
		// Above the anchor's point, its own bottom edge on it: the panel sits at
		// the bottom center of the pane, clear of the last line a person reads.
		panel.AttachTo(anchor, ui.AnchorTop, ui.AnchorBottom).Label("lease-panel")
		floating(panel.Width(leasePopoverWidth).Radius(14).Background(pal.Popover).Padding(14), pal)
		ui.Column(c).Gap(0).Children(func() {
			// Who holds it, with the dot the 会话 row carries and in the colour
			// the pane's 只读 chip is drawn in, so the three never disagree.
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(8, 8).Radius(4).Background(t.Warning)
				ui.Text(c, who).FontSize(13).FontWeight(600).TextColor(t.Text).SingleLine()
			})
			ui.Text(c, locales.LeaseReadonlyNote()).
				FontSize(12).TextColor(t.TextMuted).Margin(9, 0, 0, 0).LineHeight(1.4)
			ui.Text(c, outcome).
				FontSize(12).TextColor(t.TextMuted).Margin(4, 0, 0, 0).LineHeight(1.4)
			ui.Row(c).Gap(8).Justify(ui.End).Margin(14, 0, 0, 0).Children(func() {
				if chromeButton(c, pal, "lease-stay", locales.LeaseStay(), "", false) {
					*open = false
				}
				if chromeButton(c, pal, "lease-take", take, "", true) {
					reclaim()
					*open = false
				}
			})
		})
	})
}
