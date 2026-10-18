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
			w.sidebar(c, mode)
		})
	}, sidebarWidth, 420)
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
	for _, want := range []string{"新建标签页", "关闭标签页"} {
		if _, ok := tt.Find(want); !ok {
			t.Fatalf("the shells' panel has no %q control; texts = %v", want, tt.Texts())
		}
	}

	// A new shell, from the panel's button.
	if err := tt.Click("新建标签页"); err != nil {
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
	if err := tt.Click("结束"); err != nil {
		t.Fatalf("Click(结束): %v", err)
	}
	if got := waitSessions(f, 0); got != 0 {
		t.Fatalf("the button left %d shells, want 0", got)
	}
	// The tab stays: the shell is gone, and the tab is where its last output is
	// read. Closing it is a separate action, so it settles to one exited tab
	// rather than to none.
	settleTabs(t, w, 1)

	// And the × that shares the panel's title row closes the tab without
	// touching the process. The new shell is waited for by id rather than by
	// count: the ended one is gone from the roster, so "1 live session" would be
	// true before the new one started.
	//
	// The panel is redrawn before it is clicked: the tester holds a frame, not a
	// live view, and a button drawn after the last Frame is not there to click.
	tt.Frame()
	if err := tt.Click("新建标签页"); err != nil {
		t.Fatalf("Click(新建标签页): %v", err)
	}
	if got := waitSessions(f, 1); got != 1 {
		t.Fatalf("the button left %d live shells, want 1", got)
	}
	// The newest session in the roster is the one just started: the ended one is
	// no longer listed at all.
	list := f.Hub().Sessions()
	fresh := list[len(list)-1].SessionID
	settleTabs(t, w, 2)
	tt.Frame()
	if err := tt.Click("关闭标签页"); err != nil {
		t.Fatalf("Click(关闭标签页): %v", err)
	}
	if got := liveCount(f); got != 1 {
		t.Fatalf("closing the tab left %d live shells, want the shell still running", got)
	}
	// And the shell really is still running: the roster still holds it.
	if !hasLiveSession(f, fresh) {
		t.Fatal("closing the tab ended the shell it was watching")
	}
	settleTabs(t, w, 1)
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

// settleTabs waits until the window's tabs name the sessions the hub holds:
// what the links report is posted to the window's thread, and a test applies it
// by draining.
// settleTabs waits until the window holds n tabs, each named by the hub and
// marked alive by its attached frame. Both, not just the name: the panel's 结束
// button is drawn from `alive`, and a tab that has been named but not yet
// attached is not a shell the panel will offer to end.
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
func TestMachinePanelListsOnlyMachines(t *testing.T) {
	f := newTestFleet(t)
	w, _ := newTestShellWindow(t, "one")
	w.fleet = f

	tt := drawNav(t, w)
	if !tt.HasText("机器") {
		t.Fatalf("the column has no machines' panel; texts = %v", tt.Texts())
	}
	if !tt.HasText(f.Host().Hostname) {
		t.Fatalf("the machine row does not name this machine; texts = %v", tt.Texts())
	}
	if !tt.HasText("1 个会话") {
		t.Fatalf("the machine row does not say what this machine is running; texts = %v", tt.Texts())
	}
	if !tt.HasText("只列本机") {
		t.Fatalf("the panel does not say why it holds nothing else; texts = %v", tt.Texts())
	}
	// The relay is a pipe, not a machine: it never gets a row of its own, whether
	// it is off or on.
	if _, ok := tt.Find("中继"); ok {
		t.Fatal("the panel shows a relay row")
	}

	// Enrolled with a relay: still no relay row, and the note says where the
	// other machines come from.
	if err := f.SetRelay("wss://relay.example/relay/agent", "mac-mini", "secret", "", true); err != nil {
		t.Fatalf("SetRelay: %v", err)
	}
	tt.Frame()
	if _, ok := tt.Find("中继"); ok {
		t.Fatalf("an enrolled machine shows a relay row: %v", tt.Texts())
	}
	if tt.HasText("wss://relay.example/relay/agent") {
		t.Fatalf("the panel shows the relay's address as a row; texts = %v", tt.Texts())
	}
	if !tt.HasText("点另一台机器") {
		t.Fatalf("the note does not say what picking a machine does; texts = %v", tt.Texts())
	}
}

// Picking a machine this window cannot reach yet asks the relay first, because
// driving the rest of the fleet means asking it for a credential. The dialog is
// where a person sees that happen, and where they can say no.
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
	if !tt.HasText("接管 other box") {
		t.Fatalf("the dialog does not name the machine; texts = %v", tt.Texts())
	}
	if _, ok := tt.Find("申请并接管"); !ok {
		t.Fatalf("the dialog does not offer to ask the relay; texts = %v", tt.Texts())
	}
	if _, ok := tt.Find("取消"); !ok {
		t.Fatalf("the dialog cannot be declined; texts = %v", tt.Texts())
	}

	if err := tt.Click("取消"); err != nil {
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

func TestSidebarAppearanceSwitchesTheThemeAndSavesIt(t *testing.T) {
	w, dir := newTestShellWindow(t)
	// The appearance control is in the band across the top, which is the only
	// place the window itself can be changed from.
	tt := ui.NewTester(func(c *ui.Context) {
		applyTheme(c, w.store.Get().Theme)
		ui.Row(c).Fill().Children(func() { w.header(c) })
	}, 900, headerHeight)
	tt.SetDark(false) // the system is showing its light appearance

	light := tt.Image().RGBAAt(2, 2)
	if _, ok := tt.Find("切换到暗黑"); !ok {
		t.Fatalf("a light shell should offer the dark switch; it shows %v", tt.Texts())
	}
	if err := tt.Click("切换到暗黑"); err != nil {
		t.Fatalf("click the appearance entry: %v", err)
	}
	if got := w.store.Get().Theme; got != themeDark {
		t.Fatalf("appearance = %q, want %q", got, themeDark)
	}
	if got := settings.Load(dir).Theme; got != themeDark {
		t.Fatalf("appearance on disk = %q, want %q", got, themeDark)
	}

	// The next frame is drawn in the appearance that was just chosen, and it
	// offers the way back.
	tt.Frame()
	if dark := tt.Image().RGBAAt(2, 2); dark == light {
		t.Fatalf("the frame kept the old appearance: %v", dark)
	}
	if _, ok := tt.Find("切换到白天"); !ok {
		t.Fatalf("a dark shell should offer the light switch; it shows %v", tt.Texts())
	}
	if err := tt.Click("切换到白天"); err != nil {
		t.Fatalf("click the appearance entry: %v", err)
	}
	if got := w.store.Get().Theme; got != themeLight {
		t.Fatalf("appearance = %q, want %q", got, themeLight)
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

func TestAppearancePaneOffersAllThreeModes(t *testing.T) {
	f := newTestFleet(t)
	store := settings.Open(t.TempDir())
	// The desk is the window the choice is saved for: the pane goes through it,
	// the way it does in the shell.
	sw := newSettingsWindow(&window{store: store}, store, f)
	tt := ui.NewTester(func(c *ui.Context) { sw.view(c) }, 560, 460)
	if err := tt.Click("外观"); err != nil {
		t.Fatalf("open the appearance pane: %v", err)
	}
	for _, want := range []string{"跟随系统", "白天", "暗黑"} {
		if !tt.HasText(want) {
			t.Fatalf("the appearance pane does not offer %q; it shows %v", want, tt.Texts())
		}
	}
	if !tt.HasText("当前是「跟随系统」") {
		t.Fatalf("the pane does not say which appearance is on: %v", tt.Texts())
	}
	if err := tt.Click("暗黑"); err != nil {
		t.Fatalf("choose the dark appearance: %v", err)
	}
	if got := store.Get().Theme; got != themeDark {
		t.Fatalf("appearance = %q, want %q", got, themeDark)
	}
}

// The roster is keyed by device id and the window knows itself by hostname, so
// enrolling this machine with a relay used to put it in the panel twice — once
// as 本机, once as another machine with its own shells. A fleet panel that lists
// you twice is one nobody can read.
func TestMachinePanelDoesNotListThisMachineTwice(t *testing.T) {
	f := newTestFleet(t)
	w, _ := newTestShellWindow(t, "one")
	w.fleet = f
	if err := f.SetRelay("wss://relay.example/relay/agent", "mac-mini-7f3a", "secret", "", true); err != nil {
		t.Fatalf("SetRelay: %v", err)
	}

	// The relay's list, as the relay sees it: this machine under its enrolled id,
	// and another machine.
	w.roster = []tunnel.Machine{
		{ID: "mac-mini-7f3a", Name: f.Host().Hostname, Online: true, Host: f.Host()},
		{ID: "other-box", Name: "other box", Online: true,
			Host: protocol.Host{Platform: "darwin", Arch: "arm64"}},
	}

	tt := drawNav(t, w)
	if !tt.HasText("other box") {
		t.Fatalf("the panel does not list the other machine; texts = %v", tt.Texts())
	}
	// Its own name is on the 本机 row and nowhere else: a second row would be the
	// roster entry for this machine, which is what the device-id check skips.
	named := 0
	for _, text := range tt.Texts() {
		if strings.TrimSpace(text) == f.Host().Hostname {
			named++
		}
	}
	if named != 1 {
		t.Fatalf("this machine is named %d times in the panel, want once; texts = %v", named, tt.Texts())
	}
	// The title's count is this machine plus the other one — the self entry would
	// have made it three.
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

// A shell another machine is running that this window already has a tab for is
// *switched to* when its row is picked — that is what a list of tabs is for, and
// it used to do nothing at all: the click was swallowed and the pane stayed on
// whichever shell happened to be there.
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
