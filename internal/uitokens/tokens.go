// Package uitokens is the one place term-fleet's colors, washes and the
// measurements of its chrome are written down.
//
// The desktop shell (cmd/tfdesktop) and the browser client (web/src) are meant
// to read as one product, and they used to hold their own copies of the same
// numbers: palette.go said it was "style.css token for token", and the two had
// drifted. They cannot drift now: the browser's tokens are generated from this
// package by `go run ./gen` and checked by TestCSS (see tokens_test.go).
//
// The palette is the desktop's: its light and dark sets, its five window washes,
// and its terminal themes, which are Apple's system colors tuned to read on the
// card they sit on.
package uitokens

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

// Palette is the chrome's colors, for one appearance. The field names are
// shared by name, so that a line of the source reads the same everywhere.
type Palette struct {
	Dark bool

	Sidebar     ui.Color // under the sidebar where no material shows
	Text        ui.Color
	TextMuted   ui.Color // group headers, meta
	TextFaint   ui.Color // counts, hints
	RowHover    ui.Color
	RowSelected ui.Color
	RowSelEdge  ui.Color
	RowShadow   ui.Color

	Card        ui.Color // a session's card
	CardBorder  ui.Color
	CardShadow  ui.Color
	CardShadow2 ui.Color
	FloatShadow ui.Color
	CardDim     ui.Color // a pane the user is not in
	Edge        ui.Color // hairlines around surfaces in the dark
	EdgeHigh    ui.Color // what floats or takes a click
	Veil        ui.Color // fades a pane the user is not in, its text too
	ButtonBg    ui.Color

	Control       ui.Color
	ControlHover  ui.Color
	ControlShadow bool
	ControlLight  ui.Color
	Divider       ui.Color
	Chip          ui.Color

	Attention ui.Color // needs you
	AttnSoft  ui.Color
	Done      ui.Color // the filled check
	OnDone    ui.Color
	Danger    ui.Color
	Success   ui.Color // a live link, a machine answering
	Accent    ui.Color

	Overlay   ui.Color
	Popover   ui.Color
	Kbd       ui.Color
	KbdBorder ui.Color
}

// Light is the light appearance.
var Light = Palette{
	Sidebar:     ui.Hex("#eceef4"),
	Text:        ui.Hex("#1d1d1f"),
	TextMuted:   ui.Hex("#6e6e78"),
	TextFaint:   ui.Hex("#a3a3ad"),
	RowHover:    ui.RGBA(0, 0, 0, 0.04),
	RowSelected: ui.Hex("#ffffff"),
	RowSelEdge:  ui.RGBA(0, 0, 0, 0.05),
	RowShadow:   ui.RGBA(20, 20, 50, 0.08),

	Card:         ui.Hex("#ffffff"),
	CardBorder:   ui.RGBA(0, 0, 0, 0.07),
	CardShadow:   ui.RGBA(0, 0, 0, 0.07),
	CardShadow2:  ui.RGBA(0, 0, 0, 0.035),
	FloatShadow:  ui.RGBA(20, 20, 40, 0.14),
	CardDim:      ui.RGBA(255, 255, 255, 0.58),
	Veil:         ui.RGBA(250, 248, 252, 0.28),
	ButtonBg:     ui.Hex("#ffffff"),
	Control:      ui.Hex("#ffffff"),
	ControlHover: ui.Hex("#f7f7f9"),
	Divider:      ui.RGBA(0, 0, 0, 0.06),
	Chip:         ui.RGBA(0, 0, 0, 0.045),

	Attention: ui.Hex("#c27b0f"),
	AttnSoft:  ui.RGBA(232, 154, 40, 0.13),
	Done:      ui.Hex("#1d1d1f"),
	OnDone:    ui.Hex("#ffffff"),
	Danger:    ui.Hex("#d93a35"),
	Success:   ui.Hex("#22863a"),
	Accent:    ui.Hex("#3b6cf6"),

	Overlay:   ui.RGBA(10, 10, 30, 0.12),
	Popover:   ui.Hex("#ffffff"),
	Kbd:       ui.RGBA(0, 0, 0, 0.04),
	KbdBorder: ui.RGBA(0, 0, 0, 0.09),
}

// Dark is the dark appearance.
var Dark = Palette{
	Dark:        true,
	Sidebar:     ui.Hex("#141418"),
	Text:        ui.Hex("#ececf0"),
	TextMuted:   ui.Hex("#a9a9b4"),
	TextFaint:   ui.Hex("#83838f"),
	RowHover:    ui.RGBA(255, 255, 255, 0.05),
	RowSelected: ui.RGBA(255, 255, 255, 0.10),
	RowSelEdge:  ui.RGBA(255, 255, 255, 0.06),
	RowShadow:   ui.RGBA(0, 0, 0, 0.25),

	Card:         ui.Hex("#232328"),
	CardBorder:   ui.RGBA(255, 255, 255, 0.08),
	CardShadow:   ui.RGBA(0, 0, 0, 0.35),
	CardShadow2:  ui.RGBA(0, 0, 0, 0.2),
	FloatShadow:  ui.RGBA(0, 0, 0, 0.45),
	CardDim:      ui.RGBA(35, 35, 40, 0.5),
	Edge:         ui.RGBA(255, 255, 255, 0.075),
	EdgeHigh:     ui.RGBA(255, 255, 255, 0.11),
	Veil:         ui.RGBA(14, 13, 18, 0.42),
	ButtonBg:     ui.Hex("#404047"),
	Control:      ui.Hex("#404047"),
	ControlHover: ui.Hex("#4a4a52"),
	ControlLight: ui.RGBA(255, 255, 255, 0.09),
	Divider:      ui.RGBA(255, 255, 255, 0.07),
	Chip:         ui.RGBA(255, 255, 255, 0.07),

	Attention: ui.Hex("#f0a33a"),
	AttnSoft:  ui.RGBA(240, 163, 58, 0.15),
	Done:      ui.Hex("#ececf0"),
	OnDone:    ui.Hex("#161618"),
	Danger:    ui.Hex("#ff6961"),
	Success:   ui.Hex("#4fd17a"),
	Accent:    ui.Hex("#6b93ff"),

	Overlay:   ui.RGBA(0, 0, 0, 0.35),
	Popover:   ui.Hex("#232327"),
	Kbd:       ui.RGBA(255, 255, 255, 0.06),
	KbdBorder: ui.RGBA(255, 255, 255, 0.1),
}

// For returns the palette of an appearance.
func For(dark bool) *Palette {
	if dark {
		return &Dark
	}
	return &Light
}

// The measurements of the chrome, in DIPs. BarMid is the line the title bar's
// contents center on, and everything else is derived from it so that the
// traffic lights, the sidebar's buttons and a card's header line up.
const (
	BarMid  = 30
	TopBar  = 2 * BarMid
	CardGap = 8
	HeaderH = 2 * (BarMid - CardGap) // a card's header, 44
	RowH    = 34                     // a session's row
	GroupH  = 28                     // a group's header
	FooterH = 36                     // the sidebar's foot

	SidebarWidth = 268
	MinSidebar   = 200
	MaxSidebar   = 440

	CardRadius = 14
	RowRadius  = 8
	PaneGap    = CardGap
)

// Wash is one of the window's backgrounds: a vertical gradient in four stops,
// and a glow the card column adds from the top right.
type Wash struct {
	ID    string
	Light []string
	Dark  []string
	// LightGlow, DarkGlow is the glow's color; transparent for none.
	LightGlow ui.Color
	DarkGlow  ui.Color
}

// Washes are the backgrounds the appearance settings offer, in their order.
// rex is the default: pink lavender above, rose at the top right, butter
// cream below.
var Washes = []Wash{
	{"rex", []string{"#ebe0f3", "#f3e7f1", "#f7eee4", "#f6efcf"}, []string{"#17141f", "#191620", "#1b1916"}, ui.RGBA(247, 220, 241, 0.75), ui.RGBA(120, 70, 120, 0.22)},
	{"mist", []string{"#e3e9f4", "#ebeff6", "#f1f2f3", "#f2f1ec"}, []string{"#12151b", "#14171d", "#17181b"}, ui.RGBA(210, 225, 248, 0.75), ui.RGBA(70, 100, 150, 0.22)},
	{"meadow", []string{"#e2eee5", "#ebf2e7", "#f3f2e2", "#f5f0d6"}, []string{"#111814", "#141a16", "#181a14"}, ui.RGBA(214, 240, 222, 0.75), ui.RGBA(70, 130, 95, 0.2)},
	{"dusk", []string{"#e4dbf3", "#efdeee", "#f6e3dc", "#f7e6cf"}, []string{"#191321", "#1e161e", "#221a17"}, ui.RGBA(246, 206, 222, 0.8), ui.RGBA(150, 80, 110, 0.25)},
	{"plain", []string{"#eceef4", "#eceef4"}, []string{"#141418", "#141418"}, ui.Transparent, ui.Transparent},
}

// WashFor returns a wash by id, falling back to rex for an id the settings file
// no longer knows.
func WashFor(id string) Wash {
	for _, w := range Washes {
		if w.ID == id {
			return w
		}
	}
	return Washes[0]
}

// WashIDs are the ids the appearance settings store.
func WashIDs() []string {
	ids := make([]string, len(Washes))
	for i, w := range Washes {
		ids[i] = w.ID
	}
	return ids
}

// Base is the wash's gradient, the window's ground.
func (w Wash) Base(dark bool) ui.LinearGradient {
	stops := w.Light
	if dark {
		stops = w.Dark
	}
	out := make([]ui.GradientStop, len(stops))
	for i, h := range stops {
		out[i] = ui.GradientStop{Color: ui.Hex(h), At: float32(i) / float32(max(len(stops)-1, 1))}
	}
	return ui.LinearGradient{Angle: 180, Oklab: true, Stops: out}
}

// Glow is the wash's glow, from the top right, over the cards' column.
func (w Wash) Glow(dark bool) ui.LinearGradient {
	c := w.LightGlow
	if dark {
		c = w.DarkGlow
	}
	return ui.LinearGradient{Angle: 225, Oklab: true, Stops: []ui.GradientStop{
		{Color: c, At: 0},
		{Color: c.Alpha(0), At: 0.5},
	}}
}

// TermLight and TermDark are the terminal's colors, on the card's color: Apple's
// system colors, a little deeper in the light.
var (
	TermLight = &terminal.Theme{
		Foreground:    ui.Hex("#1d1d1f"),
		Background:    Light.Card,
		Cursor:        ui.Hex("#1d1d1f"),
		CursorText:    ui.Hex("#ffffff"),
		Selection:     ui.Hex("#cfdcfb"),
		SelectionText: ui.Hex("#1d1d1f"),
		Palette: [16]ui.Color{
			ui.Hex("#1d1d1f"), ui.Hex("#d3302f"), ui.Hex("#22863a"), ui.Hex("#b08800"),
			ui.Hex("#2f5bd8"), ui.Hex("#a033c4"), ui.Hex("#0f8199"), ui.Hex("#8e8e93"),
			ui.Hex("#5f5f66"), ui.Hex("#ef4444"), ui.Hex("#2fa84f"), ui.Hex("#d4a017"),
			ui.Hex("#3b82f6"), ui.Hex("#c04ee0"), ui.Hex("#14a3bf"), ui.Hex("#b4b4bb"),
		},
	}

	TermDark = &terminal.Theme{
		Foreground:    ui.Hex("#e6e6ea"),
		Background:    Dark.Card,
		Cursor:        ui.Hex("#e6e6ea"),
		CursorText:    ui.Hex("#161618"),
		Selection:     ui.Hex("#2f4170"),
		SelectionText: ui.Hex("#ffffff"),
		Palette: [16]ui.Color{
			ui.Hex("#3a3a40"), ui.Hex("#ff6b64"), ui.Hex("#4fd17a"), ui.Hex("#f2c14e"),
			ui.Hex("#6b9bff"), ui.Hex("#d07cf0"), ui.Hex("#4cc9e0"), ui.Hex("#c9c9cf"),
			ui.Hex("#6c6c78"), ui.Hex("#ff8a84"), ui.Hex("#74e397"), ui.Hex("#ffd479"),
			ui.Hex("#8fb3ff"), ui.Hex("#e09cf7"), ui.Hex("#7fdcec"), ui.Hex("#f5f5f7"),
		},
	}
)

// TermTheme returns the terminal's colors for an appearance.
func TermTheme(dark bool) *terminal.Theme {
	if dark {
		return TermDark
	}
	return TermLight
}

// CSS is the browser client's token sheet, generated from the values above.
// web/src/tokens.css is this text; tokens_test.go fails when the two differ.
func CSS() string {
	var b strings.Builder
	b.WriteString("/* Code generated by internal/uitokens. DO NOT EDIT.\n")
	b.WriteString("   Run `go run ./internal/uitokens/gen` after changing tokens.go. */\n\n")

	block := func(sel string, p Palette, w Wash, dark bool) {
		fmt.Fprintf(&b, "%s {\n", sel)
		fmt.Fprintf(&b, "  color-scheme: %s;\n", map[bool]string{true: "dark", false: "light"}[dark])
		line := func(name string, c ui.Color) {
			fmt.Fprintf(&b, "  --%s: %s;\n", name, cssColor(c))
		}
		line("bg", p.Sidebar)
		line("bg-raised", p.Card)
		line("bg-sunken", p.Sidebar)
		line("bg-hover", p.RowHover)
		line("line", p.CardBorder)
		line("line-soft", p.Divider)
		line("fg", p.Text)
		line("fg-dim", p.TextMuted)
		line("fg-faint", p.TextFaint)
		line("accent", p.Accent)
		line("accent-soft", p.Accent.Alpha(0.12))
		line("ok", p.Success)
		line("warn", p.Attention)
		line("err", p.Danger)
		line("chip", p.Chip)
		line("kbd-bg", p.Kbd)
		line("kbd-line", p.KbdBorder)
		line("veil", p.Veil)
		line("tab-on", p.RowSelected)
		line("overlay", p.Overlay)
		line("popover", p.Popover)
		line("term-bg", TermTheme(dark).Background)
		stops := w.Light
		if dark {
			stops = w.Dark
		}
		for i, s := range stops {
			fmt.Fprintf(&b, "  --wash-%d: %s;\n", i+1, s)
		}
		glow := w.LightGlow
		if dark {
			glow = w.DarkGlow
		}
		line("wash-glow", glow)
		b.WriteString("}\n\n")
	}

	// The sheet's default is the light appearance; the dark one is selected by
	// the attribute the client sets from its stored preference.
	block(":root", Light, Washes[0], false)
	block("[data-theme=\"dark\"]", Dark, Washes[0], true)
	return b.String()
}

// cssColor writes a color as CSS: #rrggbb when opaque, rgba() otherwise.
func cssColor(c ui.Color) string {
	if c.A == 255 {
		return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	}
	return fmt.Sprintf("rgba(%d, %d, %d, %.2f)", c.R, c.G, c.B, float32(c.A)/255)
}
