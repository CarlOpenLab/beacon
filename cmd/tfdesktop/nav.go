package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/diag"
	"github.com/CarlOpenLab/term-fleet/internal/protocol"
	"github.com/CarlOpenLab/term-fleet/internal/tunnel"
	"github.com/CarlOpenLab/term-fleet/internal/uitokens"
	"github.com/CarlOpenLab/term-fleet/locales"
)

// The column's measurements: what it is wide, what its panels ask for.
const (
	// sidebarWidth is the browser client's --w-side. The two clients list the
	// same shells in the same order with the same badges, so the column they sit
	// in is the same width too.
	sidebarWidth = 224

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
	// the row offers 结束: there is nothing to end once it has exited. An ended
	// shell offers 关闭 instead — its tab is still holding the last output, and
	// closing it is the only thing left to do with the row.
	alive bool
	pick  func()
	// end kills the process behind this shell, for every client. It is the row's
	// own action, separate from picking the row and from closing the tab.
	end func()
	// close closes the tab this row is about and ends the session behind it.
	// Empty for a row that has no tab of this window's; such a row uses end.
	close func()
	// dir is the directory a pane stands in, shown under the workspace's row
	// when the pane's name repeats. Empty for rows that do not draw it.
	dir string
	// panes are the terminals behind a split's one row, in the order the body
	// draws them. Empty for an ordinary session.
	panes []sessionEntry

	// What the shell is doing, as the daemon last said: the five states, whether
	// the user has looked at it since, what the program said about it, and when
	// the state began. The zero value is a shell the list has not described.
	status   string
	unread   bool
	message  string
	kind     string
	progress int
	since    int64
}

// withStatus fills a row from a session summary.
func (e sessionEntry) withStatus(sum protocol.Summary) sessionEntry {
	e.status, e.unread, e.message, e.kind, e.progress, e.since =
		sum.Status, sum.Unread, sum.Message, sum.Kind, sum.Progress, sum.Since
	return e
}

// sessionKey is what a row is remembered by when the person drags it: the
// shell it is about. A row that has no session yet (a shell the hub has not
// named) is remembered by its label, since it is the only name it has.
func sessionKey(e sessionEntry) string {
	if e.session != "" {
		return e.session
	}
	return "label:" + e.label
}

// sessionDrag is the value a row carries while it is being dragged. The rows
// of one machine only take each other: dragging a row across a group would be
// a move between machines, which this is not.
type sessionDrag struct {
	machine string
	key     string
}

// orderedEntries puts a machine's rows in the order the person dragged them
// into. A row the saved order has never seen keeps the list's own place at the
// end, so a new shell joins below the one it was opened from.
func (w *window) orderedEntries(machine string, entries []sessionEntry) []sessionEntry {
	saved := w.sessionOrder[machine]
	if len(saved) == 0 {
		return entries
	}
	rank := make(map[string]int, len(saved))
	for i, key := range saved {
		if _, seen := rank[key]; !seen {
			rank[key] = i
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		ri, iok := rank[sessionKey(entries[i])]
		rj, jok := rank[sessionKey(entries[j])]
		switch {
		case iok && jok:
			return ri < rj
		case iok:
			return true
		case jok:
			return false
		default:
			return false
		}
	})
	return entries
}

// summaryOf is the last word the window has about a session, from the list its
// machine pushed.
func (w *window) summaryOf(machine, id string) (protocol.Summary, bool) {
	list := w.live
	if machine != machineLocal {
		list = w.remoteSessions[machine]
	}
	for _, s := range list {
		if s.SessionID == id {
			return s, true
		}
	}
	return protocol.Summary{}, false
}

func dismissedKey(machine, id string) string {
	if machine == machineLocal {
		return id
	}
	return machine + "\x00" + id
}

// sessionEntries is the panel's list: this window's tabs when the panel is on
// this machine, and the shells the picked machine is running when it is on
// another one.
func (w *window) sessionEntries() []sessionEntry {
	return w.entriesFor(w.selectedMachine())
}

// workspaceLeaves is the tabs the split on screen holds, in the order the body
// draws them. Nil when the body is one pane, or when a pane is still waiting for
// the hub to name its shell.
func (w *window) workspaceLeaves() []*tab {
	if w.split == nil {
		return nil
	}
	var out []*tab
	var walk func(*splitNode)
	walk = func(n *splitNode) {
		if n == nil {
			return
		}
		if n.tab != nil {
			out = append(out, n.tab)
			return
		}
		walk(n.a)
		walk(n.b)
	}
	walk(w.split)
	for _, tb := range out {
		if tb.id == "" {
			return nil
		}
	}
	return out
}

// leavesOn is the split's panes that run on one machine.
func (w *window) leavesOn(machine string) []*tab {
	var out []*tab
	for _, tb := range w.workspaceLeaves() {
		if tb.machine == machine {
			out = append(out, tb)
		}
	}
	return out
}

// sidebarName is what a split's row calls a pane. A shell that has not named
// itself reports the machine's hostname as its title, which says nothing and
// makes four panes read as four copies of the machine: the directory it stands
// in is the better name then, and the row keeps the index for the rest.
func (w *window) sidebarName(tb *tab, machine string) string {
	name := tb.name()
	dir := shortDir(tb.cwd)
	if dir != "" && dir != name && name == w.machineName(machine) {
		return dir
	}
	return name
}

// workspaceRow is the one row a split earns in the sidebar — a session,
// its panes the bodies of the split. The sidebar lists what a person can switch
// to, not every shell behind a divider: pressing ⌘D splits the shell on screen,
// it does not quietly open a second session beside it.
func (w *window) workspaceRow(machine string, leaves []*tab) sessionEntry {
	first := leaves[0]
	entry := sessionEntry{
		machine: machine,
		session: first.id,
		label:   w.sidebarName(first, machine),
		meta:    locales.WorkspacePanes(len(leaves)),
		holder:  first.holder(),
		pick:    func() { w.selectTab(first) },
		end: func() {
			for _, tb := range leaves {
				w.kill(tb)
			}
		},
		close: func() {
			for _, tb := range leaves {
				w.closeTab(tb)
			}
		},
	}
	for _, tb := range leaves {
		if tb.alive {
			entry.alive = true
		}
	}
	entry.status, entry.unread, entry.progress, entry.since = w.aggregateStatus(leaves)
	for _, tb := range leaves {
		if tb == w.activeTab() {
			entry.active = true
		}
	}
	if len(leaves) > 1 {
		entry.panes = make([]sessionEntry, 0, len(leaves))
		for _, tb := range leaves {
			pane := sessionEntry{
				machine: machine,
				session: tb.id,
				label:   w.sidebarName(tb, machine),
				dir:     shortDir(tb.cwd),
				meta:    tb.size(),
				holder:  tb.holder(),
				active:  tb == w.activeTab(),
				alive:   tb.alive,
				pick:    func() { w.selectTab(tb) },
			}
			if sum, ok := w.summaryOf(machine, tb.id); ok {
				pane = pane.withStatus(sum)
			}
			entry.panes = append(entry.panes, pane)
		}
	}
	return entry
}

// aggregateStatus is what the workspace's row says about its panes: the most
// urgent of them, with that pane's message and clock. The order is the one the
// sidebar ranks a session's panes by — needs you, then failed, then finished unseen,
// then working, then quiet.
func (w *window) aggregateStatus(leaves []*tab) (state string, unread bool, progress int, since int64) {
	weight := func(sum protocol.Summary) int {
		switch {
		case sum.Status == "waiting":
			return 50
		case sum.Status == "error":
			return 40
		case sum.Status == "idle" && sum.Unread:
			return 30
		case sum.Status == "working":
			return 20
		case sum.Status == "exited":
			return 10
		}
		return 0
	}
	best := -1
	exited := 0
	for _, tb := range leaves {
		sum, ok := w.summaryOf(tb.machine, tb.id)
		if !ok {
			continue
		}
		if sum.Status == "exited" || !sum.Alive {
			exited++
		}
		if n := weight(sum); n > best {
			best = n
			state, unread, progress, since = sum.Status, sum.Unread, sum.Progress, sum.Since
		}
	}
	if best <= 0 && exited == len(leaves) {
		return "exited", false, -1, 0
	}
	if state == "" {
		return "idle", false, -1, 0
	}
	return state, unread, progress, since
}

// entriesFor is the rows of one machine's group: this window's tabs on this
// machine, or the shells the picked machine says it is running.
func (w *window) entriesFor(machine string) []sessionEntry {
	if machine == machineLocal {
		out := make([]sessionEntry, 0, len(w.order))
		seen := map[*workspace]bool{}
		// The shells the panel has a row for, so the session roster below can
		// add the ones this window has no tab for without listing any twice.
		listed := make(map[string]bool, len(w.order))
		for _, key := range w.order {
			if tb := w.tabs[key]; tb != nil && tb.machine == machineLocal && tb.id != "" {
				listed[tb.id] = true
			}
		}
		// A split built by hand (a test, or a frame before the workspace is
		// adopted) still reads as one row: its panes are grouped even though no
		// tab carries the workspace yet.
		implicitSet := map[*tab]bool{}
		var implicit []*tab
		if w.split != nil && w.ws == nil {
			implicit = workspaceLeaves(&workspace{split: w.split})
			for _, tb := range implicit {
				implicitSet[tb] = true
			}
		}
		implicitDone := false
		for _, key := range w.order {
			tb := w.tabs[key]
			if tb == nil || tb.machine != machineLocal {
				// A shell on another machine is a row of *that* machine's panel:
				// listing it here would put a row that moves the panels to another
				// machine in the middle of this machine's shells.
				continue
			}
			if implicitSet[tb] {
				if !implicitDone {
					out = append(out, w.workspaceRow(machineLocal, implicit))
					implicitDone = true
				}
				continue
			}
			if tb.ws != nil {
				if seen[tb.ws] {
					// A pane of the workspace: the workspace's row is the one
					// place it is listed.
					continue
				}
				seen[tb.ws] = true
				if leaves := workspaceLeaves(tb.ws); len(leaves) > 1 {
					out = append(out, w.workspaceRow(machineLocal, leaves))
					continue
				}
			}
			tab := tb
			entry := sessionEntry{
				machine: machineLocal,
				session: tab.id,
				label:   tab.name(),
				meta:    tab.size(),
				holder:  tab.holder(),
				active:  tab == w.activeTab(),
				alive:   tab.alive,
				pick:    func() { w.selectTab(tab) },
				end:     func() { w.kill(tab) },
				close:   func() { w.closeTab(tab) },
			}
			if sum, ok := w.summaryOf(machineLocal, tab.id); ok {
				entry = entry.withStatus(sum)
			}
			out = append(out, entry)
		}
		// The session roster is the machine's, not this window's tab strip: a
		// shell whose tab was closed is still running, and a panel that hid it
		// would say this machine has two shells while every other client —
		// Windows looking at this Mac, a phone — lists all seven. The row opens
		// a tab when picked, so the roster is the same on every client and the
		// count stops depending on which machine is asking.
		for _, s := range w.live {
			if !s.Alive || listed[s.SessionID] {
				continue
			}
			summary := s
			name := summary.Title
			if name == "" {
				name = filepath.Base(summary.Cwd)
			}
			if name == "" || name == "." || name == "/" {
				name = programName(summary.Shell)
			}
			if name == "" {
				name = locales.TabById(summary.SessionID)
			}
			out = append(out, sessionEntry{
				machine: machineLocal,
				session: summary.SessionID,
				label:   name,
				meta:    fmt.Sprintf("%dx%d", summary.Cols, summary.Rows),
				holder:  summary.Controller,
				alive:   true,
				pick:    func() { w.openOrSelect(machineLocal, summary.SessionID) },
				end:     func() { w.killSessionOn(machineLocal, summary.SessionID, false) },
			}.withStatus(summary))
		}
		return w.orderedEntries(machine, out)
	}

	list := w.remoteSessions[machine]
	out := make([]sessionEntry, 0, len(list))
	seen := map[*workspace]bool{}
	for _, s := range list {
		if !s.Alive && w.dismissed[dismissedKey(machine, s.SessionID)] {
			continue
		}
		if tab := w.tabOf(machine, s.SessionID); tab != nil && tab.ws != nil {
			if seen[tab.ws] {
				continue
			}
			seen[tab.ws] = true
			if leaves := workspaceLeaves(tab.ws); len(leaves) > 1 {
				out = append(out, w.workspaceRow(machine, leaves))
				continue
			}
		}
		summary := s
		name := summary.Title
		if name == "" {
			name = filepath.Base(summary.Cwd)
		}
		if name == "" || name == "." || name == "/" {
			if prog := programName(summary.Shell); prog != "" {
				name = prog
			} else if strings.HasPrefix(summary.SessionID, "s") {
				name = locales.TabById(summary.SessionID[1:])
			} else if summary.SessionID != "" {
				name = locales.TabById(summary.SessionID)
			} else {
				name = locales.TabFallback()
			}
		}
		// A shell the window already has a tab for is on screen when that tab is
		// the tab on screen: picking it is switching to it, not opening a second
		// tab, which is what a tab list is for.
		tab := w.tabOf(machine, summary.SessionID)
		var close func()
		if tab != nil {
			close = func() { w.closeTab(tab) }
		}
		// Who holds it, from the freshest word the window has: a tab's lease
		// frames arrive as the lease moves, while the machine's list is a
		// broadcast that can be a push behind. A row and the pane over it naming
		// two different holders is the disagreement this row exists to avoid.
		holder := summary.Controller
		if tab != nil {
			holder = tab.holder()
		}
		entry := sessionEntry{
			machine: machine,
			session: summary.SessionID,
			label:   name,
			meta:    fmt.Sprintf("%dx%d", summary.Cols, summary.Rows),
			holder:  holder,
			active:  tab != nil && tab == w.activeTab(),
			alive:   summary.Alive,
			pick:    func() { w.openOrSelect(machine, summary.SessionID) },
			end:     func() { w.killSessionOn(machine, summary.SessionID, false) },
			close:   close,
		}.withStatus(summary)
		out = append(out, entry)
	}
	// A workspace whose panes the machine's list has not caught up with still
	// gets its row: the layout is the window's own, not the broadcast's.
	for _, key := range w.order {
		tb := w.tabs[key]
		if tb == nil || tb.machine != machine || tb.ws == nil || seen[tb.ws] {
			continue
		}
		seen[tb.ws] = true
		if leaves := workspaceLeaves(tb.ws); len(leaves) > 1 {
			out = append(out, w.workspaceRow(machine, leaves))
		}
	}
	return w.orderedEntries(machine, out)
}

// sessionRow is one shell in its machine's group, drawn the way the sidebar draws
// a session: the state on the left, the name, then the quiet facts — and the ×
// that closes it, only while the pointer is on the row, because a row that wears
// every control it has says nothing with any of them.
//
// A row is dragged to move it in this window's order, and nothing else. While it
// is dragged the row is drawn at the place it would land, faint, under the copy
// that follows the pointer; placeRow slides it and its neighbours there.
func (w *window) sessionRow(c *ui.Context, pal *uitokens.Palette, e sessionEntry, key string, dragged bool, top float32) (dragging bool, dropFrom string) {
	row := ui.Row(c).Key("row:"+e.machine+"\x00"+key).
		Height(uitokens.RowH).FillWidth().Padding(0, 8, 0, 10).Gap(8).
		AlignItems(ui.Center).Radius(uitokens.RowRadius).Margin(0, 0, 1, 0).
		Label(e.label).Tooltip(w.sessionTooltip(e))
	if pal.Dark {
		// Room for the chosen row's edge in every row, so that choosing one
		// moves no text.
		row.Border(1, ui.Transparent)
	}
	// The copy that follows the pointer is the framework's; this row stays where
	// it would land, as the gap the list makes room for. The drop is worked out
	// after every row of the group is drawn (see machineGroup), so no row is
	// drawn in a list that changed under it.
	row.Drag(sessionDrag{machine: e.machine, key: key})
	if row.Dragging() {
		row.Cursor(ui.CursorGrabbing)
		dragging = true
	}
	if d, ok := ui.Drop[sessionDrag](row); ok && d.machine == e.machine {
		dropFrom = d.key
	}
	// Rows sliding under the pointer during a drag would flash their hover on
	// and off as they pass; the list is being ordered, not browsed.
	hovered := row.Hovered() && !dragged && w.rowDrag == nil
	// A split's top row is its first pane's row and the tab's own: it is chosen
	// when that pane is on screen, and the panes after it are the rows under it.
	firstPane := len(e.panes) == 0 || e.panes[0].active
	selected := e.active && firstPane
	bg := ui.Transparent
	switch {
	case dragged:
		bg = pal.RowHover.Alpha(0.55)
	case selected:
		bg = pal.RowSelected
	case hovered:
		bg = pal.RowHover
	}
	row.Background(row.AnimateColor("bg", bg, ui.Animation{Duration: 110e6}))
	if selected && !dragged {
		chosen(row, pal)
	}
	if dragged {
		// The row itself is not drawn where it would land — only the slot the
		// list paints there is. What the framework paints on the pointer is a
		// copy of this element, so the fill and the lift are what make that
		// copy look picked up rather than another row passing through.
		row.Opacity(0).Background(pal.Card).
			Shadow(0, 1, 2, 0, pal.CardShadow).
			Shadow(0, 8, 22, -6, pal.FloatShadow)
	}
	row.ContextMenu(func(m *ui.Menu) {
		if m.Item(locales.TabRename()).Chosen() {
			w.beginRename(e.machine, e.session, e.label)
		}
		if e.close != nil && m.Item(locales.TabClose()).Chosen() {
			e.close()
		}
		m.EditItems()
		// A row with a tab is closed by the item above, which now ends the
		// session too; 结束 is left for a shell this window has no tab for, where
		// there is no close to carry the act.
		if e.close == nil && e.alive && e.end != nil {
			if m.Item(locales.TabEndProcess()).Chosen() {
				e.end()
			}
		}
	})
	row.Children(func() {
		if w.renamingIs(e.machine, e.session) {
			// The name, being edited: a field in the name's place, which Enter
			// or a press elsewhere commits and Escape lets go of.
			w.renameField(c)
			return
		}
		name := ui.Text(c, e.label).FontSize(13.5).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
		if e.unread {
			name.FontWeight(650)
		}
		if !e.alive {
			name.TextColor(pal.TextMuted)
		}
		// The lease is the one thing a status glyph cannot say, so it keeps its
		// badge: 只读, 已退出, 创建中.
		if badge, color, ok := w.leaseBadge(c.Theme(), e); ok {
			badgeChip(c, badge, color)
		}
		// The state, then the way out: both stay on the row at rest, so a tab's
		// close is where the eye already is rather than a hover away. A pane in a
		// split draws its own row under this one and has no close of its own.
		if e.close != nil {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Shrink(0).Children(func() {
				statusGlyph(c, pal, e.status, e.unread, e.progress, 14)
				if iconButton(c, iconClose, locales.TabCloseNamed(e.label), 20).Clicked() {
					e.close()
				}
			})
			return
		}
		statusGlyph(c, pal, e.status, e.unread, e.progress, 14)
	})
	if row.Clicked() && !dragged {
		e.pick()
	}
	// A double-click on the row names the shell, the way every list on a Mac
	// renames what it lists.
	if row.DoubleClicked() {
		w.beginRename(e.machine, e.session, e.label)
	}
	w.placeRow(row, e.machine, key, top, !dragged)
	return dragging, dropFrom
}

// paneRow is one terminal of a split, under the workspace's row: a dotted branch
// of the tree, its state, and its name. The branch is dotted rather than a solid
// indent line, so it reads as a list under a tab instead of a second column; the
// state sits at the start of the branch, where the eye looks first. The last
// pane's branch stops at its middle, so the tree ends where its rows do. A pane
// at its prompt gets a hollow ring rather than no mark: under the workspace's
// row every pane should show where it is in the list.
func (w *window) paneRow(c *ui.Context, pal *uitokens.Palette, e sessionEntry, last bool, top float32) {
	key := sessionKey(e)
	row := ui.Row(c).Key("pane:"+e.machine+"\x00"+key).Height(uitokens.PaneH).FillWidth().Padding(0, 8, 0, 10).Gap(6).
		AlignItems(ui.Center).Radius(uitokens.RowRadius).Margin(0, 0, 1, 0).
		Label(e.label).Tooltip(w.sessionTooltip(e))
	if pal.Dark {
		row.Border(1, ui.Transparent)
	}
	hovered := row.Hovered() && w.rowDrag == nil
	selected := e.active
	bg := ui.Transparent
	switch {
	case selected:
		bg = pal.RowSelected
	case hovered:
		bg = pal.RowHover
	}
	row.Background(row.AnimateColor("bg", bg, ui.Animation{Duration: 110e6}))
	if selected {
		chosen(row, pal)
	}
	row.Children(func() {
		tree := ui.Box(c).Width(18).FillHeight().Shrink(0)
		tree.Draw(func(p *ui.Painter, r ui.Rect) {
			mid := r.Y + r.H/2
			bottom := r.Y + r.H
			if last {
				bottom = mid
			}
			line := pal.Text.Alpha(0.22)
			// A dotted trunk, and a dashed elbow that reaches the state mark.
			for y := r.Y; y < bottom; y += 4 {
				p.Fill(ui.Rect{X: r.X + 6, Y: y, W: 1, H: 1.5}, line, 0.5)
			}
			for x := r.X + 6; x < r.X+r.W+4; x += 3 {
				p.Fill(ui.Rect{X: x, Y: mid, W: 1.5, H: 1}, line, 0.5)
			}
		})
		if e.status == "" || e.status == "idle" && !e.unread {
			idleRing(c, pal, 12)
		} else {
			statusGlyph(c, pal, e.status, e.unread, e.progress, 12)
		}
		name := ui.Text(c, e.label).FontSize(12.5).TextColor(pal.TextMuted).
			SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
		if selected {
			name.TextColor(pal.Text)
		}
		if e.unread {
			name.FontWeight(600)
			name.TextColor(pal.Text)
		}
		if !e.alive {
			name.TextColor(pal.TextMuted)
		}
		if badge, color, ok := w.leaseBadge(c.Theme(), e); ok {
			badgeChip(c, badge, color)
		}
	})
	if row.Clicked() {
		e.pick()
	}
	w.placeRow(row, e.machine, "pane:"+key, top, true)
}

// idleRing is the hollow circle of a pane at its prompt, the same size as the
// filled states so that a row's name does not move as the pane changes state.
func idleRing(c *ui.Context, pal *uitokens.Palette, size float32) ui.Element {
	box := ui.Box(c).Size(size, size).Shrink(0)
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		cx, cy := r.X+r.W/2, r.Y+r.H/2
		rad := min(r.W, r.H)/2 - 1
		var ring ui.Path
		ring.Circle(cx, cy, rad*0.68)
		p.StrokePath(&ring, 1.4, pal.TextFaint.Alpha(0.75))
	})
	return box
}

// sessionMeta is the row's second line: the shell's id, its size, and what it is
// to this window — 控制中, 只读 with the name of whoever holds it, 无人控制 when
// nobody does, or 已退出. It is the browser client's line, field for field, so
// the same shell reads the same in both clients.
//
// The holder is not the controller a machine reports, and the difference shows
// on another machine's panel: the hub there names this window as the controller
// of the shell it is driving from here, and a row that read that at face value
// would name this window to itself as the one to be read-only about. The tab
// knows whether the lease is this window's; the summary does not.
func (w *window) sessionMeta(e sessionEntry) string {
	parts := []string{or(e.session, locales.LabelPending())}
	if e.meta != "" {
		parts = append(parts, e.meta)
	}
	switch {
	case e.session == "":
		parts = append(parts, locales.StatePending())
	case !e.alive:
		parts = append(parts, locales.StateEnded())
	case w.holds(e):
		parts = append(parts, locales.StateControlling())
	case e.holder != "":
		parts = append(parts, locales.StateReadonly()+" "+e.holder)
	default:
		// Neither this window's nor anybody else's. 控制中 here would say a
		// person has a keyboard that nobody has, over a shell whose pane says
		// 只读 — which is what it said before there was a word for this.
		parts = append(parts, locales.StateNobody())
	}
	return strings.Join(parts, " · ")
}

// holds is whether this window is the one driving the shell a row is about. A
// row for a shell this window has no tab for is one it is not driving: the
// window gives a lease up when the shell it belongs to leaves the screen.
func (w *window) holds(e sessionEntry) bool {
	tb := w.tabOf(e.machine, e.session)
	return tb != nil && tb.held()
}

// leaseBadge is the row's state in words, for the states worth a badge: the two
// that mean the person at the keyboard cannot type, or that there is nothing
// left to type into. 控制中 is the ordinary case and wears no badge — a row that
// badges everything says nothing. The colour is the same one the dot beside it
// already uses, so the two never disagree.
//
// A row whose shell this window is watching asks its own tab, so the row and the
// pane over it can never disagree; a shell nobody holds is not badged, because
// there is nobody to be read-only about and the click that picks the row is the
// claim.
func (w *window) leaseBadge(t *ui.Theme, e sessionEntry) (string, ui.Color, bool) {
	if tb := w.tabOf(e.machine, e.session); tb != nil {
		if tb.isEnded() {
			return locales.StateEnded(), t.Danger, true
		}
		if tb.isPending() {
			return locales.StatePending(), shellColorsFor(t).quiet, true
		}
		if readOnly(tb) {
			return locales.StateReadonly(), t.Warning, true
		}
		return "", t.Success, false
	}
	if !e.alive {
		return locales.StateEnded(), t.Danger, true
	}
	if e.holder != "" {
		return locales.StateReadonly(), t.Warning, true
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
	if e.dir != "" && e.dir != e.label {
		lines = append(lines, e.dir)
	}
	switch {
	case w.holds(e):
		// This window is a client like any other, and on this machine's panel it
		// is the one holding the lease of the shell it is showing. 当前无人控制
		// here would be the window denying itself.
		lines = append(lines, locales.StateControllerSelf())
	case e.holder != "":
		lines = append(lines, locales.StateControllerOther(e.holder))
	default:
		lines = append(lines, locales.StateControllerNobody())
	}
	return strings.Join(lines, "\n")
}

// badgeChip is a state in a small outlined chip: the browser client's badge, on
// the ui's terms. It is a word in the state's own colour inside a hairline of the
// same colour and nothing else — a state that is worth saying is worth saying
// quietly, and a row wearing a wash for every state is a row nobody can scan.
func badgeChip(c *ui.Context, text string, color ui.Color) {
	// A chip is its word and nothing else: it does not wrap, and it does not
	// give width up to the name beside it. Squeezed, 已退出 reads 已退/出 in a
	// box drawn for three characters, which is a state badge that looks broken.
	chip := ui.Row(c).Gap(4).AlignItems(ui.Center).Shrink(0).
		PaddingX(6).Height(16).Radius(radiusSmall).
		BorderWidth(1).BorderColor(color.Alpha(0.42))
	chip.Children(func() {
		ui.Text(c, text).FontSize(10).TextColor(color).SingleLine()
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
// to a shell on another machine moves them there rather than leaving them on
// a machine that is no longer on screen.
//
// With a split open, a tab already on screen in a pane becomes the focused
// pane, and any other tab lands in the focused pane — picking a third shell
// beside two others is asking to look at it beside them, not to lose them.
func (w *window) selectTab(tb *tab) {
	if tb == nil {
		return
	}
	// A tab in a workspace brings the whole layout back; a tab of its own puts
	// the layout away. The old fallback keeps a split built by hand (a test, or a
	// frame before the workspace was adopted) behaving as one.
	if tb.ws != nil {
		w.enterWorkspace(tb.ws, tb)
		return
	}
	if w.split != nil {
		if n := w.leafOf(tb); n != nil {
			w.focusSplit(n)
			return
		}
	}
	w.leaveWorkspace()
	w.selectTabRaw(tb)
}

// selectTabRaw is selectTab without the split: it names the tab the strip is
// about and syncs the size of a shell this window holds. The split's own paths
// call it once they have decided which pane is on screen.
func (w *window) selectTabRaw(tb *tab) {
	for i, key := range w.order {
		if w.tabs[key] != tb {
			continue
		}
		w.selected = i
		if tb.machine != "" {
			w.machineSel = tb.machine
		}
		if tb.held() {
			tb.conn.syncSize(0, 0)
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
		// A tab whose link went away renders nothing new, ever — selecting it
		// would put a dead pane on screen and explain nothing. Give it the
		// machine's link as it now stands first, so the click lands on a pane
		// that can show the shell again.
		if tb.sess != nil && tb.sess.link.dead() {
			if l := w.linkTo(machine); l != nil {
				w.watchTab(tb, l, machine, sessionID)
			}
		}
		w.selectTab(tb)
		return
	}
	if machine == machineLocal {
		// A shell of this machine's can be listed before it has a tab: its tab was
		// closed while the shell kept running. Picking the row opens one again,
		// which is what makes the local roster read like the remote one.
		for _, s := range w.live {
			if s.SessionID != sessionID || !s.Alive {
				continue
			}
			if err := w.mount(s); err != nil {
				w.setNotice(err.Error())
				return
			}
			if tb := w.tabOf(machineLocal, sessionID); tb != nil {
				w.selectTab(tb)
			}
			return
		}
		return
	}
	// A session row outlives its machine: the list the panel draws is the last
	// one the machine pushed, and it stays on screen while the machine is gone.
	// Opening a tab for one cannot paint anything — there is no stream to attach
	// over — and the white pane that follows reads as a broken app rather than as
	// a machine that is simply not there. The roster is the authority on
	// presence, so it answers this before a tab exists to be blank.
	if !w.machineOnline(machine) {
		w.linkNote = locales.AppMachineOffline(w.machineName(machine))
		w.setNotice(w.linkNote)
		w.invalidate()
		diag.Infof("term", "session not opened: machine offline machine=%s session=%s", machine, sessionID)
		return
	}
	if tb := w.openTab(machine, sessionID); tb != nil {
		w.showInFocus(tb)
	} else {
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
		return locales.MachineLocal()
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
		return locales.MachineOnline()
	}
	return locales.MachineOffline()
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
		host = locales.MachineLocal()
	}
	if w.fleet == nil {
		return host, locales.MachinePlatformUnknown()
	}
	h := w.fleet.Host()
	return host, h.Platform + "/" + h.Arch + " · " + locales.MachineSessionsCount(w.localSessionCount())
}

// localSessionCount is the number the machine row shows. It is the live session
// roster, not the tab strip: the panel under it lists every running shell, and a
// count of open tabs above a list of seven sessions is the disagreement this
// window used to have with every other client.
func (w *window) localSessionCount() int {
	seen := make(map[string]bool, len(w.live))
	n := 0
	for _, s := range w.live {
		if s.Alive {
			n++
			seen[s.SessionID] = true
		}
	}
	// A tab created a moment ago is alive before the next roster broadcast
	// names it; counting only the roster would make the number dip for a frame.
	for _, key := range w.order {
		tb := w.tabs[key]
		if tb == nil || tb.machine != machineLocal || tb.id == "" || !tb.alive || seen[tb.id] {
			continue
		}
		n++
		seen[tb.id] = true
	}
	if n == 0 {
		// A window whose only tab is still waiting for its session id has no
		// roster entry to count, but it does have one shell to show.
		return len(w.order)
	}
	return n
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
		return locales.MachineNoteLocalOnly()
	}
	if w.fleet.RelayClient() == nil {
		return locales.MachineNoteNoToken()
	}
	return locales.MachineNoteReady()
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

// panelHead is a panel's title row: the name, a count of what it holds, and
// whatever controls the panel needs on the right. The name is a small quiet
// capital — the browser client's .side-title — so the panels read as headings
// over the rows rather than as rows themselves.
