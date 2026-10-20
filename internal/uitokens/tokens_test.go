package uitokens

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestCSS fails when web/src/tokens.css is not what CSS() says it should be.
// The browser client's sheet is generated, and a hand edit — or a change to
// tokens.go that was never regenerated — is exactly how the two clients drift
// apart, which this package exists to prevent.
func TestCSS(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", "web", "src", "tokens.css")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v (run `go run ./internal/uitokens/gen`)", path, err)
	}
	// Git may check the sheet out with CRLF; the line endings are the
	// checkout's business, not the generator's.
	text := strings.ReplaceAll(string(got), "\r\n", "\n")
	if text != CSS() {
		t.Fatalf("web/src/tokens.css is stale; run `go run ./internal/uitokens/gen`")
	}
}

// TestWashes checks the invariants the appearance settings rely on: every wash
// has stops for both appearances, and an unknown id falls back to the first.
func TestWashes(t *testing.T) {
	for _, w := range Washes {
		if len(w.Light) < 2 || len(w.Dark) < 2 {
			t.Errorf("%s: needs at least two stops per appearance", w.ID)
		}
	}
	if WashFor("nope").ID != Washes[0].ID {
		t.Errorf("unknown wash: got %s, want %s", WashFor("nope").ID, Washes[0].ID)
	}
}
