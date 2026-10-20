package main

// The window's frame on Windows and Linux: the system's buttons are off, the
// view draws its own in the top-right card's header, and the takeover strip is
// laid out under that header rather than under the buttons.

import (
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/protocol"
	"github.com/CarlOpenLab/term-fleet/internal/uitokens"
	"github.com/CarlOpenLab/term-fleet/locales"
)

// fakeWindowFrame is the window the controls act on in a test: a headless tester
// has no real one, so it counts the calls instead.
type fakeWindowFrame struct {
	minimized  int
	maximized  int
	restored   int
	closed     int
	stillMaxed bool
}

func (f *fakeWindowFrame) Minimize()         { f.minimized++ }
func (f *fakeWindowFrame) Maximize()         { f.maximized++; f.stillMaxed = true }
func (f *fakeWindowFrame) Unmaximize()       { f.restored++; f.stillMaxed = false }
func (f *fakeWindowFrame) Close()            { f.closed++ }
func (f *fakeWindowFrame) IsMaximized() bool { return f.stillMaxed }

// frameTestWidth is the window the frame tests draw at. The close button's
// centre is frameButtonCenter(2).
const frameTestWidth = 900

// frameButtonCenter is where a control sits, from the constants the layout is
// made of: the row is right-aligned one inset from the window's edge, and the
// buttons are winButtonW wide with winButtonGap between them.
func frameButtonCenter(i int) (x, y float32) {
	right := float32(frameTestWidth - uitokens.CardGap - 10)
	x = right - float32(2-i)*(winButtonW+winButtonGap) - winButtonW/2
	y = uitokens.CardGap + (uitokens.HeaderH-winButtonH)/2 + winButtonH/2
	return x, y
}

// controlsTestWindow is a window with a shell on screen, the app's own controls
// forced on, and a frame to count what they do.
func controlsTestWindow(t *testing.T) (*window, *fakeWindowFrame) {
	t.Helper()
	before := ownWindowControls
	ownWindowControls = true
	t.Cleanup(func() { ownWindowControls = before })
	w := previewWindow(t, themeLight, true)
	f := &fakeWindowFrame{}
	w.frameStub = f
	return w, f
}

func TestFramedWindowTurnsTheSystemsControlsOff(t *testing.T) {
	before := ownWindowControls
	t.Cleanup(func() { ownWindowControls = before })

	ownWindowControls = true
	o := framedWindow(mygo.WindowOptions{TitleBarStyle: mygo.TitleBarHidden})
	if o.TitleBarStyle != mygo.TitleBarHidden {
		t.Errorf("TitleBarStyle = %q, want the hidden one", o.TitleBarStyle)
	}
	if !o.HideWindowButtons {
		t.Error("the window kept the system's controls, which would sit over the header's")
	}
	if !o.AutoHideMenuBar {
		t.Error("the window kept a menu bar under a title bar it draws itself")
	}

	// macOS keeps its traffic lights: the frame hides the title bar and the
	// system's buttons stay where every Mac app's are.
	ownWindowControls = false
	o = framedWindow(mygo.WindowOptions{TitleBarStyle: mygo.TitleBarHidden})
	if o.HideWindowButtons || o.AutoHideMenuBar {
		t.Error("a window with traffic lights had them hidden as well")
	}
	if o.TitleBarStyle != mygo.TitleBarHidden {
		t.Errorf("TitleBarStyle = %q, want the hidden one", o.TitleBarStyle)
	}
}

// TestTheWindowsControlsActOnTheWindow presses all three where they are drawn.
func TestTheWindowsControlsActOnTheWindow(t *testing.T) {
	w, f := controlsTestWindow(t)
	tt := ui.NewTester(w.view, frameTestWidth, uiHeightForFrameTest)
	tt.Frame()

	minX, y := frameButtonCenter(0)
	tt.ClickAt(minX, y)
	tt.Frame()
	if f.minimized != 1 {
		t.Fatalf("the minimize control called Minimize %d times, want once", f.minimized)
	}

	maxX, _ := frameButtonCenter(1)
	tt.ClickAt(maxX, y)
	tt.Frame()
	if f.maximized != 1 || !f.stillMaxed {
		t.Fatalf("the maximize control did not maximize: %+v", f)
	}
	tt.ClickAt(maxX, y)
	tt.Frame()
	if f.restored != 1 || f.stillMaxed {
		t.Fatalf("the maximize control did not restore a maximized window: %+v", f)
	}

	closeX, _ := frameButtonCenter(2)
	tt.ClickAt(closeX, y)
	tt.Frame()
	if f.closed != 1 {
		t.Fatalf("the close control called Close %d times, want once", f.closed)
	}
}

const uiHeightForFrameTest = 600

// TestAnEmptyWindowStillHasItsControls draws the window with no shell: the
// controls belong to the frame, not to a terminal, and a window with no way to
// close is worse than an empty one.
func TestAnEmptyWindowStillHasItsControls(t *testing.T) {
	before := ownWindowControls
	ownWindowControls = true
	t.Cleanup(func() { ownWindowControls = before })

	w, _ := newTestShellWindow(t)
	w.fleet = newTestFleet(t)
	f := &fakeWindowFrame{}
	w.frameStub = f

	tt := ui.NewTester(w.view, frameTestWidth, uiHeightForFrameTest)
	tt.Frame()
	x, y := frameButtonCenter(2)
	tt.ClickAt(x, y)
	tt.Frame()
	if f.closed != 1 {
		t.Fatalf("an empty window has no close control: %+v", f)
	}
}

// TestTheSystemDrawsTheControlsOnMacOS is the other side: with the traffic
// lights showing, the view draws nothing where they would be.
func TestTheSystemDrawsTheControlsOnMacOS(t *testing.T) {
	before := ownWindowControls
	ownWindowControls = false
	t.Cleanup(func() { ownWindowControls = before })

	w := previewWindow(t, themeLight, true)
	f := &fakeWindowFrame{}
	w.frameStub = f

	tt := ui.NewTester(w.view, frameTestWidth, uiHeightForFrameTest)
	tt.Frame()
	x, y := frameButtonCenter(2)
	tt.ClickAt(x, y)
	tt.Frame()
	if f.closed != 0 {
		t.Fatalf("a window with traffic lights drew its own close control: %+v", f)
	}
}

// TestTheTakeoverStripStaysUnderTheHeaderAndClearOfTheControls is the layout
// the report was about: the sentence about who holds the shell, and the button
// that takes it back, must not be under the window's buttons.
func TestTheTakeoverStripStaysUnderTheHeaderAndClearOfTheControls(t *testing.T) {
	w, _ := controlsTestWindow(t)
	w.tabs[1].setLease(protocol.RoleViewer, "c1", 4)

	tt := ui.NewTester(w.view, frameTestWidth, uiHeightForFrameTest)
	tt.Frame()

	take, ok := tt.Find(locales.ActionTakeBack())
	if !ok {
		t.Fatalf("the takeover button is not on screen: texts = %v", tt.Texts())
	}
	// The strip starts under the header, where the window's controls are.
	if top := float32(uitokens.CardGap + uitokens.HeaderH); take.Y < top {
		t.Errorf("the takeover button is at y=%v, inside the header (controls at %v)", take.Y, top)
	}
	if right := take.X + take.W; right > float32(frameTestWidth-uitokens.CardGap-10) {
		t.Errorf("the takeover button reaches x=%v, past the card's inset", right)
	}

	// The header's own last chip ends before the controls begin: the controls
	// are at the window's right edge, and the top-right header keeps that much
	// room (winControlsInset).
	chip, ok := tt.Find("claude")
	if !ok {
		t.Fatalf("the program chip is not on screen: texts = %v", tt.Texts())
	}
	clear := float32(frameTestWidth - uitokens.CardGap - int(winControlsInset)) // 786
	if right := chip.X + chip.W; right > clear {
		t.Errorf("the header's chip ends at x=%v, under the window controls (clear up to %v)", right, clear)
	}
}

// TestTheTopRightPaneOfASplitKeepsClearOfTheControls is the same rule in a
// split: only the card that reaches the window's right edge keeps the room, and
// it does keep it.
func TestTheTopRightPaneOfASplitKeepsClearOfTheControls(t *testing.T) {
	before := ownWindowControls
	ownWindowControls = true
	t.Cleanup(func() { ownWindowControls = before })

	w := previewWindow(t, themeLight, true)
	w.frameStub = &fakeWindowFrame{}

	right := &tab{id: "s3", machine: machineLocal, title: "~/work/api",
		ordinal: 2, term: fedTerminal(t, previewScreen()), alive: true}
	right.setLease(protocol.RoleViewer, "c1", 3)
	w.tabs[2] = right
	w.order = append(w.order, 2)
	w.split = &splitNode{
		side: true, pct: splitHalf,
		a: &splitNode{tab: w.tabs[1]}, b: &splitNode{tab: right},
	}
	w.focus = w.split.a
	w.selectTabRaw(w.tabs[1])

	tt := ui.NewTester(func(c *ui.Context) {
		applyTheme(c, themeLight)
		w.body(c, paletteFor(c))
	}, frameTestWidth, uiHeightForFrameTest)
	tt.Frame()

	// The right pane is a viewer, so its header carries 只读; with no sidebar in
	// this picture that is the only one on screen.
	chip, ok := tt.Find(locales.StateReadonly())
	if !ok {
		t.Fatalf("the taken pane does not say 只读: texts = %v", tt.Texts())
	}
	clear := float32(frameTestWidth) - winControlsInset
	if edge := chip.X + chip.W; edge > clear {
		t.Errorf("the taken pane's 只读 chip ends at x=%v, under the window controls (clear up to %v)", edge, clear)
	}
}

// TestTheLeaseGlowDoesNotShrinkThePane reads the reported bug off the pixels:
// a taken shell lit at the edge was also a shell half its height, because the
// glow's wrapper was a Fill sibling of the card instead of an overlay. The card's
// ground at the bottom is the same colour whether or not the light is on.
func TestTheLeaseGlowDoesNotShrinkThePane(t *testing.T) {
	w := previewWindow(t, themeLight, true)

	plain := ui.NewTester(w.view, frameTestWidth, uiHeightForFrameTest)
	plain.Frame()
	r1, g1, b1, _ := plain.Image().At(700, 560).RGBA()

	w.tabs[1].setLease(protocol.RoleViewer, "c9", 4)
	lit := ui.NewTester(w.view, frameTestWidth, uiHeightForFrameTest)
	lit.Frame()
	r2, g2, b2, _ := lit.Image().At(700, 560).RGBA()

	if r1 != r2 || g1 != g2 || b1 != b2 {
		t.Errorf("the card's ground changed under the lease glow: plain (%d,%d,%d), lit (%d,%d,%d)",
			r1>>8, g1>>8, b1>>8, r2>>8, g2>>8, b2>>8)
	}
}
