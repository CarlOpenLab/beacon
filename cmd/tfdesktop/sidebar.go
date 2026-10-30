package main

import (
	"hash/fnv"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

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
	// topControlInset keeps the three title controls clear of the traffic lights
	// and gives the group a little breathing room from the window edge.
	topControlInset = float32(20)
	// topControlsWidth is the fixed footprint of search, settings and fold.
	// Keeping this width explicit prevents flex layout from squeezing the SVGs
	// while the sidebar is being resized or folded.
	topControlsWidth = float32(84)
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
	x := max(hidden, round(hidden+(shownAt-hidden)*shown))
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

// sidebar draws the column: the window's buttons, the selected machine's
// sessions, and the rail of machines along the bottom.
func (w *window) sidebar(c *ui.Context, pal *uitokens.Palette) {
	bar := c.TitleBar()
	topHeight := max(float32(uitokens.TopBar), bar.Height)
	ch := w.chromeNow(c)
	w.reconcileHidden(c)

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
			// Keep the title row itself as a drag surface. The fixed toolbar is
			// drawn above it, so its buttons never participate in flex sizing.
			ui.Box(c).Width(w.sideWidth()).Height(topHeight).Shrink(0).DragWindow()
			w.sidebarToolbar(c, pal, false, w.sideWidth())

			ui.Scroll(c).Grow(1).MinHeight(0).Padding(0, 10, 16, 10).Children(func() {
				w.sidebarMachines(c, pal)
			})
			w.machineRail(c, pal)
		})
	})
}

// sidebarToolbar is a fixed, non-flexing title toolbar. Keeping it out of the
// resizable row is what prevents the SVG buttons from shrinking or jumping.
func (w *window) sidebarToolbar(c *ui.Context, pal *uitokens.Palette, hidden bool, width float32) {
	row := ui.Row(c).Key("sidebar-toolbar").Absolute().
		Top((max(float32(uitokens.TopBar), c.TitleBar().Height) - toolbarButton) / 2).
		Height(toolbarButton).Gap(2).AlignItems(ui.Center)
	if hidden {
		// Once the column is gone the card owns the title bar. Keep the fixed
		// toolbar in the same left-hand title slot until the sidebar returns.
		row.Left(titleBarLead(c.TitleBar()) + topControlInset).
			Width(topControlsWidth).Shrink(0)
	} else {
		// While the column is visible the controls are flex-end aligned. Their
		// right edge follows the panel resize instead of looking pinned to the
		// window's left edge.
		row.Left(0).Width(width).Padding(0, 12).Justify(ui.End).Shrink(0)
	}
	row.Children(func() {
		if railIconButton(c, pal, iconSearch, locales.ActionPalette()).Clicked() {
			w.openPalette()
		}
		if railIconButton(c, pal, iconSettings, locales.ActionSettings()).Clicked() {
			w.openSettings()
		}
		if iconButtonWithHover(c, iconSidebar, locales.ActionToggleSidebar(), toolbarButton, pal.RowHover).Clicked() {
			w.sidebarHidden = !hidden
		}
	})
}

// resizeHandle is the column's edge, which a drag moves and a double click puts
// back.
func (w *window) resizeHandle(c *ui.Context, pal *uitokens.Palette) {
	h := ui.Box(c).Key("sidebar-resize").Absolute().Left(w.sideWidth() - 4).
		Top(max(float32(uitokens.TopBar), c.TitleBar().Height)).Bottom(machineRailH + 2*machineRailPad).
		Width(8).Cursor(ui.CursorResizeEW)
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

// sidebarMachines draws the body of the column: one machine's sessions. The
// machine is the one chosen in the rail, always — a fleet has no useful combined
// list once every machine is a workspace of its own.
func (w *window) sidebarMachines(c *ui.Context, pal *uitokens.Palette) {
	m, ok := w.railMachine(c.Now())
	if !ok {
		w.sidebarEmpty(c, pal)
		return
	}
	// The body is keyed by machine so a switch is a cross-fade rather than a
	// list that changes under the eye: the old body leaves into a copy that fades
	// out, the new one fades in, and neither slides sideways. The two machines
	// have no left-right relation, and a slide would pretend they did.
	body := ui.Column(c).Key("sidebar-machine:" + m.ID).FillWidth().
		Transition(ui.ElementTransition{
			Duration: machineSwitchDuration,
			Colors:   true,
			Enter:    &ui.Motion{Opacity: 0},
			Exit:     &ui.Motion{Opacity: 0},
		})
	body.Children(func() {
		w.machineGroup(c, pal, m, true, false, false)
	})
}

// railMachines is the rail's order: the machines the column knows, minus the
// ones the person hid, and with the hidden exception applied. It is not sorted
// by state: a row of tiles that reorders itself as sessions come and go would
// move the machine under the pointer that was about to click it.
func (w *window) railMachines(now time.Time) []tunnel.Machine {
	out := make([]tunnel.Machine, 0, len(w.roster)+1)
	for _, m := range w.groups() {
		if w.machineShown(m.ID, now) {
			out = append(out, m)
		}
	}
	return out
}

// railMachine is the machine the column is about: the chosen one while it is
// still in the rail, and the first one in the rail when it is not. A machine
// that disappears — removed from the roster, or hidden while it has no session
// that needs the user — must not leave the column showing a machine that is no
// longer there.
func (w *window) railMachine(now time.Time) (tunnel.Machine, bool) {
	machines := w.railMachines(now)
	if len(machines) == 0 {
		return tunnel.Machine{}, false
	}
	selected := w.selectedMachine()
	for _, m := range machines {
		if m.ID == selected {
			return m, true
		}
	}
	w.machineSel = machines[0].ID
	return machines[0], true
}

// The machine rail centers short rosters and scrolls longer ones sideways.
const (
	machineRailH      = float32(34)
	machineRailPad    = float32(4)
	machineTileSize   = float32(26)
	machineTileRadius = float32(7)
	machineRailFade   = float32(14)
	machineGlyphSize  = float32(14)
	machinePetSize    = float32(16)
	machineRailButton = float32(26)
	machineTileGap    = float32(4)
)

// machineSwitch is the 120ms the body takes to cross-fade from one machine to
// the next, and the same time the chosen tile takes to invert. It is short
// enough to read as a change of place rather than a performance.
const machineSwitchDuration = 120 * time.Millisecond

var machineSwitch = ui.Animation{Duration: machineSwitchDuration}

// machineRail keeps every machine available without an overflow menu.
func (w *window) machineRail(c *ui.Context, pal *uitokens.Palette) {
	// The strip floats a little above the window's edge: a rail glued to the
	// bottom reads as part of the frame, not as a row of machines.
	ui.Row(c).Key("machine-rail").Label("machine rail").Height(machineRailH+2*machineRailPad).Shrink(0).
		Margin(0, 0, 6, 0).
		Padding(machineRailPad, 12).
		AlignItems(ui.Center).Children(func() {
		machines := w.railMachines(c.Now())
		selected := w.selectedMachine()
		strip := ui.Box(c).Grow(1).MinWidth(0).FillHeight().ClipX()
		strip.Children(func() {
			scroller := ui.ScrollHorizontal(c).Fill().TrackScroll(&w.railScroll)
			scroller.Children(func() {
				contentWidth := float32(len(machines))*(machineTileSize+8) + float32(max(0, len(machines)-1))*machineTileGap
				tiles := ui.Row(c).Width(max(w.sideWidth()-24, contentWidth)).Shrink(0).
					Height(machineRailH).Gap(machineTileGap).AlignItems(ui.Center).Justify(ui.Center).
					Role(ui.RoleTabList).FocusGroup(ui.Horizontal)
				tiles.Children(func() {
					for _, m := range machines {
						w.machineRailTile(c, pal, m, m.ID == selected)
					}
				})
			})
		})
	})
}

// railIconButton is one of the rail's two fixed buttons: a 24-point glyph in a
// 26-point frame. It is the standalone tier — the same size as the machine
// glyphs beside it — rather than the 16-point tier an icon beside a word uses.
func railIconButton(c *ui.Context, pal *uitokens.Palette, icon *ui.SVG, caption string) ui.Element {
	el := ui.Box(c).Size(machineRailButton, machineRailButton).Shrink(0).Radius(machineRailButton / 2).
		AlignItems(ui.Center).Justify(ui.Center).Label(caption).Tooltip(caption)
	if el.Hovered() {
		// A wash, not a surface: the column is acrylic, so its hover has to let
		// the material through rather than cut a solid roundel out of it.
		el.Background(pal.RowHover)
	}
	el.Children(func() {
		ui.Icon(c, icon).Size(machineGlyphSize, machineGlyphSize).TextColor(pal.TextMuted)
	})
	return el
}

// machineRailTile draws one machine: a fixed square with its glyph, its state
// as a small badge, and nothing else. The whole tile is the hit target — a
// dot-sized target would be a target only a careful hand could find.
func (w *window) machineRailTile(c *ui.Context, pal *uitokens.Palette, m tunnel.Machine, selected bool) {
	// The slot is a little wider than the tile: the badge hangs off the tile's
	// corner, and scrolling a tile into view has to bring the badge with it.
	slot := ui.Box(c).Key("rail-slot:"+m.ID).Size(machineTileSize+8, machineRailH).Shrink(0).
		AlignItems(ui.Center).Justify(ui.Center)
	slot.Children(func() {
		// The pet draws bottom-heavy — it sits on its canvas — so the tile
		// carries two points of padding under it: centered, the drawing reads a
		// point high of the circle's middle, the way a sitting pet does.
		tile := ui.Box(c).Key("rail:"+m.ID).Size(machineTileSize, machineTileSize).
			Radius(machineTileSize / 2).Padding(0, 0, 2, 0).Center().Shrink(0).
			Focusable().Role(ui.RoleTab).Selected(selected).
			Label(w.machineRailLabel(m)).Tooltip(w.machineTooltip(m)).
			Cursor(ui.CursorPointer)
		state := w.railDotState(m)
		bg := machineStateBg(c, state, m.ID == machineLocal || m.Online)
		switch {
		case tile.Pressed():
			if state == "" {
				bg = machinePressedBg(c)
			} else {
				bg = bg.Alpha(2.6)
			}
		case selected:
			// An offline machine chosen is chosen all the same: the active wash
			// is the blue every other machine gets — presence is what the
			// faint pet already says, not something the wash must repeat.
			if state == "offline" {
				bg = machineActiveBg(c)
			} else {
				bg = machineActiveStateBg(c, state)
			}
			if tile.Hovered() {
				bg = bg.Alpha(1.15)
			}
		case tile.Hovered():
			if state == "" {
				bg = machineHoverBg(c)
			} else {
				bg = bg.Alpha(1.5)
			}
		}
		tile.Background(tile.AnimateColor("bg", bg, machineSwitch))
		if state == "waiting" && !selected {
			tile.Shadow(0, 0, 0, 1.5, pal.Attention.Alpha(0.55)).Shadow(0, 1, 3, 0, pal.AttnSoft)
		}
		fg := pal.TextMuted
		if selected {
			fg = pal.Text
		} else if state == "offline" {
			// No badge: the pet is the machine's identity and carries no state
			// of its own — the tile's wash already tells waiting from failed
			// from pending, in one place and at full strength. An offline pet
			// sleeps at the faint tier, presence rather than alarm — unless it
			// is the chosen one, which reads as chosen first.
			fg = pal.TextFaint
		}
		tile.TextColor(fg)
		tile.Children(func() {
			ui.Icon(c, w.railGlyph(m)).Size(machinePetSize, machinePetSize).TextColor(fg)
		})
		// Right-click is the picker: no middleman menu naming a pet anyone can
		// already see. The builder adds no items, and a menu with none is one
		// the engine never shows — the request ends here, and the panel below
		// answers it in this same frame.
		tile.ContextMenu(func(mu *ui.Menu) {
			w.petPickerFor = m.ID
			w.petPickerSite = petPickerRail
			w.petPickerOpen = true
		})
		if selected {
			// The chosen machine is the one fact the strip must never hide: if the
			// person scrolls it out, it comes back rather than becoming merely one of
			// the tiles off the edge.
			slot.ScrollIntoView()
		}
		if tile.Clicked() {
			w.chooseMachine(m)
		}
		if w.petPickerOpen && w.petPickerFor == m.ID && w.petPickerSite == petPickerRail {
			w.petPicker(c, tile, m, true)
		}
	})
}

// machineActiveBg is the selected tile's solid-but-quiet surface. It is kept
// separate from the state washes so selecting an offline or waiting machine
// does not make the active treatment look like a warning badge.
func machineActiveBg(c *ui.Context) ui.Color {
	if c.Theme().Dark {
		return ui.RGBA(154, 190, 242, 0.28)
	}
	return ui.RGBA(35, 76, 150, 0.22)
}

func machineActiveStateBg(c *ui.Context, state string) ui.Color {
	if state != "" {
		return machineStateBg(c, state, true).Alpha(2.1)
	}
	return machineActiveBg(c)
}

// machineHoverBg is half the selected tint: hover says "a click lands here"
// without looking like the tile you already chose, so two tiles never read as
// chosen at once.
func machineHoverBg(c *ui.Context) ui.Color {
	if c.Theme().Dark {
		return ui.RGBA(154, 190, 242, 0.12)
	}
	return ui.RGBA(35, 76, 150, 0.07)
}

// machinePressedBg is that tint a step darker while the button is down, so a
// press is felt as well as acted on.
func machinePressedBg(c *ui.Context) ui.Color {
	if c.Theme().Dark {
		return ui.RGBA(154, 190, 242, 0.34)
	}
	return ui.RGBA(35, 76, 150, 0.28)
}

// machineStateBg gives the rail a quiet wash for states that otherwise differ
// only by a one-pixel glyph. It is intentionally softer than selected/hover so
// the chosen tile remains the strongest visual anchor.
func machineStateBg(c *ui.Context, state string, online bool) ui.Color {
	if c.Theme().Dark {
		switch state {
		case "waiting":
			return ui.RGBA(245, 166, 35, 0.22)
		case "failed":
			return ui.RGBA(255, 105, 97, 0.15)
		case "running":
			return ui.RGBA(79, 209, 122, 0.10)
		case "done":
			return ui.RGBA(107, 147, 255, 0.11)
		case "pending":
			return ui.RGBA(145, 125, 255, 0.20)
		case "offline":
			return ui.RGBA(130, 132, 145, 0.07)
		}
		if online {
			return ui.RGBA(79, 209, 122, 0.05)
		}
		return ui.RGBA(120, 120, 132, 0.08)
	}
	switch state {
	case "waiting":
		return ui.RGBA(232, 154, 40, 0.22)
	case "failed":
		return ui.RGBA(217, 58, 53, 0.14)
	case "running":
		return ui.RGBA(34, 134, 58, 0.10)
	case "done":
		return ui.RGBA(59, 108, 246, 0.10)
	case "pending":
		return ui.RGBA(107, 85, 210, 0.18)
	case "offline":
		return ui.RGBA(110, 110, 125, 0.07)
	}
	if online {
		return ui.RGBA(34, 134, 58, 0.045)
	}
	return ui.RGBA(110, 110, 125, 0.08)
}

// railDotState is the state the badge shows, ranked failed > needs-you >
// running > done > offline. Work in progress outranks work finished unseen,
// and a machine that is not answering is the quietest fact of all: only a
// machine with nothing else to say wears the offline dot.
func (w *window) railDotState(m tunnel.Machine) string {
	waiting, working, done, failed, _ := w.machineSessionCounts(m.ID)
	switch {
	case failed > 0:
		return "failed"
	case waiting > 0:
		return "waiting"
	case working > 0:
		return "running"
	case done > 0:
		return "done"
	case w.machineHasPending(m.ID):
		return "pending"
	case m.ID != machineLocal && !m.Online:
		return "offline"
	}
	return ""
}

func (w *window) machineHasPending(id string) bool {
	for _, tb := range w.tabs {
		if tb != nil && tb.machine == id && tb.isPending() {
			return true
		}
	}
	return false
}

// machinePlatformName turns the host's own word for its system into the one a
// person reads. Anything unrecognised is left out rather than shown raw.
func machinePlatformName(platform string) string {
	switch strings.ToLower(platform) {
	case "darwin", "macos", "mac":
		return "macOS"
	case "windows", "win32":
		return "Windows"
	case "linux":
		return "Linux"
	}
	return ""
}

// machineRailLabel is what assistive technology reads: the machine's full name
// and its state in words, never the glyph and never a single character. The
// glyph is decoration; the name and the state are the facts.
func (w *window) machineRailLabel(m tunnel.Machine) string {
	return w.machineDisplayName(m.ID) + locales.MachineListSep() + w.machineStatusWord(m)
}

// machineStatusWord is the one word that stands for a machine's state, in the
// same order the rail's badge ranks them: failed, needs you, running, done,
// offline, and quiet. The label and the dot therefore cannot disagree about
// which state a machine is in.
func (w *window) machineStatusWord(m tunnel.Machine) string {
	waiting, working, done, failed, _ := w.machineSessionCounts(m.ID)
	switch {
	case failed > 0:
		return locales.StatusFailed()
	case waiting > 0:
		return locales.FooterNeedsYou()
	case working > 0:
		return locales.FooterWorking()
	case done > 0:
		return locales.StatusDone()
	case w.machineHasPending(m.ID):
		return locales.StatePending()
	case m.ID != machineLocal && !m.Online:
		return locales.MachineOffline()
	}
	return locales.StatusIdle()
}

// machineSessionCounts is how a machine stands, counted from the rows it is
// showing. It is one walk of the group so that the badge, the word and the
// tooltip cannot disagree about what they saw.
func (w *window) machineSessionCounts(machine string) (waiting, working, done, failed, idle int) {
	for _, e := range w.entriesFor(machine) {
		switch e.status {
		case "waiting":
			waiting++
		case "working":
			working++
		case "error":
			failed++
		case "idle":
			if e.unread {
				done++
			} else {
				idle++
			}
		default:
			idle++
		}
	}
	return waiting, working, done, failed, idle
}

// machineDisplayName is the full name a machine is called by here: its own
// name, plus the shortest tail that tells it from another machine with the same
// name. Two "studio" machines are told apart by their ids rather than renamed,
// and neither is hidden behind a suffix longer than it has to be.
func (w *window) machineDisplayName(id string) string {
	return w.machineDisplayNames()[id]
}

func (w *window) machineDisplayNames() map[string]string {
	groups := make(map[string][]tunnel.Machine)
	var order []string
	for _, m := range w.groups() {
		name := m.Name
		if name == "" {
			name = w.machineName(m.ID)
		}
		name = machineDisplayHost(name, m.ID)
		if _, seen := groups[name]; !seen {
			order = append(order, name)
		}
		groups[name] = append(groups[name], m)
	}
	out := make(map[string]string, len(w.groups()))
	for _, name := range order {
		list := groups[name]
		if len(list) == 1 {
			out[list[0].ID] = name
			continue
		}
		for _, m := range list {
			out[m.ID] = name + " · " + shortestSuffix(m.ID, list)
		}
	}
	return out
}

// machineDisplayHost is a hostname as a person should see it: without the mDNS
// suffix a local network adds, and with the id standing in when the name was
// nothing but that suffix. It changes no other name, so a machine its owner
// called something keeps that word.
func machineDisplayHost(name, id string) string {
	name = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(name), "."))
	lower := strings.ToLower(name)
	for _, suffix := range []string{".localdomain", ".local"} {
		if strings.HasSuffix(lower, suffix) {
			name = name[:len(name)-len(suffix)]
			break
		}
	}
	if name == "" {
		return id
	}
	return name
}

// shortestSuffix is the least tail of id that no other machine in the group
// shares. Ids are device ids or hostnames, and their ends are usually the part
// that differs, so a tail reads more naturally than a prefix.
func shortestSuffix(id string, group []tunnel.Machine) string {
	runes := []rune(id)
	for n := 1; n <= len(runes); n++ {
		suffix := string(runes[len(runes)-n:])
		unique := true
		for _, other := range group {
			if other.ID == id {
				continue
			}
			others := []rune(other.ID)
			if len(others) > n {
				others = others[len(others)-n:]
			}
			if string(others) == suffix {
				unique = false
				break
			}
		}
		if unique {
			return suffix
		}
	}
	return id
}

// machineIconGlyph finds a pet by its settings key.
func machineIconGlyph(key string) (*ui.SVG, bool) {
	g, ok := petGlyphs[key]
	return g, ok
}

// machineIconName is the automatic rule: the machine wears the pet its name
// hashes to. Pets are identities, not kinds — a slime is not a database — so
// the name's meaning does not matter, only that it is the name the person gave
// it: the same name lands on the same pet across restarts, and a fleet spreads
// across the catalog instead of everyone wearing the first pet. The suffix a
// local network appends does not move the pet: studio and studio.local are one
// machine. A pet picked in the menu wins over the hash.
func machineIconName(name string) string {
	h := fnv.New32a()
	h.Write([]byte(strings.ToLower(machineDisplayHost(name, name))))
	return petCatalog[h.Sum32()%uint32(len(petCatalog))].key
}

// railGlyph is the one seam between a machine and its drawing. A picked pet
// wins; a machine nobody picked for wears the pet its name hashes to. A new
// rule for how pets are assigned belongs here and nowhere else.
func (w *window) railGlyph(m tunnel.Machine) *ui.SVG {
	if glyph := petGlyphs[w.machineIconKey(m)]; glyph != nil {
		return glyph
	}
	return petGlyphs[petCatalog[0].key]
}

// machineIconKey is the glyph the menu ticks: the picked one, else the one the
// name rule gives.
func (w *window) machineIconKey(m tunnel.Machine) string {
	if key := w.machineIcons[m.ID]; key != "" {
		if _, ok := machineIconGlyph(key); ok {
			return key
		}
	}
	return machineIconName(w.machineName(m.ID))
}

// setMachineIcon stores a picked glyph and applies it now. The store is the
// source of truth, so a restart and a reconnected machine keep the choice; a
// machine with no entry follows its name again.
func (w *window) setMachineIcon(id, key string) error {
	if _, ok := machineIconGlyph(key); !ok {
		return nil
	}
	if w.store != nil {
		if _, err := w.store.Update(func(st *settings.Settings) {
			if st.MachineIcons == nil {
				st.MachineIcons = make(map[string]string)
			}
			st.MachineIcons[id] = key
		}); err != nil {
			return err
		}
	}
	if w.machineIcons == nil {
		w.machineIcons = make(map[string]string)
	}
	w.machineIcons[id] = key
	w.invalidate()
	return nil
}

// The two places that offer the picker: the rail's tile draws the panel above
// itself — the rail owns the window's bottom edge, so the panel has nowhere
// else to go — and the machine group's header draws it below, as popovers
// default to.
const (
	petPickerRail   = "rail"
	petPickerHeader = "header"
)

// petPickerWidth fits one family of eight: cells of 34 with 6 between them,
// and the panel's own padding of 14 on each side.
const petPickerWidth = 8*34 + 7*6 + 2*14

// petPicker is the panel that shows the catalog as drawings: four families of
// eight, every pet on a tile of its own with its name under it. A pet is a
// drawing, and a drawing chooses by being seen — the system menu this replaces
// could name a 史莱姆 but never show one. The machine's current pet wears the
// wash the rail's chosen tile wears, so the panel and the strip agree on which
// pet is on.
func (w *window) petPicker(c *ui.Context, anchor ui.Element, m tunnel.Machine, above bool) {
	pal := paletteFor(c)
	current := w.machineIconKey(m)
	var families [][]petIcon
	for _, p := range petCatalog {
		if n := len(families); n > 0 && families[n-1][0].series == p.series {
			families[n-1] = append(families[n-1], p)
			continue
		}
		families = append(families, []petIcon{p})
	}
	ui.PopoverBase(c, anchor, &w.petPickerOpen, func(panel ui.Element) {
		if above {
			panel.AttachTo(anchor, ui.AnchorTop, ui.AnchorBottom)
		}
		panel.Label("pet-picker")
		floating(panel.Width(petPickerWidth).Radius(14).Background(pal.Popover).Padding(14), pal)
		ui.Column(c).Gap(12).Children(func() {
			ui.Text(c, locales.MachineIconMenu()).FontSize(12).FontWeight(600).TextColor(pal.Text)
			for _, family := range families {
				ui.Row(c).Gap(6).Children(func() {
					for _, p := range family {
						w.petPickerCell(c, m, p, current)
					}
				})
			}
		})
	})
}

// petPickerCell is one pet of the grid: the drawing on a tile of its own, the
// name under it. Choosing one stores it with the machine and closes the panel.
func (w *window) petPickerCell(c *ui.Context, m tunnel.Machine, p petIcon, current string) {
	pal := paletteFor(c)
	ui.Column(c).Gap(4).AlignItems(ui.Center).Children(func() {
		cell := ui.Box(c).Size(34, 34).Radius(10).Center().
			Label(p.zh + " " + p.en).Cursor(ui.CursorPointer)
		bg := ui.Transparent
		if p.key == current {
			bg = machineActiveBg(c)
		} else if cell.Hovered() {
			bg = machineHoverBg(c)
		}
		cell.Background(cell.AnimateColor("bg", bg, ui.Animation{Duration: 90e6}))
		if p.key == current {
			chosen(cell, pal)
		}
		cell.Children(func() {
			ui.Icon(c, petGlyphs[p.key]).Size(machinePetSize, machinePetSize).TextColor(pal.Text)
		})
		if cell.Clicked() {
			if err := w.setMachineIcon(m.ID, p.key); err != nil {
				w.setNotice(err.Error())
			}
			w.petPickerOpen = false
		}
		ui.Text(c, p.name()).FontSize(11).TextColor(pal.TextMuted).SingleLine()
	})
}

// jumpRail is ⌘1…⌘9: the nth machine in the strip, in the strip's order. A
// number past the last machine does nothing rather than wrapping — pressing ⌘7
// with three machines is asking for a machine that is not there.
func (w *window) jumpRail(c *ui.Context, n int) {
	machines := w.railMachines(c.Now())
	if n < 0 || n >= len(machines) {
		return
	}
	w.chooseMachine(machines[n])
}

// chooseMachine switches the column to a machine and then to the session that
// needs the person there, if there is one. Picking a machine is one act — the
// dot is not a second target — and landing on the waiting row is what makes the
// act useful: the person does not arrive at a list and start looking.
func (w *window) chooseMachine(m tunnel.Machine) {
	w.selectMachine(m)
	if w.reachPrompt != "" {
		return
	}
	w.landOnNeed(m.ID)
}

// landOnNeed chooses the first session on a machine that wants an answer and
// asks the sidebar to scroll it into view. The first is the oldest in the list's
// order, which is the one that has been waiting longest.
func (w *window) landOnNeed(machine string) {
	for _, e := range w.entriesFor(machine) {
		if e.status != "waiting" {
			continue
		}
		delete(w.collapsed, machine)
		w.railLand = e.machine + "\x00" + sessionKey(e)
		e.pick()
		w.invalidate()
		return
	}
}

// machineWaiting reports whether a machine has a session that needs the person,
// which is the only state ⌘J walks the fleet for.
func (w *window) machineWaiting(id string) bool {
	waiting, _, _, _, _ := w.machineSessionCounts(id)
	return waiting > 0
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
	// Whether a hand asked for this fold is read before the header is drawn: the
	// click that asks for it happens while the header is being drawn, and its
	// fold is the one that starts on the next frame.
	byHand := w.foldByHand[m.ID]

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
		// The pet is a hint and sometimes a wrong one. The picker is how a
		// person corrects it; the choice is stored with the machine's other
		// settings and outlives a reconnect.
		if mu.Item(locales.MachineIconMenu()).Chosen() {
			w.petPickerFor = m.ID
			w.petPickerSite = petPickerHeader
			w.petPickerOpen = true
		}
		if mu.Item(locales.ActionHideMachine()).Chosen() {
			if err := w.setMachineHidden(m.ID, true); err != nil {
				w.setNotice(err.Error())
			}
		}
	})
	if w.petPickerOpen && w.petPickerFor == m.ID && w.petPickerSite == petPickerHeader {
		w.petPicker(c, hdr, m, false)
	}
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
		ui.Text(c, w.machineDisplayName(m.ID)).FontSize(12.5).FontWeight(600).TextColor(pal.Text).
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
	if byHand {
		foldAnim = groupFold
	}
	target := float32(b2f(!collapsed))
	folded := hdr.AnimateTo("fold", target, foldAnim)
	if byHand && folded == target {
		// The fold has arrived: the next one is a jump unless a hand asks again.
		delete(w.foldByHand, m.ID)
	}
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
		if t.Dark {
			el.Background(ui.RGBA(255, 255, 255, 0.12))
		} else {
			el.Background(ui.RGBA(0, 0, 0, 0.08))
		}
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

// machineTooltip is the whole story of a machine, for the tile that has no
// room for it: the full name, the platform and build it runs when it said, and
// what it is doing now. The counts are this machine's own — the fleet-wide
// footer they used to share a line with is gone — and the words are the same
// words the tile's label and the status glyph use, so the three cannot drift
// apart.
func (w *window) machineTooltip(m tunnel.Machine) string {
	meta := []string{}
	if p := machinePlatformName(m.Host.Platform); p != "" {
		meta = append(meta, p)
	}
	if m.Host.Version != "" {
		meta = append(meta, "v"+m.Host.Version)
	}
	name := w.machineDisplayName(m.ID)
	if len(meta) > 0 {
		name += " · " + strings.Join(meta, " · ")
	}
	// Only the counts that are somebody's answer. "Needs you 0 · running 0 ·
	// idle 3" makes an idle machine look like something went wrong, and a
	// machine whose only session failed must not be called idle here while its
	// glyph is red and a screen reader says "failed". The five states are
	// counted in machineStatusWord's own order, so the first line is always a
	// subset of the word the label speaks. Offline is not a session's state and
	// is not counted.
	waiting, working, done, failed, idle := w.machineSessionCounts(m.ID)
	parts := []string{}
	for _, item := range []struct {
		word  string
		count int
	}{
		{locales.StatusFailed(), failed},
		{locales.FooterNeedsYou(), waiting},
		{locales.FooterWorking(), working},
		{locales.StatusDone(), done},
		{locales.StatusIdle(), idle},
	} {
		if item.count > 0 {
			parts = append(parts, item.word+" "+itoa(item.count))
		}
	}
	// The line is written around the word the glyph and the screen reader are
	// saying. When no session is in any state, that word is the whole line: a
	// machine that is offline says "离线", not "空闲" — being unreachable is
	// not being idle. Counting offline is still not done: it is not a session's
	// state, it is only ever the answer when nothing else is.
	top := w.machineStatusWord(m)
	if len(parts) == 0 || !strings.HasPrefix(parts[0], top) {
		parts = append([]string{top}, parts...)
	}
	state := strings.Join(parts, " · ")
	return name + "\n" + state
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
