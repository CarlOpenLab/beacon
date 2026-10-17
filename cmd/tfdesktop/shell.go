package main

import (
	_ "embed"
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"github.com/CarlOpenLab/term-fleet/internal/protocol"
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

// paneInset is how far the terminal's sheet stands off the window's ground, the
// browser client's .tf-pane::after inset. A shell that fills the window to the
// edge reads as a different kind of program from one that sits on a page.
const paneInset = 10

// terminalGround is the sheet's own colour, which does not follow the
// appearance: the ANSI sixteen are designed for a dark ground, so the terminal
// keeps the browser client's --term-bg in both appearances and only the chrome
// around it flips.
var terminalGround = ui.Hex("#0a0d12")

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

// statusHostLine is where the shell on screen runs, and what that machine is:
// its name, its platform, and how many shells it has. The browser client's
// status bar says the same, so a person moving between the two reads one line
// either way.
func (w *window) statusHostLine() string {
	tb := w.activeTab()
	if tb == nil {
		return ""
	}
	line := w.machineName(tb.machine)
	if m, ok := w.machineByID(tb.machine); ok && m.Host.Platform != "" {
		line = fmt.Sprintf("%s · %s/%s", line, m.Host.Platform, m.Host.Arch)
	}
	if n := len(w.sessionsOn(tb.machine)); n > 0 {
		line = fmt.Sprintf("%s · %d 个会话", line, n)
	}
	return line
}

// statusSessionLine is the shell on screen itself: its id, its role, its size.
func (w *window) statusSessionLine() string {
	tb := w.activeTab()
	if tb == nil {
		return ""
	}
	role := "—"
	switch {
	case !tb.alive:
		role = "已退出"
	case tb.held():
		role = "控制中"
	case tb.controller != "":
		role = "只读"
	}
	line := or(tb.id, "待创建")
	if role != "—" {
		line = fmt.Sprintf("%s · %s", line, role)
	}
	if size := tb.size(); size != "" {
		line = fmt.Sprintf("%s · %s", line, size)
	}
	return line
}

// sessionsOn is how many shells a machine is running, as the panel last heard.
func (w *window) sessionsOn(machine string) []protocol.Summary {
	if machine == machineLocal {
		return w.live
	}
	return w.remoteSessions[machine]
}

// header names the shell on screen, with the service's address under the name,
// where a window would put a subtitle. The controls a shell needs live in the
// column beside it, on the panel they act on.
//
// The band is 44px, the height the browser client's header is: the shell's name
// and where it can be reached are the same two facts there, in the same order,
// at the same size.
// tabStrip is the row of tabs under the header, the browser client's strip in
// this window's own terms: a tab per shell this window has open, whichever
// machine it runs on, with the machine named in a pill when there is more than
// one, a dot for the shell's state, and the underline coloured by role the way
// the browser's is.
//
// The header used to name the shell on screen and nothing else, which left a
// window with several shells open with no way to see them all or pick between
// them except the sidebar.
func (w *window) tabStrip(c *ui.Context, labels []string) {
	t := c.Theme()
	col := shellColorsFor(t)

	ui.Row(c).
		Height(tabBarHeight).AlignItems(ui.Stretch).
		Background(col.pane).
		BorderWidth(0, 0, 1, 0).BorderColor(col.hairline).
		Children(func() {
			ui.Scroll(c).Fill().Children(func() {
				ui.Row(c).AlignItems(ui.Stretch).Children(func() {
					for i, key := range w.order {
						tb := w.tabs[key]
						if tb == nil {
							continue
						}
						w.tab(c, tb, i == w.selected)
					}
					// One more shell, where the browser client puts its ＋.
					add := ui.Box(c).
						Width(40).FillHeight().
						AlignItems(ui.Center).Justify(ui.Center).
						Label("新建标签页").Tooltip("新建标签页 ⌘T")
					if add.Hovered() {
						add.Background(t.SurfaceHover)
					}
					add.Children(func() {
						ui.Icon(c, iconPlus).FontSize(14).TextColor(col.icon)
					})
					if add.Clicked() {
						w.newTab()
					}
				})
			})
		})
}

// tab is one tab in the strip: its machine, its state, its name, and a way to
// close it. Closing a tab stops watching the shell; ending the shell is the
// panel's own action, so the two are not the same button.
func (w *window) tab(c *ui.Context, tb *tab, active bool) {
	t := c.Theme()

	cell := ui.Row(c).
		Height(tabBarHeight).PaddingX(10).Gap(7).
		AlignItems(ui.Center).
		MaxWidth(224).
		Label(tb.label()).Tooltip(tb.label() + "\n" + w.machineName(tb.machine))
	if active {
		cell.Background(t.Background)
	} else if cell.Hovered() {
		cell.Background(t.SurfaceHover)
	}
	// The underline is the state: teal while this window holds the shell, red
	// once it is gone or its machine is offline.
	lineColor := t.Success
	switch {
	case !tb.alive:
		lineColor = t.Danger
	case tb.holder() != "":
		lineColor = t.Warning
	}
	cell.BorderWidth(0, 0, 2, 0).BorderColor(lineColor)
	if !active {
		cell.BorderColor(lineColor.Alpha(0.35))
	}

	cell.Children(func() {
		ui.Box(c).Size(7, 7).Radius(3.5).Background(w.leaseColor(t, tb.machine, tb.id, tb.holder()))
		// The machine's pill only when more than one machine is in play: on a
		// single-machine window it is a fact the sidebar already states.
		if w.multiMachine() {
			pill := ui.Box(c).Radius(4).PaddingX(5).Height(16).
				AlignItems(ui.Center).Justify(ui.Center)
			if active {
				pill.Background(t.Success.Alpha(0.12)).BorderWidth(1).BorderColor(t.Success.Alpha(0.38))
			} else {
				pill.Background(t.Background.Alpha(0.5)).BorderWidth(1).BorderColor(t.Border)
			}
			pill.Children(func() {
				ui.Text(c, w.machineName(tb.machine)).FontSize(10).TextColor(t.TextMuted)
			})
		}
		name := ui.Text(c, tb.label()).FontSize(12)
		if active {
			name.TextColor(t.Text).FontWeight(550)
		} else {
			name.TextColor(t.TextMuted)
		}
		// Closing a tab stops watching the shell; the shell keeps running, and
		// ending it is the panel's own action. So the × is on the tab, and the
		// 结束 that kills a process is not.
		close := ui.Box(c).
			Size(18, 18).Radius(4).
			AlignItems(ui.Center).Justify(ui.Center).
			Label("关闭标签页").Tooltip("关闭标签页（仅停止观看，不结束进程）")
		if close.Hovered() {
			close.Background(t.Danger.Alpha(0.12))
		}
		close.Children(func() {
			ui.Icon(c, iconClose).FontSize(11).TextColor(t.TextMuted)
		})
		if close.Clicked() {
			w.removeTab(tb)
		}
	})

	if cell.Clicked() {
		w.selectTab(tb)
	}
}

// multiMachine reports whether this window has shells on more than one machine,
// which is what decides whether a tab names the machine it belongs to.
func (w *window) multiMachine() bool {
	first := ""
	for _, key := range w.order {
		tb := w.tabs[key]
		if tb == nil {
			continue
		}
		if first == "" {
			first = tb.machine
			continue
		}
		if tb.machine != first {
			return true
		}
	}
	return false
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
			// The inspector's opener sits at the far end of the header, where
			// the browser client puts its toggle: it is about the shell on
			// screen, not about the window.
			ui.Spacer(c)
			show := iconButton(c, iconInfo, "会话检视", panelButtonSize)
			show.Tooltip("会话检视（会话的 id、尺寸、控制端）")
			if show.Clicked() {
				inspectVisible = !inspectVisible
				w.invalidate()
			}
			// The lease, and the two things a person does about it.
			//
			// 接管 and 释放 are separate buttons because they are opposite
			// questions: one takes the shell from someone else, the other hands
			// it back. Neither ends the process and neither closes the tab —
			// 结束 and 关闭标签 are their own actions, on the panel and the tab
			// strip, and coupling any of the four together is how a client ends
			// up killing a shell it only meant to stop watching.
			if tb == nil {
				return
			}
			holder := tb.holder()
			if holder == "" {
				// This window holds the lease: it can give it up without giving
				// up the shell. The hub hands the lease to the machine the shell
				// runs on, so nobody is left with a mute terminal.
				if !tb.alive {
					return
				}
				ui.Spacer(c)
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Row(c).Gap(6).AlignItems(ui.Center).Label("你正在控制").Children(func() {
						ui.Box(c).Size(8, 8).Radius(4).Background(t.Success)
						ui.Text(c, "你正在控制").FontSize(11).TextColor(t.Success)
					})
					if ui.Button(c, "释放控制权").Tooltip("释放控制权：终端继续运行，本窗口改为只读观看").Clicked() {
						w.releaseLease(tb)
					}
				})
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
				if ui.Button(c, take).Tooltip("取得控制权（对方会变成只读）").Clicked() {
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

	// The terminal sits on a sheet of its own: inset from the window's ground,
	// rounded, with a hairline off its edge. The browser client draws the same
	// sheet around its xterm, and a shell that ran edge to edge in one client
	// and floated in the other was the difference people noticed first.
	sheet := ui.Box(c).Fill().Margin(paneInset).Radius(radius).
		BorderWidth(1).BorderColor(shellColorsFor(c.Theme()).hairline).
		Background(terminalGround)
	sheet.Children(func() {
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
	})
}

// reclaim asks for the lease of a shell another client holds. This machine's own
// shell is claimed outright — the hub honours the machine's window without a
// confirmation, because it is the machine's shell; another machine's is stolen,
// and the holder is told.
func (w *window) reclaim(tb *tab) {
	tb.conn.claiming(tb.machine != machineLocal)
}

// releaseLease gives the lease up without giving up the shell: the process keeps
// running and the tab keeps watching it, this window just stops being the one
// that types into it. The hub hands the lease to the machine the shell runs on,
// so the shell is not left mute.
//
// This is the opposite question to reclaim, and its own button: 释放 is not the
// absence of 接管, it is a thing a person chooses to do — usually so somebody
// else can take the keyboard while this window keeps the output on screen.
func (w *window) releaseLease(tb *tab) {
	// The window let this go a frame ago, so the role frame answering it — which
	// names the machine as the new controller — is not a machine taking the shell
	// back and must not open the popover or tear the link down.
	tb.letGoLease()
	tb.conn.releasing()
	w.setNotice("已释放控制权：" + tb.label() + " 继续运行，本窗口改为只读")
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
			// The machine and the shell, the two facts the browser client keeps
			// in its status bar: where the shell on screen runs, and what it is.
			// Without them a window driving three machines' shells has nothing
			// on screen that says which one this is.
			if host := w.statusHostLine(); host != "" {
				ui.Text(c, host).FontSize(11).TextColor(t.TextMuted)
			}
			if sess := w.statusSessionLine(); sess != "" {
				ui.Text(c, sess).FontSize(11).TextColor(t.TextMuted)
			}
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
