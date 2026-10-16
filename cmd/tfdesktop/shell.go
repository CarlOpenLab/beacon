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

// activityRail draws the strip down the left edge: the mark, the two controls
// that change the window itself, and the state of the fleet tucked against the
// bottom. It is deliberately narrow — the window it leaves is for shells.
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
			// A fixed gap rather than a spacer: a spacer would grow, and the
			// strip's two controls would drift into the middle of the window.
			ui.Box(c).Height(12)

			// 外观 says what the click will do, and shows the side it will bring:
			// a moon while the app is light, a sun while it is dark.
			appearance := iconMoon
			if t.Dark {
				appearance = iconSun
			}
			if iconButton(c, appearance, themeActionLabel(mode, t.Dark)).Clicked() {
				w.setTheme(flipTheme(mode, t.Dark))
			}
			if iconButton(c, iconSettings, "设置").Clicked() {
				w.openSettings()
			}

			ui.Spacer(c)
			w.fleetDot(c)
		})
}

// iconButton is a square control in the strip: a wash under the pointer, and the
// caption kept as its label so it has a name on screen and in a test.
func iconButton(c *ui.Context, icon *ui.SVG, caption string) ui.Element {
	t := c.Theme()
	el := ui.Box(c).
		Size(34, 34).Radius(t.Radius).
		AlignItems(ui.Center).Justify(ui.Center).
		Label(caption).Tooltip(caption)
	if el.Hovered() {
		el.Background(t.SurfaceHover)
	}
	el.Children(func() {
		ui.Icon(c, icon).FontSize(19).TextColor(shellColorsFor(t).icon)
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

// header names the shell on screen and holds the two things a shell needs: one
// more of them, and an end to this one. The service address sits under the name,
// where a window would put a subtitle.
func (w *window) header(c *ui.Context, labels []string) {
	t := c.Theme()
	col := shellColorsFor(t)

	name := "term-fleet"
	if id := w.activeID(); id != "" {
		if tb := w.tabs[id]; tb != nil {
			name = tb.label()
		}
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
			ui.Spacer(c)
			if iconButton(c, iconClose, "关闭标签页").Clicked() {
				if id := w.activeID(); id != "" {
					go w.kill(id)
				}
			}
			if primaryButton(c, iconPlus, "新建标签页").Clicked() {
				go w.create()
			}
		})
}

// primaryButton is the one filled control in the window.
func primaryButton(c *ui.Context, icon *ui.SVG, caption string) ui.Element {
	t := c.Theme()
	el := ui.ButtonBase(c).
		Height(32).PaddingX(12).Radius(t.Radius).
		AlignItems(ui.Center).Justify(ui.Center).
		Label(caption).Background(t.Accent).
		Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, icon).FontSize(15).TextColor(t.AccentText)
				ui.Text(c, caption).FontSize(13).TextColor(t.AccentText)
			})
		})
	if el.Hovered() {
		el.Background(t.AccentHover)
	}
	return el
}

// body is the shell itself, or, with nothing open, a short invitation. The empty
// window is the only place the app speaks in a sentence, so it says what to do.
func (w *window) body(c *ui.Context, labels []string) {
	if len(labels) == 0 {
		w.emptyState(c)
		return
	}
	if id := w.activeID(); id != "" {
		if tb := w.tabs[id]; tb != nil {
			terminal.View(c, tb.term).Fill().AutoFocus()
		}
	}
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
