package main

// The body's split tree: the shells on screen when there is more than one of
// them, with a divider the pointer drags between the panes.
//
// A split starts from the pane on screen — ⌘D puts a second shell beside it,
// ⌘⇧D above or below it — and the new shell is an ordinary tab: it keeps
// running when its pane closes, and the sidebar lists it like any other. The
// tree is the window's own view of its tabs, so the sidebar stays the roster
// and the panes stay what is on screen.
//
// The focused leaf is the pane the window acts on: the keyboard types into it,
// its header names it, and ⌘W closes it. It is nil only while the body is not
// split at all, which is the ordinary single-pane case.

import (
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/uitokens"
	"github.com/CarlOpenLab/term-fleet/locales"
)

// splitNode is one leaf or one branch of the split tree. A leaf holds a tab; a
// branch holds two children and how the room between them is shared.
type splitNode struct {
	// tab is the shell a leaf shows. It is nil on a branch, which is what tells
	// the two apart.
	tab *tab

	// side is true when a branch's children sit beside each other (⌘D) and
	// false when one is above the other (⌘⇧D).
	side bool
	// pct is the percentage of the room the first child takes, 0 to 100, which
	// the divider between the panes drags.
	pct float32

	a, b *splitNode
}

// splitHalf is the room a new split gives each side: the divider starts in the
// middle and moves from there.
const splitHalf = float32(50)

// leaf reports whether the node is a leaf — the end of a branch, holding a tab.
func (n *splitNode) leaf() bool { return n != nil && n.tab != nil }

// firstLeaf is the leaf a subtree starts at: the one a pane closing leaves the
// focus on when the pane that had it is gone.
func (n *splitNode) firstLeaf() *splitNode {
	for n != nil && n.tab == nil {
		if n.a != nil {
			n = n.a
		} else {
			n = n.b
		}
	}
	return n
}

// lastLeaf is the leaf a subtree ends at, the mirror of firstLeaf.
func (n *splitNode) lastLeaf() *splitNode {
	for n != nil && n.tab == nil {
		if n.b != nil {
			n = n.b
		} else {
			n = n.a
		}
	}
	return n
}

// count is how many panes the subtree holds.
func (n *splitNode) count() int {
	if n == nil {
		return 0
	}
	if n.tab != nil {
		return 1
	}
	return n.a.count() + n.b.count()
}

// splitFocused splits the pane on screen in two, with a fresh shell on the far
// side of the new divider. side is true for panes beside each other (⌘D) and
// false for one above the other (⌘⇧D).
//
// The new shell is made on the machine of the shell being split — splitting a
// shell on another machine gives another shell there — and opens where that
// shell stands, which the far hub answers from the `from` the create carries.
func (w *window) splitFocused(side bool) {
	on := w.activeTab()
	if on == nil {
		return
	}
	machine := on.machine
	if machine == "" {
		machine = machineLocal
	}
	var fresh *tab
	if machine == machineLocal {
		fresh = w.startLocalTab()
	} else {
		fresh = w.startTabOn(machine)
	}
	if fresh == nil {
		return
	}
	leaf := &splitNode{tab: fresh}
	w.zoomed = false
	w.insertSplit(on, leaf, side)
	w.focusSplit(leaf)
}

// insertSplit puts leaf beside the pane on screen, turning that pane into a
// branch of the two. on is the shell the body was showing before leaf's tab was
// opened: opening a tab makes it the selected one, so with no split yet the
// pane to divide has to be named from before that happened. side is true for
// panes beside each other (⌘D) and false for one above the other (⌘⇧D).
func (w *window) insertSplit(on *tab, leaf *splitNode, side bool) {
	if !leaf.leaf() {
		return
	}
	at := w.focus
	if w.split == nil {
		if on == nil {
			return
		}
		at = &splitNode{tab: on}
		w.split = at
	}
	if !at.leaf() {
		return
	}
	w.split = replaceSplit(w.split, at, &splitNode{side: side, pct: splitHalf, a: at, b: leaf})
	w.focus = leaf
}

// closeSplitPane closes the pane on screen: the shell keeps running and its tab
// stays in the strip, exactly as closing a tab leaves a shell running. With one
// pane there is no split to close, and ⌘W closes the tab instead.
func (w *window) closeSplitPane() {
	if w.split != nil && w.focus.leaf() {
		tb := w.focus.tab
		w.collapseLeaf(w.focus, true)
		w.syncSplit()
		if tb != nil {
			w.setNotice(locales.AppSplitClosed(tb.name()))
		}
		return
	}
	if tb := w.activeTab(); tb != nil {
		w.removeTab(tb)
	}
}

// collapseLeaf takes a leaf out of the tree, replacing the branch that held it
// with the leaf's sibling. settle decides whether a tree left with one pane
// goes back to being the whole body right away: a caller that is about to
// change the tab set waits, because selecting the pane that is left would name
// a strip index that is about to move.
func (w *window) collapseLeaf(n *splitNode, settle bool) {
	if w.split == nil || !n.leaf() {
		return
	}
	if w.split == n {
		w.split, w.focus = nil, nil
		return
	}
	parent := w.parentOf(n)
	if parent == nil {
		return
	}
	sibling := parent.a
	if sibling == n {
		sibling = parent.b
	}
	w.split = replaceSplit(w.split, parent, sibling)
	if w.focus == n {
		w.focus = sibling.firstLeaf()
	}
	if settle {
		w.settleSplit()
	}
}

// settleSplit puts the window back to one pane when the split tree has been
// whittled down to a single leaf: a tree of one pane is the ordinary body, and
// keeping it would leave two ways to say what is on screen.
func (w *window) settleSplit() {
	if w.split == nil || !w.split.leaf() {
		return
	}
	tb := w.split.tab
	w.split, w.focus, w.zoomed = nil, nil, false
	if tb != nil {
		w.selectTabRaw(tb)
	}
}

// syncSplit re-points the window at the focused pane after the tab set changed:
// the strip's selection follows the pane on screen, and a tree left with one
// pane goes back to being the whole body.
func (w *window) syncSplit() {
	w.settleSplit()
	if w.split != nil && w.focus != nil && w.focus.tab != nil {
		w.selectTabRaw(w.focus.tab)
	}
}

// focusSplit makes a leaf the pane on screen: the one the composer types into
// and the header describes. The strip follows it, because a strip that
// highlights a shell nobody is looking at is a strip that lies.
func (w *window) focusSplit(n *splitNode) {
	if !n.leaf() {
		return
	}
	w.focus = n
	w.selectTabRaw(n.tab)
}

// leafOf is the leaf showing a tab, or nil when the tree has no such pane.
func (w *window) leafOf(tb *tab) *splitNode {
	if w.split == nil || tb == nil {
		return nil
	}
	var walk func(*splitNode) *splitNode
	walk = func(n *splitNode) *splitNode {
		if n == nil {
			return nil
		}
		if n.tab == tb {
			return n
		}
		if found := walk(n.a); found != nil {
			return found
		}
		return walk(n.b)
	}
	return walk(w.split)
}

// parentOf is the branch holding a node, or nil for the root.
func (w *window) parentOf(n *splitNode) *splitNode {
	var walk func(*splitNode) *splitNode
	walk = func(nd *splitNode) *splitNode {
		if nd == nil {
			return nil
		}
		if nd.a == n || nd.b == n {
			return nd
		}
		if found := walk(nd.a); found != nil {
			return found
		}
		return walk(nd.b)
	}
	return walk(w.split)
}

// replaceSplit swaps one node of a tree for another and returns the new root.
func replaceSplit(root, from, to *splitNode) *splitNode {
	if root == from {
		return to
	}
	if root == nil {
		return nil
	}
	root.a = replaceSplit(root.a, from, to)
	root.b = replaceSplit(root.b, from, to)
	return root
}

// splitView draws a subtree. A leaf is a shell on its card; a branch is two of
// them with the gap between, which the pointer drags from a grip that shows on
// the hover, and a double click puts back in the middle. top and right say the
// subtree touches the window's top and right edges, which is what tells the
// top-right card to keep its corner clear for the window's own controls.
func (w *window) splitView(c *ui.Context, pal *uitokens.Palette, n *splitNode, top, right bool) {
	if n == nil {
		return
	}
	if n.leaf() {
		w.paneCard(c, pal, n.tab, n == w.focus, top, right, func() { w.focusSplit(n) })
		return
	}
	pct := n.pct
	if pct <= 0 || pct >= 100 {
		pct = 50
	}
	box := ui.Row(c)
	if !n.side {
		box = ui.Column(c)
	}
	box.Fill().MinWidth(0).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
		first := ui.Column(c).Grow(pct).Basis(0).MinWidth(0).MinHeight(0).AlignItems(ui.Stretch)
		first.Children(func() {
			// Beside each other, the first card is the left one and does not
			// reach the right edge; stacked, it is the top one.
			w.splitView(c, pal, n.a, top, right && !n.side)
		})
		w.splitGap(c, pal, n, pct)
		second := ui.Column(c).Grow(100 - pct).Basis(0).MinWidth(0).MinHeight(0).AlignItems(ui.Stretch)
		second.Children(func() {
			// Beside each other, the second card is the right one and shares the
			// top edge; stacked, it is the bottom one and does not reach the top.
			w.splitView(c, pal, n.b, top && n.side, right)
		})
	})
}

// splitGap is the room between two panes, which a drag moves: a gap wide enough
// to see, and the grip that shows while the pointer is in it.
func (w *window) splitGap(c *ui.Context, pal *uitokens.Palette, n *splitNode, pct float32) {
	d := ui.Box(c).Shrink(0)
	if n.side {
		d.Width(uitokens.PaneGap).Cursor(ui.CursorResizeEW)
	} else {
		d.Height(uitokens.PaneGap).Cursor(ui.CursorResizeNS)
	}
	hovered := d.Hovered() || d.Pressed()
	if hovered {
		// A short grip, so the gap shows it moves.
		d.Draw(func(p *ui.Painter, r ui.Rect) {
			if n.side {
				p.Fill(ui.Rect{X: r.X + r.W/2 - 1.5, Y: r.Y + r.H/2 - 18, W: 3, H: 36}, pal.TextFaint.Alpha(0.7), 1.5)
			} else {
				p.Fill(ui.Rect{X: r.X + r.W/2 - 18, Y: r.Y + r.H/2 - 1.5, W: 36, H: 3}, pal.TextFaint.Alpha(0.7), 1.5)
			}
		})
	}
	if dx, dy, ok := d.Dragged(); ok {
		b := d.Bounds()
		size, delta := b.W, dx
		if !n.side {
			size, delta = b.H, dy
		}
		if size > 0 {
			n.pct = min(max(pct+delta/size*100, 12), 88)
		}
	}
	if d.DoubleClicked() {
		n.pct = 50
	}
}

// cyclePane moves the focus to the next or previous pane of the split, in the
// order the body draws them: kero's ⌘] and ⌘[.
func (w *window) cyclePane(delta int) {
	leaves := w.workspaceLeaves()
	if len(leaves) < 2 {
		return
	}
	cur := w.activeTab()
	at := -1
	for i, tb := range leaves {
		if tb == cur {
			at = i
			break
		}
	}
	if at < 0 {
		return
	}
	next := (at + delta + len(leaves)) % len(leaves)
	if n := w.leafOf(leaves[next]); n != nil {
		w.focusSplit(n)
	}
}

// zoomFocused fills the body with the focused pane, and puts the split back on
// the next press: kero's ⌘⇧↩. It is a view of the split, not another layout.
func (w *window) zoomFocused() {
	if w.split == nil {
		return
	}
	w.zoomed = !w.zoomed
	w.invalidate()
}

// focusDirection moves the focus to the nearest pane that lies in a direction,
// walking up the tree until a branch divides that way: kero's ⌘⌥arrows.
func (w *window) focusDirection(dir string) {
	leaf := w.focus
	if w.split == nil || leaf == nil {
		return
	}
	horizontal := dir == "left" || dir == "right"
	// forward is the side of a division the direction points at: right and down
	// are the second child, left and up the first.
	forward := dir == "right" || dir == "down"
	for n := leaf; ; {
		parent := w.parentOf(n)
		if parent == nil {
			return
		}
		if parent.side == horizontal {
			inFirst := w.contains(parent.a, leaf)
			// The leaf is on the near side of this division: the pane the
			// direction points at is the sibling's.
			if inFirst == forward {
				var target *splitNode
				if forward {
					target = parent.b.firstLeaf()
				} else {
					target = parent.a.lastLeaf()
				}
				if target != nil {
					w.focusSplit(target)
					return
				}
			}
		}
		n = parent
	}
}

// contains reports whether a leaf is inside a subtree.
func (w *window) contains(n, leaf *splitNode) bool {
	if n == nil || leaf == nil {
		return false
	}
	if n == leaf {
		return true
	}
	return w.contains(n.a, leaf) || w.contains(n.b, leaf)
}
