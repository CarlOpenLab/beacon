package main

import (
	"strings"
	"testing"
	"time"

	"github.com/CarlOpenLab/term-fleet/internal/protocol"
)

// The glow is the one piece of chrome that says a shell is being driven from
// elsewhere, so it has to be on screen exactly when the lease says so — and
// absent otherwise, because a window that glows at its own shells is a window
// nobody can read.
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
