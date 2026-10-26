//go:build !windows

package relay_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/CarlOpenLab/beacon/internal/protocol"
	"github.com/CarlOpenLab/beacon/internal/relay"
	"github.com/CarlOpenLab/beacon/internal/tunnel"
)

// newBareRelay is a relay with no member behind it, so a test can play the
// member itself and choose exactly what goes on the wire.
func newBareRelay(t *testing.T) *httptest.Server {
	t.Helper()
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
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func writeFrameTo(t *testing.T, conn *websocket.Conn, f tunnel.Frame) {
	t.Helper()
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("marshal %s: %v", f.T, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatalf("write %s: %v", f.T, err)
	}
}

func readFrameFrom(t *testing.T, conn *websocket.Conn, what string, pred func(tunnel.Frame) bool) tunnel.Frame {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read while waiting for %s: %v", what, err)
		}
		var f tunnel.Frame
		if err := json.Unmarshal(data, &f); err != nil {
			t.Fatalf("unmarshal while waiting for %s: %v", what, err)
		}
		if pred(f) {
			return f
		}
	}
}

// TestRelayCarriesFramesAboveTheLibraryDefault guards the relay's own read
// limits, which are separate from the hub's chunking.
//
// A member still on an older build does not chunk: it answers a re-attach with
// the session's whole ring in one message, several times coder/websocket's
// 32768-byte default. If the relay cannot read that message it closes the
// member's link with StatusMessageTooBig and every session on that machine goes
// dark — so the relay must accept the large frame and pass it on, in both
// directions (a big paste is the client-to-member case).
func TestRelayCarriesFramesAboveTheLibraryDefault(t *testing.T) {
	ts := newBareRelay(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// The member, speaking the tunnel protocol by hand.
	member, _, err := websocket.Dial(ctx, wsURL(ts.URL)+"/relay/agent", &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer " + deviceID + ":" + deviceSec}},
	})
	if err != nil {
		t.Fatalf("dial member: %v", err)
	}
	t.Cleanup(func() { _ = member.Close(websocket.StatusNormalClosure, "done") })
	member.SetReadLimit(protocol.MaxMessageBytes)
	writeFrameTo(t, member, tunnel.Frame{
		T: tunnel.TAgentHello, V: tunnel.Version,
		MachineID: deviceID, MachineName: "big frame box",
		Host: protocol.Host{Hostname: "test", Platform: "test", Arch: "test"},
	})
	f := readFrameFrom(t, member, "agent welcome", func(f tunnel.Frame) bool {
		return f.T == tunnel.TAgentWelcome
	})
	if f.V != tunnel.Version {
		t.Fatalf("welcome version %d, want %d", f.V, tunnel.Version)
	}

	// The viewer, authenticated by header so the first frame it sees is the
	// roster.
	client, _, err := websocket.Dial(ctx, wsURL(ts.URL)+"/relay/client", &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer " + clientToken}},
	})
	if err != nil {
		t.Fatalf("dial client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close(websocket.StatusNormalClosure, "done") })
	client.SetReadLimit(protocol.MaxMessageBytes)

	const clientStream = 1
	writeFrameTo(t, client, tunnel.Frame{T: tunnel.TOpen, Machine: deviceID, Stream: clientStream})
	readFrameFrom(t, client, "accept", func(f tunnel.Frame) bool {
		return f.T == tunnel.TAccept && f.Stream == clientStream
	})
	opened := readFrameFrom(t, member, "routed open", func(f tunnel.Frame) bool {
		return f.T == tunnel.TOpen
	})

	// 100 KiB of payload is a ~133 KiB message on the wire: over four times the
	// default read limit, and the shape of a session replay from an unpatched
	// member.
	payload := bytes.Repeat([]byte("x"), 100*1024)

	writeFrameTo(t, member, tunnel.Frame{T: tunnel.TData, Stream: opened.Stream, Payload: payload})
	down := readFrameFrom(t, client, "member output", func(f tunnel.Frame) bool {
		return f.T == tunnel.TData && f.Stream == clientStream
	})
	if !bytes.Equal(down.Payload, payload) {
		t.Fatalf("member -> client payload is %d bytes, want %d intact", len(down.Payload), len(payload))
	}

	writeFrameTo(t, client, tunnel.Frame{T: tunnel.TData, Stream: clientStream, Payload: payload})
	up := readFrameFrom(t, member, "client input", func(f tunnel.Frame) bool {
		return f.T == tunnel.TData && f.Stream == opened.Stream
	})
	if !bytes.Equal(up.Payload, payload) {
		t.Fatalf("client -> member payload is %d bytes, want %d intact", len(up.Payload), len(payload))
	}
}
