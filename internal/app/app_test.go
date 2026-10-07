package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brf-tech/filex/backend/pkg/pluginkit"
	"github.com/brf-tech/filex/backend/pkg/pluginkit/plugintest"
	"github.com/brf-tech/filex/backend/pkg/pluginkit/wire"

	mediathumbs "github.com/brf-tech/filex-media-thumbs"
	"github.com/brf-tech/filex-media-thumbs/internal/design"
)

// The app against plugintest's fake filex: the manifest's permissions are
// the grant, so a call the app did not ask for is refused here as filex
// refuses it. The files are the samples scripts/gen-fixtures.py writes.

// kitHost hands the app the fake filex's files.
type kitHost struct{ h *plugintest.Host }

func (k kitHost) Open(ref string) (io.ReadCloser, error) {
	b, err := k.h.ReadInput(ref)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func harness() *plugintest.Harness {
	m := mediathumbs.Manifest()
	return plugintest.NewFor(m, func(h *plugintest.Host) *pluginkit.Plugin { return New(kitHost{h}, m).Plugin() })
}

func file(t *testing.T, dir, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// thumbnail runs the export the way filex calls it: one file, its name, its
// type, the box.
func thumbnail(t *testing.T, h *plugintest.Harness, name, mime string, data []byte) (*wire.ThumbnailOutput, error) {
	t.Helper()
	ref := h.Host.AddInput(plugintest.File{Name: name, Mime: mime, Data: data})
	ext := strings.TrimPrefix(filepath.Ext(name), ".")
	return h.Plugin.Thumbnail(&wire.ThumbnailInput{
		File: ref, Ext: ext, MaxWidth: wire.ThumbnailSize, MaxHeight: wire.ThumbnailSize,
		MaxOutputBytes: wire.ThumbnailMaxOutputBytes, Locale: "en",
	})
}

// answered checks an answer the way filex does before a pixel is decoded,
// and says its format and size.
func answered(t *testing.T, out *wire.ThumbnailOutput) (string, image.Config) {
	t.Helper()
	if out == nil || len(out.Image) == 0 || len(out.Image) > wire.ThumbnailMaxOutputBytes {
		t.Fatal("no answer, or one over 4 MiB")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(out.Image))
	if err != nil {
		t.Fatal(err)
	}
	if format != "png" && format != "jpeg" {
		t.Fatalf("a thumbnail is a PNG or a JPEG, not %s", format)
	}
	if cfg.Width > wire.ThumbnailMaxPixels || cfg.Height > wire.ThumbnailMaxPixels {
		t.Fatalf("%dx%d", cfg.Width, cfg.Height)
	}
	return format, cfg
}

func TestThumbnail_EveryKind(t *testing.T) {
	for _, c := range []struct {
		dir, name, mime string
		w, h            int
	}{
		{"samples", "sample.psd", "image/vnd.adobe.photoshop", 320, 213}, // the merged image, sampled
		{"samples", "poster.af", "", 96, 64},                             // the preview PNG, as it is
		{"samples", "icon.pxd", "application/zip", 96, 96},               // the WebP, as a PNG
		{"fonts", "Go-Regular.ttf", "font/ttf", 320, 240},                // the font drawn with itself
		{"fonts", "Go-Regular.woff", "font/woff", 320, 240},
		{"fonts", "Go-Regular.woff2", "font/woff2", 320, 240},
		{"fonts", "Go-Regular.hmtx.woff2", "font/woff2", 320, 240},
	} {
		h := harness()
		out, err := thumbnail(t, h, c.name, c.mime, file(t, c.dir, c.name))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		_, cfg := answered(t, out)
		if cfg.Width != c.w || cfg.Height != c.h {
			t.Fatalf("%s: %dx%d, want %dx%d", c.name, cfg.Width, cfg.Height, c.w, c.h)
		}
	}
}

// The Affinity preview is small enough to hand over as the file carries it:
// filex scales it itself.
func TestThumbnail_APreviewIsHandedOverAsItIs(t *testing.T) {
	af := file(t, "samples", "poster.af")
	out, err := thumbnail(t, harness(), "poster.af", "", af)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(af, out.Image) {
		t.Fatal("the PNG in the file was not handed over as it is")
	}
}

// Cython's .pxd is text: refused by its recorded type without being read,
// and by its bytes when filex recorded no type. filex then asks the next
// handler, or keeps the type icon.
func TestThumbnail_CythonIsRefused(t *testing.T) {
	text := file(t, "samples", "cython.pxd")
	for _, mime := range []string{"text/x-cython", "text/plain; charset=utf-8", ""} {
		_, err := thumbnail(t, harness(), "decl.pxd", mime, text)
		var nf *design.NotFormatError
		if !errors.As(err, &nf) || nf.Reason != "cython" {
			t.Fatalf("type %q: %v", mime, err)
		}
	}
}

// Anything that is not what its name says, or not a kind the app draws, is
// an error (filex asks the next handler), never a panic.
func TestThumbnail_RefusalsAreErrors(t *testing.T) {
	for _, c := range []struct{ name, body string }{
		{"x.png", "\x89PNG"},
		{"bozuk.ttf", "\x00\x01\x00\x00" + strings.Repeat("\xff", 40)},
		{"bozuk.woff2", "wOF2" + strings.Repeat("\x00", 60)},
		{"bozuk.psd", "8BPS"},
		{"bozuk.af", "\x00\xffKA"},
		{"bos.psd", ""},
	} {
		if out, err := thumbnail(t, harness(), c.name, "", []byte(c.body)); err == nil {
			t.Fatalf("%s: an answer of %d bytes", c.name, len(out.Image))
		}
	}
}

// panicHost fails the way a hostile file could make a decoder fail.
type panicHost struct{}

func (panicHost) Open(string) (io.ReadCloser, error) { panic("a hostile file") }

func TestThumbnail_APanicIsTheFilesFailure(t *testing.T) {
	a := New(panicHost{}, mediathumbs.Manifest())
	_, err := a.Thumbnail(&wire.ThumbnailInput{File: wire.FileRef{Ref: "in:0", Name: "x.psd", Size: 10}, Ext: "psd"})
	if err == nil || !strings.Contains(err.Error(), "a hostile file") {
		t.Fatalf("a panic: %v", err)
	}
	ans, err := a.Preview(&wire.UICallInput{Context: wire.CallContext{Inputs: []wire.FileRef{{Ref: "in:0", Name: "x.psd", Size: 10}}}})
	if err != nil {
		t.Fatal(err)
	}
	if p := ans.(*PreviewAnswer); p.Problem != "damaged" {
		t.Fatalf("a panic in the page's call: %+v", p)
	}
}

// uiCall runs one of the page's calls on a file and reads the answer back
// from its JSON, as the page gets it.
func uiCall(t *testing.T, view, method string, f plugintest.File, into any) *wire.UICallOutput {
	t.Helper()
	out, err := harness().UICall(view, method, nil, f)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Error) == 0 {
		b, err := json.Marshal(out.Result)
		if err != nil {
			t.Fatal(err)
		}
		if len(b) > 4<<20 {
			t.Fatalf("%s answers %d bytes; filex caps an answer at 4 MiB", method, len(b))
		}
		if err := json.Unmarshal(b, into); err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func TestPreview_EachDesignFile(t *testing.T) {
	var af PreviewAnswer
	uiCall(t, "design", MethodPreview, plugintest.File{Name: "poster.af", Data: file(t, "samples", "poster.af")}, &af)
	if af.Kind != "affinity" || af.Program != "Affinity" || af.Problem != "" || af.Picture == nil {
		t.Fatalf("affinity: %+v", af)
	}
	if af.Picture.Mime != "image/png" || af.Picture.Width != 96 || af.Picture.Drawn || af.Picture.Source != "preview" {
		t.Fatalf("affinity picture: %+v", af.Picture)
	}
	if af.Facts["container"] != float64(12) || af.Facts["generation"] != "3" {
		t.Fatalf("affinity facts: %v", af.Facts)
	}

	var psd PreviewAnswer
	uiCall(t, "design", MethodPreview, plugintest.File{Name: "sample.psd", Data: file(t, "samples", "sample.psd")}, &psd)
	if psd.Picture == nil || psd.Picture.Mime != "image/jpeg" || !psd.Picture.Drawn || psd.Picture.Source != "merged" || psd.Picture.Width != 600 {
		t.Fatalf("psd picture: %+v", psd.Picture)
	}
	if _, _, err := image.DecodeConfig(bytes.NewReader(psd.Picture.Data)); err != nil {
		t.Fatalf("the page gets a JPEG it can show: %v", err)
	}
	if psd.Facts["mode"] != "rgb" || psd.Facts["layers"] != float64(0) || psd.Facts["merged"] != "real" || psd.Program != "Adobe Photoshop" {
		t.Fatalf("psd facts: %+v", psd)
	}

	var pxd PreviewAnswer
	uiCall(t, "design", MethodPreview, plugintest.File{Name: "icon.pxd", Mime: "application/zip", Data: file(t, "samples", "icon.pxd")}, &pxd)
	if pxd.Picture == nil || pxd.Picture.Mime != "image/webp" || pxd.Picture.Width != 96 || pxd.Picture.Source != "QuickLook/Thumbnail.webp" {
		t.Fatalf("pxd picture: %+v", pxd.Picture)
	}
}

// A design file without its picture still says what it is, and why.
func TestPreview_Problems(t *testing.T) {
	text := file(t, "samples", "cython.pxd")
	for _, mime := range []string{"text/x-cython", ""} {
		var ans PreviewAnswer
		uiCall(t, "design", MethodPreview, plugintest.File{Name: "decl.pxd", Mime: mime, Data: text}, &ans)
		if ans.Problem != "cython" || ans.Picture != nil {
			t.Fatalf("type %q: %+v", mime, ans)
		}
	}
	var none PreviewAnswer
	bare := append([]byte{0x00, 0xFF, 0x4B, 0x41, 12, 0, 0, 0}, []byte("nsrP#Inf")...)
	bare = append(bare, make([]byte, 200)...)
	uiCall(t, "design", MethodPreview, plugintest.File{Name: "bos.afdesign", Data: bare}, &none)
	if none.Problem != "no_preview" || none.Facts["document"] != "afdesign" || none.Program != "Affinity Designer" {
		t.Fatalf("no preview: %+v", none)
	}
	var cut PreviewAnswer
	psd := file(t, "samples", "sample.psd")
	uiCall(t, "design", MethodPreview, plugintest.File{Name: "kesik.psd", Data: psd[:len(psd)/2]}, &cut)
	if cut.Picture == nil || cut.Picture.Source != "thumbnail" {
		t.Fatalf("a cut PSD shows its thumbnail, which comes before the cut: %+v", cut)
	}
	var notPSD PreviewAnswer
	uiCall(t, "design", MethodPreview, plugintest.File{Name: "x.psd", Data: []byte("this is not a Photoshop file, only text long enough")}, &notPSD)
	if notPSD.Problem != "not_this_format" {
		t.Fatalf("not a PSD: %+v", notPSD)
	}
}

func TestFont_EachFormat(t *testing.T) {
	for _, name := range []string{"Go-Regular.ttf", "Go-Regular.woff", "Go-Regular.woff2"} {
		var ans struct {
			Problem string `json:"problem"`
			Info    struct {
				Format     string `json:"format"`
				Outlines   string `json:"outlines"`
				Family     string `json:"family"`
				Weight     int    `json:"weight"`
				Glyphs     int    `json:"glyphs"`
				Codepoints []int  `json:"codepoints"`
			} `json:"info"`
		}
		uiCall(t, "font", MethodFont, plugintest.File{Name: name, Data: file(t, "fonts", name)}, &ans)
		want := strings.TrimPrefix(filepath.Ext(name), ".")
		if want == "ttf" {
			want = "truetype"
		}
		if ans.Problem != "" || ans.Info.Format != want || ans.Info.Outlines != "truetype" || ans.Info.Family != "Go" || ans.Info.Weight != 400 || ans.Info.Glyphs != 712 {
			t.Fatalf("%s: %+v", name, ans)
		}
		if len(ans.Info.Codepoints) < 600 {
			t.Fatalf("%s: %d characters", name, len(ans.Info.Codepoints))
		}
	}
}

func TestFont_Problems(t *testing.T) {
	var junk FontAnswer
	uiCall(t, "font", MethodFont, plugintest.File{Name: "x.ttf", Data: []byte("not a font, only text")}, &junk)
	if junk.Problem != "not_a_font" || junk.Info != nil {
		t.Fatalf("junk: %+v", junk)
	}
	var cut FontAnswer
	w2 := file(t, "fonts", "Go-Regular.woff2")
	uiCall(t, "font", MethodFont, plugintest.File{Name: "x.woff2", Data: w2[:len(w2)/2]}, &cut)
	if cut.Problem != "damaged" {
		t.Fatalf("a cut WOFF2: %+v", cut)
	}
}

// A call on a kind the view does not open, or with no file, is refused with
// a sentence in both languages.
func TestPageCalls_Refusals(t *testing.T) {
	out := uiCall(t, "design", MethodPreview, plugintest.File{Name: "x.png", Data: []byte("\x89PNG")}, nil)
	if out.Error["en"] == "" || out.Error["tr"] == "" {
		t.Fatalf("a .png in the design view: %+v", out)
	}
	out = uiCall(t, "font", MethodFont, plugintest.File{Name: "x.psd", Data: []byte("8BPS")}, nil)
	if out.Error["en"] == "" || out.Error["tr"] == "" {
		t.Fatalf("a .psd in the font view: %+v", out)
	}
	a := New(panicHost{}, mediathumbs.Manifest())
	if _, err := a.Font(&wire.UICallInput{}); err == nil {
		t.Fatal("no file")
	}
}

// The plugin registers the export and the two calls; the only findings of
// plugintest's registration check are the interface views, which no Go
// function draws (plugintest 0.52 predates app interfaces).
func TestPlugin_Registered(t *testing.T) {
	p := New(SDKHost{}, mediathumbs.Manifest()).Plugin()
	if p.Thumbnail == nil || p.UI[MethodPreview] == nil || p.UI[MethodFont] == nil || len(p.Actions) > 0 || len(p.Views) > 0 {
		t.Fatalf("plugin %+v", p)
	}
	for _, f := range plugintest.InspectRegistered(p).Errors() {
		if f.Where == "plugin.views" && (strings.Contains(f.Message, `"design"`) || strings.Contains(f.Message, `"font"`)) {
			continue
		}
		t.Errorf("%s", f)
	}
}
