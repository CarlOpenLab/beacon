package main

// The wire between a session and the native terminal view.
//
// The terminal plugin asks for an io.ReadWriteCloser: it reads what the shell
// prints and writes what the user types. This is that, over the window's link
// instead of over the session's ring buffer — which is what makes the window a
// client like any other. The one thing a client has that a ring reader does not
// is the lease: a window that does not hold it types nothing, and the shell
// never sees the keys.

import (
	"io"
	"sync"

	"github.com/CarlOpenLab/term-fleet/internal/protocol"
)

// termConn is one terminal's end of one session.
type termConn struct {
	sess *session

	mu       sync.Mutex
	pending  []byte
	closed   bool
	once     sync.Once
	done     chan struct{}
	lastCols   int
	lastRows   int
	syncedCols int
	syncedRows int
}

func newTermConn(s *session) *termConn {
	return &termConn{sess: s, done: make(chan struct{})}
}

// Read hands the terminal the session's output, blocking until there is some.
// It reports io.EOF once the session is gone, so the view learns of the end the
// way it does when a program exits.
func (c *termConn) Read(p []byte) (int, error) {
	for {
		c.mu.Lock()
		if len(c.pending) > 0 {
			n := copy(p, c.pending)
			c.pending = c.pending[n:]
			c.mu.Unlock()
			return n, nil
		}
		if c.closed {
			c.mu.Unlock()
			return 0, io.EOF
		}
		c.mu.Unlock()

		select {
		case <-c.sess.wake:
		case <-c.done:
		}
	}
}

// feed is what the link's output handler gives the terminal. A resync resets
// what is waiting, as the terminal's screen is reset.
func (c *termConn) feed(data []byte, resync bool) {
	c.mu.Lock()
	if resync {
		c.pending = nil
	}
	c.pending = append(c.pending, data...)
	c.mu.Unlock()
	c.wake()
}

// wake tells a blocked Read there is something to take, or that it should look
// again.
func (c *termConn) wake() {
	select {
	case c.sess.wake <- struct{}{}:
	default:
	}
}

// Write takes the keys the terminal encoded. A window holding the lease sends
// them, at the epoch the hub last told it; a window that does not drops them
// here, so the shell never sees a keystroke it should not have.
func (c *termConn) Write(p []byte) (int, error) {
	if !c.sess.holds() {
		// The keys are taken and discarded: the terminal's own state stays
		// consistent, and the popover over the terminal says why nothing
		// happened.
		return len(p), nil
	}
	c.sess.link.input(c.sess.sessionID(), append([]byte(nil), p...))
	return len(p), nil
}

// Resize is how the terminal tells the hub the size of the view; the plugin
// finds it by name on the Conn.
func (c *termConn) Resize(cols, rows int) error {
	c.mu.Lock()
	c.lastCols, c.lastRows = cols, rows
	if !c.sess.holds() {
		c.mu.Unlock()
		return nil
	}
	if cols == c.syncedCols && rows == c.syncedRows {
		c.mu.Unlock()
		return nil
	}
	c.syncedCols, c.syncedRows = cols, rows
	c.mu.Unlock()

	c.sess.link.resize(c.sess.sessionID(), cols, rows)
	return nil
}

// syncSize notifies the hub of the terminal's actual size once the lease is held.
// If cols/rows are 0, it falls back to the last recorded size from Resize.
// It avoids redundant resize frames if the size is already synced.
func (c *termConn) syncSize(cols, rows int) {
	if c == nil || c.sess == nil || c.sess.link == nil {
		return
	}
	c.mu.Lock()
	if cols <= 0 || rows <= 0 {
		cols, rows = c.lastCols, c.lastRows
	} else {
		c.lastCols, c.lastRows = cols, rows
	}
	if cols <= 0 || rows <= 0 || (cols == c.syncedCols && rows == c.syncedRows) {
		c.mu.Unlock()
		return
	}
	if !c.sess.holds() {
		c.mu.Unlock()
		return
	}
	c.syncedCols, c.syncedRows = cols, rows
	c.mu.Unlock()

	c.sess.link.resize(c.sess.sessionID(), cols, rows)
}

// Close detaches this view. It never ends the session: the shell keeps running
// for whatever else is attached, exactly as a client disconnecting does.
func (c *termConn) Close() error {
	c.once.Do(func() {
		c.mu.Lock()
		c.closed = true
		c.mu.Unlock()
		close(c.done)
		c.wake()
	})
	return nil
}

// lease is what the window knows about the session's lease, for the chrome
// around the terminal.
func (c *termConn) lease() (role, controller string, epoch uint64) {
	s := c.sess
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.role, s.controller, s.epoch
}

// claiming asks for the lease, stealing it from a live holder when asked.
func (c *termConn) claiming(steal bool) {
	c.sess.link.claim(c.sess.sessionID(), steal)
}

// releasing gives the lease up without giving up the session: the shell keeps
// running and this window keeps watching it, it just stops being the one that
// types into it.
func (c *termConn) releasing() {
	c.sess.link.release(c.sess.sessionID())
}

// holderName is who holds the lease, or "" when this window does.
func (c *termConn) holderName() string { return c.sess.holder() }

// roleIsController reports whether a role from a role frame is the lease.
func roleIsController(role string) bool { return role == protocol.RoleController }

// pendingText is what has arrived and not yet been read, for a test that wants
// to see what the terminal has been given.
func (c *termConn) pendingText() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return string(c.pending)
}
