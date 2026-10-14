// Package native joins a term-fleet session to a native view: mygo's terminal
// plugin draws the session, and this file is the wire between the two.
//
// It exists because the desktop window shows sessions itself, without a
// webview. The session it draws is not a copy — it is the session the browser
// and phone attach to, so a key typed in the window and a key typed in a
// browser land in the same shell.
package native

import (
	"io"
	"sync"

	"github.com/CarlOpenLab/term-fleet/internal/session"
)

// Conn adapts a session to the terminal plugin's Conn contract: the terminal
// reads the process output from it, writes what the user types into it, and,
// because it has a Resize method, tells the PTY the size of the view.
type Conn struct {
	s *session.Session

	mu      sync.Mutex
	off     int64  // the ring offset this view has read up to
	pending []byte // output read from the ring but not yet handed over

	once   sync.Once
	closed chan struct{}
}

// NewConn builds the view end of a session. One session may have several; each
// keeps its own place in the output, as each client does over the wire.
func NewConn(s *session.Session) *Conn {
	return &Conn{s: s, closed: make(chan struct{})}
}

// Read hands over the session's output, blocking until there is some. It
// reports io.EOF once the process is gone and its output has been drained, so
// a view learns of the end the same way a client does.
func (c *Conn) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for {
		if n, ok := c.drain(p); ok {
			return n, nil
		}
		// Nothing buffered and nothing in the ring: the process is either
		// quiet or gone. Gone is only final once the ring is empty, so the
		// last write is not lost to the exit.
		if !c.s.Alive() {
			return 0, io.EOF
		}
		select {
		case <-c.s.Changes():
		case <-c.s.Exited():
		case <-c.closed:
			return 0, io.EOF
		}
	}
}

// drain copies the next output bytes into p, from the leftovers of an earlier
// read first and then from the ring. It reports false when there is nothing to
// give without waiting.
func (c *Conn) drain(p []byte) (int, bool) {
	c.mu.Lock()
	if n := copy(p, c.pending); n > 0 {
		c.pending = c.pending[n:]
		c.mu.Unlock()
		return n, true
	}
	off := c.off
	c.mu.Unlock()

	// The view's own cursor, not the session's, so a view that falls behind
	// keeps its place instead of skipping ahead; a ring that has dropped
	// bytes it never read says so in truncated, which a view cannot undo and
	// so ignores.
	data, from, _ := c.s.Ring().ReadFrom(off)
	if len(data) == 0 {
		return 0, false
	}
	n := copy(p, data)

	c.mu.Lock()
	c.off = from + int64(len(data))
	if n < len(data) {
		c.pending = append(c.pending[:0], data[n:]...)
	}
	c.mu.Unlock()
	return n, true
}

// Write takes the keys the terminal encoded and gives them to the process,
// the same path a browser keypress takes.
func (c *Conn) Write(p []byte) (int, error) {
	if err := c.s.Write(p); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Resize is how the terminal tells the PTY the size of the view; the plugin
// finds it by name on the Conn.
func (c *Conn) Resize(cols, rows int) error {
	if cols <= 0 || rows <= 0 {
		return nil
	}
	return c.s.Resize(cols, rows)
}

// Close detaches this view. It never ends the session: the shell keeps running
// for whatever else is attached, exactly as a client disconnecting does over
// the wire.
func (c *Conn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}
