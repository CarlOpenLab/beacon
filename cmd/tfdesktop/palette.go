package main

import (
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

// The window's palette is the one the browser client is drawn with, so a shell
// on this machine and a shell in a tab look like the same product.
//
// The design is "Editor Native": the browser client's stylesheet (web/src/style.css)
// holds it as CSS custom properties, and this file is the same numbers on the
// ui's terms. What it says, in one place:
//
//   - Two grounds: a window that is nearly black (or, in the light appearance,
//     nearly white), and panels — header, tabs, composer, status — one step off
//     it, with the sidebar a step below. Hierarchy is that difference and a
//     hairline, never a heavy border.
//   - One accent — a violet — is the only saturated colour in the chrome, so it
//     always means "the thing that is chosen or in control".
//   - State has its own colours and they mean the same thing everywhere: green
//     for live, amber for read-only, red for gone, grey for exited.
//   - The terminal keeps a dark sheet in both appearances: the ANSI sixteen are
//     drawn for a dark ground, and inverting them makes success and failure read
//     wrong. Only the chrome around the sheet flips.
//
// The numbers below are the CSS tokens, one for one. When the browser client's
// tokens change, these change with them.

// chromeMetrics are the layout numbers the two clients share. They are the CSS
// custom properties of the same names in web/src/style.css.
const (
	// headerHeight is the band at the top of the window: the brand, what the
	// window has to say, and the controls that act on the window. It is the
	// browser client's --h-header, and its window-drag band.
	headerHeight = 44
	// tabBarHeight is the strip of open shells under it: --tabsbar, a pill row
	// with room around the pills.
	tabBarHeight = 44
	// statusHeight is the quiet line along the bottom: --h-status.
	statusHeight = 28

	// The corner scale is the CSS one: --radius-s is the small controls, --radius-m
	// the buttons and rows, --radius-l the sheets and the composer, and a pill is
	// a pill. The ui has one Radius on its theme for the controls that ask for
	// nothing in particular, which is the medium.
	radiusSmall = 6
	radius      = 10
	radiusLarge = 14
	radiusPill  = 999

	// spacing is the ui's base unit, matching the CSS 4px scale's first step.
	spacing = 4

	// fontSize is the chrome's body size, matching --fs-13.
	fontSize = 13
)

// darkTheme is the shell in the set dark appearance: the browser client's
// [data-theme="dark"] block, token for token.
func darkTheme() *ui.Theme {
	return &ui.Theme{
		Dark:           true,
		Background:     ui.Hex("#101014"),
		Surface:        ui.Hex("#16161c"),
		SurfaceHover:   ui.Hex("#1d1d24"),
		SurfacePressed: ui.Hex("#23232c"),
		Border:         ui.Hex("#27272f"),
		Text:           ui.Hex("#e9e9f0"),
		TextMuted:      ui.Hex("#b2b2c0"),
		Accent:         ui.Hex("#a48bfa"),
		AccentHover:    ui.Hex("#8f75f2"),
		AccentPressed:  ui.Hex("#8f75f2"),
		AccentText:     ui.Hex("#16102b"),
		Danger:         ui.Hex("#f2707a"),
		Warning:        ui.Hex("#dfa73f"),
		Success:        ui.Hex("#4cc38a"),
		Selection:      ui.RGBA(0xa4, 0x8b, 0xfa, 0.32),
		Focus:          ui.Hex("#a48bfa"),
		Inverse:        ui.Hex("#e9e9f0"),
		InverseText:    ui.Hex("#101014"),
		Scrollbar:      ui.Hex("#27272f"),
		ScrollbarWidth: 8,
		Radius:         radius,
		Spacing:        spacing,
		FontSize:       fontSize,
	}
}

// lightTheme is the shell in the set light appearance: the browser client's
// [data-theme="light"] block, token for token. It is not the dark block
// inverted: the light appearance has its own ground, its own accent, and its own
// state colours, which are darker because they have to be read on white.
func lightTheme() *ui.Theme {
	return &ui.Theme{
		Background:     ui.Hex("#f7f7f8"),
		Surface:        ui.Hex("#ffffff"),
		SurfaceHover:   ui.Hex("#eeeef2"),
		SurfacePressed: ui.Hex("#e8e8ee"),
		Border:         ui.Hex("#dcdce3"),
		Text:           ui.Hex("#1b1b22"),
		TextMuted:      ui.Hex("#4c4c58"),
		Accent:         ui.Hex("#5f45e6"),
		AccentHover:    ui.Hex("#4f37d4"),
		AccentPressed:  ui.Hex("#4f37d4"),
		AccentText:     ui.Hex("#ffffff"),
		Danger:         ui.Hex("#c2394a"),
		Warning:        ui.Hex("#8f6410"),
		Success:        ui.Hex("#177d4b"),
		Selection:      ui.RGBA(0x5f, 0x45, 0xe6, 0.22),
		Focus:          ui.Hex("#5f45e6"),
		Inverse:        ui.Hex("#1b1b22"),
		InverseText:    ui.Hex("#f7f7f8"),
		Scrollbar:      ui.Hex("#dcdce3"),
		ScrollbarWidth: 8,
		Radius:         radius,
		Spacing:        spacing,
		FontSize:       fontSize,
	}
}

// shellColors are the shades the shell draws that the ui's theme has no name
// for: the ground of the panels, the sidebar's recessed ground, the face of a
// raised pill, the hairlines between them, the wash behind a chosen row and the
// quiet of a control that is not on. They are the CSS tokens --bg-raised,
// --bg-sunken, --tab-on, --line, --line-soft, --active, --kbd-bg and --fg-dim,
// which have no ui.Theme field.
type shellColors struct {
	pane     ui.Color // the header, the tabs, the composer, the status bar
	side     ui.Color // the sidebar, one step below the window ground
	field    ui.Color // chips, keys and pills: --kbd-bg, as a solid
	tabOn    ui.Color // the raised pill of the tab on screen: --tab-on
	line     ui.Color // a border that has to be seen: --line
	hairline ui.Color // the line between panes: --line-soft
	chosen   ui.Color // the wash behind a chosen row: --active
	icon     ui.Color // drawings and secondary text: --fg-2
	quiet    ui.Color // meta and placeholders: --fg-dim
}

func shellColorsFor(t *ui.Theme) shellColors {
	if t.Dark {
		return shellColors{
			pane:     ui.Hex("#16161c"),
			side:     ui.Hex("#0b0b0e"),
			field:    ui.Hex("#1e1e24"),
			tabOn:    ui.Hex("#23232c"),
			line:     ui.Hex("#27272f"),
			hairline: ui.Hex("#1c1c22"),
			chosen:   ui.RGBA(0xa4, 0x8b, 0xfa, 0.10),
			icon:     ui.Hex("#b2b2c0"),
			quiet:    ui.Hex("#8c8c9c"),
		}
	}
	return shellColors{
		pane:     ui.Hex("#ffffff"),
		side:     ui.Hex("#eeeef2"),
		field:    ui.Hex("#ececf0"),
		tabOn:    ui.Hex("#ffffff"),
		line:     ui.Hex("#dcdce3"),
		hairline: ui.Hex("#e8e8ee"),
		chosen:   ui.RGBA(0x5f, 0x45, 0xe6, 0.07),
		icon:     ui.Hex("#4c4c58"),
		quiet:    ui.Hex("#75757f"),
	}
}

// The shell's palette on the terminal's own terms, so the pane a shell runs in
// belongs to the window around it instead of looking like a program that
// wandered in. The sixteen program colours are the browser client's terminal
// theme in web/src/term.ts, which the two clients render ANSI with: the
// terminal's accent is the chrome's violet, so a prompt and a chosen tab are the
// same colour.
//
// The sheet is dark in both appearances, so this answers the same thing whichever
// appearance the window is in. It takes the flag and returns one palette anyway:
// a caller asking for the light appearance is asking the question this answers.
func terminalTheme(_ bool) *terminal.Theme {
	return &terminal.Theme{
		Foreground: ui.Hex("#d7dae2"),
		Background: ui.Hex("#0d0d12"),
		Cursor:     ui.Hex("#b6a6ff"),
		CursorText: ui.Hex("#0d0d12"),
		Selection:  ui.RGBA(0xb6, 0xa6, 0xff, 0.28),
		Palette: [16]ui.Color{
			ui.Hex("#26262e"), ui.Hex("#ff8b93"), ui.Hex("#6fd39a"), ui.Hex("#e6c07d"),
			ui.Hex("#9ecbff"), ui.Hex("#b6a6ff"), ui.Hex("#7dd3fc"), ui.Hex("#d7dae2"),
			ui.Hex("#4a4a56"), ui.Hex("#ffa3aa"), ui.Hex("#8ce3ae"), ui.Hex("#f0d095"),
			ui.Hex("#b3d7ff"), ui.Hex("#c9bcff"), ui.Hex("#9be0ff"), ui.Hex("#eef0f6"),
		},
	}
}
