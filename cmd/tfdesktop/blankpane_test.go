package main

// The two silent halves of a blank pane, pinned in tests: a session row left
// over from a machine that is gone, and a frame that arrived with no terminal to
// draw it. Neither crashes, neither shows an error, and both used to leave a
// white pane with a live shell behind it. The first must say something, and the
// second must reach the log — that is the point of adding the log at all.

import (
	"strings"
	"testing"

	"github.com/CarlOpenLab/term-fleet/internal/diag"
	"github.com/CarlOpenLab/term-fleet/internal/protocol"
	"github.com/CarlOpenLab/term-fleet/internal/tunnel"
	"github.com/CarlOpenLab/term-fleet/locales"
)

// captureLog installs an in-memory log for one test. The package default is
// process-wide, so a test that wants to read lines is the only one that may run
// with this installed; none of these tests run in parallel.
func captureLog(t *testing.T) *diag.Logger {
	t.Helper()
	logger := diag.New(diag.Debug)
	diag.SetDefault(logger)
	t.Cleanup(func() { diag.SetDefault(nil) })
	return logger
}

// A machine that has gone leaves its last session list on the panel. Clicking a
// row is a reasonable thing to do, and it must not open a tab: there is no
// stream to attach over, so the tab is a white rectangle with a live shell it
// cannot reach. The answer is a sentence.
func TestASessionRowOnAnOfflineMachineOpensNothing(t *testing.T) {
	captureLog(t)
	w, _ := newTestShellWindow(t, "one")
	w.roster = []tunnel.Machine{{ID: "box", Name: "carl-windows", Online: false}}
	w.remoteSessions = map[string][]protocol.Summary{
		"box": {{SessionID: "s1", Alive: true, Title: "PowerShell 5"}},
	}
	before := len(w.tabs)

	w.openOrSelect("box", "s1")

	if len(w.tabs) != before {
		t.Fatalf("a tab was opened for a session on an offline machine: %d tabs, want %d", len(w.tabs), before)
	}
	if want := locales.AppMachineOffline("carl-windows"); w.linkNote != want {
		t.Fatalf("the panel says %q, want %q", w.linkNote, want)
	}
	if w.currentNotice() == "" {
		t.Fatal("the click said nothing anywhere: no notice was posted")
	}
}

// The frame that decides a blank pane: the hub sent output for a session this
// link has nobody for. It is well-formed and the hub did its job, so nothing
// fails — it is counted and said once, with the machine and session on the line.
func TestAFrameForASessionNobodyWatchesIsRecorded(t *testing.T) {
	logger := captureLog(t)
	l := newLink()
	l.name = "carl-windows"

	l.dispatch(protocol.Msg{T: protocol.STOutput, SessionID: "s9", Data: []byte("replay")})

	joined := strings.Join(logger.Lines(), "\n")
	for _, want := range []string{
		"frame dropped: no session registered",
		"machine=carl-windows",
		"type=output",
		"session=s9",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("the log does not say %q:\n%s", want, joined)
		}
	}
}

// And the data's own view: a closed conn that is handed a replay. The counters
// are the useful part — fed>0, read=0 names a view that is not draining, where
// fed=0 names a replay that never arrived.
func TestOutputFedToAClosedConnIsRecorded(t *testing.T) {
	logger := captureLog(t)
	sess := newSession("s1", nil, nil)
	conn := newTermConn(sess)
	conn.machine = "carl-windows"
	_ = conn.Close()
	conn.feed([]byte("replay"), false)

	joined := strings.Join(logger.Lines(), "\n")
	for _, want := range []string{
		"output fed to a closed conn",
		"machine=carl-windows",
		"session=s1",
		"fed=0",
		"read=0",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("the log does not say %q:\n%s", want, joined)
		}
	}
}
