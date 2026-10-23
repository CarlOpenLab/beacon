//go:build !windows

package hub_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/CarlOpenLab/term-fleet/internal/protocol"
)

// peerDefaultReadLimit is coder/websocket's default read limit for a socket
// whose owner never calls SetReadLimit. It is what every peer this hub does not
// control looks like — a desktop build from before the replay fix, a relay that
// has not been redeployed yet — so a frame that reaches it must stay under it.
const peerDefaultReadLimit = 32768

// recvSized is recv with the size of the message as it was on the wire, which
// is the number a peer's read limit is measured against.
func (c *conn) recvSized() (protocol.Msg, int) {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, b, err := c.conn.Read(ctx)
	if err != nil {
		c.t.Fatalf("read: %v", err)
	}
	var m protocol.Msg
	if err := json.Unmarshal(b, &m); err != nil {
		c.t.Fatalf("unmarshal: %v", err)
	}
	return m, len(b)
}

// TestLargeReplayArrivesInPeerSizedFrames is the white pane as a test.
//
// A session's ring holds 256 KiB, and a client that attaches at cursor 0 is
// sent all of it. As one frame that is several times the 32 KiB default that
// coder/websocket gives a socket, and the peer is disconnected with
// StatusMessageTooBig before a byte of the replay is used — which reads on the
// other side as the machine closing the stream, and looks on screen like a
// terminal that never paints.
//
// So the hub must split output into pieces a default-configured peer accepts,
// while keeping the byte flow intact: same total bytes, same final cursor, and
// the screen reset (Resync) on the first piece only.
func TestLargeReplayArrivesInPeerSizedFrames(t *testing.T) {
	ts := newTestHub(t)
	a := dialTest(t, ts)
	a.create("s1")
	a.waitFor("role", func(m protocol.Msg) bool { return m.T == protocol.STRole })

	// More than the ring can hold, so the replay a reconnecting client asks for
	// is the largest one possible and is flagged truncated.
	a.send(protocol.Msg{T: protocol.CTInput, SessionID: "s1",
		Data: []byte("head -c 300000 /dev/zero | tr '\\0' 'x'; echo; printf 'REPLAY_%d_TAIL\n' 7717\n")})
	a.waitOutput("REPLAY_7717_TAIL")

	b := dialTest(t, ts)
	// Reading with a limit above the assertion, so a regression is reported as
	// the frame size it actually is rather than as an opaque read failure.
	b.conn.SetReadLimit(4 << 20)
	b.send(protocol.Msg{T: protocol.CTAttach, SessionID: "s1"})
	att := b.waitFor("attached", func(m protocol.Msg) bool {
		return m.T == protocol.STAttached && m.SessionID == "s1"
	})
	if !att.Truncated {
		t.Fatal("a replay of a ring that fell behind is not marked truncated")
	}
	if att.Cursor <= peerDefaultReadLimit {
		t.Fatalf("replay ended at cursor %d, too small to exercise the limit", att.Cursor)
	}

	var total int
	var tail []byte
	first := true
	deadline := time.Now().Add(60 * time.Second)
	for !strings.Contains(string(tail), "REPLAY_7717_TAIL") {
		if time.Now().After(deadline) {
			t.Fatalf("replay never reached the newest output; %d bytes arrived", total)
		}
		m, size := b.recvSized()
		if m.T != protocol.STOutput {
			continue
		}
		if size >= peerDefaultReadLimit {
			t.Fatalf("one output frame is %d bytes on the wire; a peer with the library default is closed at %d", size, peerDefaultReadLimit)
		}
		if first && !m.Resync {
			t.Fatal("the first frame of a truncated replay did not reset the screen")
		}
		if !first && m.Resync {
			t.Fatal("a later frame of the replay reset the screen again")
		}
		first = false
		total += len(m.Data)
		tail = append(tail, m.Data...)
		if len(tail) > 4096 {
			tail = tail[len(tail)-4096:]
		}
	}
	if total < 4*peerDefaultReadLimit {
		t.Fatalf("replay carried %d bytes, want the whole ring (well over %d)", total, 4*peerDefaultReadLimit)
	}
	if first {
		t.Fatal("a truncated replay arrived with no output frame at all")
	}
}
