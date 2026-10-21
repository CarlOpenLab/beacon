package main

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/agent"
	"github.com/CarlOpenLab/term-fleet/internal/fonts"
	"github.com/CarlOpenLab/term-fleet/internal/settings"
	"github.com/CarlOpenLab/term-fleet/internal/uitokens"
	"github.com/CarlOpenLab/term-fleet/locales"
)

// systemFamily is what the font list shows for "no family set", which the
// terminal plugin resolves to the system's monospaced font. It reads as a label
// rather than a family name, so it cannot collide with a font someone installs.
// It is a call rather than a constant because the label is translated.
func systemFamily() string { return locales.SettingsFontSystem() }

// languageChoices are the interface languages, in the order the settings window
// offers them. The first follows the languages the system says the person
// reads, which is what a first run gets; the others are the languages the app
// is written in, in the order its catalogs list them.
var languageChoices = []struct {
	tag   string
	label func() string
}{
	{"", locales.SettingsLangSystem},
	{"zh-Hans", locales.SettingsLangZh},
	{"en", locales.SettingsLangEn},
}

// normalizeLanguage maps anything unrecognised onto following the system: a
// stale or hand-edited settings file must not leave the shell in a language it
// cannot show.
func normalizeLanguage(tag string) string {
	tag = strings.TrimSpace(tag)
	for _, choice := range languageChoices {
		if choice.tag == tag {
			return tag
		}
	}
	return ""
}

// languageIndex is where tag sits in languageChoices, which is what the
// settings window's segmented control works in.
func languageIndex(tag string) int {
	tag = normalizeLanguage(tag)
	for i, choice := range languageChoices {
		if choice.tag == tag {
			return i
		}
	}
	return 0
}

// applyLanguage makes the window show a language at once. An empty tag follows
// the languages the system says the person reads again.
func applyLanguage(tag string) {
	mygo.App.SetLocale(normalizeLanguage(tag))
}

// knownFamilies are suggested on top of whatever is installed on this machine.
// They are a floor for machines whose fonts cannot be read (a bare container,
// say) rather than the whole list: the field takes any family name, because a
// curated list cannot know which builds someone installed. Nerd Font releases
// are named after the family they patch -- "Maple Mono NF" alongside "Maple
// Mono" -- and a prompt drawn with icons needs those glyphs, so the variants
// have to be reachable instead of silently falling back.
var knownFamilies = []string{
	"JetBrains Mono",
	"Maple Mono",
	"Maple Mono NF",
	"SF Mono",
	"Menlo",
	"Monaco",
	"Cascadia Mono",
	"Cascadia Code",
	"Consolas",
	"Courier New",
	"Noto Sans Mono",
	"JetBrainsMono Nerd Font",
	"FiraCode Nerd Font",
	"Hack Nerd Font",
}

const (
	paneTerminal = iota
	paneRelay
	paneServer
	paneUpdates
	paneAppearance
	paneNotifications
	paneKeyboard
)

// termFont turns stored settings into the terminal plugin's font.
func termFont(s settings.Settings) terminal.Font {
	f := terminal.Font{Family: s.FontFamily}
	if s.FontSize > 0 {
		f.Size = float32(s.FontSize)
	}
	return f
}

// familyFromLabel maps the window's font entry back to what the store keeps.
func familyFromLabel(label string) string {
	if label == systemFamily() {
		return ""
	}
	return strings.TrimSpace(label)
}

// labelForFamily is the inverse, for showing a stored family in the window.
func labelForFamily(family string) string {
	if strings.TrimSpace(family) == "" {
		return systemFamily()
	}
	return family
}

// settingsWindow is the preferences window: the terminal's font, the relay this
// machine joins, and whether the local service that serves browsers is on.
//
// It edits a draft rather than the store, so a half-typed relay address is
// never written to disk. The discrete controls — the font list, the size, the
// two switches — save as they change; the text fields save when 应用 is
// pressed, which keeps a relay link from reconnecting on every keystroke.
type settingsWindow struct {
	desk  *window
	store *settings.Store
	fleet *fleet

	pane int

	family  string
	size    float64
	relay   string
	device  string
	secret  string
	ca      string
	relayOn bool
	server  bool
	// control is whether other clients may take a shell on this machine: the
	// inverse of settings.ControlOff, because the switch reads better as the
	// thing it allows.
	control bool
	// notifyWaiting, notifyDone, notifyError and dockBadge are the notification
	// switches in the window's own terms: true means the notification happens,
	// and commit writes the inverse into the fields that name what turns it off.
	notifyWaiting bool
	notifyDone    bool
	notifyError   bool
	dockBadge     bool

	// language is the interface language: "" follows the languages the system
	// says the person reads, and a tag such as "en" or "zh-Hans" is a choice.
	// applied is the one the window last told the app to show, so a commit that
	// did not touch the language — a font size, a relay address — does not make
	// every window redraw for nothing.
	language string
	applied  string

	// caNote is the one line under the CA field: which certificate is pinned,
	// or why the last attempt failed. It is state, not a transient message,
	// because ui has no other place to show "this is the file you picked".
	caNote string

	// recording is the action whose shortcut is being typed, and recordRef is the
	// handle that takes the keys while it is; keyNotice is the line under the
	// keyboard page about a conflict or a clear.
	recording  string
	recordRef  ui.Handle
	keyNotice  string

	// status is the last thing that happened to the draft: saved, or why not.
	status string
	// statusErr marks status as a failure, which the window shows in a
	// different colour from a save.
	statusErr bool
	// savedAt is when status was written. It gives "已保存" a time, so the line
	// reads as an answer to the click that caused it rather than as decoration.
	savedAt time.Time

	// installedMu guards installed, which a background scan fills with the
	// font families on this machine. Until it lands the field suggests
	// knownFamilies only.
	installedMu sync.Mutex
	installed   []string
}

// newSettingsWindow opens on what is on disk right now.
func newSettingsWindow(desk *window, store *settings.Store, f *fleet) *settingsWindow {
	cur := store.Get()
	s := &settingsWindow{
		desk:     desk,
		store:    store,
		fleet:    f,
		family:   labelForFamily(cur.FontFamily),
		size:     cur.FontSize,
		relay:    cur.RelayURL,
		device:   cur.DeviceID,
		secret:   cur.DeviceSecret,
		ca:       cur.RelayCA,
		relayOn:  cur.RelayOn,
		server:   !cur.ServerOff,
		control:  !cur.ControlOff,
		notifyWaiting: !cur.NotifyWaitingOff,
		notifyDone:    !cur.NotifyDoneOff,
		notifyError:   !cur.NotifyErrorOff,
		dockBadge:     !cur.DockBadgeOff,
		language:      normalizeLanguage(cur.Language),
		applied:       normalizeLanguage(cur.Language),
	}
	if s.size <= 0 {
		s.size = settings.DefaultFontSize
	}
	s.caNote = caNoteFor(s.ca)
	s.loadInstalledFamilies()
	return s
}

// loadInstalledFamilies reads the machine's font directories off the UI thread:
// the scan walks every installed font, which is too slow to do while painting.
// The result only feeds suggestions, so arriving late is harmless.
func (s *settingsWindow) loadInstalledFamilies() {
	go func() {
		found := fonts.Families()
		s.installedMu.Lock()
		s.installed = found
		s.installedMu.Unlock()
		s.set(func() {})
	}()
}

// installedFamilies is the scanned list, or nil before the scan finishes.
func (s *settingsWindow) installedFamilies() []string {
	s.installedMu.Lock()
	defer s.installedMu.Unlock()
	return s.installed
}

// familyChoices is what the font field suggests: the system default, the
// families actually installed, then the well-known ones that are not installed
// already. Suggestions only — the field accepts any name.
func (s *settingsWindow) familyChoices() []string {
	choices := []string{systemFamily()}
	seen := map[string]bool{systemFamily(): true}
	for _, list := range [][]string{s.installedFamilies(), knownFamilies} {
		for _, name := range list {
			if name = strings.TrimSpace(name); name == "" || seen[name] {
				continue
			}
			seen[name] = true
			choices = append(choices, name)
		}
	}
	return choices
}

// loaded is the stored settings with the same fallbacks the fields apply.
// Comparing the draft against this answers "did the user change anything?"
// rather than "does the file spell the defaults the same way as the window?".
func (s *settingsWindow) loaded() settings.Settings {
	cur := s.store.Get()
	if cur.FontSize <= 0 {
		// The terminal reads 0 as its own default; the field shows a number, so
		// a window nobody has touched would otherwise look unsaved.
		cur.FontSize = settings.DefaultFontSize
	}
	return cur
}

// draft is the settings the fields currently describe.
func (s *settingsWindow) draft() settings.Settings {
	cur := s.loaded()
	cur.FontFamily = familyFromLabel(s.family)
	cur.FontSize = s.size
	cur.RelayURL = strings.TrimSpace(s.relay)
	cur.DeviceID = strings.TrimSpace(s.device)
	cur.DeviceSecret = strings.TrimSpace(s.secret)
	cur.RelayCA = strings.TrimSpace(s.ca)
	cur.RelayOn = s.relayOn
	cur.ServerOff = !s.server
	cur.ControlOff = !s.control
	cur.Language = normalizeLanguage(s.language)
	cur.NotifyWaitingOff = !s.notifyWaiting
	cur.NotifyDoneOff = !s.notifyDone
	cur.NotifyErrorOff = !s.notifyError
	cur.DockBadgeOff = !s.dockBadge
	return cur
}

// dirty reports whether the fields hold changes that 应用 has not written yet,
// so the window can say so instead of leaving the user to guess whether the
// click did anything.
func (s *settingsWindow) dirty() bool {
	// The keybinds are a map, which a struct comparison does not take; DeepEqual
	// compares the rest field for field as well, so the two answers agree.
	return !reflect.DeepEqual(s.draft(), s.loaded())
}

// caNoteFor is the one line under the CA field. It reads the file so a wrong,
// missing or expired certificate is visible while the window is open, instead
// of surfacing later as a machine that mysteriously stays offline.
func caNoteFor(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return locales.SettingsCaNoteEmpty()
	}
	pemBytes, err := os.ReadFile(path)
	if err != nil {
		return locales.SettingsCaUnreadable(err.Error())
	}
	info, err := agent.DescribeCert(pemBytes)
	if err != nil {
		return locales.SettingsCaInvalid(err.Error())
	}
	note := locales.SettingsCaPinned(info.Subject, info.Fingerprint[:16], info.NotAfter.Format("2006-01-02"))
	if time.Now().After(info.NotAfter) {
		note += " " + locales.SettingsCaExpired()
	}
	return note
}

func (s *settingsWindow) view(c *ui.Context) {
	applyTheme(c, s.store.Get().Theme)
	pal := paletteFor(c)
	t := c.Theme()
	// 应用 exists for the fields that commit on it, so it is offered exactly
	// when there is something to apply: a permanently enabled button trains the
	// user to ignore it, and the pending marker is what says otherwise.
	dirty := s.dirty()
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Height(uitokens.HeaderH).Shrink(0).Padding(0, 12).Gap(8).
			AlignItems(ui.Center).DragWindow().Children(func() {
			ui.Text(c, locales.SettingsTitle()).FontSize(14).FontWeight(600).TextColor(pal.Text)
			ui.Spacer(c)
			if dirty {
				ui.Text(c, locales.SettingsDirty()).FontSize(12).TextColor(t.Warning)
			}
			if chromeButton(c, pal, "restore", locales.SettingsRestore(), "", false) {
				s.restoreDefaults()
			}
			if dirty && chromeButton(c, pal, "apply", locales.SettingsApply(), "", true) {
				s.commit()
			}
			if iconButton(c, iconClose, locales.SettingsClose(), 28).Clicked() && s.desk != nil {
				s.desk.settingsOpen = false
			}
		})
		ui.Row(c).Fill().MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			// Settings: the pages down the left, the chosen one on a card
			// of its own at the right.
			ui.Column(c).Width(184).Shrink(0).FillHeight().Padding(8, 8, 8, 4).Gap(2).Children(func() {
				for _, page := range settingsPages {
					s.settingsNav(c, pal, page)
				}
			})
			ui.Column(c).Grow(1).MinWidth(0).FillHeight().Padding(8, 8, 8, 8).Children(func() {
				card := ui.Column(c).Fill().MinWidth(0).Radius(uitokens.CardRadius).Clip()
				raised(card.Background(pal.Card), pal)
				card.Children(func() {
					// The pane is the whole card, so it scrolls rather than clips:
					// a short window must not hide a switch under the fold.
					ui.Scroll(c).Fill().Children(func() {
						ui.Column(c).Fill().Padding(18).Gap(14).Children(func() {
							switch s.pane {
							case paneTerminal:
								s.terminalPane(c)
							case paneRelay:
								s.relayPane(c)
							case paneServer:
								s.serverPane(c)
							case paneAppearance:
								s.appearancePane(c)
							case paneNotifications:
								s.notificationsPage(c)
							case paneKeyboard:
								s.keyboardPage(c)
							default:
								s.updatesPane(c)
							}
						})
					})
				})
			})
		})
		if s.status != "" {
			line := ui.Text(c, s.status).FontSize(11).PaddingX(12)
			if s.statusErr {
				line.TextColor(t.Danger)
			} else {
				line.TextColor(t.Success)
			}
		}
	})
}

// settingsPage is one page of the Settings window.
type settingsPage struct {
	id    int
	title func() string
	icon  *ui.SVG
}

var settingsPages = []settingsPage{
	{paneTerminal, locales.SettingsPaneTerminal, iconTerminal},
	{paneRelay, locales.SettingsPaneRelay, iconSend},
	{paneServer, locales.SettingsPaneServer, iconMonitor},
	{paneUpdates, locales.SettingsPaneUpdates, iconRefresh},
	{paneAppearance, locales.SettingsPaneAppearance, iconSun},
	{paneNotifications, locales.SettingsPaneNotifications, iconBell},
	{paneKeyboard, locales.SettingsPaneKeyboard, iconKeyboard},
}

// settingsNav is one page in the window's own list, with the chosen page raised
// off the column.
func (s *settingsWindow) settingsNav(c *ui.Context, pal *uitokens.Palette, page settingsPage) {
	selected := s.pane == page.id
	item := ui.Row(c).Height(32).Padding(0, 10).Gap(10).AlignItems(ui.Center).
		Radius(uitokens.RowRadius).Label(page.title())
	if pal.Dark {
		item.Border(1, ui.Transparent) // room for the chosen row's edge
	}
	bg := ui.Transparent
	switch {
	case selected:
		bg = pal.RowSelected
	case item.Hovered():
		bg = pal.RowHover
	}
	item.Background(item.AnimateColor("bg", bg, ui.Animation{Duration: 110e6}))
	if selected {
		chosen(item, pal)
	}
	if item.Pressed() && !selected {
		s.pane = page.id
	}
	item.Children(func() {
		ic := ui.Icon(c, page.icon).Size(15, 15).TextColor(pal.TextMuted)
		if selected {
			ic.TextColor(pal.Accent)
		}
		ui.Text(c, page.title()).FontSize(13.5).TextColor(pal.Text).SingleLine()
	})
}

// groupTitle is a group's title, over the panel of settings it names.
func groupTitle(c *ui.Context, pal *uitokens.Palette, title string) {
	ui.Text(c, title).FontSize(12).FontWeight(600).TextColor(pal.TextMuted).Padding(0, 4)
}

// settingsGroup is a titled set of settings on a soft panel, as the settings are
// grouped: a chip of a card, its rows divided by hairlines.
func settingsGroup(c *ui.Context, pal *uitokens.Palette, title string, fn func()) {
	ui.Column(c).Gap(8).Children(func() {
		if title != "" {
			groupTitle(c, pal, title)
		}
		ui.Column(c).Radius(12).Background(pal.Chip).Padding(4, 16).Children(fn)
	})
}

// groupRows counts the rows of the group being built, so that a divider goes
// between them, as in macOS's Settings.
var groupRows int

// settingRow is a row of a group: a label, what it does, and its control.
func settingRow(c *ui.Context, pal *uitokens.Palette, label, detail string, control func()) {
	if groupRows++; groupRows > 1 {
		ui.Box(c).Height(1).Shrink(0).Background(pal.Text.Alpha(0.07))
	}
	row := ui.Row(c).Padding(11, 0).Gap(16).AlignItems(ui.Center)
	row.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, label).FontSize(13.5).TextColor(pal.Text)
			if detail != "" {
				ui.Text(c, detail).FontSize(12).TextColor(pal.TextMuted).LineHeight(1.35)
			}
		})
		ui.Row(c).Shrink(0).Gap(8).AlignItems(ui.Center).Children(control)
	})
}

// segmentedChoice is a segmented control as the platform draws one: a soft
// track, the choice a lighter fill raised a hair, thin separators between the
// others, and labels of one weight. It is intrinsic: a Form stretches its
// children, and a track stretched across the card is not a control.
func segmentedChoice(c *ui.Context, pal *uitokens.Palette, key string, value *int, labels []string) ui.SegmentedParts {
	track, thumb := ui.RGBA(0, 0, 0, 0.06), ui.Hex("#ffffff")
	if pal.Dark {
		track, thumb = ui.RGBA(255, 255, 255, 0.07), ui.RGBA(255, 255, 255, 0.2)
	}
	seg := ui.SegmentedBase(c.Key(key), value, len(labels))
	seg.Track.Height(24).Padding(2).Radius(7).Background(track).
		AlignItems(ui.Center).Children(func() {
		for n, label := range labels {
			if n > 0 {
				// A separator, where neither segment beside it is chosen.
				sep := ui.Box(c).Width(1).Height(12).Shrink(0)
				if n != *value && n-1 != *value {
					sep.Background(pal.Text.Alpha(0.14))
				}
			}
			s := seg.Segment(n).FillHeight().Padding(0, 12).Radius(5).Center()
			if n == *value {
				s.Background(thumb)
				if pal.Dark {
					s.Shadow(0, 1, 1, 0, ui.RGBA(0, 0, 0, 0.25))
				} else {
					s.Shadow(0, 0.5, 1.5, 0, ui.RGBA(0, 0, 0, 0.16)).Shadow(0, 0, 0, 0.5, ui.RGBA(0, 0, 0, 0.04))
				}
			}
			s.Children(func() {
				ui.Text(c, label).FontSize(12.5).SingleLine().TextColor(pal.Text)
			})
		}
	})
	return seg
}

// fontSizePopup chooses the terminal's size as a pop-up button, the way the
// platform's Settings does: the value on a pill, a disc with the two ways the
// menu opens, and the sizes in a menu below it.
func (s *settingsWindow) fontSizePopup(c *ui.Context, pal *uitokens.Palette) {
	sizes := []float64{10, 11, 12, 13, 14, 15, 16, 18, 20, 22, 24, 28}
	if !slices.Contains(sizes, s.size) {
		sizes = append(sizes, s.size)
		slices.Sort(sizes)
	}
	sel := ui.SelectBase(c.Key("font-size"), &s.size)
	trig := sel.Trigger
	trig.Height(28).Padding(0, 3, 0, 10).Gap(6).Radius(14).AlignItems(ui.Center).AlignSelf(ui.End).Cursor(ui.CursorDefault)
	tint := pal.Text.Alpha(0.065)
	if pal.Dark {
		tint = ui.RGBA(255, 255, 255, 0.09)
	}
	open := sel.Open()
	hover := trig.Hovered() || open
	bg, disc := ui.Transparent, tint
	switch {
	case open || trig.Pressed():
		bg, disc = tint.Mix(pal.Text, 0.06), ui.Transparent
	case hover:
		bg, disc = tint, ui.Transparent
	}
	anim := ui.Animation{Duration: 120 * time.Millisecond}
	trig.Background(trig.AnimateColor("bg", bg, anim))
	trig.Children(func() {
		ui.Text(c, formatSize(s.size)+" pt").FontSize(13).TextColor(pal.Text).SingleLine()
		d := ui.Box(c).Size(22, 22).Radius(11).Center().Shrink(0)
		d.Background(d.AnimateColor("bg", disc, anim)).Children(func() {
			ui.Icon(c, iconUpDown).Size(14, 14).TextColor(pal.Text.Alpha(0.85))
		})
	})
	sel.Popup(func(panel ui.Element) {
		floating(panel.Padding(5).Radius(13).Background(pal.Popover), pal)
		ui.Column(c).Gap(0).Children(func() {
			for _, size := range sizes {
				item := sel.Item(size)
				item.Height(24).Padding(0, 12, 0, 6).Gap(5).Radius(7).AlignItems(ui.Center)
				fg := pal.Text
				if item.Highlighted() {
					item.Background(pal.Accent)
					fg = ui.Hex("#ffffff")
				}
				item.TextColor(fg).Children(func() {
					mark := ui.Icon(c, iconCheck).Size(12, 12).Shrink(0)
					if size != s.size {
						mark.Opacity(0)
					}
					ui.Text(c, formatSize(size)+" pt").FontSize(13).SingleLine()
				})
			}
		})
	})
	if trig.Changed() {
		s.commit()
	}
}

// formatSize writes a point size the way the pop-up shows it: 13, not 13.0.
func formatSize(v float64) string { return strconv.FormatFloat(v, 'f', -1, 32) }

// appearancePane is the window's own appearance: light, dark or the system's,
// and the wash the cards sit on.
func (s *settingsWindow) appearancePane(c *ui.Context) {
	pal := paletteFor(c)
	mode := themeSystem
	background := ""
	if s.store != nil {
		mode = normalizeTheme(s.store.Get().Theme)
		background = s.store.Get().Background
	}
	if background == "" {
		background = "rex"
	}

	groupRows = 0
	settingsGroup(c, pal, locales.SettingsPaneAppearance(), func() {
		settingRow(c, pal, locales.SettingsTheme(), "", func() {
			labels := []string{locales.ThemeSystem(), locales.ThemeLight(), locales.ThemeDark()}
			choice := themeIndex(mode)
			seg := segmentedChoice(c, pal, "theme", &choice, labels)
			seg.Track.OnChange(func() {
				if s.desk != nil {
					s.desk.setTheme(themeChoices[choice].id)
				}
			})
		})
	})

	groupTitle(c, pal, locales.SettingsBackground())
	ui.Row(c).Gap(12).Wrap().Children(func() {
		for _, wsh := range uitokens.Washes {
			s.washTile(c, pal, wsh, background)
		}
	})
}

// washTile previews a wash, and chooses it on a click: the background
// settings, with a card on the tile so that the wash is seen the way the window
// shows it.
func (s *settingsWindow) washTile(c *ui.Context, pal *uitokens.Palette, wsh uitokens.Wash, current string) {
	selected := current == wsh.ID
	tile := ui.ButtonBase(c.Key("bg-"+wsh.ID)).Column().Width(104).Gap(8).
		AlignItems(ui.Stretch).Label(washName(wsh.ID))
	tile.Children(func() {
		sw := raised(ui.Box(c).Size(104, 68).Radius(12), pal).Clip()
		sw.LinearGradient(wsh.Base(pal.Dark))
		sw.Children(func() {
			ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).LinearGradient(wsh.Glow(pal.Dark))
			// A card on it, as the window shows them.
			ui.Box(c).Absolute().Top(12).Left(26).Right(8).Bottom(-6).Radius(7).Background(pal.Card)
			ui.Box(c).Absolute().Top(20).Left(8).Width(12).Height(4).Radius(2).Background(pal.TextFaint.Alpha(0.5))
			if selected {
				ui.Box(c).Absolute().Top(6).Right(6).Size(18, 18).Radius(9).Center().Background(pal.Accent).Children(func() {
					ui.Icon(c, iconCheck).Size(10, 10).TextColor(ui.Hex("#ffffff"))
				})
			}
		})
		label := ui.Text(c, washName(wsh.ID)).FontSize(12.5).TextColor(pal.TextMuted).TextAlign(ui.Center)
		if selected {
			label.TextColor(pal.Text).FontWeight(600)
		}
	})
	if tile.Clicked() && s.store != nil {
		if _, err := s.store.Update(func(st *settings.Settings) { st.Background = wsh.ID }); err != nil {
			s.status, s.statusErr = err.Error(), true
		}
		if s.desk != nil {
			s.desk.invalidate()
		}
	}
}

// washName is a wash's name in the language the app shows.
func washName(id string) string {
	switch id {
	case "rex":
		return locales.BgRex()
	case "mist":
		return locales.BgMist()
	case "meadow":
		return locales.BgMeadow()
	case "dusk":
		return locales.BgDusk()
	}
	return locales.BgPlain()
}

// paneNote is a line of prose in a pane's own column: a field with no label,
// so it starts where the controls start rather than at the pane's edge. A note
// hanging at the left margin under a form whose controls sit in a column of
// their own is the one thing that makes a settings pane read as unaligned.
func paneNote(c *ui.Context, text string, color ui.Color) {
	ui.Field(c, "", func() {
		ui.Text(c, text).FontSize(11).TextColor(color)
	})
}

// keyboardPage is every action the window knows, with the keys it answers to:
// click a shortcut and the next keys typed become it. The list is the action
// registry, so an action the window gains arrives here with nothing to wire.
func (s *settingsWindow) keyboardPage(c *ui.Context) {
	pal := paletteFor(c)
	groupRows = 0
	settingsGroup(c, pal, locales.SettingsPaneKeyboard(), func() {
		for _, a := range appActions {
			s.shortcutRow(c, pal, a)
		}
	})
	ui.Text(c, locales.SettingsKeysNote()).FontSize(12).TextColor(pal.TextMuted).LineHeight(1.4).Padding(0, 4)
	if s.keyNotice != "" {
		ui.Text(c, s.keyNotice).FontSize(12).TextColor(pal.Attention).LineHeight(1.4).Padding(0, 4)
	}
}

// shortcutRow is one action and its shortcut: a chip that records, and a × that
// clears it.
func (s *settingsWindow) shortcutRow(c *ui.Context, pal *uitokens.Palette, a action) {
	accel := s.keysFor(a)
	recording := s.recording == a.id
	if groupRows++; groupRows > 1 {
		ui.Box(c).Height(1).Shrink(0).Background(pal.Text.Alpha(0.07))
	}
	row := ui.Row(c).Padding(11, 0).Gap(16).AlignItems(ui.Center)
	row.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, a.title()).FontSize(13.5).TextColor(pal.Text).SingleLine()
		})
		ui.Row(c).Shrink(0).Gap(6).AlignItems(ui.Center).Children(func() {
			chip := ui.ButtonBase(c).Height(24).Padding(0, 10).Radius(6).MinWidth(92).
				AlignItems(ui.Center).Justify(ui.Center).Label(a.title())
			face(chip, pal, pal.Control)
			chip.TextColor(pal.Text)
			if recording {
				chip.Bind(&s.recordRef).HandleInput(func(ev ui.InputEvent) bool {
					if ev.Kind != ui.InputKeyDown {
						return ev.Kind == ui.InputText
					}
					return s.recordKey(a, ev.Mods, ev.Key)
				})
				chip.Background(pal.Accent.Alpha(0.12))
			}
			chip.Children(func() {
				switch {
				case recording:
					ui.Text(c, locales.SettingsKeysRecord()).FontSize(12.5).TextColor(pal.Accent).SingleLine()
				case accel == "":
					ui.Text(c, locales.SettingsKeysNone()).FontSize(12.5).TextColor(pal.TextFaint).SingleLine()
				default:
					ui.Text(c, glyphAccel(accel)).FontSize(12.5).TextColor(pal.Text).SingleLine()
				}
			})
			if chip.Clicked() && !recording {
				s.recording = a.id
				s.keyNotice = ""
				s.recordRef.Focus(c)
				if s.desk != nil && s.desk.win != nil {
					installMenu(s.desk)
				}
			}
			if accel != "" && !recording {
				if iconButton(c, iconClose, locales.SettingsKeysClear(), 22).Clicked() {
					s.setKeys(a, "")
				}
			}
		})
	})
	if recording && !s.recordRef.Focused(c) {
		// The keyboard went elsewhere without an answer: stop waiting for it
		// rather than leaving the chip saying it is still listening.
		c.After(200 * time.Millisecond)
	}
}

// keysFor is an action's accelerator as the settings window shows it.
func (s *settingsWindow) keysFor(a action) string {
	if s.store != nil {
		if v, ok := s.store.Get().Keybinds[a.id]; ok {
			return v
		}
	}
	return a.def
}

// recordKey takes a key typed for a shortcut, reporting whether it took it.
func (s *settingsWindow) recordKey(a action, mods ui.Modifiers, key ui.Key) bool {
	switch {
	case key == ui.KeyEscape && mods == 0:
		s.recording = ""
		return true
	case key == ui.KeyUnknown:
		return false
	case key == ui.KeyBackspace && mods == 0:
		s.setKeys(a, "")
		return true
	}
	accel := formatAccel(mods, key)
	if accel == "" {
		return false
	}
	s.setKeys(a, accel)
	if s.desk != nil && s.desk.win != nil {
		installMenu(s.desk)
	}
	return true
}

// setKeys saves an action's accelerator, says so when it collides with another
// action, and puts the menus back with the keys the window now answers to.
func (s *settingsWindow) setKeys(a action, accel string) {
	if s.store != nil {
		if _, err := s.store.Update(func(st *settings.Settings) {
			if st.Keybinds == nil {
				st.Keybinds = map[string]string{}
			}
			st.Keybinds[a.id] = accel
		}); err != nil {
			s.keyNotice = err.Error()
			return
		}
	}
	s.recording = ""
	s.keyNotice = ""
	if accel != "" {
		for _, other := range appActions {
			if other.id != a.id && s.keysFor(other) == accel {
				s.keyNotice = locales.SettingsKeysConflict(other.title())
				break
			}
		}
	}
	if s.desk != nil && s.desk.win != nil {
		installMenu(s.desk)
		s.desk.invalidate()
	}
	s.set(func() {})
}

// notificationsPage is what the window tells the user while they are looking
// elsewhere: the three events worth a notification, and the Dock badge.
func (s *settingsWindow) notificationsPage(c *ui.Context) {
	pal := paletteFor(c)
	groupRows = 0
	settingsGroup(c, pal, locales.SettingsPaneNotifications(), func() {
		settingRow(c, pal, locales.SettingsNotifyWaiting(), "", func() {
			toggle(c, pal, "notify-waiting", &s.notifyWaiting)
		})
		settingRow(c, pal, locales.SettingsNotifyDone(), "", func() {
			toggle(c, pal, "notify-done", &s.notifyDone)
		})
		settingRow(c, pal, locales.SettingsNotifyError(), "", func() {
			toggle(c, pal, "notify-error", &s.notifyError)
		})
		settingRow(c, pal, locales.SettingsDockBadge(), "", func() {
			toggle(c, pal, "dock-badge", &s.dockBadge)
		})
	})
	ui.Text(c, locales.SettingsNotifyNote()).FontSize(12).TextColor(pal.TextMuted).
		LineHeight(1.4).Padding(0, 4)
}

func (s *settingsWindow) terminalPane(c *ui.Context) {
	t := c.Theme()
	pal := paletteFor(c)
	ui.Form(c, func() {
		ui.Field(c, locales.SettingsFont(), func() {
			// Suggestions only, and no commit on change: this field takes free text,
			// so committing per keystroke would rewrite the settings and rebuild
			// the relay link on every character. Text fields save on 应用.
			ui.Autocomplete(c, &s.family, s.familyChoices()).Width(260).AlignSelf(ui.End)
		})
		ui.Field(c, locales.SettingsFontSize(), func() {
			s.fontSizePopup(c, pal)
		})
		// The language applies on the click, like the switches: a person who
		// picks another language wants to see it, not to look for 应用.
		lang := languageIndex(s.language)
		ui.Field(c, locales.SettingsLanguage(), func() {
			seg := segmentedChoice(c, pal, "language", &lang, []string{
				locales.SettingsLangSystem(), locales.SettingsLangZh(), locales.SettingsLangEn(),
			})
			seg.Track.AlignSelf(ui.End)
			seg.Track.OnChange(func() {
				s.language = languageChoices[lang].tag
				s.commit()
			})
		}).Description(locales.SettingsLanguageNote())
		paneNote(c, locales.SettingsFontNote(), t.TextMuted)
	})
}

func (s *settingsWindow) relayPane(c *ui.Context) {
	t := c.Theme()
	ui.Form(c, func() {
		ui.Field(c, locales.SettingsRelayUrl(), func() {
			ui.TextInput(c, &s.relay).Placeholder("wss://relay.example/relay/agent").Fill()
		})
		ui.Field(c, locales.SettingsRelayDevice(), func() {
			ui.TextInput(c, &s.device).Placeholder("mac-mini").Fill()
		})
		ui.Field(c, locales.SettingsRelaySecret(), func() {
			ui.TextInput(c, &s.secret).Password().Fill()
		})
		ui.Field(c, locales.SettingsRelayCa(), func() {
			ui.Column(c).Gap(6).Children(func() {
				ui.TextInput(c, &s.ca).Placeholder(locales.SettingsCaEmptyHint()).Fill()
				ui.Row(c).Gap(8).Children(func() {
					if ui.Button(c, locales.SettingsCaFetch()).Clicked() {
						s.fetchCA()
					}
					if ui.Button(c, locales.SettingsCaPick()).Clicked() {
						s.pickCA()
					}
					if s.ca != "" && ui.Button(c, locales.SettingsCaClear()).Clicked() {
						s.ca = ""
						s.commit()
					}
				})
				ui.Text(c, s.caNote).FontSize(11).TextColor(t.TextMuted)
			})
		})
		ui.Field(c, locales.SettingsRelayOn(), func() {
			// AlignSelf(Start): the field's control column stretches, and a
			// stretched switch would sit at the far end of the row, away from
			// the label it belongs to.
			ui.Switch(c, &s.relayOn).AlignSelf(ui.Start).OnChange(func() { s.commit() })
		})
		// The link's own colour: green while it is up, amber while it is not, so
		// the state is readable without parsing the sentence. An unused relay has
		// nothing to report — the agent's state is reset when the link is dropped,
		// and "connecting" about a link nobody asked for would be a lie.
		//
		// The client link is a second connection with a credential of its own,
		// so it gets its own line: a fleet with nothing in it and a link that was
		// refused look the same on the 机器 panel, and this is where the
		// difference is read.
		if s.relayOn {
			if line := s.relayStatusLine(); line != "" {
				color := t.Warning
				if s.fleet != nil && s.fleet.RelayState().Connected {
					color = t.Success
				}
				paneNote(c, line, color)
			}
		}
		if line := s.reachStatusLine(); line != "" {
			color := t.Warning
			if strings.HasPrefix(line, locales.SettingsReachReady()) {
				color = t.Success
			}
			paneNote(c, line, color)
		}
		paneNote(c, locales.SettingsRelayHelp(), t.TextMuted)
		paneNote(c, locales.SettingsTokenHelp(), t.TextMuted)
	})
}

// relayStatusLine says what the link is actually doing. Without it, a wrong
// certificate or a rejected credential looks exactly like everything being
// fine, which is how "I filled it in and it is still offline" happens.
func (s *settingsWindow) relayStatusLine() string {
	if s.fleet == nil {
		return ""
	}
	st := s.fleet.RelayState()
	switch {
	case st.Connected:
		return locales.SettingsRelayConnected(st.Detail)
	case st.Detail != "":
		return locales.SettingsRelayDetail(st.Detail)
	default:
		return locales.SettingsRelayConnecting()
	}
}

// reachStatusLine says whether this window can drive the rest of the fleet: the
// other half of relayStatusLine, which is about this machine joining it. Without
// this line a relay that never minted a credential and a fleet with nothing in it
// look the same on the 机器 panel.
func (s *settingsWindow) reachStatusLine() string {
	if s.fleet == nil || !s.relayOn {
		return ""
	}
	switch {
	case s.fleet.RelayClient() != nil:
		if exp := s.fleet.ControlExpiry(); !exp.IsZero() {
			if left := time.Until(exp).Round(time.Minute); left > 0 {
				return locales.SettingsReachReadyLeft(left.String())
			}
		}
		return locales.SettingsReachReady()
	case s.fleet.RelayClientErr() != "":
		return locales.SettingsReachFailed(s.fleet.RelayClientErr())
	default:
		return locales.SettingsReachAsking()
	}
}

// fetchCA dials the relay and pins the certificate it presents. It is the same
// trust decision a browser asks a person to make when it warns about a
// self-signed certificate, with two differences: the certificate is shown
// afterwards so it is clear what was trusted, and there is no file to find and
// no path to type.
func (s *settingsWindow) fetchCA() {
	relay := strings.TrimSpace(s.relay)
	if relay == "" {
		s.setNote(locales.SettingsCaNeedUrl())
		return
	}
	s.setNote(locales.SettingsCaFetching(relay))
	go func() {
		path, err := fetchAndPinCA(relay, s.store.Dir(), 15*time.Second)
		if err != nil {
			s.setNote(locales.SettingsCaFetchFailed(err.Error()))
			return
		}
		s.pinCA(path)
	}()
}

// fetchAndPinCA fetches the relay's certificate and saves it in dir, returning
// the file it wrote. The network and disk work live here so that the button's
// behaviour can be tested without a window to post results to.
func fetchAndPinCA(relay, dir string, timeout time.Duration) (string, error) {
	pemBytes, err := agent.FetchCACert(relay, timeout)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "relay-ca.pem")
	if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// pinCA adopts path as the CA and saves the settings, so picking a certificate
// takes one click rather than "choose it, then remember to apply".
func (s *settingsWindow) pinCA(path string) {
	s.set(func() {
		s.ca = path
		s.commit()
	})
}

// pickCA lets someone point at a PEM file they already have. The chooser is
// native and blocks until dismissed, so it runs off the UI thread.
func (s *settingsWindow) pickCA() {
	go func() {
		var parent *mygo.Window
		if s.desk != nil {
			parent = s.desk.win
		}
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: parent,
			Title:  locales.SettingsCaDialogTitle(),
			Filters: []mygo.FileFilter{
				{Name: locales.SettingsCaFilter(), Extensions: []string{"pem", "crt", "cer"}},
			},
		})
		if err != nil || len(paths) == 0 || paths[0] == "" {
			return
		}
		s.pinCA(paths[0])
	}()
}

// set runs fn on the UI thread and redraws, so async work can put its result
// on screen.
func (s *settingsWindow) set(fn func()) {
	if s.desk != nil && s.desk.win != nil {
		s.desk.win.Update(fn)
		return
	}
	fn()
}

// setNote replaces the line under the CA field, without touching the saved
// settings.
func (s *settingsWindow) setNote(note string) {
	s.set(func() { s.caNote = note })
}

func (s *settingsWindow) serverPane(c *ui.Context) {
	t := c.Theme()
	ui.Form(c, func() {
		// The description says what turning this on asks the system for. macOS
		// shows its application-firewall prompt the first time a listening
		// socket appears, and it has no API for an app to pre-authorise itself:
		// the only way through is the user clicking 允许 once, after which it
		// never asks again. A person who meets that prompt cold reads it as the
		// app doing something suspicious, so it is named here, before it happens.
		ui.Field(c, locales.SettingsServer(), func() {
			ui.Switch(c, &s.server).AlignSelf(ui.Start).OnChange(func() { s.commit() })
		}).Description(locales.SettingsServerDesc())
		// The machine's own shells, and who may type into them. It is here rather
		// than on the remote pane because it is not about the relay: a browser on
		// the LAN answers to it too. It applies on the click, like the switch
		// above, because a switch that guards a machine should not wait for 应用.
		ui.Field(c, locales.SettingsControl(), func() {
			ui.Switch(c, &s.control).AlignSelf(ui.Start).OnChange(func() { s.commit() })
		}).Description(locales.SettingsControlDesc())
		if url := s.fleet.BaseURL(); url != "" {
			paneNote(c, locales.SettingsServerUrlLine(url), t.TextMuted)
		} else {
			paneNote(c, locales.SettingsServerOffLine(), t.TextMuted)
		}
		// What the switch means, said the way the fleet works: joining a relay is
		// the consent, so this is a way to say no — not a confirmation anyone has
		// to give for each machine that wants in.
		if s.control {
			paneNote(c, locales.SettingsControlOnLine(), t.TextMuted)
		} else {
			paneNote(c, locales.SettingsControlOffLine(), t.Warning)
		}
	})
}

// commit writes the draft to disk and applies it to the running shell: the font
// to the open terminals, the relay link, and the local service.
func (s *settingsWindow) commit() {
	// Read the draft before taking the store's lock: draft() reads the store,
	// and asking for it inside Update would deadlock.
	draft := s.draft()
	stored, err := s.store.Update(func(v *settings.Settings) {
		*v = draft
	})
	if err != nil {
		s.fail(locales.SettingsSaveFailed(err.Error()))
		return
	}
	s.caNote = caNoteFor(s.ca)
	s.desk.reletter(termFont(stored))
	// The language is applied here rather than on a keystroke, like the relay:
	// the window redraws in the new language at once, which is the answer to
	// the click that chose it. A commit that left the language alone says
	// nothing, because saying it again would redraw every window for nothing.
	if lang := normalizeLanguage(stored.Language); lang != s.applied {
		s.applied = lang
		applyLanguage(lang)
	}
	if err := s.fleet.SetRelay(stored.RelayURL, stored.DeviceID, stored.DeviceSecret, stored.RelayCA, stored.RelayOn); err != nil {
		s.fail(locales.SettingsRelayFailed(err.Error()))
		return
	}
	// The client link is a second connection with a credential of its own: the
	// member one is how this machine joins the fleet, and this one — asked for on
	// the spot, short-lived — is what lets the window drive the other machines'
	// shells. Asking needs the member link to be dialling, so this comes after
	// the relay above.
	if stored.RelayOn && stored.RelayURL != "" {
		if err := s.fleet.Reach(); err != nil {
			s.fail(locales.SettingsTokenFailed(err.Error()))
			return
		}
	} else if err := s.fleet.SetRelayClient("", "", ""); err != nil {
		s.fail(locales.SettingsClientFailed(err.Error()))
		return
	}
	if err := s.fleet.SetServer(!stored.ServerOff); err != nil {
		s.fail(locales.SettingsServerFailed(err.Error()))
		return
	}
	// Who may drive this machine's shells. It is applied to the hub, so a client
	// that is holding a shell right now loses it in the same frame the switch
	// moves, rather than at the next reconnect.
	s.fleet.SetRemoteControl(!stored.ControlOff)
	if s.desk != nil {
		s.desk.refreshBadge()
	}
	s.savedAt = time.Now()
	s.statusErr = false
	s.status = s.summary(stored)
}

// fail records that the draft was not applied, so the window can show it in the
// colour that reads as "this did not work" instead of looking like a save.
func (s *settingsWindow) fail(message string) {
	s.statusErr = true
	s.status = message
}

func (s *settingsWindow) summary(stored settings.Settings) string {
	parts := []string{locales.SettingsSavedAt(s.savedAt.Format("15:04:05"))}
	if url := s.fleet.BaseURL(); url != "" {
		parts = append(parts, locales.SettingsServiceUrl(url))
	} else {
		parts = append(parts, locales.ServiceOff())
	}
	if stored.RelayOn && stored.RelayURL != "" {
		parts = append(parts, locales.SettingsJoined(stored.RelayURL))
	}
	if s.fleet.RelayClient() != nil {
		parts = append(parts, locales.SettingsCanReach())
	}
	return strings.Join(parts, " · ")
}

func (s *settingsWindow) restoreDefaults() {
	s.family = systemFamily()
	s.size = settings.DefaultFontSize
	s.relay = ""
	s.device = ""
	s.secret = ""
	s.ca = ""
	s.caNote = caNoteFor("")
	s.relayOn = false
	s.server = true
	s.control = true
	s.notifyWaiting = true
	s.notifyDone = true
	s.notifyError = true
	s.dockBadge = true
	// The language goes back to following the system, like every other default:
	// a person who restore-defaults their way out of a language they cannot
	// read gets the one their system is in.
	s.language = ""
	s.commit()
	// 恢复默认 is every default, the shortcuts included.
	if _, err := s.store.Update(func(st *settings.Settings) { st.Keybinds = nil }); err == nil && s.desk != nil && s.desk.win != nil {
		installMenu(s.desk)
		s.desk.invalidate()
	}
}
