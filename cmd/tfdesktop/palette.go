package main

import (
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

const (
	headerHeight = 48
	statusHeight = 28
	radius       = 5
	spacing      = 4
	fontSize     = 13
)

// Desktop chrome uses neutral values regardless of the saved legacy appearance.
func darkTheme() *ui.Theme {
	return &ui.Theme{
		Dark: true, Background: ui.Hex("#111111"), Surface: ui.Hex("#171717"),
		SurfaceHover: ui.Hex("#222222"), SurfacePressed: ui.Hex("#2b2b2b"),
		Border: ui.Hex("#292929"), Text: ui.Hex("#d8d8d8"), TextMuted: ui.Hex("#929292"),
		Accent: ui.Hex("#8b9b83"), AccentHover: ui.Hex("#a1af99"), AccentPressed: ui.Hex("#76886e"), AccentText: ui.Hex("#10110f"),
		Danger: ui.Hex("#cf7773"), Warning: ui.Hex("#d3aa62"), Success: ui.Hex("#72b67b"),
		Selection: ui.Hex("#454545"), Focus: ui.Hex("#8b9b83"), Inverse: ui.Hex("#eeeeee"), InverseText: ui.Hex("#111111"),
		Scrollbar: ui.Hex("#3a3a3a"), ScrollbarWidth: 6, Radius: radius, Spacing: spacing, FontSize: fontSize,
	}
}

type shellColors struct{ rail, pane, sunken, hairline, chosen, icon, quiet ui.Color }

func shellColorsFor(_ *ui.Theme) shellColors {
	return shellColors{
		rail: ui.Hex("#171717"), pane: ui.Hex("#171717"), sunken: ui.Hex("#111111"),
		hairline: ui.Hex("#292929"), chosen: ui.Hex("#2b2b2b"), icon: ui.Hex("#929292"), quiet: ui.Hex("#777777"),
	}
}

func terminalTheme(_ bool) *terminal.Theme {
	return &terminal.Theme{
		Foreground: ui.Hex("#d8d8d8"), Background: ui.Hex("#111111"), Cursor: ui.Hex("#dedede"), CursorText: ui.Hex("#111111"), Selection: ui.Hex("#454545"),
		Palette: [16]ui.Color{
			ui.Hex("#202830"), ui.Hex("#e05b5b"), ui.Hex("#78b879"), ui.Hex("#d5ad68"),
			ui.Hex("#78a9d4"), ui.Hex("#c58fc2"), ui.Hex("#70b8bf"), ui.Hex("#cbd3dc"),
			ui.Hex("#46515d"), ui.Hex("#f27b73"), ui.Hex("#8ccd80"), ui.Hex("#ebc277"),
			ui.Hex("#92c2ed"), ui.Hex("#d8a7d5"), ui.Hex("#8ad4d5"), ui.Hex("#f0f2f4"),
		},
	}
}
