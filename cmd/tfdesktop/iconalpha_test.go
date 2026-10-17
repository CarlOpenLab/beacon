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

			// The middle of the top edge is outside the art: the logo sits
			// centred with room around it. An opaque pixel there is a plate
			// composited behind the logo.
			//
			// The edge, not the corner: the plate that was there is a rounded
			// square, so its corners are cut away and only the edges carry the
			// colour. Sampling a corner passes on the broken file, which is how
			// the first version of this test managed to.
			mid := b.Min.X + b.Dx()/2
			_, _, _, edge := img.At(mid, b.Min.Y+4).RGBA()
			if edge > 0x0800 {
				t.Fatalf("top-edge alpha = %d/65535 at (%d,%d): there is an opaque plate behind the logo",
					edge, mid, b.Min.Y+4)
			}

			// And the art itself is still opaque at the centre, so the icon did
			// not become an empty file to satisfy the check above.
			_, _, _, centre := img.At(mid, b.Min.Y+b.Dy()/2).RGBA()
			if centre < 0xc000 {
				t.Fatalf("centre alpha = %d/65535, want the logo itself opaque", centre)
			}
		})
	}
}
