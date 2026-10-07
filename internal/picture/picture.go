// Package picture checks, decodes, scales and encodes the pictures this app
// handles: the ones a design file carries (PNG, JPEG, WebP, TIFF) and the
// ones it hands filex (a thumbnail is a PNG or a JPEG; a preview anything a
// browser shows).
//
// ⚠ A picture inside a file is as hostile as the file. It is measured from
// its own header before a pixel of it is decoded (Check): its bytes, its
// format, its sides and its area. A header that promises 60 000 x 60 000
// pixels would otherwise have the decoder allocate them first and fail
// later, inside a module whose memory is a few hundred megabytes at most.
package picture

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"slices"

	_ "image/gif" // a format a preview may carry

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/tiff" // Pixelmator Pro before 2.4 saved its preview as TIFF
	_ "golang.org/x/image/webp" // Pixelmator Pro 2.4 and later save it as WebP
)

// Rules are what a picture must be before it is decoded.
type Rules struct {
	MaxBytes  int
	MaxSide   int
	MaxPixels int
	// Formats are image.DecodeConfig's names of the formats accepted.
	Formats []string
}

// Embedded are the rules for a picture a design file carries. The
// previews the programs save are small (Photoshop's 160 pixels, Affinity's
// 512, Pixelmator Pro's 1024 at most); these limits leave room for a
// larger one and none for a picture that would not fit in the module.
func Embedded() Rules {
	return Rules{
		MaxBytes:  16 << 20,
		MaxSide:   8192,
		MaxPixels: 8 << 20,
		Formats:   []string{"png", "jpeg", "webp", "tiff", "gif"},
	}
}

// ErrRefused: a picture outside the rules. Wrapped with what was wrong.
var ErrRefused = errors.New("the picture is outside the limits")

// Check reads b's header against r: its format and its size. what names
// where the picture came from in an error.
func Check(b []byte, what string, r Rules) (image.Config, string, error) {
	if len(b) == 0 {
		return image.Config{}, "", fmt.Errorf("%s: no picture", what)
	}
	if r.MaxBytes > 0 && len(b) > r.MaxBytes {
		return image.Config{}, "", fmt.Errorf("%s: %d bytes, at most %d: %w", what, len(b), r.MaxBytes, ErrRefused)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return image.Config{}, "", fmt.Errorf("%s cannot be read: %w", what, err)
	}
	if !slices.Contains(r.Formats, format) {
		return image.Config{}, "", fmt.Errorf("%s is %s, not one of %v", what, format, r.Formats)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return image.Config{}, "", fmt.Errorf("%s has no pixels", what)
	}
	if (r.MaxSide > 0 && (cfg.Width > r.MaxSide || cfg.Height > r.MaxSide)) ||
		(r.MaxPixels > 0 && int64(cfg.Width)*int64(cfg.Height) > int64(r.MaxPixels)) {
		return image.Config{}, "", fmt.Errorf("%s is %dx%d: %w", what, cfg.Width, cfg.Height, ErrRefused)
	}
	return cfg, format, nil
}

// Decode checks b against r, then decodes it.
func Decode(b []byte, what string, r Rules) (image.Image, string, error) {
	if _, _, err := Check(b, what, r); err != nil {
		return nil, "", err
	}
	img, format, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, "", fmt.Errorf("%s cannot be decoded: %w", what, err)
	}
	return img, format, nil
}

// Fit is the size of a w x h picture fitted into maxW x maxH, never larger
// than it is, at least one pixel a side.
func Fit(w, h, maxW, maxH int) (int, int) {
	if w <= maxW && h <= maxH {
		return max(w, 1), max(h, 1)
	}
	ratio := float64(w) / float64(h)
	tw, th := maxW, maxH
	if ratio > float64(maxW)/float64(maxH) {
		th = int(float64(maxW) / ratio)
	} else {
		tw = int(float64(maxH) * ratio)
	}
	return max(tw, 1), max(th, 1)
}

// Scale fits img into maxW x maxH (bilinear: Catmull-Rom looks a little
// better and is several times slower in a wasm module); a picture that
// already fits is returned as it is.
func Scale(img image.Image, maxW, maxH int) image.Image {
	b := img.Bounds()
	if b.Dx() <= maxW && b.Dy() <= maxH {
		return img
	}
	tw, th := Fit(b.Dx(), b.Dy(), maxW, maxH)
	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	xdraw.BiLinear.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	return dst
}

// Opaque reports whether img has no transparent pixel, as far as its type
// can tell without looking at every pixel twice.
func Opaque(img image.Image) bool {
	if o, ok := img.(interface{ Opaque() bool }); ok {
		return o.Opaque()
	}
	return false
}

// PNG encodes img.
func PNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// JPEG encodes img at quality q.
func JPEG(img image.Image, q int) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: q}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// MimeOf is the media type of an image.DecodeConfig format name; "" for one
// a browser does not show (TIFF).
func MimeOf(format string) string {
	switch format {
	case "png":
		return "image/png"
	case "jpeg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	case "gif":
		return "image/gif"
	}
	return ""
}
