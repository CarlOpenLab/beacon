package main

// The window's frame: what mygo draws around the view, and the controls the view
// draws itself.
//
// The title bar is hidden and the view draws the window's controls itself, in
// the corner the platform keeps them in: on macOS the three dots that close,
// minimize and zoom, at the top-left; on Windows and Linux Windows 11's flat
// minimize, maximize or restore, and close, at the top-right. Drawing them rather than leaving
// them to the system keeps one set, in one look, whatever the platform: the
// system's buttons would be a second set on macOS, and elsewhere they are laid
// over the top-right corner of whatever the window draws.

import (
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/uitokens"
	"github.com/CarlOpenLab/term-fleet/locales"
)

const (
	// The controls are three dots on the line the top bar's contents center on.
	trafficLightSize = float32(12)
	trafficLightGap  = float32(8)
	trafficLightLeft = float32(14)

	// trafficLightsW is the room the three take.
	trafficLightsW = 3*trafficLightSize + 2*trafficLightGap

	// The window's controls on Windows and Linux are three flat buttons at the
	// right end of the same line, where Windows 11 keeps its own.
	windowButtonW     = float32(28)
	windowButtonH     = float32(26)
	windowButtonGap   = float32(2)
	windowButtonRight = float32(uitokens.CardGap + 10)

	// windowButtonsW is the room the three take, and windowButtonsRoom the room
	// a card's header keeps clear for them at its right.
	windowButtonsW    = 3*windowButtonW + 2*windowButtonGap
	windowButtonsRoom = windowButtonsW + 8
)

// controlsAtRight reports whether the window's controls sit at the top-right
// corner, as Windows and Linux keep them, rather than in macOS's top-left.
// It is a variable so a test can draw the other platform's corner.
var controlsAtRight = runtime.GOOS != "darwin"

// windowLead is the room the controls take at the window's left edge before its
// content starts: the traffic lights on macOS, and only a margin where they are
// at the right instead.
func windowLead() float32 {
	if controlsAtRight {
		return 12
	}
	return trafficLightLeft + trafficLightsW + 8
}

// cardTitleLead is where a card header's title starts with the sidebar folded
// away: after the window's controls and the column's own button, which the card
// header takes over with it hidden.
func cardTitleLead() float32 {
	return windowLead() + toolbarButton + 8
}

// framedWindow applies the window's frame before it is created: a hidden title
// bar under the view's own top bar, no system buttons, and a menu bar that hides
// until Alt, as it does in a window whose chrome the app draws.
func framedWindow(o mygo.WindowOptions) mygo.WindowOptions {
	o.TitleBarStyle = mygo.TitleBarHidden
	o.HideWindowButtons = true
	o.AutoHideMenuBar = true
	return o
}

// windowFrame is the part of a window the controls act on. mygo.Window satisfies
// it; a test stands its own in.
type windowFrame interface {
	Minimize()
	Maximize()
	Unmaximize()
	Close()
	IsMaximized() bool
}

// frameOf is the window's frame, or a test's stand-in for it. The app always
// has a real one by the time it draws.
func (w *window) frameOf() windowFrame {
	if w.win != nil {
		return w.win
	}
	return w.frameStub
}

var (
	// iconMinimize, iconMaximize and iconRestore are the glyphs the controls
	// show, on Lucide's 24-unit grid like the rest of the chrome.
	iconMinimize = lucide(`<path d="M5 12h14"/>`)
	iconMaximize = lucide(`<rect width="14" height="14" x="5" y="5" rx="2"/>`)
	iconRestore  = lucide(`<rect width="10" height="10" x="9" y="9" rx="1.5"/>` +
		`<path d="M6 15H5.5A1.5 1.5 0 0 1 4 13.5v-8A1.5 1.5 0 0 1 5.5 4h8A1.5 1.5 0 0 1 15 5.5V6"/>`)
)

// trafficLightColors are the three dots' colors. They are the platform's, not
// the palette's: a red light is red in the light appearance and the dark one.
var (
	trafficClose    = ui.Hex("#ff5f57")
	trafficMinimize = ui.Hex("#febc2e")
	trafficZoom     = ui.Hex("#28c840")
)

// windowControls draws the window's close, minimize and maximize or restore in
// the corner the platform keeps them in. They are drawn in an overlay, so that
// they stay where a window's controls belong: over a dialog, above a split, and
// there even when no shell is open.
func (w *window) windowControls(c *ui.Context) {
	if w.frameOf() == nil {
		return
	}
	ui.Overlay(c, func() {
		if controlsAtRight {
			w.windowButtons(c)
			return
		}
		w.trafficLights(c)
	})
}

// trafficLights draws close, minimize and zoom in the window's top-left corner:
// three dots, with their glyphs while the pointer is over them, as macOS draws
// them.
func (w *window) trafficLights(c *ui.Context) {
	frame := w.frameOf()
	if frame == nil {
		return
	}
	row := ui.Row(c).Key("window-controls").Absolute().
		Left(trafficLightLeft).
		Top((uitokens.TopBar - trafficLightSize) / 2).
		Gap(trafficLightGap).AlignItems(ui.Center)
	hovered := row.Hovered()
	row.Children(func() {
		trafficLight(c, "window-close", trafficClose, iconClose,
			locales.WindowClose(), hovered, frame.Close)

		trafficLight(c, "window-minimize", trafficMinimize, iconMinimize,
			locales.WindowMinimize(), hovered, frame.Minimize)

		maximized := frame.IsMaximized()
		icon, label := iconMaximize, locales.WindowMaximize()
		if maximized {
			icon, label = iconRestore, locales.WindowRestore()
		}
		trafficLight(c, "window-maximize", trafficZoom, icon, label, hovered, func() {
			if maximized {
				frame.Unmaximize()
			} else {
				frame.Maximize()
			}
		})
	})
}

// windowButtons draws minimize, maximize or restore and close at the right end
// of the top bar, where Windows draws them: three flat glyphs on the app's own
// surface, the close one turning red under the pointer, as Windows 11's does.
func (w *window) windowButtons(c *ui.Context) {
	frame := w.frameOf()
	if frame == nil {
		return
	}
	pal := paletteFor(c)
	row := ui.Row(c).Key("window-controls").Absolute().
		Right(windowButtonRight).
		Top((uitokens.TopBar - windowButtonH) / 2).
		Height(windowButtonH).Gap(windowButtonGap).AlignItems(ui.Center)
	row.Children(func() {
		windowButton(c, pal, "window-minimize", iconMinimize,
			locales.WindowMinimize(), false, false, frame.Minimize)

		maximized := frame.IsMaximized()
		icon, label := iconMaximize, locales.WindowMaximize()
		if maximized {
			icon, label = iconRestore, locales.WindowRestore()
		}
		windowButton(c, pal, "window-maximize", icon, label, false, true, func() {
			if maximized {
				frame.Unmaximize()
			} else {
				frame.Maximize()
			}
		})

		windowButton(c, pal, "window-close", iconClose,
			locales.WindowClose(), true, false, frame.Close)
	})
}

// windowButton is one of the window's buttons on Windows and Linux: a quiet
// glyph that gets a wash under the pointer, and the close one a red face.
func windowButton(c *ui.Context, pal *uitokens.Palette, key string, icon *ui.SVG,
	label string, dangerous, maximize bool, run func()) {
	b := ui.ButtonBase(c).Key(key).Size(windowButtonW, windowButtonH).
		Radius(radiusSmall).Center().Cursor(ui.CursorPointer).Label(label)
	if maximize {
		// The system's maximize button for the pointer: on Windows 11 it brings
		// the snap layouts while the pointer rests there, and no tooltip covers
		// them.
		b.MaximizeButton()
	} else {
		b.Tooltip(label)
	}
	col := pal.TextMuted
	switch {
	case dangerous && (b.Hovered() || b.Pressed()):
		b.Background(pal.Danger)
		col = ui.RGB(255, 255, 255)
	case b.Pressed():
		b.Background(pal.RowHover.Alpha(0.16))
		col = pal.Text
	case b.Hovered():
		b.Background(pal.RowHover)
		col = pal.Text
	}
	b.Children(func() {
		ui.Icon(c, icon).Size(16, 16).TextColor(col)
	})
	if b.Clicked() {
		run()
	}
}

// trafficLight is one of the window's dots. The glyph is drawn only while the
// pointer is over any of the three, as on the platform: a row of dots with a
// permanent ×, − and + in them reads as buttons rather than as a window's corner.
func trafficLight(c *ui.Context, key string, color ui.Color, icon *ui.SVG,
	label string, hovered bool, run func()) {
	b := ui.Box(c).Key(key).Size(trafficLightSize, trafficLightSize).
		Radius(trafficLightSize / 2).Center().Background(color).
		Cursor(ui.CursorPointer).Label(label).Tooltip(label)
	if hovered {
		b.Children(func() {
			ui.Icon(c, icon).Size(7, 7).TextColor(ui.RGBA(0, 0, 0, 0.55))
		})
	}
	if b.Clicked() {
		run()
	}
}
