package session

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/CarlOpenLab/term-fleet/internal/protocol"
	"github.com/CarlOpenLab/term-fleet/internal/status"
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
	// st follows what the session is doing, and statusChanges carries its
	// changes to whoever broadcasts them.
	st            *status.Tracker
	statusChanges chan struct{}

	mu       sync.Mutex
	pty      ptyProcess
	ptyOnce  sync.Once
	cols     int
	rows     int
	title    string
	shell    string
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
	dir := startDir(cwd)
	spec := ptySpec{
		path: path,
		args: args,
		env:  shellEnv(),
		dir:  dir,
	}
	pty, err := startPTY(spec, cols, rows)
	if err != nil {
		return nil, fmt.Errorf("start pty: %w", err)
	}
	if dir == "" {
		// No home directory to open in: the shell landed wherever this process
		// is, which is what the summary then reports.
		dir, _ = os.Getwd()
	}
	s := &Session{
		ID:            id,
		CreatedAt:     time.Now(),
		ring:          NewRing(ringCapacity),
		changes:       make(chan struct{}, 1),
		exited:        make(chan struct{}),
		statusChanges: make(chan struct{}, 1),
		pty:           pty,
		cols:          cols,
		rows:          rows,
		cwd:           dir,
		pid:           pty.PID(),
		alive:         true,
		shell:         path,
	}
	s.st = status.New(s.notifyStatus)
	go s.pump()
	go s.pollForeground()
	go s.wait()
	return s, nil
}

// startDir is where a new shell opens when the client did not say where: this
// machine's home directory. The process's own working directory is not a place
// anyone chose — the desktop app is started by the window server, where it is
// the root of the filesystem — and a terminal that opens at / is a terminal to
// leave again. A client that did say where keeps its answer, and a machine with
// no home directory keeps the old behaviour.
func startDir(cwd string) string {
	if cwd != "" {
		return cwd
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

// StatusChanges is signalled whenever what the session is doing changes.
func (s *Session) StatusChanges() <-chan struct{} { return s.statusChanges }

// pollForeground asks what is running in the pane, once a second, and tells the
// status tracker when it changes: the chip on a card's header, and the one
// signal an agent that reports nothing still leaves behind.
func (s *Session) pollForeground() {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-s.exited:
			return
		case <-tick.C:
			if s.st == nil {
				continue
			}
			s.st.SetProcess(s.Foreground())
		}
	}
}

// Foreground is the program in the foreground of the pane, or "" when the
// platform cannot say.
func (s *Session) Foreground() string {
	s.mu.Lock()
	pty, alive := s.pty, s.alive
	s.mu.Unlock()
	if !alive || pty == nil {
		return ""
	}
	name, _ := pty.Foreground()
	return name
}

func (s *Session) notifyStatus() {
	select {
	case s.statusChanges <- struct{}{}:
	default:
	}
}

// Snapshot is what the session is doing now.
func (s *Session) Snapshot() status.Snapshot {
	if s.st == nil {
		return status.Snapshot{State: status.Idle, Progress: -1}
	}
	return s.st.Snapshot()
}

// Viewed marks the session read, which a client attaching to it does.
func (s *Session) Viewed() {
	if s.st != nil {
		s.st.Viewed()
	}
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

// LiveCwd is the directory the shell stands in now, which is not the directory
// it was started in: a shell that has been cd'd elsewhere since is in that
// other place, and a client opening a second shell beside it wants that place.
// It falls back to the directory the session was started in when the platform
// cannot say where a process stands, and to "" when there is neither.
func (s *Session) LiveCwd() string {
	s.mu.Lock()
	pid, started := s.pid, s.cwd
	s.mu.Unlock()
	if pid <= 0 {
		return started
	}
	if live := liveCwd(pid); live != "" {
		return live
	}
	return started
}

// Summary renders the session into one row of the client-facing list.
func (s *Session) Summary() protocol.Summary {
	s.mu.Lock()
	defer s.mu.Unlock()
	// A name given with a rename is the name; when there is none, the title the
	// program set on the terminal (OSC 0/2) is what the row should read — that
	// is how an agent names the task it is on. An empty title from both leaves
	// the clients their own fallback.
	title := s.title
	sum := protocol.Summary{
		SessionID: s.ID,
		PID:       s.pid,
		Cwd:       s.cwd,
		Title:     title,
		Shell:     s.shell,
		Cols:      s.cols,
		Rows:      s.rows,
		Alive:     s.alive,
		Cursor:    s.ring.End(),
		CreatedAt: s.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	if s.st != nil {
		snap := s.st.Snapshot()
		if title == "" {
			sum.Title = snap.Title
		}
		sum.Status = string(snap.State)
		sum.Unread = snap.Unread
		sum.Message = snap.Message
		sum.Kind = snap.Kind
		sum.Progress = snap.Progress
		sum.Process = snap.Process
		if !snap.Since.IsZero() {
			sum.Since = snap.Since.UnixMilli()
		}
	}
	return sum
}

// Write feeds bytes to the process's stdin.
func (s *Session) Write(p []byte) error {
	s.mu.Lock()
	pty, alive := s.pty, s.alive
	s.mu.Unlock()
	if !alive {
		return ErrClosed
	}
	if s.st != nil && len(p) > 0 {
		s.st.Typed()
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
	pid, alive := s.pid, s.alive
	s.mu.Unlock()
	if alive {
		_ = killProcess(pid)
	}
	return s.closePTY()
}

// closePTY releases the PTY exactly once, whichever of the two owners gets
// there first: wait() after the process exits on its own, or Close() when the
// session is shut down. Closing it twice is not a harmless second error — on
// Windows one Close() call releases six HANDLEs, and the runtime may have
// handed a reused value to someone else in the meantime, which turns a double
// close into corruption rather than an error.
func (s *Session) closePTY() error {
	var err error
	s.ptyOnce.Do(func() { err = s.pty.Close() })
	return err
}

func (s *Session) pump() {
	buf := make([]byte, 32*1024)
	for {
		n, err := s.pty.Read(buf)
		if n > 0 {
			s.ring.Write(buf[:n])
			if s.st != nil {
				s.st.Observe(buf[:n])
			}
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
	if s.st != nil {
		s.st.Exited()
		s.notifyStatus()
	}
	_ = s.closePTY()
	close(s.exited)
}
