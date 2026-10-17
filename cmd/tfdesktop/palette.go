package main

import (
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

// The window's palette is the same one the browser client is drawn with, so a
// shell on this machine and a shell in a tab look like the same product.
//
// The source of that shared design lives in the browser client's stylesheet
// (web/src/style.css, "Command Deck"), where it is a set of CSS custom
// properties; this file is the same numbers on the ui's terms. The reasoning
// there applies here too, and is worth restating in one place:
//
//   - The ground is cold and near-black rather than blue-black, and hierarchy
//     comes from layered surfaces and hairlines rather than from borders.
//   - One accent — a teal — is the only saturated colour in the chrome, so it
//     always means "the thing that is chosen or in control".
//   - State has its own colours and they mean the same thing everywhere: teal
//     for live and controlled, amber for read-only, red for gone, grey for
//     exited.
//
// The numbers below are the CSS tokens, one for one. When the browser client's
// tokens change, these change with them.

// chromeMetrics are the layout numbers the two clients share. They are the CSS
// custom properties of the same names in web/src/style.css.
const (
	// headerHeight is the band naming the shell on screen. 44px is what the
	// browser client uses; 52px was this window's own, and a taller band bought
	// nothing but distance from the shell.
	headerHeight = 44
	// statusHeight is the quiet line along the bottom.
	statusHeight = 30

	// radius is the corner every panel shares. The CSS scale is 6/9/13px for
	// small/medium/large; the ui has one Radius, so the medium is used.
	radius = 9
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
		Background:     ui.Hex("#07090c"),
		Surface:        ui.Hex("#0c1016"),
		SurfaceHover:   ui.Hex("#121821"),
		SurfacePressed: ui.Hex("#0b0f15"),
		Border:         ui.Hex("#1a222e"),
		Text:           ui.Hex("#d9dfe8"),
		TextMuted:      ui.Hex("#8b96a5"),
		Accent:         ui.Hex("#2fd6a3"),
		AccentHover:    ui.Hex("#27bd90"),
		AccentPressed:  ui.Hex("#27bd90"),
		AccentText:     ui.Hex("#04241a"),
		Danger:         ui.Hex("#ef6b6b"),
		Warning:        ui.Hex("#e2b258"),
		Success:        ui.Hex("#2fd6a3"),
		Selection:      ui.RGBA(0x2f, 0xd6, 0xa3, 0.28),
		Focus:          ui.Hex("#2fd6a3"),
		Inverse:        ui.Hex("#d9dfe8"),
		InverseText:    ui.Hex("#07090c"),
		Scrollbar:      ui.Hex("#1a222e"),
		ScrollbarWidth: 8,
		Radius:         radius,
		Spacing:        spacing,
		FontSize:       fontSize,
	}
}

// lightTheme is the shell in the set light appearance: the browser client's
// [data-theme="light"] block, token for token.
func lightTheme() *ui.Theme {
	return &ui.Theme{
		Background:     ui.Hex("#f2f4f6"),
		Surface:        ui.Hex("#fbfcfd"),
		SurfaceHover:   ui.Hex("#e6ebef"),
		SurfacePressed: ui.Hex("#eaeef1"),
		Border:         ui.Hex("#d3dae0"),
		Text:           ui.Hex("#1b232e"),
		TextMuted:      ui.Hex("#5a6570"),
		Accent:         ui.Hex("#0d9e77"),
		AccentHover:    ui.Hex("#0b8a68"),
		AccentPressed:  ui.Hex("#0b8a68"),
		AccentText:     ui.Hex("#ffffff"),
		Danger:         ui.Hex("#cc4347"),
		Warning:        ui.Hex("#a5761b"),
		Success:        ui.Hex("#0d9e77"),
		Selection:      ui.RGBA(0x0d, 0x9e, 0x77, 0.22),
		Focus:          ui.Hex("#0d9e77"),
		Inverse:        ui.Hex("#1b232e"),
		InverseText:    ui.Hex("#f2f4f6"),
		Scrollbar:      ui.Hex("#d3dae0"),
		ScrollbarWidth: 8,
		Radius:         radius,
		Spacing:        spacing,
		FontSize:       fontSize,
	}
}

// shellColors are the shades the shell draws that the ui's theme has no name
// for: the ground of the icon strip, the face of a pane, the hairlines between
// them, and the tint behind a chosen row. They are the CSS tokens --bg-sunken,
// --bg-raised, --line and --line-soft, which have no ui.Theme field.
type shellColors struct {
	rail     ui.Color // the icon strip, one step off the window ground
	pane     ui.Color // the sidebar and the header, one step up
	sunken   ui.Color // the recessed ground the terminal sits in
	hairline ui.Color // the line between panes, quieter than a border
	chosen   ui.Color // the tint behind a chosen row
	icon     ui.Color // the strip's drawings, a shade off the text
	quiet    ui.Color // a control that is not on yet
}

func shellColorsFor(t *ui.Theme) shellColors {
	if t.Dark {
		return shellColors{
			rail:     ui.Hex("#05070a"),
			pane:     ui.Hex("#0c1016"),
			sunken:   ui.Hex("#05070a"),
			hairline: ui.Hex("#131a24"),
			chosen:   ui.RGBA(0x2f, 0xd6, 0xa3, 0.14),
			icon:     ui.Hex("#8b96a5"),
			quiet:    ui.Hex("#5c6875"),
		}
	}
	return shellColors{
		rail:     ui.Hex("#eaeef1"),
		pane:     ui.Hex("#fbfcfd"),
		sunken:   ui.Hex("#eaeef1"),
		hairline: ui.Hex("#dfe5ea"),
		chosen:   ui.RGBA(0x0d, 0x9e, 0x77, 0.10),
		icon:     ui.Hex("#5a6570"),
		quiet:    ui.Hex("#8a94a0"),
	}
}

// The shell's palette on the terminal's own terms, so the pane a shell runs in
// belongs to the window around it instead of looking like a program that
// wandered in. The sixteen program colours are the browser client's terminal
// theme in web/src/term.ts, which is the same set both clients render ANSI
// with — a violet would clash with the teal chrome, so the terminal's accent is
// the chrome's.
func terminalTheme(dark bool) *terminal.Theme {
	if dark {
		return &terminal.Theme{
			Foreground: ui.Hex("#d3dae3"),
			Background: ui.Hex("#0a0d12"),
			Cursor:     ui.Hex("#3adfae"),
			CursorText: ui.Hex("#0a0d12"),
			Selection:  ui.RGBA(0x3a, 0xdf, 0xae, 0.28),
			Palette: [16]ui.Color{
				ui.Hex("#1b2430"), ui.Hex("#f0777a"), ui.Hex("#3adfae"), ui.Hex("#e6bb66"),
				ui.Hex("#7aa7c9"), ui.Hex("#c39ad6"), ui.Hex("#6fc7d8"), ui.Hex("#d3dae3"),
				ui.Hex("#4b5766"), ui.Hex("#ff9a9c"), ui.Hex("#5cebbb"), ui.Hex("#f2cd85"),
				ui.Hex("#93bede"), ui.Hex("#d5b2e6"), ui.Hex("#8bd9e7"), ui.Hex("#eef2f7"),
			},
		}
	}
	return &terminal.Theme{
		Foreground: ui.Hex("#d3dae3"),
		Background: ui.Hex("#0a0d12"),
		Cursor:     ui.Hex("#3adfae"),
		CursorText: ui.Hex("#0a0d12"),
		Selection:  ui.RGBA(0x3a, 0xdf, 0xae, 0.28),
		Palette: [16]ui.Color{
			ui.Hex("#1b2430"), ui.Hex("#f0777a"), ui.Hex("#3adfae"), ui.Hex("#e6bb66"),
			ui.Hex("#7aa7c9"), ui.Hex("#c39ad6"), ui.Hex("#6fc7d8"), ui.Hex("#d3dae3"),
			ui.Hex("#4b5766"), ui.Hex("#ff9a9c"), ui.Hex("#5cebbb"), ui.Hex("#f2cd85"),
			ui.Hex("#93bede"), ui.Hex("#d5b2e6"), ui.Hex("#8bd9e7"), ui.Hex("#eef2f7"),
		},
	}
}
