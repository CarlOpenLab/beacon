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
	// The hub answers a release by handing the lease to the machine the shell
	// runs on, which is a role frame this window has to be given time to read.
	drainFor(t, w, func() bool { return !tb.held() })

	// The shell is untouched: still in the roster, still running.
	if got := liveCount(f); got != 1 {
		t.Fatalf("releasing the lease left %d live shells, want the shell still running", got)
	}
	// The tab is untouched too: it is where the shell's output is read.
	if len(w.order) != 1 {
		t.Fatalf("releasing the lease left %d tabs, want the one still watching", len(w.order))
	}
	// And this window no longer holds the lease — the hub handed it to the
	// machine the shell runs on, so the shell is not left mute.
	if tb.held() {
		t.Fatal("this window still holds the lease after releasing it")
	}
}

// 释放 is offered while the lease is held, and 接管 while it is not. Neither
// button ends the shell, which is what keeps them from being one control.
func TestTheLeaseButtonsAreOfferedByState(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	w.fleet = newTestFleet(t)
	tb := w.tabs[1]
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
	drainFor(t, w, func() bool { return liveCount(f) == 0 })

	if got := liveCount(f); got != 0 {
		t.Fatalf("结束 left %d live shells, want the process gone", got)
	}
	if len(w.order) != 1 {
		t.Fatalf("结束 left %d tabs, want the one holding the shell's last output", len(w.order))
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
