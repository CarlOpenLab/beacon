package status

import (
	"encoding/base64"
	"testing"
	"time"
)

// oscReport builds an OSC 7501 sequence, the way a program writes one.
func oscReport(state string, fields ...string) []byte {
	body := "7501;state=" + state
	for _, f := range fields {
		body += ":" + f
	}
	return []byte("\x1b]" + body + "\x1b\\")
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func newTracker() (*Tracker, *int) {
	changes := 0
	tr := New(func() { changes++ })
	return tr, &changes
}

func TestAProgramStateIsBelieved(t *testing.T) {
	tr, changes := newTracker()
	tr.Observe(oscReport("working", "progress=40", "msg="+b64("Migrating the database")))
	got := tr.Snapshot()
	if got.State != Working || got.Progress != 40 || got.Message != "Migrating the database" {
		t.Fatalf("working report: %+v", got)
	}
	if *changes == 0 {
		t.Fatal("a state change did not tell anyone")
	}

	tr.Observe(oscReport("blocked", "kind=permission", "msg="+b64("Approve the deploy?")))
	got = tr.Snapshot()
	if got.State != Waiting || got.Kind != "permission" || !got.Unread {
		t.Fatalf("blocked report: %+v", got)
	}

	tr.Observe(oscReport("error", "msg="+b64("build failed")))
	got = tr.Snapshot()
	if got.State != Error || !got.Unread || got.Message != "build failed" {
		t.Fatalf("error report: %+v", got)
	}

	// Typing answers whatever was waiting: the state goes back to working, and
	// the message that was waiting for an answer goes with the answer.
	tr.Typed()
	got = tr.Snapshot()
	if got.State != Working || got.Unread || got.Message != "" {
		t.Fatalf("after typing: %+v", got)
	}
}

func TestADoneReportIsUnreadUntilSeen(t *testing.T) {
	tr, _ := newTracker()
	tr.Observe(oscReport("done", "msg="+b64("Deployed to staging")))
	got := tr.Snapshot()
	if got.State != Idle || !got.Unread || got.Message != "Deployed to staging" {
		t.Fatalf("done report: %+v", got)
	}
	tr.Viewed()
	if got = tr.Snapshot(); got.Unread {
		t.Fatal("looking at the session did not mark it read")
	}
}

func TestAnUnknownReportIsDiscardedWhole(t *testing.T) {
	tr, _ := newTracker()
	tr.Observe(oscReport("working", "progress=10"))
	tr.Observe(oscReport("dancing", "progress=90", "msg="+b64("nonsense")))
	got := tr.Snapshot()
	if got.State != Working || got.Progress != 10 || got.Message != "" {
		t.Fatalf("a report with a bad state changed the tracker: %+v", got)
	}
	// A sequence with no state is nothing at all.
	tr.Observe([]byte("\x1b]7501;progress=70\x1b\\"))
	if got := tr.Snapshot(); got.Progress != 10 {
		t.Fatalf("a stateless report changed the progress: %+v", got)
	}
}

func TestAReportSplitAcrossReadsIsStillOneReport(t *testing.T) {
	tr, _ := newTracker()
	full := oscReport("waiting", "kind=question", "msg="+b64("Which database?"))
	for _, b := range full {
		tr.Observe([]byte{b})
	}
	got := tr.Snapshot()
	if got.State != Waiting || got.Kind != "question" || got.Message != "Which database?" {
		t.Fatalf("split report: %+v", got)
	}
}

func TestABellWaitsForTheUser(t *testing.T) {
	tr, _ := newTracker()
	tr.Observe([]byte("some output\x07"))
	if got := tr.Snapshot(); got.State != Waiting {
		t.Fatalf("a bell did not wait: %+v", got)
	}
	tr.Typed()
	if got := tr.Snapshot(); got.State == Waiting {
		t.Fatalf("typing did not answer the bell: %+v", got)
	}
}

func TestASpinnerTitleMeansWorking(t *testing.T) {
	tr, _ := newTracker()
	tr.Observe([]byte("\x1b]0;⠋ running tests\x07"))
	if got := tr.Snapshot(); got.State != Working {
		t.Fatalf("a spinner title did not mean working: %+v", got)
	}
	for _, title := range []string{"◐ building", "◓ zsh", "⠙ npm"} {
		if !spinnerTitle(title) {
			t.Errorf("%q is not read as a spinner", title)
		}
	}
	for _, title := range []string{"zsh", "~/dev", "", "· done"} {
		if spinnerTitle(title) {
			t.Errorf("%q is read as a spinner", title)
		}
	}
}

func TestOutputWorksAndThenGoesQuiet(t *testing.T) {
	tr, _ := newTracker()
	tr.Observe([]byte("compiling…"))
	if got := tr.Snapshot(); got.State != Working {
		t.Fatalf("output did not count as working: %+v", got)
	}
	// The wakeup the tracker scheduled turns it back to idle without another
	// byte, which is what a finished command looks like from a byte pipe.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if tr.Snapshot().State == Idle {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("output stayed working: %+v", tr.Snapshot())
}

func TestAnExitEndsTheSession(t *testing.T) {
	tr, _ := newTracker()
	tr.Observe(oscReport("working"))
	tr.Exited()
	if got := tr.Snapshot(); got.State != Exited {
		t.Fatalf("exit: %+v", got)
	}
	// A state a program reported does not outlive the shell it was reported in.
	if got := tr.Snapshot(); got.Message != "" {
		t.Fatalf("a dead shell still carries a message: %+v", got)
	}
}

func TestAMessageCannotReorderTheChrome(t *testing.T) {
	tr, _ := newTracker()
	tr.Observe(oscReport("waiting", "msg="+b64("ok\u202egnp.exe")))
	if got := tr.Snapshot(); got.Message != "okgnp.exe" {
		t.Fatalf("bidi characters were kept: %q", got.Message)
	}
}

// The foreground program is part of the snapshot, and a change of program is a
// change worth a frame: it is the chip a card's header reads.
func TestTheForegroundProgramJoinsTheSnapshot(t *testing.T) {
	tr := New(nil)
	changes := 0
	tr.onChange = func() { changes++ }
	tr.SetProcess("claude")
	if got := tr.Snapshot().Process; got != "claude" {
		t.Fatalf("process = %q", got)
	}
	if changes != 1 {
		t.Fatalf("the first program was not reported: %d changes", changes)
	}
	tr.SetProcess("claude")
	if changes != 1 {
		t.Fatalf("the same program was reported again: %d changes", changes)
	}
	tr.SetProcess("zsh")
	if changes != 2 {
		t.Fatalf("the new program was not reported: %d changes", changes)
	}
}
