#!/usr/bin/env python3
"""Measure how thick the ink in an icon actually is, from a rendered PNG.

    icon-stroke.py shot.png --box 120,640,180,700 [--scale 2] [--threshold 128]

Beacon draws one Lucide path and sets the stroke per size (1.9 at 24, 1.75 at
16, 1.6 at 12) so the three sizes look equally heavy. That is a claim about
pixels, so it is measured here and not taken from the source: for every row
inside the box, the lengths of the runs of ink are collected, thin runs (the
line art) are separated from fills, and the median thin run is reported in
1x points.

--scale is the device pixel ratio the shot was taken at (a 2x render of a
1.9pt stroke measures 3.8 raw pixels). Pass it so the answer is in points.

ONLY BOX A STRAIGHT, AXIS-ALIGNED STROKE (the side of a monitor, a flat base
line) and keep the box taller/wider than the stroke. A run measured along a
row through a curve or a diagonal is longer than the stroke is thick (a 45
degree line reads 1.41x), so a circle or a gear comes out fat; and a box
shorter than the stroke cuts it. Both mistakes were made once already.

Also check the render is not snapped: if the whole row has only a handful of
grey levels, the renderer rounded the stroke to whole pixels and 1.9 versus
2.0 (or 1.75 versus 1.6) cannot be told apart at that scale; render at 2x.

No PIL, no sips: pure Python PNG decoding, same as pixdiff.py. Run with
--self-test to check the ruler against known widths before trusting it.
"""
import struct
import sys
import zlib


# ---------------------------------------------------------------- PNG reading

def read_png(path):
    """Decode an 8-bit non-interlaced PNG to (w, h, channels, bytearray)."""
    with open(path, "rb") as f:
        data = f.read()
    if data[:8] != b"\x89PNG\r\n\x1a\n":
        raise ValueError(f"{path} is not a PNG")
    pos = 8
    w = h = depth = ctype = interlace = None
    idat = bytearray()
    while pos < len(data):
        (length,) = struct.unpack(">I", data[pos:pos + 4])
        tag = data[pos + 4:pos + 8]
        body = data[pos + 8:pos + 8 + length]
        if tag == b"IHDR":
            w, h, depth, ctype, _, _, interlace = struct.unpack(">IIBBBBB", body)
        elif tag == b"IDAT":
            idat += body
        elif tag == b"IEND":
            break
        pos += 12 + length
    if depth != 8 or interlace != 0 or ctype not in (0, 2, 4, 6):
        raise ValueError(f"{path}: unsupported PNG (depth={depth}, type={ctype}, interlace={interlace})")
    channels = {0: 1, 2: 3, 4: 2, 6: 4}[ctype]
    raw = zlib.decompress(bytes(idat))
    stride = w * channels
    out = bytearray(h * stride)
    prev = bytearray(stride)
    p = 0
    for y in range(h):
        f = raw[p]
        p += 1
        line = bytearray(raw[p:p + stride])
        p += stride
        if f == 1:
            for i in range(channels, stride):
                line[i] = (line[i] + line[i - channels]) & 0xFF
        elif f == 2:
            for i in range(stride):
                line[i] = (line[i] + prev[i]) & 0xFF
        elif f == 3:
            for i in range(stride):
                left = line[i - channels] if i >= channels else 0
                line[i] = (line[i] + ((left + prev[i]) >> 1)) & 0xFF
        elif f == 4:
            for i in range(stride):
                a = line[i - channels] if i >= channels else 0
                b = prev[i]
                c = prev[i - channels] if i >= channels else 0
                pa, pb, pc = abs(b - c), abs(a - c), abs(a + b - 2 * c)
                pr = a if (pa <= pb and pa <= pc) else (b if pb <= pc else c)
                line[i] = (line[i] + pr) & 0xFF
        out[y * stride:(y + 1) * stride] = line
        prev = line
    return w, h, channels, out


def luminance(w, h, channels, buf, x, y):
    i = (y * w + x) * channels
    if channels == 1:
        return buf[i]
    if channels == 2:
        return buf[i]
    r, g, b = buf[i], buf[i + 1], buf[i + 2]
    a = buf[i + 3] if channels == 4 else 255
    if a < 255:  # over white
        r = (r * a + 255 * (255 - a)) // 255
        g = (g * a + 255 * (255 - a)) // 255
        b = (b * a + 255 * (255 - a)) // 255
    return (r * 299 + g * 587 + b * 114) // 1000


# ------------------------------------------------------------------ measuring

def runs_in_row(w, h, channels, buf, y, x0, x1, threshold):
    """Lengths of consecutive ink runs along one row, plus the row's mean."""
    runs = []
    run = 0
    for x in range(x0, x1):
        if luminance(w, h, channels, buf, x, y) < threshold:
            run += 1
        elif run:
            runs.append(run)
            run = 0
    if run:
        runs.append(run)
    return runs


def coverage_runs(vals, bg, ink, min_cov=0.12):
    """Widths of ink segments along one line of luminance values, in pixels.

    A pixel counts as its share of full ink, so an antialiased edge pixel that
    is 40% covered adds 0.4 instead of 0 or 1. That is what lets 1.9 and 1.75
    come out different at all.
    """
    span = float(bg - ink)
    runs = []
    acc = 0.0
    n = 0
    for v in vals:
        c = (bg - v) / span
        c = 0.0 if c < 0 else (1.0 if c > 1 else c)
        if c >= min_cov:
            acc += c
            n += 1
        elif n:
            runs.append((acc, n))
            acc = 0.0
            n = 0
    if n:
        runs.append((acc, n))
    return runs


def measure(path, box, scale=1.0, threshold=128, max_thin=10, axis="both"):
    """Returns (row_widths, col_widths, total_runs) in raw pixels, coverage-weighted.

    A segment is "line art" when it is at most max_thin points wide; longer ones
    are fills or the long axis of a stroke and are skipped. `threshold` is kept
    for the old call signature and now only sets the darkest value that still
    counts as ink when the glyph is lighter than it.
    """
    w, h, channels, buf = read_png(path)
    x0, y0, x1, y1 = box
    x1 = min(x1, w)
    y1 = min(y1, h)
    grid = [[luminance(w, h, channels, buf, x, y) for x in range(x0, x1)] for y in range(y0, y1)]
    flat = [v for row in grid for v in row]
    bg = sorted(flat)[int(len(flat) * 0.9)]      # canvas = a bright percentile
    ink = sorted(flat)[max(0, int(len(flat) * 0.01))]  # darkest ~1% = full ink
    if bg - ink < 20:
        return [], [], 0
    limit = max_thin * scale
    rows, cols, total = [], [], 0
    for row in grid:
        for acc, n in coverage_runs(row, bg, ink):
            total += 1
            if n <= limit:
                rows.append(acc)
    if axis == "both":
        for x in range(len(grid[0])):
            column = [grid[y][x] for y in range(len(grid))]
            for acc, n in coverage_runs(column, bg, ink):
                if n <= limit:
                    cols.append(acc)
    return rows, cols, total


def median(xs):
    xs = sorted(xs)
    n = len(xs)
    if not n:
        return 0.0
    return xs[n // 2] if n % 2 else (xs[n // 2 - 1] + xs[n // 2]) / 2


def report(path, box, scale, threshold):
    rows, cols, total = measure(path, box, scale, threshold)
    if not rows and not cols:
        print(f"{path} {box}: no ink found — wrong box, or nothing darker than the canvas by 20 levels.")
        return 1
    print(f"{path} box={box} scale={scale:g}  (coverage-weighted, in points)")
    # In a stroked glyph a vertical stroke is seen by the row scan and a
    # horizontal one by the column scan, so both are the stroke's thickness.
    both = rows + cols
    print(f"  stroke samples: {len(both)}  median {median(both) / scale:.2f}pt  "
          f"[p25 {sorted(both)[len(both) // 4] / scale:.2f}, p75 {sorted(both)[3 * len(both) // 4] / scale:.2f}]")
    return 0


# ------------------------------------------------------------------ self-test

def write_png(path, w, h, bar_rows, bar_cols, bar_width_v):
    """White canvas with one horizontal bar (known thickness) and one vertical bar."""
    rows = []
    for y in range(h):
        line = bytearray([255] * (w * 3))
        if bar_rows[0] <= y < bar_rows[1]:
            for x in range(4, 20):
                line[x * 3:x * 3 + 3] = b"\x00\x00\x00"
        if bar_cols[0] <= y < bar_cols[1]:
            for x in range(24, 24 + bar_width_v):
                line[x * 3:x * 3 + 3] = b"\x00\x00\x00"
        rows.append(bytes([0]) + bytes(line))
    raw = zlib.compress(b"".join(rows), 9)

    def chunk(tag, body):
        c = struct.pack(">I", len(body)) + tag + body
        return c + struct.pack(">I", zlib.crc32(tag + body) & 0xFFFFFFFF)

    png = (b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", w, h, 8, 2, 0, 0, 0))
           + chunk(b"IDAT", raw) + chunk(b"IEND", b""))
    with open(path, "wb") as f:
        f.write(png)


def self_test():
    """Known stroke widths in, the same numbers out — including a fractional one."""
    import tempfile, os
    ok = True

    def check(label, got, want, tol):
        nonlocal ok
        good = abs(got - want) <= tol
        ok = ok and good
        print(f"  {label}: measured {got:.2f}, expected {want} -> {'pass' if good else 'FAIL'}")

    # integer bars: the 16px-long horizontal bar is `thickness` thick (column scan),
    # the vertical bar is 3 wide (row scan)
    for thickness in (2, 4, 7):
        tmp = os.path.join(tempfile.mkdtemp(), f"t{thickness}.png")
        write_png(tmp, 40, 40, (10, 10 + thickness), (10, 30), 3)
        rows, cols, _ = measure(tmp, (0, 0, 40, 40), 1, 128, max_thin=12)
        check(f"{thickness}px bar, column scan", median(cols), thickness, 0.05)
        check(f"{thickness}px bar, row scan (3px stem)", median(rows), 3, 0.05)

    # the same 4px stroke drawn at 2x is 8 raw pixels and must read as 4pt
    tmp = os.path.join(tempfile.mkdtemp(), "t2x.png")
    write_png(tmp, 80, 80, (20, 28), (20, 60), 4)
    rows, cols, _ = measure(tmp, (0, 0, 80, 80), 2, 128, max_thin=12)
    check("8px bar at scale 2 -> points", median(cols) / 2, 4, 0.05)

    # fractional: a stroke whose edge row is 90% covered — 1.9px — must not round to 2
    tmp = os.path.join(tempfile.mkdtemp(), "frac.png")
    write_gray_bar(tmp, 40, 40, 14, 1.9)
    rows, cols, _ = measure(tmp, (0, 0, 40, 40), 1, 128, max_thin=12)
    check("1.9px stroke (antialiased edge)", median(cols), 1.9, 0.06)
    write_gray_bar(tmp, 40, 40, 14, 1.75)
    rows, cols, _ = measure(tmp, (0, 0, 40, 40), 1, 128, max_thin=12)
    check("1.75px stroke (antialiased edge)", median(cols), 1.75, 0.06)

    if not ok:
        print("self-test FAILED — the ruler is wrong, do not quote its numbers.")
        return 1
    print("self-test passed: the ruler measures what it says it measures.")
    return 0


def write_gray_bar(path, w, h, top, thickness):
    """A dark horizontal bar of fractional thickness: partial coverage on the edge rows."""
    rows = []
    for y in range(h):
        cover = max(0.0, min(1.0, min(y + 1, top + thickness) - max(y, top)))
        v = int(round(255 - 255 * cover))
        line = bytearray([v] * (w * 3))
        for x in range(0, 4):
            line[x * 3:x * 3 + 3] = b"\xff\xff\xff"
        rows.append(bytes([0]) + bytes(line))
    # a full-ink anchor so the canvas/ink levels are the real extremes
    anchor = bytearray([255] * (w * 3))
    for x in range(30, 33):
        anchor[x * 3:x * 3 + 3] = b"\x00\x00\x00"
    for y in range(2, 8):
        rows[y] = bytes([0]) + bytes(anchor)
    raw = zlib.compress(b"".join(rows), 9)

    def chunk(tag, body):
        c = struct.pack(">I", len(body)) + tag + body
        return c + struct.pack(">I", zlib.crc32(tag + body) & 0xFFFFFFFF)

    png = (b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", w, h, 8, 2, 0, 0, 0))
           + chunk(b"IDAT", raw) + chunk(b"IEND", b""))
    with open(path, "wb") as f:
        f.write(png)


def main(argv):
    if len(argv) > 1 and argv[1] == "--self-test":
        return self_test()
    if len(argv) < 2:
        print(__doc__)
        return 2
    path = argv[1]
    box = None
    scale = 1.0
    threshold = 128
    args = argv[2:]
    i = 0
    while i < len(args):
        if args[i] == "--box":
            box = tuple(int(v) for v in args[i + 1].split(","))
            i += 2
        elif args[i] == "--scale":
            scale = float(args[i + 1])
            i += 2
        elif args[i] == "--threshold":
            threshold = int(args[i + 1])
            i += 2
        else:
            print(f"unknown argument {args[i]}")
            return 2
    if box is None:
        print("--box x0,y0,x1,y1 is required")
        return 2
    return report(path, box, scale, threshold)


if __name__ == "__main__":
    sys.exit(main(sys.argv))
