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
		w.tabs[i+1] = &tab{id: id, title: title, ordinal: i + 1}
		w.order = append(w.order, i+1)
	}
	return w, dir
}

// drawNav draws the window's navigation the way its view draws it: the strip of
// controls, and the column of shells beside it. The appearance comes from the
// store and the labels from the shells that are open.
func drawNav(t *testing.T, w *window) *ui.Tester {
	t.Helper()
	return ui.NewTester(func(c *ui.Context) {
		mode := w.store.Get().Theme
		applyTheme(c, mode)
		labels := make([]string, 0, len(w.order))
		for _, id := range w.order {
			if tb := w.tabs[id]; tb != nil {
				labels = append(labels, tb.label())
			}
		}
		// A row, as the window draws them: the strip beside the column, both as
		// tall as the window. Stacked, the strip's controls would land where no
		// window ever puts them.
		ui.Row(c).Fill().Children(func() {
			w.activityRail(c, mode)
			w.sidebar(c, labels, mode)
		})
	}, activityWidth+sidebarWidth, 420)
}

func TestSidebarListsTheOpenShellsAndPicksOne(t *testing.T) {
	w, _ := newTestShellWindow(t, "one", "two")
	tt := drawNav(t, w)

	for _, want := range []string{"one", "two"} {
		if !tt.HasText(want) {
			t.Fatalf("the column does not show %q; it shows %v", want, tt.Texts())
		}
	}
	if _, ok := tt.Find("设置"); !ok {
		t.Fatalf("the strip has no settings entry")
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

	// ...and an end to the one on screen.
	if err := tt.Click("关闭标签页"); err != nil {
		t.Fatalf("Click(关闭标签页): %v", err)
	}
	if got := waitSessions(f, 0); got != 0 {
		t.Fatalf("the button left %d shells, want 0", got)
	}
	settleTabs(t, w, 0)
}

// waitSessions waits for the hub to hold n sessions: the panel's controls act
// from their own goroutines, as the window's clicks do.
func waitSessions(f *fleet, n int) int {
	deadline := time.Now().Add(5 * time.Second)
	for {
		got := len(f.Hub().Sessions())
		if got == n || time.Now().After(deadline) {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// settleTabs waits until the window's tabs name the sessions the hub holds:
// what the links report is posted to the window's thread, and a test applies it
// by draining.
func settleTabs(t *testing.T, w *window, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		w.drain()
		named := 0
		for _, key := range w.order {
			if w.tabs[key].id != "" {
				named++
			}
		}
		if named == n && len(w.order) == n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("tabs did not settle to %d named sessions: order=%v", n, w.order)
}

// The machines' panel is the fleet's own view of itself: this machine, the relay
// it is enrolled with when there is one, and a line saying where the other
// machines will appear. A window with no relay says so rather than showing an
// empty panel.
func TestMachinePanelShowsThisMachineAndTheRelay(t *testing.T) {
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
	if !tt.HasText("未接入中继") {
		t.Fatalf("the panel does not say the relay is off; texts = %v", tt.Texts())
	}
	if _, ok := tt.Find("中继"); ok {
		t.Fatal("a machine with no relay shows a relay row")
	}

	// Enrolled with a relay: the row appears, with the address and the device id
	// the link was given, and the note changes with it. There is no client link
	// in this window — no token was ever applied — so the note has to say that
	// rather than promise machines that cannot appear.
	if err := f.SetRelay("wss://relay.example/relay/agent", "mac-mini", "secret", "", true); err != nil {
		t.Fatalf("SetRelay: %v", err)
	}
	tt.Frame()
	if !tt.HasText("wss://relay.example/relay/agent") {
		t.Fatalf("the relay row has no address; texts = %v", tt.Texts())
	}
	if !tt.HasText("设备 mac-mini") {
		t.Fatalf("the relay row has no device id; texts = %v", tt.Texts())
	}
	if !tt.HasText("没有客户端令牌") {
		t.Fatalf("the note did not own up to the missing client link; texts = %v", tt.Texts())
	}
	// ...and it has to say where the token goes, since that is the only place
	// this state can be undone.
	if !tt.HasText("设置 → 远程 → 客户端令牌") {
		t.Fatalf("the note does not say where the token goes; texts = %v", tt.Texts())
	}
}

func TestSidebarAppearanceSwitchesTheThemeAndSavesIt(t *testing.T) {
	w, dir := newTestShellWindow(t)
	tt := drawNav(t, w)
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
