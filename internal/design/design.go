// Package design reads the picture a design file carries of itself:
//
//   - Photoshop (.psd, and .psb, the large document format): the merged
//     image, sampled into the size asked for a row at a time, else the JPEG
//     thumbnail resource (1036, or 1033 from Photoshop 4). psd.go.
//   - Affinity (.af of Affinity 3, and .afphoto / .afdesign / .afpub of
//     Affinity 1 and 2: one container): the PNG the container's #Inf header
//     points at. affinity.go.
//   - Pixelmator Pro (.pxd, a ZIP since Pixelmator Pro 2.4): its
//     QuickLook/Thumbnail member, read through the ZIP's directory, never
//     unpacked. A .pxd that is not a ZIP - Cython's declaration files are
//     .pxd too, and text - is refused. pixelmator.go.
//
// None of these is decoded as a document: each program saves a flattened
// picture of the document for file browsers, and that picture is what is
// read. The formats were measured on real files; the research behind each,
// with its sources, is in the README.
//
// ⚠ All of it is hostile input. A reader.Source bounds the bytes read and
// the time; every length a file states is checked before it is followed; a
// picture the file carries is checked by its own header before a pixel of it
// is decoded (picture.Check).
package design

import (
	"errors"
	"fmt"
	"image"
	"strings"

	"github.com/brf-tech/filex-media-thumbs/internal/picture"
	"github.com/brf-tech/filex-media-thumbs/internal/reader"
)

// Kind is a format this package reads.
type Kind string

const (
	KindPSD        Kind = "psd"
	KindAffinity   Kind = "affinity"
	KindPixelmator Kind = "pixelmator"
)

// kinds is the kind each extension is read as.
var kinds = map[string]Kind{
	"psd": KindPSD, "psb": KindPSD,
	"af": KindAffinity, "afphoto": KindAffinity, "afdesign": KindAffinity, "afpub": KindAffinity,
	"pxd": KindPixelmator,
}

// KindOf is the kind of a file with this extension (lower-case or not, with
// or without its dot); "" for none.
func KindOf(ext string) Kind {
	return kinds[strings.TrimPrefix(strings.ToLower(ext), ".")]
}

// Extensions are the extensions this package reads.
func Extensions() []string {
	out := make([]string, 0, len(kinds))
	for e := range kinds {
		out = append(out, e)
	}
	return out
}

// Program is the program that writes a file with this extension, as its
// maker names it.
func Program(ext string) string {
	switch strings.TrimPrefix(strings.ToLower(ext), ".") {
	case "psd", "psb":
		return "Adobe Photoshop"
	case "afphoto":
		return "Affinity Photo"
	case "afdesign":
		return "Affinity Designer"
	case "afpub":
		return "Affinity Publisher"
	case "af":
		return "Affinity"
	case "pxd":
		return "Pixelmator Pro"
	}
	return ""
}

var (
	// ErrNoPreview: the file holds no picture of itself that can be read
	// (a Photoshop file without a merged image or a thumbnail, an Affinity
	// or Pixelmator file without its preview).
	ErrNoPreview = errors.New("the file holds no preview")
	// ErrEncrypted: the preview is inside an encrypted part of the file.
	ErrEncrypted = errors.New("the preview is encrypted")
)

// NotFormatError: the file is not of the format its name says. Reason is
// "cython" for a Cython declaration file under .pxd, "" otherwise.
type NotFormatError struct {
	Format string
	Reason string
}

func (e *NotFormatError) Error() string {
	if e.Reason == "cython" {
		return "not a Pixelmator Pro file: a text .pxd is a Cython declaration file"
	}
	return "not a " + e.Format + " file"
}

// IsNotFormat reports whether err says the file is not of its format.
func IsNotFormat(err error) bool {
	var nf *NotFormatError
	return errors.As(err, &nf)
}

// Facts are what the file states about itself, for the preview's
// description: language-neutral keys and values the page puts into words.
type Facts map[string]any

// Picture is what a design file shows of itself: the bytes of a picture it
// carries as they are (Raw, of Format, Width x Height), or a picture drawn
// from it (Image: a Photoshop file's merged image, sampled).
type Picture struct {
	Raw    []byte
	Format string
	Image  image.Image
	Width  int
	Height int
	// Source says where the picture came from: "merged" or "thumbnail" (a
	// Photoshop file), "preview" (an Affinity file's Thmb block), the ZIP
	// member's name (a Pixelmator Pro file).
	Source string
}

// Result is a file read: its picture, and its facts. Facts are kept when
// the picture could not be had: the description still says what the file is.
type Result struct {
	Kind    Kind
	Picture *Picture
	Facts   Facts
}

// Read reads the picture of a file of kind from src. box is the size the
// picture is wanted at: a merged image is sampled into box x box (a picture
// the file carries comes at its own size). A non-nil Result comes back with
// an error when the file's header was read and its picture was not.
func Read(kind Kind, ext string, src *reader.Source, box int) (*Result, error) {
	switch kind {
	case KindPSD:
		return readPSD(src, box)
	case KindAffinity:
		return readAffinity(src, ext)
	case KindPixelmator:
		return readPixelmator(src)
	}
	return nil, fmt.Errorf("design: no reader for %q", kind)
}

// raw checks b, a picture a file carries, and wraps it.
func raw(b []byte, what, source string) (*Picture, error) {
	cfg, format, err := picture.Check(b, what, picture.Embedded())
	if err != nil {
		if errors.Is(err, picture.ErrRefused) {
			return nil, fmt.Errorf("%w: %v", reader.ErrTooLarge, err)
		}
		return nil, err
	}
	return &Picture{Raw: b, Format: format, Width: cfg.Width, Height: cfg.Height, Source: source}, nil
}

// Verdict reports whether err is one of the outcomes that are not a damaged
// file: no preview, encrypted, past a limit, past the time, not this format.
func Verdict(err error) bool {
	return errors.Is(err, ErrNoPreview) || errors.Is(err, ErrEncrypted) ||
		errors.Is(err, reader.ErrTooLarge) || errors.Is(err, reader.ErrTimeout) || IsNotFormat(err)
}
