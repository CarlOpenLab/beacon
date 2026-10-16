#!/usr/bin/env python3
"""Rebuild term-fleet's brand art from the mascot master.

The master is the mascot Carl picked — a periwinkle planet with a cream face, a
ring and a terminal card — delivered as a square render on a transparent ground.
Nothing here traces or recolours it: the art is used exactly as drawn, and this
script only decides how big it is on each surface.

That the art is used as drawn is why every surface gets a dark tile. The mascot's
face is cream (248, 241, 234) and its silhouette is periwinkle: on a white tile
the face would run into the page, and on the transparent ground of a README it
would disappear on GitHub's light theme. On the shell's ink the whole mascot
reads, its dark terminal card included, because that card sits on the periwinkle
body rather than on the tile.

Reads:  resources/brand/mark.png (the master, transparent, square).
Writes: resources/icon.png, .github/assets/logo.png, web/public/favicon.png,
        cmd/tfdesktop/assets/mark.png.
        With a source image given, resources/brand/mark.png as well: the source
        cropped to what is drawn, squared, and kept at 1024.

Needs Pillow. Run from the repository root:

    python3 resources/brand/make-assets.py [new-mascot.png]
"""

import os
import sys

from PIL import Image, ImageDraw

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
MASTER = os.path.join(ROOT, "resources", "brand", "mark.png")
MASTER_SIZE = 1024

# The shell's own ink, which the window, the rail and the app icon all use.
INK = (22, 26, 34)
TILE_RADIUS = 0.204  # the corner the app icon has had since v0.2.0
# How much of a tile's width the mascot takes. The art is wider than it is tall
# (about 1.5:1), so a share near 0.9 leaves the margin a tile wants above and
# below it — the art's own ring and sparkle hold the edge — without shrinking the
# face into the middle of the tile.
ICON_SHARE = 0.9
LOGO_SHARE = 0.92
FAVICON_SHARE = 0.95
# The window's own copy of the mark is smaller than the master: the strip draws
# it 30 DIPs wide and the empty state 56, so 512 is already more than any screen
# asks for, and it is embedded in the binary.
WINDOW_MARK_SIZE = 512
WINDOW_MARK_SHARE = 0.96


def cropped_master():
    """The master, cropped to what is actually drawn and squared up."""
    return squared(Image.open(MASTER))


def squared(art):
    """Crop an image to what is drawn, centre it in a square of its long side."""
    art = art.convert("RGBA")
    box = art.getbbox()
    if box is None:
        raise SystemExit(f"{MASTER} is empty")
    art = art.crop(box)
    side = max(art.width, art.height)
    canvas = Image.new("RGBA", (side, side), (0, 0, 0, 0))
    canvas.alpha_composite(art, ((side - art.width) // 2, (side - art.height) // 2))
    return canvas


def tile(size, colour, radius=TILE_RADIUS):
    plate = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    ImageDraw.Draw(plate).rounded_rectangle(
        (0, 0, size - 1, size - 1), radius=radius * size, fill=colour + (255,)
    )
    return plate


def draw(plate, art, share):
    """Composite the mascot, centred, taking share of the plate's width."""
    width = int(round(plate.width * share))
    height = int(round(width * art.height / art.width))
    mascot = art.resize((width, height), Image.LANCZOS)
    plate.alpha_composite(mascot, ((plate.width - width) // 2, (plate.height - height) // 2))
    return plate


def main():
    if len(sys.argv) > 1:
        # A new mascot: keep it as the master, so the surfaces below can be
        # rebuilt from it later without the original render. It is square by
        # then, so scaling is all that is left.
        master = squared(Image.open(sys.argv[1]))
        if master.width != MASTER_SIZE:
            master = master.resize((MASTER_SIZE, MASTER_SIZE), Image.LANCZOS)
        master.save(MASTER)

    art = cropped_master()

    # The app icon and the browser favicon: the ink tile the app already wears,
    # with the mascot on it as drawn.
    draw(tile(1024, INK), art, ICON_SHARE).save(os.path.join(ROOT, "resources", "icon.png"))
    draw(tile(128, INK), art, FAVICON_SHARE).save(
        os.path.join(ROOT, "web", "public", "favicon.png")
    )

    # The README's logo is the same tile, so what the repository shows and what
    # the app shows are the same picture.
    draw(tile(512, INK), art, LOGO_SHARE).save(
        os.path.join(ROOT, ".github", "assets", "logo.png")
    )

    # The window draws the mascot on its own ink rail, so its copy is the art
    # without a tile, and small: the strip is 30 DIPs tall.
    window = Image.new("RGBA", (WINDOW_MARK_SIZE, WINDOW_MARK_SIZE), (0, 0, 0, 0))
    draw(window, art, WINDOW_MARK_SHARE).save(
        os.path.join(ROOT, "cmd", "tfdesktop", "assets", "mark.png")
    )

    print(
        "wrote resources/icon.png, .github/assets/logo.png, "
        "web/public/favicon.png, cmd/tfdesktop/assets/mark.png"
    )


if __name__ == "__main__":
    main()
