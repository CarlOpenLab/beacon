package session

import (
	"os"
	"path/filepath"
	"strings"
)

// resolveBranch is where a shell's checkout says which branch it is on. It
// returns the branch and the directory of the repository it came from, or two
// empty strings when dir is not inside one.
//
// Git is read from the filesystem rather than asked as a process: a session
// asks this on its foreground poll, and starting git twice a second for every
// shell would cost more than the answer is worth. HEAD is a small file, and a
// worktree's .git file names the directory that really holds it.
func resolveBranch(dir string) (branch, repo string) {
	repo, gitDir := resolveRepo(dir)
	if repo == "" {
		return "", ""
	}
	return readHead(gitDir), repo
}

// resolveRepo is resolveBranch without the HEAD read, for a caller that will
// look at HEAD again as the branch changes: the repository and the directory
// holding its HEAD are enough to find the branch without walking the tree.
func resolveRepo(dir string) (repo, gitDir string) {
	if dir == "" {
		return "", ""
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", ""
	}
	for {
		gitPath := filepath.Join(abs, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			gitDir, ok := gitDirAt(gitPath)
			if !ok {
				// The nearest .git is broken. An outer repository's branch
				// would be a different checkout's, so say nothing instead.
				return "", ""
			}
			return abs, gitDir
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", ""
		}
		abs = parent
	}
}

// gitDirAt is the directory that holds a checkout's HEAD: the .git directory
// itself, or the gitdir a worktree's .git file points at.
func gitDirAt(gitPath string) (string, bool) {
	info, err := os.Stat(gitPath)
	if err != nil {
		return "", false
	}
	if info.IsDir() {
		return gitPath, true
	}
	data, err := os.ReadFile(gitPath)
	if err != nil {
		return "", false
	}
	line := strings.TrimSpace(string(data))
	const prefix = "gitdir:"
	if !strings.HasPrefix(line, prefix) {
		return "", false
	}
	target := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if target == "" {
		return "", false
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(gitPath), target)
	}
	return filepath.Clean(target), true
}

// readHead is the branch HEAD names. A detached HEAD has no branch name; the
// short commit is the one thing that still tells one checkout from another.
func readHead(gitDir string) string {
	data, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(data))
	if ref, ok := strings.CutPrefix(line, "ref: refs/heads/"); ok {
		return ref
	}
	if len(line) >= 7 && isHex(line) {
		return line[:7]
	}
	return ""
}

// isHex reports whether s is a commit id's own alphabet.
func isHex(s string) bool {
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		case r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}
