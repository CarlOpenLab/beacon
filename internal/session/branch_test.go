package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTestFile writes one file, creating its parent directories.
func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// The branch a shell is on comes from the repository above the directory the
// shell stands in, not from the directory the session started in: a shell that
// cd'd into a project reports that project's branch.
func TestResolveBranchReadsTheRepositoryAboveTheDirectory(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/feat/sidebar\n")
	dir := filepath.Join(root, "internal", "hub")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	branch, repo := resolveBranch(dir)
	if branch != "feat/sidebar" {
		t.Fatalf("branch = %q, want feat/sidebar", branch)
	}
	if !sameDir(repo, root) {
		t.Fatalf("repository = %q, want %q", repo, root)
	}
}

// A checkout can switch branches without the shell moving. The next look at
// HEAD is what tells the row.
func TestResolveBranchFollowsAChangedHead(t *testing.T) {
	root := t.TempDir()
	head := filepath.Join(root, ".git", "HEAD")
	writeTestFile(t, head, "ref: refs/heads/main\n")

	if branch, _ := resolveBranch(root); branch != "main" {
		t.Fatalf("branch = %q, want main", branch)
	}
	writeTestFile(t, head, "ref: refs/heads/release/2.4\n")
	if branch, _ := resolveBranch(root); branch != "release/2.4" {
		t.Fatalf("branch after switching = %q, want release/2.4", branch)
	}
}

// A detached HEAD has no branch name; the short commit is what a person can
// still use to tell one checkout from another.
func TestResolveBranchUsesAShortHashWhenDetached(t *testing.T) {
	root := t.TempDir()
	sha := strings.Repeat("a1b2c3d", 5) + "0"
	writeTestFile(t, filepath.Join(root, ".git", "HEAD"), sha+"\n")

	if branch, _ := resolveBranch(root); branch != sha[:7] {
		t.Fatalf("branch = %q, want the short commit %q", branch, sha[:7])
	}
}

// A worktree's .git is a file pointing at the real git directory; its HEAD is
// the worktree's own branch, not the main checkout's.
func TestResolveBranchFollowsAWorktreeGitdirFile(t *testing.T) {
	main := t.TempDir()
	worktree := filepath.Join(main, "worktree")
	gitdir := filepath.Join(main, ".git", "worktrees", "sidebar")
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(worktree, ".git"), "gitdir: "+gitdir+"\n")
	writeTestFile(t, filepath.Join(gitdir, "HEAD"), "ref: refs/heads/feat/worktree\n")

	branch, repo := resolveBranch(worktree)
	if branch != "feat/worktree" {
		t.Fatalf("branch = %q, want feat/worktree", branch)
	}
	if !sameDir(repo, worktree) {
		t.Fatalf("repository = %q, want the worktree %q", repo, worktree)
	}
}

// Outside a repository there is no branch to show, and that is not an error.
func TestResolveBranchIsEmptyOutsideARepository(t *testing.T) {
	branch, repo := resolveBranch(t.TempDir())
	if branch != "" || repo != "" {
		t.Fatalf("branch, repository = %q, %q, want empty", branch, repo)
	}
}

// The nearest repository wins: a nested checkout is the one the shell is in,
// not the outer project that contains it.
func TestResolveBranchStopsAtTheNearestRepository(t *testing.T) {
	outer := t.TempDir()
	writeTestFile(t, filepath.Join(outer, ".git", "HEAD"), "ref: refs/heads/outer\n")
	inner := filepath.Join(outer, "vendor", "inner")
	writeTestFile(t, filepath.Join(inner, ".git", "HEAD"), "ref: refs/heads/inner\n")

	branch, repo := resolveBranch(inner)
	if branch != "inner" {
		t.Fatalf("branch = %q, want inner", branch)
	}
	if !sameDir(repo, inner) {
		t.Fatalf("repository = %q, want the nested repository %q", repo, inner)
	}
}
