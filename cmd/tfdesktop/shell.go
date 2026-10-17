package main

import (
	_ "embed"
	"runtime"
	"strconv"
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

// tabBarHeight is the row of tabs under the header. The browser client's strip
// is 40px; this window's tabs carry the same job, and the same height keeps the
// two reading as one design.
const tabBarHeight = 40

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
//
// The band is 44px, the height the browser client's header is: the shell's name
// and where it can be reached are the same two facts there, in the same order,
// at the same size.
func (w *window) header(c *ui.Context, labels []string) {
	t := c.Theme()
	col := shellColorsFor(t)

	name := "term-fleet"
	tb := w.activeTab()
	if tb != nil && tb.id != "" {
		name = tb.label()
	}

	ui.Row(c).
		Height(headerHeight).PaddingX(14).Gap(10).
		AlignItems(ui.Center).
		Background(col.pane).
		BorderWidth(0, 0, 1, 0).BorderColor(col.hairline).
		Children(func() {
			ui.Column(c).Gap(1).Children(func() {
				ui.Text(c, name).FontSize(13).FontWeight(600)
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
				// This machine's own shell is taken back; another machine's was
				// never this window's to begin with, so it is taken over.
				take := "拿回控制权"
				if tb.machine != machineLocal {
					take = "接管"
				}
				if ui.Button(c, take).Clicked() {
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

// emptyState is the window with nothing open. The browser client's empty state
// says the same thing and offers the same two ways out; this one says it in a
// native window's terms, and the tile in the middle is the app's own mark.
func (w *window) emptyState(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).
		Fill().AlignItems(ui.Center).Justify(ui.Center).Gap(10).
		Children(func() {
			markTile(c, 44)
			ui.Text(c, "还没有打开的终端").FontSize(15)
			ui.Text(c, w.emptyLine()).FontSize(12).TextColor(t.TextMuted)
		})
}

// emptyLine is what the empty window says about the machine it would open a
// shell on, so the sentence is true rather than generic.
func (w *window) emptyLine() string {
	machine := w.selectedMachine()
	if w.fleet == nil {
		return "本机服务已关闭。"
	}
	if machine != machineLocal {
		return w.machineName(machine) + " 上还没有会话。新建一个，或在别的客户端上开一个再来看。"
	}
	if n := len(w.sessionEntries()); n > 0 {
		return "左侧有 " + strconv.Itoa(n) + " 个会话。选一个，或直接新建一个终端。"
	}
	return "本机开一个新终端，或从另一台机器接管一个。"
}

// statusBar is the quiet line along the bottom: where the service can be
// reached, and which build this is.
//
// The connection is a chip — a dot in a rounded wash — the way the browser
// client states it, so "is anything attached to this machine" reads the same in
// both. The rest of the line is the same facts in the same order.
func (w *window) statusBar(c *ui.Context) {
	t := c.Theme()
	col := shellColorsFor(t)

	ui.Row(c).
		Height(statusHeight).PaddingX(14).Gap(8).
		AlignItems(ui.Center).
		Background(col.pane).
		BorderWidth(1, 0, 0, 0).BorderColor(col.hairline).
		Children(func() {
			// The chip's wash is the state's own colour at low strength: teal
			// while a client is attached, amber while the service is listening
			// for one, quiet while there is no service at all.
			state := w.fleetColor(t)
			chip := ui.Row(c).Gap(6).AlignItems(ui.Center).
				PaddingX(9).Radius(9.5).Height(19).
				Background(state.Alpha(0.10)).
				BorderWidth(1).BorderColor(state.Alpha(0.38))
			chip.Children(func() {
				ui.Box(c).Size(6, 6).Radius(3).Background(state)
				ui.Text(c, w.serviceLine()).FontSize(11).TextColor(state)
			})
			// The window's own news, when there is any: a machine that took its
			// shell back is something the person here has to be told, and the line
			// the fleet already lives on is where they look.
			if n := w.currentNotice(); n != "" {
				ui.Spacer(c)
				ui.Text(c, n).FontSize(11).TextColor(t.Warning)
			}
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
