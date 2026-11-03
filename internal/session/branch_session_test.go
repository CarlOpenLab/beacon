//go:build !windows

package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The summary carries the branch of the directory the shell stands in now:
// the sidebar's second line is the checkout the person is working in, not the
// one the tab was opened from.
func TestSummaryReportsTheBranchOfTheLiveDirectory(t *testing.T) {
	repo := t.TempDir()
	writeTestFile(t, filepath.Join(repo, ".git", "HEAD"), "ref: refs/heads/feat/sidebar\n")
	sub := filepath.Join(repo, "internal", "hub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	sess, err := Start("branch", 80, 24, sub, "/bin/sh")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer sess.Close()

	sess.checkLiveCwd()
	if got := sess.Summary().Branch; got != "feat/sidebar" {
		t.Fatalf("Summary().Branch = %q, want feat/sidebar", got)
	}
}

// A shell that cd's out of the repository has no branch any more: keeping the
// old one would name a checkout the shell is not in.
func TestSummaryDropsTheBranchWhenTheShellLeavesTheRepository(t *testing.T) {
	repo := t.TempDir()
	writeTestFile(t, filepath.Join(repo, ".git", "HEAD"), "ref: refs/heads/main\n")

	sess, err := Start("leave", 80, 24, repo, "/bin/sh")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer sess.Close()

	sess.checkLiveCwd()
	if got := sess.Summary().Branch; got != "main" {
		t.Fatalf("Summary().Branch = %q, want main", got)
	}

	other := t.TempDir()
	if err := sess.Write([]byte("cd " + other + "\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	waitForLiveCwd(t, sess, other)
	sess.checkLiveCwd()
	if got := sess.Summary().Branch; got != "" {
		t.Fatalf("Summary().Branch after leaving = %q, want empty", got)
	}
}

// The run time a row shows has to stop when the shell does. EndedAt is the
// moment the row freezes at, and it is absent while the shell is alive.
func TestSummaryReportsWhenTheShellExited(t *testing.T) {
	sess, err := Start("ended", 80, 24, t.TempDir(), "/bin/sh")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer sess.Close()

	if got := sess.Summary().EndedAt; got != "" {
		t.Fatalf("a live shell reports EndedAt = %q, want empty", got)
	}
	if err := sess.Write([]byte("exit\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	select {
	case <-sess.Exited():
	case <-time.After(10 * time.Second):
		t.Fatal("the shell did not exit")
	}
	endedAt := sess.Summary().EndedAt
	if endedAt == "" {
		t.Fatal("an exited shell reports no EndedAt")
	}
	ended, err := time.Parse(time.RFC3339Nano, endedAt)
	if err != nil {
		t.Fatalf("EndedAt = %q is not RFC3339Nano: %v", endedAt, err)
	}
	if ended.Before(sess.CreatedAt) {
		t.Fatalf("EndedAt %v is before the session was created %v", ended, sess.CreatedAt)
	}
}

// waitForLiveCwd waits until the shell's own directory is dir, following the
// same poll the app uses.
func waitForLiveCwd(t *testing.T, sess *Session, dir string) {
	t.Helper()
	want := resolve(t, dir)
	deadline := time.Now().Add(10 * time.Second)
	for {
		if got := resolve(t, sess.LiveCwd()); got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("LiveCwd = %q, want %q", sess.LiveCwd(), want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
