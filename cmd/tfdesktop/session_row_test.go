package main

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CarlOpenLab/beacon/internal/protocol"
	"github.com/CarlOpenLab/beacon/locales"
)

// A session row says the three things a person picks a shell by: its name, the
// directory it is in, and how long it has run. The branch lives in the tooltip:
// a third line under the path made every row taller than the list can spare.
func TestASessionRowShowsTitlePathAndRuntime(t *testing.T) {
	w, _ := newTestShellWindow(t, "fix the row")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home directory: %v", err)
	}
	cwd := filepath.Join(home, "dev", "own", "beacon")
	w.tabs[1].alive = true
	w.live = []protocol.Summary{{
		SessionID: "s1", Title: "fix the row", Cwd: cwd,
		Branch: "feat/sidebar-meta", Alive: true,
		CreatedAt: time.Now().Add(-90 * time.Minute).Format(time.RFC3339Nano),
	}}

	nav := drawNav(t, w)
	for _, want := range []string{"fix the row", displayCwd(cwd), "1h"} {
		if !nav.HasText(want) {
			t.Errorf("the row does not show %q; texts = %v", want, nav.Texts())
		}
	}
	if nav.HasText("feat/sidebar-meta") {
		t.Errorf("the row draws the branch under the path; texts = %v", nav.Texts())
	}
	if got := w.sessionTooltip(w.sessionEntries()[0]); !strings.Contains(got, "feat/sidebar-meta") {
		t.Errorf("the tooltip lost the branch; tooltip = %q", got)
	}
}

// The trailing slot is the row's own child, so the row's items-center centres
// it the way it centres the state glyph: the run time at rest and the × on
// hover both sit at the row's middle. Riding the path's line instead put both
// in the row's bottom half with nothing else there, and it is the one place
// the browser client's row has never put the same slot.
func TestTheTrailingSlotSitsAtTheRowsCentre(t *testing.T) {
	w, _ := newTestShellWindow(t, "fix the row")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home directory: %v", err)
	}
	cwd := filepath.Join(home, "dev", "own", "beacon")
	w.tabs[1].alive = true
	w.live = []protocol.Summary{{
		SessionID: "s1", Title: "fix the row", Cwd: cwd, Alive: true,
		CreatedAt: time.Now().Add(-90 * time.Minute).Format(time.RFC3339Nano),
	}}

	nav := drawNav(t, w)
	row, ok := nav.Find("fix the row")
	if !ok {
		t.Fatalf("the row is missing; texts = %v", nav.Texts())
	}
	mid := row.Y + row.H/2
	run, ok := nav.Find("1h")
	if !ok {
		t.Fatalf("the row has no run time; texts = %v", nav.Texts())
	}
	if got := run.Y + run.H/2; math.Abs(float64(got-mid)) > 0.5 {
		t.Errorf("the run time's middle is %.1f, want the row's %.1f", got, mid)
	}
	if run.X <= row.X+row.W/2 {
		t.Errorf("the run time is not at the trailing edge: time x=%.1f, row x=%.1f w=%.1f", run.X, row.X, row.W)
	}

	// The × takes the same centred place when the pointer arrives.
	nav.Move(row.X+row.W-14, mid)
	nav.Frame()
	kill, ok := nav.Find(locales.TabCloseNamed("fix the row"))
	if !ok {
		t.Fatalf("the hovered row has no close; texts = %v", nav.Texts())
	}
	if got := kill.Y + kill.H/2; math.Abs(float64(got-mid)) > 0.5 {
		t.Errorf("the close's middle is %.1f, want the row's %.1f", got, mid)
	}
}

// A fresh shell is named after its directory. The path line then gives the
// parent, so the two lines together read as one path instead of saying the
// same directory twice.
func TestASessionRowShowsTheParentWhenTheNameIsTheDirectory(t *testing.T) {
	w, _ := newTestShellWindow(t, "beacon")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home directory: %v", err)
	}
	cwd := filepath.Join(home, "dev", "own", "beacon")
	w.tabs[1].alive = true
	w.live = []protocol.Summary{{
		SessionID: "s1", Title: "beacon", Cwd: cwd, Alive: true,
		CreatedAt: time.Now().Add(-5 * time.Minute).Format(time.RFC3339Nano),
	}}

	nav := drawNav(t, w)
	if _, ok := nav.Find(displayCwd(cwd)); ok {
		t.Errorf("the row repeats the directory the name already gives: %v", nav.Texts())
	}
	if !nav.HasText(displayCwd(filepath.Join(home, "dev", "own"))) {
		t.Errorf("the row does not show the directory above the name; texts = %v", nav.Texts())
	}
}

// A row is two lines, its name over its path, and the branch never adds a
// third. Two rows sit 48 points apart plus the list's gap, whatever branch
// the shells stand on, so the list stays as dense as its text allows.
func TestSessionRowsStayTwoLinesWhateverTheirBranch(t *testing.T) {
	w, _ := newTestShellWindow(t, "first", "second")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home directory: %v", err)
	}
	created := time.Now().Add(-20 * time.Minute).Format(time.RFC3339Nano)
	one := filepath.Join(home, "dev", "own", "one")
	two := filepath.Join(home, "dev", "own", "two")
	w.tabs[1].alive = true
	w.tabs[2].alive = true
	w.live = []protocol.Summary{
		{SessionID: "s1", Title: "first", Cwd: one, Alive: true, CreatedAt: created},
		{SessionID: "s2", Title: "second", Cwd: two, Branch: "main", Alive: true, CreatedAt: created},
	}

	nav := drawNav(t, w)
	titleOne, ok := nav.Find("first")
	if !ok {
		t.Fatalf("the first row is missing; texts = %v", nav.Texts())
	}
	titleTwo, ok := nav.Find("second")
	if !ok {
		t.Fatalf("the second row is missing; texts = %v", nav.Texts())
	}
	pathOne, ok := nav.Find(displayCwd(one))
	if !ok {
		t.Fatalf("the first row has no path; texts = %v", nav.Texts())
	}

	if got := titleTwo.Y - titleOne.Y; got != 49 {
		t.Errorf("rows are %.1f apart, want one 48-high row plus its gap (49)", got)
	}
	if got := pathOne.Y - titleOne.Y; got <= 0 {
		t.Errorf("the path is not on the line under the name (offset %.1f)", got)
	}
	for _, branch := range []string{"main"} {
		if nav.HasText(branch) {
			t.Errorf("the row draws the branch %q; texts = %v", branch, nav.Texts())
		}
	}
}

// The panes under a split follow the session row's rule: the name over the
// directory, the run time at rest, and no branch line. Pane rows and session
// rows are the same height, so a split reads as one list, not two kinds of row.
func TestSplitPaneRowsFollowTheSessionRowRule(t *testing.T) {
	w, _ := newTestShellWindow(t, "one", "two", "three")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home directory: %v", err)
	}
	dir := func(name string) string { return filepath.Join(home, "dev", "own", name) }
	third := &splitNode{tab: w.tabs[3]}
	w.split = &splitNode{
		side: true, pct: 50,
		a: &splitNode{tab: w.tabs[1]},
		b: &splitNode{side: true, pct: 50, a: &splitNode{tab: w.tabs[2]}, b: third},
	}
	w.focus = third
	w.selected = 1
	w.tabs[1].alive, w.tabs[2].alive, w.tabs[3].alive = true, true, true
	created := time.Now().Add(-2 * time.Hour).Format(time.RFC3339Nano)
	w.live = []protocol.Summary{
		{SessionID: "s1", Title: "one", Cwd: dir("api"), Branch: "main", Alive: true, CreatedAt: created},
		{SessionID: "s2", Title: "two", Cwd: dir("web"), Branch: "feat/web", Alive: true, CreatedAt: created},
		{SessionID: "s3", Title: "three", Cwd: dir("docs"), Branch: "feat/docs", Alive: true, CreatedAt: created},
	}

	nav := drawNav(t, w)
	for _, want := range []string{displayCwd(dir("web")), displayCwd(dir("docs")), "2h"} {
		if !nav.HasText(want) {
			t.Errorf("the panes do not show %q; texts = %v", want, nav.Texts())
		}
	}
	for _, branch := range []string{"main", "feat/web", "feat/docs"} {
		if nav.HasText(branch) {
			t.Errorf("a pane row draws the branch %q; texts = %v", branch, nav.Texts())
		}
	}
	two, ok := nav.Find("two")
	if !ok {
		t.Fatalf("the second pane is missing; texts = %v", nav.Texts())
	}
	three, ok := nav.Find("three")
	if !ok {
		t.Fatalf("the third pane is missing; texts = %v", nav.Texts())
	}
	if got := three.Y - two.Y; got != 49 {
		t.Errorf("pane rows are %.1f apart, want the session row's 49", got)
	}
}

// The lease is not written on the row: the state dot already leads it, and the
// tooltip is where the exact words live. A row wearing 只读 over the name is
// the thing this design took away.
func TestASessionRowDoesNotWriteTheLeaseAsAChip(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	w.fleet = newTestFleet(t)
	tb := w.tabs[1]
	tb.alive = true
	tb.setLease(protocol.RoleViewer, "c9", 4)
	w.live = []protocol.Summary{{
		SessionID: "s1", Title: "one", Controller: "c9", Alive: true,
		CreatedAt: time.Now().Add(-10 * time.Minute).Format(time.RFC3339Nano),
	}}

	nav := drawNav(t, w)
	if nav.HasText(locales.StateReadonly()) {
		t.Errorf("the row writes 只读 over its name; texts = %v", nav.Texts())
	}
	if got := w.sessionTooltip(w.sessionEntries()[0]); !strings.Contains(got, locales.StateControllerOther("c9")) {
		t.Errorf("the tooltip lost the holder; tooltip = %q", got)
	}

	tb.alive = false
	tb.finish(1)
	nav = drawNav(t, w)
	if nav.HasText(locales.StateEnded()) {
		t.Errorf("the row writes 已退出 over its name; texts = %v", nav.Texts())
	}
}

// The trailing slot holds one thing at a time: the run time at rest, the ×
// while the pointer is on the row. They share the place, so the time never
// moves when the close arrives, and the close never squeezes the time.
func TestTheRunTimeGivesItsPlaceToTheCloseOnHover(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	w.fleet = newTestFleet(t)
	w.tabs[1].alive = true
	w.live = []protocol.Summary{{
		SessionID: "s1", Title: "one", Alive: true,
		CreatedAt: time.Now().Add(-2 * time.Hour).Format(time.RFC3339Nano),
	}}

	nav := drawNav(t, w)
	if !nav.HasText("2h") {
		t.Fatalf("the row does not show the run time at rest; texts = %v", nav.Texts())
	}
	if _, ok := nav.Find(locales.TabCloseNamed("one")); ok {
		t.Fatalf("the close is drawn before the pointer arrives; texts = %v", nav.Texts())
	}

	row, ok := nav.Find("one")
	if !ok {
		t.Fatalf("the row is missing; texts = %v", nav.Texts())
	}
	nav.Move(row.X+120, row.Y+row.H/2)
	nav.Frame()
	if nav.HasText("2h") {
		t.Errorf("the run time stays written under the close on hover; texts = %v", nav.Texts())
	}
	if _, ok := nav.Find(locales.TabCloseNamed("one")); !ok {
		t.Errorf("the hovered row has no close; texts = %v", nav.Texts())
	}
}

// Run time is the shell's life, not a state's age, and it stops at the moment
// the shell exited. A host that did not record the exit says nothing rather
// than letting the number grow.
func TestRuntimeTextIsTheShellsLifeAndFreezesAtItsEnd(t *testing.T) {
	now := time.Now()
	created := now.Add(-2 * time.Hour)
	ended := now.Add(-1 * time.Hour)

	if got := runtimeText(sessionEntry{createdAt: created}, now); got != "2h" {
		t.Errorf("a live shell's run time = %q, want 2h", got)
	}
	if got := runtimeText(sessionEntry{createdAt: created, endedAt: ended}, now); got != "1h" {
		t.Errorf("an exited shell's run time = %q, want the frozen 1h", got)
	}
	if got := runtimeText(sessionEntry{createdAt: now.Add(-30 * time.Second)}, now); got != "<1m" {
		t.Errorf("a fresh shell's run time = %q, want <1m", got)
	}
	if got := runtimeText(sessionEntry{}, now); got != "" {
		t.Errorf("a row with no creation time shows %q, want nothing", got)
	}
	if got := runtimeText(sessionEntry{createdAt: created, endedAt: created.Add(-time.Minute)}, now); got != "" {
		t.Errorf("an exit before creation shows %q, want nothing", got)
	}
}
