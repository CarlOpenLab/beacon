//go:build !windows

package hub_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/CarlOpenLab/term-fleet/internal/hub"
	"github.com/CarlOpenLab/term-fleet/internal/protocol"
)

// These are M0's acceptance tests from PLAN.md: create -> attach -> input ->
// output, plus the two behaviours that only exist because more than one client
// can watch the same PTY.

func newTestHub(t *testing.T) *httptest.Server {
	t.Helper()
	h := hub.New(hub.Options{
		Host: protocol.Host{Hostname: "test", Platform: "test", Arch: "test"},
	})
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return ts
}

type conn struct {
	t    *testing.T
	conn *websocket.Conn

	// seen is everything this client has been sent as output, accumulated
	// across waits, so a test can assert that a frame did NOT land.
	seen strings.Builder
}

// saw is the output this client has been sent so far.
func (c *conn) saw() string { return c.seen.String() }

func dialTest(t *testing.T, ts *httptest.Server) *conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/"
	wsc, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = wsc.Close(websocket.StatusNormalClosure, "test done") })

	c := &conn{t: t, conn: wsc}
	c.send(protocol.Msg{T: protocol.CTHello, V: protocol.Version})
	c.waitFor("hello", func(m protocol.Msg) bool { return m.T == protocol.STHello })
	return c
}

func (c *conn) send(m protocol.Msg) {
	c.t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		c.t.Fatalf("marshal: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.conn.Write(ctx, websocket.MessageText, b); err != nil {
		c.t.Fatalf("write: %v", err)
	}
}

func (c *conn) recv() protocol.Msg {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, b, err := c.conn.Read(ctx)
	if err != nil {
		c.t.Fatalf("read: %v", err)
	}
	var m protocol.Msg
	if err := json.Unmarshal(b, &m); err != nil {
		c.t.Fatalf("unmarshal: %v", err)
	}
	return m
}

// waitFor skips the session-list broadcasts that arrive between the frames a
// test actually cares about.
func (c *conn) waitFor(what string, pred func(protocol.Msg) bool) protocol.Msg {
	c.t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if m := c.recv(); pred(m) {
			return m
		}
	}
	c.t.Fatalf("timed out waiting for %s", what)
	return protocol.Msg{}
}

// waitOutput accumulates session output until needle shows up, and returns the
// stream cursor just past the frame that contained it.
func (c *conn) waitOutput(needle string) int64 {
	c.t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	var got strings.Builder
	for time.Now().Before(deadline) {
		m := c.recv()
		if m.T == protocol.STOutput {
			got.Write(m.Data)
			c.seen.Write(m.Data)
			if strings.Contains(got.String(), needle) {
				return m.Cursor
			}
		}
	}
	c.t.Fatalf("timed out waiting for %q in output; got %q", needle, got.String())
	return 0
}

func (c *conn) create(id string) protocol.Msg {
	c.t.Helper()
	c.send(protocol.Msg{
		T: protocol.CTCreate, SessionID: id,
		Cols: 80, Rows: 24, Shell: "/bin/sh",
	})
	return c.waitFor("created", func(m protocol.Msg) bool {
		return m.T == protocol.STCreated && m.SessionID == id
	})
}

func TestCreateInputOutput(t *testing.T) {
	ts := newTestHub(t)
	a := dialTest(t, ts)

	a.send(protocol.Msg{T: protocol.CTList, ReqID: "r1"})
	list := a.waitFor("sessions", func(m protocol.Msg) bool {
		return m.T == protocol.STSessions && m.ReqID == "r1"
	})
	if len(list.Sessions) != 0 {
		t.Fatalf("fresh daemon reported %d sessions, want 0", len(list.Sessions))
	}

	a.create("s1")

	att := a.waitFor("attached", func(m protocol.Msg) bool { return m.T == protocol.STAttached })
	if att.Session == nil {
		t.Fatal("attached carried no session summary")
	}
	if !att.Session.Alive {
		t.Fatal("attached session is not alive")
	}
	if att.Session.Cols != 80 || att.Session.Rows != 24 {
		t.Fatalf("attached size is %dx%d, want 80x24", att.Session.Cols, att.Session.Rows)
	}

	role := a.waitFor("role", func(m protocol.Msg) bool { return m.T == protocol.STRole })
	if role.Role != protocol.RoleController {
		t.Fatalf("the creating client should control the session, got %q", role.Role)
	}

	a.send(protocol.Msg{
		T: protocol.CTInput, SessionID: "s1",
		Data: []byte("printf 'RES_%d\\n' 42\n"),
	})
	a.waitOutput("RES_42")
}

func TestViewerCannotInputAndClaimPreempts(t *testing.T) {
	ts := newTestHub(t)
	a := dialTest(t, ts)
	a.create("s1")
	a.waitFor("role", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleController
	})

	b := dialTest(t, ts)
	b.send(protocol.Msg{T: protocol.CTAttach, SessionID: "s1", Cursor: 0})
	att := b.waitFor("attached", func(m protocol.Msg) bool { return m.T == protocol.STAttached })
	if att.Session == nil {
		t.Fatal("attached carried no session summary")
	}
	if att.Session.Controller == "" {
		t.Fatal("the session should report who controls it")
	}
	role := b.waitFor("role", func(m protocol.Msg) bool { return m.T == protocol.STRole })
	if role.Role != protocol.RoleViewer {
		t.Fatalf("a second client should be a viewer, got %q", role.Role)
	}

	// A viewer's keystrokes are refused, not silently dropped.
	b.send(protocol.Msg{T: protocol.CTInput, SessionID: "s1", Data: []byte("echo NOPE\n")})
	e := b.waitFor("not_controller", func(m protocol.Msg) bool { return m.T == protocol.STError })
	if e.Code != protocol.ErrNotController {
		t.Fatalf("viewer input error code is %q, want %q", e.Code, protocol.ErrNotController)
	}

	// A plain claim against a holder whose lease is live is refused, naming the
	// holder: taking a shell from under someone is asked for, not assumed.
	b.send(protocol.Msg{T: protocol.CTClaim, SessionID: "s1"})
	held := b.waitFor("lease held", func(m protocol.Msg) bool { return m.T == protocol.STError })
	if held.Code != protocol.ErrLeaseHeld {
		t.Fatalf("claim against a live lease got %q, want %q", held.Code, protocol.ErrLeaseHeld)
	}
	if held.Message == "" {
		t.Fatal("the refusal does not name the holder")
	}

	// Stealing takes control away from A, and A is told.
	b.send(protocol.Msg{T: protocol.CTClaim, SessionID: "s1", Steal: true})
	b.waitFor("role controller", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleController
	})
	a.waitFor("demoted to viewer", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleViewer
	})

	// A's input now carries an epoch that is past: it is dropped rather than
	// answered, because the role frame already told A it lost the shell. A
	// frame at the current epoch from B lands.
	epoch := b.waitFor("role with epoch", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleController && m.Epoch > 0
	}).Epoch
	a.send(protocol.Msg{
		T: protocol.CTInput, SessionID: "s1", Epoch: epoch - 1,
		Data: []byte("printf 'FROM_A\\n' 1\n"),
	})
	b.send(protocol.Msg{
		T: protocol.CTInput, SessionID: "s1", Epoch: epoch,
		Data: []byte("printf 'FROM_B\\n' 1\n"),
	})
	b.waitOutput("FROM_B")
	// Nothing from A's stale frame may have landed: it would be on the screen.
	if strings.Contains(b.saw(), "FROM_A") {
		t.Fatal("input from the demoted holder landed after the handoff")
	}
}

// printfMarker prints want without the typed command containing it. The PTY
// echoes what we send, and that echo arrives in arbitrarily sized chunks, so a
// needle that also appears in the command can be matched out of a half-written
// echo line: the cursor then points into the echo rather than past the output.
// Splitting the quoted word leaves the command text (printf 'AA'A\n') free of
// what it prints (AAA).
func printfMarker(want string) string {
	return fmt.Sprintf("printf '%s''%s\\n'\n", want[:len(want)-1], want[len(want)-1:])
}

func TestAttachResumesFromCursor(t *testing.T) {
	ts := newTestHub(t)
	a := dialTest(t, ts)
	a.create("s1")
	a.waitFor("role", func(m protocol.Msg) bool { return m.T == protocol.STRole })

	a.send(protocol.Msg{T: protocol.CTInput, SessionID: "s1", Data: []byte(printfMarker("AAA"))})
	cursor := a.waitOutput("AAA")

	// More output happens while the second client is not watching yet.
	a.send(protocol.Msg{T: protocol.CTInput, SessionID: "s1", Data: []byte(printfMarker("BBB"))})
	a.waitOutput("BBB")

	b := dialTest(t, ts)
	b.send(protocol.Msg{T: protocol.CTAttach, SessionID: "s1", Cursor: cursor})

	var got strings.Builder
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(got.String(), "BBB") {
		if m := b.recv(); m.T == protocol.STOutput {
			got.Write(m.Data)
		}
	}

	if !strings.Contains(got.String(), "BBB") {
		t.Fatalf("resume did not deliver the output produced after the cursor: %q", got.String())
	}
	if strings.Contains(got.String(), "AAA") {
		t.Fatalf("resume replayed history from before the cursor: %q", got.String())
	}
}

func TestCloseRequiresControl(t *testing.T) {
	ts := newTestHub(t)
	a := dialTest(t, ts)
	a.create("s1")
	a.waitFor("role", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleController
	})

	b := dialTest(t, ts)
	b.send(protocol.Msg{T: protocol.CTAttach, SessionID: "s1", Cursor: 0})
	b.waitFor("viewer role", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleViewer
	})

	// A viewer may stop watching, but may not kill everyone's session. Claiming
	// first is what the client does when its 结束 button is pressed.
	b.send(protocol.Msg{T: protocol.CTClose, SessionID: "s1"})
	e := b.waitFor("close refused", func(m protocol.Msg) bool { return m.T == protocol.STError })
	if e.Code != protocol.ErrNotController {
		t.Fatalf("viewer close error code is %q, want %q", e.Code, protocol.ErrNotController)
	}

	a.send(protocol.Msg{T: protocol.CTClose, SessionID: "s1"})
	a.waitFor("exit", func(m protocol.Msg) bool { return m.T == protocol.STExit })
}

// Ending a session is not a reason to lose the tab that was showing it: the
// client that asked gets the same exit frame as everyone else, so its final
// output stays on screen. A close used to answer the killer alone with
// STClosed, which the browser client read as "this tab is done" — so the one
// who ended a shell was the only one who could not read its last line.
func TestCloseTellsTheKillerTheSameExit(t *testing.T) {
	ts := newTestHub(t)
	a := dialTest(t, ts)
	a.create("s1")
	a.waitFor("role", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleController
	})
	b := dialTest(t, ts)
	b.send(protocol.Msg{T: protocol.CTAttach, SessionID: "s1", Cursor: 0})
	b.waitFor("viewer role", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleViewer
	})

	a.send(protocol.Msg{T: protocol.CTClose, SessionID: "s1"})

	// The killer hears the exit, not a private acknowledgement.
	m := a.waitFor("exit", func(msg protocol.Msg) bool { return msg.T == protocol.STExit })
	if m.SessionID != "s1" {
		t.Fatalf("exit is for %q, want s1", m.SessionID)
	}
	// And so does the watcher.
	b.waitFor("exit", func(msg protocol.Msg) bool { return msg.T == protocol.STExit })
}

// A client that only watches may still end the session it is looking at, by
// claiming first: the 结束 button on a row is not a button that silently fails.
func TestWatcherMayEndByClaimingFirst(t *testing.T) {
	ts := newTestHub(t)
	a := dialTest(t, ts)
	a.create("s1")
	a.waitFor("role", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleController
	})

	b := dialTest(t, ts)
	b.send(protocol.Msg{T: protocol.CTAttach, SessionID: "s1", Cursor: 0})
	b.waitFor("viewer role", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleViewer
	})

	// Claim with steal, then close: the two frames a 结束 press sends.
	b.send(protocol.Msg{T: protocol.CTClaim, SessionID: "s1", Steal: true})
	b.waitFor("controller role", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleController
	})
	b.send(protocol.Msg{T: protocol.CTClose, SessionID: "s1"})
	b.waitFor("exit", func(m protocol.Msg) bool { return m.T == protocol.STExit })

	// The former controller is told the shell is gone too, not just that it lost
	// the lease — otherwise its tab would sit there claiming a live shell.
	a.waitFor("exit", func(m protocol.Msg) bool { return m.T == protocol.STExit })
}

func TestResizeRejectsZero(t *testing.T) {
	ts := newTestHub(t)
	a := dialTest(t, ts)
	a.create("s1")
	a.waitFor("role", func(m protocol.Msg) bool { return m.T == protocol.STRole })

	a.send(protocol.Msg{T: protocol.CTResize, SessionID: "s1", Cols: 0, Rows: 0})
	e := a.waitFor("bad_size", func(m protocol.Msg) bool { return m.T == protocol.STError })
	if e.Code != protocol.ErrBadSize {
		t.Fatalf("zero resize error code is %q, want %q", e.Code, protocol.ErrBadSize)
	}

	a.send(protocol.Msg{T: protocol.CTResize, SessionID: "s1", Cols: 100, Rows: 30})
	a.send(protocol.Msg{T: protocol.CTList, ReqID: "after-resize"})
	list := a.waitFor("resized list", func(m protocol.Msg) bool {
		return m.T == protocol.STSessions && m.ReqID == "after-resize"
	})
	if len(list.Sessions) != 1 {
		t.Fatalf("want 1 session, got %d", len(list.Sessions))
	}
	if got := list.Sessions[0]; got.Cols != 100 || got.Rows != 30 {
		t.Fatalf("reported size is %dx%d, want 100x30", got.Cols, got.Rows)
	}
}

// ---- the lease ------------------------------------------------------------

// newLeaseHub is a hub whose lease lapses in idle, so expiry can be observed
// without waiting out the real timeout.
func newLeaseHub(t *testing.T, idle time.Duration) *hub.Hub {
	t.Helper()
	return hub.New(hub.Options{
		Host:        protocol.Host{Hostname: "test", Platform: "test", Arch: "test"},
		IdleTimeout: idle,
	})
}

// serveHub puts a hub behind a test server, as newTestHub does with the default
// options.
func serveHub(t *testing.T, h *hub.Hub) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return ts
}

// pipeConn drives the window's end of an in-process pipe with the same helpers
// a socket client has.
type pipeConn struct {
	t    *testing.T
	pipe *hub.Pipe
	seen strings.Builder
}

func newPipeConn(t *testing.T, p *hub.Pipe) *pipeConn {
	t.Helper()
	return &pipeConn{t: t, pipe: p}
}

func (c *pipeConn) send(m protocol.Msg) {
	c.t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		c.t.Fatalf("marshal: %v", err)
	}
	if err := c.pipe.Send(b); err != nil {
		c.t.Fatalf("send: %v", err)
	}
}

func (c *pipeConn) recv() protocol.Msg {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	b, err := c.pipe.Recv(ctx)
	if err != nil {
		c.t.Fatalf("recv: %v", err)
	}
	var m protocol.Msg
	if err := json.Unmarshal(b, &m); err != nil {
		c.t.Fatalf("unmarshal: %v", err)
	}
	return m
}

func (c *pipeConn) waitFor(what string, pred func(protocol.Msg) bool) protocol.Msg {
	c.t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if m := c.recv(); pred(m) {
			return m
		}
	}
	c.t.Fatalf("timeout waiting for %s", what)
	return protocol.Msg{}
}

func (c *pipeConn) waitOutput(needle string) {
	c.t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	var got strings.Builder
	for time.Now().Before(deadline) {
		m := c.recv()
		if m.T == protocol.STOutput {
			got.Write(m.Data)
			c.seen.Write(m.Data)
			if strings.Contains(got.String(), needle) {
				return
			}
		}
	}
	c.t.Fatalf("timeout waiting for %q in output; saw %q", needle, got.String())
}

// The machine's own client takes its shell back from a live remote holder with
// no confirmation and no waiting for the lease to lapse: it is the machine's
// shell, and the person sitting at it is never locked out of it.
func TestOwnerClaimIsAlwaysHonoured(t *testing.T) {
	h := newLeaseHub(t, time.Hour)
	pipe := hub.NewPipe()
	defer pipe.Close()
	go h.ServeLocal(pipe.Local())

	owner := newPipeConn(t, pipe)
	remote := dialTest(t, serveHub(t, h))
	remote.create("s1")
	remote.waitFor("role", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleController
	})

	// The owner attaches as a viewer and claims. A remote client's claim
	// against this holder would be refused; the owner's is not.
	owner.send(protocol.Msg{T: protocol.CTAttach, SessionID: "s1", Cursor: 0})
	owner.waitFor("attached", func(m protocol.Msg) bool { return m.T == protocol.STAttached })
	owner.send(protocol.Msg{T: protocol.CTClaim, SessionID: "s1"})
	role := owner.waitFor("owner controls", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleController
	})
	if role.Epoch == 0 {
		t.Fatal("the role frame carries no epoch")
	}
	remote.waitFor("demoted", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleViewer
	})

	// And the owner's input lands at that epoch.
	owner.send(protocol.Msg{
		T: protocol.CTInput, SessionID: "s1", Epoch: role.Epoch,
		Data: []byte("printf 'OWNED\\n' 1\n"),
	})
	owner.waitOutput("OWNED")
}

// A lease nobody renews lapses, and the shell is free for anyone to claim: a
// controller whose network died cannot hold a shell forever.
func TestLeaseExpiresAndTheShellIsFree(t *testing.T) {
	h := newLeaseHub(t, 120*time.Millisecond)
	ts := serveHub(t, h)

	a := dialTest(t, ts)
	a.create("s1")
	a.waitFor("role", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleController
	})

	// A viewer arrives while the lease is live, and its plain claim is refused.
	b := dialTest(t, ts)
	b.send(protocol.Msg{T: protocol.CTAttach, SessionID: "s1", Cursor: 0})
	b.waitFor("attached", func(m protocol.Msg) bool { return m.T == protocol.STAttached })
	b.send(protocol.Msg{T: protocol.CTClaim, SessionID: "s1"})
	if e := b.waitFor("held", func(m protocol.Msg) bool { return m.T == protocol.STError }); e.Code != protocol.ErrLeaseHeld {
		t.Fatalf("claim got %q, want %q", e.Code, protocol.ErrLeaseHeld)
	}

	// The lease lapses, the janitor releases it, and the same claim is
	// honoured without a steal.
	time.Sleep(150 * time.Millisecond)
	h.Reap()
	b.waitFor("told the lease is free", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleViewer && m.Controller == ""
	})
	b.send(protocol.Msg{T: protocol.CTClaim, SessionID: "s1"})
	role := b.waitFor("controller after expiry", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleController
	})
	if role.Epoch < 2 {
		t.Fatalf("epoch after a handoff is %d, want it to have moved", role.Epoch)
	}
	b.send(protocol.Msg{
		T: protocol.CTInput, SessionID: "s1", Epoch: role.Epoch,
		Data: []byte("printf 'AFTER\\n' 1\n"),
	})
	b.waitOutput("AFTER")
}

// The janitor releases a lapsed lease and leaves a live one alone. The live
// case is the one that matters: a janitor that dropped held leases would hand
// every shell to whoever asked next, lease or no lease.
func TestJanitorLeavesALiveLeaseAlone(t *testing.T) {
	h := newLeaseHub(t, time.Hour)
	ts := serveHub(t, h)

	a := dialTest(t, ts)
	a.create("s1")
	a.waitFor("role", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleController
	})

	h.Reap()
	// Still the controller, and no role frame saying otherwise arrived.
	a.send(protocol.Msg{T: protocol.CTList, ReqID: "after-reap"})
	list := a.waitFor("list", func(m protocol.Msg) bool {
		return m.T == protocol.STSessions && m.ReqID == "after-reap"
	})
	if len(list.Sessions) != 1 || list.Sessions[0].Controller == "" {
		t.Fatalf("the live lease was dropped: %+v", list.Sessions)
	}

	// A session nobody holds — its holder detached, leaving a watcher behind — is
	// not something the janitor has to release, and it must not panic on it
	// either. The watcher left behind is told the lease is free, as it is when
	// a holder's connection drops.
	watcher := dialTest(t, ts)
	watcher.send(protocol.Msg{T: protocol.CTAttach, SessionID: "s1", Cursor: 0})
	watcher.waitFor("attached", func(m protocol.Msg) bool { return m.T == protocol.STAttached })
	a.send(protocol.Msg{T: protocol.CTDetach, SessionID: "s1"})
	watcher.waitFor("told the lease is free", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleViewer && m.Controller == ""
	})
	h.Reap()

	// A lapsed one goes.
	h2 := newLeaseHub(t, time.Millisecond)
	ts2 := serveHub(t, h2)
	b := dialTest(t, ts2)
	b.create("s1")
	b.waitFor("role", func(m protocol.Msg) bool { return m.T == protocol.STRole })
	time.Sleep(20 * time.Millisecond)
	h2.Reap()
	b.waitFor("told it lost the shell", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleViewer && m.Controller == ""
	})
}

// A machine can be its own. With control turned off, every client that is not
// the machine's own window is left watching: it cannot take a shell, cannot
// start one, and a shell it was holding is taken back in the frame that flips
// the switch. The machine's own window is never refused, because a switch on a
// machine must not lock the machine out of itself.
func TestControlOffLeavesOtherClientsWatching(t *testing.T) {
	h := newLeaseHub(t, time.Hour)
	pipe := hub.NewPipe()
	defer pipe.Close()
	go h.ServeLocal(pipe.Local())

	owner := newPipeConn(t, pipe)
	ts := serveHub(t, h)
	remote := dialTest(t, ts)
	remote.create("s1")
	remote.waitFor("role", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleController
	})

	// Someone flips the switch, which takes the shell back from the remote
	// holder and says so, rather than leaving it held by a client that may no
	// longer type.
	h.SetControlAllowed(false)
	if h.ControlAllowed() {
		t.Fatal("ControlAllowed still reports the switch as on")
	}
	remote.waitFor("the lease taken back", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleViewer && m.Controller == ""
	})

	// A claim is refused as a switch, not as a live lease: the answer names the
	// reason, and stealing does not change it.
	for _, steal := range []bool{false, true} {
		remote.send(protocol.Msg{T: protocol.CTClaim, SessionID: "s1", Steal: steal})
		refused := remote.waitFor("control_off", func(m protocol.Msg) bool { return m.T == protocol.STError })
		if refused.Code != protocol.ErrControlOff {
			t.Fatalf("claim with steal=%v got %q, want %q", steal, refused.Code, protocol.ErrControlOff)
		}
		if refused.Message == "" {
			t.Error("the refusal does not say why")
		}
	}

	// Starting a shell on this machine is control of it too.
	remote.send(protocol.Msg{
		T: protocol.CTCreate, SessionID: "s2", Cols: 80, Rows: 24, Shell: "/bin/sh",
	})
	refused := remote.waitFor("no new shell", func(m protocol.Msg) bool { return m.T == protocol.STError })
	if refused.Code != protocol.ErrControlOff {
		t.Fatalf("create got %q, want %q", refused.Code, protocol.ErrControlOff)
	}

	// Watching is still allowed: the switch is about who may drive, not about
	// being able to see.
	looker := dialTest(t, ts)
	looker.send(protocol.Msg{T: protocol.CTAttach, SessionID: "s1", Cursor: 0})
	att := looker.waitFor("attached", func(m protocol.Msg) bool { return m.T == protocol.STAttached })
	if att.Session == nil {
		t.Fatal("attached carried no session summary")
	}
	looker.send(protocol.Msg{T: protocol.CTInput, SessionID: "s1", Data: []byte("printf 'NOPE\\n' 1\n")})
	denied := looker.waitFor("input refused", func(m protocol.Msg) bool { return m.T == protocol.STError })
	if denied.Code != protocol.ErrNotController {
		t.Fatalf("a viewer's input got %q, want %q", denied.Code, protocol.ErrNotController)
	}

	// The machine's own window is unaffected: it claims, it types, it lands.
	owner.send(protocol.Msg{T: protocol.CTAttach, SessionID: "s1", Cursor: 0})
	owner.waitFor("attached", func(m protocol.Msg) bool { return m.T == protocol.STAttached })
	owner.send(protocol.Msg{T: protocol.CTClaim, SessionID: "s1"})
	role := owner.waitFor("owner controls", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleController
	})
	owner.send(protocol.Msg{
		T: protocol.CTInput, SessionID: "s1", Epoch: role.Epoch,
		Data: []byte("printf 'MINE\\n' 1\n"),
	})
	owner.waitOutput("MINE")

	// And the switch turns back on, so the refusal is not one-way.
	h.SetControlAllowed(true)
	remote.send(protocol.Msg{T: protocol.CTClaim, SessionID: "s1", Steal: true})
	remote.waitFor("controls again", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleController
	})
}

// A hub can be built refusing other clients' control, which is the headless
// case: a machine whose shells are its own from the moment it starts.
func TestHubStartsWithControlOff(t *testing.T) {
	h := hub.New(hub.Options{
		Host:       protocol.Host{Hostname: "test", Platform: "test", Arch: "test"},
		ControlOff: true,
	})
	ts := serveHub(t, h)

	a := dialTest(t, ts)
	a.send(protocol.Msg{
		T: protocol.CTCreate, SessionID: "s1", Cols: 80, Rows: 24, Shell: "/bin/sh",
	})
	refused := a.waitFor("no new shell", func(m protocol.Msg) bool { return m.T == protocol.STError })
	if refused.Code != protocol.ErrControlOff {
		t.Fatalf("create on a machine that allows no control got %q, want %q", refused.Code, protocol.ErrControlOff)
	}
}

// A lease that is let go goes home. The machine's own window is the shell's home,
// so a client that stops driving a shell — the desktop does exactly this when the
// screen moves to another shell, and the browser when a pane closes — does not
// leave the machine's own shell mute with nothing on screen to say why.
func TestAReleasedLeaseGoesHomeToTheMachine(t *testing.T) {
	h := newLeaseHub(t, time.Hour)
	pipe := hub.NewPipe()
	defer pipe.Close()
	go h.ServeLocal(pipe.Local())

	owner := newPipeConn(t, pipe)
	remote := dialTest(t, serveHub(t, h))
	remote.create("s1")
	owner.send(protocol.Msg{T: protocol.CTAttach, SessionID: "s1", Cursor: 0})
	owner.waitFor("attached", func(m protocol.Msg) bool { return m.T == protocol.STAttached })
	owner.waitFor("a viewer while the client drives", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleViewer && m.Controller != ""
	})

	remote.send(protocol.Msg{T: protocol.CTRelease, SessionID: "s1"})

	// The machine takes its shell back without claiming it, and the client that
	// let go is told who has it now.
	role := owner.waitFor("the machine has its shell back", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleController
	})
	remote.waitFor("told the machine has it", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleViewer && m.Controller != ""
	})

	// And what the machine types lands, at the epoch it was given.
	owner.send(protocol.Msg{
		T: protocol.CTInput, SessionID: "s1", Epoch: role.Epoch,
		Data: []byte("printf 'HOME\n' 1\n"),
	})
	owner.waitOutput("HOME")
}

// A release from the machine's own window is honoured: the lease is let go and
// stays let go.
//
// giveAway hands a lease nobody holds to the machine's own window when it is
// watching, which is right for a lease that lapsed or a client that dropped. It
// is wrong for the client that just released it on purpose: drop has already put
// that client among the watchers, so handing the lease "home" handed it straight
// back to the one that let it go — and a release did nothing at all for the
// machine's own window, which is the client that releases most, because it holds
// its own shells by default.
func TestTheMachinesOwnWindowCanLetAShellGo(t *testing.T) {
	h := newLeaseHub(t, time.Hour)
	pipe := hub.NewPipe()
	defer pipe.Close()
	go h.ServeLocal(pipe.Local())

	owner := newPipeConn(t, pipe)
	owner.send(protocol.Msg{T: protocol.CTCreate, Cols: 80, Rows: 24, Shell: "/bin/sh"})
	created := owner.waitFor("created", func(m protocol.Msg) bool { return m.T == protocol.STCreated })
	shell := created.SessionID
	owner.waitFor("the lease", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleController
	})

	owner.send(protocol.Msg{T: protocol.CTRelease, SessionID: shell})

	// The window is told it is a viewer, with nobody holding the shell: the
	// release was honoured rather than answered by taking the lease back.
	role := owner.waitFor("a viewer after letting go", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleViewer
	})
	if role.Controller != "" {
		t.Fatalf("controller after the release = %q, want nobody", role.Controller)
	}

	// And the roster agrees: the shell has no controller now.
	list := owner.waitFor("the roster", func(m protocol.Msg) bool { return m.T == protocol.STSessions })
	for _, s := range list.Sessions {
		if s.SessionID == shell && s.Controller != "" {
			t.Fatalf("the roster still names %q as controller", s.Controller)
		}
	}

	// And typing is refused: the window let the shell go and the hub says so
	// rather than dropping the keys silently.
	owner.send(protocol.Msg{T: protocol.CTInput, SessionID: shell, Data: []byte("x")})
	err := owner.waitFor("input refused", func(m protocol.Msg) bool { return m.T == protocol.STError })
	if err.Code != protocol.ErrNotController {
		t.Fatalf("input after a release was answered %q, want %q", err.Code, protocol.ErrNotController)
	}
}
