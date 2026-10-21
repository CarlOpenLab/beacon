package main

import (
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/uitokens"
)

// The chrome's measurements and colors. The colors live in internal/uitokens,
// which the browser client's tokens.css is generated from, so the two clients
// cannot drift; the numbers below are the ones the two layouts share.

// chromeMetrics are the layout numbers the two clients share. They are the CSS
// custom properties of the same names in web/src/style.css, and the app's own
// numbers (see uitokens).
const (
	// headerHeight is a card's header: the session's name, where it stands, and
	// the controls that act on its pane. The header is 2*(barMid-cardGap) = 44, the
	// line the traffic lights and the sidebar's buttons center on.
	headerHeight = uitokens.HeaderH
	// tabBarHeight is the strip of open shells in the browser client, a pill row
	// with room around the pills. The desktop has no strip; its sidebar is the
	// list of shells.
	tabBarHeight = 44
	// statusHeight is the quiet line along the bottom of the browser client.
	statusHeight = 28

	// The corner scale is the CSS one: --radius-s is the small controls, --radius-m
	// the buttons and rows, --radius-l the cards, and a pill is a pill.
	radiusSmall = 6
	radius      = 8
	radiusLarge = uitokens.CardRadius
	radiusPill  = 999

	// spacing is the ui's base unit, matching the CSS 4px scale's first step.
	spacing = 4

	// fontSize is the chrome's body size, matching --fs-13.
	fontSize = 13
)

// darkTheme is the chrome in a dark appearance.
func darkTheme() *ui.Theme { return chromeTheme(ui.DarkTheme(), true) }

// lightTheme is the chrome in a light appearance. The light one is not the dark
// one inverted: it has its own ground, its own accent, and state colors that are
// deeper because they have to read on white.
func lightTheme() *ui.Theme { return chromeTheme(ui.LightTheme(), false) }

// chromeTheme makes a ui theme the app's, from whatever theme the frame or the
// system already had: the accent, the surfaces and the sizes of its controls.
// The appearance's own Dark flag stays the caller's.
func chromeTheme(base *ui.Theme, dark bool) *ui.Theme {
	t := *base
	t.Dark = dark
	pal := uitokens.For(dark)

	t.Background = pal.Sidebar
	// Fields take the platform's surface: white on a light card with a hairline
	// around it, and a wash a little lighter than a dark card. Buttons drawn with
	// chromeButton keep pal.Control; the generated controls read these.
	if dark {
		t.Surface = ui.RGBA(255, 255, 255, 0.06)
		t.SurfaceHover = ui.RGBA(255, 255, 255, 0.09)
		t.SurfacePressed = ui.RGBA(255, 255, 255, 0.12)
		t.Border = ui.RGBA(255, 255, 255, 0.13)
	} else {
		t.Surface = ui.Hex("#ffffff")
		t.SurfaceHover = ui.Hex("#f7f7f9")
		t.SurfacePressed = ui.Hex("#eeeef1")
		t.Border = ui.RGBA(0, 0, 0, 0.13)
	}
	t.Text = pal.Text
	t.TextMuted = pal.TextMuted
	t.Accent = pal.Accent
	t.AccentHover = pal.Accent.Mix(ui.Hex("#ffffff"), 0.08)
	t.AccentPressed = pal.Accent.Mix(ui.Hex("#000000"), 0.08)
	t.AccentText = ui.Hex("#ffffff")
	t.Danger = pal.Danger
	t.Warning = pal.Attention
	t.Success = pal.Success
	t.Selection = pal.Accent.Alpha(0.25)
	t.Focus = pal.Accent
	t.Inverse = pal.Text
	t.InverseText = pal.Sidebar
	t.Scrollbar = pal.KbdBorder
	t.ScrollbarWidth = 8
	t.Radius = 7
	t.Spacing = spacing
	t.FontSize = fontSize
	return &t
}

// terminalTheme is the terminal's colors for an appearance: the app's, which are
// Apple's system colors tuned to read on the card they sit on. ANSI stays
// distinct and TrueColor passes through untouched.
func terminalTheme(dark bool) *terminal.Theme {
	return uitokens.TermTheme(dark)
}
