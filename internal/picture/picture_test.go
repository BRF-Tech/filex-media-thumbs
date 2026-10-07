package picture

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func solid(c color.Color, w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

func pngOf(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, solid(color.RGBA{200, 30, 30, 255}, w, h)); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// A picture is measured from its own header before a pixel is decoded.
func TestCheck_TheRules(t *testing.T) {
	ok := pngOf(t, 3, 2)
	cfg, format, err := Check(ok, "the preview", Embedded())
	if err != nil || format != "png" || cfg.Width != 3 || cfg.Height != 2 {
		t.Fatalf("Check = %v %q %v", cfg, format, err)
	}

	r := Embedded()
	r.MaxSide = 2
	if _, _, err := Check(ok, "the preview", r); !errors.Is(err, ErrRefused) {
		t.Fatalf("a side over the limit: %v, want ErrRefused", err)
	}
	r = Embedded()
	r.MaxPixels = 5
	if _, _, err := Check(ok, "the preview", r); !errors.Is(err, ErrRefused) {
		t.Fatalf("an area over the limit: %v, want ErrRefused", err)
	}
	r = Embedded()
	r.MaxBytes = 10
	if _, _, err := Check(ok, "the preview", r); !errors.Is(err, ErrRefused) {
		t.Fatalf("bytes over the limit: %v, want ErrRefused", err)
	}
	r = Embedded()
	r.Formats = []string{"jpeg"}
	if _, _, err := Check(ok, "the preview", r); err == nil || errors.Is(err, ErrRefused) {
		t.Fatalf("a format not accepted: %v", err)
	}
	if _, _, err := Check(nil, "the preview", Embedded()); err == nil {
		t.Fatal("no bytes is no picture")
	}
	if _, _, err := Check([]byte("not a picture at all"), "the preview", Embedded()); err == nil {
		t.Fatal("bytes that are no picture")
	}
}

// A header that promises a huge picture is refused without decoding it.
func TestCheck_AHugeHeaderIsNotDecoded(t *testing.T) {
	b := pngOf(t, 1, 1)
	// IHDR: its type at 12, the width at 16, the height at 20, its CRC (of
	// the type and the data) at 29.
	binary.BigEndian.PutUint32(b[16:], 60_000)
	binary.BigEndian.PutUint32(b[20:], 60_000)
	binary.BigEndian.PutUint32(b[29:], crc32.ChecksumIEEE(b[12:29]))
	if _, _, err := Decode(b, "the preview", Embedded()); !errors.Is(err, ErrRefused) {
		t.Fatalf("a 60 000 x 60 000 header: %v, want ErrRefused", err)
	}
}

func TestDecode_WebPWithAlpha(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "design", "testdata", "pixelmator-thumbnail.webp"))
	if err != nil {
		t.Fatal(err)
	}
	img, format, err := Decode(b, "the preview", Embedded())
	if err != nil || format != "webp" || img.Bounds().Dx() != 32 {
		t.Fatalf("Decode(webp) = %v %q %v", img.Bounds(), format, err)
	}
	if Opaque(img) {
		t.Fatal("the WebP carries alpha")
	}
	if MimeOf(format) != "image/webp" || MimeOf("tiff") != "" {
		t.Fatal("MimeOf")
	}
}

func TestFitAndScale(t *testing.T) {
	for _, c := range []struct{ w, h, mw, mh, tw, th int }{
		{1280, 40, 320, 320, 320, 10},
		{40, 1280, 320, 320, 10, 320},
		{100, 50, 320, 320, 100, 50},
		{10000, 1, 320, 320, 320, 1},
	} {
		tw, th := Fit(c.w, c.h, c.mw, c.mh)
		if tw != c.tw || th != c.th {
			t.Fatalf("Fit(%d, %d) = %d x %d, want %d x %d", c.w, c.h, tw, th, c.tw, c.th)
		}
	}
	big := solid(color.RGBA{10, 200, 30, 255}, 640, 320)
	got := Scale(big, 320, 320)
	if got.Bounds().Dx() != 320 || got.Bounds().Dy() != 160 {
		t.Fatalf("Scale = %v", got.Bounds())
	}
	r, g, b, _ := got.At(100, 80).RGBA()
	if r>>8 > 20 || g>>8 < 180 || b>>8 > 40 {
		t.Fatalf("the colour moved: %d,%d,%d", r>>8, g>>8, b>>8)
	}
	small := solid(color.White, 10, 10)
	if Scale(small, 320, 320) != image.Image(small) {
		t.Fatal("a picture that fits is returned as it is")
	}
}

func TestEncoders(t *testing.T) {
	img := solid(color.RGBA{1, 2, 3, 255}, 8, 8)
	if !Opaque(img) {
		t.Fatal("an opaque RGBA reads as transparent")
	}
	p, err := PNG(img)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := png.Decode(bytes.NewReader(p)); err != nil {
		t.Fatal(err)
	}
	j, err := JPEG(img, 80)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jpeg.Decode(bytes.NewReader(j)); err != nil {
		t.Fatal(err)
	}
}
