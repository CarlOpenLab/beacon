package main

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/beacon/internal/protocol"
	"github.com/CarlOpenLab/beacon/internal/settings"
	"github.com/CarlOpenLab/beacon/internal/tunnel"
	"github.com/CarlOpenLab/beacon/locales"
)

// The rail is the fleet as a row of tiles: this machine first, then the
// roster's order, with hidden machines left out. The order never follows state,
// because a row that reordered itself would move the machine under the pointer
// that was about to click it.
func TestTheMachineRailListsShownMachinesInOrder(t *testing.T) {
	w, _ := newTestShellWindow(t, "local shell")
	w.fleet = newTestFleet(t)
	w.roster = []tunnel.Machine{
		{ID: "m1", Name: "one.local", Online: true},
		{ID: "m2", Name: "two", Online: true},
		{ID: "m3", Name: "gone", Online: true},
	}
	if err := w.setMachineHidden("m3", true); err != nil {
		t.Fatal(err)
	}

	tt := drawNav(t, w)
	var xs []float32
	for _, m := range w.groups()[:3] {
		label := w.machineRailLabel(m)
		r, ok := tt.Find(label)
		if !ok {
			t.Fatalf("%q is missing from the rail; texts = %v", label, tt.Texts())
		}
		xs = append(xs, r.X)
	}
	for i := 1; i < len(xs); i++ {
		if xs[i] <= xs[i-1] {
			t.Fatalf("the rail is out of order: x = %v", xs)
		}
	}
	if _, ok := tt.Find(w.machineRailLabel(w.roster[2])); ok {
		t.Fatal("a hidden machine is still in the rail")
	}
}

// The accessibility label is the full name and the status word, and nothing
// else: the glyph is decoration and the machine type is not the machine's name.
// The name is a display name, so a hostname's mDNS suffix never reaches a
// screen reader.
func TestTheMachineRailLabelIsNameAndStatusOnly(t *testing.T) {
	w, _ := newTestShellWindow(t)
	w.fleet = newTestFleet(t)
	w.roster = []tunnel.Machine{{ID: "m2", Name: "db.local", Online: true}}

	cases := []struct {
		status string
		unread bool
		want   string
	}{
		{"waiting", false, locales.FooterNeedsYou()},
		{"working", false, locales.FooterWorking()},
		{"error", false, locales.StatusFailed()},
		{"idle", true, locales.StatusDone()},
		{"idle", false, locales.StatusIdle()},
		{"", false, locales.StatusIdle()},
	}
	for _, tc := range cases {
		w.remoteSessions = map[string][]protocol.Summary{
			"m2": {{SessionID: "r1", Title: "query", Alive: true, Status: tc.status, Unread: tc.unread}},
		}
		got := w.machineRailLabel(w.roster[0])
		if want := "db" + locales.MachineListSep() + tc.want; got != want {
			t.Errorf("label for %q = %q, want %q", tc.status, got, want)
		}
		for _, word := range []string{"Database", "数据库", "Server", "server", "laptop"} {
			if strings.Contains(got, word) {
				t.Errorf("label %q carries the icon or machine type %q", got, word)
			}
		}
	}
}

// The display name drops the mDNS suffix a local network adds. A name that was
// nothing but the suffix falls back to the id rather than becoming empty.
func TestMachineDisplayHostStripsTheLocalSuffix(t *testing.T) {
	cases := []struct{ name, id, want string }{
		{"studio.local", "m2", "studio"},
		{"studio.localdomain", "m2", "studio"},
		{"studio.LOCAL", "m2", "studio"},
		{" studio.local. ", "m2", "studio"},
		{"studio", "m2", "studio"},
		{"", "m2", "m2"},
		{".local", "m2", "m2"},
	}
	for _, tc := range cases {
		if got := machineDisplayHost(tc.name, tc.id); got != tc.want {
			t.Errorf("machineDisplayHost(%q, %q) = %q, want %q", tc.name, tc.id, got, tc.want)
		}
	}
}

// A machine nobody picked for wears the pet its name hashes to: the same name
// lands on the same pet across restarts, casing and stray spaces do not move
// it, and a fleet spreads across the catalog instead of everyone wearing the
// first pet.
func TestMachinePetFollowsTheNameStably(t *testing.T) {
	keys := make(map[string]bool, len(petCatalog))
	for _, p := range petCatalog {
		keys[p.key] = true
	}
	first := machineIconName("studio")
	if !keys[first] {
		t.Fatalf("machineIconName(%q) = %q, which is not a pet in the catalog", "studio", first)
	}
	for _, variant := range []string{"Studio", "STUDIO", " studio ", "studio.local"} {
		if got := machineIconName(variant); got != first {
			t.Errorf("machineIconName(%q) = %q, want %q", variant, got, first)
		}
	}
	seen := map[string]bool{}
	for i := 0; i < 4*len(petCatalog); i++ {
		seen[machineIconName(fmt.Sprintf("beacon-host-%d", i))] = true
	}
	if len(seen) < 2 {
		t.Errorf("%d names all landed on one pet; the hash never spreads", 4*len(petCatalog))
	}
}

// The catalog is four families of eight, every pet carrying a parsed drawing,
// a name in both languages, and a key no other pet holds.
func TestThePetCatalogIsFourFamiliesOfEight(t *testing.T) {
	if len(petCatalog) != 32 {
		t.Fatalf("the catalog holds %d pets, want 32", len(petCatalog))
	}
	seen := map[string]bool{}
	families := map[string]int{}
	for _, p := range petCatalog {
		if seen[p.key] {
			t.Errorf("pet %q appears twice", p.key)
		}
		seen[p.key] = true
		families[p.series]++
		if petGlyphs[p.key] == nil {
			t.Errorf("pet %s has no drawing", p.key)
		}
		if p.zh == "" || p.en == "" {
			t.Errorf("pet %s is missing a name", p.key)
		}
	}
	if len(families) != 4 {
		t.Fatalf("the catalog has %d families, want 4", len(families))
	}
	for family, n := range families {
		if n != 8 {
			t.Errorf("family %s holds %d pets, want 8", family, n)
		}
	}
	// Every pet reports a non-empty name matching the language in force.
	for _, p := range petCatalog {
		if p.name() == "" {
			t.Errorf("pet %s has empty name", p.key)
		}
	}
}

// Every pet is 16 rows of 16 characters on the three-character palette, keeps
// a transparent margin so it never touches its tile's rim, and draws enough
// body to be seen at one pixel to a point.
func TestEveryPetIsASixteenBySixteenDrawing(t *testing.T) {
	for _, p := range petCatalog {
		if len(p.rows) != 16 {
			t.Errorf("%s has %d rows, want 16", p.key, len(p.rows))
			continue
		}
		ink := 0
		minX, maxX := 16, -1
		for y, row := range p.rows {
			if len(row) != 16 {
				t.Errorf("%s row %d is %d wide, want 16", p.key, y, len(row))
				continue
			}
			for x := 0; x < 16; x++ {
				ch := row[x]
				if ch != '#' && ch != '+' && ch != '.' {
					t.Errorf("%s row %d draws %q, want # + or .", p.key, y, ch)
				}
				if ch != '.' {
					ink++
					minX = min(minX, x)
					maxX = max(maxX, x)
				}
			}
		}
		if ink == 0 {
			t.Errorf("%s draws nothing", p.key)
		}
		if minX < 1 || maxX > 14 {
			t.Errorf("%s reaches columns %d..%d: a pet keeps a margin inside its tile", p.key, minX, maxX)
		}
	}
}

// Picking a pet from the machine card's menu stores it with the machine and
// applies it to the rail at once. The settings file is the source of truth, so
// a restart keeps the choice.
func TestTheMachineIconMenuStoresAChoice(t *testing.T) {
	w, dir := newTestShellWindow(t)
	w.fleet = newTestFleet(t)
	w.machineSel = "m2"
	w.roster = []tunnel.Machine{{ID: "m2", Name: "studio", Online: true}}

	tt := drawNav(t, w)
	if err := tt.RightClick("studio"); err != nil {
		t.Fatalf("right-click the machine: %v", err)
	}
	// The machine card's menu keeps its other actions, so the picker is one
	// item among them; choosing it lays the drawings out below the card.
	if err := tt.ChooseMenuItem(locales.MachineIconMenu()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()

	if err := tt.Click("史莱姆 Gloop"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()

	if got := w.machineIcons["m2"]; got != "gloop" {
		t.Fatalf("the window holds %q, want gloop", got)
	}
	if got := settings.Load(dir).MachineIcons["m2"]; got != "gloop" {
		t.Fatalf("the settings file holds %q, want gloop", got)
	}
	if got := w.machineIconKey(w.roster[0]); got != "gloop" {
		t.Fatalf("the rail draws %q, want gloop", got)
	}
}

// The rail tile needs no menu at all: right-click is the picker, and the pets
// on it are the choices.
func TestRailIconMenuStoresAChoice(t *testing.T) {
	w, dir := newTestShellWindow(t)
	w.roster = []tunnel.Machine{{ID: "m2", Name: "studio", Online: true}}
	tt := drawNav(t, w)
	if err := tt.RightClick(w.machineRailLabel(w.roster[0])); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("圆雀 Cheep"); err != nil {
		t.Fatal(err)
	}
	if got := settings.Load(dir).MachineIcons["m2"]; got != "cheep" {
		t.Fatalf("rail icon choice saved as %q, want cheep", got)
	}
}

func TestShortMachineRailIsCenteredAndInset(t *testing.T) {
	for _, width := range []float32{200, 268, 440} {
		w, _ := newTestShellWindow(t)
		w.sidebarWidth = width
		w.roster = []tunnel.Machine{{ID: "m2", Name: "studio", Online: true}}
		tt := drawNav(t, w)
		machines := w.groups()
		first, ok := tt.Find(w.machineRailLabel(machines[0]))
		if !ok {
			t.Fatal("first machine missing")
		}
		last, ok := tt.Find(w.machineRailLabel(machines[1]))
		if !ok {
			t.Fatal("last machine missing")
		}
		if got := (first.X + last.X + last.W) / 2; got != width/2 {
			t.Errorf("width %v: rail center = %v, want %v", width, got, width/2)
		}
		if bottomGap := 420 - (first.Y + first.H); bottomGap < 8 {
			t.Errorf("width %v: bottom gap = %v, want at least 8", width, bottomGap)
		}
		rail, ok := tt.Find("machine rail")
		if !ok {
			t.Fatal("machine rail missing")
		}
		topGap := first.Y - rail.Y
		bottomGap := rail.Y + rail.H - first.Y - first.H
		if topGap != bottomGap || topGap != 8 {
			t.Errorf("width %v: rail padding top=%v bottom=%v, want 8 on both sides", width, topGap, bottomGap)
		}
	}
}

// Picking a quiet machine in the rail is a switch of the column, not of the
// terminal: its header and its empty note appear, while the shell on screen
// stays where it was.
func TestChoosingAMachineInTheRailShowsItsSessions(t *testing.T) {
	w, _ := newTestShellWindow(t, "local shell")
	w.fleet = newTestFleet(t)
	w.roster = []tunnel.Machine{{ID: "m2", Name: "quiet one", Online: true}}

	tt := drawNav(t, w)
	if err := tt.Click(w.machineRailLabel(w.roster[0])); err != nil {
		t.Fatal(err)
	}
	tt.Frame()

	if got := w.selectedMachine(); got != "m2" {
		t.Fatalf("selected machine = %q, want m2", got)
	}
	if tb := w.activeTab(); tb == nil || tb.machine != machineLocal {
		t.Fatal("picking a machine moved the terminal off this machine")
	}
	if _, ok := tt.Find("quiet one"); !ok {
		t.Fatalf("the column does not show the picked machine; texts = %v", tt.Texts())
	}
	if !tt.HasText(locales.SidebarMachineLook()) {
		t.Fatalf("the picked machine has no empty note; texts = %v", tt.Texts())
	}
}

// The machine header's menu takes the whole machine out of the column and the
// rail, sessions and all, and the choice reaches the settings file.
func TestHidingAMachineFromItsMenuPersists(t *testing.T) {
	w, dir := newTestShellWindow(t, "one")
	w.fleet = newTestFleet(t)
	w.machineSel = "m2"
	w.roster = []tunnel.Machine{{ID: "m2", Name: "hidden box", Online: true}}
	w.remoteSessions = map[string][]protocol.Summary{
		"m2": {{SessionID: "r1", Title: "remote shell", Alive: true}},
	}

	tt := drawNav(t, w)
	if !tt.HasText("remote shell") {
		t.Fatalf("the machine's session is missing before hiding; texts = %v", tt.Texts())
	}
	if err := tt.RightClick("hidden box"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem(locales.ActionHideMachine()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("hidden box") || tt.HasText("remote shell") {
		t.Fatalf("the hidden machine still shows; texts = %v", tt.Texts())
	}
	if !w.machineHidden("m2") {
		t.Fatal("the window did not take the machine out of the column")
	}

	// The file is the source of truth: a restart reads it back into the same
	// hidden set.
	stored := settings.Load(dir)
	if !slices.Contains(stored.HiddenMachines, "m2") {
		t.Fatalf("the settings file holds %v, want m2 hidden", stored.HiddenMachines)
	}
	if !hiddenSet(stored.HiddenMachines)["m2"] {
		t.Fatal("a restart would not read m2 back as hidden")
	}
}

// The window is built from the file, so a restart keeps the machines the
// person put away: hiding is a setting, not a look.
func TestHiddenMachinesArriveFromTheSettingsFile(t *testing.T) {
	store := settings.Open(t.TempDir())
	if _, err := store.Update(func(st *settings.Settings) {
		st.HiddenMachines = []string{"m2"}
	}); err != nil {
		t.Fatal(err)
	}
	w := newWindow(newTestFleet(t), store, "", "")
	if !w.machineHidden("m2") {
		t.Fatal("a new window did not read m2 back as hidden")
	}
}

// The settings switch is the other half of the menu item: it writes the same
// list, so a machine hidden from either place is hidden from both.
func TestTheSidebarSettingsSwitchWritesTheStore(t *testing.T) {
	store := settings.Open(t.TempDir())
	desk := &window{
		store:  store,
		tabs:   make(map[int]*tab),
		roster: []tunnel.Machine{{ID: "m2", Name: "switch box", Online: true}},
	}
	sw := newSettingsWindow(desk, store, nil)
	sw.pane = paneSidebar

	tt := ui.NewTester(sw.view, 720, 560)
	tt.Frame()
	if !tt.HasText(locales.SettingsSidebarNote()) {
		t.Fatalf("the sidebar settings do not explain the exception; texts = %v", tt.Texts())
	}
	if !tt.HasText("switch box") {
		t.Fatalf("the settings do not list the machine; texts = %v", tt.Texts())
	}

	if err := tt.Click(locales.SettingsSidebarToggle("switch box")); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !desk.machineHidden("m2") {
		t.Fatal("the switch did not hide the machine in the window")
	}
	if got := store.Get().HiddenMachines; !slices.Contains(got, "m2") {
		t.Fatalf("stored HiddenMachines = %v, want m2", got)
	}

	if err := tt.Click(locales.SettingsSidebarToggle("switch box")); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if desk.machineHidden("m2") {
		t.Fatal("the switch did not show the machine again")
	}
	if got := store.Get().HiddenMachines; slices.Contains(got, "m2") {
		t.Fatalf("stored HiddenMachines = %v, want m2 removed", got)
	}
}

// A hidden machine comes back in the rail for a session that needs the user,
// and leaves again a breath after that need passes — not on the frame that
// answered it.
func TestAHiddenMachineReturnsForANeedAndLeavesAgain(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	w.fleet = newTestFleet(t)
	w.roster = []tunnel.Machine{{ID: "m2", Name: "hidden box", Online: true}}
	w.remoteSessions = map[string][]protocol.Summary{
		"m2": {{SessionID: "r1", Title: "asks a question", Alive: true, Status: "waiting"}},
	}
	if err := w.setMachineHidden("m2", true); err != nil {
		t.Fatal(err)
	}

	tt := drawNav(t, w)
	tt.Frame()
	if !tt.HasText("hidden box") {
		t.Fatalf("a hidden machine needing the user did not come back; texts = %v", tt.Texts())
	}

	w.remoteSessions["m2"][0].Status = "idle"
	tt.Frame()
	if !tt.HasText("hidden box") {
		t.Fatal("the machine left on the frame that answered it")
	}
	tt.Advance(1 * time.Second)
	tt.Frame()
	if !tt.HasText("hidden box") {
		t.Fatal("the machine left before its release delay")
	}

	tt.Advance(600 * time.Millisecond)
	tt.Frame()
	if tt.HasText("hidden box") {
		t.Fatalf("the machine stayed after the release delay; texts = %v", tt.Texts())
	}
}

// With nothing left to show, the column says so and makes the second row the
// way to the switch that changes it.
func TestTheEmptyStateOpensTheSidebarSettings(t *testing.T) {
	w, _ := newTestShellWindow(t)
	w.fleet = newTestFleet(t)
	w.roster = []tunnel.Machine{{ID: "m2", Name: "hidden box", Online: true}}
	if err := w.setMachineHidden(machineLocal, true); err != nil {
		t.Fatal(err)
	}
	if err := w.setMachineHidden("m2", true); err != nil {
		t.Fatal(err)
	}

	tt := drawNav(t, w)
	if !tt.HasText(locales.SidebarNoneVisible()) {
		t.Fatalf("the empty column does not say so; texts = %v", tt.Texts())
	}
	if !tt.HasText(locales.SidebarRestoreMachines()) {
		t.Fatalf("the empty column offers no way back; texts = %v", tt.Texts())
	}
	if err := tt.Click(locales.SidebarRestoreMachines()); err != nil {
		t.Fatal(err)
	}
	if !w.settingsOpen || w.settingsContent == nil {
		t.Fatal("the empty state's second row did not open the settings")
	}
	if got := w.settingsContent.pane; got != paneSidebar {
		t.Fatalf("settings opened on pane %d, want the sidebar", got)
	}
}

// The rail's hover copy is this machine's own numbers. The fleet-wide footer
// the counts used to live in is gone, so the words and numbers have to agree
// with the tile's badge.
func TestTheMachineTooltipCountsOneMachine(t *testing.T) {
	w, _ := newTestShellWindow(t)
	w.fleet = newTestFleet(t)
	m := tunnel.Machine{
		ID: "m2", Name: "db.local", Online: true,
		Host: protocol.Host{Platform: "linux", Version: "1.2.3"},
	}
	w.roster = []tunnel.Machine{m}
	w.remoteSessions = map[string][]protocol.Summary{
		"m2": {
			{SessionID: "r1", Status: "waiting", Alive: true},
			{SessionID: "r2", Status: "working", Alive: true},
			{SessionID: "r3", Status: "idle", Alive: true},
		},
	}
	want := "db · Linux · v1.2.3\n" +
		locales.FooterNeedsYou() + " 1 · " +
		locales.FooterWorking() + " 1 · " +
		locales.StatusIdle() + " 1"
	if got := w.machineTooltip(m); got != want {
		t.Fatalf("tooltip = %q, want %q", got, want)
	}
}

// A count of zero is not an answer: an idle machine's tooltip says it is idle
// and nothing else, and a machine with no sessions at all says the same rather
// than three zeros. A platform the app has no word for is left out instead of
// being printed as the host spells it.
func TestTheMachineTooltipSaysOnlyWhatIsTrue(t *testing.T) {
	w, _ := newTestShellWindow(t)
	w.fleet = newTestFleet(t)
	cases := []struct {
		name     string
		m        tunnel.Machine
		sessions []protocol.Summary
		want     string
	}{
		{"idle only", tunnel.Machine{ID: "m2", Name: "db.local", Online: true,
			Host: protocol.Host{Platform: "darwin"}},
			[]protocol.Summary{{SessionID: "r1", Status: "idle", Alive: true}},
			"db · macOS\n" + locales.StatusIdle() + " 1"},
		{"no sessions", tunnel.Machine{ID: "m2", Name: "db.local", Online: true},
			nil, "db\n" + locales.StatusIdle()},
		{"a failed session", tunnel.Machine{ID: "m2", Name: "db.local", Online: true,
			Host: protocol.Host{Platform: "linux"}},
			[]protocol.Summary{{SessionID: "r1", Status: "error", Alive: true}},
			"db · Linux\n" + locales.StatusFailed() + " 1"},
		{"offline, no sessions", tunnel.Machine{ID: "m2", Name: "db.local", Online: false},
			nil, "db\n" + locales.MachineOffline()},
		{"a failure above a question", tunnel.Machine{ID: "m2", Name: "db.local", Online: true},
			[]protocol.Summary{
				{SessionID: "r1", Status: "error", Alive: true},
				{SessionID: "r2", Status: "waiting", Alive: true},
				{SessionID: "r3", Status: "waiting", Alive: true},
			},
			"db\n" + locales.StatusFailed() + " 1 · " + locales.FooterNeedsYou() + " 2"},
		{"a done session", tunnel.Machine{ID: "m2", Name: "db.local", Online: true},
			[]protocol.Summary{{SessionID: "r1", Status: "idle", Alive: true, Unread: true}},
			"db\n" + locales.StatusDone() + " 1"},
		{"two states", tunnel.Machine{ID: "m2", Name: "db.local", Online: true,
			Host: protocol.Host{Platform: "windows"}},
			[]protocol.Summary{
				{SessionID: "r1", Status: "waiting", Alive: true},
				{SessionID: "r2", Status: "waiting", Alive: true},
				{SessionID: "r3", Status: "working", Alive: true},
			},
			"db · Windows\n" + locales.FooterNeedsYou() + " 2 · " + locales.FooterWorking() + " 1"},
	}
	for _, tc := range cases {
		w.roster = []tunnel.Machine{tc.m}
		w.remoteSessions = map[string][]protocol.Summary{"m2": tc.sessions}
		got := w.machineTooltip(tc.m)
		if got != tc.want {
			t.Errorf("%s: tooltip = %q, want %q", tc.name, got, tc.want)
		}
		// The tooltip's state line always begins with the word a screen reader
		// says, so the red glyph and the hover copy cannot disagree.
		last := got[strings.LastIndex(got, "\n")+1:]
		if word := w.machineStatusWord(tc.m); !strings.HasPrefix(last, word) {
			t.Errorf("%s: tooltip says %q but the label says %q", tc.name, last, word)
		}
	}
}

// The rail's dot is the machine's highest state, ranked failed > needs-you >
// running > done > offline. The ranking is why a machine with both a failure
// and a question wears the failure, and why work in progress is not hidden by a
// done shell that happens to be on the same machine.
func TestTheMachineRailDotRanksStates(t *testing.T) {
	w, _ := newTestShellWindow(t)
	w.roster = []tunnel.Machine{
		{ID: "m2", Name: "online", Online: true},
		{ID: "m3", Name: "offline", Online: false},
	}
	cases := []struct {
		machine string
		list    []protocol.Summary
		want    string
	}{
		{"m2", nil, ""},
		{"m2", []protocol.Summary{{Status: "idle", Alive: true}}, ""},
		{"m2", []protocol.Summary{{Status: "working", Alive: true}}, "running"},
		{"m2", []protocol.Summary{{Status: "idle", Unread: true, Alive: true}}, "done"},
		{"m2", []protocol.Summary{{Status: "waiting", Alive: true}}, "waiting"},
		{"m2", []protocol.Summary{{Status: "error", Alive: true}}, "failed"},
		{"m2", []protocol.Summary{
			{Status: "waiting", Alive: true},
			{Status: "error", Alive: true},
		}, "failed"},
		{"m3", nil, "offline"},
		{"m3", []protocol.Summary{{Status: "idle", Unread: true, Alive: true}}, "done"},
	}
	for _, tc := range cases {
		w.remoteSessions = map[string][]protocol.Summary{tc.machine: tc.list}
		m, ok := w.machineByID(tc.machine)
		if !ok {
			t.Fatalf("machine %q is not in the roster", tc.machine)
		}
		if got := w.railDotState(m); got != tc.want {
			t.Errorf("dot for %q with %d sessions = %q, want %q", tc.machine, len(tc.list), got, tc.want)
		}
	}
}

// ⌘1…⌘9 name machines in the rail's order, not tabs. A number past the last
// machine does nothing rather than wrapping onto the first.
func TestNumberKeysChooseMachinesInTheRail(t *testing.T) {
	w, _ := newTestShellWindow(t)
	w.roster = []tunnel.Machine{{ID: "m2", Name: "second", Online: true}}
	// The machine has a link, so choosing it does not ask the relay for one in
	// the middle of the test; the shortcut is about the rail's order.
	w.remote = map[string]*relayLink{"m2": {link: newLink(), machine: "m2"}}

	tt := ui.NewTester(w.view, 900, 600)
	tt.Frame()

	tt.Key(ui.Cmd, ui.Key2)
	tt.Frame()
	if got := w.selectedMachine(); got != "m2" {
		t.Fatalf("⌘2 chose %q, want the second machine", got)
	}

	tt.Key(ui.Cmd, ui.Key3)
	tt.Frame()
	if got := w.selectedMachine(); got != "m2" {
		t.Fatalf("⌘3 with two machines chose %q, want no change", got)
	}

	tt.Key(ui.Cmd, ui.Key1)
	tt.Frame()
	if got := w.selectedMachine(); got != machineLocal {
		t.Fatalf("⌘1 chose %q, want this machine", got)
	}
}

// Folding a group by hand is movement, so a person who asked the system for
// less motion gets the rows where they land, in the first frame: the framework
// takes every animated value to its target under ReduceMotion, and this is the
// group most likely to be folded a hundred times a day.
func TestFoldingAGroupLandsAtOnceWithLessMotion(t *testing.T) {
	for _, reduce := range []bool{false, true} {
		w, _ := newTestShellWindow(t, "one")
		w.fleet = newTestFleet(t)
		w.collapsed = make(map[string]bool)
		w.foldByHand = make(map[string]bool)

		tt := drawNav(t, w)
		if reduce {
			tt.SetPreferences(ui.Preferences{ReduceMotion: true})
			tt.Frame()
		}
		if err := tt.Click(w.machineDisplayName(machineLocal)); err != nil {
			t.Fatal(err)
		}
		tt.Advance(20 * time.Millisecond)
		tt.Frame()
		mid := tt.HasText("one")
		tt.Advance(400 * time.Millisecond)
		tt.Frame()
		end := tt.HasText("one")

		if end {
			t.Fatalf("reduce=%v: the group did not fold; texts = %v", reduce, tt.Texts())
		}
		if reduce && mid {
			t.Fatalf("less motion left a frame with the rows still there")
		}
		if !reduce && !mid {
			t.Fatalf("the fold went in one frame though motion is allowed")
		}
	}
}

// The hot buttons lead the title row: search, settings, then the folding
// control, all on the title's own line.
func TestTheHotButtonsSitBesideTheFoldingButton(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	w.fleet = newTestFleet(t)
	// The toolbar is a fixed overlay owned by the full window, rather than a
	// child of the clipping sidebar. Draw the real view so its absolute frame is
	// the same one used by the desktop window.
	tt := ui.NewTester(w.view, previewWidth, previewHeight)
	tt.Frame()

	fold, ok := tt.Find(locales.ActionToggleSidebar())
	if !ok {
		t.Fatal("the folding button is not on screen")
	}
	search, ok := tt.Find(locales.ActionPalette())
	if !ok {
		t.Fatal("the search button is not on screen")
	}
	gear, ok := tt.Find(locales.ActionSettings())
	if !ok {
		t.Fatal("the settings button is not on screen")
	}
	if got := gear.X - (search.X + search.W); got != 2 {
		t.Errorf("settings is %v points from search, want 2", got)
	}
	if got := fold.X - (gear.X + gear.W); got != 2 {
		t.Errorf("folding button is %v points from settings, want 2", got)
	}
	// Their boxes differ in size; the row centres them, so it is the centres
	// that have to agree, not the tops.
	if a, b := search.Y+search.H/2, fold.Y+fold.H/2; a != b {
		t.Errorf("search is centred at y=%v and the folding button at y=%v", a, b)
	}
	if a, b := gear.Y+gear.H/2, fold.Y+fold.H/2; a != b {
		t.Errorf("settings is centred at y=%v and the folding button at y=%v", a, b)
	}
}
