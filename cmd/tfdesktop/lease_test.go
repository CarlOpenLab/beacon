package main

import (
	"image"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/protocol"
	"github.com/CarlOpenLab/term-fleet/internal/tunnel"
)

// The pane's 只读 chip and the tab's dot are the two pieces of chrome that say a
// shell is being driven from elsewhere, so they have to be on screen exactly when
// the lease says so — and absent otherwise, because a window that marks its own
// shells as somebody else's is a window nobody can read.
func TestLeaseChromeFollowsTheLease(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	w.fleet = newTestFleet(t)

	// Held by this window: no chip on the pane, no mark on the row.
	tb := w.tabs[1]
	tb.alive = true
	tb.setLease(protocol.RoleController, "", 3)
	if tb.holder() != "" || !tb.held() {
		t.Fatalf("a controller should read as holding the lease: holder=%q held=%v", tb.holder(), tb.held())
	}
	tb.term = fedTerminal(t, previewScreen())
	if tt := ui.NewTester(func(c *ui.Context) {
		applyTheme(c, w.store.Get().Theme)
		w.body(c)
	}, 900, 600); tt.HasText("只读") {
		t.Errorf("a shell this window holds is marked 只读; texts = %v", tt.Texts())
	}

	// Held by someone else: the holder is named, this window does not hold, and
	// the sheet says so in its own corner.
	tb.setLease(protocol.RoleViewer, "c9", 4)
	if got := tb.holder(); got != "c9" {
		t.Fatalf("holder = %q, want c9", got)
	}
	if tb.held() {
		t.Fatal("a viewer reads as holding the lease")
	}
	if tt := ui.NewTester(func(c *ui.Context) {
		applyTheme(c, w.store.Get().Theme)
		w.body(c)
	}, 900, 600); !tt.HasText("只读") {
		t.Errorf("the sheet does not mark itself 只读; texts = %v", tt.Texts())
	}

	// The mark is drawn on the tab's row, too.
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

// The light along the pane's edge is drawn under one condition and no other: a
// shell this machine runs, with another client on its keyboard. It is the one
// piece of chrome here that is not words — a person reads it from across the
// room, as "somebody has this machine's terminal" — so lighting up for a shell
// on a machine they are not sitting at would spend it on a fact they have no
// stake in, and lighting up for the ordinary case would spend it on nothing.
func TestTheGlowIsOnlyForThisMachinesTakenShells(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	w.fleet = newTestFleet(t)
	tb := w.tabs[1]

	for _, tc := range []struct {
		name       string
		machine    string
		role       string
		controller string
		alive      bool
		want       bool
	}{
		{"this machine's shell, held here", machineLocal, protocol.RoleController, "", true, false},
		{"this machine's shell, taken from here", machineLocal, protocol.RoleViewer, "c9", true, true},
		{"this machine's shell, ended while taken", machineLocal, protocol.RoleViewer, "c9", false, false},
		{"another machine's shell, taken there", "mac-studio", protocol.RoleViewer, "mac-studio", true, false},
		{"another machine's shell, held here", "mac-studio", protocol.RoleController, "", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tb.machine = tc.machine
			tb.alive = tc.alive
			tb.setLease(tc.role, tc.controller, 1)
			if got := w.drivenElsewhere(tb); got != tc.want {
				t.Errorf("drivenElsewhere = %v, want %v", got, tc.want)
			}
		})
	}
}

// And the condition reaches the picture: the same shell, held by the same client,
// is lit when it runs here and dark when it runs on another machine. This is read
// off the pixels because the light is drawn by a shader, which no label and no
// snapshot can see.
func TestOnlyThisMachinesTakenShellIsLit(t *testing.T) {
	for _, tc := range []struct {
		name    string
		machine string
		lit     bool
	}{
		{"this machine's shell, taken from it", machineLocal, true},
		{"another machine's shell, taken from it", "mac-studio", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, _ := newTestShellWindow(t, "one")
			w.fleet = newTestFleet(t)
			tb := w.tabs[1]
			tb.machine = tc.machine
			tb.alive = true
			tb.term = fedTerminal(t, previewScreen())
			tb.setLease(protocol.RoleViewer, "c9", 4)

			tt := ui.NewTester(func(c *ui.Context) {
				applyTheme(c, w.store.Get().Theme)
				w.body(c)
			}, 900, 600)
			tt.Frame()
			// The fade is the eye's; the state is a step. Run the clock past it so
			// the picture is of the state and not of the transition into it.
			tt.Advance(glowFade)
			tt.Frame()

			if got := paneEdgeLit(tt.Image()); got != tc.lit {
				t.Errorf("the pane's edge is lit = %v, want %v", got, tc.lit)
			}
		})
	}
}

// paneEdgeLit is whether a picture has light along the pane's left edge. The band
// is centred on that edge, so the lit pixel is the edge itself; it counts as lit
// when it is brighter than the paper around it and bluer than anything either the
// terminal's black or the window's grey can be, which is what the accent's light
// looks like as a number.
func paneEdgeLit(img *image.RGBA) bool {
	for _, x := range []int{paneInset - 2, paneInset - 1, paneInset, paneInset + 1, paneInset + 2} {
		r, _, _, _ := img.At(x, 300).RGBA()
		_, _, b, _ := img.At(x, 300).RGBA()
		if int(b>>8)-int(r>>8) > 24 && b>>8 > 40 {
			return true
		}
	}
	return false
}

// A shell that has ended is marked on the sheet as well, and with its own word:
// 只读 and 已退出 are different states and a person reading the pane has to be
// able to tell which one they are looking at.
func TestAnEndedShellIsMarkedOnTheSheet(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	w.fleet = newTestFleet(t)
	tb := w.tabs[1]
	tb.term = fedTerminal(t, previewScreen())
	tb.alive = false
	tb.finish(0)

	tt := ui.NewTester(func(c *ui.Context) {
		applyTheme(c, w.store.Get().Theme)
		w.body(c)
	}, 900, 600)
	if !tt.HasText("已退出") {
		t.Fatalf("an ended shell is not marked on the sheet; texts = %v", tt.Texts())
	}
}

// A shell taken from this window says so at once — in the panel that names the
// holder and offers it back, not only in a corner of the pane. The person is in
// the middle of typing; a subtle mark is not an answer.
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
	// machine — is not news: it gets the pane's mark and the tab's, not a panel in
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

// The way back has to be where the typing is: the band at the top names the
// holder of the shell on screen and offers it back in one click, and the
// machines' panel says how many shells on this machine are somebody else's. Both
// go away with the lease, because chrome that outlives its reason is chrome
// nobody can trust.
func TestATakenShellIsMarkedInTheHeaderAndTheMachinePanel(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	w.fleet = newTestFleet(t)
	tb := w.tabs[1]
	tb.machine = machineLocal
	tb.alive = true
	tb.setLease(protocol.RoleViewer, "c9", 4)
	w.live = []protocol.Summary{{SessionID: tb.id, Controller: "c9", Alive: true}}

	head := ui.NewTester(func(c *ui.Context) {
		applyTheme(c, w.store.Get().Theme)
		w.header(c)
	}, 720, headerHeight)
	if !head.HasText("只读：该会话正被 c9 控制") {
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
	if head.HasText("只读：该会话正被 c9 控制") {
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
func TestAMachineTakingItsShellBackKeepsTheWayBack(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	w.fleet = newTestFleet(t)
	tb := w.tabs[1]
	tb.machine = "other-box"
	tb.alive = true
	sess := newSession(tb.id, nil, nil)
	tb.sess = sess
	w.remote = map[string]*relayLink{}
	w.remoteSessions = map[string][]protocol.Summary{
		"other-box": {{SessionID: tb.id, Alive: true}},
	}
	// A link that exists and carries nothing, as one for a machine the window has
	// looked at but not routed a stream to yet does.
	stub := &relayLink{link: newLink(), machine: "other-box"}
	stub.setSend(func(protocol.Msg) bool { return true })
	w.remote["other-box"] = stub
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
	// window is a viewer and that machine is the one typing now.
	h.Role(protocol.RoleViewer, "other-box", 4)
	w.drain()

	// The window keeps watching the shell and keeps its link to the machine. A
	// shell taken back is a lease that moved hands, not a machine that went away:
	// the tab stays, the popover opens, and 接管 in it takes the shell back.
	//
	// This used to drop the relay stream and remove the tab instead, which left
	// the 接管 button with nothing to be drawn on — a client that reached a
	// machine's shell and lost it had no way back at all.
	if len(w.tabs) != 1 {
		t.Fatalf("%d tabs left after the machine took its shell back, want the one still watching it", len(w.tabs))
	}
	if _, ok := w.remote["other-box"]; !ok {
		t.Error("the link to the machine whose shell was taken back is gone")
	}
	if _, ok := w.remoteSessions["other-box"]; !ok {
		t.Error("the window forgot the shells of a machine it is still connected to")
	}
	if got := w.selectedMachine(); got != "other-box" {
		t.Errorf("the panels are on %q, want to stay on the machine being watched", got)
	}
	if !w.leaseOpen {
		t.Error("the popover naming the new holder did not open")
	}

	// And the way back is on screen: the header says who is typing, and offers
	// 接管 for a shell that was never this window's to begin with.
	tt := ui.NewTester(func(c *ui.Context) { w.view(c) }, 900, 600)
	tt.Frame()
	if !tt.HasText("接管") {
		t.Fatalf("no 接管 button on screen; texts = %v", tt.Texts())
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
