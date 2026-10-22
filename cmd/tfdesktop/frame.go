package main

// The window's frame: what mygo draws around the view, and the controls the view
// draws itself.
//
// The title bar is hidden. On macOS the system's traffic lights are kept, moved
// onto the same 30px line the rest of the chrome centres on, and the view gives
// them the room c.TitleBar reports instead of drawing anything there: the three
// dots are AppKit's, down to their glyphs and their grey in an inactive window.
// On Windows and Linux the system's buttons are taken away, and the view draws
// Windows 11's minimize, maximize or restore, and close at the top-right, where
// those platforms keep them.

import (
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/uitokens"
	"github.com/CarlOpenLab/term-fleet/locales"
)

const (
	// trafficLightX and trafficLightY place macOS's traffic lights: the close
	// button's top-left corner, 18 DIPs in from the window's corner, with the
	// lights' centre on the line the top bar's contents centre on. MyGo puts
	// them a DIP above the position, and the buttons are 14 tall, which leaves
	// a top bar 60 tall, like the sidebar's own.
	trafficLightX = 18
	trafficLightY = uitokens.TopBar/2 - 7

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

// controlsAtRight reports whether the view draws the window's controls at the
// top-right corner, as Windows and Linux keep them, rather than leaving them to
// the system at macOS's top-left. It is a variable so a test can draw either.
var controlsAtRight = runtime.GOOS != "darwin"

// framedWindow applies the window's frame before it is created: a hidden title
// bar under the view's own top bar, the system's controls only where the view
// does not draw its own, and a menu bar that hides until Alt, as it does in a
// window whose chrome the app draws.
func framedWindow(o mygo.WindowOptions) mygo.WindowOptions {
	o.AutoHideMenuBar = true
	if controlsAtRight {
		o.TitleBarStyle = mygo.TitleBarHidden
		o.HideWindowButtons = true
		return o
	}
	// macOS keeps its traffic lights: they are moved onto the line the chrome
	// centres on, and the view lays out around what c.TitleBar reports.
	o.TitleBarStyle = mygo.TitleBarHiddenInset
	o.TrafficLightPosition = &mygo.Point{X: trafficLightX, Y: trafficLightY}
	return o
}

// titleBarLead is where content begins after the room the system's window
// controls report at the top-left, plus a gap: the traffic lights on macOS, and
// only a margin where the view draws the controls at the right instead.
func titleBarLead(bar ui.TitleBar) float32 {
	if bar.Left > 0 {
		return bar.Left + 8
	}
	return 12
}

// cardTitleLead is where a card header's title starts with the sidebar folded
// away: after that room and the column's own button, which the card header
// takes over with it hidden.
func cardTitleLead(bar ui.TitleBar) float32 {
	return titleBarLead(bar) + toolbarButton + 8
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

// windowControls draws the window's own controls, and only on Windows and Linux:
// macOS's traffic lights are the system's, so there is nothing to draw there.
func (w *window) windowControls(c *ui.Context) {
	if !controlsAtRight || w.frameOf() == nil {
		return
	}
	ui.Overlay(c, func() {
		w.windowButtons(c)
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
