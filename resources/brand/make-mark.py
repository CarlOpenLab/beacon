#!/usr/bin/env python3
"""Beacon's mark, drawn from geometry — the brand has no raster master any more.

The mark is a lamp and two beams leaving it: the product's promise in one shape
(a session calls for you, and the call is light). Everything is drawn at four
times the target size and scaled down, so the 16px favicon is the same drawing
as the app icon rather than a blurred copy of it.

Sizes and where they land
    1024  resources/icon.png            app icon (macOS/Windows packaging),
                                       inset by APP_ICON_PAD on each side
     512  .github/assets/logo.png       README header
     128  web/public/favicon.png        browser tab
      32  web/public/favicon-32.png
      16  web/public/favicon-16.png
     512  cmd/tfdesktop/assets/mark.png in-app mark, cream lamp: dark ground only
     512  cmd/tfdesktop/assets/mark-ink.png in-app mark, ink lamp: light ground
    1024  pages/assets/icon.png         landing site icon
     512  pages/assets/logo.png         landing og:image
 142x120  pages/assets/logo-mark.png   the mark beside the wordmark, ink lamp:
                                       the page header and footer are paper, and a
                                       cream lamp on paper is a hole, not a lamp

Small sizes get their own optics: the lamp grows and the beams shorten, because
a straight scale turns two thin wedges into grey dust under 32px.

Needs Pillow. Run from the repository root:

    python3 resources/brand/make-mark.py
"""

import os

from PIL import Image, ImageDraw

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

INK = (22, 26, 34)        # the desktop shell's ink, which the window and rail use
CREAM = (242, 238, 228)   # landing --paper
SIGNAL = (196, 68, 28)    # landing --signal
TILE_RADIUS = 0.204       # the corner the app icon has worn since v0.2.0
# An app icon is not a wallpaper: the OS draws it in a slot that is larger than
# the artwork is meant to be, and every other icon in that slot carries a
# transparent margin. A full-bleed tile fills the slot edge to edge and reads a
# tenth larger than its neighbours on the desktop and in the Dock, which is how
# it was reported. 10% a side puts the tile at 80% of the canvas — the macOS
# grid's proportion, and what the icons beside it on Windows already do.
APP_ICON_PAD = 0.10


def mark(size, tile=True, optical=1.0, lamp=CREAM, pad=0.0):
    """Draw the mark at `size` px. `optical` > 1 fattens the drawing for tiny sizes.

    `lamp` is the disc's colour: CREAM for the tile and for dark ground, INK for
the bare mark on light ground, where a cream lamp on paper is a hole, not a lamp.
    `pad` is the transparent margin on each side, as a fraction of `size`: the
tile (and with it the lamp and beams) is drawn inside the remaining square.
    """
    ss = 4
    n = size * ss
    img = Image.new("RGBA", (n, n), (0, 0, 0, 0))
    d = ImageDraw.Draw(img)

    margin = round(n * pad)
    box = n - 2 * margin                      # the tile's side, inside the margin
    if tile:
        d.rounded_rectangle([margin, margin, margin + box - 1, margin + box - 1],
                            radius=TILE_RADIUS * box, fill=INK + (255,))

    u = box / 1024.0                          # one design unit
    cx, cy = margin + 300 * u, margin + 512 * u
    r = (168 + 46 * (optical - 1)) * u        # the lamp grows with optical correction
    reach = margin + box                      # the beams run off the tile's edge

    # Two beams leaving the lamp: narrow where they leave it, wide where they
    # run off the tile. Fattened at small sizes, or they turn into grey dust.
    near_in = (74 + 34 * (optical - 1)) * u
    near_out = (26 + 22 * (optical - 1)) * u
    far_in = (120 + 46 * (optical - 1)) * u
    far_out = (300 - 74 * (optical - 1)) * u

    for sign in (-1, 1):
        d.polygon(
            [
                (cx + r * 0.80, cy + sign * near_out),
                (reach, cy + sign * far_in),
                (reach, cy + sign * far_out),
                (cx + r * 0.80, cy + sign * near_in),
            ],
            fill=SIGNAL + (255,),
        )

    d.ellipse([cx - r, cy - r, cx + r, cy + r], fill=lamp + (255,))
    return img.resize((size, size), Image.LANCZOS)


def main():
    def out(rel):
        path = os.path.join(ROOT, rel)
        os.makedirs(os.path.dirname(path), exist_ok=True)
        return path

    mark(1024, pad=APP_ICON_PAD).save(out("resources/icon.png"))
    mark(512).save(out(".github/assets/logo.png"))
    mark(128, optical=1.12).save(out("web/public/favicon.png"))
    mark(32, optical=1.30).save(out("web/public/favicon-32.png"))
    mark(16, optical=1.45).save(out("web/public/favicon-16.png"))
    mark(512, tile=False).save(out("cmd/tfdesktop/assets/mark.png"))
    mark(512, tile=False, lamp=INK).save(out("cmd/tfdesktop/assets/mark-ink.png"))

    # The landing site ships its own copies; keep them the same drawing.
    mark(1024).save(out("pages/assets/icon.png"))
    mark(512).save(out("pages/assets/logo.png"))
    # The page header draws this at 30px tall, so it carries the small-size
    # optics: at that height the unbeefed beams fade to a red smudge.
    wide = Image.new("RGBA", (142, 120), (0, 0, 0, 0))
    art = mark(120, tile=False, lamp=INK, optical=1.12)
    wide.alpha_composite(art, ((142 - 120) // 2, 0))
    wide.save(out("pages/assets/logo-mark.png"))
    mark(32, optical=1.30).save(out("pages/assets/favicon-32.png"))
    mark(16, optical=1.45).save(out("pages/assets/favicon-16.png"))
    print("wrote icon, logo, favicon (128/32/16), window mark, landing assets")


if __name__ == "__main__":
    main()
