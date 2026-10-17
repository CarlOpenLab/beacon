package session

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

// A session has to report the program it runs. Clients label a tab with it when
// the shell has not set a title of its own, which is what turns a bare "s1"
// into "zsh 1" or "PowerShell 1".
func TestSummaryReportsTheShell(t *testing.T) {
	sess, err := Start("s1", 80, 24, "", "")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer sess.Close()

	shell := sess.Summary().Shell
	if shell == "" {
		t.Fatal("the summary does not report which shell the session runs")
	}
	if !strings.HasPrefix(shell, "/") && !looksLikeWindowsPath(shell) {
		t.Errorf("Shell = %q, want a path (or a program name)", shell)
	}
}

// An explicit shell must be the one reported, not the default.
func TestSummaryReportsAnExplicitShell(t *testing.T) {
	shell := "sh"
	if runtime.GOOS == "windows" {
		shell = "cmd.exe"
	}
	sess, err := Start("s2", 80, 24, "", shell)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer sess.Close()

	if got := sess.Summary().Shell; got != shell {
		t.Fatalf("Shell = %q, want %q", got, shell)
	}
}

func looksLikeWindowsPath(path string) bool {
	return len(path) > 1 && path[1] == ':'
}

// A shell with no directory to open in starts at home. Where the process happens
// to have been started is not a place a person chose — the desktop app is
// started by the window server, where it is the root of the filesystem — and a
// terminal that opens at / is a terminal to leave again.
func TestANewShellOpensAtHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory to open in")
	}
	sess, err := Start("s3", 80, 24, "", "")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer sess.Close()

	if got := sess.Summary().Cwd; got != home {
		t.Fatalf("Cwd = %q, want this machine's home directory %q", got, home)
	}
}

// A client that says where gets what it asked for: the default is a default, not
// an override.
func TestANewShellKeepsTheDirectoryItWasGiven(t *testing.T) {
	dir := t.TempDir()
	sess, err := Start("s4", 80, 24, dir, "")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer sess.Close()

	if got := sess.Summary().Cwd; got != dir {
		t.Fatalf("Cwd = %q, want the directory the client asked for %q", got, dir)
	}
}
