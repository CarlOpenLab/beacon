package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/settings"
)

// The appearance preference is stored as one of these ids. The empty id is the
// system's own choice, which is also what a first run gets.
const (
	themeSystem = ""
	themeLight  = "light"
	themeDark   = "dark"
)

// themeChoices is the appearance menu in the order the settings window offers
// it. The rail's entry, the settings control and the stored value all read this
// list, so the three cannot drift apart.
var themeChoices = []struct{ id, label string }{
	{themeSystem, "跟随系统"},
	{themeLight, "白天"},
	{themeDark, "暗黑"},
}

// normalizeTheme maps anything unrecognised onto the system default: a stale or
// hand-edited settings file must not leave the shell in a mode it cannot show.
func normalizeTheme(mode string) string {
	for _, choice := range themeChoices {
		if choice.id == mode {
			return mode
		}
	}
	return themeSystem
}

// themeIndex is where mode sits in themeChoices, which is what the settings
// window's segmented control works in.
func themeIndex(mode string) int {
	mode = normalizeTheme(mode)
	for i, choice := range themeChoices {
		if choice.id == mode {
			return i
		}
	}
	return 0
}

// themeName is the mode's own label, for the settings window to say out loud
// which appearance is on screen.
func themeName(mode string) string {
	return themeChoices[themeIndex(mode)].label
}

// themeFor is the palette an explicit mode asks for. The system default is nil
// on purpose: the ui follows the system unless a frame sets a theme, so
// following the system is the absence of a choice, not a third palette.
func themeFor(mode string) *ui.Theme {
	switch normalizeTheme(mode) {
	case themeLight:
		return lightTheme()
	case themeDark:
		return darkTheme()
	}
	return nil
}

// applyTheme puts the saved appearance on the frame. Every window asks for it
// on every frame: the ui reads the theme per frame, so this is a decision and
// not a one-off setting.
func applyTheme(c *ui.Context, mode string) {
	if theme := themeFor(mode); theme != nil {
		c.SetTheme(theme)
	}
}

// flipTheme is what the rail's appearance entry does: it names the mode a click
// switches to. From the system default it picks the opposite of what is on
// screen now, so the first click always changes something the user can see.
func flipTheme(mode string, dark bool) string {
	switch normalizeTheme(mode) {
	case themeLight:
		return themeDark
	case themeDark:
		return themeLight
	}
	if dark {
		return themeLight
	}
	return themeDark
}

// themeActionLabel names what a click on the rail's appearance entry will do.
func themeActionLabel(mode string, dark bool) string {
	if flipTheme(mode, dark) == themeDark {
		return "切换到暗黑"
	}
	return "切换到白天"
}

// setTheme saves an appearance and repaints every window the shell has open, so
// a switch made in either place — the rail or the settings window — is visible
// in the terminal at once.
func (w *window) setTheme(mode string) {
	mode = normalizeTheme(mode)
	if _, err := w.store.Update(func(s *settings.Settings) { s.Theme = mode }); err != nil {
		log.Printf("tfdesktop: save appearance: %v", err)
	}
	if win := w.win; win != nil {
		win.Invalidate()
	}
	w.invalidateSettings()
}

// windowGround is the colour a window is filled with until its first frame
// paints. It has to agree with the appearance that frame will ask for: a dark
// flash in front of a light window is the one piece of the dark theme a
// light-mode user sees. The system's own answer is read from the app, which is
// running by the time any window is opened.
func windowGround(mode string) string {
	switch normalizeTheme(mode) {
	case themeLight:
		return "#f6f7fb"
	case themeDark:
		return "#0b0d15"
	}
	if mygo.Theme.IsDark() {
		return "#0b0d15"
	}
	return "#f6f7fb"
}
