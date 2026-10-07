// Package fonts reads font files for their thumbnail and their preview:
// TrueType and OpenType fonts (and the first font of a collection) as they
// are, WOFF and WOFF2 turned back into the font they hold. A font has no
// picture of itself; its thumbnail is the font drawn with itself (card.go),
// and its preview's facts - names, weight, variable axes, the characters it
// draws - are read from five of its tables (info.go).
//
// ⚠ A font is hostile input. Every offset a table states is checked against
// its bytes before it is followed; a decoded WOFF or WOFF2 is held to a
// size; the drawing goes through golang.org/x/image/font/sfnt, and the
// caller recovers a panic in it as the file's failure.
package fonts

import (
	"encoding/binary"
	"errors"
	"strings"
)

// Format is what a font file is, by its first bytes.
type Format string

const (
	FormatTrueType   Format = "truetype"
	FormatOpenType   Format = "opentype" // PostScript (CFF) outlines
	FormatCollection Format = "collection"
	FormatWOFF       Format = "woff"
	FormatWOFF2      Format = "woff2"
	FormatUnknown    Format = "unknown"
)

// FormatOf is what b's first bytes say it is.
func FormatOf(b []byte) Format {
	if len(b) < 4 {
		return FormatUnknown
	}
	switch tag := string(b[:4]); {
	case binary.BigEndian.Uint32(b) == 0x00010000 || tag == "true":
		return FormatTrueType
	case tag == "OTTO":
		return FormatOpenType
	case tag == "ttcf":
		return FormatCollection
	case tag == "wOFF":
		return FormatWOFF
	case tag == "wOF2":
		return FormatWOFF2
	}
	return FormatUnknown
}

// ErrNotAFont: the file is none of the formats above.
var ErrNotAFont = errors.New("not a font file")

// Extensions are the extensions this package reads.
func Extensions() []string { return []string{"otf", "ttf", "woff", "woff2"} }

// IsFontExt reports whether a file with this extension is read here.
func IsFontExt(ext string) bool {
	switch strings.TrimPrefix(strings.ToLower(ext), ".") {
	case "otf", "ttf", "woff", "woff2":
		return true
	}
	return false
}

// ToSFNT is the TrueType or OpenType font in b, whatever b is, at most maxOut
// bytes of it, and the format b was. A WOFF or WOFF2 is decoded; a font or
// a collection is b itself.
func ToSFNT(b []byte, maxOut int) ([]byte, Format, error) {
	f := FormatOf(b)
	switch f {
	case FormatTrueType, FormatOpenType, FormatCollection:
		return b, f, nil
	case FormatWOFF:
		out, err := DecodeWOFF(b, maxOut)
		return out, f, err
	case FormatWOFF2:
		out, err := DecodeWOFF2(b, maxOut)
		return out, f, err
	}
	return nil, f, ErrNotAFont
}

// flavorOf is the outline kind of an SFNT font (or of a collection's first
// font): OpenType for CFF, TrueType for the rest.
func flavorOf(sfnt []byte) Format {
	if len(sfnt) >= 16 && string(sfnt[:4]) == "ttcf" {
		at := int(binary.BigEndian.Uint32(sfnt[12:16]))
		if at >= 0 && at+4 <= len(sfnt) && string(sfnt[at:at+4]) == "OTTO" {
			return FormatOpenType
		}
		return FormatTrueType
	}
	if len(sfnt) >= 4 && string(sfnt[:4]) == "OTTO" {
		return FormatOpenType
	}
	return FormatTrueType
}
