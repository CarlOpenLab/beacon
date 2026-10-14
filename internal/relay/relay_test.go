//go:build !windows

package relay_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/CarlOpenLab/term-fleet/internal/agent"
	"github.com/CarlOpenLab/term-fleet/internal/daemon"
	"github.com/CarlOpenLab/term-fleet/internal/protocol"
	"github.com/CarlOpenLab/term-fleet/internal/relay"
	"github.com/CarlOpenLab/term-fleet/internal/tunnel"
)

const (
	clientToken = "client-token-for-tests"
	deviceID    = "member-1"
	deviceSec   = "member-1-secret"
)

// harness wires a real daemon (real PTY), a real relay, and a real agent, all
// over real sockets. Nothing is stubbed: if the tunnel could not carry a
// session, these tests could not pass.
type harness struct {
	t      *testing.T
	relay  *httptest.Server
	daemon *daemon.Server
	router *relay.Relay
	cancel context.CancelFunc
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	d, err := daemon.New(daemon.Options{Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("daemon: %v", err)
	}
	go func() { _ = d.Serve(ctx) }()
	t.Cleanup(func() { _ = d.Close() })

	rl, err := relay.New(relay.Config{
		Devices:     map[string]string{deviceID: deviceSec},
		ClientToken: clientToken,
	})
	if err != nil {
		t.Fatalf("relay: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/relay/agent", rl.ServeAgent)
	mux.HandleFunc("/relay/client", rl.ServeClient)
	mux.HandleFunc("/relay/machines", rl.Machines)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	ag, err := agent.New(agent.Config{
		URL:         wsURL(ts.URL) + "/relay/agent",
		DeviceID:    deviceID,
		Secret:      deviceSec,
		MachineName: "test box",
		Hub:         d.Hub(),
		MinBackoff:  10 * time.Millisecond,
		MaxBackoff:  50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("agent: %v", err)
	}
	go func() { _ = ag.Run(ctx) }()

	return &harness{t: t, relay: ts, daemon: d, router: rl, cancel: cancel}
}

func wsURL(httpURL string) string { return "ws" + strings.TrimPrefix(httpURL, "http") }

// ---------------------------------------------------------------- test client

// client is a browser-equivalent: one tunnel socket, many streams.
//
// Output is accumulated per stream rather than queued like other frames. That
// is not a convenience: a test that waits for one stream's handshake while
// another stream's output arrives would otherwise throw that output away, and
// the resulting failure looks exactly like a product bug.
type client struct {
	t    *testing.T
	conn *websocket.Conn
	in   chan tunnel.Frame

	mu      sync.Mutex
	outText map[uint32]string
}

func (h *harness) dial() *client {
	h.t.Helper()
	url := wsURL(h.relay.URL) + "/relay/client?token=" + clientToken
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		h.t.Fatalf("dial relay as client: %v", err)
	}
	h.t.Cleanup(func() { _ = conn.Close(websocket.StatusNormalClosure, "test done") })

	c := &client{t: h.t, conn: conn, in: make(chan tunnel.Frame, 512), outText: map[uint32]string{}}
	go func() {
		defer close(c.in)
		for {
			_, data, err := conn.Read(context.Background())
			if err != nil {
				return
			}
			var f tunnel.Frame
			if err := json.Unmarshal(data, &f); err != nil {
				return
			}
			if f.T == tunnel.TData {
				var m protocol.Msg
				if err := json.Unmarshal(f.Payload, &m); err == nil && m.T == protocol.STOutput {
					c.mu.Lock()
					c.outText[f.Stream] += string(m.Data)
					c.mu.Unlock()
					continue
				}
			}
			c.in <- f
		}
	}()
	return c
}

// awaitOutput waits until the session on a stream has printed substr.
func (c *client) awaitOutput(stream uint32, substr string) string {
	c.t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		got := c.outText[stream]
		c.mu.Unlock()
		if strings.Contains(got, substr) {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
	c.mu.Lock()
	got := c.outText[stream]
	c.mu.Unlock()
	c.t.Fatalf("stream %d never printed %q; it printed %q", stream, substr, got)
	return ""
}

func (c *client) output(stream uint32) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.outText[stream]
}

func (c *client) write(f tunnel.Frame) {
	c.t.Helper()
	b, err := json.Marshal(f)
	if err != nil {
		c.t.Fatalf("marshal tunnel frame: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.conn.Write(ctx, websocket.MessageText, b); err != nil {
		c.t.Fatalf("write tunnel frame: %v", err)
	}
}

func (c *client) waitFor(what string, pred func(tunnel.Frame) bool) tunnel.Frame {
	c.t.Helper()
	deadline := time.After(15 * time.Second)
	for {
		select {
		case f, ok := <-c.in:
			if !ok {
				c.t.Fatalf("relay socket closed while waiting for %s", what)
			}
			if pred(f) {
				return f
			}
		case <-deadline:
			c.t.Fatalf("timed out waiting for %s", what)
		}
	}
}

// inner sends one session-protocol frame inside a stream, the way a browser
// would after the agent has accepted the stream.
func (c *client) inner(stream uint32, m protocol.Msg) {
	c.t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		c.t.Fatalf("marshal inner frame: %v", err)
	}
	c.write(tunnel.Frame{T: tunnel.TData, Stream: stream, Payload: b})
}

func decode(t *testing.T, f tunnel.Frame) protocol.Msg {
	t.Helper()
	var m protocol.Msg
	if err := json.Unmarshal(f.Payload, &m); err != nil {
		t.Fatalf("decode inner frame %q: %v", string(f.Payload), err)
	}
	return m
}

// awaitInner waits for the next inner frame on a stream that satisfies pred.
func (c *client) awaitInner(stream uint32, what string, pred func(protocol.Msg) bool) protocol.Msg {
	c.t.Helper()
	f := c.waitFor(what, func(f tunnel.Frame) bool {
		if f.T != tunnel.TData || f.Stream != stream {
			return false
		}
		m := decode(c.t, f)
		// Surface the session protocol's own error frames instead of letting
		// the wait time out: "not_controller" and friends are answers, not
		// silence, and a timeout would hide which one it was.
		if m.T == protocol.STError {
			c.t.Fatalf("while waiting for %s: session protocol error %s: %s", what, m.Code, m.Message)
		}
		return pred(m)
	})
	return decode(c.t, f)
}

// waitMachinesOnline blocks until the relay reports the member as reachable.
func (c *client) waitMachinesOnline() tunnel.Machine {
	c.t.Helper()
	f := c.waitFor("machine online", func(f tunnel.Frame) bool {
		if f.T != tunnel.TMachines {
			return false
		}
		for _, m := range f.Machines {
			if m.ID == deviceID && m.Online {
				return true
			}
		}
		return false
	})
	for _, m := range f.Machines {
		if m.ID == deviceID {
			return m
		}
	}
	c.t.Fatal("machine list changed while being scanned")
	return tunnel.Machine{}
}

// ---------------------------------------------------------------- tests

// TestSessionOverRelay is the point of the whole package: a client that has
// never met the daemon drives a real PTY through the relay.
func TestSessionOverRelay(t *testing.T) {
	h := newHarness(t)
	c := h.dial()

	m := c.waitMachinesOnline()
	if m.Name != "test box" {
		t.Fatalf("machine name is %q, want %q", m.Name, "test box")
	}
	if m.Host.Platform == "" {
		t.Fatal("machine list lost the member's platform")
	}

	const stream = 1
	c.write(tunnel.Frame{T: tunnel.TOpen, Machine: deviceID, Stream: stream})
	acc := c.waitFor("accept", func(f tunnel.Frame) bool {
		return f.T == tunnel.TAccept && f.Stream == stream
	})
	if acc.Machine != deviceID {
		t.Fatalf("accept names machine %q, want %q", acc.Machine, deviceID)
	}

	c.inner(stream, protocol.Msg{T: protocol.CTHello, V: protocol.Version})
	hello := c.awaitInner(stream, "inner hello", func(m protocol.Msg) bool { return m.T == protocol.STHello })
	if hello.Host == nil || hello.Host.Platform == "" {
		t.Fatal("inner hello lost the host description")
	}

	c.inner(stream, protocol.Msg{
		T: protocol.CTCreate, SessionID: "s1", Cols: 80, Rows: 24, Shell: "/bin/sh",
	})
	c.awaitInner(stream, "created", func(m protocol.Msg) bool { return m.T == protocol.STCreated })
	att := c.awaitInner(stream, "attached", func(m protocol.Msg) bool { return m.T == protocol.STAttached })
	if att.Session == nil || !att.Session.Alive {
		t.Fatal("the session the relay attached is not alive")
	}
	role := c.awaitInner(stream, "role", func(m protocol.Msg) bool { return m.T == protocol.STRole })
	if role.Role != protocol.RoleController {
		t.Fatalf("the only client should control the session, got %q", role.Role)
	}

	// The output is a real shell answering a real command, carried end to end.
	c.inner(stream, protocol.Msg{
		T: protocol.CTInput, SessionID: "s1", Data: []byte("printf 'THROUGH_RELAY_%d\\n' 7\n"),
	})
	c.awaitOutput(stream, "THROUGH_RELAY_7")
}

// TestStreamsAreIndependent proves one client can watch two sessions without
// the two sharing state — the property the PC tab bar and the phone list both
// rest on.
func TestStreamsAreIndependent(t *testing.T) {
	h := newHarness(t)
	c := h.dial()
	c.waitMachinesOnline()

	for _, tc := range []struct {
		stream uint32
		id     string
		word   string
	}{{1, "one", "WORD_ONE"}, {2, "two", "WORD_TWO"}} {
		c.write(tunnel.Frame{T: tunnel.TOpen, Machine: deviceID, Stream: tc.stream})
		c.waitFor("accept", func(f tunnel.Frame) bool {
			return f.T == tunnel.TAccept && f.Stream == tc.stream
		})
		c.inner(tc.stream, protocol.Msg{T: protocol.CTHello, V: protocol.Version})
		c.awaitInner(tc.stream, "inner hello", func(m protocol.Msg) bool { return m.T == protocol.STHello })
		c.inner(tc.stream, protocol.Msg{
			T: protocol.CTCreate, SessionID: tc.id, Cols: 80, Rows: 24, Shell: "/bin/sh",
		})
		c.awaitInner(tc.stream, "created", func(m protocol.Msg) bool { return m.T == protocol.STCreated })
		c.awaitInner(tc.stream, "attached", func(m protocol.Msg) bool { return m.T == protocol.STAttached })
		c.inner(tc.stream, protocol.Msg{
			T: protocol.CTInput, SessionID: tc.id, Data: []byte("printf '" + tc.word + "\\n'\n"),
		})
	}

	// Each word must arrive on its own stream, never the other's.
	c.awaitOutput(1, "WORD_ONE")
	c.awaitOutput(2, "WORD_TWO")
	if strings.Contains(c.output(1), "WORD_TWO") || strings.Contains(c.output(2), "WORD_ONE") {
		t.Fatal("output crossed between streams")
	}
}

// TestMemberOfflineIsRefused: the relay must not accept a stream for a machine
// it cannot reach, or a client would hang forever waiting for output.
func TestMemberOfflineIsRefused(t *testing.T) {
	h := newHarness(t)
	c := h.dial()
	c.waitFor("initial machine list", func(f tunnel.Frame) bool { return f.T == tunnel.TMachines })

	c.write(tunnel.Frame{T: tunnel.TOpen, Machine: "no-such-machine", Stream: 1})
	f := c.waitFor("error", func(f tunnel.Frame) bool { return f.T == tunnel.TError })
	if f.Code != tunnel.ErrMachineOffline {
		t.Fatalf("error code is %q, want %q", f.Code, tunnel.ErrMachineOffline)
	}
}

// TestMemberDropClosesClientStreams: when a member goes away, the client is
// told, so the UI can mark the tab rather than show a frozen screen.
func TestMemberDropClosesClientStreams(t *testing.T) {
	h := newHarness(t)
	c := h.dial()
	c.waitMachinesOnline()

	const stream = 1
	c.write(tunnel.Frame{T: tunnel.TOpen, Machine: deviceID, Stream: stream})
	c.waitFor("accept", func(f tunnel.Frame) bool { return f.T == tunnel.TAccept && f.Stream == stream })

	h.cancel() // the member's link dies

	f := c.waitFor("stream closed", func(f tunnel.Frame) bool {
		return f.T == tunnel.TClose && f.Stream == stream
	})
	if f.Code != tunnel.ErrMachineOffline {
		t.Fatalf("close code is %q, want %q", f.Code, tunnel.ErrMachineOffline)
	}
}

// TestAuth covers both doors: a member with the wrong secret, and a client
// with the wrong token, are turned away before any routing happens.
func TestAuth(t *testing.T) {
	h := newHarness(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, resp, err := websocket.Dial(ctx, wsURL(h.relay.URL)+"/relay/agent", &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer " + deviceID + ":wrong"}},
	})
	if err == nil {
		t.Fatal("a member with the wrong secret was allowed in")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong member secret got %v, want 401", resp)
	}

	_, resp2, err := websocket.Dial(ctx, wsURL(h.relay.URL)+"/relay/client?token=nope", nil)
	if err == nil {
		t.Fatal("a client with the wrong token was allowed in")
	}
	if resp2 == nil || resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong client token got %v, want 401", resp2)
	}
}

// TestAgentRedialsAfterLinkLoss is the counterpart to the dial-out design: the
// relay drops every link, and the member comes back on its own — no operator,
// no restart. A client reconnecting afterwards sees the machine online again,
// with the sessions it left behind still in place.
func TestAgentRedialsAfterLinkLoss(t *testing.T) {
	h := newHarness(t)

	c1 := h.dial()
	c1.waitMachinesOnline()
	const stream = 1
	c1.write(tunnel.Frame{T: tunnel.TOpen, Machine: deviceID, Stream: stream})
	c1.waitFor("accept", func(f tunnel.Frame) bool { return f.T == tunnel.TAccept && f.Stream == stream })
	c1.inner(stream, protocol.Msg{T: protocol.CTHello, V: protocol.Version})
	c1.awaitInner(stream, "inner hello", func(m protocol.Msg) bool { return m.T == protocol.STHello })
	c1.inner(stream, protocol.Msg{
		T: protocol.CTCreate, SessionID: "survivor", Cols: 80, Rows: 24, Shell: "/bin/sh",
	})
	c1.awaitInner(stream, "attached", func(m protocol.Msg) bool { return m.T == protocol.STAttached })

	// Drop every link the relay holds. Sessions belong to the member, so they
	// must outlive this.
	h.router.Shutdown("test: forced link loss")

	c2 := h.dial()
	c2.waitMachinesOnline()

	// The session created before the outage is still there to attach to.
	const stream2 = 2
	c2.write(tunnel.Frame{T: tunnel.TOpen, Machine: deviceID, Stream: stream2})
	c2.waitFor("accept", func(f tunnel.Frame) bool { return f.T == tunnel.TAccept && f.Stream == stream2 })
	c2.inner(stream2, protocol.Msg{T: protocol.CTHello, V: protocol.Version})
	c2.awaitInner(stream2, "inner hello", func(m protocol.Msg) bool { return m.T == protocol.STHello })
	c2.inner(stream2, protocol.Msg{T: protocol.CTList, ReqID: "after-outage"})
	list := c2.awaitInner(stream2, "session list", func(m protocol.Msg) bool {
		return m.T == protocol.STSessions && m.ReqID == "after-outage"
	})
	found := false
	for _, s := range list.Sessions {
		if s.SessionID == "survivor" && s.Alive {
			found = true
		}
	}
	if !found {
		t.Fatalf("the session did not survive the relay outage: %+v", list.Sessions)
	}
}
