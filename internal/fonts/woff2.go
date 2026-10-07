package fonts

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/andybalholm/brotli"

	"github.com/brf-tech/filex-media-thumbs/internal/reader"
)

// WOFF 2.0 (W3C Recommendation, https://www.w3.org/TR/WOFF2/): an SFNT font
// whose tables are Brotli-compressed as one stream, some of them transformed
// first. Written from the specification; the reference decoders (Google's
// woff2, FreeType's sfwoff2.c, fontTools' woff2.py) were read for the points
// the specification leaves to them.
//
//	header (48 bytes)  wOF2, flavor, length, numTables, reserved,
//	                   totalSfntSize, totalCompressedSize, versions,
//	                   metadata and private data blocks (not read)
//	table directory    per table: flags (a known tag's index, or 63 and the
//	                   tag), origLength (UIntBase128), transformLength when
//	                   the table is transformed
//	compressed data    one Brotli stream: the tables back to back, in the
//	                   directory's order, each origLength bytes or, when
//	                   transformed, transformLength (a transformed loca is
//	                   not in the stream at all)
//
// Transforms undone here: glyf and loca (version 0, the default of every
// encoder: the glyphs as separate streams of contours, points, flags,
// triplet-encoded coordinates, composites, bounding boxes and instructions,
// and no loca), and hmtx (version 1: left side bearings left out where they
// equal the glyph's xMin). A font collection (flavor ttcf) is not read.

// woff2Tags are the known tags by their index in a directory entry's flags.
var woff2Tags = [63]string{
	"cmap", "head", "hhea", "hmtx", "maxp", "name", "OS/2", "post",
	"cvt ", "fpgm", "glyf", "loca", "prep", "CFF ", "VORG", "EBDT",
	"EBLC", "gasp", "hdmx", "kern", "LTSH", "PCLT", "VDMX", "vhea",
	"vmtx", "BASE", "GDEF", "GPOS", "GSUB", "EBSC", "JSTF", "MATH",
	"CBDT", "CBLC", "COLR", "CPAL", "SVG ", "sbix", "acnt", "avar",
	"bdat", "bloc", "bsln", "cvar", "fdsc", "feat", "fmtx", "fvar",
	"gvar", "hsty", "just", "lcar", "mort", "morx", "opbd", "prop",
	"trak", "Zapf", "Silf", "Glat", "Gloc", "Feat", "Sill",
}

// errWOFF2Short: a stream or a structure ends before what it says it holds.
var errWOFF2Short = fmt.Errorf("woff2: %w", reader.ErrEndsEarly)

// woff2Table is one directory entry.
type woff2Table struct {
	tag       string
	version   int // the transform version, flags bits 6-7
	origLen   uint32
	transLen  uint32
	transform bool // transformLength was given: the stream holds the transformed table
	data      []byte
}

// DecodeWOFF2 turns a WOFF2 file into the SFNT font it holds, at most maxOut
// bytes of it.
func DecodeWOFF2(b []byte, maxOut int) ([]byte, error) {
	if len(b) < 48 || string(b[:4]) != "wOF2" {
		return nil, errors.New("woff2: not a WOFF2 file")
	}
	r := &rd{b: b, off: 4}
	flavor := r.u32()
	if flavor == 0x74746366 { // ttcf
		return nil, errors.New("woff2: a font collection is not read")
	}
	length := r.u32()
	numTables := int(r.u16())
	r.u16() // reserved
	totalSfnt := r.u32()
	totalCompressed := r.u32()
	r.off = 48
	if int64(length) > int64(len(b)) {
		return nil, errWOFF2Short
	}
	if numTables == 0 || numTables > maxTables {
		return nil, fmt.Errorf("woff2: %d tables", numTables)
	}
	if int64(totalSfnt) > int64(maxOut) {
		return nil, reader.ErrTooLarge
	}

	tables := make([]*woff2Table, 0, numTables)
	byTag := make(map[string]*woff2Table, numTables)
	var streamLen int64
	for i := 0; i < numTables; i++ {
		flags := r.u8()
		t := &woff2Table{version: int(flags >> 6)}
		if idx := flags & 0x3F; idx == 63 {
			var tag [4]byte
			binary.BigEndian.PutUint32(tag[:], r.u32())
			t.tag = string(tag[:])
		} else {
			t.tag = woff2Tags[idx]
		}
		t.origLen = r.base128()
		switch {
		case (t.tag == "glyf" || t.tag == "loca") && t.version == 0:
			t.transform = true
			t.transLen = r.base128()
		case (t.tag == "glyf" || t.tag == "loca") && t.version == 3:
		case t.tag == "hmtx" && t.version == 1:
			t.transform = true
			t.transLen = r.base128()
		case t.version == 0:
		default:
			return nil, fmt.Errorf("woff2: the %q table has an unknown transform %d", t.tag, t.version)
		}
		if r.err != nil {
			return nil, r.err
		}
		if _, dup := byTag[t.tag]; dup {
			return nil, fmt.Errorf("woff2: the %q table twice", t.tag)
		}
		if t.tag == "loca" && t.transform && t.transLen != 0 {
			return nil, errors.New("woff2: a transformed loca with data of its own")
		}
		if t.transform {
			streamLen += int64(t.transLen)
		} else {
			streamLen += int64(t.origLen)
		}
		if streamLen > int64(maxOut) || int64(t.origLen) > int64(maxOut) {
			return nil, reader.ErrTooLarge
		}
		tables = append(tables, t)
		byTag[t.tag] = t
	}
	glyf, loca := byTag["glyf"], byTag["loca"]
	if (glyf == nil) != (loca == nil) || (glyf != nil && glyf.transform != loca.transform) {
		return nil, errors.New("woff2: glyf and loca must both be there, both transformed or neither")
	}

	comp := r.bytes(int(totalCompressed))
	if r.err != nil {
		return nil, r.err
	}
	stream := make([]byte, streamLen)
	br := brotli.NewReader(bytes.NewReader(comp))
	if _, err := io.ReadFull(br, stream); err != nil {
		return nil, fmt.Errorf("woff2: the compressed tables: %w", err)
	}
	var one [1]byte
	if k, _ := br.Read(one[:]); k > 0 {
		return nil, errors.New("woff2: the compressed tables are longer than the directory says")
	}

	var at int64
	for _, t := range tables {
		n := int64(t.origLen)
		if t.transform {
			n = int64(t.transLen)
		}
		if t.tag == "loca" && t.transform {
			continue
		}
		t.data = stream[at : at+n]
		at += n
	}

	out := make(map[string][]byte, len(tables))
	for _, t := range tables {
		out[t.tag] = t.data
	}
	var xMins []int16
	if glyf != nil && glyf.transform {
		g, l, mins, err := rebuildGlyf(glyf.data, int64(loca.origLen), maxOut)
		if err != nil {
			return nil, err
		}
		out["glyf"], out["loca"], xMins = g, l, mins
	}
	if hmtx := byTag["hmtx"]; hmtx != nil && hmtx.transform {
		if xMins == nil {
			if glyf == nil {
				return nil, errors.New("woff2: a transformed hmtx without glyf")
			}
			var err error
			if xMins, err = glyphXMins(out["glyf"], out["loca"], out["head"]); err != nil {
				return nil, err
			}
		}
		h, err := rebuildHmtx(hmtx.data, out["hhea"], out["maxp"], xMins)
		if err != nil {
			return nil, err
		}
		out["hmtx"] = h
	}
	return buildSFNT(flavor, out)
}

// rd reads big-endian numbers off a byte slice; the first read past its end
// sets err and every read after it answers zero.
type rd struct {
	b   []byte
	off int
	err error
}

func (r *rd) take(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || r.off+n > len(r.b) {
		r.err = errWOFF2Short
		return nil
	}
	s := r.b[r.off : r.off+n]
	r.off += n
	return s
}

func (r *rd) u8() uint8 {
	if s := r.take(1); s != nil {
		return s[0]
	}
	return 0
}

func (r *rd) u16() uint16 {
	if s := r.take(2); s != nil {
		return binary.BigEndian.Uint16(s)
	}
	return 0
}

func (r *rd) i16() int16 { return int16(r.u16()) }

func (r *rd) u32() uint32 {
	if s := r.take(4); s != nil {
		return binary.BigEndian.Uint32(s)
	}
	return 0
}

func (r *rd) bytes(n int) []byte { return r.take(n) }

// base128 reads a UIntBase128: at most five bytes, seven bits each, no
// leading zero byte, no overflow past 32 bits.
func (r *rd) base128() uint32 {
	var v uint32
	for i := 0; i < 5; i++ {
		c := r.u8()
		if r.err != nil {
			return 0
		}
		if i == 0 && c == 0x80 {
			r.err = errors.New("woff2: a number with a leading zero")
			return 0
		}
		if v&0xFE000000 != 0 {
			r.err = errors.New("woff2: a number past 32 bits")
			return 0
		}
		v = v<<7 | uint32(c&0x7F)
		if c&0x80 == 0 {
			return v
		}
	}
	r.err = errors.New("woff2: a number longer than five bytes")
	return 0
}

// u255 reads a 255UInt16.
func (r *rd) u255() uint16 {
	switch c := r.u8(); c {
	case 253:
		return r.u16()
	case 255:
		return uint16(r.u8()) + 253
	case 254:
		return uint16(r.u8()) + 253*2
	default:
		return uint16(c)
	}
}

// rebuildGlyf undoes the glyf transform: it writes the glyf table and its
// loca (of locaLen bytes, as the directory says), and answers each glyph's
// xMin for a transformed hmtx.
func rebuildGlyf(b []byte, locaLen int64, maxOut int) (glyf, loca []byte, xMins []int16, err error) {
	h := &rd{b: b}
	h.u16() // reserved
	options := h.u16()
	numGlyphs := int(h.u16())
	indexFormat := h.u16()
	sizes := make([]int, 7)
	for i := range sizes {
		sizes[i] = int(h.u32())
	}
	if h.err != nil {
		return nil, nil, nil, h.err
	}
	entry := int64(2)
	if indexFormat != 0 {
		entry = 4
	}
	if int64(numGlyphs+1)*entry != locaLen {
		return nil, nil, nil, errors.New("woff2: loca's length does not fit the number of glyphs")
	}
	if sizes[0] != 2*numGlyphs {
		return nil, nil, nil, errors.New("woff2: the contour counts do not fit the number of glyphs")
	}
	nContours := &rd{b: h.bytes(sizes[0])}
	nPoints := &rd{b: h.bytes(sizes[1])}
	flags := &rd{b: h.bytes(sizes[2])}
	glyphs := &rd{b: h.bytes(sizes[3])}
	composites := &rd{b: h.bytes(sizes[4])}
	bitmapLen := ((numGlyphs + 31) >> 5) << 2
	if sizes[5] < bitmapLen {
		return nil, nil, nil, errors.New("woff2: the bounding box stream is shorter than its bitmap")
	}
	bboxAll := h.bytes(sizes[5])
	instructions := &rd{b: h.bytes(sizes[6])}
	var overlap []byte
	if options&1 != 0 {
		overlap = h.bytes((numGlyphs + 7) >> 3)
	}
	if h.err != nil {
		return nil, nil, nil, h.err
	}
	bboxBitmap := bboxAll[:bitmapLen]
	bboxes := &rd{b: bboxAll[bitmapLen:]}
	bit := func(bm []byte, i int) bool { return bm[i>>3]&(0x80>>(i&7)) != 0 }

	var out bytes.Buffer
	locaOut := make([]byte, 0, locaLen)
	xMins = make([]int16, numGlyphs)
	putLoca := func() error {
		if indexFormat == 0 {
			if out.Len() > 0x1FFFE {
				return errors.New("woff2: the glyphs outgrow a short loca")
			}
			locaOut = binary.BigEndian.AppendUint16(locaOut, uint16(out.Len()/2))
			return nil
		}
		locaOut = binary.BigEndian.AppendUint32(locaOut, uint32(out.Len()))
		return nil
	}
	align := 4
	if indexFormat == 0 {
		align = 2
	}
	var w [2]byte
	put16 := func(v uint16) { binary.BigEndian.PutUint16(w[:], v); out.Write(w[:]) }

	for g := 0; g < numGlyphs; g++ {
		if err := putLoca(); err != nil {
			return nil, nil, nil, err
		}
		explicit := bit(bboxBitmap, g)
		n := int(nContours.i16())
		if nContours.err != nil {
			return nil, nil, nil, nContours.err
		}
		switch {
		case n == 0:
			if explicit {
				return nil, nil, nil, errors.New("woff2: an empty glyph with a bounding box")
			}
		case n > 0:
			if err := simpleGlyph(&out, n, explicit, overlap != nil && bit(overlap, g), nPoints, flags, glyphs, bboxes, instructions, &xMins[g]); err != nil {
				return nil, nil, nil, err
			}
		case n == -1:
			if !explicit {
				return nil, nil, nil, errors.New("woff2: a composite glyph without its bounding box")
			}
			xMin, yMin, xMax, yMax := bboxes.i16(), bboxes.i16(), bboxes.i16(), bboxes.i16()
			if bboxes.err != nil {
				return nil, nil, nil, bboxes.err
			}
			xMins[g] = xMin
			put16(0xFFFF)
			for _, v := range []int16{xMin, yMin, xMax, yMax} {
				put16(uint16(v))
			}
			hasInstructions := false
			for more := true; more; {
				cf := composites.u16()
				size := 4 // the glyph index and two byte arguments
				if cf&0x0001 != 0 {
					size += 2 // ARG_1_AND_2_ARE_WORDS
				}
				switch {
				case cf&0x0008 != 0:
					size += 2 // WE_HAVE_A_SCALE
				case cf&0x0040 != 0:
					size += 4 // WE_HAVE_AN_X_AND_Y_SCALE
				case cf&0x0080 != 0:
					size += 8 // WE_HAVE_A_TWO_BY_TWO
				}
				rest := composites.bytes(size)
				if composites.err != nil {
					return nil, nil, nil, composites.err
				}
				put16(cf)
				out.Write(rest)
				hasInstructions = hasInstructions || cf&0x0100 != 0
				more = cf&0x0020 != 0
			}
			if hasInstructions {
				n := glyphs.u255()
				ins := instructions.bytes(int(n))
				if glyphs.err != nil || instructions.err != nil {
					return nil, nil, nil, errWOFF2Short
				}
				put16(n)
				out.Write(ins)
			}
		default:
			return nil, nil, nil, fmt.Errorf("woff2: a glyph of %d contours", n)
		}
		for out.Len()%align != 0 {
			out.WriteByte(0)
		}
		if out.Len() > maxOut {
			return nil, nil, nil, reader.ErrTooLarge
		}
	}
	if err := putLoca(); err != nil {
		return nil, nil, nil, err
	}
	return out.Bytes(), locaOut, xMins, nil
}

// simpleGlyph writes one simple glyph of n contours.
func simpleGlyph(out *bytes.Buffer, n int, explicit, overlap bool, nPoints, flagStream, glyphs, bboxes, instructions *rd, xMinOut *int16) error {
	var xMin, yMin, xMax, yMax int16
	if explicit {
		xMin, yMin, xMax, yMax = bboxes.i16(), bboxes.i16(), bboxes.i16(), bboxes.i16()
		if bboxes.err != nil {
			return bboxes.err
		}
	}
	ends := make([]uint16, n)
	total := 0
	for c := 0; c < n; c++ {
		total += int(nPoints.u255())
		if nPoints.err != nil {
			return nPoints.err
		}
		if total > 0xFFFF {
			return errors.New("woff2: a glyph of more than 65535 points")
		}
		if total == 0 {
			return errors.New("woff2: a glyph whose first contour has no point")
		}
		ends[c] = uint16(total - 1)
	}
	dxs := make([]int32, total)
	dys := make([]int32, total)
	on := make([]bool, total)
	var x, y int32
	for p := 0; p < total; p++ {
		f := flagStream.u8()
		if flagStream.err != nil {
			return flagStream.err
		}
		on[p] = f&0x80 == 0
		dx, dy, err := triplet(f&0x7F, glyphs)
		if err != nil {
			return err
		}
		dxs[p], dys[p] = dx, dy
		x += dx
		y += dy
		if x < -32768 || x > 32767 || y < -32768 || y > 32767 {
			return errors.New("woff2: a point outside the coordinate range")
		}
		if !explicit {
			if p == 0 {
				xMin, xMax, yMin, yMax = int16(x), int16(x), int16(y), int16(y)
			} else {
				xMin, xMax = min(xMin, int16(x)), max(xMax, int16(x))
				yMin, yMax = min(yMin, int16(y)), max(yMax, int16(y))
			}
		}
	}
	insLen := glyphs.u255()
	ins := instructions.bytes(int(insLen))
	if glyphs.err != nil || instructions.err != nil {
		return errWOFF2Short
	}
	*xMinOut = xMin

	var w [2]byte
	put16 := func(v uint16) { binary.BigEndian.PutUint16(w[:], v); out.Write(w[:]) }
	put16(uint16(int16(n)))
	for _, v := range []int16{xMin, yMin, xMax, yMax} {
		put16(uint16(v))
	}
	for _, e := range ends {
		put16(e)
	}
	put16(insLen)
	out.Write(ins)

	// The TrueType encoding: a zero step takes no byte (X_IS_SAME), a step
	// of at most 255 one byte and its sign (SHORT_VECTOR, POSITIVE), anything
	// else two bytes.
	const (
		onCurve       = 0x01
		xShort        = 0x02
		yShort        = 0x04
		xSame         = 0x10
		ySame         = 0x20
		overlapSimple = 0x40
	)
	flagsOut := make([]byte, total)
	for p := 0; p < total; p++ {
		var f byte
		if on[p] {
			f |= onCurve
		}
		if p == 0 && overlap {
			f |= overlapSimple
		}
		switch dx := dxs[p]; {
		case dx == 0:
			f |= xSame
		case dx >= -255 && dx <= 255:
			f |= xShort
			if dx > 0 {
				f |= xSame
			}
		}
		switch dy := dys[p]; {
		case dy == 0:
			f |= ySame
		case dy >= -255 && dy <= 255:
			f |= yShort
			if dy > 0 {
				f |= ySame
			}
		}
		flagsOut[p] = f
	}
	out.Write(flagsOut)
	for p := 0; p < total; p++ {
		switch f, d := flagsOut[p], dxs[p]; {
		case f&xShort != 0:
			out.WriteByte(byte(abs32(d)))
		case f&xSame == 0:
			put16(uint16(int16(d)))
		}
	}
	for p := 0; p < total; p++ {
		switch f, d := flagsOut[p], dys[p]; {
		case f&yShort != 0:
			out.WriteByte(byte(abs32(d)))
		case f&ySame == 0:
			put16(uint16(int16(d)))
		}
	}
	return nil
}

func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

// triplet decodes one point's step from the glyph stream, by its flag
// (0-127; the on-curve bit already taken off): the specification's table of
// 128 encodings, in its five shapes. The low bit of the flag is the sign of
// the step it gives first, the next bit the sign of the other.
func triplet(f byte, g *rd) (dx, dy int32, err error) {
	sign := func(bit byte, v int32) int32 {
		if f&bit != 0 {
			return v
		}
		return -v
	}
	switch {
	case f < 10:
		b0 := int32(g.u8())
		dy = sign(1, int32(f&14)<<7+b0)
	case f < 20:
		b0 := int32(g.u8())
		dx = sign(1, int32((f-10)&14)<<7+b0)
	case f < 84:
		b := int32(f - 20)
		b0 := int32(g.u8())
		dx = sign(1, 1+(b&0x30)+(b0>>4))
		dy = sign(2, 1+((b&0x0C)<<2)+(b0&0x0F))
	case f < 120:
		b := int32(f - 84)
		b0, b1 := int32(g.u8()), int32(g.u8())
		dx = sign(1, 1+((b/12)<<8)+b0)
		dy = sign(2, 1+(((b%12)>>2)<<8)+b1)
	case f < 124:
		b0, b1, b2 := int32(g.u8()), int32(g.u8()), int32(g.u8())
		dx = sign(1, (b0<<4)+(b1>>4))
		dy = sign(2, ((b1&0x0F)<<8)+b2)
	default:
		b0, b1, b2, b3 := int32(g.u8()), int32(g.u8()), int32(g.u8()), int32(g.u8())
		dx = sign(1, (b0<<8)+b1)
		dy = sign(2, (b2<<8)+b3)
	}
	if g.err != nil {
		return 0, 0, g.err
	}
	return dx, dy, nil
}

// glyphXMins reads each glyph's xMin from an untransformed glyf and loca:
// what a transformed hmtx needs when glyf itself came as it is.
func glyphXMins(glyf, loca, head []byte) ([]int16, error) {
	if len(head) < 54 {
		return nil, errors.New("woff2: no head table to read loca by")
	}
	long := binary.BigEndian.Uint16(head[50:]) != 0
	entry := 2
	if long {
		entry = 4
	}
	n := len(loca)/entry - 1
	if n < 0 {
		return nil, errors.New("woff2: an empty loca")
	}
	offset := func(i int) int {
		if long {
			return int(binary.BigEndian.Uint32(loca[i*4:]))
		}
		return int(binary.BigEndian.Uint16(loca[i*2:])) * 2
	}
	out := make([]int16, n)
	for i := 0; i < n; i++ {
		start, end := offset(i), offset(i+1)
		if end == start {
			continue
		}
		if start < 0 || start+4 > len(glyf) {
			return nil, errors.New("woff2: loca points outside glyf")
		}
		out[i] = int16(binary.BigEndian.Uint16(glyf[start+2:]))
	}
	return out, nil
}

// rebuildHmtx undoes the hmtx transform: the advances are there, and the
// left side bearings left out are the glyphs' xMin (0 for an empty glyph).
func rebuildHmtx(b, hhea, maxp []byte, xMins []int16) ([]byte, error) {
	if len(hhea) < 36 || len(maxp) < 6 {
		return nil, errors.New("woff2: a transformed hmtx without hhea or maxp")
	}
	numMetrics := int(binary.BigEndian.Uint16(hhea[34:]))
	numGlyphs := int(binary.BigEndian.Uint16(maxp[4:]))
	if numMetrics < 1 || numMetrics > numGlyphs || numGlyphs > len(xMins) {
		return nil, errors.New("woff2: hmtx does not fit the number of glyphs")
	}
	r := &rd{b: b}
	flags := r.u8()
	if flags&0xFC != 0 || flags&0x03 == 0 {
		return nil, errors.New("woff2: a transformed hmtx with flags it cannot have")
	}
	advances := make([]uint16, numMetrics)
	for i := range advances {
		advances[i] = r.u16()
	}
	lsbs := make([]int16, numGlyphs)
	copy(lsbs, xMins[:numGlyphs])
	if flags&0x01 == 0 {
		for i := 0; i < numMetrics; i++ {
			lsbs[i] = r.i16()
		}
	}
	if flags&0x02 == 0 {
		for i := numMetrics; i < numGlyphs; i++ {
			lsbs[i] = r.i16()
		}
	}
	if r.err != nil {
		return nil, r.err
	}
	out := make([]byte, 0, 4*numMetrics+2*(numGlyphs-numMetrics))
	for i := 0; i < numMetrics; i++ {
		out = binary.BigEndian.AppendUint16(out, advances[i])
		out = binary.BigEndian.AppendUint16(out, uint16(lsbs[i]))
	}
	for i := numMetrics; i < numGlyphs; i++ {
		out = binary.BigEndian.AppendUint16(out, uint16(lsbs[i]))
	}
	return out, nil
}
