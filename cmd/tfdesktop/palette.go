package main

import (
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

// The window has a palette of its own rather than the ui's default greys: a deep
// navy ground, a violet for the one thing that is chosen, and a green for
// something that is live. It follows the app's mark, so the window and the icon
// beside it in the dock agree.

// darkTheme is the shell in the set dark appearance.
func darkTheme() *ui.Theme {
	return &ui.Theme{
		Dark:           true,
		Background:     ui.Hex("#0b0d15"),
		Surface:        ui.Hex("#131926"),
		SurfaceHover:   ui.Hex("#1c2333"),
		SurfacePressed: ui.Hex("#2b2547"),
		Border:         ui.Hex("#242c3d"),
		Text:           ui.Hex("#eceef6"),
		TextMuted:      ui.Hex("#8a93a8"),
		Accent:         ui.Hex("#8d28e2"),
		AccentHover:    ui.Hex("#a04af0"),
		AccentPressed:  ui.Hex("#7220bd"),
		AccentText:     ui.Hex("#ffffff"),
		Danger:         ui.Hex("#f2555f"),
		Warning:        ui.Hex("#f0a83c"),
		Success:        ui.Hex("#3ed879"),
		Selection:      ui.RGBA(0x8d, 0x28, 0xe2, 0.35),
		Focus:          ui.Hex("#a04af0"),
		Inverse:        ui.Hex("#eceef6"),
		InverseText:    ui.Hex("#0b0d15"),
		Scrollbar:      ui.Hex("#2b3448"),
		ScrollbarWidth: 8,
		Radius:         10,
		Spacing:        4,
		FontSize:       14,
	}
}

// lightTheme is the shell in the set light appearance: the same accent on paper.
func lightTheme() *ui.Theme {
	return &ui.Theme{
		Background:     ui.Hex("#f6f7fb"),
		Surface:        ui.Hex("#ffffff"),
		SurfaceHover:   ui.Hex("#eef1f9"),
		SurfacePressed: ui.Hex("#e7e0f8"),
		Border:         ui.Hex("#dbe1ee"),
		Text:           ui.Hex("#131824"),
		TextMuted:      ui.Hex("#5f6a82"),
		Accent:         ui.Hex("#7b22cf"),
		AccentHover:    ui.Hex("#8d28e2"),
		AccentPressed:  ui.Hex("#66189f"),
		AccentText:     ui.Hex("#ffffff"),
		Danger:         ui.Hex("#d83843"),
		Warning:        ui.Hex("#b06f0a"),
		Success:        ui.Hex("#12a05a"),
		Selection:      ui.RGBA(0x7b, 0x22, 0xcf, 0.20),
		Focus:          ui.Hex("#7b22cf"),
		Inverse:        ui.Hex("#1b2130"),
		InverseText:    ui.Hex("#f6f7fb"),
		Scrollbar:      ui.Hex("#c4ccdd"),
		ScrollbarWidth: 8,
		Radius:         10,
		Spacing:        4,
		FontSize:       14,
	}
}

// shellColors are the few shades the shell draws that the ui's theme has no name
// for: the ground of the icon strip, the face of a pane, and the hairlines
// between them. They are derived from the theme's own lightness, so the shell
// still works when the appearance follows the system and the ui lends its theme.
type shellColors struct {
	rail     ui.Color // the icon strip, one step off the window ground
	pane     ui.Color // the sidebar and the header, one step up
	hairline ui.Color // the line between panes, quieter than a border
	chosen   ui.Color // the tint behind the rail's chosen item
	icon     ui.Color // the strip's drawings, a shade off the text
	quiet    ui.Color // a control that is not on yet
}

func shellColorsFor(t *ui.Theme) shellColors {
	if t.Dark {
		return shellColors{
			rail:     ui.Hex("#0e121c"),
			pane:     ui.Hex("#111725"),
			hairline: ui.Hex("#1f2736"),
			chosen:   ui.RGBA(0x8d, 0x28, 0xe2, 0.20),
			icon:     ui.Hex("#aab3c7"),
			quiet:    ui.Hex("#3a4356"),
		}
	}
	return shellColors{
		rail:     ui.Hex("#ffffff"),
		pane:     ui.Hex("#fbfcfe"),
		hairline: ui.Hex("#e6eaf3"),
		chosen:   ui.RGBA(0x7b, 0x22, 0xcf, 0.12),
		icon:     ui.Hex("#5f6a82"),
		quiet:    ui.Hex("#b8c0d0"),
	}
}

// The shell's palette on the terminal's own terms: the same navy ground and the
// same violet, so the pane a shell runs in belongs to the window around it
// instead of looking like a program that wandered in. The sixteen program
// colours are the ones the brand can live with — a violet that matches the
// accent, a green that matches the live dot.
func terminalTheme(dark bool) *terminal.Theme {
	if dark {
		return &terminal.Theme{
			Foreground: ui.Hex("#d9dded"),
			Background: ui.Hex("#0b0d15"),
			Cursor:     ui.Hex("#a04af0"),
			CursorText: ui.Hex("#0b0d15"),
			Selection:  ui.RGBA(0x8d, 0x28, 0xe2, 0.30),
			Palette: [16]ui.Color{
				ui.Hex("#20263a"), ui.Hex("#f2555f"), ui.Hex("#3ed879"), ui.Hex("#f0a83c"),
				ui.Hex("#7aa2f7"), ui.Hex("#a04af0"), ui.Hex("#4fd2d2"), ui.Hex("#c8cede"),
				ui.Hex("#3d4562"), ui.Hex("#ff7a84"), ui.Hex("#6ef0a0"), ui.Hex("#ffc46b"),
				ui.Hex("#9db8ff"), ui.Hex("#c07ef7"), ui.Hex("#7ee6e6"), ui.Hex("#ffffff"),
			},
		}
	}
	return &terminal.Theme{
		Foreground: ui.Hex("#1b2130"),
		Background: ui.Hex("#ffffff"),
		Cursor:     ui.Hex("#7b22cf"),
		Selection:  ui.RGBA(0x7b, 0x22, 0xcf, 0.20),
		Palette: [16]ui.Color{
			ui.Hex("#2a3244"), ui.Hex("#c62b37"), ui.Hex("#0f8f4e"), ui.Hex("#a86a06"),
			ui.Hex("#2f5fd0"), ui.Hex("#7b22cf"), ui.Hex("#0b7f86"), ui.Hex("#8a93a8"),
			ui.Hex("#59617a"), ui.Hex("#e04a56"), ui.Hex("#17a862"), ui.Hex("#c98a10"),
			ui.Hex("#4779e8"), ui.Hex("#9333e0"), ui.Hex("#0f9ba6"), ui.Hex("#131824"),
		},
	}
}
