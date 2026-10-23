// Package diag is the fleet's own record of what it did: one line per event,
// in a file a person can send us when something on screen does not match what
// the machines say.
//
// The reason it exists is the one bug a terminal app cannot see about itself: a
// pane that stays empty while the shell behind it is alive. Nothing crashes,
// no error is shown, and the interesting question — did the replay reach this
// window, or was it sent to a session object nobody draws any more — is only
// answerable from a timeline. A timeline is what this package writes.
//
// Two properties matter more than completeness:
//
//   - Low volume by default. Debug lines are opt-in, and the two hot paths
//     (output bytes, frames) are summarised rather than logged one per frame —
//     a log that grows with the shell's output is a log nobody can send.
//   - Correlatable lines. Every line carries the machine and session it is
//     about, with stable key=value fields, so a blank pane reported at 10:14
//     can be matched to the attach that preceded it.
//
// The package's default logger is a no-op, which is what tests and library
// users get. main sets a real one, and every package logs through the
// package-level functions so nothing has to thread a logger through its
// signatures to be diagnosable.
package diag

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

// Level is how much a logger writes. The order is the one a person expects:
// an Error is always written, a Debug line only when asked for.
type Level int8

const (
	// Off writes nothing. It is the level of the default logger, so a program
	// that never configures one pays for the calls and produces no output.
	Off Level = iota
	// Error is something that failed: a terminal engine that would not start,
	// a frame that could not be sent.
	Error
	// Warn is something that did not go as intended but did not fail: a frame
	// dropped for a session nobody watches, a link replaced by a new one.
	Warn
	// Info is the shape of a run: links opening and closing, tabs attaching,
	// sessions created and ended. It is the default when a log is turned on.
	Info
	// Debug is the protocol's own traffic and the hot paths' counters. It is
	// verbose enough to answer "did the bytes arrive" and is meant to be on
	// while a problem is reproduced, not always.
	Debug
)

// ParseLevel reads the spelling a flag or the settings file uses. An unknown
// word is an error rather than a silent Info: a person who typed "verbose"
// meant something, and getting no logs without being told why is worse than
// starting with the default and saying so.
func ParseLevel(s string) (Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "info":
		return Info, nil
	case "off", "none":
		return Off, nil
	case "error":
		return Error, nil
	case "warn", "warning":
		return Warn, nil
	case "debug", "verbose":
		return Debug, nil
	}
	return Info, fmt.Errorf("unknown log level %q (error, warn, info or debug)", s)
}

// String is the level's word, for a settings field and for the log itself.
func (l Level) String() string {
	switch l {
	case Error:
		return "ERROR"
	case Warn:
		return "WARN"
	case Info:
		return "INFO"
	case Debug:
		return "DEBUG"
	}
	return "OFF"
}

// maxBytes is where a log is rotated. A week of ordinary use stays well under
// it; the debug level spends it in an afternoon, which is what rotation is for.
const maxBytes = 8 << 20

// ringLines is how much recent output a logger keeps in memory, for the
// settings window's "copy what just happened" and for a test that wants to
// assert a line without reading a file.
const ringLines = 1000

// Logger writes one line per event to a file, and keeps the most recent lines
// in memory. A nil *Logger is not valid; use Default or Nop.
type Logger struct {
	mu    sync.Mutex
	out   io.Writer
	file  *os.File
	path  string
	name  string
	level Level
	// limit is where the file rotates, as a field rather than the constant so a
	// test can fill one in a few lines instead of eight megabytes.
	limit   int64
	written int64
	ring    []string
	next    int
	full    bool
	now     func() time.Time
	// drops counts lines refused at the current level, so a person who turns
	// debug on for a moment can see how much was left out before that.
	drops int64
}

// Open starts a logger that appends to name.log in dir. A log that has grown
// past maxBytes is rotated to name.log.1 first, so the file a person sends is
// never truncated mid-line and never unbounded.
//
// A directory that cannot be created is an error: the caller reports it and
// keeps running without a file, because losing the log must not cost the app.
func Open(dir, name string, level Level) (*Logger, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, name+".log")
	if info, err := os.Stat(path); err == nil && info.Size() > maxBytes {
		_ = os.Remove(path + ".1")
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	var written int64
	if info, err := f.Stat(); err == nil {
		written = info.Size()
	}
	l := &Logger{out: f, file: f, path: path, name: name, level: level, limit: maxBytes, written: written, now: time.Now}
	return l, nil
}

// New captures lines in memory only, which is what a test wants: it asserts
// the timeline without leaving a file behind. A path of "" means there is no
// file to name.
func New(level Level) *Logger {
	return &Logger{level: level, limit: maxBytes, now: time.Now}
}

// Install opens a log, makes it the default, and points the standard library's
// log at it, so the lines the app already writes land in the same file as the
// ones this package writes. It is what both programs do at startup: nothing
// else has to know a file exists for the log to be complete.
//
// A program that fails here keeps running — losing the log must not cost the
// app — so the error is returned for the caller to report once, and the
// default stays the no-op logger.
func Install(dir, name string, level Level) (*Logger, error) {
	l, err := Open(dir, name, level)
	if err != nil {
		return nil, err
	}
	SetDefault(l)
	log.SetFlags(0)
	// stderr stays in the stream as well: a window launched from a terminal,
	// and a `tfd -h`-style run, should still show what went wrong without
	// anyone finding a file first.
	log.SetOutput(io.MultiWriter(os.Stderr, l))
	return l, nil
}

var (
	defaultMu sync.Mutex
	defaultL  = &Logger{level: Off, now: time.Now}
)

// SetDefault installs the logger the package-level functions write to. Passing
// nil restores the no-op logger; main calls it once, before anything logs.
func SetDefault(l *Logger) {
	defaultMu.Lock()
	if l == nil {
		l = &Logger{level: Off, now: time.Now}
	}
	defaultL = l
	defaultMu.Unlock()
}

// Default is the process-wide logger. It is never nil.
func Default() *Logger {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	return defaultL
}

// Path is the file the logger appends to, or "" when it writes nobody.
func (l *Logger) Path() string {
	if l == nil {
		return ""
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.path
}

// Level is what the logger is willing to write right now.
func (l *Logger) Level() Level {
	if l == nil {
		return Off
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.level
}

// SetLevel changes what is written from the next line on. The settings
// window's 诊断 switch calls it while the app runs.
func (l *Logger) SetLevel(level Level) {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.level = level
	l.drops = 0
	l.mu.Unlock()
}

// Enabled reports whether a line at level would be written. The hot paths ask
// before formatting anything expensive.
func (l *Logger) Enabled(level Level) bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return level <= l.level
}

// Close flushes and closes the file. Further lines go to the ring only.
func (l *Logger) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	l.out = nil
	return err
}

// Write lets the standard library's log package write into this one, which is
// what captures the log lines already in the app without touching every call:
// main redirects log's output here, and they arrive as INFO app lines.
func (l *Logger) Write(p []byte) (int, error) {
	if l == nil {
		return len(p), nil
	}
	line := strings.TrimRight(string(p), "\r\n")
	if line != "" {
		l.Logf(Info, "app", "%s", line)
	}
	return len(p), nil
}

// Logf writes one line at level, in category. Categories are short and stable:
// "term" for the terminal pipeline, "link" for a machine's connection, "hub"
// for the serving side, "relay" for the stream, "app" for the shell's own news.
func (l *Logger) Logf(level Level, category, format string, args ...any) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if level > l.level {
		l.drops++
		return
	}
	stamp := l.now().Format("2006-01-02T15:04:05.000-07:00")
	msg := fmt.Sprintf(format, args...)
	line := stamp + " " + level.String() + " " + category + " " + msg
	l.remember(line)
	if l.out == nil {
		return
	}
	// A write error is not actionable from here — the disk is full or the file
	// was removed — and returning it would only lose the line twice. The ring
	// still has it.
	n := len(line) + 1
	nw, err := io.WriteString(l.out, line+"\n")
	if nw > 0 {
		l.written += int64(nw)
	}
	if err == nil && n > 0 && l.written >= l.limit {
		l.rotateLocked()
	}
}

// rotateLocked starts a new file when the current one has reached the limit.
// One old file is kept (.log.1): enough to hold what led up to a problem that is
// still happening, and bounded so a long-running window cannot fill a disk.
func (l *Logger) rotateLocked() {
	if l.file == nil {
		return
	}
	_ = l.file.Close()
	_ = os.Remove(l.path + ".1")
	_ = os.Rename(l.path, l.path+".1")
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		// Nowhere left to write: the ring keeps the lines, and the next Open
		// picks the file back up.
		l.file, l.out = nil, nil
		return
	}
	l.file, l.out = f, f
	l.written = 0
}

// remember keeps the newest lines in memory, overwriting the oldest.
func (l *Logger) remember(line string) {
	if len(l.ring) < ringLines {
		l.ring = append(l.ring, line)
		return
	}
	l.ring[l.next] = line
	l.next = (l.next + 1) % ringLines
	l.full = true
}

// Lines returns the recent lines in order, oldest first. It is what the
// diagnostics page shows and what a test asserts on.
func (l *Logger) Lines() []string {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.full {
		out := make([]string, len(l.ring))
		copy(out, l.ring)
		return out
	}
	out := make([]string, 0, len(l.ring))
	out = append(out, l.ring[l.next:]...)
	out = append(out, l.ring[:l.next]...)
	return out
}

// Recent joins the newest n lines into one block, for a copy button.
func (l *Logger) Recent(n int) string {
	lines := l.Lines()
	if n > 0 && len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// ---- package-level conveniences, over the default logger -------------------

// Infof logs at Info on the default logger.
func Infof(category, format string, args ...any) { Default().Logf(Info, category, format, args...) }

// Warnf logs at Warn on the default logger.
func Warnf(category, format string, args ...any) { Default().Logf(Warn, category, format, args...) }

// Errorf logs at Error on the default logger.
func Errorf(category, format string, args ...any) { Default().Logf(Error, category, format, args...) }

// Debugf logs at Debug on the default logger. It is cheap to call in a hot
// path: the level is checked before the arguments are formatted.
func Debugf(category, format string, args ...any) {
	if d := Default(); d.Enabled(Debug) {
		d.Logf(Debug, category, format, args...)
	}
}

// DebugEnabled reports whether a caller should gather the fields for a Debug
// line — a session's size, say, which costs a lock to read.
func DebugEnabled() bool { return Default().Enabled(Debug) }

// buildVersion is set by the release build with -ldflags -X, because a binary
// built from a checkout has no version in its build info that anybody wants to
// read on another machine's roster.
var buildVersion string

// BuildVersion is the version the binary was built as, for the first line of a
// log: "devel" for a `go run`, and the module's version for a release build.
// A bug report that starts with a different build than the tree it is read
// against is worth catching there rather than three questions later.
func BuildVersion() string {
	if buildVersion != "" {
		return buildVersion
	}
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" {
		return "devel"
	}
	return info.Main.Version
}

var (
	instanceOnce sync.Once
	instanceName string
)

// Instance names this running process: the hostname, the pid and the second it
// started. It is what tells two links that reach the same daemon — a machine
// enrolled twice, a route added later — from two machines, and it changes when
// the daemon restarts, which is exactly when its sessions changed too.
func Instance() string {
	instanceOnce.Do(func() {
		host, _ := os.Hostname()
		if host == "" {
			host = "unknown"
		}
		instanceName = fmt.Sprintf("%s/%d/%d", host, os.Getpid(), time.Now().Unix())
	})
	return instanceName
}
