package diag

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// A line says when, how loud, what about, and the fields a search needs — the
// four things a person reading a log to answer "did the replay arrive" looks
// for.
func TestALineCarriesItsTimeLevelCategoryAndFields(t *testing.T) {
	l := New(Info)
	l.now = func() time.Time { return time.Date(2026, 10, 21, 10, 14, 2, 123e6, time.Local) }
	l.Logf(Info, "term", "attach sent machine=%s session=%s cursor=%d", "carl-windows", "s3", 0)

	lines := l.Lines()
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want one: %v", len(lines), lines)
	}
	line := lines[0]
	for _, want := range []string{
		"2026-10-21T10:14:02.123",
		" INFO term ",
		"attach sent",
		"machine=carl-windows",
		"session=s3",
		"cursor=0",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("line %q does not contain %q", line, want)
		}
	}
}

// Debug lines are dropped at Info, kept at Debug, and switching up while the
// app runs is what the settings switch does.
func TestDebugIsOptInAndCanBeTurnedOnLater(t *testing.T) {
	l := New(Info)
	l.Logf(Debug, "link", "output bytes=1")
	if got := len(l.Lines()); got != 0 {
		t.Fatalf("a debug line was written at info level: %d lines", got)
	}
	l.SetLevel(Debug)
	l.Logf(Debug, "link", "output bytes=1")
	if got := len(l.Lines()); got != 1 {
		t.Fatalf("a debug line was dropped at debug level: %d lines", got)
	}
}

// The ring keeps the newest lines and forgets the oldest, which is what makes
// a long-running window's recent history readable.
func TestTheRingKeepsTheNewestLines(t *testing.T) {
	l := New(Debug)
	for i := 0; i < ringLines+25; i++ {
		l.Logf(Info, "app", "line %d", i)
	}
	lines := l.Lines()
	if len(lines) != ringLines {
		t.Fatalf("the ring holds %d lines, want %d", len(lines), ringLines)
	}
	if !strings.HasSuffix(lines[0], "line 25") {
		t.Errorf("the oldest line is %q, want line 25", lines[0])
	}
	if !strings.HasSuffix(lines[len(lines)-1], "line 1024") {
		t.Errorf("the newest line is %q, want line 1024", lines[len(lines)-1])
	}
	recent := strings.Split(l.Recent(2), "\n")
	if len(recent) != 2 || !strings.HasSuffix(recent[0], "line 1023") || !strings.HasSuffix(recent[1], "line 1024") {
		t.Errorf("Recent(2) is %q, want the last two lines only", recent)
	}
}

// The file is append-only and holds the same line the ring does, which is what
// a person sends us: one file with the timeline in it.
func TestOpenAppendsToAFile(t *testing.T) {
	dir := t.TempDir()
	l, err := Open(dir, "test", Info)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	l.Logf(Info, "term", "attach sent session=s1")
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	path := filepath.Join(dir, "test.log")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(b), "attach sent session=s1") {
		t.Fatalf("the log does not hold the line: %q", b)
	}

	// A second run appends rather than truncating: a crash and a restart leave
	// one story, not two.
	again, err := Open(dir, "test", Info)
	if err != nil {
		t.Fatalf("Open again: %v", err)
	}
	again.Logf(Info, "term", "attached session=s1")
	_ = again.Close()
	b, _ = os.ReadFile(path)
	if strings.Count(string(b), "session=s1") != 2 {
		t.Fatalf("the second run did not append: %q", b)
	}
	if got := l.Path(); got != path {
		t.Errorf("the closed logger names %q, want the file it wrote %q", got, path)
	}
}

// The standard library's log is redirected here, so the lines the app already
// writes arrive with a level and a category instead of vanishing into a
// stderr nobody can see under a window.
func TestStandardLogLinesArriveAsAppLines(t *testing.T) {
	l := New(Info)
	saved := log.Writer()
	log.SetOutput(l)
	log.SetFlags(0)
	t.Cleanup(func() { log.SetOutput(saved) })

	log.Print("tfdesktop: terminal engine unavailable")
	lines := l.Lines()
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want one: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], " INFO app ") || !strings.Contains(lines[0], "terminal engine unavailable") {
		t.Errorf("the standard log line is %q", lines[0])
	}
}

// Many goroutines log at once — the link's reader, the hub, the window's
// thread — and the file must come out as whole lines, not as interleaved ones.
func TestConcurrentWritersKeepLinesWhole(t *testing.T) {
	dir := t.TempDir()
	l, err := Open(dir, "test", Info)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				l.Logf(Info, "term", "writer=%d line=%d", i, j)
			}
		}(i)
	}
	wg.Wait()
	_ = l.Close()

	b, err := os.ReadFile(filepath.Join(dir, "test.log"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 400 {
		t.Fatalf("got %d lines, want 400", len(lines))
	}
	for _, line := range lines {
		if !strings.Contains(line, " INFO term writer=") || !strings.Contains(line, " line=") {
			t.Fatalf("a line was torn: %q", line)
		}
	}
}

func TestParseLevel(t *testing.T) {
	for in, want := range map[string]Level{
		"":        Info,
		"info":    Info,
		"INFO":    Info,
		"debug":   Debug,
		"verbose": Debug,
		"warn":    Warn,
		"warning": Warn,
		"error":   Error,
		"off":     Off,
	} {
		got, err := ParseLevel(in)
		if err != nil || got != want {
			t.Errorf("ParseLevel(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := ParseLevel("loud"); err == nil {
		t.Error("an unknown level was accepted")
	}
}

// The default logger is a no-op until main installs one: a library test that
// logs must not leave files or expect output.
func TestTheDefaultLoggerIsSilent(t *testing.T) {
	old := Default()
	SetDefault(nil)
	t.Cleanup(func() { SetDefault(old) })

	Infof("app", "this goes nowhere")
	if got := len(Default().Lines()); got != 0 {
		t.Fatalf("the no-op logger kept %d lines", got)
	}
	if got := Default().Path(); got != "" {
		t.Fatalf("the no-op logger names %q", got)
	}
}

// The file rotates in place while the app runs, so a window left open for days
// does not grow one file without bound — and the newest lines, the ones a
// report is about, are always in the current file.
func TestTheFileRotatesWhileItRuns(t *testing.T) {
	dir := t.TempDir()
	l, err := Open(dir, "test", Info)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	l.limit = 256
	for i := 0; i < 40; i++ {
		l.Logf(Info, "term", "attach sent session=s%d", i)
	}
	_ = l.Close()

	path := filepath.Join(dir, "test.log")
	if info, err := os.Stat(path + ".1"); err != nil || info.Size() == 0 {
		t.Fatalf("no rotated file: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat the current log: %v", err)
	}
	if info.Size() > l.limit {
		t.Fatalf("the current file is %d bytes, want at most %d", info.Size(), l.limit)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(b), "session=s39") {
		t.Fatalf("the newest line is not in the current file: %q", b)
	}
}
