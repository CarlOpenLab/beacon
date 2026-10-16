package main

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/protocol"
	"github.com/CarlOpenLab/term-fleet/internal/tunnel"
)

// The glow is the one piece of chrome that says a shell is being driven from
// elsewhere, so it has to be on screen exactly when the lease says so — and
// absent otherwise, because a window that glows at its own shells is a window
// nobody can read.
func TestLeaseChromeFollowsTheLease(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	w.fleet = newTestFleet(t)

	// Held by this window: no glow, no popover, no mark on the tab.
	tb := w.tabs[1]
	tb.setLease(protocol.RoleController, "", 3)
	if tb.holder() != "" || !tb.held() {
		t.Fatalf("a controller should read as holding the lease: holder=%q held=%v", tb.holder(), tb.held())
	}

	// Held by someone else: the holder is named, and this window does not hold.
	tb.setLease(protocol.RoleViewer, "c9", 4)
	if got := tb.holder(); got != "c9" {
		t.Fatalf("holder = %q, want c9", got)
	}
	if tb.held() {
		t.Fatal("a viewer reads as holding the lease")
	}

	// The mark is drawn on the tab's row.
	tt := drawNav(t, w)
	if !tt.HasText("c9") {
		t.Fatalf("the tab row does not mark the holder; texts = %v", tt.Texts())
	}

	// Taking it back clears all of it.
	tb.setLease(protocol.RoleController, "", 5)
	if tb.holder() != "" {
		t.Fatalf("holder after reclaim = %q, want none", tb.holder())
	}
}

// The flow moves with the clock and stands still where the desktop asks for
// less motion: the same clock, two answers.
func TestGlowFlowMovesAndStandsStill(t *testing.T) {
	now := glowEpoch.Add(4 * time.Second)
	if got := glowFlow(now, false); got != 4 {
		t.Fatalf("four seconds of glow reads as %v, want 4", got)
	}
	if got := glowFlow(now, true); got != 0 {
		t.Fatalf("with less motion asked for, the flow is at %v, want 0", got)
	}
}

// A shell taken from this window says so at once — in the panel that names the
// holder and offers it back, not only in the glow. The person is in the middle
// of typing; a subtle edge is not an answer.
func TestLosingTheLeaseOpensThePanelAboutIt(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	w.fleet = newTestFleet(t)
	tb := w.tabs[1]
	sess := newSession(tb.id, nil, nil)
	tb.sess = sess
	h := tabHandlers{w: w, sess: sess}

	h.Role(protocol.RoleController, "", 3)
	w.drain()
	if w.leaseOpen {
		t.Fatal("holding the lease opened the panel")
	}

	h.Role(protocol.RoleViewer, "c9", 4)
	w.drain()
	if !w.leaseOpen {
		t.Fatal("a shell taken from this window did not say so")
	}

	// A shell this window never held — one it opened as a viewer, on another
	// machine — is not news: it gets the glow and the tab's mark, not a panel in
	// its face.
	w2, _ := newTestShellWindow(t, "two")
	tb2 := w2.tabs[1]
	sess2 := newSession(tb2.id, nil, nil)
	tb2.sess = sess2
	tabHandlers{w: w2, sess: sess2}.Role(protocol.RoleViewer, "c9", 1)
	w2.drain()
	if w2.leaseOpen {
		t.Error("a shell this window never held opened the panel anyway")
	}
}

// A refused keystroke is how a person finds out their typing is going nowhere,
// and the hub already answers it loudly. The window has to turn that answer into
// the panel that explains it — silence here is the bug the report was about.
func TestRefusedInputOpensThePanelThatExplainsIt(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	w.fleet = newTestFleet(t)
	tb := w.tabs[1]
	sess := newSession(tb.id, nil, nil)
	tb.sess = sess
	tb.setLease(protocol.RoleViewer, "c9", 4)
	h := tabHandlers{w: w, sess: sess}

	h.Failed(protocol.ErrNotController, "another client controls "+tb.id+"; claim it first")
	w.drain()
	if !w.leaseOpen {
		t.Fatal("a refused keystroke did not open the panel that explains it")
	}

	// An error that is not about the lease is a different thing, and must not put
	// the lease panel over the shell.
	w.leaseOpen = false
	h.Failed(protocol.ErrNoSession, "no live session named "+tb.id)
	w.drain()
	if w.leaseOpen {
		t.Error("an unrelated error opened the lease panel")
	}

	// A machine that refuses other clients' control has no holder to name, so its
	// refusal is not the popover either: it goes in the line under the machines,
	// which is where a refusal that belongs to a whole machine is read.
	h.Failed(protocol.ErrControlOff, "this machine does not allow other clients to control its shells")
	w.drain()
	if w.leaseOpen {
		t.Error("a control-off refusal opened the lease panel, which names a holder")
	}
	if !strings.Contains(w.linkNote, "不允许其他客户端控制") {
		t.Errorf("linkNote = %q, want it to say the machine refused", w.linkNote)
	}
}

// The way back has to be where the typing is: the header names the holder of the
// shell on screen and offers it back in one click, and the machines' panel says
// how many shells on this machine are somebody else's. Both go away with the
// lease, because chrome that outlives its reason is chrome nobody can trust.
func TestATakenShellIsMarkedInTheHeaderAndTheMachinePanel(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	w.fleet = newTestFleet(t)
	tb := w.tabs[1]
	tb.machine = machineLocal
	tb.setLease(protocol.RoleViewer, "c9", 4)
	w.live = []protocol.Summary{{SessionID: tb.id, Controller: "c9", Alive: true}}

	head := ui.NewTester(func(c *ui.Context) {
		applyTheme(c, w.store.Get().Theme)
		w.header(c, []string{"one"})
	}, 640, 52)
	if !head.HasText("c9 正在控制") {
		t.Fatalf("the header does not name the holder; texts = %v", head.Texts())
	}
	if _, ok := head.Find("拿回控制权"); !ok {
		t.Fatalf("the header does not offer the shell back; texts = %v", head.Texts())
	}

	nav := drawNav(t, w)
	if !nav.HasText("1 个会话正被其他客户端控制") {
		t.Fatalf("the machine panel does not say a shell is being driven; texts = %v", nav.Texts())
	}

	// The lease comes back. The session list may still be a push behind, so the
	// role frame this window holds is what decides: the shell is this window's,
	// and neither the header nor the panel has anything left to say.
	tb.setLease(protocol.RoleController, "", 5)
	head.Frame()
	if head.HasText("c9 正在控制") {
		t.Error("the header still names a holder after the lease came back")
	}
	if _, ok := head.Find("拿回控制权"); ok {
		t.Error("the header still offers back a shell this window holds")
	}
	nav.Frame()
	if nav.HasText("正被其他客户端控制") {
		t.Errorf("the machine panel still reports a driven shell; texts = %v", nav.Texts())
	}
}

// A machine that takes its shell back ends the connection: the tabs this window
// had open there close, the link is dropped, the panels return to this machine,
// and the window says who did it. Leaving a dead tab on screen would be a
// connection that cannot type pretending to be one that can.
func TestAMachineTakingItsShellBackEndsTheConnection(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	w.fleet = newTestFleet(t)
	tb := w.tabs[1]
	tb.machine = "other-box"
	sess := newSession(tb.id, nil, nil)
	tb.sess = sess
	w.remote = map[string]*relayLink{}
	w.remoteSessions = map[string][]protocol.Summary{
		"other-box": {{SessionID: tb.id, Alive: true}},
	}
	w.machineSel = "other-box"
	w.roster = []tunnel.Machine{{
		ID: "other-box", Name: "other box", Online: true,
		Host: protocol.Host{Platform: "linux", Arch: "amd64"},
	}}

	h := tabHandlers{w: w, sess: sess}
	h.Role(protocol.RoleController, "", 3)
	w.drain()
	if tb.holder() != "" {
		t.Fatal("this window did not hold the shell to begin with")
	}

	// The far machine's own window takes it back: the role frame says this
	// window is a viewer and nobody else holds it either.
	h.Role(protocol.RoleViewer, "", 4)
	w.drain()

	if len(w.tabs) != 0 {
		t.Fatalf("%d tabs left after the machine took its shell back, want none", len(w.tabs))
	}
	if _, ok := w.remote["other-box"]; ok {
		t.Error("the link to the machine that took its shell back is still open")
	}
	if _, ok := w.remoteSessions["other-box"]; ok {
		t.Error("the window still lists shells from a machine it is no longer connected to")
	}
	if got := w.selectedMachine(); got != machineLocal {
		t.Errorf("the panels are still on %q, want this machine", got)
	}
	// And it says who did it, on screen: a shell that vanishes without a word is
	// the thing this rule exists to avoid.
	if !strings.Contains(w.currentNotice(), "other box") {
		t.Fatalf("notice = %q, want it to name the machine", w.currentNotice())
	}
	tt := ui.NewTester(func(c *ui.Context) { w.view(c) }, 900, 600)
	tt.Frame()
	if !tt.HasText("拿回了控制权") {
		t.Fatalf("the notice is not on screen; texts = %v", tt.Texts())
	}
}

// The panel under the glow hangs from the top of a pane, and a pane can be
// narrower than a sentence: it has a width of its own so it cannot run past the
// terminal it is about.
func TestLeasePanelStaysInsideThePane(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	w.fleet = newTestFleet(t)
	tb := w.tabs[1]
	tb.term = fedTerminal(t, previewScreen())
	tb.setLease(protocol.RoleViewer, "c9", 4)
	w.leaseOpen = true

	tt := ui.NewTester(func(c *ui.Context) { w.view(c) }, 900, 600)
	tt.Frame()
	rect, ok := tt.Find("c9 正在控制这个终端")
	if !ok {
		t.Fatalf("the panel does not name the holder; texts = %v", tt.Texts())
	}
	if rect.W > leasePopoverWidth {
		t.Errorf("the panel's text is %v DIPs wide, want it inside %v", rect.W, leasePopoverWidth)
	}
	if _, ok := tt.Find("拿回控制权"); !ok {
		t.Fatalf("the panel lost the way back; texts = %v", tt.Texts())
	}
}
