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

	ui.Column(c).Fill().Children(func() {
		panelHead(c, "会话", len(labels), func() {
			if iconButton(c, iconPlus, "新建标签页", panelButtonSize).Clicked() {
				go w.create()
			}
			if iconButton(c, iconClose, "关闭标签页", panelButtonSize).Clicked() {
				if id := w.activeID(); id != "" {
					go w.kill(id)
				}
			}
		})

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

// machinePanelNeed is how much room the machines' panel's content asks for:
// its own base, plus a row per machine the roster knows and a row under each for
// the shells that machine is running.
func (w *window) machinePanelNeed() float32 {
	need := float32(machinePanelBase)
	for _, m := range w.roster {
		if m.ID == machineLocal {
			continue
		}
		need += machineRowHeight
		need += float32(len(w.remoteSessions[m.ID])) * sessionRowHeight
	}
	return need
}

// machinePanel is the bottom half of the column: the machines this fleet can
// reach. This machine is always there; the relay's roster adds the rest, each
// with the shells it is running once the window has looked at it.
func (w *window) machinePanel(c *ui.Context) {
	t := c.Theme()

	ui.Column(c).Fill().Children(func() {
		panelHead(c, "机器", w.machineCount(), nil)

		ui.Scroll(c).Fill().Children(func() {
			ui.Column(c).Gap(2).Padding(0, 8, 8, 8).Children(func() {
				// This machine, whatever else the fleet holds: the dot is the
				// service's state, as the strip's is, and the row is where a
				// person looks to see which machine they are on.
				name, meta := w.localMachine()
				ui.Column(c).Gap(1).Padding(6, 8).Radius(t.Radius).Children(func() {
					ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
						ui.Box(c).Size(8, 8).Radius(4).Background(w.fleetColor(t))
						ui.Text(c, name).FontSize(12)
					})
					ui.Text(c, meta).FontSize(11).TextColor(t.TextMuted)
				}).Tooltip(w.serviceLine())

				if w.relayOn() {
					st := w.fleet.RelayState()
					url, device := w.fleet.RelayIdentity()
					ui.Column(c).Gap(1).Padding(6, 8).Radius(t.Radius).Children(func() {
						ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
							ui.Box(c).Size(8, 8).Radius(4).Background(relayColor(t, st.Connected, st.Connecting))
							ui.Text(c, "中继").FontSize(12)
						})
						ui.Text(c, url).FontSize(11).TextColor(t.TextMuted)
						ui.Text(c, "设备 "+device).FontSize(11).TextColor(t.TextMuted)
					}).Tooltip(st.Detail)
				}

				// The rest of the fleet: one row per machine the relay knows,
				// and under it the shells that machine is running — which appear
				// once the window has a stream to it.
				for _, m := range w.roster {
					if m.ID == w.fleet.Host().Hostname || m.ID == machineLocal {
						continue
					}
					w.machineRow(c, m)
				}

				ui.Text(c, w.machineNote()).FontSize(11).TextColor(t.TextMuted)
			})
		})
	})
}

// machineRow is one machine of the roster: its name and where it is, and the
// shells it is running once the window has looked. Clicking the row asks that
// machine's hub for its session list; clicking a shell opens a tab on it.
func (w *window) machineRow(c *ui.Context, m tunnel.Machine) {
	t := c.Theme()
	row := ui.Column(c).Gap(1).Padding(6, 8).Radius(t.Radius).Children(func() {
		ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(8, 8).Radius(4).Background(relayColor(t, m.Online, false))
			ui.Text(c, m.Name).FontSize(12)
			if !m.Online {
				ui.Text(c, "离线").FontSize(10).TextColor(t.TextMuted)
			}
		})
		ui.Text(c, m.Host.Platform+"/"+m.Host.Arch).FontSize(11).TextColor(t.TextMuted)
	})
	row.Tooltip(m.ID + " · " + machineState(m))
	if m.Online {
		if row.Clicked() {
			w.lookAtMachine(m.ID)
		}
	}

	// The shells this machine is running, once its hub has said.
	for _, s := range w.remoteSessions[m.ID] {
		w.remoteSessionRow(c, m, s)
	}
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
// second look re-uses the stream, so the panel can be clicked freely.
func (w *window) lookAtMachine(machine string) {
	if l := w.remoteLink(machine); l != nil {
		l.list()
	}
}

// openRemoteSession attaches a tab to a shell on another machine.
func (w *window) openRemoteSession(machine, sessionID string) {
	if w.remoteLink(machine) == nil {
		return
	}
	if w.tabOf(machine, sessionID) != nil {
		return
	}
	w.openTab(machine, sessionID)
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

// machineNote is the line under the machines: what the relay is doing, and what
// the panel is for. It is the honest answer to "where are my other machines?"
// until the window can drive them.
func (w *window) machineNote() string {
	if !w.relayOn() {
		return "未接入中继。接入后，其他机器的会话会列在这里。"
	}
	return "同一中继上的其他机器会列在这里。"
}

// machineCount is what the panel's title counts: this machine, and every other
// machine the roster says is online. The relay row is not a machine — it is what
// makes the fleet — so it never counts.
func (w *window) machineCount() int {
	n := 1
	for _, m := range w.roster {
		if m.Online && m.ID != machineLocal {
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
