package native

import (
	"bytes"
	"io"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/CarlOpenLab/term-fleet/internal/session"
)

// shellLine returns a line to type into whichever shell the platform runs, so
// the tests exercise a real PTY on both Unix and Windows.
func shellLine(unix, windows string) string {
	if runtime.GOOS == "windows" {
		return windows
	}
	return unix
}

func startSession(t *testing.T) *session.Session {
	t.Helper()
	s, err := session.Start("test", 80, 24, t.TempDir(), "")
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// readUntil reads from c until want appears, and fails the test on timeout.
func readUntil(t *testing.T, c *Conn, want string, d time.Duration) string {
	t.Helper()
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		tmp := make([]byte, 4096)
		for {
			n, err := c.Read(tmp)
			buf.Write(tmp[:n])
			if err != nil || strings.Contains(buf.String(), want) {
				done <- buf.String()
				return
			}
		}
	}()
	select {
	case out := <-done:
		return out
	case <-time.After(d):
		t.Fatalf("timed out waiting for %q", want)
		return ""
	}
}

// TestConnRoundTrip is the point of the package: what the process writes comes
// out of Read, and what Write hands over reaches the process.
func TestConnRoundTrip(t *testing.T) {
	s := startSession(t)
	c := NewConn(s)
	t.Cleanup(func() { _ = c.Close() })

	if _, err := c.Write([]byte(shellLine("printf 'TFMARK\\n'\n", "echo TFMARK\r\n"))); err != nil {
		t.Fatalf("write: %v", err)
	}
	if out := readUntil(t, c, "TFMARK", 15*time.Second); !strings.Contains(out, "TFMARK") {
		t.Fatalf("read %q, want it to contain TFMARK", out)
	}
}

// TestConnReportsEOFWhenSessionEnds pins that a view learns of the end the way
// a client does, and only after the last write has been handed over.
func TestConnReportsEOFWhenSessionEnds(t *testing.T) {
	s := startSession(t)
	c := NewConn(s)
	t.Cleanup(func() { _ = c.Close() })

	if _, err := c.Write([]byte(shellLine("printf 'TFBYE\\n'; exit\n", "echo TFBYE & exit\r\n"))); err != nil {
		t.Fatalf("write: %v", err)
	}

	deadline := time.Now().Add(15 * time.Second)
	var got bytes.Buffer
	tmp := make([]byte, 4096)
	for {
		n, err := c.Read(tmp)
		got.Write(tmp[:n])
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("no EOF after the process exited; read %q", got.String())
		}
	}
	if !strings.Contains(got.String(), "TFBYE") {
		t.Fatalf("read %q, want the final output before EOF", got.String())
	}
}

// TestConnCloseDetachesWithoutEndingSession pins that closing a view is a
// detach, not a kill: the window closing a tab must not take the shell away
// from a browser that is still attached.
func TestConnCloseDetachesWithoutEndingSession(t *testing.T) {
	s := startSession(t)
	c := NewConn(s)
	if err := c.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if !s.Alive() {
		t.Fatal("session ended when its view closed")
	}

	// A closed view reports EOF rather than blocking forever.
	if n, err := c.Read(make([]byte, 8)); err != io.EOF {
		t.Fatalf("Read after Close = (%d, %v), want (0, EOF)", n, err)
	}

	// And the session is still usable by the next view.
	next := NewConn(s)
	t.Cleanup(func() { _ = next.Close() })
	if _, err := next.Write([]byte(shellLine("printf 'TFNEXT\\n'\n", "echo TFNEXT\r\n"))); err != nil {
		t.Fatalf("write on second view: %v", err)
	}
	if out := readUntil(t, next, "TFNEXT", 15*time.Second); !strings.Contains(out, "TFNEXT") {
		t.Fatalf("second view read %q, want it to contain TFNEXT", out)
	}
}

// TestConnViewKeepsItsOwnPlace pins that two views of one session each get the
// whole output, not a split of it.
func TestConnViewKeepsItsOwnPlace(t *testing.T) {
	s := startSession(t)
	first := NewConn(s)
	t.Cleanup(func() { _ = first.Close() })

	if _, err := first.Write([]byte(shellLine("printf 'TFSHARED\\n'\n", "echo TFSHARED\r\n"))); err != nil {
		t.Fatalf("write: %v", err)
	}
	if out := readUntil(t, first, "TFSHARED", 15*time.Second); !strings.Contains(out, "TFSHARED") {
		t.Fatalf("first view read %q", out)
	}

	// A view attached afterwards starts at the beginning of the ring, so the
	// line the first view already saw is still there for it.
	second := NewConn(s)
	t.Cleanup(func() { _ = second.Close() })
	if out := readUntil(t, second, "TFSHARED", 15*time.Second); !strings.Contains(out, "TFSHARED") {
		t.Fatalf("second view read %q, want the same output from the start", out)
	}
}

// TestConnResizeReachesTheProcess pins that the view's size is the PTY's size.
func TestConnResizeReachesTheProcess(t *testing.T) {
	s := startSession(t)
	c := NewConn(s)
	t.Cleanup(func() { _ = c.Close() })

	if err := c.Resize(100, 30); err != nil {
		t.Fatalf("resize: %v", err)
	}
	if cols, rows := s.Size(); cols != 100 || rows != 30 {
		t.Fatalf("session size = %dx%d, want 100x30", cols, rows)
	}
	// A nonsense size is ignored rather than pushed to the PTY.
	if err := c.Resize(0, 0); err != nil {
		t.Fatalf("resize to zero: %v", err)
	}
	if cols, rows := s.Size(); cols != 100 || rows != 30 {
		t.Fatalf("session size after a zero resize = %dx%d, want it unchanged", cols, rows)
	}
}
