package main

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/tunnel"
)

const sidebarWidth = 224

func (w *window) sidebar(c *ui.Context, _ string) {
	col := shellColorsFor(c.Theme())
	ui.Column(c).Width(sidebarWidth).FillHeight().Background(col.pane).
		BorderWidth(0, 1, 0, 0).BorderColor(col.hairline).Children(func() {
		ui.Row(c).Height(54).PaddingX(18).Gap(10).AlignItems(ui.Center).Children(func() {
			markTile(c, 24)
			ui.Text(c, "term-fleet").FontSize(14).FontWeight(550)
		})
		w.sessionPanel(c)
		ui.Box(c).Height(1).FillWidth().Background(col.hairline)
		w.machinePanel(c)
		ui.Box(c).Height(1).FillWidth().Background(col.hairline)
		settings := ui.Row(c).Height(42).PaddingX(12).Gap(9).AlignItems(ui.Center).FillWidth().Label("设置 ⌘,")
		if settings.Hovered() {
			settings.Background(c.Theme().SurfaceHover)
		}
		settings.Children(func() {
			ui.Icon(c, iconSettings).FontSize(15).TextColor(c.Theme().TextMuted)
			ui.Text(c, "设置").FontSize(12).TextColor(c.Theme().TextMuted)
		})
		if settings.Clicked() {
			w.openSettings()
		}
	})
}

func markTile(c *ui.Context, size float32) {
	if mark != nil {
		ui.Image(c, mark).Size(size, size).Radius(size * .28).Label("term-fleet")
	}
}

func (w *window) sessionPanel(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Height(34).PaddingX(14).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "会话").FontSize(11).FontWeight(600).TextColor(t.TextMuted)
			ui.Spacer(c)
			if iconButton(c, iconPlus, "新建会话 ⌘T", 30).Clicked() {
				w.newTab()
			}
		})
		entries := w.sessionEntries()
		if len(entries) == 0 {
			ui.Text(c, "还没有会话").FontSize(11).TextColor(t.TextMuted).Padding(14, 14)
			return
		}
		ui.Scroll(c).Fill().Children(func() {
			ui.Column(c).Gap(3).Padding(4, 8, 10, 8).Children(func() {
				for _, entry := range entries {
					w.sessionRow(c, entry)
				}
			})
		})
	})
}

// sessionEntry is one row of the 会话 panel: what it is called, what it is, and
// what picking it does. The panel is built from these whether the shells belong
// to this machine or to another one, so the two cases cannot drift apart.
type sessionEntry struct {
	machine string
	session string
	label   string
	meta    string
	// holder is who else holds the shell, for the row's mark: it is what says a
	// shell is being driven from elsewhere without opening it.
	holder string
	active bool
	// alive is whether the shell is still running, which is what decides whether
	// the row offers 结束: there is nothing to end once it has exited.
	alive bool
	pick  func()
	// end kills the process behind this shell, for every client. It is the row's
	// own action, separate from picking the row and from closing the tab.
	end func()
}

// sessionEntries is the panel's list: this window's tabs when the panel is on
// this machine, and the shells the picked machine is running when it is on
// another one.
func (w *window) sessionEntries() []sessionEntry {
	machine := w.selectedMachine()
	if machine == machineLocal {
		out := make([]sessionEntry, 0, len(w.order))
		for _, key := range w.order {
			tb := w.tabs[key]
			if tb == nil || tb.machine != machineLocal {
				// A shell on another machine is a row of *that* machine's panel:
				// listing it here would put a row that moves the panels to another
				// machine in the middle of this machine's shells.
				continue
			}
			tab := tb
			out = append(out, sessionEntry{
				machine: machineLocal,
				session: tab.id,
				label:   tab.label(),
				meta:    tab.size(),
				holder:  tab.holder(),
				active:  tab == w.activeTab(),
				alive:   tab.alive,
				pick:    func() { w.selectTab(tab) },
				end:     func() { w.kill(tab) },
			})
		}
		return out
	}

	list := w.remoteSessions[machine]
	out := make([]sessionEntry, 0, len(list))
	for _, s := range list {
		summary := s
		name := summary.Title
		if name == "" {
			name = filepath.Base(summary.Cwd)
		}
		if name == "" || name == "." || name == "/" {
			name = summary.SessionID
		}
		// A shell the window already has a tab for is on screen when that tab is
		// the tab on screen: picking it is switching to it, not opening a second
		// tab, which is what a tab list is for.
		tab := w.tabOf(machine, summary.SessionID)
		out = append(out, sessionEntry{
			machine: machine,
			session: summary.SessionID,
			label:   name,
			meta:    fmt.Sprintf("%dx%d", summary.Cols, summary.Rows),
			holder:  summary.Controller,
			active:  tab != nil && tab == w.activeTab(),
			alive:   summary.Alive,
			pick:    func() { w.openOrSelect(machine, summary.SessionID) },
			end:     func() { w.killSessionOn(machine, summary.SessionID) },
		})
	}
	return out
}

// sessionRow is one shell in the 会话 panel, drawn the same way whether it runs
// on this machine or another one.
//
// The row carries a badge naming its state — 控制中 / 只读 — rather than a bare
// dot and a holder's name in the corner: the browser client's rows read that
// way, and a badge says the same thing without needing the colour to be
// distinguishable. The meta line keeps only the terminal size, which is the one
// number that tells two look-alike shells apart; the rest is in the tooltip.
func (w *window) sessionRow(c *ui.Context, e sessionEntry) {
	t := c.Theme()
	row := ui.Column(c).Gap(2).Padding(5, 9).Radius(4).FillWidth()
	if e.active {
		row.Background(t.SurfacePressed)
	} else if row.Hovered() {
		row.Background(t.SurfaceHover)
	}
	row.Label(e.label).Tooltip(w.sessionTooltip(e)).ContextMenu(func(m *ui.Menu) {
		if m.Item("关闭标签页").Chosen() {
			if tb := w.tabOf(e.machine, e.session); tb != nil {
				w.removeTab(tb)
			}
		}
		if e.alive && m.Item("结束进程…").Chosen() {
			w.endPrompt = &e
		}
		if tb := w.tabOf(e.machine, e.session); tb != nil && tb.alive && tb.held() && m.Item("释放控制权").Chosen() {
			w.releaseLease(tb)
		}
	})
	row.Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			mark := shellColorsFor(t).quiet
			if e.holder != "" {
				mark = ui.Hex("#d3aa62")
			}
			if tb := w.tabOf(e.machine, e.session); tb != nil && tb.held() {
				mark = ui.Hex("#72b67b")
			}
			ui.Box(c).Size(6, 6).Radius(3).Background(mark)
			ui.Text(c, e.label).FontSize(12).FillWidth().NoWrap().Ellipsis("…")
		})
	})
	if row.Clicked() {
		e.pick()
	}
}

// leaseBadge is the row's state in words: what this window holds, what someone
// else holds, or nothing at all when the shell is unclaimed. The colour is the
// same one the dot beside it already uses, so the two never disagree.
func (w *window) leaseBadge(t *ui.Theme, e sessionEntry) (string, ui.Color, bool) {
	if tb := w.tabOf(e.machine, e.session); tb != nil {
		if !tb.alive {
			return "已退出", shellColorsFor(t).quiet, true
		}
		if tb.held() {
			return "控制中", t.Success, true
		}
		if e.holder != "" {
			return "只读", t.Warning, true
		}
		return "", t.Success, false
	}
	if e.holder != "" {
		return "只读", t.Warning, true
	}
	return "", t.Success, false
}

// sessionTooltip is the row's full story, the numbers the row itself has no room
// for: the session id, where it runs, its size, and who holds it.
func (w *window) sessionTooltip(e sessionEntry) string {
	lines := []string{w.machineName(e.machine), e.session}
	if e.meta != "" {
		lines = append(lines, e.meta)
	}
	if e.holder != "" {
		lines = append(lines, "控制端 "+e.holder)
	} else {
		lines = append(lines, "当前无人控制")
	}
	return strings.Join(lines, "\n")
}

// newestTabOn is the most recently opened shell this window is watching on a
// machine, or nil when it has none there.
func (w *window) newestTabOn(machine string) *tab {
	for i := len(w.order) - 1; i >= 0; i-- {
		tb := w.tabs[w.order[i]]
		if tb != nil && tb.machine == machine {
			return tb
		}
	}
	return nil
}

// selectTab puts a tab on screen, and with it the machine the panels are about:
// the 机器 and 会话 panels follow the shell a person is looking at, so switching
// to a shell on another machine moves them there rather than leaving them on a
// machine that is no longer on screen.
func (w *window) selectTab(tb *tab) {
	for i, key := range w.order {
		if w.tabs[key] != tb {
			continue
		}
		w.selected = i
		if tb.machine != "" {
			w.machineSel = tb.machine
		}
		w.invalidate()
		return
	}
}

// openOrSelect shows the tab watching a shell, or opens one. Clicking a shell
// that is already open has to switch to it: a list that only reacts the first
// time a shell is picked is not a list of tabs.
func (w *window) openOrSelect(machine, sessionID string) {
	if tb := w.tabOf(machine, sessionID); tb != nil {
		w.selectTab(tb)
		return
	}
	if machine == machineLocal {
		// A shell of this machine's always has a tab: the session list mounts one.
		return
	}
	if w.openTab(machine, sessionID) == nil {
		w.linkNote = w.noLinkReason(machine)
	}
}

// stepTab moves the shell on screen to the next or previous tab this window has
// open, whichever machine each one runs on — what a tab strip's own keys do, and
// the reason the panel's rows are not the only way between shells.
func (w *window) stepTab(delta int) {
	if len(w.order) < 2 {
		return
	}
	on := w.activeTab()
	at := 0
	for i, key := range w.order {
		if w.tabs[key] == on {
			at = i
			break
		}
	}
	next := (at + delta) % len(w.order)
	if next < 0 {
		next += len(w.order)
	}
	if tb := w.tabs[w.order[next]]; tb != nil {
		w.selectTab(tb)
	}
}

// ownMachine reports whether a roster entry is this machine. A roster is keyed
// by the device id a machine was enrolled under, while the window knows itself
// by hostname, so both are checked: without the device one, a fleet panel lists
// the machine you are sitting at as another machine — and offers to open its own
// shells over the relay.
func (w *window) ownMachine(m tunnel.Machine) bool {
	if m.ID == machineLocal {
		return true
	}
	if w.fleet == nil {
		return false
	}
	if m.ID == w.fleet.Host().Hostname {
		return true
	}
	_, device := w.fleet.RelayIdentity()
	return device != "" && m.ID == device
}

func (w *window) machinePanel(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Height(126).Children(func() {
		ui.Row(c).Height(32).PaddingX(14).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "机器").FontSize(11).FontWeight(600).TextColor(t.TextMuted)
			ui.Spacer(c)
			ui.Text(c, strconv.Itoa(w.machineCount())).FontSize(10).TextColor(t.TextMuted)
		})
		ui.Scroll(c).Fill().Children(func() {
			ui.Column(c).Gap(2).Padding(2, 8, 8, 8).Children(func() {
				w.machineChoice(c, w.localEntry())
				for _, m := range w.roster {
					if !w.ownMachine(m) {
						w.machineChoice(c, m)
					}
				}
			})
		})
	})
}

// localEntry is this machine as the panel lists it: the same shape as a roster
// entry, so one row draws both and nothing about the first row is special.
func (w *window) localEntry() tunnel.Machine {
	name, _ := w.localMachine()
	entry := tunnel.Machine{ID: machineLocal, Name: name, Online: true}
	if w.fleet != nil {
		entry.Host = w.fleet.Host()
	}
	return entry
}

func (w *window) machineChoice(c *ui.Context, m tunnel.Machine) {
	t := c.Theme()
	selected := w.selectedMachine() == m.ID
	row := ui.Row(c).Height(32).PaddingX(9).Gap(8).AlignItems(ui.Center).Radius(4).FillWidth().Tooltip(m.ID + " · " + machineState(m))
	if selected {
		row.Background(t.SurfacePressed)
	} else if row.Hovered() {
		row.Background(t.SurfaceHover)
	}
	row.Children(func() {
		ui.Row(c).Gap(9).AlignItems(ui.Center).Children(func() {
			mark := shellColorsFor(t).quiet
			if m.Online {
				mark = ui.Hex("#72b67b")
			}
			ui.Box(c).Size(7, 7).Radius(3.5).Background(mark)
			name := m.Name
			if len([]rune(name)) > 21 {
				name = string([]rune(name)[:20]) + "…"
			}
			ui.Text(c, name).FontSize(12).FillWidth().NoWrap().Ellipsis("…")
			if m.ID != machineLocal && !m.Online {
				ui.Text(c, "离线").FontSize(10).TextColor(t.TextMuted)
			}
		})
	})
	if row.Clicked() {
		w.selectMachine(m)
	}
}

// machineByID is a roster entry, or this machine's when asked for it.
func (w *window) machineByID(id string) (tunnel.Machine, bool) {
	if id == machineLocal {
		return w.localEntry(), true
	}
	for _, m := range w.roster {
		if m.ID == id {
			return m, true
		}
	}
	if w.fleet != nil {
		if c := w.fleet.RelayClient(); c != nil {
			if m, ok := c.Machine(id); ok {
				return m, true
			}
		}
	}
	return tunnel.Machine{}, false
}

// machineName is what the panels call a machine: its own name, or the id when
// the roster has never heard of it.
func (w *window) machineName(id string) string {
	if m, ok := w.machineByID(id); ok && m.Name != "" {
		return m.Name
	}
	if id == machineLocal {
		return "本机"
	}
	return id
}

// lookAtMachine opens the window's link to a machine and asks what it runs. A
// second look re-uses the stream, so the panel can be looked at freely. It is
// what picking a machine does; a machine the window cannot reach says why, in
// the panel, because a click that does nothing is the one thing a fleet panel
// must never do.
func (w *window) lookAtMachine(machine string) {
	w.ensureSessionList(machine)
}

// machineState is what a roster machine's tooltip says about it.
func machineState(m tunnel.Machine) string {
	if m.Online {
		return "在线"
	}
	return "离线"
}

// localMachine is this machine's row: its name, and the line under it. A window
// with no fleet behind it — a test's, or one whose fleet is gone — still says
// what it is rather than drawing an empty row.
func (w *window) localMachine() (name, meta string) {
	host := w.hostName()
	if host == "" {
		host = "本机"
	}
	if w.fleet == nil {
		return host, "平台未知"
	}
	h := w.fleet.Host()
	return host, h.Platform + "/" + h.Arch + " · " + fmt.Sprintf("%d 个会话", len(w.order))
}

// hostName is what this machine calls itself.
func (w *window) hostName() string {
	if w.fleet == nil {
		return ""
	}
	return w.fleet.Host().Hostname
}

// machineCount is what the panel's title counts: this machine, and every other
// machine the roster says is online. The relay row is not a machine — it is what
// makes the fleet — so it never counts.
func (w *window) machineCount() int {
	n := 1
	for _, m := range w.roster {
		if m.Online && !w.ownMachine(m) {
			n++
		}
	}
	return n
}

// panelHead is a panel's title row: the name, a count of what it holds, and
// whatever controls the panel needs on the right.
func panelHead(c *ui.Context, title string, count int, controls func()) {
	t := c.Theme()
	col := shellColorsFor(t)
	ui.Row(c).
		Height(30).PaddingX(10).PaddingY(4).Gap(6).
		AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(col.hairline).
		Children(func() {
			ui.Text(c, title).FontSize(11).FontWeight(600).TextColor(t.TextMuted)
			ui.Text(c, strconv.Itoa(count)).FontSize(11).TextColor(t.TextMuted)
			ui.Spacer(c)
			if controls != nil {
				controls()
			}
		})
}
