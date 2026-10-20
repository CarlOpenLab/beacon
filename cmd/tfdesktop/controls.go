package main

import (
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/uitokens"
)

// kero's small things: a chip, a keystroke, a status glyph. They are the pieces
// the sidebar and a card's header are built from, so they live in one place and
// the two cannot drift.

// chipBox is a chip: a pill of quiet text, on the ground it names.
func chipBox(c *ui.Context, pal *uitokens.Palette, bg, fg ui.Color, fn func()) ui.Element {
	return ui.Row(c).Height(22).Padding(0, 9).Gap(6).Radius(11).Shrink(0).
		AlignItems(ui.Center).Background(bg).TextColor(fg).Children(fn)
}

// kbd is a keystroke as kero writes one: small, in the chrome's quiet greys.
func kbd(c *ui.Context, pal *uitokens.Palette, keys string) {
	ui.Text(c, keys).FontSize(11).TextColor(pal.TextMuted).Padding(1, 5).Radius(4).
		Background(pal.Kbd).SingleLine()
}

// itoa is strconv.Itoa for the small numbers the chrome draws, so that the call
// sites read as one word.
func itoa(n int) string { return strconv.Itoa(n) }
