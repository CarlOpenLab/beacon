package main

import (
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CarlOpenLab/beacon/internal/protocol"
	"github.com/CarlOpenLab/beacon/internal/settings"
	"github.com/CarlOpenLab/beacon/internal/uitokens"
	"github.com/egoist/mygo/ui"
)

// The session row's second line stands on the window's acrylic material, not
// inside a card, so its two facts — the directory and the run time — take the
// palette's primary text ink. On a bright material the muted grey they used to
// take falls under 2:1 and the line cannot be read; the card header's path
// learned this in a07947e and the row never followed.
func TestTheRowsSecondLineTakesThePrimaryInk(t *testing.T) {
	w, _ := newTestShellWindow(t, "fix the row")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home directory: %v", err)
	}
	cwd := filepath.Join(home, "dev", "own", "beacon")
	w.tabs[1].alive = true
	if _, err := w.store.Update(func(s *settings.Settings) { s.Theme = themeDark }); err != nil {
		t.Fatal(err)
	}
	w.live = []protocol.Summary{{
		SessionID: "s1", Title: "fix the row", Cwd: cwd, Alive: true,
		CreatedAt: time.Now().Add(-90 * time.Minute).Format(time.RFC3339Nano),
	}}

	nav := drawNav(t, w)
	pal := uitokens.Dark
	want := [3]uint32{uint32(pal.Text.R), uint32(pal.Text.G), uint32(pal.Text.B)}

	for _, fact := range []string{"~/dev/own/beacon", "1h"} {
		box, ok := nav.Find(fact)
		if !ok {
			t.Fatalf("the row does not show %q; texts = %v", fact, nav.Texts())
		}
		got, ok := inkOf(nav.Image(), box)
		if !ok {
			t.Fatalf("the row paints no glyph for %q", fact)
		}
		if got != want {
			t.Errorf("%q is drawn in #%02x%02x%02x, want the primary ink #%02x%02x%02x",
				fact, got[0], got[1], got[2], want[0], want[1], want[2])
		}
	}
}

// inkOf returns the colour the glyph inside box was drawn in: the pixel there
// furthest from the box's own background.
func inkOf(img *image.RGBA, box ui.Rect) ([3]uint32, bool) {
	var best float64
	var out [3]uint32
	var found bool
	bg := median(img, box)
	for y := int(box.Y); y < int(box.Y+box.H); y++ {
		for x := int(box.X); x < int(box.X+box.W); x++ {
			if !image.Pt(x, y).In(img.Bounds()) {
				continue
			}
			r, g, b, _ := img.At(x, y).RGBA()
			d := dist(r>>8, g>>8, b>>8, bg)
			if d > best {
				best, found, out = d, true, [3]uint32{r >> 8, g >> 8, b >> 8}
			}
		}
	}
	return out, found
}

// median is the background the glyph sits on: the colour most of the box is.
func median(img *image.RGBA, box ui.Rect) [3]uint32 {
	counts := map[[3]uint32]int{}
	for y := int(box.Y); y < int(box.Y+box.H); y++ {
		for x := int(box.X); x < int(box.X+box.W); x++ {
			if !image.Pt(x, y).In(img.Bounds()) {
				continue
			}
			r, g, b, _ := img.At(x, y).RGBA()
			counts[[3]uint32{r >> 8, g >> 8, b >> 8}]++
		}
	}
	var best int
	var out [3]uint32
	for c, n := range counts {
		if n > best {
			best, out = n, c
		}
	}
	return out
}

func dist(r, g, b uint32, to [3]uint32) float64 {
	dr, dg, db := float64(r)-float64(to[0]), float64(g)-float64(to[1]), float64(b)-float64(to[2])
	return dr*dr + dg*dg + db*db
}
