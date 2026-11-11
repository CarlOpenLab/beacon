package uitokens

import (
	"os"
	"path/filepath"
	"runtime"
	"regexp"
	"strings"
	"testing"
)

// The sidebar stands on the window's acrylic material rather than inside a card,
// so the session row's two secondary facts — the directory and the run time —
// take the palette's primary text ink. On a bright material the muted grey they
// used to take falls under 2:1 and the line cannot be read; a07947e raised the
// column's other chrome for exactly this and the row's second line was missed.
//
// This is the browser client's sheet, which is where a silent regression hides:
// it is a plain CSS file, nothing regenerates it, and the desktop client can be
// right while the browser client serves the grey that cannot be read.
func TestSessionRowSecondLineTakesThePrimaryInk(t *testing.T) {
	sheet := webStyleSheet(t)
	for _, sel := range []string{".tf-session-path", ".tf-session-run"} {
		block, ok := ruleFor(sheet, sel)
		if !ok {
			t.Fatalf("%s is gone from web/src/style.css", sel)
		}
		if got, want := inkOf(block), "--fg"; got != want {
			t.Errorf("%s is painted in var(%s), want var(--fg): the sidebar row stands on the acrylic material, where the muted grey is unreadable over a bright backdrop", sel, got)
		}
	}
}

func webStyleSheet(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", "web", "src", "style.css")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return strings.ReplaceAll(string(got), "\r\n", "\n")
}

// ruleFor returns the declarations of a top-level rule for the selector,
// ignoring comments.
func ruleFor(sheet, selector string) (string, bool) {
	re := regexp.MustCompile(`(?s)\n` + regexp.QuoteMeta(selector) + `\s*\{([^{}]*)\}`)
	m := re.FindStringSubmatch(sheet)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// inkOf returns the colour property a block sets, as the var() it names.
func inkOf(block string) string {
	re := regexp.MustCompile(`(?m)^\s*color:\s*var\((--[a-z-]+)\)`)
	m := re.FindStringSubmatch(block)
	if m == nil {
		return "(none)"
	}
	return m[1]
}
