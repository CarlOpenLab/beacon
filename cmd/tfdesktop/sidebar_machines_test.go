package main

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/beacon/internal/protocol"
	"github.com/CarlOpenLab/beacon/internal/settings"
	"github.com/CarlOpenLab/beacon/internal/tunnel"
	"github.com/CarlOpenLab/beacon/locales"
)

// The machines with nothing under them share one row, and the row opens them
// into ordinary headers and takes them away again.
func TestQuietMachinesFoldIntoOthers(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	w.fleet = newTestFleet(t)
	w.roster = []tunnel.Machine{
		{ID: "m1", Name: "busy box", Online: true},
		{ID: "m2", Name: "quiet one", Online: true},
		{ID: "m3", Name: "quiet two", Online: true},
		{ID: "m4", Name: "quiet three", Online: true},
		{ID: "m5", Name: "quiet four", Online: true},
	}
	w.remoteSessions = map[string][]protocol.Summary{
		"m1": {{SessionID: "r1", Title: "remote shell", Alive: true, Cols: 80, Rows: 24}},
	}

	tt := drawNav(t, w)
	for _, name := range []string{"busy box", w.machineName(machineLocal)} {
		if !tt.HasText(name) {
			t.Fatalf("%q does not have a header; texts = %v", name, tt.Texts())
		}
	}
	if !tt.HasText(locales.SidebarOthers(4)) {
		t.Fatalf("the column has no other row for four machines; texts = %v", tt.Texts())
	}
	for _, name := range []string{"quiet one", "quiet two", "quiet three", "quiet four"} {
		if tt.HasText(name) {
			t.Fatalf("%q keeps a header while it is folded; texts = %v", name, tt.Texts())
		}
	}

	if err := tt.Click(locales.SidebarOthers(4)); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	for _, name := range []string{"quiet one", "quiet two", "quiet three", "quiet four"} {
		if !tt.HasText(name) {
			t.Fatalf("%q did not grow under the other row; texts = %v", name, tt.Texts())
		}
	}

	if err := tt.Click(locales.SidebarOthers(4)); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	for _, name := range []string{"quiet one", "quiet two", "quiet three", "quiet four"} {
		if tt.HasText(name) {
			t.Fatalf("%q stayed after the other row folded; texts = %v", name, tt.Texts())
		}
	}
}

// A machine this window has no rows for still keeps its header while one of
// its sessions needs an answer: the loudest state outranks the quiet list.
func TestAMachineNeedingYouIsNotOther(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	w.fleet = newTestFleet(t)
	w.roster = []tunnel.Machine{
		{ID: "m1", Name: "waiting box", Online: true},
		{ID: "m2", Name: "quiet one", Online: true},
		{ID: "m3", Name: "quiet two", Online: true},
	}
	w.remoteSessions = map[string][]protocol.Summary{
		"m1": {{SessionID: "r1", Title: "asks a question", Alive: true, Status: "waiting"}},
	}

	tt := drawNav(t, w)
	if !tt.HasText("waiting box") {
		t.Fatalf("the waiting machine lost its header; texts = %v", tt.Texts())
	}
	if !tt.HasText(locales.SidebarOthers(2)) {
		t.Fatalf("the other row counts the waiting machine; texts = %v", tt.Texts())
	}
	if tt.HasText("quiet one") || tt.HasText("quiet two") {
		t.Fatalf("a quiet machine kept a header; texts = %v", tt.Texts())
	}
}

// The machine header's menu takes the whole machine out of the column, sessions
// and all, and the choice reaches the settings file.
func TestHidingAMachineFromItsMenuPersists(t *testing.T) {
	w, dir := newTestShellWindow(t, "one")
	w.fleet = newTestFleet(t)
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
	// hidden set, and only this window's own folded state is forgotten.
	stored := settings.Load(dir)
	if !slices.Contains(stored.HiddenMachines, "m2") {
		t.Fatalf("the settings file holds %v, want m2 hidden", stored.HiddenMachines)
	}
	if !hiddenSet(stored.HiddenMachines)["m2"] {
		t.Fatal("a restart would not read m2 back as hidden")
	}
}

// The window is built from the file, so a restart keeps the machines the
// person put away and starts the other row folded again: the open state was
// only ever a look.
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
	if w.othersOpen {
		t.Fatal("a new window started with the other row open, but that state is not persisted")
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

// A hidden machine comes back for a session that needs the user, and leaves
// again a breath after that need passes — not on the frame that answered it.
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
