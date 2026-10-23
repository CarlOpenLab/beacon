//go:build !windows

package relay_test

import (
	"testing"

	"github.com/CarlOpenLab/term-fleet/internal/protocol"
	"github.com/CarlOpenLab/term-fleet/internal/tunnel"
)

// TestReattachReplaysLargeRing is the white pane end to end, over real
// sockets: a viewer fills a session's ring, loses its link, and comes back on a
// fresh relay socket with cursor 0 — the shape of the desktop reconnecting.
//
// Every socket in this test keeps coder/websocket's default 32 KiB read limit,
// which is what the desktop and the relay had when the bug was reported. The
// replay is a megabyte, so before output was chunked the first frame was several
// times the limit: the relay closed the agent link with StatusMessageTooBig
// ("read limited at 32769 bytes"), the desktop saw "machine closed the stream",
// and the pane stayed blank forever. A pass here means the replay arrives whole
// at a peer that never called SetReadLimit.
func TestReattachReplaysLargeRing(t *testing.T) {
	h := newHarness(t)
	c := h.dial()
	c.waitMachinesOnline()

	const stream = 1
	c.write(tunnel.Frame{T: tunnel.TOpen, Machine: deviceID, Stream: stream})
	c.waitFor("accept", func(f tunnel.Frame) bool {
		return f.T == tunnel.TAccept && f.Stream == stream
	})
	c.inner(stream, protocol.Msg{T: protocol.CTHello, V: protocol.Version})
	c.awaitInner(stream, "inner hello", func(m protocol.Msg) bool { return m.T == protocol.STHello })
	c.inner(stream, protocol.Msg{
		T: protocol.CTCreate, SessionID: "s1", Cols: 100, Rows: 30, Shell: "/bin/sh",
	})
	c.awaitInner(stream, "created", func(m protocol.Msg) bool { return m.T == protocol.STCreated })
	c.awaitInner(stream, "attached", func(m protocol.Msg) bool { return m.T == protocol.STAttached })

	// Continue past the ring so the replay a reconnecting viewer asks for is
	// both truncated and as large as it can be.
	c.inner(stream, protocol.Msg{
		T: protocol.CTInput, SessionID: "s1",
		Data: []byte("head -c 1300000 /dev/zero | tr '\\0' 'x'; echo; printf 'REPLAY_%d_TAIL\n' 7717\n"),
	})
	c.awaitOutput(stream, "REPLAY_7717_TAIL")

	// The reconnect: a new socket, a new stream number, and an attach at cursor
	// 0 — the desktop's current behaviour after its link dies.
	const again = 1
	c2 := h.dial()
	c2.waitMachinesOnline()
	c2.write(tunnel.Frame{T: tunnel.TOpen, Machine: deviceID, Stream: again})
	c2.waitFor("accept", func(f tunnel.Frame) bool {
		return f.T == tunnel.TAccept && f.Stream == again
	})
	c2.inner(again, protocol.Msg{T: protocol.CTHello, V: protocol.Version})
	c2.awaitInner(again, "inner hello", func(m protocol.Msg) bool { return m.T == protocol.STHello })
	c2.inner(again, protocol.Msg{T: protocol.CTAttach, SessionID: "s1", Cursor: 0})
	att := c2.awaitInner(again, "attached", func(m protocol.Msg) bool { return m.T == protocol.STAttached })
	if !att.Truncated {
		t.Fatal("the replay of a ring that fell behind was not marked truncated")
	}

	// The newest bytes — the marker the shell printed last — must survive the
	// trip, which they cannot if the link died on the first frame.
	got := c2.awaitOutput(again, "REPLAY_7717_TAIL")
	if len(got) < 100*1024 {
		t.Fatalf("replay delivered %d bytes, want the ring's 256 KiB", len(got))
	}
}
