package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/beacon/internal/settings"
	"github.com/CarlOpenLab/beacon/internal/uitokens"
	"github.com/CarlOpenLab/beacon/locales"
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
	{themeSystem, locales.ThemeSystem()},
	{themeLight, locales.ThemeLight()},
	{themeDark, locales.ThemeDark()},
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

// blurChoices are the frosted blur intensity options.
var blurChoices = []struct {
	id    string
	label func() string
}{
	{"none", locales.BlurNone},
	{"subtle", locales.BlurSubtle},
	{"medium", locales.BlurMedium},
	{"deep", locales.BlurDeep},
}

// blurIndex returns the index of the blur setting in blurChoices.
func blurIndex(id string) int {
	for i, choice := range blurChoices {
		if choice.id == id {
			return i
		}
	}
	return 1 // default to subtle
}

// themeFor is the palette an explicit mode asks for, or nil to follow the
// system. It is the appearance the window's material follows, before any frame
// runs.
func themeFor(mode string) *ui.Theme {
	switch normalizeTheme(mode) {
	case themeLight:
		return lightTheme()
	case themeDark:
		return darkTheme()
	}
	return nil
}

// applyAppearance tells the app which appearance the person asked for, so that
// the window's material, its menus and the frame agree. The empty mode follows
// the system.
func applyAppearance(mode string) {
	switch normalizeTheme(mode) {
	case themeLight:
		mygo.Theme.SetSource(mygo.ThemeLight)
	case themeDark:
		mygo.Theme.SetSource(mygo.ThemeDark)
	default:
		mygo.Theme.SetSource(mygo.ThemeSystem)
	}
}

// paletteFor returns the colors of the appearance in force in a frame. The
// frame's theme is what applyTheme left there, so this is the one place the two
// agree.
func paletteFor(c *ui.Context) *uitokens.Palette {
	return uitokens.For(c.Theme().Dark)
}

// paletteOf returns the colors of an appearance, for the code that has a theme
// but no frame (a settings window's first frame, a test).
func paletteOf(t *ui.Theme) *uitokens.Palette {
	return uitokens.For(t.Dark)
}

// applyTheme makes the frame's chrome the app's: the accent, the surfaces and the
// sizes of its controls. The mode is the stored appearance; the empty mode is
// whatever the system says, which is what c.Theme() already holds.
func applyTheme(c *ui.Context, mode string) {
	t := c.Theme()
	switch normalizeTheme(mode) {
	case themeLight:
		c.SetTheme(chromeTheme(t, false))
	case themeDark:
		c.SetTheme(chromeTheme(t, true))
	default:
		c.SetTheme(chromeTheme(t, t.Dark))
	}
}

// backdrop paints the window's wash: the base on the root, and the glow from
// the top right on the row that holds everything else. Over the window's
// material, which shows through when the window is translucent; the plain
// wash is solid, while the frosted wash (or a reduced opacity) lets the vibrancy
// material blur through.
func backdrop(c *ui.Context, pal *uitokens.Palette, row ui.Element, washID string, opacity float32) {
	w := uitokens.WashFor(washID)
	if w.ID == "frosted" || opacity < 1.0 {
		if c.Vibrancy() {
			c.Root().Background(ui.Transparent)
			return
		}
		bg := pal.Sidebar
		if opacity < 1.0 {
			bg = bg.Alpha(opacity)
		}
		c.Root().Background(bg)
		return
	}
	if w.ID == "plain" {
		c.Root().Background(pal.Sidebar)
		return
	}
	alpha := float32(1)
	if c.Vibrancy() {
		alpha = min(opacity, 0.95)
	}
	base := w.Base(pal.Dark)
	for i := range base.Stops {
		base.Stops[i].Color = base.Stops[i].Color.Alpha(alpha)
	}
	c.Root().LinearGradient(base)
	row.LinearGradient(w.Glow(pal.Dark))
}

// shellColors are the shades the shell draws that the ui's theme has no name
// for. It is the palette above in the names the shell's older code uses, so
// that a surface-by-surface restyle need not rename everything at once.
type shellColors struct {
	pane     ui.Color // a card
	side     ui.Color // the sidebar
	field    ui.Color // chips, keys and pills
	tabOn    ui.Color // the chosen row
	line     ui.Color // a border that has to be seen
	hairline ui.Color // the line between panes
	chosen   ui.Color // the wash behind a chosen row
	icon     ui.Color // drawings and secondary text
	quiet    ui.Color // meta and placeholders
}

func shellColorsFor(t *ui.Theme) shellColors {
	pal := paletteOf(t)
	return shellColors{
		pane:     pal.Card,
		side:     pal.Sidebar,
		field:    pal.Chip,
		tabOn:    pal.RowSelected,
		line:     pal.CardBorder,
		hairline: pal.Divider,
		chosen:   pal.RowSelected,
		icon:     pal.TextMuted,
		quiet:    pal.TextFaint,
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
		return locales.ThemeToDark()
	}
	return locales.ThemeToLight()
}

// setTheme saves an appearance, tells the app which one it is, and repaints
// every window the shell has open, so a switch made in the settings window is
// visible in the terminal at once.
func (w *window) setTheme(mode string) {
	if w == nil {
		return
	}
	mode = normalizeTheme(mode)
	if w.store != nil {
		if _, err := w.store.Update(func(s *settings.Settings) { s.Theme = mode }); err != nil {
			log.Printf("tfdesktop: save appearance: %v", err)
		}
	}
	applyAppearance(mode)
	if win := w.win; win != nil {
		win.Invalidate()
	}
	w.invalidateSettings()
}

// vibrancyFor maps a blur setting ("none", "subtle", "medium", "deep") to a MyGo Vibrancy material.
func vibrancyFor(blur string) mygo.Vibrancy {
	switch blur {
	case "none":
		return mygo.VibrancyNone
	case "subtle":
		return mygo.VibrancySidebar
	case "medium":
		return mygo.VibrancyPopover
	case "deep":
		return mygo.VibrancyAcrylic
	}
	return mygo.VibrancySidebar
}

// setBackgroundOpacity updates the background opacity preference and repaints.
func (w *window) setBackgroundOpacity(v float64) {
	if w == nil {
		return
	}
	v = min(max(v, 0.30), 1.0)
	if w.store != nil {
		if _, err := w.store.Update(func(s *settings.Settings) { s.BackgroundOpacity = v }); err != nil {
			log.Printf("tfdesktop: save background opacity: %v", err)
		}
	}
	if win := w.win; win != nil {
		win.Invalidate()
	}
	w.invalidateSettings()
}

// setBackgroundBlur updates the blur material preference, updates the window's
// vibrancy immediately, and repaints.
func (w *window) setBackgroundBlur(b string) {
	if w == nil {
		return
	}
	if w.store != nil {
		if _, err := w.store.Update(func(s *settings.Settings) { s.BackgroundBlur = b }); err != nil {
			log.Printf("tfdesktop: save background blur: %v", err)
		}
	}
	if win := w.win; win != nil {
		win.SetVibrancy(vibrancyFor(b))
		win.Invalidate()
	}
	w.invalidateSettings()
}

// backgroundOpacity returns the active background opacity.
func (w *window) backgroundOpacity() float64 {
	if w == nil || w.store == nil {
		return 1.0
	}
	return w.store.Get().Opacity()
}

// backgroundBlur returns the active background blur.
func (w *window) backgroundBlur() string {
	if w == nil || w.store == nil {
		return "subtle"
	}
	return w.store.Get().Blur()
}

// windowGround is the color the window is painted before its first frame: the
// wash's first stop, so that the frame that follows does not flash a color the
// wash never shows.
func windowGround(mode string, dark bool) string {
	switch normalizeTheme(mode) {
	case themeLight:
		dark = false
	case themeDark:
		dark = true
	}
	w := uitokens.WashFor("plain")
	stops := w.Light
	if dark {
		stops = w.Dark
	}
	return stops[0]
}
