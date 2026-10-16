package main

import (
	_ "embed"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

// The shell is the frame the rest of the window sits in: the icon strip down the
// left, the column of shells beside it, the name of the shell on screen with the
// two things it needs, and the state of the fleet along the bottom.

// activityWidth is the strip's width: one column of square controls.
const activityWidth = 56

// activityButtonSize is the square of the strip's controls.
const activityButtonSize = 34

//go:embed assets/mark.png
var markPNG []byte

// mark is the app's own icon, decoded once. It heads the strip, and it is what
// stands in the middle of the window while no shell is open.
var mark = decodeMark()

func decodeMark() *ui.Bitmap {
	bmp, err := ui.DecodeBitmap(markPNG)
	if err != nil {
		return nil
	}
	return bmp
}

// markTile draws the mark at a size, or nothing at all when it could not be
// read: a missing picture must not take the window down with it.
func markTile(c *ui.Context, size float32) {
	if mark == nil {
		return
	}
	ui.Image(c, mark).Size(size, size).Radius(size * 0.28).Label("term-fleet")
}

// activityRail draws the strip down the left edge: the mark heads it, and the
// two controls that change the window itself sit against the bottom, over the
// state of the fleet. It is deliberately narrow — the window it leaves is for
// shells.
func (w *window) activityRail(c *ui.Context, mode string) {
	t := c.Theme()
	col := shellColorsFor(t)

	ui.Column(c).
		Width(activityWidth).FillHeight().
		AlignItems(ui.Center).Justify(ui.Start).
		Gap(6).PaddingY(12).
		Background(col.rail).
		BorderWidth(0, 1, 0, 0).BorderColor(col.hairline).
		Children(func() {
			markTile(c, 30)
			// A spacer rather than a fixed gap: the strip's controls belong at
			// the bottom, whatever height the window is, and a gap would leave
			// them drifting up with the mark.
			ui.Spacer(c)

			// The strip's footer, under a hairline: what changes the window,
			// then the fleet's state under it. The mark names the app; these
			// are everything the app itself can be told to do.
			ui.Box(c).FillWidth().Height(1).Background(col.hairline)
			ui.Box(c).Height(8)

			// 外观 says what the click will do, and shows the side it will bring:
			// a moon while the app is light, a sun while it is dark.
			appearance := iconMoon
			if t.Dark {
				appearance = iconSun
			}
			if iconButton(c, appearance, themeActionLabel(mode, t.Dark), activityButtonSize).Clicked() {
				w.setTheme(flipTheme(mode, t.Dark))
			}
			if iconButton(c, iconSettings, "设置", activityButtonSize).Clicked() {
				w.openSettings()
			}

			ui.Box(c).Height(4)
			w.fleetDot(c)
		})
}

// iconButton is a square control in the strip: a wash under the pointer, and the
// caption kept as its label so it has a name on screen and in a test.
func iconButton(c *ui.Context, icon *ui.SVG, caption string, size float32) ui.Element {
	t := c.Theme()
	el := ui.Box(c).
		Size(size, size).Radius(t.Radius).
		AlignItems(ui.Center).Justify(ui.Center).
		Label(caption).Tooltip(caption)
	if el.Hovered() {
		el.Background(t.SurfaceHover)
	}
	el.Children(func() {
		ui.Icon(c, icon).FontSize(size * 0.55).TextColor(shellColorsFor(t).icon)
	})
	return el
}

// fleetDot ends the strip with the state of the local service, so the answer to
// "is anything attached to this machine" is always on screen.
func (w *window) fleetDot(c *ui.Context) {
	t := c.Theme()
	ui.Box(c).
		Size(9, 9).Radius(4.5).
		Background(w.fleetColor(t)).
		Tooltip(w.serviceLine())
}

// fleetColor is the dot's colour: green while a client is attached, amber while
// the service is listening for one, and quiet while there is no service at all.
func (w *window) fleetColor(t *ui.Theme) ui.Color {
	switch {
	case w.fleet == nil:
		return shellColorsFor(t).quiet
	case w.fleet.ClientPresent():
		return t.Success
	case w.fleet.ServerOn():
		return t.Warning
	}
	return shellColorsFor(t).quiet
}

// serviceLine is what the window says about the local service: where it can be
// reached, or that there is no service at all. A window without a service is a
// window, not a crash: it is the state the shell starts in.
func (w *window) serviceLine() string {
	if w.fleet == nil {
		return "本机服务已关闭"
	}
	return w.serviceLabel()
}

// header names the shell on screen, with the service's address under the name,
// where a window would put a subtitle. The controls a shell needs live in the
// column beside it, on the panel they act on.
func (w *window) header(c *ui.Context, labels []string) {
	t := c.Theme()
	col := shellColorsFor(t)

	name := "term-fleet"
	tb := w.activeTab()
	if tb != nil && tb.id != "" {
		name = tb.label()
	}

	ui.Row(c).
		Height(52).PaddingX(14).Gap(10).
		AlignItems(ui.Center).
		Background(col.pane).
		BorderWidth(0, 0, 1, 0).BorderColor(col.hairline).
		Children(func() {
			ui.Column(c).Gap(1).Children(func() {
				ui.Text(c, name).FontSize(14).FontWeight(600)
				ui.Text(c, w.serviceLine()).FontSize(11).TextColor(t.TextMuted)
			})
			// A shell someone else is driving says so here, with the way back: the
			// glow says "not yours right now", the popover explains it, and this is
			// the button a person looks for when the keyboard has stopped working.
			if tb == nil {
				return
			}
			holder := tb.holder()
			if holder == "" {
				return
			}
			ui.Spacer(c)
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Label(holder + " 正在控制").Children(func() {
					ui.Box(c).Size(8, 8).Radius(4).Background(t.Warning)
					ui.Text(c, holder+" 正在控制").FontSize(11).TextColor(t.Warning)
				})
				if ui.Button(c, "拿回控制权").Clicked() {
					w.reclaim(tb)
				}
			})
		})
}

// body is the shell itself, or, with nothing open, a short invitation. The empty
// window is the only place the app speaks in a sentence, so it says what to do.
func (w *window) body(c *ui.Context, labels []string) {
	if len(labels) == 0 {
		w.emptyState(c)
		return
	}
	tb := w.activeTab()
	if tb == nil || tb.term == nil {
		return
	}

	holder := tb.holder()

	pane := ui.Box(c).Fill().Children(func() {
		terminal.View(c, tb.term).Fill().AutoFocus()
	})
	if holder == "" {
		return // this window holds the lease: nothing to say, nothing to draw
	}

	// The fade is an animation, so a desktop asking for less motion gets the
	// glow at once rather than slowly; the breathing stands still for the same
	// reason, which is the preference the glow can read.
	strength := pane.Animate("lease", 1, glowFade)
	leaseGlow(c, float64(strength), c.Preferences().ReduceMotion)

	// The popover hangs from a marker at the top of the pane, so it floats over
	// the terminal's first rows: a panel hung from the pane itself would have
	// no room below it and flip above the window.
	marker := ui.Box(c).Height(2)
	leasePopover(c, marker, &w.leaseOpen, holder, tb.machine != machineLocal, func() {
		w.reclaim(tb)
	})
}

// reclaim asks for the lease of a shell another client holds. This machine's own
// shell is claimed outright — the hub honours the machine's window without a
// confirmation, because it is the machine's shell; another machine's is stolen,
// and the holder is told.
func (w *window) reclaim(tb *tab) {
	tb.conn.claiming(tb.machine != machineLocal)
}

func (w *window) emptyState(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).
		Fill().AlignItems(ui.Center).Justify(ui.Center).Gap(10).
		Children(func() {
			markTile(c, 56)
			ui.Text(c, "还没有终端").FontSize(15)
			ui.Text(c, "点「新建标签页」在本机开一个，或从另一台机器接管一个。").
				FontSize(12).TextColor(t.TextMuted)
		})
}

// statusBar is the quiet line along the bottom: where the service can be reached,
// and which build this is.
func (w *window) statusBar(c *ui.Context) {
	t := c.Theme()
	col := shellColorsFor(t)

	ui.Row(c).
		Height(28).PaddingX(14).Gap(8).
		AlignItems(ui.Center).
		Background(col.pane).
		BorderWidth(1, 0, 0, 0).BorderColor(col.hairline).
		Children(func() {
			ui.Box(c).Size(8, 8).Radius(4).Background(w.fleetColor(t))
			ui.Text(c, w.serviceLine()).FontSize(11).TextColor(t.TextMuted)
			ui.Spacer(c)
			ui.Text(c, buildLine()).FontSize(11).TextColor(t.TextMuted)
		})
}

// buildLine is the version and the platform, the way a status bar says it.
func buildLine() string {
	if v := strings.TrimSpace(mygo.App.Version()); v != "" {
		return "v" + v + " · " + runtime.GOOS + "/" + runtime.GOARCH
	}
	return runtime.GOOS + "/" + runtime.GOARCH
}
