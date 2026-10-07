package fonts

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// A font's thumbnail is the font itself: "Aa" large, its capitals, small
// letters and digits under it, and its name in a strip along the bottom, on
// the page filex draws its own text and archive thumbnails on (320 x 240, the
// same paper and ink), so a folder of fonts reads like the rest of the
// explorer. A font without Latin letters is drawn with the first letters it
// does have (Greek, Cyrillic, Hebrew, Arabic, Devanagari, Thai, kana, CJK,
// Hangul), and a symbol or icon font with its symbols.
//
// Read by golang.org/x/image/font/sfnt: TrueType and PostScript (CFF)
// outlines, the first font of a collection. It does not read CFF2, and a
// colour font (emoji bitmaps) has no outline to draw: ErrNoOutlines then. A
// variable font is drawn at its default instance.

// The page and the card's layout.
const (
	pageW        = 320
	pageH        = 240
	pageMargin   = 14
	captionSize  = 13
	bigSize      = 80
	bigBaseline  = 86
	lineSize     = 17
	firstLine    = 120
	lineLead     = 22
	captionStrip = 32
	// probe: how many letters of the other scripts are looked for.
	probe = 60
)

var (
	paper = color.RGBA{0xFB, 0xFB, 0xF8, 0xFF}
	ink   = color.RGBA{0x2A, 0x2E, 0x36, 0xFF}
	muted = color.RGBA{0x7A, 0x80, 0x8A, 0xFF}
	rule  = color.RGBA{0xE2, 0xE4, 0xE8, 0xFF}
)

// ErrNoOutlines: the font has nothing this can draw (a colour or bitmap
// font, or a font with no letter of any script looked for).
var ErrNoOutlines = errors.New("the font has no outline to draw")

// latinLines are the lines a font with Latin letters is drawn with.
var latinLines = []string{"ABCDEFGHIJKLMNOPQRSTUVWXYZ", "abcdefghijklmnopqrstuvwxyz", "0123456789 &?!@"}

// scripts are looked through, in order, for a font without Latin letters.
var scripts = [][2]rune{
	{0x00C0, 0x00FF}, // Latin-1 letters
	{0x0391, 0x03C9}, // Greek
	{0x0410, 0x044F}, // Cyrillic
	{0x05D0, 0x05EA}, // Hebrew
	{0x0627, 0x064A}, // Arabic
	{0x0905, 0x0939}, // Devanagari
	{0x0E01, 0x0E2E}, // Thai
	{0x3041, 0x3096}, // Hiragana
	{0x30A1, 0x30FA}, // Katakana
	{0x4E00, 0x4FFF}, // CJK ideographs, the first of them
	{0xAC00, 0xAD00}, // Hangul
	{0x2190, 0x21FF}, // arrows
	{0x2600, 0x27BF}, // symbols and dingbats
	{0xE000, 0xE0FF}, // private use: icon fonts
	{0xF000, 0xF0FF}, // private use: symbol fonts (Wingdings and the like)
	{0x0021, 0x007E}, // ASCII: a font with a few Latin signs and nothing else
}

// parse reads a TrueType or OpenType font; of a collection, the first.
func parse(b []byte) (*sfnt.Font, error) {
	if len(b) >= 4 && string(b[:4]) == "ttcf" {
		c, err := sfnt.ParseCollection(b)
		if err != nil {
			return nil, err
		}
		return c.Font(0)
	}
	return sfnt.Parse(b)
}

// Card draws the thumbnail of the font in data (an SFNT: ToSFNT first).
// fileName stands in for a font that does not name itself.
func Card(data []byte, fileName string) (image.Image, error) {
	f, err := parse(data)
	if err != nil {
		return nil, fmt.Errorf("font: %w", err)
	}
	var buf sfnt.Buffer
	sample := sampleOf(f, &buf)
	if len(sample.big) == 0 {
		return nil, ErrNoOutlines
	}
	// A colour font (emoji bitmaps or layers) has no outline to draw.
	if gi, err := f.GlyphIndex(&buf, sample.big[0]); err == nil && gi != 0 {
		if _, err := f.LoadGlyph(&buf, gi, fixed.I(bigSize), nil); err != nil {
			if errors.Is(err, sfnt.ErrColoredGlyph) {
				return nil, ErrNoOutlines
			}
			return nil, fmt.Errorf("font: %w", err)
		}
	}
	big, err := opentype.NewFace(f, &opentype.FaceOptions{Size: bigSize, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return nil, fmt.Errorf("font: %w", err)
	}
	small, err := opentype.NewFace(f, &opentype.FaceOptions{Size: lineSize, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return nil, fmt.Errorf("font: %w", err)
	}
	caption, err := captionFace()
	if err != nil {
		return nil, err
	}
	width := pageW - 2*pageMargin
	img := image.NewRGBA(image.Rect(0, 0, pageW, pageH))
	draw.Draw(img, img.Bounds(), image.NewUniform(paper), image.Point{}, draw.Src)
	put(img, big, ink, pageMargin, bigBaseline, fit(big, string(sample.big), width))
	y := firstLine
	for _, line := range sample.lines {
		put(img, small, ink, pageMargin, y, fit(small, line, width))
		y += lineLead
	}
	draw.Draw(img, image.Rect(0, pageH-captionStrip, pageW, pageH), image.NewUniform(rule), image.Point{}, draw.Src)
	put(img, caption, muted, pageMargin, pageH-captionStrip/2+captionSize/2-2, fit(caption, label(f, &buf, fileName), width))
	return img, nil
}

// captionFace is Go Mono, the face filex writes its own text thumbnails in.
// A face is made for each drawing: a Face is not safe to share.
func captionFace() (font.Face, error) {
	f, err := opentype.Parse(gomono.TTF)
	if err != nil {
		return nil, err
	}
	return opentype.NewFace(f, &opentype.FaceOptions{Size: captionSize, DPI: 72, Hinting: font.HintingFull})
}

// sample is what a font is drawn with: the large letters and the lines.
type sample struct {
	big   []rune
	lines []string
}

// sampleOf picks what f can draw: Latin when it has it, else the first
// letters or symbols of scripts it has.
func sampleOf(f *sfnt.Font, buf *sfnt.Buffer) sample {
	has := func(r rune) bool {
		gi, err := f.GlyphIndex(buf, r)
		return err == nil && gi != 0
	}
	if has('A') || has('a') || has('0') {
		var s sample
		for _, r := range "Aa" {
			if has(r) {
				s.big = append(s.big, r)
			}
		}
		for _, line := range latinLines {
			if kept := keepRunes(line, has); strings.TrimSpace(kept) != "" {
				s.lines = append(s.lines, kept)
			}
		}
		if len(s.big) == 0 && len(s.lines) > 0 {
			first := []rune(strings.TrimSpace(s.lines[0]))
			s.big = first[:min(2, len(first))]
		}
		return s
	}
	var found []rune
	for _, rg := range scripts {
		for r := rg[0]; r <= rg[1] && len(found) < probe; r++ {
			if (unicode.IsGraphic(r) || unicode.Is(unicode.Co, r)) && !unicode.IsSpace(r) && has(r) {
				found = append(found, r)
			}
		}
		if len(found) >= probe {
			break
		}
	}
	if len(found) == 0 {
		return sample{}
	}
	s := sample{big: found[:min(2, len(found))]}
	for i := 0; i < len(found) && len(s.lines) < len(latinLines); i += 20 {
		s.lines = append(s.lines, string(found[i:min(i+20, len(found))]))
	}
	return s
}

// keepRunes is s with only the letters has accepts.
func keepRunes(s string, has func(rune) bool) string {
	return strings.Map(func(r rune) rune {
		if r == ' ' || has(r) {
			return r
		}
		return -1
	}, s)
}

// label is the font's full name, else its family and style, else the
// file's name; control characters dropped.
func label(f *sfnt.Font, buf *sfnt.Buffer, fileName string) string {
	name, err := f.Name(buf, sfnt.NameIDFull)
	if err != nil || strings.TrimSpace(name) == "" {
		family, _ := f.Name(buf, sfnt.NameIDFamily)
		style, _ := f.Name(buf, sfnt.NameIDSubfamily)
		name = family + " " + style
	}
	if name = clean(name); name == "" {
		return fileName
	}
	return name
}

func put(dst draw.Image, face font.Face, c color.Color, x, baseline int, s string) {
	d := &font.Drawer{Dst: dst, Src: image.NewUniform(c), Face: face, Dot: fixed.P(x, baseline)}
	d.DrawString(s)
}

// fit cuts s to width pixels, ending in an ellipsis when it had to cut.
func fit(face font.Face, s string, width int) string {
	if font.MeasureString(face, s).Round() <= width {
		return s
	}
	const ell = "…"
	for len(s) > 0 {
		_, size := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-size]
		if font.MeasureString(face, s+ell).Round() <= width {
			return strings.TrimRight(s, " ") + ell
		}
	}
	return ell
}
