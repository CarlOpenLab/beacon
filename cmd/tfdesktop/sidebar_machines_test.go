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

// Picking a machine this window has no shell on keeps the terminal where it
// was, so the column must not take that click as "this is where I am": the
// machine stays in the other row, and the note under a folded machine is not
// repeated for every name in it.
func TestClickingAQuietMachineKeepsItInTheOthers(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	w.fleet = newTestFleet(t)
	w.collapsed = make(map[string]bool)
	w.foldByHand = make(map[string]bool)
	w.roster = []tunnel.Machine{{ID: "m1", Name: "quiet one", Online: true}}

	tt := drawNav(t, w)
	tt.Frame()
	if !tt.HasText(locales.SidebarOthers(1)) {
		t.Fatalf("the quiet machine has no other row; texts = %v", tt.Texts())
	}

	if err := tt.Click(locales.SidebarOthers(1)); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	if err := tt.Click("quiet one"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	tt.Frame()

	// Still folded away: the panes did not move, so neither does the column.
	if err := tt.Click(locales.SidebarOthers(1)); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	if !tt.HasText(locales.SidebarOthers(1)) {
		t.Fatalf("the other row is gone after a click that changed nothing; texts = %v", tt.Texts())
	}
	if n := countText(tt.Texts(), locales.SidebarMachineLook()); n != 0 {
		t.Fatalf("the folded machine still teaches under itself %d times; texts = %v", n, tt.Texts())
	}
}

func countText(texts []string, want string) int {
	n := 0
	for _, s := range texts {
		if s == want {
			n++
		}
	}
	return n
}

// Folding a group by hand is movement, so a person who asked the system for
// less motion gets the column where it lands in the first frame; and a click on
// the header, the way a person makes it, is a fold by hand — the flag must not
// be spent on the frame that draws the click.
func TestFoldingAGroupByClickFollowsMotionPreferences(t *testing.T) {
	for _, reduce := range []bool{false, true} {
		w, _ := newTestShellWindow(t, "one")
		w.fleet = newTestFleet(t)
		w.collapsed = make(map[string]bool)
		w.foldByHand = make(map[string]bool)
		w.roster = []tunnel.Machine{{ID: "m2", Name: "second", Online: true}}
		w.remoteSessions = map[string][]protocol.Summary{
			"m2": {{SessionID: "r1", Title: "remote one", Alive: true}},
		}

		tt := drawNav(t, w)
		if reduce {
			tt.SetPreferences(ui.Preferences{ReduceMotion: true})
		}
		tt.Frame()
		start, ok := tt.Find("second")
		if !ok {
			t.Fatalf("the second machine is not in the column; texts = %v", tt.Texts())
		}

		if err := tt.Click(w.machineName(machineLocal)); err != nil {
			t.Fatal(err)
		}
		tt.Advance(16 * time.Millisecond)
		tt.Frame()
		early, ok := tt.Find("second")
		if !ok {
			t.Fatalf("reduce=%v: the second machine left the column", reduce)
		}
		tt.Advance(400 * time.Millisecond)
		tt.Frame()
		rest, ok := tt.Find("second")
		if !ok {
			t.Fatalf("reduce=%v: the second machine left the column", reduce)
		}
		if rest.Y >= start.Y {
			t.Fatalf("reduce=%v: the group did not fold: rest y=%.1f, start y=%.1f", reduce, rest.Y, start.Y)
		}
		switch {
		case reduce && early.Y != rest.Y:
			t.Fatalf("less motion left the column moving: y=%.1f then %.1f", early.Y, rest.Y)
		case !reduce && early.Y <= rest.Y:
			t.Fatalf("the fold went in one frame though motion is allowed: y=%.1f then %.1f", early.Y, rest.Y)
		}
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
		if err := tt.Click(w.machineName(machineLocal)); err != nil {
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
