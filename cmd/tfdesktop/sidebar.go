package main

import (
	"runtime"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/tunnel"
	"github.com/CarlOpenLab/term-fleet/internal/uitokens"
	"github.com/CarlOpenLab/term-fleet/locales"
)

// The sidebar is kero's: groups of sessions with their own headers, the chosen
// session a raised row, and a foot of quiet icon buttons. term-fleet's groups
// are machines — this one first, then every machine the relay says is here —
// which is the same shape kero gives a project's sessions; a machine this
// window has not asked for its shells yet says so in one row instead of showing
// an empty group.

// sideWidth is how wide the column is, clamped to what a name fits in.
func (w *window) sideWidth() float32 {
	if w.sidebarWidth == 0 {
		return uitokens.SidebarWidth
	}
	return min(max(w.sidebarWidth, uitokens.MinSidebar), uitokens.MaxSidebar)
}

// lead is where the sidebar's contents start: after the traffic lights on macOS,
// at the window's own edge elsewhere.
func lead(bar ui.TitleBar) float32 {
	if bar.Left > 0 {
		return bar.Left + uitokens.CardGap
	}
	return 12
}

// sidebar draws the column: the window's buttons, the machine groups, and the
// foot with the palette, the settings and how the fleet stands.
func (w *window) sidebar(c *ui.Context, pal *uitokens.Palette) {
	bar := c.TitleBar()
	topHeight := float32(uitokens.TopBar)
	if bar.Height > topHeight {
		topHeight = bar.Height
	}

	ui.Column(c).Width(w.sideWidth()).FillHeight().Shrink(0).ClipX().Children(func() {
		w.resizeHandle(c, pal)
		ui.Row(c).Height(topHeight).Shrink(0).Padding(0, 12, 0, lead(bar)).Gap(2).AlignItems(ui.Center).DragWindow().Children(func() {
			// On macOS the traffic lights hold the left of the bar, so the
			// buttons go right, as they do in every Mac app that hides its
			// sidebar; elsewhere there is nothing to clear, and they lead.
			if runtime.GOOS == "darwin" {
				ui.Spacer(c)
			}
			if iconButton(c, iconSidebar, locales.ActionToggleSidebar(), 28).Clicked() {
				w.sidebarHidden = true
			}
			if iconButton(c, iconPlus, locales.ActionNewTerminalTip(), 28).Clicked() {
				w.newTab()
			}
			if runtime.GOOS != "darwin" {
				ui.Spacer(c)
			}
		})

		ui.Scroll(c).Grow(1).MinHeight(0).Padding(0, 10, 16, 10).Children(func() {
			for i, m := range w.groups() {
				w.machineGroup(c, pal, m, i)
			}

		})
		w.sidebarFooter(c, pal)
	})
	// Times on the rows move on without anything else changing.
	c.After(20 * time.Second)
}

// resizeHandle is the column's edge, which a drag moves and a double click puts
// back.
func (w *window) resizeHandle(c *ui.Context, pal *uitokens.Palette) {
	h := ui.Box(c).Absolute().Left(w.sideWidth()-4).Top(0).Bottom(0).Width(8).Cursor(ui.CursorResizeEW)
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

// machineGroup is one machine and, under it, its sessions: kero's project group,
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
		// The machine's state, where kero's group has a count: a dot that says
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
		if hovered {
			if iconButton(c, iconPlus, locales.ActionNewTerminalTip(), 20).Clicked() {
				w.newTabOn(m.ID)
			}
		}
	})
	if collapsed {
		return
	}
	for _, e := range entries {
		w.sessionRow(c, pal, e)
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
// settings; then, at the right, how the fleet stands — kero's quiet footer, in
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
