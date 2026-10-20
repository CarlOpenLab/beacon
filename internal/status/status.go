// Package status tells what a session is doing, from what its output says and
// from what the program in it reports.
//
// A terminal here is a byte pipe, not an emulator: there is no screen to read.
// So the signals are the ones a pipe still carries — the OSC reports a program
// writes (7501 for its own state, 9 for a notification), the terminal's title,
// the bell, and how recently output arrived at all. That is enough to answer the
// question a fleet has to answer: which of these shells is working, and which
// one is waiting for me.
package status

import (
	"encoding/base64"
	"strconv"
	"strings"
	"sync"
	"time"
)

// State is what a session is doing.
type State string

const (
	// Idle is nothing happening: the shell is at its prompt.
	Idle State = "idle"
	// Working is a program busy: an agent thinking, a command running.
	Working State = "working"
	// Waiting is a program that cannot go on without the user: a permission
	// prompt, a question.
	Waiting State = "waiting"
	// Error is a program that reported it failed.
	Error State = "error"
	// Exited is a shell whose process ended.
	Exited State = "exited"
)

// Snapshot is a session's state at one moment, as the protocol carries it.
type Snapshot struct {
	State    State
	Unread   bool
	Message  string
	Kind     string // permission, question, auth
	Progress int    // 0-100, -1 for none
	Since    time.Time
	// Process is the program in the foreground of the PTY: "zsh" at a prompt,
	// "claude" while an agent runs. It is "" when the platform cannot say.
	Process string
}

// report is what a program said about itself, with OSC 7501.
type report struct {
	state    State
	done     bool
	kind     string
	message  string
	progress int
	title    string
	at       time.Time
}

// Tracker follows one session's output, and the state that output implies.
type Tracker struct {
	// onChange is called when the snapshot changes, from whichever goroutine
	// noticed. It is the daemon's cue to tell its clients.
	onChange func()

	mu       sync.Mutex
	scan     scanner
	rep      report
	title    string
	bell     bool
	process  string
	lastOut  time.Time
	unread   bool
	state    State
	since    time.Time
	progress int
	// timer is the wakeup that turns a burst of output back into idle without
	// another byte arriving.
	timer *time.Timer
	// seen is the last snapshot handed out, for "did it change".
	seen Snapshot
}

// WorkingWindow is how long output counts as "still working": a program that
// has said nothing for this long is at its prompt again. Short enough that a
// finished command's row goes quiet while a person is still reading it, long
// enough that a command that pauses between lines does not blink.
const WorkingWindow = 1200 * time.Millisecond

// New makes a tracker. onChange may be nil.
func New(onChange func()) *Tracker {
	return &Tracker{onChange: onChange, progress: -1, state: Idle, since: time.Now()}
}

// Observe feeds output to the tracker.
func (t *Tracker) Observe(p []byte) {
	if len(p) == 0 {
		return
	}
	t.mu.Lock()
	var got report
	var bell bool
	var title string
	has := false
	t.scan.scan(p, func(ev event) {
		switch ev.kind {
		case evReport:
			got, has = ev.report, true
		case evBell:
			bell = true
		case evTitle:
			title = ev.text
		}
	})
	if has {
		now := time.Now()
		got.at = now
		// A working or waiting report belongs to the program that made it: it
		// stops when the program does, or when the user types. A done or an
		// error stays until the user looks.
		t.rep = got
		t.state = got.state
		t.progress = got.progress
		t.since = now
		if got.state == Waiting || got.state == Error || got.done {
			t.unread = true
		}
		if got.state == Idle {
			t.bell = false
		}
	}
	if title != "" {
		t.title = title
	}
	if bell {
		t.bell = true
	}
	t.lastOut = time.Now()
	t.recomputeLocked()
	t.mu.Unlock()
	t.emit()
}

// Typed is called when the user types into the session: whatever was waiting
// is being answered, so the row is not waiting any more, and it is read.
func (t *Tracker) Typed() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.unread = false
	if t.state == Waiting || t.state == Error || t.rep.done {
		t.rep = report{}
		t.bell = false
		t.state = Working
		t.since = time.Now()
	}
	t.recomputeLocked()
}

// SetProcess records the program in the foreground, which the session polls out
// of the kernel. A change is worth a frame: it is the chip a person reads.
func (t *Tracker) SetProcess(name string) {
	t.mu.Lock()
	changed := t.process != name
	t.process = name
	t.mu.Unlock()
	if changed {
		t.emit()
	}
}

// Viewed marks the session read: a client is looking at it.
func (t *Tracker) Viewed() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.unread = false
}

// Exited records that the shell's process is gone.
func (t *Tracker) Exited() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state == Exited {
		return
	}
	t.state = Exited
	t.since = time.Now()
	if t.rep.state != "" && t.rep.state != Waiting {
		// The program that said it was working ended with the shell.
		t.rep = report{}
	}
}

// Snapshot returns the current state.
func (t *Tracker) Snapshot() Snapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.snapshotLocked()
}

func (t *Tracker) snapshotLocked() Snapshot {
	s := Snapshot{State: t.state, Unread: t.unread, Progress: -1, Since: t.since, Process: t.process}
	if t.rep.state != "" {
		s.Message = t.rep.message
		s.Kind = t.rep.kind
		if t.rep.progress >= 0 {
			s.Progress = t.rep.progress
		}
	}
	if s.State == Exited {
		s.Progress = -1
	}
	return s
}

// recomputeLocked decides the state from the signals, in the order they can be
// trusted: what a program said, then the bell it rang, then a title that shows
// a spinner, then output that is still arriving.
func (t *Tracker) recomputeLocked() {
	if t.state == Exited {
		return
	}
	// A program that reported its own state is believed until it goes quiet in
	// a way the report cannot explain: done and error stay for the user.
	if t.rep.done && t.rep.state == Idle {
		// "done" is a state of its own: the work finished and the user has not
		// looked, so the shell is idle but says so with a check, not with output
		// that happened to arrive with the report.
		t.state = Idle
		return
	}
	switch t.rep.state {
	case Waiting, Error:
		t.state = t.rep.state
		return
	case Working:
		if time.Since(t.rep.at) < 30*time.Second {
			t.state = Working
			t.scheduleLocked()
			return
		}
	}
	if t.bell {
		t.state = Waiting
		return
	}
	if spinnerTitle(t.title) {
		t.state = Working
		t.scheduleLocked()
		return
	}
	if time.Since(t.lastOut) < WorkingWindow {
		if t.state != Working {
			t.since = time.Now()
		}
		t.state = Working
		t.scheduleLocked()
		return
	}
	if t.state != Idle {
		t.since = time.Now()
	}
	t.state = Idle
}

// scheduleLocked wakes the tracker when the working window runs out, so a quiet
// session returns to idle without another byte.
func (t *Tracker) scheduleLocked() {
	if t.timer != nil {
		return
	}
	t.timer = time.AfterFunc(WorkingWindow+120*time.Millisecond, func() {
		t.mu.Lock()
		t.timer = nil
		t.recomputeLocked()
		t.mu.Unlock()
		t.emit()
	})
}

// emit calls onChange when the snapshot differs from the last one handed out.
func (t *Tracker) emit() {
	t.mu.Lock()
	s := t.snapshotLocked()
	changed := s.State != t.seen.State || s.Unread != t.seen.Unread || s.Message != t.seen.Message ||
		s.Kind != t.seen.Kind || s.Progress != t.seen.Progress || s.Process != t.seen.Process
	if changed {
		t.seen = s
	}
	fn := t.onChange
	t.mu.Unlock()
	if changed && fn != nil {
		fn()
	}
}

// spinnerTitle reports whether a terminal title starts with a spinner, which is
// how a program that does not use OSC 7501 still says it is busy: a braille
// cell, or one of the quarter circles.
func spinnerTitle(title string) bool {
	for _, r := range strings.TrimSpace(title) {
		switch {
		case r >= 0x2800 && r <= 0x28FF: // braille
			return true
		case r >= '◐' && r <= '◒': // ◐ ◑ ◒
			return true
		case r == '◓':
			return true
		}
		return false
	}
	return false
}

// --- the scanner ---

type eventKind int

const (
	evReport eventKind = iota
	evBell
	evTitle
)

type event struct {
	kind   eventKind
	report report
	text   string
}

// scanner finds the OSC sequences and bells in a program's output, following
// them across reads.
type scanner struct {
	state int // 0 text, 1 after ESC, 2 in an OSC, 3 skipping an OSC, 4 ESC in an OSC
	buf   []byte
}

const maxOSC = 4096

func (s *scanner) scan(p []byte, fn func(event)) {
	for _, b := range p {
		switch s.state {
		case 0:
			switch b {
			case 0x1b:
				s.state = 1
			case 0x07:
				fn(event{kind: evBell})
			case 0x9d: // C1 OSC
				s.begin()
			}
		case 1:
			if b == ']' {
				s.begin()
			} else if b != 0x1b {
				s.state = 0
			}
		case 2, 3:
			switch b {
			case 0x07, 0x9c:
				s.end(fn)
			case 0x1b:
				s.state = 4
			default:
				if s.state == 2 {
					s.buf = append(s.buf, b)
					if len(s.buf) > maxOSC {
						s.state, s.buf = 3, s.buf[:0]
					}
				}
			}
		case 4:
			// ESC inside an OSC: "\" ends it, anything else is the next byte's
			// business.
			if b == '\\' {
				s.end(fn)
			} else {
				s.state = 2
			}
		}
	}
}

func (s *scanner) begin() { s.state, s.buf = 2, s.buf[:0] }

func (s *scanner) end(fn func(event)) {
	body := string(s.buf)
	s.state, s.buf = 0, s.buf[:0]
	switch {
	case strings.HasPrefix(body, "7501;"):
		if rep, ok := parseReport(body[len("7501;"):]); ok {
			fn(event{kind: evReport, report: rep})
		}
	case strings.HasPrefix(body, "0;"), strings.HasPrefix(body, "2;"):
		fn(event{kind: evTitle, text: body[2:]})
	case strings.HasPrefix(body, "9;"):
		// ConEmu's notification: shown as a message, not as a state.
		fn(event{kind: evReport, report: report{state: Waiting, message: sanitize(body[2:]), at: time.Now()}})
	}
}

// parseReport reads a 7501 body: "state=blocked:kind=permission:progress=40:
// msg=<base64>:title=<base64>". A report with no state, or a state this package
// does not know, is discarded whole.
func parseReport(body string) (report, bool) {
	rep := report{progress: -1}
	fields := strings.Split(body, ":")
	for _, f := range fields {
		k, v, ok := strings.Cut(f, "=")
		if !ok {
			continue
		}
		switch k {
		case "state":
			switch State(v) {
			case Idle, Working, Waiting, Error:
				rep.state = State(v)
			case "done":
				// the name for "finished, and the user has not looked": the
				// state a fleet draws as a check.
				rep.state = Idle
				rep.done = true
			case "blocked":
				rep.state = Waiting
			case "clear":
				rep.state = ""
			default:
				return report{}, false
			}
		case "kind":
			if v == "permission" || v == "question" || v == "auth" {
				rep.kind = v
			}
		case "progress":
			if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 100 {
				rep.progress = n
			}
		case "msg":
			rep.message = sanitize(decode(v))
		case "title":
			rep.title = sanitize(decode(v))
		}
	}
	if rep.state == "" {
		return report{}, false
	}
	return rep, true
}

func decode(s string) string {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return s
	}
	return string(b)
}

// sanitize drops bidi and invisible formatting characters: a message is drawn in
// the window's chrome, and a program must not be able to reorder it.
func sanitize(s string) string {
	if len(s) > 2048 {
		s = s[:2048]
	}
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069, r == 0x200E, r == 0x200F, r == 0x061C:
			return -1
		}
		return r
	}, s)
}
