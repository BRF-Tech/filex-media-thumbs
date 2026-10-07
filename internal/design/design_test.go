package design

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brf-tech/filex-media-thumbs/internal/reader"
)

// Every format is built here, in code, the way its program writes it - the
// shapes are the ones measured on real files (the README's research): a
// Photoshop file's header, resources and merged image (Adobe's
// specification); an Affinity container with its #Inf header and Thmb block
// (Affinity 3.2.3 and Photo 2.6.5 files); a Pixelmator Pro ZIP with
// QuickLook/Thumbnail.webp (Pixelmator Pro 2.4 to 4.0 files; the WebP
// pictures in testdata were made for these tests, with alpha, as Pixelmator
// writes them). Files whole, as filex hands them over: a stream.

var (
	red   = color.RGBA{0xDC, 0x1E, 0x1E, 0xFF}
	green = color.RGBA{0x1E, 0xB4, 0x3C, 0xFF}
	blue  = color.RGBA{0x1E, 0x3C, 0xDC, 0xFF}
)

const budget = 256 << 20

func solid(c color.Color, w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

func jpegOf(t *testing.T, c color.Color, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, solid(c, w, h), &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func pngOf(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, solid(c, w, h)); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// closeTo: within a JPEG's or a resampler's slack of want.
func closeTo(t *testing.T, got color.Color, want color.RGBA, msg string) {
	t.Helper()
	r, g, b, _ := got.RGBA()
	d := func(a uint32, w uint8) int {
		v := int(a>>8) - int(w)
		if v < 0 {
			return -v
		}
		return v
	}
	if d(r, want.R) >= 40 || d(g, want.G) >= 40 || d(b, want.B) >= 40 {
		t.Fatalf("%s: got %d,%d,%d want %v", msg, r>>8, g>>8, b>>8, want)
	}
}

// pictureOf is the picture of a result, decoded when it came as bytes.
func pictureOf(t *testing.T, res *Result) image.Image {
	t.Helper()
	if res == nil || res.Picture == nil {
		t.Fatal("no picture")
	}
	if res.Picture.Image != nil {
		return res.Picture.Image
	}
	img, _, err := image.Decode(bytes.NewReader(res.Picture.Raw))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func testdataFile(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func sample(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "samples", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// ── Photoshop ───────────────────────────────────────────────────────────

type psdSpec struct {
	large                                bool
	width, height, channels, depth, mode int
	compression                          int
	// planes: one per channel, width*height*depth/8 bytes each.
	planes  [][]byte
	palette []byte
	// thumb, thumb4: the JPEG of resource 1036 and 1033 (nil: none).
	thumb, thumb4 []byte
	// merged: resource 1057's hasRealMergedData, -1 for no 1057.
	merged int
	// layers: the layer and mask section as it is.
	layers []byte
}

// packBitsOf encodes a row: a repeat run when the row is one value, else
// literal runs of at most 128 bytes.
func packBitsOf(row []byte) []byte {
	var out []byte
	same := len(row) > 1
	for _, b := range row {
		same = same && b == row[0]
	}
	if same {
		for rest := len(row); rest > 0; {
			k := min(rest, 128)
			out = append(out, byte(int8(1-k)), row[0])
			rest -= k
		}
		return out
	}
	for i := 0; i < len(row); i += 128 {
		k := min(len(row)-i, 128)
		out = append(out, byte(k-1))
		out = append(out, row[i:i+k]...)
	}
	return out
}

func thumbResource(jpg []byte, w, h int) []byte {
	var b bytes.Buffer
	for _, v := range []uint32{1, uint32(w), uint32(h), uint32((w*24 + 31) / 32 * 4), uint32((w*24 + 31) / 32 * 4 * h), uint32(len(jpg))} {
		_ = binary.Write(&b, binary.BigEndian, v)
	}
	_ = binary.Write(&b, binary.BigEndian, uint16(24))
	_ = binary.Write(&b, binary.BigEndian, uint16(1))
	b.Write(jpg)
	return b.Bytes()
}

func buildPSD(s psdSpec) []byte {
	var b bytes.Buffer
	be := func(v any) { _ = binary.Write(&b, binary.BigEndian, v) }
	b.WriteString("8BPS")
	if s.large {
		be(uint16(2))
	} else {
		be(uint16(1))
	}
	b.Write(make([]byte, 6))
	be(uint16(s.channels))
	be(uint32(s.height))
	be(uint32(s.width))
	be(uint16(s.depth))
	be(uint16(s.mode))
	be(uint32(len(s.palette)))
	b.Write(s.palette)

	var res bytes.Buffer
	block := func(id int, data []byte) {
		res.WriteString("8BIM")
		_ = binary.Write(&res, binary.BigEndian, uint16(id))
		res.Write([]byte{0, 0}) // an empty name, padded to two bytes
		_ = binary.Write(&res, binary.BigEndian, uint32(len(data)))
		res.Write(data)
		if len(data)%2 == 1 {
			res.WriteByte(0)
		}
	}
	block(1005, []byte{0, 72, 0, 0, 0, 1, 0, 1, 0, 72, 0, 0, 0, 1, 0, 1}) // resolution info, unread
	if s.merged >= 0 {
		block(1057, []byte{0, 0, 0, 1, byte(s.merged), 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1})
	}
	if s.thumb4 != nil {
		block(1033, thumbResource(s.thumb4, 8, 8))
	}
	if s.thumb != nil {
		block(1036, thumbResource(s.thumb, 8, 8))
	}
	be(uint32(res.Len()))
	b.Write(res.Bytes())

	if s.large {
		be(uint64(len(s.layers)))
	} else {
		be(uint32(len(s.layers)))
	}
	b.Write(s.layers)

	be(uint16(s.compression))
	rowBytes := s.width * s.depth / 8
	if s.compression == 0 {
		for _, p := range s.planes {
			b.Write(p)
		}
		return b.Bytes()
	}
	var rows [][]byte
	for _, p := range s.planes {
		for y := 0; y < s.height; y++ {
			rows = append(rows, packBitsOf(p[y*rowBytes:(y+1)*rowBytes]))
		}
	}
	for _, r := range rows {
		if s.large {
			be(uint32(len(r)))
		} else {
			be(uint16(len(r)))
		}
	}
	for _, r := range rows {
		b.Write(r)
	}
	return b.Bytes()
}

// rgbPlanes: a w x h RGB image, its left half left and its right half right.
func rgbPlanes(w, h int, left, right color.RGBA) [][]byte {
	planes := [][]byte{make([]byte, w*h), make([]byte, w*h), make([]byte, w*h)}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := left
			if x >= w/2 {
				c = right
			}
			planes[0][y*w+x], planes[1][y*w+x], planes[2][y*w+x] = c.R, c.G, c.B
		}
	}
	return planes
}

func readPSDBytes(b []byte, box int) (*Result, error) {
	src := reader.FromBytes(b, budget)
	defer src.Close()
	return Read(KindPSD, "psd", src, box)
}

// The merged image is drawn, raw and RLE: its two halves stay where they are.
func TestPSD_MergedImage(t *testing.T) {
	for _, compression := range []int{0, 1} {
		b := buildPSD(psdSpec{width: 16, height: 6, channels: 3, depth: 8, mode: psdModeRGB, compression: compression,
			planes: rgbPlanes(16, 6, red, green), merged: 1})
		res, err := readPSDBytes(b, 320)
		if err != nil {
			t.Fatalf("compression %d: %v", compression, err)
		}
		img := pictureOf(t, res)
		if img.Bounds() != image.Rect(0, 0, 16, 6) {
			t.Fatalf("compression %d: %v", compression, img.Bounds())
		}
		closeTo(t, img.At(2, 3), red, "left half")
		closeTo(t, img.At(13, 3), green, "right half")
		if res.Picture.Source != "merged" {
			t.Fatalf("source %q", res.Picture.Source)
		}
		if res.Facts["format"] != "psd" || res.Facts["mode"] != "rgb" || res.Facts["width"] != 16 || res.Facts["merged"] != "real" {
			t.Fatalf("facts %v", res.Facts)
		}
	}
}

// A merged image wider than the box is sampled into it, a row and a pixel
// at a time: the picture is the box's size, never the document's.
func TestPSD_LargeMergedImageIsSampled(t *testing.T) {
	const w, h = 1280, 40
	b := buildPSD(psdSpec{width: w, height: h, channels: 3, depth: 8, mode: psdModeRGB, compression: 1,
		planes: rgbPlanes(w, h, blue, red), merged: -1})
	for _, box := range []int{320, 1600} {
		res, err := readPSDBytes(b, box)
		if err != nil {
			t.Fatal(err)
		}
		img := pictureOf(t, res)
		wantW := min(box, w)
		if img.Bounds().Dx() != wantW || img.Bounds().Dy() != h*wantW/w {
			t.Fatalf("box %d: %v", box, img.Bounds())
		}
		closeTo(t, img.At(10, 2), blue, "left")
		closeTo(t, img.At(wantW-10, 2), red, "right")
		if _, said := res.Facts["merged"]; said {
			t.Fatal("a file without 1057 says nothing of its merged image")
		}
	}
}

// 16 bits a channel: the high byte. Grayscale, CMYK (stored inverted) and
// indexed colour.
func TestPSD_OtherModes(t *testing.T) {
	gray16 := make([]byte, 4*2*2)
	for i := 0; i < len(gray16); i += 2 {
		gray16[i], gray16[i+1] = 0x80, 0xFF
	}
	res, err := readPSDBytes(buildPSD(psdSpec{width: 4, height: 2, channels: 1, depth: 16, mode: psdModeGray, planes: [][]byte{gray16}, merged: 1}), 320)
	if err != nil {
		t.Fatal(err)
	}
	closeTo(t, pictureOf(t, res).At(1, 1), color.RGBA{0x80, 0x80, 0x80, 0xFF}, "gray 16")

	// No ink but black at half: a mid gray.
	cmyk := [][]byte{bytes.Repeat([]byte{255}, 4), bytes.Repeat([]byte{255}, 4), bytes.Repeat([]byte{255}, 4), bytes.Repeat([]byte{128}, 4)}
	res, err = readPSDBytes(buildPSD(psdSpec{width: 2, height: 2, channels: 4, depth: 8, mode: psdModeCMYK, planes: cmyk, merged: 1}), 320)
	if err != nil {
		t.Fatal(err)
	}
	closeTo(t, pictureOf(t, res).At(0, 0), color.RGBA{128, 128, 128, 0xFF}, "cmyk")

	palette := make([]byte, 768)
	palette[5], palette[256+5], palette[512+5] = green.R, green.G, green.B
	res, err = readPSDBytes(buildPSD(psdSpec{width: 2, height: 2, channels: 1, depth: 8, mode: psdModeIndexed,
		planes: [][]byte{{5, 5, 5, 5}}, palette: palette, merged: 1}), 320)
	if err != nil {
		t.Fatal(err)
	}
	closeTo(t, pictureOf(t, res).At(1, 0), green, "indexed")
}

// Saved without "Maximize compatibility": 1057 says the merged image is not
// real (Photoshop writes it white), so the thumbnail resource is the picture.
func TestPSD_NoRealMergedImageUsesTheThumbnail(t *testing.T) {
	white := color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}
	b := buildPSD(psdSpec{width: 16, height: 6, channels: 3, depth: 8, mode: psdModeRGB,
		planes: rgbPlanes(16, 6, white, white), merged: 0, thumb: jpegOf(t, blue, 8, 8)})
	res, err := readPSDBytes(b, 320)
	if err != nil {
		t.Fatal(err)
	}
	if res.Picture.Source != "thumbnail" || res.Picture.Format != "jpeg" || res.Picture.Raw == nil {
		t.Fatalf("the thumbnail comes as the JPEG the file carries: %+v", res.Picture)
	}
	closeTo(t, pictureOf(t, res).At(4, 4), blue, "the thumbnail, not the white merged image")
	if res.Facts["merged"] != "not_real" {
		t.Fatalf("facts %v", res.Facts)
	}
}

// Photoshop 4's thumbnail (1033) is blue first.
func TestPSD_Photoshop4ThumbnailIsBlueFirst(t *testing.T) {
	b := buildPSD(psdSpec{width: 4, height: 4, channels: 3, depth: 32, mode: psdModeRGB, merged: -1, thumb4: jpegOf(t, red, 8, 8)})
	res, err := readPSDBytes(b, 320)
	if err != nil {
		t.Fatal(err)
	}
	closeTo(t, pictureOf(t, res).At(4, 4), color.RGBA{red.B, red.G, red.R, 0xFF}, "red and blue swapped back")
}

// A colour mode not read here (Lab) with no thumbnail: no preview. With the
// thumbnail: the thumbnail. The facts are said either way.
func TestPSD_UnreadModeFallsBack(t *testing.T) {
	lab := psdSpec{width: 2, height: 2, channels: 3, depth: 8, mode: psdModeLab, planes: rgbPlanes(2, 2, red, red), merged: 1}
	res, err := readPSDBytes(buildPSD(lab), 320)
	if !errors.Is(err, ErrNoPreview) {
		t.Fatalf("Lab without a thumbnail: %v", err)
	}
	if res == nil || res.Facts["mode"] != "lab" {
		t.Fatalf("the facts are kept without a picture: %+v", res)
	}
	lab.thumb = jpegOf(t, green, 8, 8)
	res, err = readPSDBytes(buildPSD(lab), 320)
	if err != nil {
		t.Fatal(err)
	}
	closeTo(t, pictureOf(t, res).At(2, 2), green, "the thumbnail")
}

// Damaged: a file that ends in its merged image fails; a header outside the
// format fails; not a PSD is said so.
func TestPSD_Damaged(t *testing.T) {
	whole := buildPSD(psdSpec{width: 16, height: 6, channels: 3, depth: 8, mode: psdModeRGB, compression: 0, planes: rgbPlanes(16, 6, red, red), merged: 1})
	_, err := readPSDBytes(whole[:len(whole)-40], 320)
	if !errors.Is(err, reader.ErrEndsEarly) {
		t.Fatalf("a cut file: %v", err)
	}
	huge := buildPSD(psdSpec{width: 40000, height: 2, channels: 3, depth: 8, mode: psdModeRGB, merged: 1})
	if _, err := readPSDBytes(huge, 320); err == nil || !strings.Contains(err.Error(), "outside the format") {
		t.Fatalf("a header outside the format: %v", err)
	}
	_, err = readPSDBytes([]byte("8BPX this is not a Photoshop file at all"), 320)
	if !IsNotFormat(err) {
		t.Fatalf("not a Photoshop file: %v", err)
	}
}

// A PSB: version 2, an 8-byte layer section length, 4-byte row lengths.
func TestPSD_LargeDocumentFormat(t *testing.T) {
	b := buildPSD(psdSpec{large: true, width: 10, height: 4, channels: 3, depth: 8, mode: psdModeRGB, compression: 1,
		planes: rgbPlanes(10, 4, green, blue), merged: 1, layers: bytes.Repeat([]byte{7}, 300)})
	res, err := readPSDBytes(b, 320)
	if err != nil {
		t.Fatal(err)
	}
	img := pictureOf(t, res)
	closeTo(t, img.At(1, 1), green, "left")
	closeTo(t, img.At(8, 1), blue, "right")
	if res.Facts["format"] != "psb" {
		t.Fatalf("facts %v", res.Facts)
	}
}

// The number of layers is read from the start of the layer section.
func TestPSD_LayersAreCounted(t *testing.T) {
	var layers bytes.Buffer
	_ = binary.Write(&layers, binary.BigEndian, uint32(6)) // layer info length
	_ = binary.Write(&layers, binary.BigEndian, int16(-3)) // three layers, alpha first
	layers.Write(make([]byte, 4))
	b := buildPSD(psdSpec{width: 4, height: 2, channels: 3, depth: 8, mode: psdModeRGB, planes: rgbPlanes(4, 2, red, red), merged: 1, layers: layers.Bytes()})
	res, err := readPSDBytes(b, 320)
	if err != nil {
		t.Fatal(err)
	}
	if res.Facts["layers"] != 3 {
		t.Fatalf("layers %v", res.Facts["layers"])
	}
	none := buildPSD(psdSpec{width: 4, height: 2, channels: 3, depth: 8, mode: psdModeRGB, planes: rgbPlanes(4, 2, red, red), merged: 1})
	res, err = readPSDBytes(none, 320)
	if err != nil || res.Facts["layers"] != 0 {
		t.Fatalf("no layer section: %v %v", res.Facts["layers"], err)
	}
}

// Past the reading budget (the layers stand between the head and the merged
// image, and a stream reads them): drawn from the thumbnail when the file has
// one, too large when it has not.
func TestPSD_PastTheBudget(t *testing.T) {
	spec := psdSpec{width: 8, height: 4, channels: 3, depth: 8, mode: psdModeRGB, planes: rgbPlanes(8, 4, red, red), merged: 1,
		layers: make([]byte, 64<<10)}
	_, err := Read(KindPSD, "psd", reader.FromBytes(buildPSD(spec), 16<<10), 320)
	if !errors.Is(err, reader.ErrTooLarge) {
		t.Fatalf("past the budget: %v", err)
	}
	spec.thumb = jpegOf(t, blue, 8, 8)
	res, err := Read(KindPSD, "psd", reader.FromBytes(buildPSD(spec), 16<<10), 320)
	if err != nil {
		t.Fatal(err)
	}
	closeTo(t, pictureOf(t, res).At(4, 4), blue, "the thumbnail, read before the layers")
}

// The sample file scripts/gen-fixtures.py writes (Pillow reads it the same).
func TestPSD_TheSample(t *testing.T) {
	res, err := readPSDBytes(sample(t, "sample.psd"), 320)
	if err != nil {
		t.Fatal(err)
	}
	img := pictureOf(t, res)
	if img.Bounds() != image.Rect(0, 0, 320, 213) {
		t.Fatalf("600 x 400 into 320: %v", img.Bounds())
	}
	closeTo(t, img.At(130, 80), color.RGBA{250, 250, 250, 255}, "the white square")
}

func TestUnpackBits(t *testing.T) {
	dst := make([]byte, 8)
	unpackBits(dst, []byte{2, 'a', 'b', 'c', 0xFD, 'z', 0x80, 0})
	if string(dst) != "abczzzz\x00" {
		t.Fatalf("literal, repeat, a no-op, a short row left black: %q", dst)
	}
	dst = []byte("xxxx")
	unpackBits(dst, []byte{0xF9, 'q'}) // eight q's into four
	if string(dst) != "qqqq" {
		t.Fatalf("a run past the row is cut: %q", dst)
	}
	unpackBits(dst, []byte{5, 'a'}) // a literal that promises more than it has
	if !bytes.Equal(dst, []byte{'a', 0, 0, 0}) {
		t.Fatalf("a short literal: %v", dst)
	}
}

// ── Affinity ────────────────────────────────────────────────────────────

// affinityOf builds an Affinity container: the header, compressed-looking
// streams (and, when decoy is given, a placed image's own PNG among them),
// the stream table, the preview block and the metadata after it. pointAt
// replaces the header's preview offset (0: the real one).
func affinityOf(preview, decoy []byte, pointAt uint64) []byte {
	var b bytes.Buffer
	le := func(v any) { _ = binary.Write(&b, binary.LittleEndian, v) }
	b.Write(affinityMagic)
	le(uint32(12))
	b.WriteString("nsrP#Inf")
	b.Write(make([]byte, 24)) // three offsets, filled below
	b.WriteString("Prot\x08\x00\x00\x00#Fil")
	b.Write([]byte{0x28, 0xB5, 0x2F, 0xFD})
	b.Write(bytes.Repeat([]byte{0xA5, 0x3C, 0x00, 0x91}, 64))
	if decoy != nil {
		b.Write(decoy)
	}
	b.Write(bytes.Repeat([]byte{0x11, 0x22}, 50))
	fat := b.Len()
	b.WriteString("#FT4")
	b.Write(bytes.Repeat([]byte{0x01}, 20))
	thmb := b.Len()
	b.Write([]byte{0xFF, 0xFF, 0xFF, 0xFF})
	b.WriteString("Thmb")
	le(uint32(1))
	le(uint32(len(preview) + 13))
	le(uint32(29))
	le(uint32(0))
	le(uint32(len(preview)))
	b.WriteByte(1)
	b.Write(preview)
	b.WriteString("\xff\xff\xff\xffMeta")
	b.WriteString(`{"document":{"clientVersion":"3.2.3"}}`)
	out := b.Bytes()
	binary.LittleEndian.PutUint64(out[16:], uint64(fat))
	at := uint64(thmb)
	if pointAt != 0 {
		at = pointAt
	}
	binary.LittleEndian.PutUint64(out[24:], at)
	return out
}

func readAffinityBytes(b []byte, ext string) (*Result, error) {
	src := reader.FromBytes(b, budget)
	defer src.Close()
	return Read(KindAffinity, ext, src, 320)
}

// The header leads to the preview - not to the first picture in the file,
// which here is a placed image's own PNG (an incremental save keeps one).
func TestAffinity_TheHeaderLeadsToThePreview(t *testing.T) {
	b := affinityOf(pngOf(t, 12, 8, green), pngOf(t, 4, 4, red), 0)
	res, err := readAffinityBytes(b, "afphoto")
	if err != nil {
		t.Fatal(err)
	}
	if res.Picture.Format != "png" || res.Picture.Width != 12 || res.Picture.Source != "preview" {
		t.Fatalf("picture %+v", res.Picture)
	}
	closeTo(t, pictureOf(t, res).At(3, 3), green, "the preview")
	if res.Facts["container"] != 12 || res.Facts["generation"] != "3" || res.Facts["document"] != "afphoto" {
		t.Fatalf("facts %v", res.Facts)
	}
}

// A header that does not lead to a preview block (an older container): the
// file is searched for its PNG.
func TestAffinity_SearchedWhenTheHeaderDoesNotLead(t *testing.T) {
	b := affinityOf(pngOf(t, 6, 6, blue), nil, 50)
	res, err := readAffinityBytes(b, "af")
	if err != nil {
		t.Fatal(err)
	}
	closeTo(t, pictureOf(t, res).At(1, 1), blue, "the PNG found")
}

// No preview, not an Affinity file, a preview past the limit.
func TestAffinity_NoPreviewDamagedAndTooLarge(t *testing.T) {
	noPreview := affinityOf([]byte("not a png at all, only bytes long enough to be a block"), nil, 0)
	if _, err := readAffinityBytes(noPreview, "af"); !errors.Is(err, ErrNoPreview) {
		t.Fatalf("no preview: %v", err)
	}
	if _, err := readAffinityBytes([]byte("\x00\xff\x4b\x42 not an Affinity file, and long enough to read a header"), "af"); !IsNotFormat(err) {
		t.Fatalf("not an Affinity file: %v", err)
	}
	defer func(v int64) { affinityPreviewMax = v }(affinityPreviewMax)
	affinityPreviewMax = 64
	if _, err := readAffinityBytes(affinityOf(pngOf(t, 40, 40, green), nil, 0), "af"); !errors.Is(err, reader.ErrTooLarge) {
		t.Fatalf("a preview past the limit: %v", err)
	}
}

// The sample file scripts/gen-fixtures.py writes.
func TestAffinity_TheSample(t *testing.T) {
	res, err := readAffinityBytes(sample(t, "poster.af"), "af")
	if err != nil {
		t.Fatal(err)
	}
	if res.Picture.Width != 96 || res.Picture.Height != 64 {
		t.Fatalf("picture %dx%d", res.Picture.Width, res.Picture.Height)
	}
}

// ── Pixelmator Pro ──────────────────────────────────────────────────────

// pxdOf builds a Pixelmator Pro file: a ZIP of stored members, in the order
// Pixelmator Pro writes them (deflate when asked).
func pxdOf(t *testing.T, deflate bool, members [][2]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	method := zip.Store
	if deflate {
		method = zip.Deflate
		zw.RegisterCompressor(zip.Deflate, func(w io.Writer) (io.WriteCloser, error) { return flate.NewWriter(w, flate.BestCompression) })
	}
	for _, m := range members {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: m[0], Method: method})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(m[1])); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func readPixelmatorBytes(b []byte) (*Result, error) {
	src := reader.FromBytes(b, budget)
	defer src.Close()
	return Read(KindPixelmator, "pxd", src, 320)
}

func TestPixelmator_QuickLookThumbnail(t *testing.T) {
	for _, deflate := range []bool{false, true} {
		body := pxdOf(t, deflate, [][2]string{
			{"metadata.info", "SQLite format 3\x00 and a document's tables"},
			{"data/0FF022B0-8276-4BEB-B484-94606F639857", strings.Repeat("tile", 500)},
			{"QuickLook/Thumbnail.webp", string(testdataFile(t, "pixelmator-thumbnail.webp"))},
			{"QuickLook/Icon.webp", string(testdataFile(t, "pixelmator-icon.webp"))},
		})
		res, err := readPixelmatorBytes(body)
		if err != nil {
			t.Fatalf("deflate %v: %v", deflate, err)
		}
		if res.Picture.Width != 32 || res.Picture.Format != "webp" || res.Picture.Source != "QuickLook/Thumbnail.webp" {
			t.Fatalf("the Thumbnail, not the Icon: %+v", res.Picture)
		}
		closeTo(t, pictureOf(t, res).At(4, 8), color.RGBA{220, 30, 30, 0xFF}, "the Thumbnail's opaque half")
		if res.Facts["members"] != 4 || res.Facts["preview_member"] != "QuickLook/Thumbnail.webp" {
			t.Fatalf("facts %v", res.Facts)
		}
	}
}

// The preview member far from the end: the file is read again up to it.
func TestPixelmator_APreviewBeforeALongTail(t *testing.T) {
	body := pxdOf(t, false, [][2]string{
		{"QuickLook/Thumbnail.webp", string(testdataFile(t, "pixelmator-thumbnail.webp"))},
		{"data/tiles", strings.Repeat("x", zipTail+(1<<20))},
	})
	res, err := readPixelmatorBytes(body)
	if err != nil {
		t.Fatal(err)
	}
	if res.Picture.Width != 32 {
		t.Fatalf("picture %+v", res.Picture)
	}
}

func TestPixelmator_IconWhenThereIsNoThumbnail(t *testing.T) {
	res, err := readPixelmatorBytes(pxdOf(t, false, [][2]string{{"metadata.info", "x"}, {"QuickLook/Icon.webp", string(testdataFile(t, "pixelmator-icon.webp"))}}))
	if err != nil {
		t.Fatal(err)
	}
	if res.Picture.Width != 8 {
		t.Fatalf("the Icon: %+v", res.Picture)
	}
}

func TestPixelmator_NoPreviewEncryptedAndDamaged(t *testing.T) {
	if _, err := readPixelmatorBytes(pxdOf(t, false, [][2]string{{"metadata.info", "x"}, {"data/a", "y"}})); !errors.Is(err, ErrNoPreview) {
		t.Fatalf("no QuickLook member: %v", err)
	}

	enc := pxdOf(t, false, [][2]string{{"QuickLook/Thumbnail.webp", string(testdataFile(t, "pixelmator-thumbnail.webp"))}})
	// Set the encrypted bit in the central directory entry (flags at +8).
	i := bytes.Index(enc, []byte("PK\x01\x02"))
	enc[i+8] |= 1
	if _, err := readPixelmatorBytes(enc); !errors.Is(err, ErrEncrypted) {
		t.Fatalf("an encrypted member: %v", err)
	}

	if _, err := readPixelmatorBytes([]byte("PK\x03\x04 but nothing of a ZIP after it, binary enough")); err == nil || IsNotFormat(err) {
		t.Fatalf("a ZIP header with no directory: %v", err)
	}

	defer func(v int64) { pixelmatorPreviewMax = v }(pixelmatorPreviewMax)
	pixelmatorPreviewMax = 16
	if _, err := readPixelmatorBytes(pxdOf(t, false, [][2]string{{"QuickLook/Thumbnail.webp", string(testdataFile(t, "pixelmator-thumbnail.webp"))}})); !errors.Is(err, reader.ErrTooLarge) {
		t.Fatalf("a preview past the limit: %v", err)
	}
}

// Cython's declaration files are .pxd too, and text: refused as Cython's.
// Another binary file under .pxd is refused as not Pixelmator's.
func TestPixelmator_CythonDeclarationIsRefused(t *testing.T) {
	_, err := readPixelmatorBytes(sample(t, "cython.pxd"))
	var nf *NotFormatError
	if !errors.As(err, &nf) || nf.Reason != "cython" {
		t.Fatalf("a Cython file: %v", err)
	}
	_, err = readPixelmatorBytes([]byte{0x89, 'P', 'N', 'G', 0, 0, 0, 0, 1, 2, 3, 4})
	if !errors.As(err, &nf) || nf.Reason != "" {
		t.Fatalf("a binary non-ZIP: %v", err)
	}
	if _, err := readPixelmatorBytes([]byte("PK")); !IsNotFormat(err) {
		t.Fatalf("two bytes: %v", err)
	}
	if !looksLikeText([]byte("cimport numpy\n# ünicode, cut: \xc3")) {
		t.Fatal("UTF-8 text cut inside its last character is text")
	}
	if looksLikeText([]byte("a\x00b")) {
		t.Fatal("a NUL is not text")
	}
}

func TestPixelmator_TheSample(t *testing.T) {
	res, err := readPixelmatorBytes(sample(t, "icon.pxd"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Picture.Width != 96 {
		t.Fatalf("picture %+v", res.Picture)
	}
}

// ── kinds ───────────────────────────────────────────────────────────────

func TestKindsAndPrograms(t *testing.T) {
	for name, want := range map[string]Kind{
		"psd": KindPSD, ".PSB": KindPSD, "af": KindAffinity, "afphoto": KindAffinity, "AFDESIGN": KindAffinity, "afpub": KindAffinity,
		"pxd": KindPixelmator, "fog": "", "png": "", "": "",
	} {
		if got := KindOf(name); got != want {
			t.Fatalf("KindOf(%q) = %q, want %q", name, got, want)
		}
	}
	if len(Extensions()) != 7 {
		t.Fatalf("Extensions = %v", Extensions())
	}
	if Program("afpub") != "Affinity Publisher" || Program(".psd") != "Adobe Photoshop" || Program("pxd") != "Pixelmator Pro" {
		t.Fatal("Program")
	}
}
