package main

// The window's frame: macOS keeps its traffic lights and the view lays out
// around the room they report; Windows and Linux take the system's buttons away
// and the view draws its own at the top-right. The takeover sentence lives in
// the card's header, so it never takes a row away from the terminal.

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

// frameTestWidth and uiHeightForFrameTest are the window the frame tests draw at.
const (
	frameTestWidth       = 900
	uiHeightForFrameTest = 600
)

// controlsTestWindow is a window with a shell on screen and a frame to count
// what the controls do.
func controlsTestWindow(t *testing.T) (*window, *fakeWindowFrame) {
	t.Helper()
	w := previewWindow(t, themeLight, true)
	f := &fakeWindowFrame{}
	w.frameStub = f
	return w, f
}

// TestFramedWindowKeepsOneSetOfControls reads the frame options: the view's own
// buttons on Windows and Linux, with the system's taken away, and the system's
// traffic lights on macOS, moved onto the line the chrome centres on.
func TestFramedWindowKeepsOneSetOfControls(t *testing.T) {
	o := framedWindow(mygo.WindowOptions{})
	if !o.AutoHideMenuBar {
		t.Error("the window kept a menu bar under a hidden title bar")
	}
	if controlsAtRight {
		if o.TitleBarStyle != mygo.TitleBarHidden {
			t.Errorf("TitleBarStyle = %q, want %q", o.TitleBarStyle, mygo.TitleBarHidden)
		}
		if !o.HideWindowButtons {
			t.Error("the window kept the system's buttons, which would sit beside the drawn ones")
		}
		if o.TrafficLightPosition != nil {
			t.Errorf("TrafficLightPosition = %+v, want none where the view draws the controls", o.TrafficLightPosition)
		}
		return
	}
	if o.TitleBarStyle != mygo.TitleBarHiddenInset {
		t.Errorf("TitleBarStyle = %q, want %q", o.TitleBarStyle, mygo.TitleBarHiddenInset)
	}
	if o.HideWindowButtons {
		t.Error("the window hid its traffic lights, which are macOS's own")
	}
	if o.TrafficLightPosition == nil || o.TrafficLightPosition.X != trafficLightX || o.TrafficLightPosition.Y != trafficLightY {
		t.Errorf("TrafficLightPosition = %+v, want %d,%d", o.TrafficLightPosition, trafficLightX, trafficLightY)
	}
}

// TestTheChromeClearsTheSystemsLights is the macOS layout: the sidebar's own
// buttons start after the room the traffic lights report, and the card header
// takes the same room before its title when the sidebar is folded away.
func TestTheChromeClearsTheSystemsLights(t *testing.T) {
	for _, bar := range []ui.TitleBar{{Left: 68, Height: 60}, {}} {
		if got := titleBarLead(bar); got < bar.Left+8 || got < 12 {
			t.Errorf("titleBarLead(%+v) = %v, want the lights' room plus a gap", bar, got)
		}
		if bar.Left == 0 {
			if got := titleBarLead(bar); got != 12 {
				t.Errorf("titleBarLead(%+v) = %v, want a margin with no controls", bar, got)
			}
		}
	}
	if got, want := cardTitleLead(ui.TitleBar{Left: 68}), float32(68+8)+toolbarButton+8; got != want {
		t.Errorf("cardTitleLead = %v, want %v", got, want)
	}

	w, _ := newTestShellWindow(t)
	w.fleet = newTestFleet(t)
	tt := ui.NewTester(w.view, frameTestWidth, uiHeightForFrameTest)
	tt.SetTitleBar(ui.TitleBar{Left: 68, Height: 60})
	tt.Frame()

	show, ok := tt.Find(locales.ActionToggleSidebar())
	if !ok {
		t.Fatalf("the sidebar's toggle is not on screen; texts = %v", tt.Texts())
	}
	if show.X < 68+8 {
		t.Errorf("the sidebar's toggle is at x=%v, inside the traffic lights' room", show.X)
	}
}

// TestTheTakeoverBannerStaysInTheHeader is the layout the report was about:
// taking or releasing the lease used to add a strip under the header, and the
// strip took its height out of the terminal — the same shell re-fitted and lost
// its last rows just because control had moved. The sentence and the button are
// in the header now, and the terminal keeps the same rectangle in both states.
func TestTheTakeoverBannerStaysInTheHeader(t *testing.T) {
	w, _ := controlsTestWindow(t)

	controlling := ui.NewTester(w.view, frameTestWidth, uiHeightForFrameTest)
	controlling.Frame()
	before, ok := controlling.Find("Terminal")
	if !ok {
		t.Fatalf("the terminal is not on screen: texts = %v", controlling.Texts())
	}

	w.tabs[1].setLease(protocol.RoleViewer, "c1", 4)
	taken := ui.NewTester(w.view, frameTestWidth, uiHeightForFrameTest)
	taken.Frame()

	take, ok := taken.Find(locales.ActionTakeBack())
	if !ok {
		t.Fatalf("the takeover button is not on screen: texts = %v", taken.Texts())
	}
	// Inside the header band, not on a strip under it.
	if bottom := float32(uitokens.CardGap + uitokens.HeaderH); take.Y+take.H > bottom {
		t.Errorf("the takeover button reaches y=%v, past the header (bottom %v)", take.Y+take.H, bottom)
	}
	if right := take.X + take.W; right > float32(frameTestWidth-uitokens.CardGap-10) {
		t.Errorf("the takeover button reaches x=%v, past the card's inset", right)
	}

	// The terminal is the same rectangle whether this window holds the shell or
	// watches it: no row appears above it and none is taken away.
	after, ok := taken.Find("Terminal")
	if !ok {
		t.Fatalf("the terminal is not on screen while watching: texts = %v", taken.Texts())
	}
	if before.Y != after.Y || before.H != after.H {
		t.Errorf("the terminal moved from y=%v h=%v to y=%v h=%v when the lease left",
			before.Y, before.H, after.Y, after.H)
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
