package main

// The app icon must be transparent where the art is empty.
//
// The logo was supplied as transparent art, but resources/icon.png — the file
// mygo.json names as the app icon, and the one the macOS Dock and the Windows
// taskbar both read — had an opaque dark plate composited behind it. The result
// was a dark square in both places, which is what the report was about.
//
// mygo renders the icon for both platforms from this file, and its resampler
// works in premultiplied alpha, so whatever alpha is in the source is what
// ships. That makes the source the only place to check.
//
// This reads the pixels rather than trusting the file's colour type: a PNG can
// declare RGBA and still have an opaque plate in every background pixel.

import (
	"bytes"
	"image/png"
	"os"
	"testing"
)

// iconFiles are the assets that must have a transparent background.
var iconFiles = []string{
	"../../resources/icon.png",
	"../../.github/assets/logo.png",
	"../../web/public/favicon.png",
	"assets/mark.png",
}

func TestTheAppIconHasNoOpaquePlate(t *testing.T) {
	for _, path := range iconFiles {
		t.Run(path, func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			img, err := png.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			b := img.Bounds()
			if b.Dx() != b.Dy() {
				t.Fatalf("%dx%d, want a square icon", b.Dx(), b.Dy())
			}

			// The design changed after the first version of this test: the icon is
			// now a lamp on a dark rounded tile, so the tile *is* opaque along the
			// edge, and that is on purpose — it is what makes the lamp legible in
			// the Dock and on this site's paper-coloured header.
			//
			// What must stay true is the thing the report was about: the tile has to
			// be rounded, not a full-bleed square. A corner is cut by the rounding,
			// so a corner that paints anything at all is a square plate composited
			// behind the art — the bug this test was written for. Sampling the edge
			// instead would now pass on that bug, which is why the sample moved.
			for _, corner := range [][2]int{
				{b.Min.X + 2, b.Min.Y + 2},
				{b.Max.X - 3, b.Min.Y + 2},
				{b.Min.X + 2, b.Max.Y - 3},
				{b.Max.X - 3, b.Max.Y - 3},
			} {
				_, _, _, alpha := img.At(corner[0], corner[1]).RGBA()
				if alpha > 0x0800 {
					t.Fatalf("corner alpha = %d/65535 at (%d,%d): there is a square plate behind the logo",
						alpha, corner[0], corner[1])
				}
			}

			// And something is drawn: a file emptied to satisfy the check above is
			// not an icon.
			opaque := 0
			for y := b.Min.Y; y < b.Max.Y; y += 8 {
				for x := b.Min.X; x < b.Max.X; x += 8 {
					if _, _, _, a := img.At(x, y).RGBA(); a > 0x8000 {
						opaque++
					}
				}
			}
			if opaque < (b.Dx()/8)*(b.Dy()/8)/20 {
				t.Fatalf("only %d sampled pixels are opaque: the icon is empty", opaque)
			}
		})
	}
}
