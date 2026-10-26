package main

import (
	"slices"
	"strings"
	"unicode"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/beacon/internal/uitokens"
	"github.com/CarlOpenLab/beacon/locales"
)

// paletteState is the command palette's: a field that finds sessions, machines
// and commands as one types.
type paletteState struct {
	open   bool
	query  string
	index  int
	scroll ui.ScrollState
}

// paletteItem is a session or a command the palette offers.
type paletteItem struct {
	title   string
	detail  string
	keys    string
	session bool
	run     func()
	score   int
}

func (w *window) openPalette() { w.pal = paletteState{open: true} }

func (w *window) paletteItems() []paletteItem {
	var items []paletteItem
	for _, m := range w.groups() {
		for _, e := range w.entriesFor(m.ID) {
			entry := e
			items = append(items, paletteItem{
				title:   entry.label,
				detail:  w.machineName(entry.machine) + " · " + entry.meta,
				session: true,
				run:     entry.pick,
			})
		}
	}
	// The commands are the action registry's, with the keys in force: a
	// shortcut rebound in Settings shows its new keys here too.
	var cmds []paletteItem
	for _, id := range []string{
		"session.new", "pane.split.right", "pane.split.down", "pane.close",
		"sidebar.toggle", "session.attention", "session.refresh", "settings.open",
	} {
		a, ok := actionByID(id)
		if !ok {
			continue
		}
		act := a
		cmds = append(cmds, paletteItem{
			title: strings.TrimSuffix(act.title(), "…"),
			keys:  glyphAccel(w.accel(act)),
			run:   func() { act.run(w) },
		})
	}
	return append(items, cmds...)
}

// fuzzy scores how well query matches s, as its letters in order: higher is
// better, -1 no match. Starts of words and runs score more. It is the app's, so
// the same query offers the same order.
func fuzzy(query, s string) int {
	if query == "" {
		return 0
	}
	q := []rune(strings.ToLower(query))
	r := []rune(strings.ToLower(s))
	score, qi, run := 0, 0, 0
	for i := 0; i < len(r) && qi < len(q); i++ {
		if q[qi] == ' ' {
			qi++
			continue
		}
		if r[i] == q[qi] {
			bonus := 1
			if i == 0 || !unicode.IsLetter(r[i-1]) && !unicode.IsDigit(r[i-1]) {
				bonus += 6
			}
			run++
			score += bonus + run*2
			qi++
		} else {
			run = 0
		}
	}
	if qi < len(q) {
		return -1
	}
	return score*10 - len(r)
}

func (w *window) paletteView(c *ui.Context, pal *uitokens.Palette) {
	if !w.pal.open {
		return
	}
	var items []paletteItem
	for _, it := range w.paletteItems() {
		sc := fuzzy(w.pal.query, it.title+" "+it.detail)
		if t := fuzzy(w.pal.query, it.title); t >= 0 {
			sc = max(sc, t+50)
		}
		if sc < 0 {
			continue
		}
		it.score = sc
		items = append(items, it)
	}
	if w.pal.query != "" {
		slices.SortStableFunc(items, func(x, y paletteItem) int { return y.score - x.score })
	}
	w.pal.index = min(max(w.pal.index, 0), max(len(items)-1, 0))

	var chosen func()
	ui.DialogBase(c, &w.pal.open, func(backdrop, panel ui.Element) {
		backdrop.Background(pal.Overlay).Justify(ui.Start).Padding(84, 0, 0, 0)
		floating(panel.Width(600).MaxHeight(460).Radius(14).Background(pal.Popover), pal).Clip().
			Label(locales.PaletteLabel())
		ui.Row(c).Height(50).Shrink(0).Padding(0, 16).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconSearch).Size(16, 16).TextColor(pal.TextMuted)
			in := ui.TextInputBase(c.Key("palette-q"), &w.pal.query).Grow(1).FontSize(15).
				Placeholder(locales.PalettePlaceholder()).AutoFocus()
			if in.Changed() {
				w.pal.index = 0
			}
			if c.Shortcut(0, ui.KeyDown) || c.Shortcut(ui.Ctrl, ui.KeyN) {
				w.pal.index = min(w.pal.index+1, len(items)-1)
			}
			if c.Shortcut(0, ui.KeyUp) || c.Shortcut(ui.Ctrl, ui.KeyP) {
				w.pal.index = max(w.pal.index-1, 0)
			}
			if (in.Submitted() || c.Shortcut(0, ui.KeyEnter)) && len(items) > 0 {
				chosen = items[w.pal.index].run
			}
		})
		list := ui.Scroll(c.Key("palette-results")).Grow(1).MinHeight(0).Padding(6).TrackScroll(&w.pal.scroll)
		list.Children(func() {
			if len(items) == 0 {
				ui.Text(c, locales.PaletteNoMatches()).FontSize(13).TextColor(pal.TextFaint).Padding(12, 12)
			}
			lastKind := true
			for i, it := range items {
				isSession := it.session
				if i > 0 && isSession != lastKind && w.pal.query == "" {
					ui.Text(c, locales.PaletteCommands()).FontSize(11.5).FontWeight(600).
						TextColor(pal.TextFaint).Padding(10, 10, 4, 10)
				}
				lastKind = isSession
				row := ui.Row(c.Key(i)).Height(38).Padding(0, 10).Gap(10).AlignItems(ui.Center).Radius(8)
				if i == w.pal.index {
					row.Background(pal.Accent.Alpha(0.12))
					row.ScrollIntoView()
				} else if row.Hovered() {
					row.Background(pal.RowHover)
				}
				if row.Clicked() {
					chosen = it.run
				}
				row.Children(func() {
					if isSession {
						ui.Box(c).Size(8, 8).Radius(4).Background(pal.Success)
					} else {
						ui.Icon(c, iconChevron).Size(14, 14).TextColor(pal.TextFaint)
					}
					ui.Text(c, it.title).FontSize(13.5).TextColor(pal.Text).SingleLine().
						Ellipsis("…").Shrink(0).MaxWidth(300)
					ui.Text(c, it.detail).FontSize(12.5).TextColor(pal.TextFaint).SingleLine().
						Ellipsis("…").Grow(1).MinWidth(0)
					if it.keys != "" {
						kbd(c, pal, it.keys)
					}
				})
			}
		})
	})
	if chosen != nil {
		w.pal.open = false
		chosen()
	}
}
