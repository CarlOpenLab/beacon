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

// The shell is the frame the rest of the window sits in: the band across the
// top (the product's name, the one thing the window has to say about the shell
// on screen, and the controls that act on the window), the strip of open shells
// under it, and the quiet line along the bottom.
//
// It is the browser client's frame in a native window's terms, and the two are
// meant to read as one product: the same band heights, the same strip of pills,
// the same three facts along the bottom in the same order.

// paneInset is how far the terminal's sheet stands off the window's ground, the
// browser client's .tf-panes margin. A shell that fills the window to the edge
// reads as a different kind of program from one that sits on a page.
const paneInset = 10

// terminalGround is the sheet's own colour, which does not follow the
// appearance: the ANSI sixteen are designed for a dark ground, so the terminal
// keeps the browser client's --term-bg in both appearances and only the chrome
// around it flips.
var terminalGround = ui.Hex("#0d0d12")

//go:embed assets/mark.png
var markPNG []byte

// mark is the app's own icon, decoded once. It heads the window's brand and it
// is what stands in the middle of the window while no shell is open.
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

// header is the band across the top of the window: the product's name on the
// left, what the window has to say in the middle, and the controls on the right.
//
// It names the product rather than the shell on screen, which is the browser
// client's arrangement: the tab says which shell this is, the sidebar lists the
// rest, and the one sentence worth the middle of the band — a shell that is not
// this window's to type into — is what the band is actually for.
func (w *window) header(c *ui.Context) {
	t := c.Theme()
	col := shellColorsFor(t)

	ui.Row(c).
		Height(headerHeight).PaddingX(12).Gap(10).
		AlignItems(ui.Center).
		Background(col.pane).
		BorderWidth(0, 0, 1, 0).BorderColor(col.hairline).
		Children(func() {
			ui.Row(c).Gap(7).AlignItems(ui.Center).Label("term-fleet").Children(func() {
				markTile(c, 18)
				ui.Text(c, "term-fleet").FontSize(13).FontWeight(600)
			})
			ui.Spacer(c)
			w.headerBanner(c)
			w.headerActions(c)
		})
}

// headerActions is the right-hand group: what the window can be told to do,
// then the one thing a person does most.
//
// 新建终端 is the accent control, and it is allowed to be: opening a shell is
// what the window is for. The three beside it are quiet icon buttons, and 释放
// only appears while this window is driving a shell that is not this machine's
// — a control that is not about right now has no business being drawn.
//
// 释放控制权 on a shell this machine runs is the one case it is left out of,
// and not because it is rare: releasing it drops the lease and hands it to
// nobody, so the only thing the button does there is stop this terminal
// accepting the typing of the person sitting at it, with no holder to name and
// nothing on screen to say why. A shell of this machine's is typed into here;
// there is no one to give it to.
func (w *window) headerActions(c *ui.Context) {
	t := c.Theme()

	if tb := w.activeTab(); tb != nil && tb.alive && tb.held() && tb.machine != machineLocal {
		if ui.Button(c, "释放控制权").
			Tooltip("释放控制权：终端继续运行，交还给运行它的机器").Clicked() {
			w.releaseLease(tb)
		}
	}
	inspect := iconButton(c, iconInfo, "会话检视", headerButtonSize)
	inspect.Tooltip("会话检视（会话的 id、尺寸、控制端）")
	if inspect.Clicked() {
		inspectVisible = !inspectVisible
		w.invalidate()
	}
	appearance := iconMoon
	if t.Dark {
		appearance = iconSun
	}
	if iconButton(c, appearance, themeActionLabel(w.store.Get().Theme, t.Dark), headerButtonSize).Clicked() {
		w.setTheme(flipTheme(w.store.Get().Theme, t.Dark))
	}
	if iconButton(c, iconSettings, "设置", headerButtonSize).Clicked() {
		w.openSettings()
	}

	// The primary control, with the keystroke that does the same thing written
	// on it: a window that has a shortcut should say so where the button is.
	new := ui.PrimaryButton(c, "").
		Height(headerButtonSize).PaddingX(11).Gap(7).
		Label("新建终端").Tooltip("新建终端 ⌘T")
	new.Children(func() {
		ui.Text(c, "新建终端").FontSize(12).FontWeight(600)
		kbdChip(c, "⌘T", true)
	})
	if new.Clicked() {
		w.newTab()
	}
}

// headerButtonSize is the square the header's icon buttons take, and the height
// of the primary control beside them. It is the browser client's .btn height.
const headerButtonSize = 30

// headerBanner is the one sentence the window has to say about the shell on
// screen, with the action that answers it. It is the browser client's banner:
// a state in the state's own colour, and one button.
func (w *window) headerBanner(c *ui.Context) {
	text, action, tone, run := w.bannerState(c)
	if text == "" {
		return
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, text).FontSize(12).TextColor(tone).MaxWidth(420).SingleLine()
		if action == "" {
			return
		}
		act := ui.Box(c).
			Height(20).PaddingX(9).Radius(radiusPill).
			AlignItems(ui.Center).Justify(ui.Center).
			Label(action).Tooltip(action)
		act.Background(tone.Alpha(0.10)).BorderWidth(1).BorderColor(tone.Alpha(0.42))
		if act.Hovered() {
			act.Background(tone.Alpha(0.22))
		}
		act.Children(func() {
			ui.Text(c, action).FontSize(11).FontWeight(600).TextColor(tone)
		})
		if act.Clicked() {
			run()
		}
	})
}

// bannerState is what the band has to say, as a sentence, the word on the
// button that answers it, the colour it is said in, and what the button does.
//
// Only two things are worth the middle of the band, and both are about the
// shell on screen: it has ended, or it is not this window's to type into. What
// just happened to the window — a lease given up, a machine that took a shell
// back — is news rather than state, and the bottom line is where news goes.
func (w *window) bannerState(c *ui.Context) (text, action string, tone ui.Color, run func()) {
	t := c.Theme()
	tb := w.activeTab()
	if tb == nil {
		return "", "", t.TextMuted, nil
	}
	if !tb.alive {
		// The window says the state and offers the one thing left to do with it.
		// The sentence does not name the button: a banner that reads
		// "…关闭标签 [关闭标签]" is a banner nobody reads twice.
		return "会话已退出，最后几行仍可读取。", "关闭标签", t.Danger, func() {
			w.removeTab(tb)
		}
	}
	if holder := tb.holder(); readOnly(tb) {
		// This machine's own shell is taken back; another machine's was never
		// this window's to begin with, so it is taken over.
		take := "拿回控制权"
		if tb.machine != machineLocal {
			take = "接管"
		}
		if holder == "" {
			// Nobody's, and said as such: a band that named a holder here would
			// have a blank where the name goes. The way in is the same button —
			// a claim on a free lease is honoured at once, with no one to steal
			// it from and no one to tell.
			return "只读：当前无人控制", take, t.Warning, func() {
				w.reclaim(tb)
			}
		}
		return "只读：该会话正被 " + holder + " 控制", take, t.Warning, func() {
			w.reclaim(tb)
		}
	}
	return "", "", t.TextMuted, nil
}

// kbdChip is a keystroke written the way a keyboard writes it: a chip in the
// chrome's quiet greys, or in the accent's own ink when it sits on the accent.
func kbdChip(c *ui.Context, text string, onAccent bool) {
	t := c.Theme()
	col := shellColorsFor(t)

	chip := ui.Box(c).Height(18).PaddingX(6).Radius(radiusSmall).
		AlignItems(ui.Center).Justify(ui.Center)
	ink := col.quiet
	if onAccent {
		chip.Background(ui.RGBA(0, 0, 0, 0.16)).BorderWidth(1).BorderColor(ui.RGBA(1, 1, 1, 0.22))
		ink = t.AccentText
	} else {
		chip.Background(col.field).BorderWidth(1).BorderColor(col.line)
	}
	chip.Children(func() {
		ui.Text(c, text).FontSize(11).TextColor(ink)
	})
}

// iconButton is a square control in the chrome: a wash under the pointer, and
// the caption kept as its label so it has a name on screen and in a test.
func iconButton(c *ui.Context, icon *ui.SVG, caption string, size float32) ui.Element {
	t := c.Theme()
	el := ui.Box(c).
		Size(size, size).Radius(radiusSmall).
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

// fleetColor is the service's colour in the status chip: green while a client
// is attached, amber while the service is listening for one, and quiet while
// there is no service at all.
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

// tabStrip is the row of open shells: a pill per tab, whichever machine it runs
// on, with the machine named in a chip when there is more than one machine in
// play, a dot for the shell's state, and a way to close it. The pill on screen
// is raised off the strip; the rest sit flat on it.
func (w *window) tabStrip(c *ui.Context) {
	col := shellColorsFor(c.Theme())

	ui.Row(c).
		Height(tabBarHeight).PaddingX(10).Gap(4).
		AlignItems(ui.Center).
		Background(col.pane).
		BorderWidth(0, 0, 1, 0).BorderColor(col.hairline).
		Children(func() {
			ui.Scroll(c).Fill().Children(func() {
				ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
					for i, key := range w.order {
						tb := w.tabs[key]
						if tb == nil {
							continue
						}
						w.tab(c, tb, i == w.selected)
					}
					// One more shell, where the browser client puts its ＋.
					add := ui.Box(c).
						Size(30, 30).Radius(radiusPill).
						AlignItems(ui.Center).Justify(ui.Center).
						Label("新建标签页").Tooltip("新建终端 ⌘T")
					if add.Hovered() {
						add.Background(col.field)
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
	col := shellColorsFor(t)

	pill := ui.Row(c).
		Height(30).PaddingStart(11).PaddingEnd(6).Gap(7).
		Radius(radiusPill).
		AlignItems(ui.Center).
		MaxWidth(230).
		Label(tb.name()).Tooltip(tabTooltip(tb, w.machineName(tb.machine)))
	if active {
		// The pill on screen is raised: a step up from the strip's own face, a
		// hairline all round, and the light that falls on a raised thing.
		pill.Background(col.tabOn).
			BorderWidth(1).BorderColor(col.line).
			Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.35))
	} else if pill.Hovered() {
		pill.Background(col.field)
	}
	if !tb.alive {
		pill.Opacity(0.6)
	}

	pill.Children(func() {
		ui.Box(c).Size(7, 7).Radius(3.5).Background(w.leaseColor(t, tb.machine, tb.id, tb.holder(), tb.alive))
		// The machine's chip only when more than one machine is in play: on a
		// single-machine window it is a fact the sidebar already states. It is
		// the browser client's chip to the pixel — 86 wide at most, one line,
		// cut — because a hostname that wraps inside a 16-high chip is a chip
		// with three lines of text spilling out of it.
		if w.multiMachine() {
			machine := ui.Box(c).Height(16).MaxWidth(86).Shrink(0).
				PaddingX(6).Radius(radiusSmall).
				AlignItems(ui.Center).Justify(ui.Center).
				Background(col.field).BorderWidth(1).BorderColor(col.line)
			machine.Tooltip(w.machineName(tb.machine))
			machine.Children(func() {
				ui.Text(c, w.machineName(tb.machine)).FontSize(10).TextColor(col.quiet).SingleLine()
			})
		}
		// One line, cut with an ellipsis, the way the browser client's
		// .tf-tab-label is: the tab is a pill of a fixed 30, so a name that wraps
		// inside it spills out of the strip. What the strip does not show — the
		// whole name, the machine, the exit code — is in the tooltip, and a shell
		// that has ended is the dimmed pill with the dot in the exit's colour.
		name := ui.Text(c, tb.name()).FontSize(12.5).SingleLine()
		if active {
			name.TextColor(t.Text)
		} else {
			name.TextColor(t.TextMuted)
		}
		// Closing a tab stops watching the shell; the shell keeps running, and
		// ending it is the panel's own action. So the × is on the tab, and the
		// 结束 that kills a process is not.
		close := ui.Box(c).
			Size(18, 18).Radius(radiusPill).
			AlignItems(ui.Center).Justify(ui.Center).
			Label("关闭标签页").Tooltip("关闭标签页（仅停止观看，不结束进程）")
		if close.Hovered() {
			close.Background(col.field)
		}
		close.Children(func() {
			ui.Icon(c, iconClose).FontSize(11).TextColor(col.icon)
		})
		if close.Clicked() {
			w.removeTab(tb)
		}
	})

	if pill.Clicked() {
		w.selectTab(tb)
	}
}

// tabTooltip is what a tab says on hover: the shell's name, the machine it runs
// on, and — for a shell that has ended — the code it ended with, which is the
// one thing about it the strip has no room for.
func tabTooltip(tb *tab, machine string) string {
	lines := []string{tb.name(), machine}
	if line := tb.exitLine(); line != "" {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
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

// body is the shell itself, or, with nothing open, the empty state. The terminal
// sits on a sheet of its own — inset from the window's ground, rounded, with a
// hairline off its edge — the way the browser client draws its panes, and a
// shell that is not this window's to type into is marked on the sheet rather
// than behind it.
func (w *window) body(c *ui.Context) {
	if len(w.order) == 0 {
		w.emptyState(c)
		return
	}
	tb := w.activeTab()
	if tb == nil || tb.term == nil {
		return
	}

	// Whether this window may type into the shell on screen, and who it is that
	// can instead — the one answer the sheet and the room around it are drawn
	// from, and the panel's own words. See readOnly.
	holder := tb.holder()

	// The pane floats on the window's ground, and the light along its edge is
	// drawn over both, so it can spill past the edge it belongs to. A light
	// clipped to the pane would be a coloured border on it.
	ui.Box(c).Fill().Children(func() {
		sheet := ui.Box(c).Fill().Margin(paneInset).Radius(radiusLarge).
			BorderWidth(1).BorderColor(shellColorsFor(c.Theme()).hairline).
			Background(terminalGround)
		sheet.Children(func() {
			ui.Box(c).Fill().Children(func() {
				terminal.View(c, tb.term).Fill().AutoFocus()
			})
			if !tb.alive {
				// A shell that has ended says so on the sheet: the tab and the
				// banner say it too, and the pane is where a person is looking.
				endedBadge(c)
			} else if readOnly(tb) {
				readOnlyBadge(c)
			}
		})

		// What is left below is the shell that is somebody else's or nobody's. A
		// shell that has ended is not that — the banner has already said what
		// happened to it — and a panel offering 拿回控制权 over a process the hub
		// has forgotten would be offering a button that cannot work.
		if !tb.alive || !readOnly(tb) {
			return
		}

		// The light along the edge is for one state only: a shell this machine
		// runs, with another client on its keyboard. Every other 只读 shell gets
		// the chip and the panel and no light, because the light is a thing a
		// person reads from across the room — "somebody has this machine's
		// terminal" — and it would say nothing at all about a process on a
		// machine they are not sitting at.
		if w.drivenElsewhere(tb) {
			// The fade is an animation, so a desktop asking for less motion gets
			// the light at once rather than slowly, and the drift stands still
			// for the same reason, which is the preference the shader reads.
			strength := ui.Box(c).Fill().Animate("lease", 1, glowFade)
			leaseGlow(c, float64(strength), c.Preferences().ReduceMotion)
		}

		// The panel hangs from a marker at the top of the pane, so it floats over
		// the terminal's first rows: a panel hung from the pane itself would have
		// no room below it and flip above the window. The marker is an overlay
		// like the badge, so the terminal keeps the whole sheet.
		marker := ui.Box(c).Absolute().Top(paneInset).Start(paneInset).Size(2, 2)
		leasePopover(c, marker, &w.leaseOpen, holder, tb.machine != machineLocal, func() {
			w.reclaim(tb)
		})
	})
}

// readOnlyBadge is the mark on a sheet another client is typing into, in the
// corner the browser client puts its own: the word, in the state's colour, on
// the smallest piece of chrome that can hold it.
//
// It is on every shell another client holds, wherever that shell runs. The light
// along the pane's edge is not: it is drawn for this machine's own shells alone,
// because "somebody has this machine's terminal" is a fact about the machine a
// person is sitting at, and a pane that lit up for another machine's shell would
// be lighting up for something its reader has no stake in. The chip is the part
// that reads the same everywhere, so a shell this window only watches still says
// what it is without shouting.
func readOnlyBadge(c *ui.Context) {
	badge(c, "只读", c.Theme().Warning)
}

// endedBadge is the same mark for a shell that has exited.
func endedBadge(c *ui.Context) {
	badge(c, "已退出", c.Theme().Danger)
}

// badge draws a state word in the sheet's top-right corner, as an overlay the
// pointer goes through: it says what the sheet is without taking any of it.
func badge(c *ui.Context, text string, color ui.Color) {
	ui.Box(c).Absolute().Top(12).End(14).PassThrough().Children(func() {
		chip := ui.Row(c).Height(20).PaddingX(8).Radius(radiusSmall).
			AlignItems(ui.Center).Justify(ui.Center).
			Background(color.Alpha(0.12)).BorderWidth(1).BorderColor(color.Alpha(0.42))
		chip.Children(func() {
			ui.Text(c, text).FontSize(10).LetterSpacing(0.4).TextColor(color)
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
	w.setNotice("已释放控制权：" + tb.name() + " 继续运行，本窗口改为只读")
}

// emptyState is the window with nothing open. It is not a dead end: it says what
// the window is for and offers the one action that starts it, the way the
// browser client's empty state does.
func (w *window) emptyState(c *ui.Context) {
	t := c.Theme()
	col := shellColorsFor(t)

	ui.Column(c).
		Fill().AlignItems(ui.Center).Justify(ui.Center).Padding(24).
		Children(func() {
			ui.Column(c).Gap(8).MaxWidth(400).AlignItems(ui.Start).Children(func() {
				// The mark in a tile of its own, the accent's wash under it: the
				// empty window is the one screen that is all chrome, so it is the
				// one screen allowed a picture.
				tile := ui.Box(c).Size(44, 44).Radius(radius).
					AlignItems(ui.Center).Justify(ui.Center).
					Background(col.chosen).BorderWidth(1).BorderColor(t.Accent.Alpha(0.42))
				tile.Children(func() {
					markTile(c, 30)
				})
				ui.Box(c).Height(4)
				ui.Text(c, "还没有打开的终端").FontSize(15).FontWeight(600)
				ui.Text(c, w.emptyLine()).FontSize(12.5).TextColor(t.TextMuted)
				ui.Box(c).Height(6)
				ui.Row(c).Gap(8).Children(func() {
					new := ui.PrimaryButton(c, "").
						Height(headerButtonSize).PaddingX(12).Gap(7).
						Label("新建终端").Tooltip("新建终端 ⌘T")
					new.Children(func() {
						ui.Text(c, "新建终端").FontSize(12).FontWeight(600)
						kbdChip(c, "⌘T", true)
					})
					if new.Clicked() {
						w.newTab()
					}
					if ui.Button(c, "设置…").Clicked() {
						w.openSettings()
					}
				})
			})
		})
}

// emptyLine is what the empty window says about the machine it would open a
// shell on, so the sentence is true rather than generic.
func (w *window) emptyLine() string {
	machine := w.selectedMachine()
	if w.fleet == nil {
		return "本机服务已关闭。打开设置可以把它重新起起来。"
	}
	if machine != machineLocal {
		return w.machineName(machine) + " 上还没有会话。新建一个，或在别的客户端上开一个再来看。"
	}
	if n := len(w.sessionEntries()); n > 0 {
		return "左侧有 " + strconv.Itoa(n) + " 个会话。选一个，或者新建一个终端。"
	}
	return "本机开一个新终端，或从另一台机器接管一个。"
}

// statusBar is the quiet line along the bottom: the state of the local service,
// where the shell on screen runs, what it is, and what just happened.
//
// The connection is a chip — a dot in a rounded wash — the way the browser
// client states it, so "is anything attached to this machine" reads the same in
// both, and the rest is the same three facts in the same order.
func (w *window) statusBar(c *ui.Context) {
	t := c.Theme()
	col := shellColorsFor(t)

	ui.Row(c).
		Height(statusHeight).PaddingX(12).Gap(14).
		AlignItems(ui.Center).
		Background(col.pane).
		BorderWidth(1, 0, 0, 0).BorderColor(col.hairline).
		Children(func() {
			// The chip's wash is the state's own colour at low strength: green
			// while a client is attached, amber while the service is listening
			// for one, quiet while there is no service at all.
			state := w.fleetColor(t)
			chip := ui.Row(c).Gap(6).AlignItems(ui.Center).
				PaddingX(8).Height(19).Radius(radiusPill).
				Background(state.Alpha(0.10)).
				BorderWidth(1).BorderColor(state.Alpha(0.38)).
				Tooltip(w.serviceLine() + " · " + buildLine())
			chip.Children(func() {
				ui.Box(c).Size(6, 6).Radius(3).Background(state)
				ui.Text(c, w.serviceLine()).FontSize(11).TextColor(state)
			})
			// The machine the shell on screen runs on. Its platform is on the
			// same line, and the build's own platform is not repeated beside it:
			// on this machine's own shells the two are the same string, and a
			// status bar that says darwin/arm64 twice says it once too often.
			// Which build this is lives in the chip's tooltip and in 设置 → 更新.
			if host := w.statusHostLine(); host != "" {
				ui.Text(c, host).FontSize(11).TextColor(col.quiet)
			}
			ui.Spacer(c)
			// The window's own news, when there is any: a machine that took its
			// shell back, or a lease given up, is something the person here has to
			// be told, and the line the fleet already lives on is where they look.
			if n := w.currentNotice(); n != "" {
				ui.Text(c, n).FontSize(11).TextColor(t.Warning).MaxWidth(460).SingleLine()
			}
			// The shell on screen itself, at the end of the line, which is where
			// the browser client keeps it.
			if sess := w.statusSessionLine(); sess != "" {
				ui.Text(c, sess).FontSize(11).TextColor(t.TextMuted)
			}
		})
}

// buildLine is the version and the platform, the way a status bar says it.
func buildLine() string {
	if v := strings.TrimSpace(mygo.App.Version()); v != "" {
		return "v" + v + " · " + runtime.GOOS + "/" + runtime.GOARCH
	}
	return runtime.GOOS + "/" + runtime.GOARCH
}
