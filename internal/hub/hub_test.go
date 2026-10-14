//go:build !windows

package hub_test

import (
	"context"
	"encoding/json"
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
}

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
	deadline := time.Now().Add(15 * time.Second)
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
	deadline := time.Now().Add(15 * time.Second)
	var got strings.Builder
	for time.Now().Before(deadline) {
		m := c.recv()
		if m.T == protocol.STOutput {
			got.Write(m.Data)
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

	// Claiming takes control away from A, and A is told.
	b.send(protocol.Msg{T: protocol.CTClaim, SessionID: "s1"})
	b.waitFor("role controller", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleController
	})
	a.waitFor("demoted to viewer", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleViewer
	})

	a.send(protocol.Msg{T: protocol.CTInput, SessionID: "s1", Data: []byte("echo NOPE\n")})
	e2 := a.waitFor("a denied", func(m protocol.Msg) bool { return m.T == protocol.STError })
	if e2.Code != protocol.ErrNotController {
		t.Fatalf("demoted client error code is %q, want %q", e2.Code, protocol.ErrNotController)
	}

	b.send(protocol.Msg{
		T: protocol.CTInput, SessionID: "s1",
		Data: []byte("printf 'FROM_B\\n' 1\n"),
	})
	b.waitOutput("FROM_B")
}

func TestAttachResumesFromCursor(t *testing.T) {
	ts := newTestHub(t)
	a := dialTest(t, ts)
	a.create("s1")
	a.waitFor("role", func(m protocol.Msg) bool { return m.T == protocol.STRole })

	a.send(protocol.Msg{T: protocol.CTInput, SessionID: "s1", Data: []byte("printf 'AAA\\n'\n")})
	cursor := a.waitOutput("AAA")

	// More output happens while the second client is not watching yet.
	a.send(protocol.Msg{T: protocol.CTInput, SessionID: "s1", Data: []byte("printf 'BBB\\n'\n")})
	a.waitOutput("BBB")

	b := dialTest(t, ts)
	b.send(protocol.Msg{T: protocol.CTAttach, SessionID: "s1", Cursor: cursor})

	var got strings.Builder
	deadline := time.Now().Add(15 * time.Second)
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

	// A viewer may stop watching, but may not kill everyone's session.
	b.send(protocol.Msg{T: protocol.CTClose, SessionID: "s1"})
	e := b.waitFor("close refused", func(m protocol.Msg) bool { return m.T == protocol.STError })
	if e.Code != protocol.ErrNotController {
		t.Fatalf("viewer close error code is %q, want %q", e.Code, protocol.ErrNotController)
	}

	a.send(protocol.Msg{T: protocol.CTClose, SessionID: "s1"})
	a.waitFor("closed", func(m protocol.Msg) bool { return m.T == protocol.STClosed })
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
