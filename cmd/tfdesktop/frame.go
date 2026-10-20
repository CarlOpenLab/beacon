package main

// The window's frame: what mygo draws around the view, and the controls the view
// draws itself.
//
// The title bar is hidden and the view draws the window's controls in the top-left
// corner: three dots, close, minimize and zoom, where a Mac app's are. They are
// drawn on every platform rather than left to the system: the system's buttons on
// macOS would be a second set beside these, and on Windows and Linux they are a
// Windows-shaped set laid over the top-right corner of whatever the window draws,
// which here is a card's header whose chips they would cover. Drawing them keeps
// the three in one place, in one look, whatever the platform.

import (
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

	// trafficLightsW is the room the three take, and windowLead the room the
	// window's own content gives them before it starts.
	trafficLightsW = 3*trafficLightSize + 2*trafficLightGap
	windowLead     = trafficLightLeft + trafficLightsW + 8
)

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
	// iconMinimize, iconMaximize and iconRestore are the glyphs that show in the
	// dots while the pointer rests on them, on Lucide's 24-unit grid like the
	// rest of the chrome.
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

// windowControls draws close, minimize and zoom in the window's top-left
// corner: three dots, with their glyphs while the pointer is over them. It is
// drawn at the window's root so that they stay where a window's controls belong:
// over a dialog's backdrop, above a split, and there even when no shell is open.
func (w *window) windowControls(c *ui.Context) {
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
