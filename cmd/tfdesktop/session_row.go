package main

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The second and third lines of a session row: the path the shell stands in,
// shortened to what a 268-point column can hold, and the shell's run time.

// displayCwd is a directory as a row writes it: the home directory is ~, and a
// path too deep for the column keeps its last three parts behind an ellipsis.
// The end is what tells one project from another; the root is the part safe to
// drop.
func displayCwd(dir string) string {
	if dir == "" {
		return ""
	}
	dir = filepath.Clean(dir)
	sep := string(os.PathSeparator)
	parts := func(p string) []string {
		raw := strings.Split(p, sep)
		out := raw[:0]
		for _, part := range raw {
			if part != "" {
				out = append(out, part)
			}
		}
		return out
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if dir == filepath.Clean(home) {
			return "~"
		}
		if rel, err := filepath.Rel(home, dir); err == nil && rel != ".." &&
			!strings.HasPrefix(rel, ".."+sep) {
			if parts := parts(rel); len(parts) > 3 {
				return "…" + sep + strings.Join(parts[len(parts)-3:], sep)
			}
			return "~" + sep + rel
		}
	}
	if parts := parts(dir); len(parts) > 3 {
		return "…" + sep + strings.Join(parts[len(parts)-3:], sep)
	}
	return dir
}

// rowPath is the path line under a row's name. When the name is the directory's
// own last part — a fresh shell that has not titled itself — the line gives the
// parent instead, so the two lines reconstruct one path rather than repeating
// it.
func rowPath(e sessionEntry) string {
	shown := displayCwd(e.cwd)
	if shown == "" {
		return ""
	}
	if e.label == filepath.Base(filepath.Clean(e.cwd)) {
		return parentDisplay(shown)
	}
	return shown
}

// parentDisplay drops the last part of a display path.
func parentDisplay(display string) string {
	sep := string(os.PathSeparator)
	cut := strings.TrimRight(display, sep)
	if cut == "" {
		return display
	}
	i := strings.LastIndex(cut, sep)
	switch {
	case i < 0:
		return display
	case i == 0:
		return sep
	default:
		return cut[:i]
	}
}

// runtimeText is how long the shell has run, shortly: the row's right column.
// The end is the shell's exit when it has one, so the number stops instead of
// growing past the process; a host that did not record the exit shows nothing.
func runtimeText(e sessionEntry, now time.Time) string {
	if e.createdAt.IsZero() {
		return ""
	}
	end := now
	if !e.endedAt.IsZero() {
		end = e.endedAt
	}
	d := end.Sub(e.createdAt)
	if d < 0 {
		return ""
	}
	switch {
	case d < time.Minute:
		return "<1m"
	case d < time.Hour:
		return itoa(int(d/time.Minute)) + "m"
	case d < 24*time.Hour:
		return itoa(int(d/time.Hour)) + "h"
	case d < 7*24*time.Hour:
		return itoa(int(d/(24*time.Hour))) + "d"
	}
	return itoa(int(d/(7*24*time.Hour))) + "w"
}
