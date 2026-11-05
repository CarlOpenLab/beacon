package main

// A create the hub refuses is answered about a session id this side has never
// seen: the hub names the shell before it exists, and the tab waiting for it is
// still unbound. Routing by session id alone dropped that answer, and the tab
// stayed at "正在创建终端…" forever with the reason only in the log.

import (
	"testing"

	"github.com/CarlOpenLab/beacon/internal/protocol"
	"github.com/CarlOpenLab/beacon/locales"
)

// createRecorder is the Handlers side of a link test: it records the answer a
// session gets instead of drawing one.
type createRecorder struct {
	code      string
	message   string
	failed    int
	createdID string
}

func (h *createRecorder) Attached(protocol.Summary, int64, bool) {}
func (h *createRecorder) Created(id string)                      { h.createdID = id }
func (h *createRecorder) Output([]byte, bool)                    {}
func (h *createRecorder) Role(string, string, uint64)            {}
func (h *createRecorder) Exit(int, bool)                         {}
func (h *createRecorder) Closed()                                {}
func (h *createRecorder) Gone(string)                            {}
func (h *createRecorder) Failed(code, message string) {
	h.failed++
	h.code, h.message = code, message
}

func TestARefusedCreateReachesTheWaitingTab(t *testing.T) {
	l := newLink()
	l.name = "local"
	h := &createRecorder{}
	l.watch(newSession("", l, h))

	l.dispatch(protocol.Msg{
		T:         protocol.STError,
		Code:      protocol.ErrBadRequest,
		Message:   "start pty: fork/exec /bin/zsh: no such file or directory",
		SessionID: "s13",
	})

	if h.failed != 1 {
		t.Fatalf("the waiting tab heard %d answers, want 1", h.failed)
	}
	if h.code != protocol.ErrBadRequest || h.message == "" {
		t.Fatalf("the waiting tab heard code=%q message=%q, want the hub's reason", h.code, h.message)
	}
	if len(l.pending) != 0 {
		t.Fatalf("%d sessions are still waiting after the create was answered", len(l.pending))
	}
}

// An error about a session this link no longer watches is not a create answer:
// a tab closed a moment ago can still be named by a late frame, and closing the
// tab that was just opened because of it would be a worse bug than the one this
// routing exists for.
func TestAnUnrelatedErrorDoesNotConsumeTheWaitingTab(t *testing.T) {
	l := newLink()
	l.name = "local"
	h := &createRecorder{}
	l.watch(newSession("", l, h))

	l.dispatch(protocol.Msg{
		T:         protocol.STError,
		Code:      protocol.ErrNoSession,
		Message:   "no live session named s-old",
		SessionID: "s-old",
	})

	if h.failed != 0 {
		t.Fatal("a late no_session error was read as the answer to a create")
	}
	if len(l.pending) != 1 {
		t.Fatalf("the waiting tab was taken by an unrelated error: %d pending", len(l.pending))
	}
}

// A tab closed while its create is still in flight must not stay in the
// pending list. It has no session id, so nothing else cleans it up, and the
// created answer adopts the oldest waiting session: one closed tab there took
// every later create's answer, so each new tab stayed at "正在创建终端…" while
// a tab the person had already closed was given the shell. That is the state
// the window reached after one refused create, and it made every create after
// it hang.
func TestAClosedWaitingTabDoesNotTakeTheNextCreate(t *testing.T) {
	l := newLink()
	l.name = "local"
	closed := &createRecorder{}
	old := newSession("", l, closed)
	l.watch(old)
	l.unwatchSession(old) // the tab was closed before the hub answered

	waiting := &createRecorder{}
	s := newSession("", l, waiting)
	l.watch(s)

	l.dispatch(protocol.Msg{T: protocol.STCreated, SessionID: "s14"})

	if closed.createdID != "" {
		t.Fatalf("a closed tab was given the new shell %q", closed.createdID)
	}
	if waiting.createdID != "s14" {
		t.Fatalf("the waiting tab heard %q, want s14", waiting.createdID)
	}
	if len(l.pending) != 0 {
		t.Fatalf("%d sessions still waiting after the create was answered", len(l.pending))
	}
}

// The window's own behaviour: the tab goes, and the reason is on screen rather
// than in a pane that would otherwise say "正在创建终端…" forever.
func TestACreateFailureClosesThePendingTabAndSaysWhy(t *testing.T) {
	f := newTestFleet(t)
	w, _ := newTestShellWindow(t)
	w.fleet = f
	w.local = openLocalLink(f.Hub())
	w.local.onSessions = w.sessionsArrived
	w.local.onGone = w.linkGone
	t.Cleanup(func() { w.local.close("test done") })

	tb := w.openTab(machineLocal, "")
	if tb == nil {
		t.Fatal("no tab was opened")
	}
	reason := "start pty: fork/exec /bin/zsh: no such file or directory"
	w.local.link.dispatch(protocol.Msg{
		T:         protocol.STError,
		Code:      protocol.ErrBadRequest,
		Message:   reason,
		SessionID: "s13",
	})

	drainFor(t, w, func() bool { return len(w.order) == 0 })
	if got, want := w.currentNotice(), locales.AppCreateFailed(reason); got != want {
		t.Fatalf("notice = %q, want %q", got, want)
	}
}
