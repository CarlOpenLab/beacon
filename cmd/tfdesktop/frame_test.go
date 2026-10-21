package main

// The window's frame: the system's buttons are off, and the view draws its own,
// as macOS draws its traffic lights. The takeover strip is laid out under the
// card's header, where the sentence belongs, rather than under any controls.

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

// frameTestWidth is the window the frame tests draw at.
const frameTestWidth = 900

// trafficLightCenter is where one of the three dots sits, from the constants the
// layout is made of: close, minimize and zoom, left to right.
func trafficLightCenter(i int) (x, y float32) {
	x = trafficLightLeft + float32(i)*(trafficLightSize+trafficLightGap) + trafficLightSize/2
	y = uitokens.TopBar / 2
	return x, y
}

// controlsTestWindow is a window with a shell on screen and a frame to count
// what the controls do.
func controlsTestWindow(t *testing.T) (*window, *fakeWindowFrame) {
	t.Helper()
	w := previewWindow(t, themeLight, true)
	f := &fakeWindowFrame{}
	w.frameStub = f
	return w, f
}

func TestFramedWindowTurnsTheSystemsControlsOff(t *testing.T) {
	o := framedWindow(mygo.WindowOptions{TitleBarStyle: mygo.TitleBarHidden})
	if o.TitleBarStyle != mygo.TitleBarHidden {
		t.Errorf("TitleBarStyle = %q, want the hidden one", o.TitleBarStyle)
	}
	if !o.HideWindowButtons {
		t.Error("the window kept the system's controls, which would sit beside the drawn ones")
	}
	if !o.AutoHideMenuBar {
		t.Error("the window kept a menu bar under a title bar it draws itself")
	}
}

// TestTheWindowControlsActOnTheWindow presses all three where they are drawn.
func TestTheWindowControlsActOnTheWindow(t *testing.T) {
	w, f := controlsTestWindow(t)
	tt := ui.NewTester(w.view, frameTestWidth, uiHeightForFrameTest)
	tt.Frame()

	minX, y := trafficLightCenter(1)
	tt.ClickAt(minX, y)
	tt.Frame()
	if f.minimized != 1 {
		t.Fatalf("the minimize control called Minimize %d times, want once", f.minimized)
	}

	maxX, _ := trafficLightCenter(2)
	tt.ClickAt(maxX, y)
	tt.Frame()
	if f.maximized != 1 || !f.stillMaxed {
		t.Fatalf("the zoom control did not maximize: %+v", f)
	}
	tt.ClickAt(maxX, y)
	tt.Frame()
	if f.restored != 1 || f.stillMaxed {
		t.Fatalf("the zoom control did not restore a maximized window: %+v", f)
	}

	closeX, _ := trafficLightCenter(0)
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
	w, _ := newTestShellWindow(t)
	w.fleet = newTestFleet(t)
	f := &fakeWindowFrame{}
	w.frameStub = f

	tt := ui.NewTester(w.view, frameTestWidth, uiHeightForFrameTest)
	tt.Frame()
	x, y := trafficLightCenter(0)
	tt.ClickAt(x, y)
	tt.Frame()
	if f.closed != 1 {
		t.Fatalf("an empty window has no close control: %+v", f)
	}
}

// TestTheControlsSitInTheTopLeft reads the corner off the layout: the three dots
// start at the window's edge, on the top bar's center line, and their labels are
// the window's own.
func TestTheControlsSitInTheTopLeft(t *testing.T) {
	w, _ := controlsTestWindow(t)
	tt := ui.NewTester(w.view, frameTestWidth, uiHeightForFrameTest)
	tt.Frame()

	for _, label := range []string{locales.WindowClose(), locales.WindowMinimize()} {
		rect, ok := tt.Find(label)
		if !ok {
			t.Fatalf("%q is not on screen; texts = %v", label, tt.Texts())
		}
		if rect.X >= float32(frameTestWidth)/2 {
			t.Errorf("%q is at x=%v, not in the window's left corner", label, rect.X)
		}
		if center := rect.Y + rect.H/2; center < uitokens.TopBar/2-2 || center > uitokens.TopBar/2+2 {
			t.Errorf("%q is centered at y=%v, not on the top bar's line %v", label, center, uitokens.TopBar/2)
		}
	}
}

// TestTheTakeoverStripStaysUnderTheHeader is the layout the report was about:
// the sentence about who holds the shell, and the button that takes it back,
// must be under the card's header rather than inside it.
func TestTheTakeoverStripStaysUnderTheHeader(t *testing.T) {
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
		t.Errorf("the takeover button is at y=%v, inside the header (top %v)", take.Y, top)
	}
	if right := take.X + take.W; right > float32(frameTestWidth-uitokens.CardGap-10) {
		t.Errorf("the takeover button reaches x=%v, past the card's inset", right)
	}

	// The header's own last chip ends within the card, where the window's own
	// controls are no longer drawn over it.
	chip, ok := tt.Find("claude")
	if !ok {
		t.Fatalf("the program chip is not on screen: texts = %v", tt.Texts())
	}
	if right := chip.X + chip.W; right > float32(frameTestWidth-uitokens.CardGap-10) {
		t.Errorf("the header's chip ends at x=%v, past the card's inset", right)
	}
}

// TestTheTopRightPaneOfASplitKeepsItsWholeHeader is the same rule in a split:
// with the controls in the top-left corner, the top-right pane's header has the
// room for its chips and no corner to keep clear.
func TestTheTopRightPaneOfASplitKeepsItsWholeHeader(t *testing.T) {
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
	if edge := chip.X + chip.W; edge > float32(frameTestWidth)-uitokens.CardGap {
		t.Errorf("the taken pane's 只读 chip ends at x=%v, past the card's edge", edge)
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

// TestTheWindowButtonsSitAtTheTopRight is the layout Windows and Linux get: the
// controls belong in the top-right corner, as Windows draws them, not in
// macOS's top-left.
func TestTheWindowButtonsSitAtTheTopRight(t *testing.T) {
	previous := controlsAtRight
	controlsAtRight = true
	defer func() { controlsAtRight = previous }()

	w, f := controlsTestWindow(t)
	tt := ui.NewTester(w.view, frameTestWidth, uiHeightForFrameTest)
	tt.Frame()

	for _, label := range []string{locales.WindowClose(), locales.WindowMinimize()} {
		rect, ok := tt.Find(label)
		if !ok {
			t.Fatalf("%q is not on screen; texts = %v", label, tt.Texts())
		}
		if rect.X <= float32(frameTestWidth)/2 {
			t.Errorf("%q is at x=%v, not in the window's right corner", label, rect.X)
		}
		if center := rect.Y + rect.H/2; center < uitokens.TopBar/2-2 || center > uitokens.TopBar/2+2 {
			t.Errorf("%q is centered at y=%v, not on the top bar's line %v", label, center, uitokens.TopBar/2)
		}
	}

	min, _ := tt.Find(locales.WindowMinimize())
	tt.ClickAt(min.X+min.W/2, min.Y+min.H/2)
	tt.Frame()
	if f.minimized != 1 {
		t.Fatalf("the minimize button called Minimize %d times, want once", f.minimized)
	}

	max, _ := tt.Find(locales.WindowMaximize())
	tt.ClickAt(max.X+max.W/2, max.Y+max.H/2)
	tt.Frame()
	if f.maximized != 1 || !f.stillMaxed {
		t.Fatalf("the maximize button did not maximize: %+v", f)
	}

	close, _ := tt.Find(locales.WindowClose())
	tt.ClickAt(close.X+close.W/2, close.Y+close.H/2)
	tt.Frame()
	if f.closed != 1 {
		t.Fatalf("the close button called Close %d times, want once", f.closed)
	}
}
