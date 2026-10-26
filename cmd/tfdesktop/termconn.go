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

	"github.com/egoist/mygo/plugins/terminal"

	"github.com/CarlOpenLab/beacon/internal/diag"
	"github.com/CarlOpenLab/beacon/internal/protocol"
)

// termConn is one terminal's end of one session.
type termConn struct {
	sess *session

	// machine is the roster id this session's shell runs on, for the log lines
	// below: a session id is counted per machine ("s1" on every one of them), so
	// the log needs both to name a pane.
	machine string

	mu         sync.Mutex
	pending    []byte
	closed     bool
	once       sync.Once
	done       chan struct{}
	lastCols   int
	lastRows   int
	syncedCols int
	syncedRows int

	// screen is the view this conn feeds, for the size the screen lays out at.
	// It is nil until that terminal is made: this conn is its Conn, so it
	// cannot be handed over before.
	screen *terminal.Terminal

	// fed and read are the two ends of this conn's life: what the link gave the
	// terminal, and what the terminal took. A blank pane whose conn says
	// fed>0, read=0 is a view that is not draining; one that says fed=0 is a
	// replay that never arrived — the two answers this log exists to separate.
	fed      int64
	read     int64
	logged   int64 // fed at the last summary line, so the next one waits a while
	noReader int64 // frames fed after Close, counted for the warning above
}

func newTermConn(s *session) *termConn {
	return &termConn{sess: s, done: make(chan struct{})}
}

// label names this conn's shell for a log line.
func (c *termConn) label() (string, string) {
	return c.machine, c.sess.sessionID()
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
			first := c.read == 0
			c.read += int64(n)
			c.mu.Unlock()
			if first {
				machine, sessionID := c.label()
				diag.Debugf("term", "terminal read first machine=%s session=%s bytes=%d", machine, sessionID, n)
			}
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
	machine, sessionID := c.label()
	c.mu.Lock()
	if resync {
		c.pending = nil
	}
	first := c.fed == 0 && len(data) > 0
	c.fed += int64(len(data))
	closed := c.closed
	total := c.fed
	waiting := len(c.pending) + len(data)
	c.pending = append(c.pending, data...)
	c.mu.Unlock()

	if closed && len(data) > 0 {
		// The replay is being handed to a terminal that has been closed. This is
		// what a re-registration bug looks like from the data's side: the bytes
		// are right, the session is live, and nothing will ever draw them. Said
		// once and then every hundredth frame — the evidence matters, a line per
		// frame of a replays worth of output does not.
		c.mu.Lock()
		c.noReader++
		n := c.noReader
		c.mu.Unlock()
		if n == 1 || n%100 == 0 {
			diag.Warnf("term", "output fed to a closed conn machine=%s session=%s bytes=%d total=%d count=%d",
				machine, sessionID, len(data), total, n)
		}
	} else if first {
		diag.Infof("term", "output first machine=%s session=%s bytes=%d resync=%t", machine, sessionID, len(data), resync)
	} else if resync || total-c.logged >= 256<<10 {
		c.mu.Lock()
		c.logged = total
		c.mu.Unlock()
		diag.Debugf("term", "output machine=%s session=%s bytes=%d total=%d waiting=%d resync=%t",
			machine, sessionID, len(data), total, waiting, resync)
	}
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
//
// Holding the lease, the view is the size the shell's screen has: the screen is
// laid out at what the hub is told, so that the bytes the program writes for
// this size land here exactly as they do on the machine that runs it, and so
// that every viewer of the same shell lays them out at the same width. Not
// holding it, the view only records the size: a viewer watches a screen that
// already has a size, and changing the outline here is not changing the shell.
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
	screen := c.screen
	c.mu.Unlock()

	c.sess.link.resize(c.sess.sessionID(), cols, rows)
	if screen != nil {
		screen.Resize(cols, rows)
	}
	return nil
}

// syncSize notifies the hub of the view's actual size once the lease is held.
// If cols/rows are 0, it falls back to the last size the view reported from
// Resize, and then — before any frame has laid the view out — to the screen's
// own size, which is the best answer there is. It avoids redundant resize
// frames if the size is already synced.
func (c *termConn) syncSize(cols, rows int) {
	if c == nil || c.sess == nil || c.sess.link == nil {
		return
	}
	c.mu.Lock()
	if cols <= 0 || rows <= 0 {
		// The view's size, not the screen's: taking the lease gives the shell
		// the size of the window that now drives it, and the screen may still
		// be laid out at the size of the window that drove it before.
		cols, rows = c.lastCols, c.lastRows
	} else {
		c.lastCols, c.lastRows = cols, rows
	}
	screen := c.screen
	c.mu.Unlock()

	if cols <= 0 || rows <= 0 {
		if screen == nil {
			return
		}
		cols, rows = screen.Size()
	}
	if cols <= 0 || rows <= 0 || !c.sess.holds() {
		return
	}

	c.mu.Lock()
	if cols == c.syncedCols && rows == c.syncedRows {
		c.mu.Unlock()
		return
	}
	c.syncedCols, c.syncedRows = cols, rows
	c.mu.Unlock()

	c.sess.link.resize(c.sess.sessionID(), cols, rows)
	if screen != nil {
		// Taking the lease gives the shell this pane's size: the screen takes
		// it here, at the same moment the hub is told, so the output written
		// for the old size is not laid out at the new one.
		screen.Resize(cols, rows)
	}
}

// watchScreen hands this conn the view it feeds, which is how the screen learns
// the size it lays out at. It is set once the terminal is made: the conn is that
// terminal's Conn, so it cannot be handed over before.
func (c *termConn) watchScreen(term *terminal.Terminal) {
	c.mu.Lock()
	c.screen = term
	c.mu.Unlock()
}

// takeSize tells the hub the size of the view as the size of the shell's
// screen, even when this conn told it that same size before. Taking the lease is
// the moment the shell's size becomes this window's: the window that held it
// until now may have resized the shell, so what this conn last sent is not what
// the shell has — and a window that stayed quiet on that reasoning would leave
// the shell at the other window's width, drawn for it, in a pane of its own.
func (c *termConn) takeSize() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.syncedCols, c.syncedRows = 0, 0
	c.mu.Unlock()
	c.syncSize(0, 0)
}

// viewSize is the size the view last told this conn it fits, or 0, 0 before a
// frame has laid it out.
func (c *termConn) viewSize() (cols, rows int) {
	if c == nil {
		return 0, 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastCols, c.lastRows
}

// layOut puts the screen at the size the shell's own terminal has, and reports
// whether that took. A viewer's pane is a window onto a screen that already
// exists: the replay it is fed was written for that size, with the program's
// cursor moves measured against it, so laying it out at this pane's width
// instead would wrap lines the machine's window has whole and put every later
// redraw of the program in the wrong place.
func (c *termConn) layOut(cols, rows int) bool {
	if c == nil || cols <= 0 || rows <= 0 {
		return false
	}
	c.mu.Lock()
	screen := c.screen
	c.mu.Unlock()
	if screen == nil {
		return false
	}
	if haveCols, haveRows := screen.Size(); haveCols == cols && haveRows == rows {
		return false
	}
	screen.Resize(cols, rows)
	return true
}

// Close detaches this view. It never ends the session: the shell keeps running
// for whatever else is attached, exactly as a client disconnecting does.
func (c *termConn) Close() error {
	c.once.Do(func() {
		machine, sessionID := c.label()
		c.mu.Lock()
		c.closed = true
		fed, read, waiting := c.fed, c.read, len(c.pending)
		c.mu.Unlock()
		// What this conn was given against what it took: a closed tab that was
		// fed a replay it never read is the fingerprint of frames still routed
		// to a pane nobody draws any more.
		diag.Infof("term", "conn closed machine=%s session=%s fed=%d read=%d pending=%d",
			machine, sessionID, fed, read, waiting)
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
