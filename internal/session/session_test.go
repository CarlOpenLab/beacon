package session

import (
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
