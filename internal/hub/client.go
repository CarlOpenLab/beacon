package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/CarlOpenLab/term-fleet/internal/protocol"
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

	mu      sync.Mutex
	cursors map[string]int64
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

// send enqueues a frame. It never blocks the hub: a client that is not
// draining its queue is dropped, and it can reconnect and resume by cursor.
func (c *client) send(m protocol.Msg) {
	select {
	case <-c.done:
	case c.out <- m:
	default:
		go c.close("slow consumer")
	}
}

func (c *client) close(reason string) {
	c.once.Do(func() {
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
