package main

// The window's frame: what mygo draws around the view, and the controls the view
// draws itself.
//
// macOS keeps its traffic lights — they are the platform's, they sit where every
// Mac app's do, and a hand-drawn pair of circles would only be an imitation of
// them. Windows and Linux have no such agreement: the system's buttons are laid
// over the top-right corner of whatever the window draws, which on this window is
// a card's header, whose chips and hover controls they would cover. So the view
// draws its own, inside the top-right card's header, as kero does: minimize,
// maximize/restore and close in the chrome's quiet greys, with the system's
// buttons switched off.

import (
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/uitokens"
	"github.com/CarlOpenLab/term-fleet/locales"
)

// ownWindowControls tells that the window draws minimize, maximize and close
// itself. macOS keeps its traffic lights; everywhere else the view draws them.
//
// It is a variable rather than a constant for the tests: a Darwin checkout can
// then draw and press the Windows-looking controls without a Windows machine.
var ownWindowControls = runtime.GOOS != "darwin"

const (
	// winButtonW and winButtonH are one control's square, and winButtonGap the
	// room between two of them. The three together are 88 wide.
	winButtonW   = float32(28)
	winButtonH   = float32(26)
	winButtonGap = float32(2)

	// winControlsInset is what a card's header on the window's right edge keeps
	// clear for them: the header's own 10px inset, the three buttons, and the
	// gap that keeps a chip from touching them. A chip in that header ends this
	// far from the card's right edge.
	winControlsInset = float32(10) + 3*winButtonW + 2*winButtonGap + 8
)

// framedWindow applies the window's frame before it is created: a hidden title
// bar under the view's own header, and, where the view draws the controls, no
// system buttons. The menu bar hides until Alt, as it does in a window whose
// chrome the app draws.
func framedWindow(o mygo.WindowOptions) mygo.WindowOptions {
	o.TitleBarStyle = mygo.TitleBarHidden
	if ownWindowControls {
		o.HideWindowButtons = true
		o.AutoHideMenuBar = true
	}
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
	// iconMinimize, iconMaximize and iconRestore are the controls' drawings, on
	// Lucide's 24-unit grid like the rest of the chrome.
	iconMinimize = lucide(`<path d="M5 12h14"/>`)
	iconMaximize = lucide(`<rect width="14" height="14" x="5" y="5" rx="2"/>`)
	iconRestore  = lucide(`<rect width="10" height="10" x="9" y="9" rx="1.5"/>` +
		`<path d="M6 15H5.5A1.5 1.5 0 0 1 4 13.5v-8A1.5 1.5 0 0 1 5.5 4h8A1.5 1.5 0 0 1 15 5.5V6"/>`)
)

// windowControls draws minimize, maximize and close in the top-right corner of
// the window, inside the card's header, in place of the system's buttons.
//
// It is drawn at the window's root rather than in a card's header so that it
// stays where a window's controls belong: over a dialog's backdrop, above a
// split, and there even when no shell is open. The top-right card keeps its right
// edge clear with winControlsInset; the rest of the frame is untouched.
func (w *window) windowControls(c *ui.Context, pal *uitokens.Palette) {
	frame := w.frameOf()
	if !ownWindowControls || frame == nil {
		return
	}
	ui.Row(c).Key("window-controls").Absolute().
		Top(uitokens.CardGap + (uitokens.HeaderH-winButtonH)/2).
		Right(uitokens.CardGap + 10).
		Gap(winButtonGap).AlignItems(ui.Center).Children(func() {
		frameButton(c, pal, "window-minimize", iconMinimize, locales.WindowMinimize(),
			pal.TextMuted, pal.RowHover, pal.Text, frame.Minimize)

		// The maximize button the content draws gets Windows 11's snap
		// layouts as the system's does, through MaximizeButton: the view
		// still handles the click, which toggles the window here. It has no
		// tooltip, as kero's has none: the snap flyout is the one thing that
		// should appear as the pointer rests on it.
		maximized := frame.IsMaximized()
		icon, label := iconMaximize, locales.WindowMaximize()
		if maximized {
			icon, label = iconRestore, locales.WindowRestore()
		}
		b := frameButtonEl(c, pal, "window-maximize", icon, label,
			pal.TextMuted, pal.RowHover, pal.Text)
		b.MaximizeButton()
		if b.Clicked() {
			if maximized {
				frame.Unmaximize()
			} else {
				frame.Maximize()
			}
		}

		frameButton(c, pal, "window-close", iconClose, locales.WindowClose(),
			pal.TextMuted, pal.Danger, ui.RGB(255, 255, 255), frame.Close)
	})
}

// frameButton is one of the window's controls, drawn in the palette's quiet
// grey, with a wash under the pointer. Close's wash is the danger colour, with
// the drawing in white: the one control that ends the window says so before it
// is pressed, as Windows' own does.
func frameButton(c *ui.Context, pal *uitokens.Palette, key string, icon *ui.SVG,
	label string, fg, hoverBg, hoverFg ui.Color, run func()) {
	b := frameButtonEl(c, pal, key, icon, label, fg, hoverBg, hoverFg)
	b.Tooltip(label)
	if b.Clicked() {
		run()
	}
}

// frameButtonEl is the element frameButton presses, for the one control that
// needs to mark itself as something more (the maximize button's snap layouts).
func frameButtonEl(c *ui.Context, pal *uitokens.Palette, key string, icon *ui.SVG,
	label string, fg, hoverBg, hoverFg ui.Color) ui.Element {
	b := ui.Box(c).Key(key).Size(winButtonW, winButtonH).Radius(6).Center().
		TextColor(fg).Label(label)
	if b.Pressed() || b.Hovered() {
		b.Background(hoverBg).TextColor(hoverFg)
	}
	b.Children(func() {
		ui.Icon(c, icon).Size(16, 16)
	})
	return b
}
