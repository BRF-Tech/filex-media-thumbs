#!/usr/bin/env python3
"""
Write the files the tests and a manual check in filex use (testdata/).

  testdata/fonts/Go-Regular.ttf          the Go font (golang.org/x/image/font/gofont/ttfs),
                                         copied as it is; its license is Go-Regular.LICENSE.txt
  testdata/fonts/Go-Regular.woff         the same font as WOFF 1.0 (fontTools, zlib tables)
  testdata/fonts/Go-Regular.woff2        the same font as WOFF2 with the glyf/loca transform
                                         (fontTools' default, as every encoder does)
  testdata/fonts/Go-Regular.hmtx.woff2   the same, with the hmtx transform too
  testdata/samples/poster.af             an Affinity container: the 00 FF 4B 41 header,
                                         "nsrP#Inf" with the offsets of the stream table and
                                         the preview block, compressed-looking streams, "#FT4",
                                         the "Thmb" block with a 96 x 64 PNG, metadata
  testdata/samples/icon.pxd              a Pixelmator Pro file: a ZIP of stored members
                                         (metadata.info, QuickLook/Thumbnail.webp 96 x 96 with
                                         alpha, QuickLook/Icon.webp 32 x 32)
  testdata/samples/sample.psd            a Photoshop file: 600 x 400 RGB, 8 bits, the merged
                                         image PackBits-compressed, a 1036 JPEG thumbnail and
                                         1057 saying the merged image is real
  testdata/samples/cython.pxd            a Cython declaration file: text under .pxd
  internal/design/testdata/pixelmator-thumbnail.webp, pixelmator-icon.webp
                                         small WebP pictures with alpha, as Pixelmator Pro
                                         writes them (made by filex's #147 work, kept as they are)

The shapes of the design files are the ones measured on real files (see the
README); the pictures are made here, so the files carry nobody else's work.

Needs fontTools, brotli and Pillow:

  python3 -m pip install fonttools brotli pillow
  python3 scripts/gen-fixtures.py            (from the repository root)

The WOFF and WOFF2 files depend on the fontTools version that wrote them;
the tests compare what they decode to with the TTF, not their bytes, so a
newer fontTools may rewrite them without breaking anything.
"""

import io
import struct
import sys
import zipfile
from pathlib import Path

from PIL import Image
from fontTools.ttLib import TTFont
from fontTools.ttLib.woff2 import compress as woff2_compress

ROOT = Path(__file__).resolve().parent.parent
FONTS = ROOT / "testdata" / "fonts"
SAMPLES = ROOT / "testdata" / "samples"


def png(w, h, a, b):
    im = Image.new("RGB", (w, h))
    for y in range(h):
        for x in range(w):
            t = x / (w - 1)
            im.putpixel((x, y), tuple(int(a[i] * (1 - t) + b[i] * t) for i in range(3)))
    for y in range(h // 3, 2 * h // 3):
        for x in range(w // 3, 2 * w // 3):
            im.putpixel((x, y), (250, 250, 250))
    buf = io.BytesIO()
    im.save(buf, "PNG")
    return buf.getvalue()


def webp(w, h, color):
    im = Image.new("RGBA", (w, h), color + (255,))
    for y in range(h):
        for x in range(w):
            if (x - w / 2) ** 2 + (y - h / 2) ** 2 > (w / 2.2) ** 2:
                im.putpixel((x, y), (0, 0, 0, 0))
    buf = io.BytesIO()
    im.save(buf, "WEBP", quality=85)
    return buf.getvalue()


def affinity(preview):
    b = bytearray()
    b += bytes([0x00, 0xFF, 0x4B, 0x41]) + struct.pack("<I", 12) + b"nsrP#Inf" + bytes(24)
    b += b"Prot\x08\x00\x00\x00#Fil" + bytes([0x28, 0xB5, 0x2F, 0xFD]) + bytes((i * 37) & 0xFF for i in range(400))
    fat = len(b)
    b += b"#FT4" + bytes(32)
    thmb = len(b)
    b += b"\xff\xff\xff\xffThmb" + struct.pack("<IIIII", 1, len(preview) + 13, 29, 0, len(preview)) + b"\x01" + preview
    b += b"\xff\xff\xff\xffMeta" + b'{"document":{"title":"poster","pageCount":1}}'
    struct.pack_into("<QQQ", b, 16, fat, thmb, 0)
    return bytes(b)


def pixelmator():
    z = io.BytesIO()
    with zipfile.ZipFile(z, "w", zipfile.ZIP_STORED) as zf:
        zf.writestr("metadata.info", b"SQLite format 3\x00" + bytes(240))
        zf.writestr("QuickLook/Thumbnail.webp", webp(96, 96, (40, 160, 90)))
        zf.writestr("QuickLook/Icon.webp", webp(32, 32, (40, 160, 90)))
    return z.getvalue()


def packbits(row):
    """PackBits: a repeat run for three or more equal bytes (at most 128),
    literal runs (at most 128 bytes) for the rest."""
    out = bytearray()
    i, n = 0, len(row)
    while i < n:
        j = i
        while j + 1 < n and row[j + 1] == row[i] and j - i < 127:
            j += 1
        run = j - i + 1
        if run >= 3:
            out.append((257 - run) & 0xFF)
            out.append(row[i])
            i = j + 1
            continue
        lit = bytearray()
        while i < n and len(lit) < 128:
            if i + 2 < n and row[i] == row[i + 1] == row[i + 2]:
                break
            lit.append(row[i])
            i += 1
        out.append(len(lit) - 1)
        out += lit
    return bytes(out)


def psd(width, height):
    im = Image.new("RGB", (width, height))
    for y in range(height):
        for x in range(width):
            im.putpixel((x, y), (int(255 * (8 * x // width) / 7), int(255 * y / height), 160))
    for y in range(height // 4, height // 2):
        for x in range(width // 4, width // 2):
            im.putpixel((x, y), (250, 250, 250))
    thumb = im.copy()
    thumb.thumbnail((160, 160))
    jpg = io.BytesIO()
    thumb.save(jpg, "JPEG", quality=85)
    jpg = jpg.getvalue()
    tw, th = thumb.size

    b = bytearray()
    b += b"8BPS" + struct.pack(">H", 1) + bytes(6) + struct.pack(">HIIHH", 3, height, width, 8, 3)
    b += struct.pack(">I", 0)  # colour mode data

    res = bytearray()

    def block(rid, data):
        nonlocal res
        res += b"8BIM" + struct.pack(">H", rid) + b"\x00\x00" + struct.pack(">I", len(data)) + data
        if len(data) % 2:
            res += b"\x00"

    block(1005, bytes([0, 72, 0, 0, 0, 1, 0, 1, 0, 72, 0, 0, 0, 1, 0, 1]))
    block(1057, struct.pack(">I", 1) + b"\x01" + bytes(12))
    wb = (tw * 24 + 31) // 32 * 4
    block(1036, struct.pack(">IIIIIIHH", 1, tw, th, wb, wb * th, len(jpg), 24, 1) + jpg)
    b += struct.pack(">I", len(res)) + res
    b += struct.pack(">I", 0)  # no layers

    b += struct.pack(">H", 1)  # PackBits
    rows = []
    px = im.load()
    for c in range(3):
        for y in range(height):
            rows.append(packbits(bytes(px[x, y][c] for x in range(width))))
    for r in rows:
        b += struct.pack(">H", len(r))
    for r in rows:
        b += r
    return bytes(b)


def woff2_tables(path):
    """The directory of a WOFF2 file: (tag, transform version) per table."""
    data = path.read_bytes()
    from fontTools.ttLib.woff2 import WOFF2Reader

    reader = WOFF2Reader(io.BytesIO(data))
    return {tag: entry.transformVersion for tag, entry in reader.tables.items()}


def main():
    FONTS.mkdir(parents=True, exist_ok=True)
    SAMPLES.mkdir(parents=True, exist_ok=True)
    ttf = FONTS / "Go-Regular.ttf"
    if not ttf.exists():
        sys.exit(f"{ttf} is missing: copy it from golang.org/x/image/font/gofont/ttfs")

    f = TTFont(str(ttf))
    f.flavor = "woff"
    f.save(str(FONTS / "Go-Regular.woff"))
    f = TTFont(str(ttf))
    f.flavor = "woff2"
    f.save(str(FONTS / "Go-Regular.woff2"))
    woff2_compress(str(ttf), str(FONTS / "Go-Regular.hmtx.woff2"), transform_tables={"glyf", "loca", "hmtx"})
    for name in ("Go-Regular.woff2", "Go-Regular.hmtx.woff2"):
        t = woff2_tables(FONTS / name)
        print(f"{name}: glyf transform {t.get('glyf')}, hmtx transform {t.get('hmtx')}", file=sys.stderr)

    (SAMPLES / "poster.af").write_bytes(affinity(png(96, 64, (30, 90, 200), (200, 40, 120))))
    (SAMPLES / "icon.pxd").write_bytes(pixelmator())
    (SAMPLES / "sample.psd").write_bytes(psd(600, 400))
    (SAMPLES / "cython.pxd").write_bytes(b'cdef extern from "math.h":\n    double sin(double x)\n    double cos(double x)\n')
    print("wrote", FONTS, SAMPLES, file=sys.stderr)


if __name__ == "__main__":
    main()
