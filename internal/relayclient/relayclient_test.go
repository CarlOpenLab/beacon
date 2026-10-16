//go:build !windows

package relayclient_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/CarlOpenLab/term-fleet/internal/protocol"
	"github.com/CarlOpenLab/term-fleet/internal/relay"
	"github.com/CarlOpenLab/term-fleet/internal/relayclient"
	"github.com/CarlOpenLab/term-fleet/internal/tunnel"
)

// A real relay behind a real HTTP server, and a hand-rolled member dialled into
// it: the client's roster, its streams and the frames they carry are exercised
// against the thing that will serve them, because a mock of this protocol would
// only test the mock.
func newRelay(t *testing.T) *httptest.Server {
	t.Helper()
	r, err := relay.New(relay.Config{
		Devices:     map[string]string{"m1": "sec1"},
		ClientToken: "tok",
	})
	if err != nil {
		t.Fatalf("relay.New: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/relay/agent", r.ServeAgent)
	mux.HandleFunc("/relay/client", r.ServeClient)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func wsURL(base, path string) string {
	return "ws" + strings.TrimPrefix(base, "http") + path
}

func mustFrame(t *testing.T, f tunnel.Frame) []byte {
	t.Helper()
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("marshal frame: %v", err)
	}
	return b
}

// dialMember speaks the member half of the protocol: it says hello, waits to be
// welcomed, and answers an open with an accept plus one hello frame inside the
// stream — the smallest thing that proves a stream carries the session protocol.
func dialMember(t *testing.T, base, id, secret string) func() {
	t.Helper()
	ctx := context.Background()
	ws, _, err := websocket.Dial(ctx, wsURL(base, "/relay/agent"), &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer " + id + ":" + secret}},
	})
	if err != nil {
		t.Fatalf("member dial: %v", err)
	}
	if err := ws.Write(ctx, websocket.MessageText, mustFrame(t, tunnel.Frame{
		T:           tunnel.TAgentHello,
		V:           tunnel.Version,
		MachineID:   id,
		MachineName: id,
		Host:        protocol.Host{Hostname: id, Platform: "test", Arch: "test"},
	})); err != nil {
		t.Fatalf("member hello: %v", err)
	}
	_, b, err := ws.Read(ctx)
	if err != nil {
		t.Fatalf("member welcome: %v", err)
	}
	var welcome tunnel.Frame
	if err := json.Unmarshal(b, &welcome); err != nil || welcome.T != tunnel.TAgentWelcome {
		t.Fatalf("welcome = %+v (err %v)", welcome, err)
	}

	go func() {
		for {
			_, b, err := ws.Read(ctx)
			if err != nil {
				return
			}
			var f tunnel.Frame
			if err := json.Unmarshal(b, &f); err != nil {
				continue
			}
			if f.T != tunnel.TOpen {
				continue
			}
			_ = ws.Write(ctx, websocket.MessageText, mustFrame(t, tunnel.Frame{T: tunnel.TAccept, Stream: f.Stream}))
			hello, _ := json.Marshal(protocol.Msg{T: protocol.STHello, V: protocol.Version})
			_ = ws.Write(ctx, websocket.MessageText, mustFrame(t, tunnel.Frame{T: tunnel.TData, Stream: f.Stream, Payload: hello}))
		}
	}()
	return func() { _ = ws.CloseNow() }
}

// handlers records what a stream reports.
type handlers struct {
	frames chan protocol.Msg
	opened chan string
	closed chan string
}

func (h handlers) Open() {
	if h.opened != nil {
		h.opened <- "open"
	}
}

func (h handlers) Frame(m protocol.Msg) { h.frames <- m }

func (h handlers) Close(reason string) {
	if h.closed != nil {
		h.closed <- reason
	}
}

// The client hears the roster, and a machine's stream carries the session
// protocol into it: this is the whole point of the package.
func TestClientGetsRosterAndStreams(t *testing.T) {
	ts := newRelay(t)
	stop := dialMember(t, ts.URL, "m1", "sec1")
	defer stop()

	// The hook goes in with the connection: the roster can arrive while Dial is
	// still waiting for the welcome, and one assigned afterwards would miss it.
	rostered := make(chan []tunnel.Machine, 4)
	c, err := relayclient.Dial(context.Background(), wsURL(ts.URL, "/relay/client"), "tok",
		relayclient.Options{}, relayclient.Hooks{OnRoster: func(m []tunnel.Machine) { rostered <- m }})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	// The member is online, and the roster says so.
	select {
	case m := <-rostered:
		if len(m) != 1 || m[0].ID != "m1" || !m[0].Online {
			t.Fatalf("roster = %+v, want one online m1", m)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no roster arrived")
	}

	// A stream to it is accepted, and the frame the member's hub sends arrives.
	h := handlers{frames: make(chan protocol.Msg, 8), opened: make(chan string, 1)}
	s := c.Open("m1", h)
	defer s.Close()

	select {
	case <-h.opened:
	case <-time.After(5 * time.Second):
		t.Fatal("the stream was never accepted")
	}
	select {
	case m := <-h.frames:
		if m.T != protocol.STHello {
			t.Fatalf("first frame = %q, want hello", m.T)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no frame arrived on the stream")
	}
}

// A wrong token is refused, and the client says so rather than retrying
// forever behind a spinner.
func TestClientRefusesABadToken(t *testing.T) {
	ts := newRelay(t)
	if _, err := relayclient.Dial(context.Background(), wsURL(ts.URL, "/relay/client"), "wrong", relayclient.Options{}, relayclient.Hooks{}); err == nil {
		t.Fatal("a bad token connected")
	}
}
