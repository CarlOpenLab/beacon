package main

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/tunnel"
)

// The column's measurements: what it is wide, what its panels ask for.
const (
	// sidebarWidth is the browser client's --w-side. The two clients list the
	// same shells in the same order with the same badges, so the column they sit
	// in is the same width too.
	sidebarWidth = 288

	// The room the machines' panel needs before the shells' panel takes the rest
	// of the column, and the least the shells' panel will accept. The line
	// between the panels starts there and is dragged from there.
	minSessionPanel = 140

	// machinePanelBase is the panel's own room: its title row, this machine, the
	// relay, and the note under them.
	machinePanelBase = 120
	// machineRowHeight is what one machine adds to the machines' panel, and
	// machinePanelMost is the most that panel will reserve before the shells take
	// the rest: a long roster scrolls rather than pushing the shells out of the
	// column.
	machineRowHeight = 46
	machinePanelMost = 300

	// heldLineHeight is the room one line of warning under this machine's row
	// takes: the height of an 11pt text line and the gap above it, so the note
	// about shells other clients are driving does not clip.
	heldLineHeight = 18

	// panelButtonSize is the square of the icon buttons in a panel's title row.
	// The strip's are bigger; a panel's title row is a thin thing.
	panelButtonSize = 22
)

// sidebar draws the column beside the strip: the shells on top, the machines
// under them, with a draggable line between. It is rebuilt from the window's
// own state on every frame, so a shell that appears — started here, or taken
// over from another machine later — shows up as soon as its tab does.
func (w *window) sidebar(c *ui.Context, mode string) {
	t := c.Theme()
	col := shellColorsFor(t)

	ui.Column(c).
		Width(sidebarWidth).FillHeight().
		Background(col.side).
		BorderWidth(0, 1, 0, 0).BorderColor(col.hairline).
		Children(func() {
			// The column fills the window, so the window's height is the room
			// the two panels share. Until the line between them is dragged the
			// machines take what their content needs — this machine, the relay,
			// and a row per machine the roster knows, with its shells — and the
			// shells take the rest, so a fleet's panel is never a sliver.
			if w.sidebarSplit < 0 {
				_, height := c.Size()
				need := min(w.machinePanelNeed(), machinePanelMost)
				w.sidebarSplit = max(minSessionPanel, height-need)
			}
			ui.SplitVertical(c, &w.sidebarSplit, func() {
				w.sessionPanel(c)
			}, func() {
				w.machinePanel(c)
			})
		})
}

// sessionPanel is the top half of the column: what this machine is running, and
// the two things a shell needs — one more, and an end to this one. The controls
// sit beside the title rather than across the window, because this is the panel
// they act on.
func (w *window) sessionPanel(c *ui.Context) {
	t := c.Theme()
	machine := w.selectedMachine()
	remote := machine != machineLocal
	entries := w.sessionEntries()

	ui.Column(c).Fill().Children(func() {
		// The title counts what the panel is showing, and the two controls act on
		// the machine it is showing: 新建 opens a shell there, and the × closes
		// the tab watching it. A panel whose buttons quietly applied to a
		// different machine than its list would be the worst of both.
		//
		// The × closes the tab and only the tab: the shell keeps running, and
		// ending it is the row's own 结束 button. The two used to be the same
		// button, which meant "close this tab" killed a process on a machine
		// somewhere — for every client watching it, not just this window.
		panelHead(c, "会话", len(entries), func() {
			newTip := "在本机开一个新终端"
			if remote {
				newTip = "在 " + w.machineName(machine) + " 上开一个新终端"
			}
			if iconButton(c, iconPlus, "新建标签页", panelButtonSize).Tooltip(newTip).Clicked() {
				w.newTab()
			}
			if iconButton(c, iconClose, "关闭标签页", panelButtonSize).
				Tooltip("关闭标签页（仅停止观看，不结束进程）").Clicked() {
				w.removeTab(w.activeTab())
			}
		})

		if len(entries) == 0 {
			col := shellColorsFor(t)
			ui.Column(c).Gap(4).Padding(14, 10).Children(func() {
				if remote {
					// Saying nothing here would leave a machine that is running
					// nothing looking like a machine that cannot be read.
					ui.Text(c, "这台机器上还没有会话").FontSize(12.5).TextColor(col.quiet)
					ui.Text(c, "点标题行的 ＋ 在它上面开一个。").FontSize(11).TextColor(col.quiet)
					return
				}
				ui.Text(c, "主机上还没有会话").FontSize(12.5).TextColor(col.quiet)
				ui.Text(c, "点标题行的 ＋，或直接 ⌘T。").FontSize(11).TextColor(col.quiet)
			})
			return
		}

		ui.Scroll(c).Fill().Children(func() {
			ui.Column(c).Gap(2).Padding(0, 8, 8, 8).Children(func() {
				for _, e := range entries {
					w.sessionRow(c, e)
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
// The row is the browser client's: the shell's name on the first line with a
// badge when something is wrong with it — 只读, 已退出 — and the facts on the
// second in one quiet line. The name is what a person scans for, so nothing else
// shares its line; the numbers are what tells two look-alike shells apart, so
// they share theirs.
func (w *window) sessionRow(c *ui.Context, e sessionEntry) {
	t := c.Theme()
	col := shellColorsFor(t)

	row := ui.Column(c).Gap(2).Padding(7, 9).Radius(radius).BorderWidth(1)
	if e.active {
		// The shell on screen is the one row that is chosen, and it is chosen the
		// way the browser client chooses it: the accent's wash, and a line of the
		// same accent around it.
		row.Background(col.chosen).BorderColor(t.Accent.Alpha(0.42))
	} else if row.Hovered() {
		row.Background(col.field)
	}
	row.Tooltip(w.sessionTooltip(e))
	row.Children(func() {
		ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(7, 7).Radius(3.5).
				Background(w.leaseColor(t, e.machine, e.session, e.holder, e.alive))
			name := ui.Text(c, e.label).FontSize(12.5)
			if e.active {
				name.TextColor(t.Text)
			}
			if badge, color, ok := w.leaseBadge(t, e); ok {
				badgeChip(c, badge, color)
			}
			// 结束, on the row it ends. It is the one action here that kills a
			// process, so it sits apart from picking the row — a click anywhere
			// else on the row selects it — and it says what it does in its
			// tooltip, because the word alone does not distinguish it from
			// 关闭标签页.
			//
			// It stays drawn rather than appearing under the pointer: the browser
			// client can afford a button that shows up on hover because a pointer
			// is how a browser is used, and this window is not always used that
			// way — and a control that is not on screen is a control nobody looks
			// for. It is quiet instead: a hairline of the state's colour, no wash
			// until the pointer is on it.
			if e.alive {
				ui.Spacer(c)
				kill := ui.Box(c).
					Width(38).Height(20).Radius(radiusSmall).
					AlignItems(ui.Center).Justify(ui.Center).
					Label("结束会话").Tooltip("结束会话：终止这台机器上的进程，对所有客户端生效（不会关闭标签）")
				kill.BorderWidth(1).BorderColor(t.Danger.Alpha(0.35))
				if kill.Hovered() {
					kill.Background(t.Danger.Alpha(0.14))
				}
				kill.Children(func() {
					ui.Text(c, "结束").FontSize(10.5).TextColor(t.Danger)
				})
				if kill.Clicked() {
					e.end()
				}
			}
		})
		ui.Text(c, w.sessionMeta(e)).FontSize(10.5).TextColor(col.quiet).SingleLine()
	})
	if row.Clicked() {
		e.pick()
	}
}

// sessionMeta is the row's second line: the shell's id, its size, and what it is
// to this window — 控制中, 只读 with the name of whoever holds it, or 已退出. It
// is the browser client's line, field for field, so the same shell reads the
// same in both clients.
func (w *window) sessionMeta(e sessionEntry) string {
	parts := []string{or(e.session, "待创建")}
	if e.meta != "" {
		parts = append(parts, e.meta)
	}
	switch {
	case !e.alive:
		parts = append(parts, "已退出")
	case e.holder != "":
		parts = append(parts, "只读 "+e.holder)
	default:
		parts = append(parts, "控制中")
	}
	return strings.Join(parts, " · ")
}

// leaseBadge is the row's state in words, for the states worth a badge: the two
// that mean the person at the keyboard cannot type, or that there is nothing
// left to type into. 控制中 is the ordinary case and wears no badge — a row that
// badges everything says nothing. The colour is the same one the dot beside it
// already uses, so the two never disagree.
func (w *window) leaseBadge(t *ui.Theme, e sessionEntry) (string, ui.Color, bool) {
	if tb := w.tabOf(e.machine, e.session); tb != nil {
		if !tb.alive {
			return "已退出", t.Danger, true
		}
		if tb.held() {
			return "", t.Success, false
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

// badgeChip is a state in a small outlined chip: the browser client's badge, on
// the ui's terms. It is a word in the state's own colour inside a hairline of the
// same colour and nothing else — a state that is worth saying is worth saying
// quietly, and a row wearing a wash for every state is a row nobody can scan.
func badgeChip(c *ui.Context, text string, color ui.Color) {
	chip := ui.Row(c).Gap(4).AlignItems(ui.Center).
		PaddingX(6).Height(16).Radius(radiusSmall).
		BorderWidth(1).BorderColor(color.Alpha(0.42))
	chip.Children(func() {
		ui.Text(c, text).FontSize(10).TextColor(color)
	})
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

// tabDigits are the keys ⌘1…⌘9 answer to, in the order the numbers run. They are
// the browser client's own shortcut, and the shortcut list the empty window
// prints promises them, so the window has to keep the promise.
var tabDigits = []ui.Key{
	ui.Key1, ui.Key2, ui.Key3, ui.Key4, ui.Key5,
	ui.Key6, ui.Key7, ui.Key8, ui.Key9,
}

// jumpTab puts the nth tab on screen, counting from zero. A number past the last
// tab does nothing rather than wrapping: a person pressing ⌘7 with three shells
// open is asking for a tab that is not there, and the nearest one is not what
// they asked for.
func (w *window) jumpTab(n int) {
	if n < 0 || n >= len(w.order) {
		return
	}
	if tb := w.tabs[w.order[n]]; tb != nil {
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

// machinePanelNeed is how much room the machines' panel's content asks for:
// its own base, plus a row per machine the roster knows and a row under each for
// the shells that machine is running.
func (w *window) machinePanelNeed() float32 {
	need := float32(machinePanelBase)
	// The line about shells other clients are driving is one more line under this
	// machine's row, and the panel has to ask for the room to draw it.
	if w.localHeldByOthers() > 0 {
		need += heldLineHeight
	}
	for _, m := range w.roster {
		if w.ownMachine(m) {
			continue
		}
		need += machineRowHeight
	}
	return need
}

// machinePanel is the machines this window can reach, as a list you switch
// between: this machine first, then every other machine on the relay. The row
// that is picked is the machine the 会话 panel below is about, and the one its
// 新建标签页 opens a shell on — so driving another machine is a matter of picking
// it, not of remembering which row belongs to whom.
func (w *window) machinePanel(c *ui.Context) {
	t := c.Theme()

	ui.Column(c).Fill().Children(func() {
		panelHead(c, "机器", w.machineCount(), nil)

		ui.Scroll(c).Fill().Children(func() {
			ui.Column(c).Gap(2).Padding(0, 8, 8, 8).Children(func() {
				// This machine, whatever else the fleet holds: the dot is the
				// service's state, as the strip's is, and the row is where a person
				// looks to see which machine they are on.
				w.machineChoice(c, w.localEntry())

				// A shell on this machine that someone else is driving is the next
				// thing to say: the person at the keyboard is about to find out
				// anyway, by typing into a shell that does not answer.
				if w.localHeldByOthers() > 0 && w.selectedMachine() == machineLocal {
					ui.Text(c, fmt.Sprintf("%d 个会话正被其他客户端控制", w.localHeldByOthers())).
						FontSize(11).TextColor(t.Warning)
				}

				for _, m := range w.roster {
					if w.ownMachine(m) {
						continue
					}
					w.machineChoice(c, m)
				}

				if w.linkNote != "" {
					ui.Text(c, w.linkNote).FontSize(11).TextColor(t.Warning)
				}
				ui.Text(c, w.machineNote()).FontSize(11).TextColor(t.TextMuted)
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

// machineChoice is one machine in the panel. Picking a machine is what this
// panel is for, so the row a person is driving is the row that looks picked: a
// left bar in the accent, a raised ground, and the hover a plain row has.
func (w *window) machineChoice(c *ui.Context, m tunnel.Machine) {
	t := c.Theme()
	col := shellColorsFor(t)
	local := m.ID == machineLocal
	selected := w.selectedMachine() == m.ID

	dot := relayColor(t, m.Online, false)
	meta := m.Host.Platform + "/" + m.Host.Arch
	tooltip := m.ID + " · " + machineState(m)
	if local {
		dot = w.fleetColor(t)
		_, meta = w.localMachine()
		tooltip = w.serviceLine()
	}

	row := ui.Column(c).Gap(2).Padding(7, 9).Radius(radius).BorderWidth(1)
	if selected {
		row.Background(col.chosen).BorderColor(t.Accent.Alpha(0.42))
	} else if row.Hovered() {
		row.Background(col.field)
	}
	row.Tooltip(tooltip).Children(func() {
		ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(8, 8).Radius(4).Background(dot)
			ui.Text(c, m.Name).FontSize(12.5)
			if !m.Online {
				badgeChip(c, "离线", t.TextMuted)
			}
		})
		ui.Text(c, meta).FontSize(10.5).TextColor(col.quiet).SingleLine()
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

// leaseColor is a shell's dot: the same three states the row's words and badge
// name, in one colour each. Green is the ordinary case — this window drives the
// shell, or nobody is named and the machine in front of the PTY has it. Grey is
// a shell another client is typing into: a state, not a fault, and the one the
// pane's 只读 chip and the composer's placeholder already say in words. Red is a
// shell that is gone.
//
// Amber is deliberately not among them. It is the read-only colour on the chip
// and the badge, and a dot that also carried it would be the second place saying
// the same thing in a different vocabulary.
func (w *window) leaseColor(t *ui.Theme, machine, sessionID, controller string, alive bool) ui.Color {
	if !alive {
		return t.Danger
	}
	if tb := w.tabOf(machine, sessionID); tb != nil && tb.held() {
		return t.Success
	}
	if controller != "" {
		return shellColorsFor(t).quiet
	}
	return t.Success
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

// relayOn reports whether this machine is enrolled with a relay. A window with
// no fleet has no relay, which is the same answer as one with the link off.
func (w *window) relayOn() bool {
	return w.fleet != nil && w.fleet.RelayOn()
}

// machineNote is the line under the machines: what the panel is for, and what is
// stopping it from being more than this machine. It is the honest answer to
// "where are my other machines?".
func (w *window) machineNote() string {
	if !w.relayOn() {
		return "只列本机：在 设置 → 远程 接入中继后，同一中继上的其他机器会出现在这里。"
	}
	if w.fleet.RelayClient() == nil {
		return "点另一台机器会向中继申请一枚短时接管令牌；中继没发下来的原因在 设置 → 远程。"
	}
	return "点一台机器就切过去，它会成为上面「会话」的对象。"
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

// relayColor is the relay row's dot: green once the relay has welcomed this
// member, amber while the link is being established, and quiet when it is down
// — the same three answers the strip's service dot gives.
func relayColor(t *ui.Theme, connected, connecting bool) ui.Color {
	switch {
	case connected:
		return t.Success
	case connecting:
		return t.Warning
	}
	return shellColorsFor(t).quiet
}

// panelHead is a panel's title row: the name, a count of what it holds, and
// whatever controls the panel needs on the right. The name is a small quiet
// capital — the browser client's .side-title — so the panels read as headings
// over the rows rather than as rows themselves.
func panelHead(c *ui.Context, title string, count int, controls func()) {
	t := c.Theme()
	col := shellColorsFor(t)
	ui.Row(c).
		Height(34).PaddingX(10).Gap(7).
		AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(col.hairline).
		Children(func() {
			ui.Text(c, title).FontSize(11).FontWeight(600).LetterSpacing(0.4).TextColor(col.quiet)
			count2 := ui.Box(c).Height(17).PaddingX(7).Radius(radiusPill).
				AlignItems(ui.Center).Justify(ui.Center).
				Background(col.field).BorderWidth(1).BorderColor(col.line)
			count2.Children(func() {
				ui.Text(c, strconv.Itoa(count)).FontSize(10.5).TextColor(t.TextMuted)
			})
			ui.Spacer(c)
			if controls != nil {
				controls()
			}
		})
}
