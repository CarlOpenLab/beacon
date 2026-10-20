package main

import (
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/uitokens"
)

// Surfaces stand out by their color and an edge, crisp, on whole pixels. In the
// light, the edge is a small soft shadow and no line; in the dark, where shadows
// hardly show, a hairline a little lighter than the surface, over a dark shadow,
// as a lit edge. These are kero's levels, and the same numbers either way.

// raised lifts a surface off what is under it a little: a card, a chosen row.
func raised(e ui.Element, pal *uitokens.Palette) ui.Element {
	if pal.Dark {
		return darkShadow(litEdge(e, pal))
	}
	return e.Shadow(0, 1, 2, 0, pal.CardShadow).Shadow(0, 1, 6, 0, pal.CardShadow2)
}

// chosen lifts the chosen row of a list a hair: an even ring a shade darker than
// what is under it, and a faint shadow close under it. In the dark, as raised.
func chosen(e ui.Element, pal *uitokens.Palette) ui.Element {
	if pal.Dark {
		return raised(e, pal)
	}
	return e.Shadow(0, 0, 0, 0.5, ui.RGBA(0, 0, 0, 0.06)).Shadow(0, 0.5, 2, 0, ui.RGBA(0, 0, 0, 0.05))
}

// floating lifts a surface above the window: the palette, dialogs.
func floating(e ui.Element, pal *uitokens.Palette) ui.Element {
	if pal.Dark {
		return darkShadow(litEdge(e, pal)).Shadow(0, 16, 40, 0, pal.FloatShadow)
	}
	return e.Shadow(0, 1, 3, 0, pal.CardShadow).Shadow(0, 16, 40, 0, pal.FloatShadow)
}

// litEdge draws a dark surface's edge as if lit from above: a 1px border in the
// text's color, 18% at the top fading evenly to 6% at the bottom.
func litEdge(e ui.Element, pal *uitokens.Palette) ui.Element {
	return e.Border(1, pal.Text.Alpha(0.12)).BorderLinearGradient(ui.LinearGradient{
		Angle: 180, From: pal.Text.Alpha(0.18), To: pal.Text.Alpha(0.06),
	})
}

// darkShadow is a surface's shadow in the dark: three tight layers.
func darkShadow(e ui.Element) ui.Element {
	return e.Shadow(0, 1, 1, 0, ui.RGBA(0, 0, 0, 0.2)).
		Shadow(0, 2, 2, -1, ui.RGBA(0, 0, 0, 0.2)).
		Shadow(0, 4, 6, -3, ui.RGBA(0, 0, 0, 0.24))
}

// face styles a control's face: a soft shadow in the light, a lit edge in the
// dark, as macOS draws its buttons.
func face(e ui.Element, pal *uitokens.Palette, bg ui.Color) ui.Element {
	e.Background(bg)
	if !pal.Dark {
		return e.Shadow(0, 0.5, 1.5, 0, ui.RGBA(0, 0, 0, 0.13)).Shadow(0, 0, 0, 0.5, ui.RGBA(0, 0, 0, 0.05))
	}
	return darkShadow(litEdge(e, pal))
}

// keroButton is a button as kero draws it: a raised surface, or the accent's for
// the primary one, with a label and, optionally, its keys.
func keroButton(c *ui.Context, pal *uitokens.Palette, key, label, keys string, primary bool) bool {
	b := ui.ButtonBase(c.Key(key)).Height(30).Padding(0, 14).Gap(8).Radius(8).AlignItems(ui.Center).Label(label)
	bg, fg := pal.Control, pal.Text
	if primary {
		bg, fg = pal.Accent, ui.Hex("#ffffff")
	}
	switch {
	case b.Pressed():
		bg = bg.Mix(ui.Hex("#000000"), 0.08)
	case b.Hovered() && primary:
		bg = bg.Mix(ui.Hex("#ffffff"), 0.08)
	case b.Hovered():
		bg = pal.ControlHover
	}
	face(b, pal, b.AnimateColor("bg", bg, ui.Animation{Duration: 90e6})).TextColor(fg)
	b.Children(func() {
		ui.Text(c, label).FontSize(13).FontWeight(500).SingleLine()
		if keys != "" {
			ui.Text(c, keys).FontSize(12).TextColor(fg.Alpha(0.55)).SingleLine()
		}
	})
	return b.Clicked()
}

// toggle is kero's switch: a track that shows on and off alike, the accent's when
// on, with a knob that springs across. It returns the switch.
func toggle(c *ui.Context, pal *uitokens.Palette, key string, on *bool) ui.Element {
	sw := ui.SwitchBase(c.Key(key), on).Size(38, 22).Radius(11).Padding(2).Shrink(0)
	track := pal.Text.Alpha(0.14)
	if pal.Dark {
		track = ui.RGBA(255, 255, 255, 0.2)
	}
	if *on {
		track = pal.Accent
	}
	sw.Background(sw.AnimateColor("track", track, ui.Animation{Duration: 150e6}))
	x := sw.AnimateTo("knob", b2f(*on)*16, ui.Animation{Spring: ui.SpringOptions{Duration: 260e6, Bounce: 0.15}})
	sw.Children(func() {
		ui.Box(c).Size(18, 18).Radius(9).Background(ui.Hex("#ffffff")).Translate(x, 0).
			Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.22)).Shadow(0, 0, 0, 0.5, ui.RGBA(0, 0, 0, 0.06))
	})
	return sw
}

// b2f is a bool as the float an animation interpolates.
func b2f(b bool) float32 {
	if b {
		return 1
	}
	return 0
}
