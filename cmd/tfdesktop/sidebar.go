package main

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/tunnel"
	"github.com/CarlOpenLab/term-fleet/internal/uitokens"
	"github.com/CarlOpenLab/term-fleet/locales"
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
				for i, m := range w.groups() {
					w.machineGroup(c, pal, m, i)
				}

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

// machineGroup is one machine and, under it, its sessions: a project group,
// with the machine's state in the header and a "+" that opens a shell there.
func (w *window) machineGroup(c *ui.Context, pal *uitokens.Palette, m tunnel.Machine, index int) {
	selected := w.selectedMachine() == m.ID
	collapsed := w.collapsed[m.ID]
	entries := w.entriesFor(m.ID)

	top := float32(18)
	if index == 0 {
		top = 2
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
	})
	if hdr.Clicked() {
		if !selected {
			w.selectMachine(m)
			delete(w.collapsed, m.ID)
		} else {
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
		ui.Text(c, name).FontSize(12.5).FontWeight(600).TextColor(pal.TextMuted).
			SingleLine().Ellipsis("…").MinWidth(0).Shrink(1)
		ui.Textf(c, "%d", len(entries)).FontSize(12.5).TextColor(pal.TextFaint).Shrink(0)
		chev := ui.Icon(c, iconChevron).Size(11, 11).TextColor(pal.TextFaint)
		chev.Rotate(chev.AnimateTo("turn", 90*float32(b2f(!collapsed)), ui.Snappy))
		chev.Opacity(chev.Animate("shown", float32(b2f(hovered || collapsed)), 120e6))
		ui.Spacer(c)
		// Ask this machine for its session list again. It stays on the group at
		// rest: a refresh is the answer to a list that looks stale, and a button
		// that only appears under the pointer is not there when it is needed.
		if refreshButton(c, pal, w.refreshing(m.ID), locales.SessionRefreshMachine()).Clicked() {
			w.refreshSessions(m.ID)
		}
		if iconButton(c, iconPlus, locales.SessionNewOnMachine(), 20).Clicked() {
			w.newTabOn(m.ID)
		}
	})
	if collapsed {
		return
	}
	for _, e := range entries {
		w.sessionRow(c, pal, e)
		// The first pane is the tab's own row; the others hang under it.
		for i, pane := range e.panes {
			if i == 0 {
				continue
			}
			w.paneRow(c, pal, pane, i == len(e.panes)-1)
		}
	}
	if len(entries) == 0 {
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
		return name + " · " + meta + "\n" + state
	}
	return m.Name + " · " + m.ID + "\n" + state
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
		chip := func(kind, label string, n int, color ui.Color) {
			if n == 0 {
				return
			}
			row := ui.Row(c).Gap(5).Padding(0, 6).AlignItems(ui.Center).Radius(12).
				Label(label + " " + itoa(n)).Tooltip(label)
			if row.Hovered() {
				row.Background(pal.RowHover)
			}
			row.Children(func() {
				statusGlyph(c, pal, kind, kind == "idle", -1, 12)
				ui.Textf(c, "%d", n).FontSize(12).FontWeight(500).TextColor(color).FontFeatures("tnum")
			})
			if row.Clicked() {
				w.jumpTo(kind)
			}
		}
		chip("waiting", locales.FooterNeedsYou(), waiting, pal.Attention)
		chip("working", locales.FooterWorking(), working, pal.TextMuted)
		chip("idle", locales.FooterDone(), done, pal.TextMuted)
		if waiting+working+done == 0 {
			row := ui.Row(c).Gap(6).Padding(0, 6).AlignItems(ui.Center).
				Label(w.serviceLine()).Tooltip(w.machineNote())
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
