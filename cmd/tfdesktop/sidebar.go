package main

import (
	"math"
	"sort"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/beacon/internal/protocol"
	"github.com/CarlOpenLab/beacon/internal/settings"
	"github.com/CarlOpenLab/beacon/internal/tunnel"
	"github.com/CarlOpenLab/beacon/internal/uitokens"
	"github.com/CarlOpenLab/beacon/locales"
)

// The sidebar is a column of machines, each with its sessions: the chosen one a
// raised row, a group header that folds it away, and a foot of quiet icon
// buttons. A machine the window has not asked for its shells yet says so in one
// row instead of showing an empty group.

// The column folds and unfolds rather than appearing whole: the width it takes
// is animated, and the cards beside it move with it. The measurements below are
// what the animation and the layout agree on.
const (
	// toolbarButton is the square of the top bar's button, and of the same
	// button when the card header holds it with the column hidden.
	toolbarButton = float32(28)

	// headerPadShown is a card header's own left inset while the column is
	// shown. With it hidden the header starts after the window's controls (the
	// traffic lights, where the platform keeps them at the left) and the
	// column's button, which it then carries.
	headerPadShown = float32(uitokens.CardGap + 8)
)

// sidebarMotion is the fold's curve. It lands quickly and moves in every frame:
// a spring's long tail, quantized to the column's pixels, would crawl and stop.
var sidebarMotion = ui.Animation{Duration: 260 * time.Millisecond, Ease: ui.CubicBezier(0.25, 1, 0.5, 1)}

// groupFold is the curve a group's rows take when the person folds them away by
// hand: shorter than the column's own fold, so the two are not mistaken for one
// movement, and eased so the rows leave rather than snap out of the column.
var groupFold = ui.Animation{Duration: 180 * time.Millisecond, Ease: ui.CubicBezier(0.22, 1, 0.36, 1)}

// groupFoldNow is what a group folds with when the window, not the person,
// opened it: a jump to a session, a machine picked in the column. It lands on
// the next frame, which is the still column the person expects to find there.
var groupFoldNow = ui.Animation{Duration: time.Millisecond, Ease: ui.CubicBezier(0.22, 1, 0.36, 1)}

// hiddenReleaseDelay is how long a hidden machine stays after the session that
// needed the user stopped needing them. A row that vanished under the pointer
// that had just answered it would read as the click deleting the machine.
const hiddenReleaseDelay = 1500 * time.Millisecond

// sideChrome is where the fold has the sidebar and the cards this frame, in
// whole pixels so that edges stay crisp. Each is rounded from one progress so
// that they move together.
type sideChrome struct {
	ready     bool
	shown     float32 // 0 hidden, 1 shown
	width     float32 // the column's visible width
	mainPad   float32 // the left padding of the cards' column
	headerPad float32 // a card header's left padding
}

// chromeFor is the layout at a point in the fold, around the room the system's
// window controls report (bar) for as long as the column is folding away.
func chromeFor(progress, width float32, bar ui.TitleBar) sideChrome {
	round := func(v float32) float32 { return float32(math.Round(float64(v))) }
	s := min(max(progress, 0), 1)
	w := round(width * s)
	shown := float32(1)
	if width > 0 {
		shown = w / width
	}
	pad := round(uitokens.CardGap * (1 - shown))
	// Where the header's title starts in the window, rounded once: after the
	// column's buttons with it hidden, 16 into the card with it shown.
	hidden, shownAt := cardTitleLead(bar), width+headerPadShown
	x := round(hidden + (shownAt-hidden)*shown)
	return sideChrome{ready: true, shown: shown, width: w, mainPad: pad, headerPad: x - w - pad}
}

// chromeNow is the layout the window is drawn with. A view that has not run the
// fold yet — a panel drawn on its own in a test — is at the state the window
// says it is in.
func (w *window) chromeNow(c *ui.Context) sideChrome {
	if w.chrome.ready {
		return w.chrome
	}
	if w.sidebarHidden {
		return sideChrome{shown: 0, width: 0, mainPad: uitokens.CardGap,
			headerPad: cardTitleLead(c.TitleBar()) - uitokens.CardGap}
	}
	return sideChrome{shown: 1, width: w.sideWidth(), headerPad: headerPadShown}
}

// sideWidth is how wide the column is, clamped to what a name fits in.
func (w *window) sideWidth() float32 {
	if w.sidebarWidth == 0 {
		return uitokens.SidebarWidth
	}
	return min(max(w.sidebarWidth, uitokens.MinSidebar), uitokens.MaxSidebar)
}

// sidebar draws the column: the window's buttons, the machine groups, and the
// foot with the palette, the settings and how the fleet stands.
func (w *window) sidebar(c *ui.Context, pal *uitokens.Palette) {
	bar := c.TitleBar()
	topHeight := max(float32(uitokens.TopBar), bar.Height)
	lead := titleBarLead(bar)
	ch := w.chromeNow(c)

	// The column is as wide as the fold has it and clips what does not fit;
	// its contents keep their full width and slide out under the clip, so the
	// rows do not reflow as the column narrows.
	side := ui.Row(c).Width(ch.width).FillHeight().Shrink(0).ClipX()
	side.Children(func() {
		ui.Column(c).Width(w.sideWidth()).FillHeight().Shrink(0).
			Margin(0, 0, 0, ch.width-w.sideWidth()).
			Opacity(ch.shown).Children(func() {
			if ch.shown > 0.99 {
				w.resizeHandle(c, pal)
			}
			ui.Row(c).Height(topHeight).Shrink(0).Padding(0, 12, 0, lead).Gap(2).
				AlignItems(ui.Center).DragWindow().Children(func() {
				if iconButton(c, iconSidebar, locales.ActionToggleSidebar(), toolbarButton).Clicked() {
					w.sidebarHidden = true
				}
				ui.Spacer(c)
			})

			ui.Scroll(c).Grow(1).MinHeight(0).Padding(0, 10, 16, 10).Children(func() {
				w.sidebarMachines(c, pal)
			})
			w.sidebarFooter(c, pal)
		})
	})
}

// sidebarPeek draws the top bar's buttons over the cards while the column is
// folding away. They stay where they were in the window rather than travelling
// with the column, so the pointer finds them where it left them, and they fade
// as the card's own header takes over.
func (w *window) sidebarPeek(c *ui.Context) {
	ch := w.chromeNow(c)
	// The column buttons slide out with the column; these take over before they
	// leave, and stay where they were in the window as the card slides under
	// them.
	alpha := min(1, (0.75-ch.shown)/0.10)
	if alpha <= 0.01 {
		return
	}
	ui.Row(c).Key("sidebar-peek").Absolute().
		Left(titleBarLead(c.TitleBar())).
		Top((uitokens.TopBar - toolbarButton) / 2).
		Height(toolbarButton).Gap(2).AlignItems(ui.Center).Opacity(alpha).Children(func() {
		if iconButton(c, iconSidebar, locales.ActionToggleSidebar(), toolbarButton).Clicked() {
			w.sidebarHidden = false
		}
	})
}

// resizeHandle is the column's edge, which a drag moves and a double click puts
// back.
func (w *window) resizeHandle(c *ui.Context, pal *uitokens.Palette) {
	h := ui.Box(c).Absolute().Left(w.sideWidth() - 4).Top(0).Bottom(0).Width(8).Cursor(ui.CursorResizeEW)
	if dx, _, ok := h.Dragged(); ok {
		if w.sideDrag == 0 {
			w.sideDrag = w.sideWidth()
		}
		w.sideDrag += dx
		w.sidebarWidth = min(max(w.sideDrag, uitokens.MinSidebar), uitokens.MaxSidebar)
	} else if w.sideDrag != 0 && !h.Pressed() {
		w.sideDrag = 0
	}
	if h.DoubleClicked() {
		w.sidebarWidth = uitokens.SidebarWidth
	}
	_ = pal
}

// groups is the sidebar's list: this machine, and every other machine the roster
// knows. The relay itself is the pipe, not a machine, and the roster's copy of
// this machine is the first row already.
func (w *window) groups() []tunnel.Machine {
	out := []tunnel.Machine{w.localEntry()}
	for _, m := range w.roster {
		if !w.ownMachine(m) {
			out = append(out, m)
		}
	}
	return out
}

// sidebarMachines draws the column's machine list: the groups that earn a
// place, the one row that holds the quiet ones, and, when nothing is left, the
// two rows that say so and offer the way back.
func (w *window) sidebarMachines(c *ui.Context, pal *uitokens.Palette) {
	w.reconcileHidden(c)
	normal, other := w.splitMachines(c.Now())
	for i, m := range normal {
		divided := i > 0 && len(w.entriesFor(normal[i-1].ID)) > 0
		w.machineGroup(c, pal, m, i == 0, divided, false)
	}
	if len(other) > 0 {
		w.otherMachinesRow(c, pal, len(other))
	}
	// The other row is the only thing left when every quiet machine is folded
	// away, and the two empty rows are what a column with no machine at all
	// says. Hidden machines are not counted here: they are the setting's job,
	// not a group with nothing under it. While the quiet headers are still on
	// their way out the empty rows wait, so they do not shove the fold down.
	folding := w.foldedMachines(c, pal, other)
	if len(normal) == 0 && (len(other) == 0 || (!w.othersOpen && !folding)) {
		w.sidebarEmpty(c, pal)
	}
}

// splitMachines separates the machines that keep a header from the quiet ones
// the other row holds. A machine is other only when it has no rows, needs
// nothing from the user, and is not the one this window is working on: any of
// those makes it a place the person may be about to look.
func (w *window) splitMachines(now time.Time) (normal, other []tunnel.Machine) {
	selected := w.workingMachine()
	for _, m := range w.groups() {
		if !w.machineShown(m.ID, now) {
			continue
		}
		entries := w.entriesFor(m.ID)
		if len(entries) == 0 && !entriesNeedYou(entries) && m.ID != selected {
			other = append(other, m)
			continue
		}
		normal = append(normal, m)
	}
	return normal, other
}

// workingMachine is the machine the person is working on, which is the one the
// panes show. It is not the last machine clicked: picking a machine this window
// has no shell on keeps the terminal where it was, so a click that changed
// nothing on screen must not change the column either.
func (w *window) workingMachine() string {
	if tb := w.activeTab(); tb != nil {
		return tb.machine
	}
	return w.selectedMachine()
}

// entriesNeedYou is the product's loudest state: a session waiting for an
// answer or reporting a failure. It ranks above a quiet machine, and above the
// person's choice to hide one.
func entriesNeedYou(entries []sessionEntry) bool {
	for _, e := range entries {
		if e.status == "waiting" || e.status == "error" {
			return true
		}
	}
	return false
}

func (w *window) machineNeedsYou(id string) bool {
	return entriesNeedYou(w.entriesFor(id))
}

// hiddenSet is the machines a settings file hides, as a set the drawing code
// can ask without walking the list every frame.
func hiddenSet(ids []string) map[string]bool {
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

// reconcileHidden applies the hidden machines' exception: one that now needs
// the user is shown, and one whose need has just passed keeps its row for a
// moment. The delay is a frame timer rather than a sleep, so the row leaves
// when the window's own clock says, not when a wall clock happens to.
func (w *window) reconcileHidden(c *ui.Context) {
	if len(w.hidden) == 0 {
		return
	}
	now := c.Now()
	for id := range w.hidden {
		if w.machineNeedsYou(id) {
			if w.hiddenNeed == nil {
				w.hiddenNeed = make(map[string]bool)
			}
			w.hiddenNeed[id] = true
			delete(w.hideAt, id)
			continue
		}
		if w.hiddenNeed[id] {
			delete(w.hiddenNeed, id)
			if w.hideAt == nil {
				w.hideAt = make(map[string]time.Time)
			}
			w.hideAt[id] = now.Add(hiddenReleaseDelay)
		}
		until, ok := w.hideAt[id]
		if !ok {
			continue
		}
		if !now.Before(until) {
			delete(w.hideAt, id)
			continue
		}
		c.After(until.Sub(now))
	}
}

// machineShown says whether a machine has a header this frame. A hidden one
// comes back while it needs the user, and for the release delay after.
func (w *window) machineShown(id string, now time.Time) bool {
	if !w.machineHidden(id) {
		return true
	}
	if w.hiddenNeed[id] {
		return true
	}
	until, ok := w.hideAt[id]
	return ok && now.Before(until)
}

func (w *window) machineHidden(id string) bool {
	return w.hidden[id]
}

// setMachineHidden writes the person's choice to the settings file and applies
// it to this window. The store is the only place it lives, so a restart shows
// the same column.
func (w *window) setMachineHidden(id string, hide bool) error {
	if w.store != nil {
		_, err := w.store.Update(func(st *settings.Settings) {
			ids := make([]string, 0, len(st.HiddenMachines)+1)
			for _, have := range st.HiddenMachines {
				if have != id {
					ids = append(ids, have)
				}
			}
			if hide {
				ids = append(ids, id)
			}
			st.HiddenMachines = ids
		})
		if err != nil {
			return err
		}
	}
	if hide {
		if w.hidden == nil {
			w.hidden = make(map[string]bool)
		}
		w.hidden[id] = true
	} else {
		delete(w.hidden, id)
	}
	delete(w.hiddenNeed, id)
	delete(w.hideAt, id)
	w.invalidate()
	return nil
}

// showAllMachines lets every hidden machine back, for restore defaults.
func (w *window) showAllMachines() error {
	if w.store != nil {
		if _, err := w.store.Update(func(st *settings.Settings) { st.HiddenMachines = nil }); err != nil {
			return err
		}
	}
	w.hidden = map[string]bool{}
	w.hiddenNeed = map[string]bool{}
	w.hideAt = map[string]time.Time{}
	w.invalidate()
	return nil
}

// otherMachinesRow is the quiet machines behind one line. It is a look, not a
// setting: the folded state is this window's, and a restart begins folded.
func (w *window) otherMachinesRow(c *ui.Context, pal *uitokens.Palette, count int) {
	label := locales.SidebarOthers(count)
	row := ui.Row(c).Height(28).Margin(4, 0, 2, 0).Padding(0, 8, 0, 10).Gap(6).
		AlignItems(ui.Center).Radius(6)
	hovered := row.Hovered()
	row.Children(func() {
		chev := ui.Icon(c, iconChevron).Size(11, 11).TextColor(pal.TextFaint)
		chev.Rotate(chev.AnimateTo("turn", 90*float32(b2f(w.othersOpen)), ui.Snappy))
		ui.Text(c, label).FontSize(12).FontWeight(500).TextColor(pal.Text.Alpha(0.8)).SingleLine()
	})
	if hovered {
		row.Background(pal.RowHover.Alpha(0.4))
	}
	if row.Clicked() {
		w.othersOpen = !w.othersOpen
		w.invalidate()
	}
}

// foldedMachines grows the quiet machines under the other row, in the order
// they keep in the column. While closed it draws nothing, so a folded name is
// not merely transparent but absent. It reports whether any of them is still
// on screen, which is what keeps the empty state from appearing mid-fold.
func (w *window) foldedMachines(c *ui.Context, pal *uitokens.Palette, machines []tunnel.Machine) bool {
	fold := ui.Column(c).Key("sidebar-others").Shrink(0)
	folded := fold.AnimateTo("fold", float32(b2f(w.othersOpen)), groupFold)
	if folded == 0 {
		return false
	}
	var list ui.Element
	fold.Children(func() {
		list = ui.Column(c).FillWidth()
		list.Children(func() {
			// Open, these are machines and the rows above are sessions: without
			// a line the two run together and the group has no end. Folded, the
			// row is a summary of its own and a line under it would claim a
			// second group, so the line lives inside the fold and leaves with it.
			ui.Box(c).Height(0.5).FillWidth().Margin(10, 10, 0, 10).Background(pal.Text.Alpha(0.12))
			for i, m := range machines {
				divided := i > 0 && len(w.entriesFor(machines[i-1].ID)) > 0
				w.machineGroup(c, pal, m, i == 0, divided, false)
			}
		})
	})
	if folded < 1 {
		fold.ClipY().Height(folded * list.Bounds().H).Opacity(folded)
	}
	return true
}

// sidebarEmpty is what a column with nothing to show says, and the row that
// takes the person to the switch that changes it. The second row is an entry,
// not a note: a dead end that tells someone where to go is still a dead end.
func (w *window) sidebarEmpty(c *ui.Context, pal *uitokens.Palette) {
	ui.Row(c).Height(30).Margin(4, 0, 0, 0).Padding(0, 12).AlignItems(ui.Center).Children(func() {
		ui.Text(c, locales.SidebarNoneVisible()).FontSize(13).TextColor(pal.TextMuted).SingleLine()
	})
	row := ui.Row(c).Height(44).Padding(0, 8, 0, 10).Gap(6).AlignItems(ui.Center).Radius(6)
	hovered := row.Hovered()
	row.Children(func() {
		ui.Text(c, locales.SidebarRestoreMachines()).FontSize(13).FontWeight(500).TextColor(pal.Text).SingleLine()
		ui.Spacer(c)
		ui.Icon(c, iconChevron).Size(11, 11).TextColor(pal.TextFaint)
	})
	if hovered {
		row.Background(pal.RowHover.Alpha(0.4))
	}
	if row.Clicked() {
		w.openSettingsAt(paneSidebar)
	}
}

// settingsMachines is every machine the settings page can offer a switch for:
// the ones the column knows, then any hidden id the roster has forgotten, so a
// machine that went away can still be let back.
func (w *window) settingsMachines() []tunnel.Machine {
	out := w.groups()
	seen := make(map[string]bool, len(out))
	for _, m := range out {
		seen[m.ID] = true
	}
	var forgotten []string
	for id := range w.hidden {
		if !seen[id] {
			forgotten = append(forgotten, id)
		}
	}
	sort.Strings(forgotten)
	for _, id := range forgotten {
		out = append(out, tunnel.Machine{ID: id, Name: w.machineName(id)})
	}
	return out
}

// machineGroup is one machine and, under it, its sessions: a project group,
// with the machine's state in the header and a "+" that opens a shell there.
func (w *window) machineGroup(c *ui.Context, pal *uitokens.Palette, m tunnel.Machine, first, divided, quiet bool) {
	selected := w.selectedMachine() == m.ID
	collapsed := w.collapsed[m.ID]
	entries := w.entriesFor(m.ID)

	top := float32(6)
	if first {
		top = 2
	}
	// A line rather than a stretch of blank: while scrolling, the gap between two
	// groups and the gap between two sessions of one group look the same.
	if divided {
		ui.Box(c).Height(0.5).FillWidth().Margin(10, 10, 0, 10).Background(pal.Text.Alpha(0.12))
	}
	hdr := ui.Row(c).Height(uitokens.GroupH).Margin(top, 0, 3, 0).Padding(0, 6, 0, 10).Gap(6).
		AlignItems(ui.Center).Radius(6).Tooltip(w.machineTooltip(m))
	hovered := hdr.Hovered()
	hdr.ContextMenu(func(mu *ui.Menu) {
		if mu.Item(locales.ActionNewTerminalTip()).Chosen() {
			w.newTabOn(m.ID)
		}
		if mu.Item(locales.SessionRefresh()).Chosen() {
			w.refreshSessions(m.ID)
		}
		if mu.Item(locales.MachineRefresh()).Chosen() {
			w.refreshMachines()
		}
		if mu.Item(locales.ActionHideMachine()).Chosen() {
			if err := w.setMachineHidden(m.ID, true); err != nil {
				w.setNotice(err.Error())
			}
		}
	})
	if hdr.Clicked() {
		if !selected {
			w.selectMachine(m)
			delete(w.collapsed, m.ID)
		} else {
			// The person folded this group by hand: the rows leave with the
			// fold, where a jump into a session lands on a group already open.
			if w.foldByHand == nil {
				w.foldByHand = make(map[string]bool)
			}
			w.foldByHand[m.ID] = true
			if collapsed {
				delete(w.collapsed, m.ID)
			} else {
				w.collapsed[m.ID] = true
			}
		}
	}
	if hovered {
		hdr.Background(pal.RowHover.Alpha(0.5))
	}
	hdr.Children(func() {
		// The machine's state, where a project group has a count: a dot that says
		// whether this machine answers, and the count of what it runs.
		state := w.machineDot(c, m)
		ui.Box(c).Size(7, 7).Radius(4).Background(state).Shrink(0)
		name := m.Name
		if name == "" {
			name = w.machineName(m.ID)
		}
		ui.Text(c, name).FontSize(12.5).FontWeight(600).TextColor(pal.Text).
			SingleLine().Ellipsis("…").MinWidth(0).Shrink(1)
		ui.Textf(c, "%d", len(entries)).FontSize(11).TextColor(pal.TextFaint).FontFeatures("tnum").Shrink(0)
		chev := ui.Icon(c, iconChevron).Size(11, 11).TextColor(pal.TextFaint)
		chev.Rotate(chev.AnimateTo("turn", 90*float32(b2f(!collapsed)), ui.Snappy))
		chev.Opacity(chev.Animate("shown", float32(b2f(hovered || collapsed)), 120e6))
		ui.Spacer(c)
		// Ask this machine for its session list again — under the pointer, where a
		// button that acts on the group belongs, and in the menu for a hand that
		// never hovers. Its square stays at rest so the count and the + hold still.
		refreshSlot := ui.Box(c).Size(20, 20).Shrink(0)
		if hovered || w.refreshing(m.ID) {
			refreshSlot.Children(func() {
				if refreshButton(c, pal, w.refreshing(m.ID), locales.SessionRefreshMachine()).Clicked() {
					w.refreshSessions(m.ID)
				}
			})
		}
		if iconButton(c, iconPlus, locales.SessionNewOnMachine(), 20).Clicked() {
			w.newTabOn(m.ID)
		}
	})
	// How far the rows are in the column: 1 open, 0 folded away. Only a hand on
	// the header moves it; every other fold is a jump that lands at once.
	foldAnim := groupFoldNow
	if w.foldByHand[m.ID] {
		foldAnim = groupFold
		delete(w.foldByHand, m.ID)
	}
	folded := hdr.AnimateTo("fold", float32(b2f(!collapsed)), foldAnim)
	if collapsed && folded == 0 {
		return
	}
	// The group's rows are drawn in this window's order, with the row being
	// dragged already where it would land: the list closes the hole it left and
	// opens one there, and the rows between slide into their new places
	// (placeRow). The drop is worked out once every row is on screen, so no row
	// is drawn in a list that changed under it.
	rows, dragKey, _ := w.previewEntries(m.ID, entries)
	tops := make([]float32, len(rows))
	// The rows live in a column of their own, so that a fold clips them to the
	// height they have so far: set on the rows themselves, the height read back
	// would be the one the last frame left them with, and the fold would
	// compound instead of reaching the end.
	fold := ui.Column(c).Shrink(0)
	var list ui.Element
	var dragging bool
	var droppedFrom string
	var slotTop = float32(-1)
	fold.Children(func() {
		list = ui.Column(c).FillWidth()
		// The row being dragged is not drawn where it would land, only the hole it
		// leaves: the copy under the pointer is the row itself, painted by the
		// framework, so the list needs a slot for it to drop into.
		list.Draw(func(p *ui.Painter, r ui.Rect) {
			if slotTop < 0 {
				return
			}
			p.Fill(ui.Rect{X: r.X, Y: r.Y + slotTop, W: r.W, H: uitokens.RowH},
				pal.RowHover, uitokens.RowRadius)
		})
		list.Children(func() {
			y := float32(0)
			for i, e := range rows {
				key := sessionKey(e)
				tops[i] = y
				if key == dragKey {
					slotTop = y
				}
				rowDragging, drop := w.sessionRow(c, pal, e, key, key == dragKey, y)
				if rowDragging {
					dragging = true
					d := w.rowDrag
					if d == nil || d.machine != m.ID || d.key != key {
						// The drag has just begun: the row starts where the list
						// already had it, so the first preview changes nothing.
						at := 0
						for _, other := range rows[:i] {
							if sessionKey(other) != key {
								at++
							}
						}
						d = &rowDrag{machine: m.ID, key: key, at: at}
						w.rowDrag = d
					}
					d.active = true
				}
				if drop != "" {
					droppedFrom = drop
				}
				y += uitokens.RowH + 1
				// The first pane is the tab's own row; the others hang under it.
				for j, pane := range e.panes {
					if j == 0 {
						continue
					}
					w.paneRow(c, pal, pane, j == len(e.panes)-1, y)
					y += uitokens.PaneH + 1
				}
			}
		})
	})
	if folded < 1 {
		fold.ClipY().Height(folded * list.Bounds().H).Opacity(folded)
	}
	if w.rowDrag != nil && w.rowDrag.machine == m.ID {
		d := w.rowDrag
		if dragging || droppedFrom != "" {
			// Where the pointer is now, read in the list's own coordinates:
			// the scroll may have moved under it while the drag went on.
			_, py, _ := list.PointerPosition()
			d.at = dropIndex(py, rows, tops, d.key)
		}
		if droppedFrom != "" {
			w.rowDrag = nil
			w.moveSessionIndex(m.ID, droppedFrom, d.at)
			return
		}
		if !dragging {
			// Let go where no row of this group took it: the preview goes back
			// to the order the list had.
			w.rowDrag = nil
			w.invalidate()
			return
		}
	}
	// A note stands in for rows only once the group is open: on the way out it
	// would be the one thing left behind in a group that is leaving.
	if folded == 1 && len(entries) == 0 && !quiet {
		note := locales.SidebarMachineEmpty()
		if m.ID != machineLocal && len(w.remoteSessions[m.ID]) == 0 {
			note = locales.SidebarMachineLook()
		}
		row := ui.Row(c).Height(30).Padding(0, 12).AlignItems(ui.Center).Label(note)
		row.Children(func() {
			ui.Text(c, note).FontSize(12).TextColor(pal.TextFaint).SingleLine().Ellipsis("…")
		})
		if row.Clicked() && m.ID != machineLocal {
			w.selectMachine(m)
			w.refreshSessions(m.ID)
		}
	}
}

// refreshButton is a machine group's refresh: the arrow at rest, a turning ring
// while the machine's list is being asked for, and the arrow again once the list
// lands. A click that gave no sign would read as a click that did nothing, which
// is exactly what a list that looks stale already claims.
func refreshButton(c *ui.Context, pal *uitokens.Palette, turning bool, caption string) ui.Element {
	t := c.Theme()
	el := ui.Box(c).Size(20, 20).Radius(radiusSmall).
		AlignItems(ui.Center).Justify(ui.Center).Label(caption).Tooltip(caption)
	if el.Hovered() {
		el.Background(t.SurfaceHover)
	}
	el.Children(func() {
		if !turning {
			ui.Icon(c, iconRefresh).FontSize(11).TextColor(shellColorsFor(t).icon)
			return
		}
		// The ring turns a third of a second a revolution, in the same language
		// as a working shell's own ring: a ring that never lands, because the
		// answer is what stops it.
		ui.Box(c).Size(14, 14).Draw(func(p *ui.Painter, r ui.Rect) {
			p.AnimationFrame()
			cx, cy := r.X+r.W/2, r.Y+r.H/2
			rad := min(r.W, r.H)/2 - 1.2
			var track ui.Path
			track.Circle(cx, cy, rad)
			p.StrokePath(&track, 1.5, pal.Text.Alpha(0.15))
			deg := float32(p.Now().UnixMilli()%900) / 900 * 360
			p.Transform(ui.Rotation(deg), cx, cy, func() {
				var arc ui.Path
				arc.Circle(cx, cy, rad)
				p.StrokePath(arc.Trim(0, 0.3), 1.5, pal.Text)
			})
		})
	})
	return el
}

// machineDot is a machine's state in one colour: the local service's own three
// answers for this machine, online or gone for the rest.
func (w *window) machineDot(c *ui.Context, m tunnel.Machine) ui.Color {
	t := c.Theme()
	pal := paletteFor(c)
	if m.ID == machineLocal {
		return w.fleetColor(t)
	}
	if !m.Online {
		return pal.Danger
	}
	return pal.Success
}

// machineTooltip is the whole story of a machine, for the header that has no
// room for it.
func (w *window) machineTooltip(m tunnel.Machine) string {
	state := machineState(m)
	if m.ID == machineLocal {
		name, meta := w.localMachine()
		return name + " · " + meta + versionSuffix(m.Host) + "\n" + state
	}
	return m.Name + " · " + m.ID + versionSuffix(m.Host) + "\n" + state
}

// versionSuffix names the build a machine runs, when it said. A machine too old
// to answer with one says nothing rather than "unknown": the absence is the
// fact, and the sentence around it already says the machine is reachable.
func versionSuffix(h protocol.Host) string {
	if h.Version == "" {
		return ""
	}
	return " · v" + h.Version
}

// newTabOn opens a shell on a machine, picking it first: the panels and the new
// shell then agree about what the window is working on.
func (w *window) newTabOn(machine string) {
	if w.selectedMachine() != machine {
		if m, ok := w.machineByID(machine); ok {
			w.selectMachine(m)
		}
	}
	w.newTab()
}

// sidebarFooter is the foot: the buttons for a shell, the palette and the
// settings; then, at the right, how the fleet stands — a quiet footer, in
// place of the band across the bottom of the window.
func (w *window) sidebarFooter(c *ui.Context, pal *uitokens.Palette) {
	ui.Row(c).Height(uitokens.FooterH).Shrink(0).Padding(0, 12, 4, 12).Gap(2).AlignItems(ui.Center).DragWindow().Children(func() {
		if iconButton(c, iconSearch, locales.ActionPalette(), 26).Clicked() {
			w.openPalette()
		}
		if iconButton(c, iconSettings, locales.ActionSettings(), 26).Clicked() {
			w.openSettings()
		}
		ui.Spacer(c)
		// What across the sessions wants a look, each a button going to the next
		// session of it, and how the fleet stands when nothing does.
		waiting, working, done := w.statusCounts()
		// One number stays out, the one that wants a hand; the other two are in
		// its tooltip, where they do not compete for 268 points of footer.
		all := locales.FooterNeedsYou() + " " + itoa(waiting) + " · " +
			locales.FooterWorking() + " " + itoa(working) + " · " +
			locales.FooterDone() + " " + itoa(done)
		if waiting > 0 {
			row := ui.Row(c).Gap(5).Padding(0, 6).AlignItems(ui.Center).Radius(12).
				Label(locales.FooterNeedsYou() + " " + itoa(waiting)).Tooltip(all)
			if row.Hovered() {
				row.Background(pal.RowHover)
			}
			row.Children(func() {
				statusGlyph(c, pal, "waiting", false, -1, 12)
				ui.Textf(c, "%d", waiting).FontSize(12).FontWeight(500).TextColor(pal.Attention).FontFeatures("tnum")
			})
			if row.Clicked() {
				w.jumpTo("waiting")
			}
		}
		if waiting == 0 {
			row := ui.Row(c).Gap(6).Padding(0, 6).AlignItems(ui.Center).
				Label(w.serviceLine()).Tooltip(w.machineNote() + "\n" + all)
			row.Children(func() {
				ui.Box(c).Size(6, 6).Radius(3).Background(w.fleetColor(c.Theme()))
				ui.Textf(c, "%d", w.machineCount()).FontSize(12).TextColor(pal.TextFaint).FontFeatures("tnum")
			})
			if row.Clicked() {
				w.refreshMachines()
			}
		}
	})
}
