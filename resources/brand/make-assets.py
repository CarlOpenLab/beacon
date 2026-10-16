#!/usr/bin/env python3
"""Rebuild term-fleet's brand art from the logo sheet.

The mark is the middle tile of the sheet's third row: Carl picked it, and the
sheet only exists as a raster render (dark background, soft glow), so nothing
here is a vector trace — the mark is rebuilt from the sheet's own pixels and
re-coloured, which keeps the glow and the two flat colours exactly as drawn.

Reads:  the sheet (a 3x3 grid of variants) at SHEET, or the extracted master at
        resources/brand/mark.png once it exists.
Writes: resources/icon.png, .github/assets/logo.png, web/public/favicon.png,
        resources/brand/mark.png (the master).

Needs Pillow. Run from the repository root:

    python3 resources/brand/make-assets.py [sheet.png]

The sheet is not kept in the repository (it is a 1.4 MB render); pass its path
the first time, or drop the extracted master in place and run it with no
argument.
"""

import os
import sys

import numpy as np
from PIL import Image, ImageDraw

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
MASTER = os.path.join(ROOT, "resources", "brand", "mark.png")

# The tile Carl picked, counted the way a person counts: third row, second
# column. The grid is found from the sheet itself, so this is all that has to be
# known about the source.
CELL_ROW, CELL_COL = 3, 2

SHEET_BG = np.array([32, 32, 39])  # the sheet's own background
BLUE = np.array([150, 176, 250])  # the sheet's accent
WARM = np.array([249, 242, 237])  # the sheet's glyph

# The shell's palette, which the app icon and the rail already use.
INK = (22, 26, 34)
ACCENT_ON_WHITE = (91, 118, 232)  # the accent darkened to read on white
TILE_RADIUS = 0.204  # the old icon's corner radius, as a share of its side
MARK_SHARE = 0.62  # how much of the tile's width the mark takes


def bands(profile, threshold):
    """The runs of rows/columns that carry ink, which is how the grid is found."""
    out, start = [], None
    for i, value in enumerate(profile):
        if value > threshold and start is None:
            start = i
        elif value <= threshold and start is not None:
            out.append((start, i - 1))
            start = None
    if start is not None:
        out.append((start, len(profile) - 1))
    return [b for b in out if b[1] - b[0] > 20]


def cell_box(sheet):
    """The pixel box of the chosen tile, found by projecting the sheet's ink."""
    ink = np.abs(sheet - SHEET_BG).sum(axis=2) > 40
    rows = bands(ink.sum(axis=1), max(3, sheet.shape[1] // 200))
    cols = bands(ink.sum(axis=0), max(3, sheet.shape[0] // 200))
    if len(rows) != 3 or len(cols) != 3:
        raise SystemExit(f"expected a 3x3 sheet, found {len(rows)}x{len(cols)} tiles")
    row, col = rows[CELL_ROW - 1], cols[CELL_COL - 1]
    return col[0], row[0], col[1] + 1, row[1] + 1


def mark_from_sheet(sheet):
    """The mark as (coverage, whiteness): how much of the mark covers a pixel,
    and how far that pixel sits between the accent and the glyph."""
    x0, y0, x1, y1 = cell_box(sheet)
    tile = sheet[y0:y1, x0:x1].astype(np.float64)

    gap = np.abs(tile - SHEET_BG).sum(axis=2)
    # The glyph is the furthest colour from the background, so it sets the scale
    # at which a pixel counts as fully covered.
    solid = np.abs(WARM - SHEET_BG).sum()
    coverage = np.clip((gap - 8) / (solid - 8), 0, 1)

    # Un-premultiply against the sheet's background, so a half-lit glow pixel
    # keeps the accent's own colour instead of turning muddy.
    safe = np.maximum(coverage, 1e-6)[..., None]
    colour = np.clip((tile - SHEET_BG * (1 - safe)) / safe, 0, 255)

    axis = WARM - BLUE
    whiteness = np.clip(((colour - BLUE) @ axis) / (axis @ axis), 0, 1)

    # Crop to the mark's own bounds, with a margin that keeps the glow's edge
    # from touching the tile it is drawn on, and keep the square aspect: the
    # mark is wider than it is tall, and every surface below wants a square.
    ys, xs = np.nonzero(coverage > 0.4)
    cx, cy = (xs.min() + xs.max()) / 2, (ys.min() + ys.max()) / 2
    side = max(xs.max() - xs.min(), ys.max() - ys.min()) * 1.06
    half = side / 2
    ox, oy = int(round(cx - half)), int(round(cy - half))
    size = int(round(side))

    def crop(channel):
        # The margin can reach past the tile's edge, so place the part that
        # exists instead of slicing with a negative index, which would silently
        # read from the far side of the tile.
        pad = np.zeros((size, size), dtype=np.float64)
        x_from, y_from = max(ox, 0), max(oy, 0)
        x_to, y_to = min(ox + size, channel.shape[1]), min(oy + size, channel.shape[0])
        if x_to > x_from and y_to > y_from:
            pad[y_from - oy : y_to - oy, x_from - ox : x_to - ox] = channel[y_from:y_to, x_from:x_to]
        return pad

    return crop(coverage), crop(whiteness)


def resize(channel, size):
    """Upscale a coverage/whiteness plane on its own, so the flat colours below
    stay flat and only the edges soften."""
    img = Image.fromarray((np.clip(channel, 0, 1) * 255).astype(np.uint8), "L")
    return np.asarray(img.resize((size, size), Image.LANCZOS)).astype(np.float64) / 255


def draw_mark(canvas, coverage, whiteness, glyph, accent, share):
    """Composite the mark onto a square canvas, centred and scaled to share."""
    side = int(round(canvas.width * share))
    cov = resize(coverage, side)
    white = resize(whiteness, side)
    rgb = np.zeros((side, side, 4))
    rgb[..., :3] = np.array(accent) * (1 - white[..., None]) + np.array(glyph) * white[..., None]
    rgb[..., 3] = cov * 255
    mark = Image.fromarray(rgb.astype(np.uint8), "RGBA")
    canvas.alpha_composite(mark, ((canvas.width - side) // 2, (canvas.height - side) // 2))
    return canvas


def rounded_tile(size, colour, radius):
    tile = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    ImageDraw.Draw(tile).rounded_rectangle(
        (0, 0, size - 1, size - 1), radius=radius * size, fill=colour + (255,)
    )
    return tile


def square(size, colour):
    return Image.new("RGBA", (size, size), colour + (255,))


def main():
    if len(sys.argv) > 1:
        sheet = np.asarray(Image.open(sys.argv[1]).convert("RGB")).astype(np.float64)
        coverage, whiteness = mark_from_sheet(sheet)
        master = Image.fromarray(
            np.dstack([
                np.array(BLUE) * (1 - whiteness[..., None]) + np.array(WARM) * whiteness[..., None],
                coverage * 255,
            ]).astype(np.uint8),
            "RGBA",
        )
        master.resize((512, 512), Image.LANCZOS).save(MASTER)
    else:
        master = Image.open(MASTER).convert("RGBA")
        arr = np.asarray(master).astype(np.float64)
        coverage = arr[..., 3] / 255
        colour = arr[..., :3]
        axis = WARM - BLUE
        whiteness = np.clip(((colour - BLUE) @ axis) / (axis @ axis), 0, 1)

    # The app icon: the shell's own dark tile, with the mark as the sheet drew
    # it — accent and glyph both.
    icon = draw_mark(
        rounded_tile(1024, INK, TILE_RADIUS), coverage, whiteness, WARM, BLUE, MARK_SHARE
    )
    icon.save(os.path.join(ROOT, "resources", "icon.png"))

    # The README's logo and the web favicon: the white tile the brand has used
    # since v0.2.0, with the glyph in ink and the accent darkened to read on it.
    draw_mark(square(512, (255, 255, 255)), coverage, whiteness, INK, ACCENT_ON_WHITE, MARK_SHARE).save(
        os.path.join(ROOT, ".github", "assets", "logo.png")
    )
    favicon = draw_mark(
        rounded_tile(128, (255, 255, 255), TILE_RADIUS), coverage, whiteness, INK, ACCENT_ON_WHITE, 0.66
    )
    favicon.save(os.path.join(ROOT, "web", "public", "favicon.png"))

    print("wrote resources/icon.png, .github/assets/logo.png, web/public/favicon.png")


if __name__ == "__main__":
    main()