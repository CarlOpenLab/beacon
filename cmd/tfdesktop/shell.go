package main

import (
	_ "embed"
	"fmt"
	"github.com/CarlOpenLab/term-fleet/internal/protocol"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
	"strconv"
	"strings"
)

//go:embed assets/mark.png
var markPNG []byte

var mark = decodeMark()

func decodeMark() *ui.Bitmap {
	bmp, err := ui.DecodeBitmap(markPNG)
	if err != nil {
		return nil
	}
	return bmp
}

func iconButton(c *ui.Context, icon *ui.SVG, caption string, size float32) ui.Element {
	el := ui.Box(c).Size(size, size).Radius(4).AlignItems(ui.Center).Justify(ui.Center).Label(caption).Tooltip(caption)
	if el.Hovered() {
		el.Background(c.Theme().SurfaceHover)
	}
	el.Children(func() { ui.Icon(c, icon).FontSize(size * .55).TextColor(c.Theme().TextMuted) })
	return el
}

func (w *window) header(c *ui.Context, _ []string) {
	t := c.Theme()
	ui.Row(c).Height(headerHeight).PaddingX(14).Gap(8).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		if iconButton(c, iconSidebar, "显示或收起侧栏", 28).Clicked() {
			w.sidebarHidden = !w.sidebarHidden
		}
		machine := ui.Text(c, w.compactMachineName(w.selectedMachine())).FontSize(12).TextColor(t.TextMuted).NoWrap().Ellipsis("…")
		machine.MaxWidth(145)
		if tb := w.visibleTab(); tb != nil {
			ui.Text(c, "/").FontSize(12).TextColor(shellColorsFor(t).quiet)
			ui.Text(c, tb.label()).FontSize(12).FillWidth().NoWrap().Ellipsis("…")
			ui.Spacer(c)
			if tb.alive {
				// Less frequent actions live in a menu, not in the working surface.
				menu := iconButton(c, iconMore, "会话操作", 28)
				menu.ContextMenu(func(m *ui.Menu) {
					w.sessionMenu(m, sessionEntry{machine: tb.machine, session: tb.id, label: tb.label(), alive: tb.alive, end: func() { w.kill(tb) }})
				})
				if menu.Clicked() {
					w.leaseOpen = !w.leaseOpen
				}
				ui.Popover(c, menu, &w.leaseOpen, func() {
					if !tb.held() && ui.Button(c, "拿回控制权").Clicked() {
						w.reclaim(tb)
						w.leaseOpen = false
					}
					if tb.held() && ui.Button(c, "释放控制权").Clicked() {
						w.releaseLease(tb)
						w.leaseOpen = false
					}
					if ui.Button(c, "结束会话…").Clicked() {
						e := sessionEntry{label: tb.label(), end: func() { w.kill(tb) }}
						w.endPrompt = &e
						w.leaseOpen = false
					}
				})
			}
			if iconButton(c, iconClose, "关闭标签页", 28).Tooltip("关闭标签页，不结束进程 ⌘W").Clicked() {
				w.removeTab(tb)
			}
		} else {
			ui.Spacer(c)
		}
	})
}

func (w *window) compactMachineName(machine string) string {
	name := strings.TrimSuffix(w.machineName(machine), ".local")
	if len([]rune(name)) > 20 {
		return string([]rune(name)[:19]) + "…"
	}
	return name
}

// A selected machine with no open tab must never display another machine's shell.
func (w *window) visibleTab() *tab {
	tb := w.activeTab()
	if tb != nil && tb.machine == w.selectedMachine() {
		return tb
	}
	return nil
}

func (w *window) body(c *ui.Context, _ []string) {
	tb := w.visibleTab()
	if tb == nil {
		w.emptyState(c)
		return
	}
	ui.Column(c).Fill().Background(c.Theme().Background).Children(func() {
		if !tb.alive {
			w.panelNotice(c, "会话已退出，可以继续查看输出。")
		} else if !tb.held() {
			ui.Row(c).Height(40).PaddingX(20).Gap(12).AlignItems(ui.Center).Shrink(0).Background(c.Theme().Surface).Children(func() {
				ui.Text(c, "只读："+or(tb.holder(), "其他客户端")+" 正在操作这个终端。").FontSize(12).TextColor(c.Theme().TextMuted)
				ui.Spacer(c)
				if tb.alive && !tb.held() && ui.Button(c, "拿回控制权").Clicked() {
					w.reclaim(tb)
				}
			})
		}
		if tb.term != nil {
			ui.Box(c).Fill().Padding(22, 28).Children(func() { terminal.View(c, tb.term).Fill().AutoFocus() })
		} else {
			ui.Spacer(c)
		}
	})
}

func (w *window) panelNotice(c *ui.Context, message string) {
	ui.Row(c).Height(40).PaddingX(20).AlignItems(ui.Center).Shrink(0).Background(c.Theme().Surface).Children(func() {
		ui.Text(c, message).FontSize(12).TextColor(c.Theme().TextMuted)
	})
}

func (w *window) emptyState(c *ui.Context) {
	ui.Column(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Gap(12).Children(func() {
		markTile(c, 44)
		ui.Text(c, "还没有打开的终端").FontSize(14)
		ui.Text(c, w.emptyLine()).FontSize(12).TextColor(c.Theme().TextMuted)
		if m, ok := w.machineByID(w.selectedMachine()); (!ok || m.Online) && ui.Button(c, "新建终端").Clicked() {
			w.newTab()
		}
	})
}

func (w *window) statusBar(c *ui.Context) {
	ui.Row(c).Height(statusHeight).PaddingX(28).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Text(c, w.machineName(w.selectedMachine())).FontSize(10).TextColor(c.Theme().TextMuted)
		if n := w.currentNotice(); n != "" {
			ui.Text(c, n).FontSize(10).TextColor(c.Theme().TextMuted)
		}
		ui.Spacer(c)
		if tb := w.visibleTab(); tb != nil && tb.term != nil {
			cols, rows := tb.term.Size()
			ui.Text(c, fmt.Sprintf("%d × %d", cols, rows)).FontSize(10).TextColor(c.Theme().TextMuted)
		}
	})
}

func (w *window) endSessionDialog(c *ui.Context) {
	e := w.endPrompt
	if e == nil {
		return
	}
	open := true
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(360).Padding(24).Gap(16).Children(func() {
			ui.Text(c, "结束「"+e.label+"」？").FontSize(15)
			ui.Text(c, "进程将停止，所有客户端都会断开。关闭标签页可以保留进程。").FontSize(12).TextColor(c.Theme().TextMuted)
			ui.Row(c).Gap(10).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Clicked() {
					w.endPrompt = nil
				}
				if ui.PrimaryButton(c, "结束会话").Clicked() {
					if e.end != nil {
						e.end()
					}
					w.endPrompt = nil
				}
			})
		})
	})
	if !open {
		w.endPrompt = nil
	}
}

func (w *window) settingsDialog(c *ui.Context) {
	open := w.settingsOpen
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(680).Height(520).Background(c.Theme().Background).Children(func() {
			if w.settingsContent != nil {
				w.settingsContent.view(c)
			}
		})
	})
	if w.settingsOpen && !open {
		w.settingsOpen = false
	}
}

func (w *window) sessionMenu(m *ui.Menu, e sessionEntry) {
	if tb := w.tabOf(e.machine, e.session); tb != nil {
		if m.Item("关闭标签页").Chosen() {
			w.removeTab(tb)
		}
		if tb.alive && tb.held() && m.Item("释放控制权").Chosen() {
			w.releaseLease(tb)
		}
	}
	if e.alive && m.Item("结束会话…").Chosen() {
		w.endPrompt = &e
	}
}

// statusHostLine is where the shell on screen runs, and what that machine is:
// its name, its platform, and how many shells it has. The browser client's
// status bar says the same, so a person moving between the two reads one line
// either way.
func (w *window) relayOn() bool { return w.fleet != nil && w.fleet.RelayOn() }

func (w *window) serviceLine() string {
	if w.fleet == nil {
		return "本机服务已关闭"
	}
	if address := w.fleet.BaseURL(); address != "" {
		return "本机服务已开启 · " + address
	}
	return "本机服务已关闭"
}

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
