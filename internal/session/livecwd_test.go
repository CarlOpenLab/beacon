//go:build !windows

package session

import (
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// The directory a shell is *in* is not the directory it was started in: a shell
// that has been cd'd elsewhere is in that other place, and a client opening a
// second shell beside it wants that place. The summary keeps the start
// directory; LiveCwd is the answer that follows the shell.
func TestLiveCwdFollowsTheShell(t *testing.T) {
	dir := t.TempDir()
	sess, err := Start("live", 80, 24, dir, "/bin/sh")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer sess.Close()

	// Before anything is typed, the live directory is the started one.
	if got, want := resolve(t, sess.LiveCwd()), resolve(t, dir); got != want {
		t.Fatalf("LiveCwd = %q, want the started directory %q", got, want)
	}

	other := t.TempDir()
	if err := sess.Write([]byte("cd " + other + "\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	want := resolve(t, other)
	deadline := time.Now().Add(10 * time.Second)
	for {
		if got := resolve(t, sess.LiveCwd()); got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("LiveCwd = %q, want the directory the shell cd'd to %q", sess.LiveCwd(), want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A session that cannot be asked — no process any more — keeps the directory it
// was started in rather than inventing one.
func TestLiveCwdFallsBackToTheStartedDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a ConPTY process cannot be asked where it stands")
	}
	dir := t.TempDir()
	sess, err := Start("gone", 80, 24, dir, "/bin/sh")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	_ = sess.Close()

	if got, want := resolve(t, sess.LiveCwd()), resolve(t, dir); got != want {
		t.Fatalf("LiveCwd = %q after the process was reaped, want the started directory %q", got, want)
	}
}

// resolve resolves symlinks so a comparison holds on a machine where the
// temporary directory is one: macOS keeps /tmp and /var as links, and the
// operating system answers with the real path.
func resolve(t *testing.T, dir string) string {
	t.Helper()
	got, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return dir
	}
	return got
}
