package main

import (
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

// Neutral desktop chrome; status and terminal output keep their own colors.

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
		Background:     ui.Hex("#111111"),
		Surface:        ui.Hex("#191919"),
		SurfaceHover:   ui.Hex("#222222"),
		SurfacePressed: ui.Hex("#2b2b2b"),
		Border:         ui.Hex("#303030"),
		Text:           ui.Hex("#dedede"),
		TextMuted:      ui.Hex("#a0a0a0"),
		Accent:         ui.Hex("#d0d0d0"),
		AccentHover:    ui.Hex("#e0e0e0"),
		AccentPressed:  ui.Hex("#e0e0e0"),
		AccentText:     ui.Hex("#111111"),
		Danger:         ui.Hex("#cf7773"),
		Warning:        ui.Hex("#d3aa62"),
		Success:        ui.Hex("#72b67b"),
		Selection:      ui.RGBA(153, 153, 153, 0.32),
		Focus:          ui.Hex("#d0d0d0"),
		Inverse:        ui.Hex("#dedede"),
		InverseText:    ui.Hex("#111111"),
		Scrollbar:      ui.Hex("#303030"),
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
		Background:     ui.Hex("#f0f0f0"),
		Surface:        ui.Hex("#ffffff"),
		SurfaceHover:   ui.Hex("#e6e6e6"),
		SurfacePressed: ui.Hex("#dddddd"),
		Border:         ui.Hex("#d0d0d0"),
		Text:           ui.Hex("#222222"),
		TextMuted:      ui.Hex("#666666"),
		Accent:         ui.Hex("#606060"),
		AccentHover:    ui.Hex("#4a4a4a"),
		AccentPressed:  ui.Hex("#4a4a4a"),
		AccentText:     ui.Hex("#ffffff"),
		Danger:         ui.Hex("#ad4545"),
		Warning:        ui.Hex("#8d661f"),
		Success:        ui.Hex("#35744a"),
		Selection:      ui.RGBA(102, 102, 102, 0.22),
		Focus:          ui.Hex("#606060"),
		Inverse:        ui.Hex("#222222"),
		InverseText:    ui.Hex("#f0f0f0"),
		Scrollbar:      ui.Hex("#d0d0d0"),
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
			pane:     ui.Hex("#191919"),
			side:     ui.Hex("#141414"),
			field:    ui.Hex("#222222"),
			tabOn:    ui.Hex("#2b2b2b"),
			line:     ui.Hex("#303030"),
			hairline: ui.Hex("#292929"),
			chosen:   ui.Hex("#292929"),
			icon:     ui.Hex("#a0a0a0"),
			quiet:    ui.Hex("#777777"),
		}
	}
	return shellColors{
		pane:     ui.Hex("#ffffff"),
		side:     ui.Hex("#e6e6e6"),
		field:    ui.Hex("#ececf0"),
		tabOn:    ui.Hex("#ffffff"),
		line:     ui.Hex("#d0d0d0"),
		hairline: ui.Hex("#dddddd"),
		chosen:   ui.Hex("#e5e5e5"),
		icon:     ui.Hex("#666666"),
		quiet:    ui.Hex("#75757f"),
	}
}

// Preserve distinct ANSI colors and TrueColor output on a neutral background.
func terminalTheme(_ bool) *terminal.Theme {
	return &terminal.Theme{
		Foreground: ui.Hex("#d7dae2"),
		Background: ui.Hex("#111111"),
		Cursor:     ui.Hex("#dedede"),
		CursorText: ui.Hex("#111111"),
		Selection:  ui.RGBA(153, 153, 153, 0.28),
		Palette: [16]ui.Color{
			ui.Hex("#26262e"), ui.Hex("#ff8b93"), ui.Hex("#6fd39a"), ui.Hex("#e6c07d"),
			ui.Hex("#9ecbff"), ui.Hex("#b6a6ff"), ui.Hex("#7dd3fc"), ui.Hex("#d7dae2"),
			ui.Hex("#4a4a56"), ui.Hex("#ffa3aa"), ui.Hex("#8ce3ae"), ui.Hex("#f0d095"),
			ui.Hex("#b3d7ff"), ui.Hex("#c9bcff"), ui.Hex("#9be0ff"), ui.Hex("#eef0f6"),
		},
	}
}
