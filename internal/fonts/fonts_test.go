package fonts

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"unicode/utf16"

	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"github.com/brf-tech/filex-media-thumbs/internal/reader"
)

// The fixtures (testdata/fonts, scripts/gen-fixtures.py): the Go font as it
// ships, and the same font as WOFF (fontTools, zlib), as WOFF2 with the
// glyf/loca transform (every encoder's default) and as WOFF2 with the hmtx
// transform too. A decoded font is compared with the TTF by what it draws:
// every glyph's outline, advance and left side bearing, its names and its
// characters - not by its bytes, which an encoder may arrange its own way.

const maxOut = 64 << 20

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fonts", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestFormatOf(t *testing.T) {
	for b, want := range map[string]Format{
		"\x00\x01\x00\x00rest": FormatTrueType,
		"truerest":             FormatTrueType,
		"OTTOrest":             FormatOpenType,
		"ttcfrest":             FormatCollection,
		"wOFFrest":             FormatWOFF,
		"wOF2rest":             FormatWOFF2,
		"PK\x03\x04":           FormatUnknown,
		"ab":                   FormatUnknown,
	} {
		if got := FormatOf([]byte(b)); got != want {
			t.Fatalf("FormatOf(%q) = %q, want %q", b, got, want)
		}
	}
	if _, _, err := ToSFNT([]byte("not a font, at all"), maxOut); !errors.Is(err, ErrNotAFont) {
		t.Fatalf("ToSFNT(junk): %v", err)
	}
	if !IsFontExt(".WOFF2") || IsFontExt("ttc") || IsFontExt("psd") {
		t.Fatal("IsFontExt")
	}
}

// sameFont fails unless got draws exactly what want draws.
func sameFont(t *testing.T, want, got []byte) {
	t.Helper()
	fw, err := sfnt.Parse(want)
	if err != nil {
		t.Fatal(err)
	}
	fg, err := sfnt.Parse(got)
	if err != nil {
		t.Fatalf("the decoded font does not parse: %v", err)
	}
	if fw.NumGlyphs() != fg.NumGlyphs() {
		t.Fatalf("%d glyphs, want %d", fg.NumGlyphs(), fw.NumGlyphs())
	}
	var bw, bg sfnt.Buffer
	ppem := fixed.I(1000)
	for i := 0; i < fw.NumGlyphs(); i++ {
		x := sfnt.GlyphIndex(i)
		sw, errW := fw.LoadGlyph(&bw, x, ppem, nil)
		sg, errG := fg.LoadGlyph(&bg, x, ppem, nil)
		if (errW == nil) != (errG == nil) {
			t.Fatalf("glyph %d: %v / %v", i, errW, errG)
		}
		if !reflect.DeepEqual([]sfnt.Segment(sw), []sfnt.Segment(sg)) {
			t.Fatalf("glyph %d draws differently", i)
		}
		aw, _ := fw.GlyphAdvance(&bw, x, ppem, font.HintingNone)
		ag, _ := fg.GlyphAdvance(&bg, x, ppem, font.HintingNone)
		if aw != ag {
			t.Fatalf("glyph %d advance %v, want %v", i, ag, aw)
		}
	}
	for _, r := range "AaQg&@0ğŞİıçÖ€" {
		iw, _ := fw.GlyphIndex(&bw, r)
		ig, _ := fg.GlyphIndex(&bg, r)
		if iw != ig {
			t.Fatalf("%q maps to %d, want %d", r, ig, iw)
		}
	}
	nw, _ := fw.Name(&bw, sfnt.NameIDFull)
	ng, _ := fg.Name(&bg, sfnt.NameIDFull)
	if nw != ng {
		t.Fatalf("full name %q, want %q", ng, nw)
	}
	if !slices.Equal(lsbs(t, want), lsbs(t, got)) {
		t.Fatal("the left side bearings differ")
	}
}

// lsbs is every glyph's left side bearing, from hhea and hmtx.
func lsbs(t *testing.T, b []byte) []int16 {
	t.Helper()
	tables, err := tablesOf(b)
	if err != nil {
		t.Fatal(err)
	}
	hhea, hmtx, maxp := tables["hhea"], tables["hmtx"], tables["maxp"]
	metrics := int(binary.BigEndian.Uint16(hhea[34:]))
	glyphs := int(binary.BigEndian.Uint16(maxp[4:]))
	out := make([]int16, glyphs)
	for i := 0; i < glyphs; i++ {
		at := 4*metrics + 2*(i-metrics)
		if i < metrics {
			at = 4*i + 2
		}
		out[i] = int16(binary.BigEndian.Uint16(hmtx[at:]))
	}
	return out
}

func TestWOFF_DecodesToTheFont(t *testing.T) {
	ttf := fixture(t, "Go-Regular.ttf")
	got, format, err := ToSFNT(fixture(t, "Go-Regular.woff"), maxOut)
	if err != nil || format != FormatWOFF {
		t.Fatalf("ToSFNT(woff) = %q, %v", format, err)
	}
	sameFont(t, ttf, got)
}

// The glyf/loca transform undone: contours, triplet-encoded points, flags,
// bounding boxes and instructions back into glyf, and a short loca.
func TestWOFF2_DecodesToTheFont(t *testing.T) {
	ttf := fixture(t, "Go-Regular.ttf")
	got, format, err := ToSFNT(fixture(t, "Go-Regular.woff2"), maxOut)
	if err != nil || format != FormatWOFF2 {
		t.Fatalf("ToSFNT(woff2) = %q, %v", format, err)
	}
	sameFont(t, ttf, got)
}

// The hmtx transform undone too: the left side bearings left out are the
// glyphs' xMin.
func TestWOFF2_HmtxTransform(t *testing.T) {
	b := fixture(t, "Go-Regular.hmtx.woff2")
	if v := woff2TransformOf(t, b, "hmtx"); v != 1 {
		t.Fatalf("the fixture's hmtx has transform %d; scripts/gen-fixtures.py writes it with 1", v)
	}
	got, _, err := ToSFNT(b, maxOut)
	if err != nil {
		t.Fatal(err)
	}
	sameFont(t, fixture(t, "Go-Regular.ttf"), got)
}

// woff2TransformOf is the transform version of a table in a WOFF2 directory.
func woff2TransformOf(t *testing.T, b []byte, want string) int {
	t.Helper()
	r := &rd{b: b, off: 12}
	n := int(r.u16())
	r.off = 48
	for i := 0; i < n; i++ {
		flags := r.u8()
		tag := ""
		if idx := flags & 0x3F; idx == 63 {
			var b4 [4]byte
			binary.BigEndian.PutUint32(b4[:], r.u32())
			tag = string(b4[:])
		} else {
			tag = woff2Tags[idx]
		}
		r.base128()
		v := int(flags >> 6)
		if ((tag == "glyf" || tag == "loca") && v == 0) || (tag == "hmtx" && v == 1) {
			r.base128()
		}
		if tag == want {
			return v
		}
	}
	t.Fatalf("no %s table", want)
	return -1
}

func TestWOFF2_Damaged(t *testing.T) {
	good := fixture(t, "Go-Regular.woff2")
	be := binary.BigEndian

	if _, err := DecodeWOFF2(good[:60], maxOut); err == nil {
		t.Fatal("a cut file decodes")
	}

	big := bytes.Clone(good)
	be.PutUint32(big[16:], 0xFFFFFFFF) // totalSfntSize
	if _, err := DecodeWOFF2(big, maxOut); !errors.Is(err, reader.ErrTooLarge) {
		t.Fatalf("a font that unpacks past the limit: %v", err)
	}
	if _, err := DecodeWOFF2(good, 1<<10); !errors.Is(err, reader.ErrTooLarge) {
		t.Fatalf("a small limit: %v", err)
	}

	short := bytes.Clone(good)
	be.PutUint32(short[20:], be.Uint32(short[20:])-100) // totalCompressedSize
	if _, err := DecodeWOFF2(short, maxOut); err == nil {
		t.Fatal("a cut Brotli stream decodes")
	}

	ttc := bytes.Clone(good)
	copy(ttc[4:8], "ttcf")
	if _, err := DecodeWOFF2(ttc, maxOut); err == nil || !strings.Contains(err.Error(), "collection") {
		t.Fatalf("a collection: %v", err)
	}

	// A header of one table, and the directory after it.
	woff2 := func(directory ...byte) []byte {
		b := make([]byte, 48, 48+len(directory))
		copy(b, "wOF2")
		be.PutUint32(b[4:], 0x00010000)
		be.PutUint32(b[8:], uint32(48+len(directory)))
		be.PutUint16(b[12:], 1)
		return append(b, directory...)
	}
	// The first number of the directory starts with a zero byte.
	if _, err := DecodeWOFF2(woff2(0x00, 0x80, 0x01), maxOut); err == nil || !strings.Contains(err.Error(), "leading zero") {
		t.Fatalf("a leading zero: %v", err)
	}
	// An unknown transform (1) of an ordinary table (name, index 5).
	if _, err := DecodeWOFF2(woff2(0x40|0x05, 0x10), maxOut); err == nil || !strings.Contains(err.Error(), "unknown transform") {
		t.Fatalf("an unknown transform: %v", err)
	}
}

func TestWOFF2_Numbers(t *testing.T) {
	for _, c := range []struct {
		in   []byte
		want uint32
		ok   bool
	}{
		{[]byte{0x3F}, 63, true},
		{[]byte{0x81, 0x00}, 128, true},
		{[]byte{0x8F, 0xFF, 0xFF, 0xFF, 0x7F}, 0xFFFFFFFF, true},
		{[]byte{0x80, 0x01}, 0, false},                   // a leading zero
		{[]byte{0x90, 0x80, 0x80, 0x80, 0x00}, 0, false}, // past 32 bits
		{[]byte{0x81, 0x81, 0x81, 0x81, 0x81, 0x01}, 0, false},
		{[]byte{0x81}, 0, false}, // cut
	} {
		r := &rd{b: c.in}
		got := r.base128()
		if (r.err == nil) != c.ok || (c.ok && got != c.want) {
			t.Fatalf("base128(% x) = %d, %v", c.in, got, r.err)
		}
	}
	for _, c := range []struct {
		in   []byte
		want uint16
	}{
		{[]byte{252}, 252},
		{[]byte{255, 0}, 253},
		{[]byte{254, 0}, 506},
		{[]byte{253, 0x12, 0x34}, 0x1234},
	} {
		r := &rd{b: c.in}
		if got := r.u255(); got != c.want || r.err != nil {
			t.Fatalf("u255(% x) = %d, %v", c.in, got, r.err)
		}
	}
}

// Each of the 128 point encodings decodes to a step of the size and the sign
// the specification gives it.
func TestWOFF2_Triplets(t *testing.T) {
	for _, c := range []struct {
		flag   byte
		bytes  []byte
		dx, dy int32
	}{
		{0, []byte{5}, 0, -5},
		{1, []byte{5}, 0, 5},
		{3, []byte{5}, 0, 256 + 5},
		{10, []byte{7}, -7, 0},
		{11, []byte{7}, 7, 0},
		{20, []byte{0x23}, -(1 + 2), -(1 + 3)},
		{23, []byte{0x23}, 1 + 2, 1 + 3},
		{84, []byte{9, 10}, -(1 + 9), -(1 + 10)},
		{87 + 12, []byte{9, 10}, 1 + 256 + 9, 1 + 10},
		{120, []byte{0x12, 0x34, 0x56}, -(0x12<<4 + 3), -(4<<8 + 0x56)},
		{127, []byte{0x01, 0x02, 0x03, 0x04}, 0x0102, 0x0304},
	} {
		r := &rd{b: c.bytes}
		dx, dy, err := triplet(c.flag, r)
		if err != nil || dx != c.dx || dy != c.dy || r.off != len(c.bytes) {
			t.Fatalf("triplet(%d, % x) = %d, %d, %v (read %d bytes)", c.flag, c.bytes, dx, dy, err, r.off)
		}
	}
	if _, _, err := triplet(127, &rd{b: []byte{1, 2}}); err == nil {
		t.Fatal("a cut glyph stream")
	}
}

func TestWOFF_Damaged(t *testing.T) {
	good := fixture(t, "Go-Regular.woff")
	if _, err := DecodeWOFF(good[:40], maxOut); err == nil {
		t.Fatal("a cut header")
	}
	if _, err := DecodeWOFF(good[:200], maxOut); err == nil {
		t.Fatal("a cut file")
	}
	if _, err := DecodeWOFF(good, 1<<10); !errors.Is(err, reader.ErrTooLarge) {
		t.Fatalf("a small limit: %v", err)
	}
	// The first table's compressed length larger than its length.
	worse := bytes.Clone(good)
	orig := binary.BigEndian.Uint32(worse[44+12:])
	binary.BigEndian.PutUint32(worse[44+8:], orig+1)
	if _, err := DecodeWOFF(worse, maxOut); err == nil {
		t.Fatal("a table larger packed than unpacked")
	}
}

// ── what a font says about itself ───────────────────────────────────────

func TestReadInfo_TheGoFont(t *testing.T) {
	ttf := fixture(t, "Go-Regular.ttf")
	info := ReadInfo(ttf, FormatTrueType)
	if info.Family != "Go" || info.Style != "Regular" || info.FullName != "Go Regular" {
		t.Fatalf("names %q %q %q", info.Family, info.Style, info.FullName)
	}
	if !strings.HasPrefix(info.Version, "Version 2.010") || info.Designer != "Kris Holmes and Charles Bigelow" || info.Maker != "Bigelow & Holmes Inc." {
		t.Fatalf("version %q designer %q maker %q", info.Version, info.Designer, info.Maker)
	}
	if !strings.HasPrefix(info.License, "Copyright (c) 2016 Bigelow & Holmes Inc.. All rights reserved. Distribution") {
		t.Fatalf("the license is one line: %q", info.License[:min(len(info.License), 120)])
	}
	if n := len([]rune(info.License)); !strings.HasSuffix(info.License, "…") || n > maxName+1 || n < maxName-1 {
		t.Fatalf("a long license is cut: %d characters", n)
	}
	if info.Weight != 400 || info.Width != 5 || info.Italic || info.Glyphs != 712 || info.Outlines != FormatTrueType {
		t.Fatalf("info %+v", info)
	}
	if len(info.Axes) != 0 || len(info.Instances) != 0 {
		t.Fatal("a static font has no axes")
	}
	if info.CodepointsTotal < 600 || len(info.Codepoints) != info.CodepointsTotal || !slices.IsSorted(info.Codepoints) {
		t.Fatalf("%d codepoints of %d", len(info.Codepoints), info.CodepointsTotal)
	}
	for _, c := range []int{'A', 'z', 0x130, 0x131, 0x15F, 0x11F, 0x20AC} {
		if _, ok := slices.BinarySearch(info.Codepoints, c); !ok {
			t.Fatalf("U+%04X is not listed", c)
		}
	}

	// The same font as WOFF2 says the same.
	sf, format, err := ToSFNT(fixture(t, "Go-Regular.woff2"), maxOut)
	if err != nil {
		t.Fatal(err)
	}
	w := ReadInfo(sf, format)
	w.Format = info.Format
	if !reflect.DeepEqual(w, info) {
		t.Fatal("the WOFF2 says something else than the TTF")
	}
}

// nameTable writes a `name` table of Windows English records.
func nameTable(names map[int]string) []byte {
	ids := make([]int, 0, len(names))
	for id := range names {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	var strs []byte
	var recs []byte
	be := binary.BigEndian
	for _, id := range ids {
		u := utf16.Encode([]rune(names[id]))
		s := make([]byte, 2*len(u))
		for i, v := range u {
			be.PutUint16(s[2*i:], v)
		}
		for _, v := range []uint16{3, 1, 0x409, uint16(id), uint16(len(s)), uint16(len(strs))} {
			recs = be.AppendUint16(recs, v)
		}
		strs = append(strs, s...)
	}
	out := be.AppendUint16(nil, 0)
	out = be.AppendUint16(out, uint16(len(ids)))
	out = be.AppendUint16(out, uint16(6+len(recs)))
	return append(append(out, recs...), strs...)
}

// cmap4Table maps first..last to glyphs 1.. through one segment.
func cmap4Table(first, last int) []byte {
	be := binary.BigEndian
	sub := []uint16{4, 0, 0, 4, 4, 1, 0, uint16(last), 0xFFFF, 0, uint16(first), 0xFFFF, uint16((1 - first) & 0xFFFF), 1, 0, 0}
	sub[1] = uint16(2 * len(sub))
	var out []byte
	for _, v := range []uint16{0, 1, 3, 1} {
		out = be.AppendUint16(out, v)
	}
	out = be.AppendUint32(out, 12)
	for _, v := range sub {
		out = be.AppendUint16(out, v)
	}
	return out
}

// cmap12Table maps the groups (start, end, first glyph).
func cmap12Table(groups [][3]uint32) []byte {
	be := binary.BigEndian
	var out []byte
	for _, v := range []uint16{0, 1, 3, 10} {
		out = be.AppendUint16(out, v)
	}
	out = be.AppendUint32(out, 12)
	out = be.AppendUint16(out, 12)
	out = be.AppendUint16(out, 0)
	out = be.AppendUint32(out, uint32(16+12*len(groups)))
	out = be.AppendUint32(out, 0)
	out = be.AppendUint32(out, uint32(len(groups)))
	for _, g := range groups {
		out = be.AppendUint32(out, g[0])
		out = be.AppendUint32(out, g[1])
		out = be.AppendUint32(out, g[2])
	}
	return out
}

func fixedOf(v float64) uint32 { return uint32(int32(v * 65536)) }

// A variable font: its axes, with their names, and its named instances.
func TestReadInfo_AVariableFont(t *testing.T) {
	be := binary.BigEndian
	fvar := []byte{}
	for _, v := range []uint16{1, 0, 16, 2, 2, 20, 1, 12} {
		fvar = be.AppendUint16(fvar, v)
	}
	axis := func(tag string, lo, def, hi float64, nameID uint16) {
		fvar = append(fvar, tag...)
		fvar = be.AppendUint32(fvar, fixedOf(lo))
		fvar = be.AppendUint32(fvar, fixedOf(def))
		fvar = be.AppendUint32(fvar, fixedOf(hi))
		fvar = be.AppendUint16(fvar, 0)
		fvar = be.AppendUint16(fvar, nameID)
	}
	axis("wght", 100, 400, 900, 256)
	axis("wdth", 75, 100, 100, 257)
	fvar = be.AppendUint16(fvar, 258) // the instance's name
	fvar = be.AppendUint16(fvar, 0)
	fvar = be.AppendUint32(fvar, fixedOf(700))
	fvar = be.AppendUint32(fvar, fixedOf(87.5))

	os2 := make([]byte, 78)
	be.PutUint16(os2[4:], 400)
	be.PutUint16(os2[6:], 5)
	be.PutUint16(os2[62:], 1) // italic
	maxp := make([]byte, 6)
	be.PutUint16(maxp[4:], 5)
	sf, err := buildSFNT(0x00010000, map[string][]byte{
		"name": nameTable(map[int]string{1: "Test Sans", 2: "Italic", 256: "Weight", 257: "Width", 258: "Bold Semi Condensed"}),
		"fvar": fvar,
		"OS/2": os2,
		"maxp": maxp,
		"cmap": cmap4Table('A', 'C'),
	})
	if err != nil {
		t.Fatal(err)
	}
	info := ReadInfo(sf, FormatTrueType)
	want := []Axis{{Tag: "wght", Min: 100, Default: 400, Max: 900, Name: "Weight"}, {Tag: "wdth", Min: 75, Default: 100, Max: 100, Name: "Width"}}
	if !reflect.DeepEqual(info.Axes, want) {
		t.Fatalf("axes %+v", info.Axes)
	}
	if len(info.Instances) != 1 || info.Instances[0].Name != "Bold Semi Condensed" || info.Instances[0].Coords["wght"] != 700 || info.Instances[0].Coords["wdth"] != 87.5 {
		t.Fatalf("instances %+v", info.Instances)
	}
	if !info.Italic || info.Family != "Test Sans" || info.Glyphs != 5 {
		t.Fatalf("info %+v", info)
	}
	if !slices.Equal(info.Codepoints, []int{'A', 'B', 'C'}) || info.CodepointsTotal != 3 {
		t.Fatalf("codepoints %v (%d)", info.Codepoints, info.CodepointsTotal)
	}
}

func TestReadInfo_Format12AndItsBounds(t *testing.T) {
	cmap := cmap12Table([][3]uint32{{0x1F600, 0x1F602, 10}, {0x41, 0x41, 3}})
	cps, total := readCmap(cmap)
	if !slices.Equal(cps, []int{0x41, 0x1F600, 0x1F601, 0x1F602}) || total != 4 {
		t.Fatalf("format 12: %v (%d)", cps, total)
	}

	// Overlapping groups of the whole of Unicode: the walk stops, the list
	// is capped.
	var groups [][3]uint32
	for i := 0; i < 40; i++ {
		groups = append(groups, [3]uint32{0, 0x10FFFF, 1})
	}
	cps, total = readCmap(cmap12Table(groups))
	if len(cps) != MaxCodepoints || total > maxCodepointWalk || total < MaxCodepoints {
		t.Fatalf("a hostile map: %d listed, %d counted", len(cps), total)
	}

	// A group past the table, a table cut short: nothing read past its end.
	cut := cmap12Table([][3]uint32{{0x41, 0x43, 1}})
	be := binary.BigEndian
	be.PutUint32(cut[12+12:], 1000) // a thousand groups promised
	cps, _ = readCmap(cut)
	if !slices.Equal(cps, []int{0x41, 0x42, 0x43}) {
		t.Fatalf("a cut format 12: %v", cps)
	}
	if cps, total := readCmap([]byte{0, 0, 0, 1}); cps != nil || total != 0 {
		t.Fatal("a cmap without its subtable")
	}
}

// A table directory that points outside the file leaves those tables out.
func TestTablesOf_BoundsAndChecksums(t *testing.T) {
	sf, err := buildSFNT(0x00010000, map[string][]byte{
		"head": make([]byte, 54),
		"name": nameTable(map[int]string{1: "X"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if checksum(sf) != 0xB1B0AFBA {
		t.Fatalf("the whole font sums to %#x, want 0xB1B0AFBA", checksum(sf))
	}
	tables, err := tablesOf(sf)
	if err != nil || len(tables) != 2 {
		t.Fatalf("tables %v %v", len(tables), err)
	}
	bad := bytes.Clone(sf)
	binary.BigEndian.PutUint32(bad[12+8:], 0xFFFFFF00) // the first table's offset
	tables, err = tablesOf(bad)
	if err != nil || len(tables) != 1 {
		t.Fatalf("a table outside the file is left out: %d, %v", len(tables), err)
	}
	if _, err := tablesOf([]byte("\x00\x01\x00\x00\x00\x00")); err == nil {
		t.Fatal("a cut directory")
	}
	if _, err := buildSFNT(0x00010000, map[string][]byte{"toolong": nil}); err == nil {
		t.Fatal("a tag that is not four bytes")
	}
}

// ── the thumbnail ───────────────────────────────────────────────────────

// A font draws itself: "Aa" in the large band, its name in the strip.
func TestCard_DrawsTheFont(t *testing.T) {
	for _, name := range []string{"Go-Regular.ttf", "Go-Regular.woff", "Go-Regular.woff2"} {
		sf, _, err := ToSFNT(fixture(t, name), maxOut)
		if err != nil {
			t.Fatal(err)
		}
		img, err := Card(sf, name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if img.Bounds() != image.Rect(0, 0, pageW, pageH) {
			t.Fatalf("%s: %v", name, img.Bounds())
		}
		inked := 0
		for y := 20; y < bigBaseline; y++ {
			for x := pageMargin; x < 140; x++ {
				if r, _, _, _ := img.At(x, y).RGBA(); r>>8 < 0x80 {
					inked++
				}
			}
		}
		if inked < 200 {
			t.Fatalf("%s: the large letters are not drawn (%d dark pixels)", name, inked)
		}
	}
	f, err := parse(fixture(t, "Go-Regular.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	var buf sfnt.Buffer
	if got := label(f, &buf, "x.ttf"); got != "Go Regular" {
		t.Fatalf("label %q", got)
	}
}

func TestCard_Damaged(t *testing.T) {
	if _, err := Card(append([]byte{0, 1, 0, 0}, bytes.Repeat([]byte{0xFF}, 40)...), "bozuk.ttf"); err == nil || !strings.HasPrefix(err.Error(), "font:") {
		t.Fatalf("a damaged font: %v", err)
	}
	// A font with tables but no glyph of any script looked for.
	maxp := make([]byte, 6)
	sf, err := buildSFNT(0x00010000, map[string][]byte{"maxp": maxp, "cmap": cmap4Table('A', 'A')})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Card(sf, "empty.ttf"); err == nil {
		t.Fatal("a font sfnt cannot read draws")
	}
}
