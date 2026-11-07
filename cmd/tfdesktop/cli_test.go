package main

// The command line is how the app is summoned from a shell: "beacon ." opens
// a tab where the shell stands, whether Beacon was already running or not. A
// launch that names nothing behaves as it always has.

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CarlOpenLab/beacon/internal/protocol"
)

func TestParseOptionsTakesTheFirstPositionalAsTheDirectory(t *testing.T) {
	o := parseOptions([]string{"-shell", "pwsh", ".", "ignored"}, io.Discard, flag.ContinueOnError)
	if o.shell != "pwsh" {
		t.Errorf("shell = %q, want pwsh", o.shell)
	}
	if o.dir != "." {
		t.Errorf("dir = %q, want the first positional argument", o.dir)
	}
}

func TestParseOptionsReadsAnEmptyLine(t *testing.T) {
	o := parseOptions(nil, io.Discard, flag.ContinueOnError)
	if o.dir != "" || o.cwd != "" || o.shell != "" {
		t.Fatalf("an empty line carried %+v", o)
	}
}

// A relative directory belongs to the shell that typed it, not to the app's
// own working directory — the window server starts the app somewhere nobody
// chose.
func TestResolveDirJoinsRelativePathsToTheCallersDirectory(t *testing.T) {
	wd := t.TempDir()
	if got, want := resolveDir(".", wd), filepath.Clean(wd); got != want {
		t.Errorf(`resolveDir(".", %q) = %q, want %q`, wd, got, want)
	}
	if got, want := resolveDir("sub", wd), filepath.Join(wd, "sub"); got != want {
		t.Errorf("resolveDir(%q) = %q, want %q", "sub", got, want)
	}
	abs := t.TempDir()
	if got := resolveDir(abs, wd); got != abs {
		t.Errorf("an absolute path was rewritten: %q", got)
	}
	if got := resolveDir("", wd); got != "" {
		t.Errorf(`resolveDir("") = %q, want empty`, got)
	}
}

// "beacon main.go" opens a shell beside the file, which is how a person asks
// for a terminal in a project after selecting one file in it.
func TestResolveDirNamesAFilesDirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := resolveDir("main.go", dir); got != dir {
		t.Errorf("resolveDir(main.go) = %q, want %q", got, dir)
	}
}

func TestForwardedDirReadsThePositionalArgumentAndCwd(t *testing.T) {
	wd := t.TempDir()
	if got, want := forwardedDir([]string{"."}, wd), filepath.Clean(wd); got != want {
		t.Errorf(`forwardedDir(".") = %q, want %q`, got, want)
	}
	if got := forwardedDir(nil, wd); got != "" {
		t.Errorf("a line with no argument asked for %q", got)
	}
	dir := t.TempDir()
	if got := forwardedDir([]string{"-cwd", dir}, wd); got != dir {
		t.Errorf("forwardedDir(-cwd) = %q, want %q", got, dir)
	}
	if got, want := forwardedDir([]string{"-cwd", dir, "."}, wd), filepath.Clean(wd); got != want {
		t.Errorf("a positional argument lost to -cwd: %q", got)
	}
}

// A command line handed over by a second launch opens a tab where the shell
// that typed it stood, whichever machine the window happens to be showing.
func TestHandleSecondInstanceOpensATabInTheCallersDirectory(t *testing.T) {
	f := newTestFleet(t)
	w, _ := newTestShellWindow(t)
	w.fleet = f
	w.local = openLocalLink(f.Hub())
	t.Cleanup(func() { w.local.close("test done") })
	waitForPaste(t, "the local link", func() bool { return w.local.online() })

	creates := make(chan protocol.Msg, 1)
	w.local.sendMu.Lock()
	send := w.local.send
	w.local.send = func(m protocol.Msg) bool {
		if m.T == protocol.CTCreate {
			creates <- m
			return true
		}
		if send == nil {
			return true
		}
		return send(m)
	}
	w.local.sendMu.Unlock()

	dir := t.TempDir()
	w.handleSecondInstance([]string{"."}, dir)

	select {
	case m := <-creates:
		if m.Cwd != dir {
			t.Fatalf("the shell was asked to open in %q, want %q", m.Cwd, dir)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no shell was asked for")
	}
}

// A line that names nothing only brings the window forward: the tab strip is
// left as it was.
func TestHandleSecondInstanceWithoutADirectoryOpensNoTab(t *testing.T) {
	f := newTestFleet(t)
	w, _ := newTestShellWindow(t)
	w.fleet = f
	w.local = openLocalLink(f.Hub())
	t.Cleanup(func() { w.local.close("test done") })
	waitForPaste(t, "the local link", func() bool { return w.local.online() })

	open := len(w.tabs)
	w.handleSecondInstance(nil, t.TempDir())
	if got := len(w.tabs); got != open {
		t.Fatalf("a line with no directory opened %d tabs", got-open)
	}
}

func TestNewTabAtIgnoresAnEmptyDirectory(t *testing.T) {
	f := newTestFleet(t)
	w, _ := newTestShellWindow(t)
	w.fleet = f
	w.local = openLocalLink(f.Hub())
	t.Cleanup(func() { w.local.close("test done") })

	w.newTabAt("")
	if live := f.Hub().Sessions(); len(live) != 0 {
		t.Fatalf("an empty directory opened %d shells", len(live))
	}
}
