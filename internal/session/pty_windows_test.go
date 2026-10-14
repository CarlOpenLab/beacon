//go:build windows

package session

import (
	"os"
	"strings"
	"testing"
	"time"
)

// These run on Windows only, and they are the reason the Windows build is worth
// claiming at all: ConPTY either produces a working terminal or it produces
// nothing, and no amount of cross-compiling proves which.

func windowsCmdShell() ptySpec {
	return ptySpec{
		path: `C:\Windows\System32\cmd.exe`,
		env:  append(os.Environ(), "TERM=xterm-256color"),
	}
}

func TestConPTYRoundTrip(t *testing.T) {
	if !conPTYAvailable() {
		t.Skip("ConPTY is not available on this Windows build")
	}
	p, err := startPTY(windowsCmdShell(), 80, 24)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer p.Close()

	if pid := p.PID(); pid == 0 {
		t.Fatal("PID() = 0, want the console's process id")
	}

	found := make(chan string, 1)
	go func() {
		var seen strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := p.Read(buf)
			if n > 0 {
				seen.Write(buf[:n])
				if strings.Contains(seen.String(), "WINDOWS_PTY_OK") {
					found <- seen.String()
					return
				}
			}
			if err != nil {
				found <- seen.String()
				return
			}
		}
	}()

	// ConPTY wants CRLF: the console host treats a bare LF as a literal newline,
	// which leaves the command sitting in the input buffer.
	if _, err := p.Write([]byte("echo WINDOWS_PTY_OK\r\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	select {
	case got := <-found:
		if !strings.Contains(got, "WINDOWS_PTY_OK") {
			t.Fatalf("the command never came back; saw %q", got)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for output")
	}
}

func TestConPTYResize(t *testing.T) {
	if !conPTYAvailable() {
		t.Skip("ConPTY is not available on this Windows build")
	}
	p, err := startPTY(windowsCmdShell(), 80, 24)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer p.Close()

	// Shrinking then growing exercises both directions; a resize that silently
	// fails shows up in the client as a terminal that cannot reflow.
	for _, size := range [][2]int{{40, 12}, {120, 40}} {
		if err := p.Resize(size[0], size[1]); err != nil {
			t.Fatalf("resize to %dx%d: %v", size[0], size[1], err)
		}
	}
}

func TestConPTYExitCode(t *testing.T) {
	if !conPTYAvailable() {
		t.Skip("ConPTY is not available on this Windows build")
	}
	spec := windowsCmdShell()
	spec.args = []string{"/c", "exit", "7"}
	p, err := startPTY(spec, 80, 24)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer p.Close()

	// Drain, or a full output pipe blocks the child before it can exit.
	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := p.Read(buf); err != nil {
				return
			}
		}
	}()

	done := make(chan int, 1)
	go func() {
		code, err := p.Wait()
		if err != nil {
			t.Logf("wait: %v", err)
		}
		done <- code
	}()

	select {
	case code := <-done:
		if code != 7 {
			t.Fatalf("exit code = %d, want 7", code)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the process never exited")
	}
}

func TestQuoteArg(t *testing.T) {
	// The cases that matter are the ones where a naive quoting produces a
	// command line that CreateProcessW parses back differently.
	cases := []struct{ in, want string }{
		{`C:\Windows\System32\cmd.exe`, `C:\Windows\System32\cmd.exe`},
		{`C:\Program Files\Git\bin\bash.exe`, `"C:\Program Files\Git\bin\bash.exe"`},
		{``, `""`},
		{`a"b`, `"a\"b"`},
		{`a\`, `a\`},
		{`a b\`, `"a b\\"`},
		{`a b\"`, `"a b\\\""`},
	}
	for _, c := range cases {
		if got := quoteArg(c.in); got != c.want {
			t.Errorf("quoteArg(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
