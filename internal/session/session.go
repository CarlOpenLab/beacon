package session

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/CarlOpenLab/term-fleet/internal/protocol"
)

var (
	// ErrClosed is returned when a session's process is already gone.
	ErrClosed = errors.New("session: closed")
	// ErrBadSize is returned for a resize that would hand the PTY a zero or
	// negative dimension. Zero is not a valid terminal size; programs like vim
	// and htop read it as "the window disappeared" and repaint badly on the
	// way back. See PLAN.md section 2.
	ErrBadSize = errors.New("session: cols and rows must be positive")
)

// ringCapacity bounds the per-session scrollback kept for reconnecting
// clients and for TUI applications that repaint the whole screen.
const ringCapacity = 256 * 1024

// Session owns one PTY process and the output history a client can rewind
// into. It has no notion of clients: attach/detach lives in the hub, so a
// session outlives every view of it.
type Session struct {
	ID        string
	CreatedAt time.Time

	ring    *Ring
	changes chan struct{} // coalesced "there is new output" nudge
	exited  chan struct{} // closed once the process is reaped

	mu       sync.Mutex
	pty      ptyProcess
	cols     int
	rows     int
	title    string
	cwd      string
	pid      int
	alive    bool
	exitCode int
}

// Start launches shell behind a fresh PTY of the given size.
func Start(id string, cols, rows int, cwd, shell string) (*Session, error) {
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	path, args := shellCommand(shell)
	spec := ptySpec{
		path: path,
		args: args,
		env:  append(os.Environ(), "TERM=xterm-256color"),
		dir:  cwd,
	}
	pty, err := startPTY(spec, cols, rows)
	if err != nil {
		return nil, fmt.Errorf("start pty: %w", err)
	}
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	s := &Session{
		ID:        id,
		CreatedAt: time.Now(),
		ring:      NewRing(ringCapacity),
		changes:   make(chan struct{}, 1),
		exited:    make(chan struct{}),
		pty:       pty,
		cols:      cols,
		rows:      rows,
		cwd:       cwd,
		pid:       pty.PID(),
		alive:     true,
	}
	go s.pump()
	go s.wait()
	return s, nil
}

// Changes is signalled whenever new output lands in the ring. Signals are
// coalesced: a reader that wakes up reads everything up to the current end of
// the stream, so a dropped signal costs nothing.
func (s *Session) Changes() <-chan struct{} { return s.changes }

// Exited is closed once the process has been reaped and its exit code stored.
// Reading the ring after this is safe and final.
func (s *Session) Exited() <-chan struct{} { return s.exited }

// Ring exposes the output history.
func (s *Session) Ring() *Ring { return s.ring }

func (s *Session) Alive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.alive
}

func (s *Session) ExitCode() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exitCode
}

func (s *Session) Size() (cols, rows int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cols, s.rows
}

func (s *Session) PID() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pid
}

func (s *Session) SetTitle(title string) {
	s.mu.Lock()
	s.title = title
	s.mu.Unlock()
}

// Summary renders the session into one row of the client-facing list.
func (s *Session) Summary() protocol.Summary {
	s.mu.Lock()
	defer s.mu.Unlock()
	return protocol.Summary{
		SessionID: s.ID,
		PID:       s.pid,
		Cwd:       s.cwd,
		Title:     s.title,
		Cols:      s.cols,
		Rows:      s.rows,
		Alive:     s.alive,
		Cursor:    s.ring.End(),
		CreatedAt: s.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
}

// Write feeds bytes to the process's stdin.
func (s *Session) Write(p []byte) error {
	s.mu.Lock()
	pty, alive := s.pty, s.alive
	s.mu.Unlock()
	if !alive {
		return ErrClosed
	}
	_, err := pty.Write(p)
	return err
}

// Resize changes the PTY window. The size is recorded even if the kernel call
// fails, so the reported size stays the size clients asked for.
func (s *Session) Resize(cols, rows int) error {
	if cols <= 0 || rows <= 0 {
		return ErrBadSize
	}
	s.mu.Lock()
	if !s.alive {
		s.mu.Unlock()
		return ErrClosed
	}
	s.cols, s.rows = cols, rows
	pty := s.pty
	s.mu.Unlock()
	return pty.Resize(cols, rows)
}

// Close kills the process and its children. The session is forgotten by the
// manager once wait() reaps it.
func (s *Session) Close() error {
	s.mu.Lock()
	pty, pid, alive := s.pty, s.pid, s.alive
	s.mu.Unlock()
	if alive {
		_ = killProcess(pid)
	}
	return pty.Close()
}

func (s *Session) pump() {
	buf := make([]byte, 32*1024)
	for {
		n, err := s.pty.Read(buf)
		if n > 0 {
			s.ring.Write(buf[:n])
			s.notify()
		}
		if err != nil {
			return
		}
	}
}

func (s *Session) notify() {
	select {
	case s.changes <- struct{}{}:
	default:
	}
}

func (s *Session) wait() {
	code, err := s.pty.Wait()
	if err != nil {
		code = -1
	}
	s.mu.Lock()
	s.exitCode = code
	s.alive = false
	s.mu.Unlock()
	_ = s.pty.Close()
	close(s.exited)
}
