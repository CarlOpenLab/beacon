package session

import (
	"io"
	"sync"
	"testing"
	"time"
)

// fakePTY counts how many times it is closed and lets a test hold Wait open.
type fakePTY struct {
	mu       sync.Mutex
	closes   int
	exited   chan struct{} // closed by the test to let Wait return
	readDone chan struct{} // closed by the test to release Read
}

func (p *fakePTY) Read(b []byte) (int, error) {
	<-p.readDone
	return 0, io.EOF
}

func (p *fakePTY) Write(b []byte) (int, error) { return len(b), nil }

func (p *fakePTY) Close() error {
	p.mu.Lock()
	p.closes++
	p.mu.Unlock()
	return nil
}

func (p *fakePTY) Resize(cols, rows int) error { return nil }
func (p *fakePTY) Wait() (int, error)          { <-p.exited; return 0, nil }
func (p *fakePTY) PID() int                    { return 0 }

func (p *fakePTY) closeCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closes
}

// A session has two owners of its PTY: wait() releases it when the process
// exits on its own, and Close() releases it when the session is shut down.
// Both run in the ordinary end-of-life path, so the PTY must come back exactly
// once. On Windows a second Close() hands back HANDLEs the runtime may already
// have reused, which corrupts the heap instead of returning an error.
func TestCloseAndWaitReleaseThePTYOnce(t *testing.T) {
	fp := &fakePTY{exited: make(chan struct{}), readDone: make(chan struct{})}
	s := newBareSession(fp)

	go s.wait()

	// Shut the session down while the process is still "running"...
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// ...and let the process exit on its own as well.
	close(fp.exited)
	select {
	case <-s.exited:
	case <-time.After(2 * time.Second):
		t.Fatal("wait() never finished")
	}

	if got := fp.closeCount(); got != 1 {
		t.Fatalf("PTY closed %d times, want exactly 1: the second close is what corrupts on Windows", got)
	}

	// A repeated shutdown must stay a no-op too.
	_ = s.Close()
	if got := fp.closeCount(); got != 1 {
		t.Fatalf("PTY closed %d times after a second Close, want 1", got)
	}
}

// newBareSession builds a session around an injected PTY without starting a
// process. alive is false and pid is 0 so the test's Close() never signals a
// real process group.
func newBareSession(pty ptyProcess) *Session {
	return &Session{
		ID:        "s1",
		CreatedAt: time.Now(),
		ring:      NewRing(1024),
		changes:   make(chan struct{}, 1),
		exited:    make(chan struct{}),
		pty:       pty,
		cols:      80,
		rows:      24,
		alive:     false,
	}
}
