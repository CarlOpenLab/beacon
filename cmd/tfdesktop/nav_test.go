package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/protocol"
	"github.com/CarlOpenLab/term-fleet/internal/settings"
	"github.com/CarlOpenLab/term-fleet/internal/tunnel"
	"github.com/CarlOpenLab/term-fleet/internal/uitokens"
	"github.com/CarlOpenLab/term-fleet/locales"
)

// newTestShellWindow is a shell window with shells open and no hub behind it:
// the rail reads the tab order and the labels, and nothing else.
func newTestShellWindow(t *testing.T, titles ...string) (*window, string) {
	t.Helper()
	dir := t.TempDir()
	w := &window{
		store:        settings.Open(dir),
		tabs:         make(map[int]*tab),
		sidebarSplit: -1,
		nextKey:      1,
	}
	for i, title := range titles {
		id := fmt.Sprintf("s%d", i+1)
		w.tabs[i+1] = &tab{id: id, machine: machineLocal, title: title, ordinal: i + 1}
		w.order = append(w.order, i+1)
	}
	return w, dir
}

// drawNav draws the window's navigation the way its view draws it: the sidebar
// of shells and machines. The appearance comes from the store, and the shells
// from what the window has open.
func drawNav(t *testing.T, w *window) *ui.Tester {
	t.Helper()
	return ui.NewTester(func(c *ui.Context) {
		mode := w.store.Get().Theme
		applyTheme(c, mode)
		ui.Row(c).Fill().Children(func() {
			w.sidebar(c, paletteFor(c))
		})
	}, int(w.sideWidth()), 420)
}

func TestPrintSidebarLayout(t *testing.T) {
	w, _ := newTestShellWindow(t, "zsh 1")
	tt := drawNav(t, w)
	for _, text := range tt.Texts() {
		if r, ok := tt.Find(text); ok {
			t.Logf("Found %q at X=%.1f Y=%.1f W=%.1f H=%.1f", text, r.X, r.Y, r.W, r.H)
		}
	}
}

func TestSidebarGroupsMachinesInOrder(t *testing.T) {
	w, _ := newTestShellWindow(t, "zsh 1")
	w.roster = []tunnel.Machine{
		{ID: "m1", Name: "carl-windows", Online: false},
		{ID: "m2", Name: "win-20240410OWZ", Online: true},
	}
	tt := drawNav(t, w)

	// This machine's group first, then the roster's order: the column is a list
	// of machines, and the machine a person is sitting at leads it.
	local, ok := tt.Find(w.machineName(machineLocal))
	if !ok {
		t.Fatalf("this machine's group is missing; texts = %v", tt.Texts())
	}
	first, ok := tt.Find("carl-windows")
	if !ok {
		t.Fatalf("carl-windows missing; texts = %v", tt.Texts())
	}
	second, ok := tt.Find("win-20240410OWZ")
	if !ok {
		t.Fatalf("win-20240410OWZ missing; texts = %v", tt.Texts())
	}
	if !(local.Y < first.Y && first.Y < second.Y) {
		t.Fatalf("groups are out of order: local %.1f, carl-windows %.1f, win %.1f", local.Y, first.Y, second.Y)
	}

	// The foot is pinned to the bottom of the column, under every group: the
	// palette and the settings do not scroll away with the sessions.
	setRect, ok := tt.Find(locales.ActionSettings())
	if !ok {
		t.Fatalf("设置 is missing from the footer; texts = %v", tt.Texts())
	}
	if setRect.Y <= second.Y {
		t.Fatalf("the footer is not under the groups: win %.1f, 设置 %.1f", second.Y, setRect.Y)
	}
}

func TestSessionPanelListsTheOpenShellsAndPicksOne(t *testing.T) {
	w, _ := newTestShellWindow(t, "one", "two")
	tt := drawNav(t, w)

	for _, want := range []string{"one", "two"} {
		if !tt.HasText(want) {
			t.Fatalf("the column does not show %q; it shows %v", want, tt.Texts())
		}
	}

	if err := tt.Click("two"); err != nil {
		t.Fatalf("click the second shell: %v", err)
	}
	if w.selected != 1 {
		t.Fatalf("selected = %d, want the second shell", w.selected)
	}
	if err := tt.Click("one"); err != nil {
		t.Fatalf("click the first shell: %v", err)
	}
	if w.selected != 0 {
		t.Fatalf("selected = %d, want the first shell", w.selected)
	}
}

// The shells' panel holds the two things a shell needs — one more, and an end to
// this one — as icon buttons beside the panel's title, because that is the panel
// they act on. They have to work from there, not merely be present: the button
// speaks to the hub over the window's own link, and the hub's answer comes back
// as frames.
func TestSessionPanelOffersWorkingShellControls(t *testing.T) {
	f := newTestFleet(t)
	w, _ := newTestShellWindow(t)
	w.fleet = f
	w.local = openLocalLink(f.Hub())
	w.local.onSessions = w.sessionsArrived
	w.local.onGone = w.linkGone

	tt := drawNav(t, w)
	if _, ok := tt.Find(locales.ActionNewTerminalTip()); !ok {
		t.Fatal("the session panel has no new terminal control")
	}

	// A new shell, from the panel's button.
	if err := tt.Click(locales.ActionNewTerminalTip()); err != nil {
		t.Fatalf("Click(新建标签页): %v", err)
	}
	if got := waitSessions(f, 1); got != 1 {
		t.Fatalf("the button left %d shells, want 1", got)
	}

	// The hub pushed the session list, and the window opened a tab for it — the
	// same path a shell started from a browser takes.
	settleTabs(t, w, 1)

	// The row's 结束 is drawn from what the hub said, so the panel is redrawn
	// before it is looked for: the tester's frame is a snapshot, not a live view.
	tt.Frame()

	// ...and an end to the one on screen. 结束 is its own button, on the row of
	// the shell it ends — not the × that closes the tab, which used to be wired
	// to the same act and so killed a process on a machine somewhere every time
	// somebody meant to stop watching it.
	if err := tt.RightClick(w.activeTab().name()); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem(locales.TabEndProcess()); err != nil {
		t.Fatal(err)
	}
	if got := waitSessions(f, 0); got != 0 {
		t.Fatalf("the button left %d shells, want 0", got)
	}
	// The tab stays: the shell is gone, and the tab is where its last output is
	// read. Closing it is a separate action, so it settles to one exited tab
	// rather than to none — and the exit frame is what marks it exited, which is
	// the mark the panel draws the row from: a tab the window never marked is a
	// row still offering to end a shell that is already gone.
	drainFor(t, w, func() bool { return len(w.order) == 1 && !w.tabs[w.order[0]].alive })
	tt.Frame()
	if !tt.HasText(locales.StateEnded()) {
		t.Fatalf("the row of the shell that was just ended does not say so; texts = %v", tt.Texts())
	}
	if tt.HasText(locales.TabEndProcess()) {
		t.Errorf("an ended shell is still offered 结束, which would do nothing; texts = %v", tt.Texts())
	}
	// And the row is not a dead end: the way to stop watching a shell that is
	// already gone is on it, in the place the 结束 was.
	if _, ok := tt.Find(locales.TabCloseNamed(w.tabs[w.order[0]].name())); !ok {
		t.Errorf("the ended shell's row offers no way to close the tab; texts = %v", tt.Texts())
	}

	// And the × that shares the panel's title row closes the tab without
	// touching the process. The new shell is waited for by id rather than by
	// count: the ended one is gone from the roster, so "1 live session" would be
	// true before the new one started.
	//
	// The panel is redrawn before it is clicked: the tester holds a frame, not a
	// live view, and a button drawn after the last Frame is not there to click.
	tt.Frame()
	if err := tt.Click(locales.ActionNewTerminalTip()); err != nil {
		t.Fatalf("Click(新建标签页): %v", err)
	}
	if got := waitSessions(f, 1); got != 1 {
		t.Fatalf("the button left %d live shells, want 1", got)
	}
	// The newest session in the roster is the one just started: the ended one is
	// no longer listed at all.
	list := f.Hub().Sessions()
	fresh := list[len(list)-1].SessionID
	// Both tabs are on screen — the one whose shell has ended and the new one,
	// which is the tab on top and so the tab the panel's × closes.
	drainFor(t, w, func() bool {
		return len(w.order) == 2 && w.selected == 1 && w.tabs[w.order[1]].alive
	})
	tt.Frame()
	// The row's × shows on the hover, so the way out a pointer-less test takes
	// is the row's own menu: the same act, one step further in.
	if err := tt.RightClick(w.activeTab().name()); err != nil {
		t.Fatalf("right-click the row: %v", err)
	}
	if err := tt.ChooseMenuItem(locales.TabClose()); err != nil {
		t.Fatalf("choose 关闭标签页: %v", err)
	}
	if got := liveCount(f); got != 1 {
		t.Fatalf("closing the tab left %d live shells, want the shell still running", got)
	}
	// And the shell really is still running: the roster still holds it.
	if !hasLiveSession(f, fresh) {
		t.Fatal("closing the tab ended the shell it was watching")
	}
	// The closed tab is gone and the ended one is what is left: it is the tab
	// whose shell stopped, and the window keeps it.
	settleTabCount(t, w, 1)
	if w.tabs[w.order[0]].alive {
		t.Fatal("the window closed the tab of a running shell and kept the one whose shell had ended")
	}
}

// A shell that ends keeps its tab — the tab is where its last output is read —
// so the row naming it has to carry the way out of that state. It used to be
// the one row in the panel with no control on it at all: 结束 is hidden once
// there is nothing to end, and nothing took its place, so a row reading 已退出
// was a row a person could only stare at.
func TestAnEndedShellsRowOffersTheWayOut(t *testing.T) {
	w, _ := newTestShellWindow(t, "gone")
	w.fleet = newTestFleet(t)
	tb := w.tabs[1]
	tb.alive = false
	tb.finish(3)

	nav := drawNav(t, w)
	if !nav.HasText(locales.StateEnded()) {
		t.Fatalf("the row does not say the shell has ended; texts = %v", nav.Texts())
	}
	if nav.HasText(locales.TabEndProcess()) {
		t.Errorf("an ended shell is still offered 结束, which would do nothing; texts = %v", nav.Texts())
	}
	if err := nav.Click(locales.TabCloseNamed("gone")); err != nil {
		t.Fatalf("the row of an ended shell has no way to close it: %v; texts = %v", err, nav.Texts())
	}
	if len(w.order) != 0 || len(w.tabs) != 0 {
		t.Errorf("the row's 关闭 left %d tabs, want the tab gone", len(w.order))
	}

	// A shell that is still running keeps its tab until it is closed on purpose:
	// the same row reads 控制中 and offers 结束, which is the other act.
	w2, _ := newTestShellWindow(t, "live")
	w2.fleet = newTestFleet(t)
	tb2 := w2.tabs[1]
	tb2.alive = true
	tb2.setLease(protocol.RoleController, "", 1)
	nav2 := drawNav(t, w2)
	// The × a running row offers is a hover's, so a row at rest keeps only the
	// acts that are always worth offering: its menu holds both, and 结束 is the
	// one that is not simply "stop watching".
	if err := nav2.RightClick("live"); err != nil {
		t.Fatal(err)
	}
	menu := strings.Join(nav2.Menu(), " ")
	if !strings.Contains(menu, locales.TabEndProcess()) {
		t.Fatalf("the process action is missing from the context menu: %q", menu)
	}
	if !strings.Contains(menu, locales.TabClose()) {
		t.Fatalf("the detach action is missing from the context menu: %q", menu)
	}
}

// A row for a shell nobody is driving says so. 控制中 is the ordinary case for a
// row about to be clicked, but it is not this one: it would say a person has a
// keyboard that nobody has, over a shell whose pane reads 只读 — the same
// disagreement the pane, the band and the panel are built to avoid.
func TestARowNamesNobodyWhenNobodyHoldsTheShell(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	w.fleet = newTestFleet(t)
	tb := w.tabs[1]
	tb.alive = true
	tb.machine = "mac-studio"
	tb.setLease(protocol.RoleViewer, "", 2)
	w.remoteSessions = map[string][]protocol.Summary{
		"mac-studio": {{SessionID: tb.id, Title: "one", Cols: 74, Rows: 24, Alive: true}},
	}
	w.machineSel = "mac-studio"
	w.roster = []tunnel.Machine{{
		ID: "mac-studio", Name: "mac studio", Online: true,
		Host: protocol.Host{Platform: "darwin", Arch: "arm64"},
	}}

	nav := drawNav(t, w)
	if !nav.HasText(locales.StateReadonly()) {
		t.Fatalf("the row does not say the shell is nobody's; texts = %v", nav.Texts())
	}
	if nav.HasText(locales.StateControlling()) {
		t.Errorf("the row still claims the shell is being driven; texts = %v", nav.Texts())
	}

	// And once this window takes it, the row is the ordinary one again: no 只读
	// badge, and the line says whose it is.
	tb.setLease(protocol.RoleController, "", 3)
	nav.Frame()
	if nav.HasText(locales.StateNobody()) {
		t.Errorf("the row still says nobody holds a shell this window drives; texts = %v", nav.Texts())
	}

	if nav.HasText(locales.StateReadonly()) {
		t.Errorf("a shell this window drives wears 只读; texts = %v", nav.Texts())
	}
}

// hasLiveSession reports whether the hub still holds a running session by id.
func hasLiveSession(f *fleet, id string) bool {
	for _, s := range f.Hub().Sessions() {
		if s.SessionID == id && s.Alive {
			return true
		}
	}
	return false
}

// waitSessions waits until the hub holds n *live* sessions and returns that
// count. Live, not total: an exited shell stays in the manager so its output can
// still be read, so counting everything would never reach zero after an end.
func waitSessions(f *fleet, n int) int {
	deadline := time.Now().Add(5 * time.Second)
	for {
		got := liveCount(f)
		if got == n || time.Now().After(deadline) {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// liveCount is how many of the hub's sessions are still running.
func liveCount(f *fleet) int {
	n := 0
	for _, s := range f.Hub().Sessions() {
		if s.Alive {
			n++
		}
	}
	return n
}

// settleTabCount waits until the window holds n tabs, whichever of them are
// still alive: a tab of a shell that has ended is one the window keeps — the tab
// is where the shell's last output is read — so a test that has ended a shell
// counts the window's tabs rather than its live ones.
func settleTabCount(t *testing.T, w *window, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		w.drain()
		if len(w.order) == n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("the window holds %d tabs, want %d: order=%v", len(w.order), n, w.order)
}

// settleTabs waits until the window holds n tabs, each named by the hub and
// marked alive by its attached frame. All of them, not just the count: the
// panel's 结束 button is drawn from `alive`, and a tab that has been named but
// not yet attached is not a shell the panel will offer to end. A window with an
// ended shell in it wants settleTabCount instead.
func settleTabs(t *testing.T, w *window, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		w.drain()
		named := 0
		for _, key := range w.order {
			if w.tabs[key].id != "" && w.tabs[key].alive {
				named++
			}
		}
		if named == n && len(w.order) == n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("tabs did not settle to %d live sessions: order=%v", n, w.order)
}

// The machines' panel is the fleet's own view of itself: this machine, the relay
// it is enrolled with when there is one, and a line saying where the other
// machines will appear. A window with no relay says so rather than showing an
// empty panel.
func TestSidebarNamesEveryMachine(t *testing.T) {
	f := newTestFleet(t)
	w, _ := newTestShellWindow(t, "one")
	w.fleet = f

	tt := drawNav(t, w)
	// This machine leads the column as its own group, named as the roster names
	// it. There is no separate machines panel any more: a machine is a group,
	// and its sessions hang under it.
	if !tt.HasText(f.Host().Hostname) {
		t.Fatalf("the column does not name this machine; texts = %v", tt.Texts())
	}

	// The relay is a pipe, not a machine: the only name it has is its address,
	// so the address staying out of the column is the whole guard.
	if err := f.SetRelay("wss://relay.example/relay/agent", "mac-mini", "secret", "", true); err != nil {
		t.Fatalf("SetRelay: %v", err)
	}
	tt.Frame()
	if _, ok := tt.Find("wss://relay.example/relay/agent"); ok {
		t.Fatalf("an enrolled machine shows a relay row: %v", tt.Texts())
	}
}

func TestPickingAnUnreachableMachineAsksFirst(t *testing.T) {
	f := newTestFleet(t)
	w, _ := newTestShellWindow(t, "one")
	w.fleet = f
	// Enrolled, with no client link: the state before the first credential.
	if err := f.SetRelay("wss://relay.example/relay/agent", "mac-mini", "secret", "", true); err != nil {
		t.Fatalf("SetRelay: %v", err)
	}
	w.roster = []tunnel.Machine{{
		ID: "other-box", Name: "other box", Online: true,
		Host: protocol.Host{Platform: "linux", Arch: "amd64"},
	}}

	if w.selectedMachine() != machineLocal {
		t.Fatal("a window does not start on its own machine")
	}

	entry, ok := w.machineByID("other-box")
	if !ok {
		t.Fatal("the roster entry is not reachable by id")
	}
	w.selectMachine(entry)
	if w.selectedMachine() != "other-box" {
		t.Fatalf("picking a machine left the panels on %q", w.selectedMachine())
	}
	if w.reachPrompt != "other-box" {
		t.Fatalf("picking an unreachable machine did not ask for a credential (prompt %q)", w.reachPrompt)
	}

	// The dialog is on screen, with the machine's name and both answers.
	tt := ui.NewTester(w.view, 900, 600)
	tt.Frame()
	if !tt.HasText(locales.ReachTitle("other box")) {
		t.Fatalf("the dialog does not name the machine; texts = %v", tt.Texts())
	}
	if _, ok := tt.Find(locales.ReachConfirm()); !ok {
		t.Fatalf("the dialog does not offer to ask the relay; texts = %v", tt.Texts())
	}
	if _, ok := tt.Find(locales.ActionCancel()); !ok {
		t.Fatalf("the dialog cannot be declined; texts = %v", tt.Texts())
	}

	if err := tt.Click(locales.ActionCancel()); err != nil {
		t.Fatalf("Click(取消): %v", err)
	}
	tt.Frame()
	if w.reachPrompt != "" {
		t.Errorf("拒绝之后对话框还在：%q", w.reachPrompt)
	}
	if w.selectedMachine() != machineLocal {
		t.Errorf("declining left the panels on %q, want this machine", w.selectedMachine())
	}
}

// The window is the app's: a wash behind cards, not a flat dark ground. The two
// appearances are both real - the light one is not the dark one inverted - and a
// card takes the palette's own color in each.
func TestDesktopUsesTheWash(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	tb := w.tabs[1]
	tb.alive = true
	tb.term = fedTerminal(t, previewScreen())

	tt := ui.NewTester(w.view, 900, 600)
	tt.SetDark(true)
	tt.Frame()
	got := tt.Image().RGBAAt(700, 300)
	dark := uitokens.Dark.Card
	if got.R != dark.R || got.G != dark.G || got.B != dark.B {
		t.Fatalf("the dark card is %v, want %v", got, dark)
	}
	// The wash itself shows at the window's edge, beside the card.
	edge := tt.Image().RGBAAt(892, 300)
	if edge.R == dark.R && edge.G == dark.G && edge.B == dark.B {
		t.Fatal("the wash does not show beside the card")
	}

	tt.SetDark(false)
	tt.Frame()
	got = tt.Image().RGBAAt(700, 300)
	light := uitokens.Light.Card
	if got.R != light.R || got.G != light.G || got.B != light.B {
		t.Fatalf("the light card is %v, want %v", got, light)
	}
}

func TestFlipTheme(t *testing.T) {
	cases := []struct {
		mode       string
		systemDark bool
		want       string
	}{
		{themeSystem, false, themeDark}, // the first click always changes something
		{themeSystem, true, themeLight},
		{themeLight, false, themeDark},
		{themeDark, false, themeLight},
		{"Nonsense", false, themeDark}, // an unreadable file still leaves a way out
	}
	for _, tc := range cases {
		if got := flipTheme(tc.mode, tc.systemDark); got != tc.want {
			t.Errorf("flipTheme(%q, %v) = %q, want %q", tc.mode, tc.systemDark, got, tc.want)
		}
	}
}

func TestThemeForFollowsTheSystemUnlessAsked(t *testing.T) {
	if themeFor(themeSystem) != nil {
		t.Error("the system default should leave the ui's own theme alone")
	}
	if !themeFor(themeDark).Dark {
		t.Error("the dark appearance is not dark")
	}
	if themeFor(themeLight).Dark {
		t.Error("the light appearance is dark")
	}
}

func TestSettingsOpensSwitchesPanesAndCloses(t *testing.T) {
	w, _ := newTestShellWindow(t)
	w.fleet = newTestFleet(t)
	tt := ui.NewTester(w.view, 1000, 700)
	if err := tt.Click(locales.ActionSettings()); err != nil {
		t.Fatal(err)
	}
	if !w.settingsOpen || !tt.HasText(locales.SettingsFont()) {
		t.Fatalf("settings did not open: %v", tt.Texts())
	}
	for _, pane := range []string{
		locales.SettingsPaneRelay(), locales.SettingsPaneServer(),
		locales.SettingsPaneUpdates(), locales.SettingsPaneTerminal(),
	} {
		if err := tt.Click(pane); err != nil {
			t.Fatal(err)
		}
		if !w.settingsOpen {
			t.Fatalf("pane %s closed settings: %v", pane, tt.Texts())
		}
	}
	if err := tt.Click(locales.SettingsClose()); err != nil {
		t.Fatal(err)
	}
	if w.settingsOpen {
		t.Fatal("settings did not close")
	}
	tt.SetSize(720, 420)
	tt.Key(ui.Cmd, ui.KeyComma)
	if !w.settingsOpen {
		t.Fatal("settings shortcut did not open the modal")
	}
	if err := tt.Click(locales.SettingsClose()); err != nil {
		t.Fatalf("settings cannot close at the minimum window size: %v", err)
	}
	tt.Key(ui.Cmd, ui.KeyComma)
	tt.Key(0, ui.KeyEscape)
	if w.settingsOpen {
		t.Fatal("Escape did not dismiss settings")
	}
}

// The roster is keyed by device id and the window knows itself by hostname, so
// enrolling this machine with a relay used to put it in the panel twice — once
// as 本机, once as another machine with its own shells. A fleet panel that lists
// you twice is one nobody can read.
func TestSidebarNamesThisMachineOnce(t *testing.T) {
	f := newTestFleet(t)
	w, _ := newTestShellWindow(t, "one")
	w.fleet = f
	w.roster = []tunnel.Machine{
		{ID: "dev-2", Name: "other box", Online: true,
			Host: protocol.Host{Platform: "darwin", Arch: "arm64"}},
	}

	tt := drawNav(t, w)
	if !tt.HasText("other box") {
		t.Fatalf("the column does not list the other machine; texts = %v", tt.Texts())
	}
	// Its own name is on its group's header and nowhere else: a second group
	// would be the roster entry for this machine, which is what the device-id
	// check skips.
	named := 0
	wantHost := f.Host().Hostname
	for _, text := range tt.Texts() {
		if strings.TrimSpace(text) == wantHost {
			named++
		}
	}
	if named != 1 {
		t.Fatalf("this machine is named %d times in the column, want once; texts = %v", named, tt.Texts())
	}
	// The count is this machine plus the other one — the self entry would have
	// made it three.
	if got := w.machineCount(); got != 2 {
		t.Fatalf("machineCount = %d, want this machine plus the other one", got)
	}

	// A machine with no relay still counts itself, and only itself.
	w.roster = nil
	tt.Frame()
	if got := w.machineCount(); got != 1 {
		t.Fatalf("machineCount with no roster = %d, want this machine alone", got)
	}
}

func TestPickingARemoteShellSwitchesToItsTab(t *testing.T) {
	w, _ := newTestShellWindow(t, "local one")
	w.fleet = newTestFleet(t)
	w.roster = []tunnel.Machine{{
		ID: "other-box", Name: "other box", Online: true,
		Host: protocol.Host{Platform: "linux", Arch: "amd64"},
	}}
	w.remoteSessions = map[string][]protocol.Summary{
		"other-box": {
			{SessionID: "s7", Title: "remote one", Cols: 120, Rows: 40, Alive: true},
			{SessionID: "s8", Title: "remote two", Cols: 120, Rows: 40, Alive: true},
		},
	}
	w.machineSel = "other-box"

	// Two tabs on that machine, as clicking each row would have left them.
	first := &tab{id: "s7", machine: "other-box", title: "remote one", ordinal: 1}
	second := &tab{id: "s8", machine: "other-box", title: "remote two", ordinal: 2}
	w.tabs[2], w.tabs[3] = first, second
	w.order = append(w.order, 2, 3)

	// With this machine's shell on screen, neither remote row claims to be it.
	entries := w.sessionEntries()
	if len(entries) != 2 {
		t.Fatalf("the panel lists %d rows, want the machine's two shells", len(entries))
	}
	for _, e := range entries {
		if e.active {
			t.Fatalf("row %q is marked as the shell on screen while it is not", e.label)
		}
	}

	// Picking the second row puts its tab on screen.
	tt := drawNav(t, w)
	if err := tt.Click("remote two"); err != nil {
		t.Fatalf("Click(remote two): %v", err)
	}
	if got := w.activeTab(); got != second {
		t.Fatalf("picking the row left %v on screen, want the second tab", got)
	}
	if got := w.selectedMachine(); got != "other-box" {
		t.Fatalf("the panels moved to %q, want the machine whose shell is on screen", got)
	}
	// ...and the row says so, which is the active state the panel was missing.
	entries = w.sessionEntries()
	if !entries[1].active || entries[0].active {
		t.Fatalf("active rows = %v/%v, want only the second", entries[0].active, entries[1].active)
	}

	// The same click again is a switch, not a second tab for the same shell.
	before := len(w.tabs)
	if err := tt.Click("remote two"); err != nil {
		t.Fatalf("Click(remote two) again: %v", err)
	}
	if len(w.tabs) != before {
		t.Fatalf("picking an open shell opened another tab: %d tabs, want %d", len(w.tabs), before)
	}
}

// A tab strip's own keys: ⌘⇧] and ⌘⇧[ walk the shells this window has open,
// wherever each one runs.
func TestSwitchingShellsWithTheKeyboard(t *testing.T) {
	w, _ := newTestShellWindow(t, "one", "two", "three")
	w.fleet = newTestFleet(t)

	tt := ui.NewTester(func(c *ui.Context) { w.view(c) }, 900, 600)
	tt.Frame()
	if w.activeTab() != w.tabs[1] {
		t.Fatalf("the window does not start on its first shell")
	}

	tt.Key(ui.Cmd|ui.Shift, ui.KeyBracketRight)
	tt.Frame()
	if w.activeTab() != w.tabs[2] {
		t.Fatalf("⌘⇧] left the wrong shell on screen")
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyBracketRight)
	tt.Frame()
	if w.activeTab() != w.tabs[3] {
		t.Fatalf("⌘⇧] left the wrong shell on screen")
	}
	// Round the end, and back.
	tt.Key(ui.Cmd|ui.Shift, ui.KeyBracketRight)
	tt.Frame()
	if w.activeTab() != w.tabs[1] {
		t.Fatalf("⌘⇧] past the last shell did not come round to the first")
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyBracketLeft)
	tt.Frame()
	if w.activeTab() != w.tabs[3] {
		t.Fatalf("⌘⇧[ from the first shell did not come round to the last")
	}
}

// The 会话 panel lists the picked machine's shells and no others. A tab on
// another machine used to be a row in this machine's list — it looked like one
// of this machine's shells — and picking it moved the panels to that machine,
// which reads as switching tabs landing somewhere else entirely.
func TestTheSessionPanelListsOnlyThePickedMachinesShells(t *testing.T) {
	w, _ := newTestShellWindow(t, "local one", "local two")
	w.fleet = newTestFleet(t)
	w.roster = []tunnel.Machine{{
		ID: "other-box", Name: "other box", Online: true,
		Host: protocol.Host{Platform: "linux", Arch: "amd64"},
	}}
	w.remoteSessions = map[string][]protocol.Summary{
		"other-box": {{SessionID: "s7", Title: "on other box", Cols: 120, Rows: 40, Alive: true}},
	}
	remote := &tab{id: "s7", machine: "other-box", title: "on other box", ordinal: 3}
	w.tabs[3] = remote
	w.order = append(w.order, 3)

	labels := func() []string {
		var out []string
		for _, e := range w.sessionEntries() {
			out = append(out, e.label)
		}
		return out
	}

	// This machine picked: its own two shells, and not the one on the other
	// machine, even though this window has a tab for it.
	w.machineSel = machineLocal
	if got := labels(); len(got) != 2 || got[0] != "local one" || got[1] != "local two" {
		t.Fatalf("this machine's list = %v, want its own two shells", got)
	}

	// The other machine picked: its shell, and only it.
	w.machineSel = "other-box"
	if got := labels(); len(got) != 1 || got[0] != "on other box" {
		t.Fatalf("the other machine's list = %v, want the shell it is running", got)
	}
}

func TestTheGroupMenuOffersBothRefreshes(t *testing.T) {
	f := newTestFleet(t)
	w, _ := newTestShellWindow(t)
	w.fleet = f
	w.local = openLocalLink(f.Hub())
	w.local.onSessions = w.sessionsArrived
	w.local.onGone = w.linkGone

	tt := drawNav(t, w)
	// The refreshes live on the group a person is looking at, not in a panel
	// title: right-clicking this machine asks about this machine, and the whole
	// roster has its own answer in the same menu.
	if err := tt.RightClick(w.machineName(machineLocal)); err != nil {
		t.Fatalf("right-click the group: %v", err)
	}
	menu := strings.Join(tt.Menu(), " ")
	if !strings.Contains(menu, locales.SessionRefresh()) {
		t.Fatalf("the group's menu has no session refresh: %q", menu)
	}
	if !strings.Contains(menu, locales.MachineRefresh()) {
		t.Fatalf("the group's menu has no machine refresh: %q", menu)
	}
	if err := tt.ChooseMenuItem(locales.SessionRefresh()); err != nil {
		t.Fatalf("choose 刷新会话: %v", err)
	}
	w.drain()
	if w.notice != locales.AppRefreshedLocal() {
		t.Fatalf("notice after session refresh = %q, want '已刷新本机会话'", w.notice)
	}
}
