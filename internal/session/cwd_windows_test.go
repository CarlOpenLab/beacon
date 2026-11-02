//go:build windows

package session

import (
	"os"
	"testing"
	"time"
)

// askUntil waits for liveCwd to report want. The shell is a real process, so
// the directory changes when it gets to the command, not when the bytes are
// written; a poll is the only honest way to read it.
func askUntil(t *testing.T, sess *Session, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		got := sess.LiveCwd()
		if sameDir(got, want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("LiveCwd = %q, want %q", got, want)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// cmd keeps its process directory in step with cd, and the PEB read follows
// it: a new tab opened beside a cmd shell opens where that shell stands.
func TestLiveCwdFollowsCmd(t *testing.T) {
	start := t.TempDir()
	dir := t.TempDir()
	sess, err := Start("cmd-cwd", 80, 24, start, `C:\Windows\System32\cmd.exe`)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer sess.Close()

	if err := sess.Write([]byte("cd /d " + dir + "\r\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	askUntil(t, sess, dir)
	if got := sess.Summary().Cwd; !sameDir(got, dir) {
		t.Fatalf("Summary().Cwd = %q, want %q", got, dir)
	}
}

// PowerShell's Set-Location does not change the process's working directory,
// so the shell is started with the prompt wrapper that syncs it; without the
// wrapper the PEB would still name the start directory and a new tab would
// always open at home.
func TestLiveCwdFollowsPowerShell(t *testing.T) {
	const shell = `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`
	if _, err := os.Stat(shell); err != nil {
		t.Skipf("Windows PowerShell is not installed: %v", err)
	}
	start := t.TempDir()
	dir := t.TempDir()
	sess, err := Start("pwsh-cwd", 80, 24, start, shell)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer sess.Close()

	if err := sess.Write([]byte("Set-Location '" + dir + "'\r\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	askUntil(t, sess, dir)
}

// A path in the PEB can carry an NT namespace prefix; the answer is a plain
// path another process can be started in.
func TestTrimNTNamespace(t *testing.T) {
	cases := []struct{ in, want string }{
		{`C:\work`, `C:\work`},
		{`\??\C:\work`, `C:\work`},
		{`\\?\C:\work`, `C:\work`},
		{`\\server\share`, `\\server\share`},
	}
	for _, c := range cases {
		if got := trimNTNamespace(c.in); got != c.want {
			t.Errorf("trimNTNamespace(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
