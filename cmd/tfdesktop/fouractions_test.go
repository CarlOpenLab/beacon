package main

// The four things a client can do to a shell, and the rule that they are four.
//
//   接管       take the lease from whoever holds it
//   释放       give the lease up, leaving the shell running
//   结束       kill the process, for every client
//   关闭标签   stop watching, leaving the process alone
//
// They were not always four. The 会话 panel's × was wired to the same act as
// 结束, so closing a tab killed a process on a machine somewhere — for every
// client watching it, not only the window that clicked. And 释放 did not exist
// on either client at all: the frame was in the protocol and nothing sent it.
//
// These tests pin the separation down from the side that matters: what the
// buttons actually do to the shell.

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/protocol"
)

// A window that holds the lease is offered 释放, and pressing it gives the lease
// up without ending the shell: the process keeps running and the tab stays.
func TestReleasingTheLeaseLeavesTheShellRunning(t *testing.T) {
	f := newTestFleet(t)
	w, _ := newTestShellWindow(t)
	w.fleet = f
	w.local = openLocalLink(f.Hub())
	w.local.onSessions = w.sessionsArrived
	w.local.onGone = w.linkGone

	tb := w.openTab(machineLocal, "")
	w.local.create(80, 24, "", "")
	waitSessions(f, 1)
	settleTabs(t, w, 1)
	w.drain()

	if tb == nil {
		t.Fatal("no tab was opened")
	}
	// The lease arrives as a role frame, not with the attach.
	drainFor(t, w, func() bool { return tb.held() })
	if !tb.held() {
		t.Fatalf("this window should hold the lease of the shell it opened: holder=%q", tb.holder())
	}

	w.releaseLease(tb)
	settleTabs(t, w, 1)
	w.drain()
	// The hub answers a release with a role frame this window has to be given
	// time to read.
	drainFor(t, w, func() bool { return !tb.held() })

	// The shell is untouched: still in the roster, still running.
	if got := liveCount(f); got != 1 {
		t.Fatalf("releasing the lease left %d live shells, want the shell still running", got)
	}
	// The tab is untouched too: it is where the shell's output is read.
	if len(w.order) != 1 {
		t.Fatalf("releasing the lease left %d tabs, want the one still watching", len(w.order))
	}
	// And this window no longer holds the lease. The lease goes to nobody: the
	// hub hands a released lease to the machine the shell runs on, and this
	// window *is* that machine's window, so there is no one to hand it to. That
	// is the whole reason the button is not drawn for a shell of this machine's
	// (TestTheLeaseButtonsAreOfferedByState): the act works, and it leaves the
	// shell free for a client on the relay to take — which is a thing to ask
	// for on a shell you are driving somewhere else, not on your own terminal.
	if tb.held() {
		t.Fatal("this window still holds the lease after releasing it")
	}
}

// 释放 is offered while the lease is held on a shell this window is driving on
// another machine, and 接管 while it is not. Neither button ends the shell,
// which is what keeps them from being one control.
//
// A shell of this machine's is never offered 释放 — see the case at the end,
// which is the one a person meets first and the one that made no sense.
func TestTheLeaseButtonsAreOfferedByState(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	w.fleet = newTestFleet(t)
	tb := w.tabs[1]
	// The lease's controls are about a shell somebody else could be typing
	// into, so the shell in question is one on another machine: this window
	// reaches it over the relay and holds its lease until it lets go.
	tb.machine = "mac-studio"
	// A live shell: the header offers the lease's controls only while there is a
	// shell to hold, and a tab the test made has not been told it is running.
	tb.alive = true

	// The buttons are in the header, which drawNav does not draw: the panel it
	// renders is the sidebar, and the lease's own controls sit beside the shell's
	// name. So this draws the window.
	drawWindow := func() *ui.Tester {
		return ui.NewTester(func(c *ui.Context) {
			applyTheme(c, w.store.Get().Theme)
			w.view(c)
		}, 900, 600)
	}

	// Holding: the header offers 释放 and not 接管.
	tb.setLease(protocol.RoleController, "", 3)
	tt := drawWindow()
	tt.Frame()
	if !tt.HasText("释放控制权") {
		t.Fatalf("a window holding the lease is not offered 释放: %v", tt.Texts())
	}
	if tt.HasText("接管") || tt.HasText("拿回控制权") {
		t.Fatalf("a window holding the lease is offered a takeover it does not need: %v", tt.Texts())
	}

	// Not holding: the other way round.
	tb.setLease(protocol.RoleViewer, "c9", 4)
	tt = drawWindow()
	tt.Frame()
	if !tt.HasText("接管") && !tt.HasText("拿回控制权") {
		t.Fatalf("a window without the lease is not offered it back: %v", tt.Texts())
	}
	if tt.HasText("释放控制权") {
		t.Fatalf("a window without the lease is offered 释放: %v", tt.Texts())
	}

	// The same lease, held on this machine's own shell, is offered nothing: the
	// lease would be dropped and handed to nobody, so the one thing 释放 could
	// do here is stop this terminal taking the typing of the person sitting at
	// it — a button whose only effect is to break the thing it is drawn on.
	tb.machine = machineLocal
	tb.setLease(protocol.RoleController, "", 5)
	tt = drawWindow()
	tt.Frame()
	if tt.HasText("释放控制权") {
		t.Errorf("this machine's own shell is offered 释放: %v", tt.Texts())
	}
}

// The panel's × closes the tab and nothing else. This is the regression that
// mattered: it used to end the shell, because closing a tab and ending a session
// were the same button.
func TestClosingATabDoesNotEndTheShell(t *testing.T) {
	f := newTestFleet(t)
	w, _ := newTestShellWindow(t)
	w.fleet = f
	w.local = openLocalLink(f.Hub())
	w.local.onSessions = w.sessionsArrived
	w.local.onGone = w.linkGone

	tb := w.openTab(machineLocal, "")
	w.local.create(80, 24, "", "")
	waitSessions(f, 1)
	settleTabs(t, w, 1)
	w.drain()

	w.removeTab(tb)

	if got := liveCount(f); got != 1 {
		t.Fatalf("closing the tab left %d live shells, want the shell still running", got)
	}
	if len(w.order) != 0 {
		t.Fatalf("closing the tab left %d tabs, want none", len(w.order))
	}
}

// 结束 kills the process and leaves the tab: the tab is where the shell's last
// output is read, so ending a shell is not a reason to lose it.
func TestEndingAShellLeavesTheTabToReadIt(t *testing.T) {
	f := newTestFleet(t)
	w, _ := newTestShellWindow(t)
	w.fleet = f
	w.local = openLocalLink(f.Hub())
	w.local.onSessions = w.sessionsArrived
	w.local.onGone = w.linkGone

	tb := w.openTab(machineLocal, "")
	w.local.create(80, 24, "", "")
	waitSessions(f, 1)
	settleTabs(t, w, 1)
	w.drain()

	// The lease first: 结束 claims it when it is missing, and this test is about
	// the tab, not about the claim.
	drainFor(t, w, func() bool { return tb.held() })
	w.killSessionOn(machineLocal, tb.id)
	settleTabs(t, w, 1)
	// Both, and in one wait. The hub's own count drops the moment the process
	// exits, while the exit frame still has to travel to the window and be
	// drained; stopping at the count alone reads the tab before its word has
	// arrived, which is how this test passed on two platforms and failed on the
	// third.
	drainFor(t, w, func() bool { return liveCount(f) == 0 && !tb.alive })

	if got := liveCount(f); got != 0 {
		t.Fatalf("结束 left %d live shells, want the process gone", got)
	}
	if len(w.order) != 1 {
		t.Fatalf("结束 left %d tabs, want the one holding the shell's last output", len(w.order))
	}
	// And the tab is marked: the exit frame is the hub's word that the shell is
	// gone, and it is the only word there is. The session list never carries it —
	// the hub forgets a session as its process exits, so the list that follows
	// does not name the session at all. A tab waiting for a summary to say
	// alive=false waited forever, and while it waited the panel drew a live
	// shell's row over a shell that was gone: 结束 for a session the hub answers
	// no_session to, and no 关闭 to get the row off the screen.
	if tb.alive {
		t.Fatal("the tab is still marked alive after the shell it watches ended")
	}
}

// drainFor pumps the window until cond holds, or fails the test.
//
// The yield matters: the hub answers on its own goroutine, and a drain only runs
// what has already been posted. Without the sleep this spins 200 times inside one
// scheduler slice and gives up before the answer has been written.
func drainFor(t *testing.T, w *window, cond func() bool) {
	t.Helper()
	for i := 0; i < 500; i++ {
		w.drain()
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("the condition never held")
}
