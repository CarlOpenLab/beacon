package main

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/protocol"
	"github.com/CarlOpenLab/term-fleet/internal/tunnel"
)

// The rail's selection is the id of the shell on screen, prefixed so that one
// field can carry what the column of shells asked for.
const (
	railTabPrefix = "tab:"

	// sidebarWidth fits a shell's label and where it is working.
	sidebarWidth = 216

	// The room the machines' panel needs before the shells' panel takes the rest
	// of the column, and the least the shells' panel will accept. The line
	// between the panels starts there and is dragged from there.
	minSessionPanel = 140

	// machinePanelBase is the panel's own room: its title row, this machine, the
	// relay, and the note under them.
	machinePanelBase = 120
	// machineRowHeight and sessionRowHeight are what one machine and one of its
	// shells add to the panel, and machinePanelMost is the most it will reserve
	// before the shells take the rest: a long roster scrolls rather than
	// pushing the shells out of the column.
	machineRowHeight = 46
	sessionRowHeight = 36
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
func (w *window) sidebar(c *ui.Context, labels []string, mode string) {
	t := c.Theme()
	col := shellColorsFor(t)

	// The column draws with the window's selection field and that field is never
	// seeded here: the click that changes the selection is written into the very
	// pointer the column was registered with, so overwriting it before the click
	// is read back would swallow the click. When the field names a shell that is
	// still open it *is* the window's idea of the shell on screen.
	if w.indexOfTab(strings.TrimPrefix(w.rail, railTabPrefix)) < 0 {
		w.rail = ""
		if id := w.activeID(); id != "" {
			w.rail = railTabPrefix + id
		}
	}

	ui.Column(c).
		Width(sidebarWidth).FillHeight().
		Background(col.pane).
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
				w.sessionPanel(c, labels, mode)
			}, func() {
				w.machinePanel(c)
			})
		})
}

// sessionPanel is the top half of the column: what this machine is running, and
// the two things a shell needs — one more, and an end to this one. The controls
// sit beside the title rather than across the window, because this is the panel
// they act on.
func (w *window) sessionPanel(c *ui.Context, labels []string, mode string) {
	t := c.Theme()
	machine := w.selectedMachine()
	remote := machine != machineLocal

	ui.Column(c).Fill().Children(func() {
		// The title counts what the panel is showing, and the two controls act on
		// the machine it is showing: 新建 opens a shell there, 关闭 ends the one on
		// screen. A panel whose buttons quietly applied to a different machine than
		// its list would be the worst of both.
		count := len(labels)
		if remote {
			count = len(w.remoteSessions[machine])
		}
		panelHead(c, "会话", count, func() {
			newTip := "在本机开一个新终端"
			if remote {
				newTip = "在 " + w.machineName(machine) + " 上开一个新终端"
			}
			if iconButton(c, iconPlus, "新建标签页", panelButtonSize).Tooltip(newTip).Clicked() {
				w.newTab()
			}
			if iconButton(c, iconClose, "关闭标签页", panelButtonSize).Clicked() {
				w.kill(w.activeTab())
			}
		})

		if remote {
			w.remoteSessionList(c, machine)
			return
		}

		rail := ui.Sidebar(c, &w.rail, func() {
			if len(labels) == 0 {
				ui.Text(c, "还没有终端").FontSize(12).TextColor(t.TextMuted)
				return
			}
			for i, key := range w.order {
				tb := w.tabs[key]
				if tb == nil {
					continue
				}
				label := tb.id
				if i < len(labels) {
					label = labels[i]
				}
				item := ui.SidebarItem(c, railTabPrefix+tb.id, iconTerminal, label)
				// A shell another client holds says so on its row, not only in
				// the pane: the strip is where a person looks for which tab is
				// being driven from elsewhere.
				if holder := tb.holder(); holder != "" {
					item.Children(func() { leaseTabMark(c, holder) })
				}
			}
		}).Fill()

		switch {
		case rail.Changed():
			w.railAction(c, mode)
		case w.rail != w.selectionID():
			// The shell on screen changed somewhere else — one was opened or closed, or
			// a shell was taken over — so the column follows it on the next frame.
			w.rail = w.selectionID()
		}
	})
}

// remoteSessionList is the shells another machine is running, as the 会话 panel
// shows them while that machine is the one picked: one row per shell, and
// clicking a row opens a tab here watching it.
func (w *window) remoteSessionList(c *ui.Context, machine string) {
	t := c.Theme()
	m, ok := w.machineByID(machine)
	if !ok {
		return
	}
	list := w.remoteSessions[machine]
	if len(list) == 0 {
		// Saying nothing here would leave a machine that is running nothing
		// looking like a machine that cannot be read.
		ui.Column(c).Gap(6).Padding(12, 10).Children(func() {
			ui.Text(c, "这台机器上没有活着的终端。").FontSize(12).TextColor(t.TextMuted)
			ui.Text(c, "点标题行的 ＋ 在它上面开一个。").FontSize(11).TextColor(t.TextMuted)
		})
		return
	}
	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).Gap(2).Padding(0, 8, 8, 8).Children(func() {
			for _, summary := range list {
				w.remoteSessionRow(c, m, summary)
			}
		})
	})
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

	row := ui.Column(c).Gap(1).Padding(6, 8).Radius(t.Radius)
	if selected {
		row.Background(t.SurfacePressed).BorderWidth(0, 0, 0, 2).BorderColor(t.Accent)
	} else if row.Hovered() {
		row.Background(t.SurfaceHover)
	}
	row.Tooltip(tooltip).Children(func() {
		ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(8, 8).Radius(4).Background(dot)
			ui.Text(c, m.Name).FontSize(12)
			if !m.Online {
				ui.Text(c, "离线").FontSize(10).TextColor(t.TextMuted)
			}
		})
		ui.Text(c, meta).FontSize(11).TextColor(t.TextMuted)
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

// remoteSessionRow is one shell on another machine. Opening it attaches over
// that machine's link, so the lease that decides who may type is that machine's
// hub's, exactly as it is for a shell on this one.
func (w *window) remoteSessionRow(c *ui.Context, m tunnel.Machine, s protocol.Summary) {
	t := c.Theme()
	name := s.Title
	if name == "" {
		name = filepath.Base(s.Cwd)
	}
	if name == "" || name == "." || name == "/" {
		name = s.SessionID
	}
	row := ui.Column(c).Gap(1).Padding(5, 8).Margin(2, 0, 0, 12).Radius(t.Radius).Children(func() {
		ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(6, 6).Radius(3).Background(w.leaseColor(t, m.ID, s.SessionID, s.Controller))
			ui.Text(c, name).FontSize(11)
		})
		ui.Text(c, fmt.Sprintf("%s · %dx%d", s.SessionID, s.Cols, s.Rows)).
			FontSize(10).TextColor(t.TextMuted)
	})
	row.Tooltip(m.Name + " 上的 " + s.SessionID)
	if row.Clicked() {
		w.openRemoteSession(m.ID, s.SessionID)
	}
}

// lookAtMachine opens the window's link to a machine and asks what it runs. A
// second look re-uses the stream, so the panel can be looked at freely. It is
// what picking a machine does; a machine the window cannot reach says why, in
// the panel, because a click that does nothing is the one thing a fleet panel
// must never do.
func (w *window) lookAtMachine(machine string) {
	w.ensureSessionList(machine)
}

// openRemoteSession attaches a tab to a shell on another machine.
func (w *window) openRemoteSession(machine, sessionID string) {
	if w.tabOf(machine, sessionID) != nil {
		return
	}
	if w.openTab(machine, sessionID) == nil {
		w.linkNote = w.noLinkReason(machine)
	}
}

// machineState is what a roster machine's tooltip says about it.
func machineState(m tunnel.Machine) string {
	if m.Online {
		return "在线"
	}
	return "离线"
}

// leaseColor is a shell's dot in the panel: green while a tab of this window
// holds the lease, amber while another client does, quiet while nobody does. A
// controller that is not one of this window's tabs is somebody else's by
// definition.
func (w *window) leaseColor(t *ui.Theme, machine, sessionID, controller string) ui.Color {
	if controller == "" {
		return shellColorsFor(t).quiet
	}
	if tb := w.tabOf(machine, sessionID); tb != nil && tb.held() {
		return t.Success
	}
	return t.Warning
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

// selectionID is the column's id for the shell on screen, or "" for none.
func (w *window) selectionID() string {
	if id := w.activeID(); id != "" {
		return railTabPrefix + id
	}
	return ""
}

// railAction runs what the column asked for. A click was written into the
// selection field while the column was being drawn, and the column reports it in
// the same frame, so the choice takes effect at once.
func (w *window) railAction(c *ui.Context, mode string) {
	if !strings.HasPrefix(w.rail, railTabPrefix) {
		return
	}
	if i := w.indexOfTab(strings.TrimPrefix(w.rail, railTabPrefix)); i >= 0 {
		w.selected = i
	}
}

// activeID is the session id of the shell on screen, or "" when there is none.
func (w *window) activeID() string {
	if tb := w.activeTab(); tb != nil {
		return tb.id
	}
	return ""
}

// indexOfTab is where a session sits in the tab order, or -1. A shell the hub
// has not named yet cannot be chosen from the column, so it is not there.
func (w *window) indexOfTab(id string) int {
	if id == "" {
		return -1
	}
	for i, key := range w.order {
		if tb := w.tabs[key]; tb != nil && tb.id == id {
			return i
		}
	}
	return -1
}
