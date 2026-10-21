package main

import (
	"image"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/protocol"
	"github.com/CarlOpenLab/term-fleet/internal/tunnel"
	"github.com/CarlOpenLab/term-fleet/locales"
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
		w.body(c, paletteFor(c))
	}, 900, 600); tt.HasText(locales.StateReadonly()) {
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
		w.body(c, paletteFor(c))
	}, 900, 600); !tt.HasText(locales.StateReadonly()) {
		t.Errorf("the sheet does not mark itself 只读; texts = %v", tt.Texts())
	}

	// The mark is drawn on the tab's row, too.
	tt := drawNav(t, w)
	if !tt.HasText(locales.StateReadonly()) || !strings.Contains(w.sessionTooltip(w.sessionEntries()[0]), "c9") {
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
				w.body(c, paletteFor(c))
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
// when it is brighter and warmer than the terminal's neutral ground.
func paneEdgeLit(img *image.RGBA) bool {
	for _, x := range []int{paneInset - 2, paneInset - 1, paneInset, paneInset + 1, paneInset + 2} {
		r, _, b, _ := img.At(x, 300).RGBA()
		if int(r>>8)-int(b>>8) > 15 && r>>8 > 40 {
			return true
		}
	}
	return false
}

// A shell that has ended is marked on the sheet as well, and with its own word:
// 只读 and 已退出 are different states and a person reading the pane has to be
// able to tell which one they are looking at. The pane on screen says it on the
// strip under its header, so the mark in a pane's own header is read on another
// pane of a split.
func TestAnEndedShellIsMarkedOnTheSheet(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	w.fleet = newTestFleet(t)
	ended := w.tabs[1]
	ended.term = fedTerminal(t, previewScreen())
	ended.alive = false
	ended.finish(0)

	// A live second pane puts the ended one off screen.
	live := &tab{id: "s2", machine: machineLocal, title: "two", ordinal: 2,
		term: fedTerminal(t, previewScreen()), alive: true}
	w.tabs[2] = live
	w.order = append(w.order, 2)
	w.split = &splitNode{
		side: true, pct: splitHalf,
		a: &splitNode{tab: ended}, b: &splitNode{tab: live},
	}
	w.focus = w.split.b
	w.selectTabRaw(live)

	tt := ui.NewTester(func(c *ui.Context) {
		applyTheme(c, w.store.Get().Theme)
		w.body(c, paletteFor(c))
	}, 900, 600)
	if !tt.HasText(locales.StateEnded()) {
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
	if !strings.Contains(w.linkNote, locales.AppControlOff()) {
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

	// The card's header is where the window says whose shell it is now: the
	// banner chip names the holder and offers the way back.
	tb.term = fedTerminal(t, previewScreen())
	head := ui.NewTester(w.view, 720, 460)
	head.Frame()
	if !head.HasText(locales.BannerReadonlyHolder("c9")) {
		t.Fatalf("the card's header does not name the holder; texts = %v", head.Texts())
	}
	if _, ok := head.Find(locales.ActionTakeBack()); !ok {
		t.Fatalf("the card's header does not offer the shell back; texts = %v", head.Texts())
	}

	nav := drawNav(t, w)
	if !nav.HasText(locales.StateReadonly()) {
		t.Fatalf("the machine panel does not say a shell is being driven; texts = %v", nav.Texts())
	}

	// The lease comes back. The session list may still be a push behind, so the
	// role frame this window holds is what decides: the shell is this window's,
	// and neither the header nor the panel has anything left to say.
	tb.setLease(protocol.RoleController, "", 5)
	head.Frame()
	if head.HasText(locales.BannerReadonlyHolder("c9")) {
		t.Error("the header still names a holder after the lease came back")
	}
	if _, ok := head.Find(locales.ActionTakeBack()); ok {
		t.Error("the header still offers back a shell this window holds")
	}
	nav.Frame()
	if nav.HasText(locales.StateReadonly()) {
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
	// The card's header carries the banner and the lease's panel, so the tab
	// needs the terminal the card is drawn around.
	tb.term = fedTerminal(t, previewScreen())
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
	if !tt.HasText(locales.ActionTakeOver()) {
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
	tb.alive = true
	tb.setLease(protocol.RoleViewer, "c9", 4)
	w.leaseOpen = true

	tt := ui.NewTester(func(c *ui.Context) { w.view(c) }, 900, 600)
	tt.Frame()
	rect, ok := tt.Find(locales.LeaseWho("c9"))
	if !ok {
		t.Fatalf("the panel does not name the holder; texts = %v", tt.Texts())
	}
	if rect.W > leasePopoverWidth {
		t.Errorf("the panel's text is %v DIPs wide, want it inside %v", rect.W, leasePopoverWidth)
	}
	if _, ok := tt.Find(locales.ActionTakeBack()); !ok {
		t.Fatalf("the panel lost the way back; texts = %v", tt.Texts())
	}
}

// The panel sits at the bottom center of the pane it is about, not in its
// top-left corner: it explains the shell under the terminal, and its way out
// belongs within reach of the last line a person reads.
func TestTheLeasePanelSitsAtTheBottomCenter(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	w.fleet = newTestFleet(t)
	tb := w.tabs[1]
	tb.term = fedTerminal(t, previewScreen())
	tb.alive = true
	tb.setLease(protocol.RoleViewer, "c9", 4)
	w.leaseOpen = true

	tt := ui.NewTester(func(c *ui.Context) { w.view(c) }, 900, 600)
	tt.Frame()
	panel, ok := tt.Find("lease-panel")
	if !ok {
		t.Fatalf("the lease panel is not on screen; texts = %v", tt.Texts())
	}
	// The card starts after the 268 column and ends at the window's 8px inset,
	// so its center is 268 + (900-268-8)/2.
	if center := panel.X + panel.W/2; center < 578 || center > 582 {
		t.Errorf("the panel is centered at x=%v, want the pane's center 580", center)
	}
	if bottom := panel.Y + panel.H; bottom < 570 || bottom > 578 {
		t.Errorf("the panel's bottom is at y=%v, want it above the pane's foot (574)", bottom)
	}
}

// The panel has a state for a holder that is nobody. It opens on the frame that
// takes a shell away from this window, and that frame can hand the shell to no
// one: a shell on another machine whose lease was let go while the machine that
// owns it was not watching is free. Built from a name the window does not have,
// the panel put a blank where the name goes — " 正在控制这个终端" — and promised
// that " 会变成只读". The offer stands all the same: a claim on a free lease is
// honoured at once, and there is nobody to notify or to steal from.
func TestTheLeasePanelNamesNobodyWhenNobodyHoldsTheShell(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	w.fleet = newTestFleet(t)
	tb := w.tabs[1]
	tb.term = fedTerminal(t, previewScreen())
	tb.machine = "mac-studio"
	tb.alive = true
	// A viewer, with no controller: the window is looking at a shell nobody holds.
	tb.setLease(protocol.RoleViewer, "", 2)
	w.leaseOpen = true

	tt := ui.NewTester(func(c *ui.Context) { w.view(c) }, 900, 600)
	tt.Frame()
	if !tt.HasText(locales.LeaseWhoNobody()) {
		t.Fatalf("the panel does not say the shell is nobody's; texts = %v", tt.Texts())
	}
	if !tt.HasText(locales.LeaseOutcomeFree()) {
		t.Fatalf("the panel does not say what pressing the button does; texts = %v", tt.Texts())
	}
	for _, text := range tt.Texts() {
		if strings.HasPrefix(text, " ") || text == locales.LeaseOutcomeOver("") {
			t.Errorf("the panel is about a holder with no name: %q", text)
		}
	}
	// The way in is still on screen: a shell nobody holds is a shell this window
	// can take without taking it from anyone.
	if _, ok := tt.Find(locales.ActionTakeOver()); !ok {
		t.Fatalf("the panel lost the way in; texts = %v", tt.Texts())
	}
}

// The panel can be dismissed and the state cannot: a shell nobody drives takes
// no typing, so the band and the sheet have to carry it too. A person who closed
// the panel, or who never opened it — the shell came back this way — still has
// to be able to read why their keys are going nowhere, and to have the way in
// within reach.
func TestAFreeShellIsMarkedReadOnlyAndOffersTheWayIn(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	w.fleet = newTestFleet(t)
	tb := w.tabs[1]
	tb.term = fedTerminal(t, previewScreen())
	tb.machine = "mac-studio"
	tb.alive = true
	tb.setLease(protocol.RoleViewer, "", 2)

	tt := ui.NewTester(func(c *ui.Context) { w.view(c) }, 900, 600)
	tt.Frame()
	if !tt.HasText(locales.BannerReadonlyNobody()) {
		t.Fatalf("the band does not say the shell is nobody's; texts = %v", tt.Texts())
	}
	if _, ok := tt.Find(locales.ActionTakeOver()); !ok {
		t.Fatalf("the band does not offer the way in; texts = %v", tt.Texts())
	}

	// The sheet says it as well, in the corner the browser client uses.
	sheet := ui.NewTester(func(c *ui.Context) {
		applyTheme(c, w.store.Get().Theme)
		w.body(c, paletteFor(c))
	}, 900, 600)
	if !sheet.HasText(locales.StateReadonly()) {
		t.Fatalf("the sheet is not marked 只读; texts = %v", sheet.Texts())
	}

	// And a shell this window holds has none of it: the mark is about the lease,
	// not about the window having opinions.
	tb.setLease(protocol.RoleController, "", 3)
	tt.Frame()
	if tt.HasText(locales.StateReadonly()) {
		t.Errorf("a shell this window holds is marked 只读; texts = %v", tt.Texts())
	}
	sheet.Frame()
	if sheet.HasText(locales.StateReadonly()) {
		t.Errorf("a shell this window holds is marked on the sheet; texts = %v", sheet.Texts())
	}
}

// A shell whose lease the window has not heard about yet is not a shell somebody
// else holds. The word arrives in a role frame, so between opening a shell and
// the hub's answer the window knows nothing — and a window that read that
// silence as 只读 would flash the mark and the band over every shell a person
// opens.
func TestAShellWhoseLeaseIsNotKnownYetIsNotMarked(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	w.fleet = newTestFleet(t)
	tb := w.tabs[1]
	tb.term = fedTerminal(t, previewScreen())
	tb.alive = true

	if tb.known() {
		t.Fatal("a fresh tab claims to know its lease")
	}
	tt := ui.NewTester(func(c *ui.Context) { w.view(c) }, 900, 600)
	tt.Frame()
	if tt.HasText(locales.StateReadonly()) {
		t.Errorf("a shell with no role frame yet is marked 只读; texts = %v", tt.Texts())
	}
	if tt.HasText(locales.BannerReadonlyNobody()) || tt.HasText(locales.LeaseWhoNobody()) {
		t.Errorf("a shell with no role frame yet names nobody as its holder; texts = %v", tt.Texts())
	}
}

// When a terminal view resizes in read-only mode, it records its dimensions.
// Once the lease is acquired (controller role), syncSize propagates the actual
// dimensions to the hub so remote PTY size matches client window size.
func TestTermConnResizeSyncOnClaim(t *testing.T) {
	f := newTestFleet(t)
	l := openLocalLink(f.Hub())
	sess := newSession("test-s1", l.link, nil)
	conn := newTermConn(sess)

	// In read-only / viewer mode, Resize records dimensions without sending.
	if err := conn.Resize(120, 36); err != nil {
		t.Fatalf("Resize failed: %v", err)
	}
	if conn.lastCols != 120 || conn.lastRows != 36 {
		t.Fatalf("lastCols=%d lastRows=%d, want 120x36", conn.lastCols, conn.lastRows)
	}

	// Now promote to controller and sync size.
	sess.setRole(protocol.RoleController, "", 1)
	if !sess.holds() {
		t.Fatal("session should hold lease")
	}

	// syncSize should now propagate the saved dimensions without error.
	conn.syncSize(0, 0)
}
