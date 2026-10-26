package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/CarlOpenLab/beacon/internal/diag"
	"github.com/CarlOpenLab/beacon/internal/protocol"
)

// client is one attached viewer. Every session it watches shares this
// transport, and it keeps a per-session cursor so the hub can send only what
// is new.
type client struct {
	h    *Hub
	id   string
	tp   Transport
	out  chan protocol.Msg
	done chan struct{}
	once sync.Once

	// owner marks the client that IS the machine: the desktop window's own
	// connection, over an in-process transport rather than a socket. Its claim
	// is always honoured, because a shell on this machine is the machine's to
	// take back — the one case where no confirmation and no lease expiry
	// stands between a person and their own shell.
	owner bool

	// caps are what the client announced in its hello. A capability is only
	// used when both ends named it, so an old client gets exactly the wire it
	// always got.
	capsMu sync.Mutex
	capList []string
	deflate atomic.Bool

	mu      sync.Mutex
	cursors map[string]int64
	// up is the file a paste is currently writing, if any; upPath is where it
	// is going, kept beside it so an abort can remove the partial file.
	up     *upload
	upPath string
}

func (h *Hub) newClient(tp Transport) *client {
	h.mu.Lock()
	h.nextSeq++
	seq := h.nextSeq
	h.mu.Unlock()

	return &client{
		h:       h,
		id:      fmt.Sprintf("c%d", seq),
		tp:      tp,
		out:     make(chan protocol.Msg, outboundQueue),
		done:    make(chan struct{}),
		cursors: map[string]int64{},
	}
}

// ownerClient is newClient marked as the machine itself.
func (h *Hub) ownerClient(tp Transport) *client {
	c := h.newClient(tp)
	c.owner = true
	return c
}

// send enqueues a frame. It never blocks the hub: a client that is not
// draining its queue is dropped, and it can reconnect and resume by cursor.
func (c *client) send(m protocol.Msg) {
	// Compress the bulk here rather than at the call sites: every path that
	// hands a viewer output — a live flush and an attach's replay alike — goes
	// through this one function, so there is one place to get it right. The
	// cursor is untouched: it counts the bytes the program wrote, not the
	// bytes the frame carried.
	if m.T == protocol.STOutput && c.deflate.Load() && protocol.WorthCompressing(m.Data) {
		if z, err := protocol.Deflate(m.Data); err == nil && len(z) < len(m.Data) {
			m.Data, m.Z = z, true
		}
	}
	select {
	case <-c.done:
	case c.out <- m:
	default:
		// A queue that fills is how a viewer is lost under a busy shell, and it
		// is invisible to both ends: the hub drops the client, and the viewer
		// only sees its frames stop. Named here with the frame that did not fit.
		diag.Warnf("hub", "client dropped: queue full client=%s frame=%s session=%s", c.id, m.T, m.SessionID)
		go c.close("slow consumer")
	}
}

// negotiated records what the client said it understands, and turns on the
// features the hub shares with it.
func (c *client) negotiated(caps []string) {
	c.capsMu.Lock()
	c.capList = append([]string(nil), caps...)
	c.capsMu.Unlock()
	c.deflate.Store(protocol.HasCap(caps, protocol.CapDeflate))
}

// caps is what the client announced.
func (c *client) caps() []string {
	c.capsMu.Lock()
	defer c.capsMu.Unlock()
	return append([]string(nil), c.capList...)
}

func (c *client) close(reason string) {
	c.once.Do(func() {
		c.abortUpload("connection closed")
		close(c.done)
		_ = c.tp.Close(reason)
	})
}

func (c *client) fail(code, message, sessionID string) {
	c.send(protocol.Msg{
		T:         protocol.STError,
		Code:      code,
		Message:   message,
		SessionID: sessionID,
	})
}

func (c *client) cursor(sessionID string) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cursors[sessionID]
}

func (c *client) setCursor(sessionID string, v int64) {
	c.mu.Lock()
	c.cursors[sessionID] = v
	c.mu.Unlock()
}

func (c *client) readLoop(ctx context.Context) {
	for {
		data, err := c.tp.Read(ctx)
		if err != nil {
			return
		}
		var m protocol.Msg
		if err := json.Unmarshal(data, &m); err != nil {
			c.fail(protocol.ErrBadRequest, "invalid frame: "+err.Error(), "")
			continue
		}
		c.h.handle(c, m)
	}
}

func (c *client) writeLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.done:
			return
		case m := <-c.out:
			b, err := json.Marshal(m)
			if err != nil {
				continue
			}
			wctx, cancel := context.WithTimeout(ctx, writeTimeout)
			err = c.tp.Write(wctx, b)
			cancel()
			if err != nil {
				c.close("write failed")
				return
			}
		}
	}
}

// heartbeat keeps the connection honest. A phone that walks out of Wi-Fi range
// is noticed here rather than after the OS gives up on the socket.
func (c *client) heartbeat(ctx context.Context) {
	t := time.NewTicker(pingInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.done:
			return
		case <-t.C:
			pctx, cancel := context.WithTimeout(ctx, pingTimeout)
			err := c.tp.Ping(pctx)
			cancel()
			if err != nil {
				c.close("ping timeout")
				return
			}
		}
	}
}
