package main

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/egoist/mygo/ui"
)

// newSplitWindow is a window with its own hub behind it, the way the app's
// window is wired: the shells it opens are real, and the frames that name them
// come back over the window's own link.
func newSplitWindow(t *testing.T) (*window, *fleet) {
	t.Helper()
	f := newTestFleet(t)
	w, _ := newTestShellWindow(t)
	w.fleet = f
	w.local = openLocalLink(f.Hub())
	w.local.onSessions = w.sessionsArrived
	w.local.onGone = w.linkGone
	return w, f
}

// testShell is the program a test's shell runs: sh where there is one, and
// cmd.exe on Windows, which is the only choice that works everywhere.
func testShell() string {
	if runtime.GOOS == "windows" {
		return "cmd.exe"
	}
	return "/bin/sh"
}

// openShell gives the window one shell of its own, in dir, and waits until the
// hub has named it and the window has adopted the tab.
func openShell(t *testing.T, w *window, f *fleet, dir string) *tab {
	t.Helper()
	tb := w.openTab(machineLocal, "")
	if tb == nil {
		t.Fatal("the window opened no tab")
	}
	w.local.create(80, 24, dir, testShell(), "")
	waitSessions(f, liveCount(f)+1)
	settleTabs(t, w, len(w.order))
	w.drain()
	return tb
}

// drawWindow draws the whole window the way its view does, so the shortcuts and
// the body are the real ones.
func drawWindow(w *window) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) {
		applyTheme(c, w.store.Get().Theme)
		w.view(c)
	}, 900, 600)
}

// otherTab is the window's one tab that is not the one given.
func otherTab(t *testing.T, w *window, not *tab) *tab {
	t.Helper()
	for _, key := range w.order {
		if tb := w.tabs[key]; tb != nil && tb != not {
			return tb
		}
	}
	t.Fatalf("the window holds no tab besides %v", not)
	return nil
}

// resolvedDir resolves symlinks so a comparison holds on a machine where the
// temporary directory is one: macOS keeps /var as a link, and the operating
// system answers with the real path.
func resolvedDir(t *testing.T, dir string) string {
	t.Helper()
	got, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return dir
	}
	return got
}

// ⌘D splits the shell on screen beside a fresh one, and the fresh shell opens
// where the shell beside it stands — not at the home directory. That is the
// whole point of the key: a person who has cd'd somewhere wants the second pane
// there, without typing the path again.
func TestSplitOpensTheNewShellWhereTheShellOnScreenStands(t *testing.T) {
	w, f := newSplitWindow(t)
	dir := t.TempDir()
	first := openShell(t, w, f, dir)

	tt := drawWindow(w)
	tt.Key(ui.Cmd, ui.KeyD)
	tt.Frame()

	drainFor(t, w, func() bool { return w.split != nil && w.split.count() == 2 })
	if !w.split.side {
		t.Fatal("⌘D should put the two panes beside each other")
	}
	second := otherTab(t, w, first)
	if w.activeTab() != second {
		t.Fatal("the pane ⌘D opened is not the one on screen")
	}

	// The second shell exists, and it was born in the directory the first one
	// stands in.
	waitSessions(f, 2)
	settleTabs(t, w, 2)
	w.drain()
	want := resolvedDir(t, dir)
	for _, s := range f.Hub().Sessions() {
		if s.SessionID != second.id {
			continue
		}
		if s.Cwd != want {
			t.Fatalf("the split shell opened in %q, want the directory the first shell stands in, %q", s.Cwd, want)
		}
	}

	// Both panes are laid out: each holds a terminal the split gave a size to.
	for _, n := range splitLeaves(w.split) {
		if n.tab.term == nil {
			t.Fatal("a pane of the split has no terminal")
		}
		if cols, rows := n.tab.term.Size(); cols <= 0 || rows <= 0 {
			t.Fatalf("a pane of the split was laid out at %dx%d", cols, rows)
		}
	}
}

// ⌘⇧D is the same split with the panes one above the other.
func TestShiftCmdDSplitsAboveAndBelow(t *testing.T) {
	w, f := newSplitWindow(t)
	openShell(t, w, f, t.TempDir())

	tt := drawWindow(w)
	tt.Key(ui.Cmd|ui.Shift, ui.KeyD)
	tt.Frame()

	drainFor(t, w, func() bool { return w.split != nil && w.split.count() == 2 })
	if w.split.side {
		t.Fatal("⌘⇧D should stack the panes, not put them beside each other")
	}
}

// ⌘W over a split closes the pane, not the shell: the process keeps running and
// its tab stays in the strip, the way closing any tab leaves a shell running.
func TestClosingAPaneLeavesItsShellRunning(t *testing.T) {
	w, f := newSplitWindow(t)
	openShell(t, w, f, t.TempDir())

	tt := drawWindow(w)
	tt.Key(ui.Cmd, ui.KeyD)
	tt.Frame()
	drainFor(t, w, func() bool { return w.split != nil && w.split.count() == 2 })
	waitSessions(f, 2)
	settleTabs(t, w, 2)
	w.drain()

	tt.Key(ui.Cmd, ui.KeyW)
	tt.Frame()
	drainFor(t, w, func() bool { return w.split == nil })

	if got := liveCount(f); got != 2 {
		t.Fatalf("closing the pane left %d live shells, want both still running", got)
	}
	if len(w.order) != 2 {
		t.Fatalf("closing the pane left %d tabs, want both still in the strip", len(w.order))
	}
}

// ⌘T with a split open gives the pane on screen a new shell: the split keeps
// its shape and the shell the pane had stays in the strip, still running.
func TestNewTabInASplitLandsInThePaneOnScreen(t *testing.T) {
	w, f := newSplitWindow(t)
	first := openShell(t, w, f, t.TempDir())

	tt := drawWindow(w)
	tt.Key(ui.Cmd, ui.KeyD)
	tt.Frame()
	drainFor(t, w, func() bool { return w.split != nil && w.split.count() == 2 })
	second := w.activeTab()

	tt.Key(ui.Cmd, ui.KeyT)
	tt.Frame()
	waitSessions(f, 3)
	settleTabs(t, w, 3)
	w.drain()

	if len(w.order) != 3 {
		t.Fatalf("the window holds %d tabs, want the two shells and the new one", len(w.order))
	}
	if w.split == nil || w.split.count() != 2 {
		t.Fatal("⌘T took the split apart")
	}
	third := w.activeTab()
	if third == first || third == second {
		t.Fatal("⌘T did not put the new shell in the pane on screen")
	}
	if got := liveCount(f); got != 3 {
		t.Fatalf("%d live shells, want all three", got)
	}
}

// Picking a shell that is already a pane moves the focus to it; picking one
// that is not puts it in the pane on screen. Either way the split survives,
// because the strip is the roster and the panes are what is on screen.
func TestTheStripMovesTheFocusAndKeepsTheSplit(t *testing.T) {
	w, f := newSplitWindow(t)
	first := openShell(t, w, f, t.TempDir())

	tt := drawWindow(w)
	tt.Key(ui.Cmd, ui.KeyD)
	tt.Frame()
	drainFor(t, w, func() bool { return w.split != nil && w.split.count() == 2 })
	second := w.activeTab()

	// A third shell, opened the way a browser's is: it has a tab and no pane.
	third := w.openTab(machineLocal, "")
	w.local.create(80, 24, "", testShell(), "")
	waitSessions(f, 3)
	settleTabs(t, w, 3)
	w.drain()
	tt.Frame()

	w.selectTab(first)
	if w.activeTab() != first {
		t.Fatal("picking the shell in the first pane did not focus that pane")
	}
	w.selectTab(third)
	if w.activeTab() != third {
		t.Fatal("picking a shell with no pane did not put it on screen")
	}
	if w.split == nil || w.split.count() != 2 {
		t.Fatal("picking a shell took the split apart")
	}
	if w.focus.tab != third {
		t.Fatal("the shell picked from the strip is not in the focused pane")
	}
	// The pane it replaced keeps its shell in the strip, still running.
	if second.id == "" {
		t.Fatal("the shell the focused pane had is gone")
	}
}

// A tab that is a pane goes away with its pane: closing it from the strip takes
// the pane out of the tree and leaves the other one whole.
func TestClosingATabThatIsAPaneLeavesTheOtherPane(t *testing.T) {
	w, f := newSplitWindow(t)
	first := openShell(t, w, f, t.TempDir())

	tt := drawWindow(w)
	tt.Key(ui.Cmd, ui.KeyD)
	tt.Frame()
	drainFor(t, w, func() bool { return w.split != nil && w.split.count() == 2 })
	waitSessions(f, 2)
	settleTabs(t, w, 2)
	w.drain()
	second := otherTab(t, w, first)

	w.removeTab(first)

	if w.split != nil {
		t.Fatal("a tree with one pane left should be the ordinary body again")
	}
	if len(w.order) != 1 || w.tabs[w.order[0]] != second {
		t.Fatalf("the window holds %v, want the second shell alone", w.order)
	}
	if w.activeTab() != second {
		t.Fatal("the shell that is left is not the one on screen")
	}
	if got := liveCount(f); got != 2 {
		t.Fatalf("closing the tab left %d live shells, want both still running", got)
	}
}

// The tree itself: a branch becomes its sibling when a leaf goes, the focus
// lands on the leaf a subtree starts at, and a tree of one pane is the ordinary
// body rather than a split of one.
func TestTheSplitTreeCollapsesToItsSibling(t *testing.T) {
	w := &window{tabs: map[int]*tab{}}
	a := &splitNode{tab: &tab{}}
	b := &splitNode{tab: &tab{}}
	c := &splitNode{tab: &tab{}}
	d := &splitNode{tab: &tab{}}
	// ((a | b) over c), with the focus on a.
	inner := &splitNode{side: true, pct: 50, a: a, b: b}
	root := &splitNode{pct: 50, a: inner, b: c}
	w.split, w.focus = root, a

	w.collapseLeaf(a, false)
	if w.split != root {
		t.Fatal("collapsing a leaf of a branch moved the root")
	}
	if root.a != b {
		t.Fatal("the branch that held the closed leaf kept it instead of taking its sibling")
	}
	if w.focus != b {
		t.Fatal("the focus did not land on the sibling's subtree")
	}

	// The last leaf of that branch goes too, and the tree settles to one pane.
	w.collapseLeaf(b, true)
	if w.split != nil || w.focus != nil {
		t.Fatalf("a tree of one pane should have settled back to the ordinary body, got %v", w.split)
	}

	// And a leaf split out of the root replaces the root, rather than leaving
	// the old root hanging beside it.
	w.split, w.focus = d, d
	w.insertSplit(d.tab, &splitNode{tab: &tab{}}, true)
	if w.split.a != d || w.split.b == nil {
		t.Fatalf("splitting the root left the tree as %v", w.split)
	}
	if w.split.count() != 2 {
		t.Fatalf("the split holds %d panes, want 2", w.split.count())
	}
}

// splitLeaves is every leaf of a tree, in order.
func splitLeaves(n *splitNode) []*splitNode {
	if n == nil {
		return nil
	}
	if n.leaf() {
		return []*splitNode{n}
	}
	return append(splitLeaves(n.a), splitLeaves(n.b)...)
}
