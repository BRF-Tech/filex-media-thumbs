// Package app is what filex calls: the `thumbnail` export (the picture
// filex shows for a file in every view) and the two calls the preview page
// makes to its module (`preview` for a design file, `font` for a font). It
// is assembled over a Host so the tests can hand it plugintest's fake filex
// (app_test.go) and the module the real one (cmd/plugin).
package app

import (
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/brf-tech/filex-media-thumbs/internal/design"
	"github.com/brf-tech/filex-media-thumbs/internal/fonts"
	"github.com/brf-tech/filex-media-thumbs/internal/picture"
	"github.com/brf-tech/filex-media-thumbs/internal/reader"
	"github.com/brf-tech/filex/backend/pkg/pluginkit"
	"github.com/brf-tech/filex/backend/pkg/pluginkit/thumbkit"
	"github.com/brf-tech/filex/backend/pkg/pluginkit/wire"
)

// The calls the preview page makes (`fx.call(method)`).
const (
	MethodPreview = "preview"
	MethodFont    = "font"
)

// The limits a call works under. filex's own stand behind them - the
// largest file it sends a thumbnail call (32 MB unless the administrator
// raised it), the time per call (10 s for a thumbnail, the manifest's
// call_timeout_s for the page), the module's memory - and these turn a
// file that is too much into a reason instead of a killed instance.
const (
	// thumbDeadline / pageDeadline: the call gives up a little before filex
	// would close it. A thumbnail's time is the administrator's (1 to 60 s);
	// past it filex records app_timeout, which says the right thing.
	thumbDeadline = 55 * time.Second
	pageDeadline  = 25 * time.Second
	// fontMaxBytes: a font is read whole; the largest CJK families are
	// around 20 MB. sfntMaxBytes: what a WOFF or WOFF2 may unpack to.
	fontMaxBytes = 32 << 20
	sfntMaxBytes = 64 << 20
	// viewBox: the size a Photoshop file's merged image is drawn at for the
	// page; viewMaxBytes: the most a picture handed to the page weighs (it
	// crosses as base64 in an answer filex caps at 4 MiB).
	viewBox      = 1600
	viewMaxBytes = 2_500_000
	// passThroughSide: a PNG or JPEG the file carries this small or smaller
	// is handed to filex as it is; filex scales it itself.
	passThroughSide = 1024
)

// Host is what the app needs of filex: the one file a call was handed.
type Host interface {
	// Open opens the file of a call's reference, from its start.
	Open(ref string) (io.ReadCloser, error)
}

// SDKHost is the real filex (pluginkit's host functions; inside wasm only).
type SDKHost struct{}

// Open opens ref with file_open.
func (SDKHost) Open(ref string) (io.ReadCloser, error) {
	in, err := pluginkit.OpenInput(ref)
	if err != nil {
		return nil, err
	}
	return in, nil
}

// App answers filex's calls.
type App struct {
	host     Host
	manifest wire.Manifest
	// now is the clock the deadlines are set by (a test may stop it).
	now func() time.Time
}

// New is the app over host, describing manifest.
func New(host Host, manifest wire.Manifest) *App {
	return &App{host: host, manifest: manifest, now: time.Now}
}

// Plugin is what pluginkit.Run registers.
func (a *App) Plugin() *pluginkit.Plugin {
	return &pluginkit.Plugin{
		Manifest:  a.manifest,
		Thumbnail: a.Thumbnail,
		UI: map[string]pluginkit.UICallFunc{
			MethodPreview: a.Preview,
			MethodFont:    a.Font,
		},
	}
}

// source reads the file of ref, of size bytes (-1: unknown), until the
// deadline. The budget lets a reader go through the file about three times
// (a Pixelmator file is read to its end for its directory, then again to its
// preview), never less than 64 MB, never more than 1 GB.
func (a *App) source(ref string, size int64, deadline time.Duration) *reader.Source {
	budget := int64(64 << 20)
	if size > 0 {
		budget = max(budget, min(3*size+(16<<20), 1<<30))
	} else {
		budget = 512 << 20
	}
	return reader.New(func() (io.ReadCloser, error) { return a.host.Open(ref) }, size, budget, a.now().Add(deadline))
}

// extOf is a file name's extension, lower-case, without its dot.
func extOf(name string) string {
	return strings.TrimPrefix(strings.ToLower(path.Ext(name)), ".")
}

// cythonByType: a .pxd filex recorded as text is Cython's, not Pixelmator's.
func cythonByType(ext, mime string) bool {
	return ext == "pxd" && strings.HasPrefix(strings.ToLower(mime), "text/")
}

// ── the thumbnail ──────────────────────────────────────────────────────

// Thumbnail is the `thumbnail` export: the picture of one file, a PNG or a
// JPEG. An error makes filex ask the next handler in the administrator's
// list, or keep the file's type icon, and lands in the app's log.
func (a *App) Thumbnail(in *wire.ThumbnailInput) (out *wire.ThumbnailOutput, err error) {
	defer func() {
		if r := recover(); r != nil {
			out, err = nil, fmt.Errorf("media-thumbs: reading %s failed: %v", in.File.Name, r)
		}
	}()
	ext := strings.TrimPrefix(strings.ToLower(in.Ext), ".")
	if ext == "" {
		ext = extOf(in.File.Name)
	}
	src := a.source(in.File.Ref, in.File.Size, thumbDeadline)
	defer src.Close()
	if kind := design.KindOf(ext); kind != "" {
		if cythonByType(ext, in.File.Mime) {
			return nil, &design.NotFormatError{Format: "Pixelmator Pro", Reason: "cython"}
		}
		res, err := design.Read(kind, ext, src, wire.ThumbnailSize)
		if err != nil {
			return nil, err
		}
		return thumbOf(res.Picture)
	}
	if fonts.IsFontExt(ext) {
		data, err := src.ReadAll(fontMaxBytes)
		if err != nil {
			return nil, err
		}
		sf, _, err := fonts.ToSFNT(data, sfntMaxBytes)
		if err != nil {
			return nil, err
		}
		img, err := fonts.Card(sf, in.File.Name)
		if err != nil {
			return nil, err
		}
		return thumbkit.PNG(img)
	}
	return nil, fmt.Errorf("media-thumbs draws design files and fonts, not .%s", ext)
}

// thumbOf is a picture as a thumbnail answer: a small PNG or JPEG the file
// carries as it is, anything else drawn into filex's box as a PNG.
func thumbOf(p *design.Picture) (*wire.ThumbnailOutput, error) {
	if p == nil {
		return nil, design.ErrNoPreview
	}
	if p.Raw != nil && (p.Format == "png" || p.Format == "jpeg") && len(p.Raw) <= wire.ThumbnailMaxOutputBytes &&
		p.Width <= passThroughSide && p.Height <= passThroughSide {
		return &wire.ThumbnailOutput{Image: p.Raw}, nil
	}
	img := p.Image
	if img == nil {
		var err error
		if img, _, err = picture.Decode(p.Raw, "the preview", picture.Embedded()); err != nil {
			return nil, err
		}
	}
	return thumbkit.PNG(picture.Scale(img, wire.ThumbnailSize, wire.ThumbnailSize))
}

// ── the page's calls ───────────────────────────────────────────────────

// PreviewAnswer is what `preview` answers the page: what the file is, what
// it says about itself, and its picture - or why there is none (Problem:
// no_preview, encrypted, too_large, timeout, cython, not_this_format,
// damaged, unavailable). Detail is the reader's own words, in English, for
// the browser's console; the page shows a sentence of its own.
type PreviewAnswer struct {
	Kind    string         `json:"kind"`
	Program string         `json:"program"`
	Problem string         `json:"problem,omitempty"`
	Detail  string         `json:"detail,omitempty"`
	Facts   design.Facts   `json:"facts,omitempty"`
	Picture *PictureAnswer `json:"picture,omitempty"`
}

// PictureAnswer is a picture for the page: its bytes (base64 in the JSON),
// its type, its size, where it came from, and whether it was drawn here
// (Drawn: scaled or re-encoded) rather than handed over as the file
// carries it.
type PictureAnswer struct {
	Mime   string `json:"mime"`
	Data   []byte `json:"data"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Source string `json:"source"`
	Drawn  bool   `json:"drawn,omitempty"`
}

// FontAnswer is what `font` answers the page: the font's facts, or why they
// could not be read (Problem: too_large, timeout, not_a_font, damaged,
// unavailable). The page draws the font itself, from the file.
type FontAnswer struct {
	Problem string      `json:"problem,omitempty"`
	Detail  string      `json:"detail,omitempty"`
	Info    *fonts.Info `json:"info,omitempty"`
}

// errNoFile: the page was opened without a file.
var errNoFile = &pluginkit.UIError{Text: wire.Text{
	"en": "The preview was opened without a file.",
	"tr": "Önizleme bir dosya olmadan açıldı.",
}}

// firstFile is the file the page was opened with.
func firstFile(in *wire.UICallInput) (wire.FileRef, error) {
	if len(in.Context.Inputs) == 0 || in.Context.Inputs[0].Ref == "" {
		return wire.FileRef{}, errNoFile
	}
	return in.Context.Inputs[0], nil
}

// Preview is the page's `preview` call: a design file's picture and facts.
func (a *App) Preview(in *wire.UICallInput) (out any, err error) {
	f, err := firstFile(in)
	if err != nil {
		return nil, err
	}
	ext := extOf(f.Name)
	kind := design.KindOf(ext)
	if kind == "" {
		return nil, &pluginkit.UIError{Text: wire.Text{
			"en": "This preview opens Photoshop, Affinity and Pixelmator Pro files.",
			"tr": "Bu önizleme Photoshop, Affinity ve Pixelmator Pro dosyalarını açar.",
		}}
	}
	ans := &PreviewAnswer{Kind: string(kind), Program: design.Program(ext)}
	defer func() {
		if r := recover(); r != nil {
			ans.Picture, ans.Problem, ans.Detail = nil, "damaged", fmt.Sprintf("panic: %v", r)
			out, err = ans, nil
		}
	}()
	if cythonByType(ext, f.Mime) {
		ans.Problem = "cython"
		return ans, nil
	}
	src := a.source(f.Ref, f.Size, pageDeadline)
	defer src.Close()
	res, rerr := design.Read(kind, ext, src, viewBox)
	if res != nil {
		ans.Facts = res.Facts
	}
	if rerr != nil {
		ans.Problem, ans.Detail = problemOf(rerr), rerr.Error()
		return ans, nil
	}
	pic, perr := pageOf(res.Picture)
	if perr != nil {
		ans.Problem, ans.Detail = problemOf(perr), perr.Error()
		return ans, nil
	}
	ans.Picture = pic
	return ans, nil
}

// pageOf is a picture as the page shows it: as the file carries it when a
// browser shows that kind and it is small enough, else drawn into viewBox
// and encoded (JPEG when it is opaque, PNG when it is not), halved until it
// fits viewMaxBytes.
func pageOf(p *design.Picture) (*PictureAnswer, error) {
	if p == nil {
		return nil, design.ErrNoPreview
	}
	if p.Raw != nil {
		if mime := picture.MimeOf(p.Format); mime != "" && len(p.Raw) <= viewMaxBytes {
			return &PictureAnswer{Mime: mime, Data: p.Raw, Width: p.Width, Height: p.Height, Source: p.Source}, nil
		}
	}
	img := p.Image
	if img == nil {
		var err error
		if img, _, err = picture.Decode(p.Raw, "the preview", picture.Embedded()); err != nil {
			return nil, err
		}
	}
	side := viewBox
	for try := 0; try < 4; try++ {
		scaled := picture.Scale(img, side, side)
		var (
			b    []byte
			mime string
			err  error
		)
		if picture.Opaque(scaled) {
			b, err = picture.JPEG(scaled, 88)
			mime = "image/jpeg"
		} else {
			b, err = picture.PNG(scaled)
			mime = "image/png"
		}
		if err != nil {
			return nil, err
		}
		if len(b) <= viewMaxBytes {
			sb := scaled.Bounds()
			return &PictureAnswer{Mime: mime, Data: b, Width: sb.Dx(), Height: sb.Dy(), Source: p.Source, Drawn: true}, nil
		}
		side /= 2
	}
	return nil, fmt.Errorf("the preview does not fit the page's answer: %w", reader.ErrTooLarge)
}

// Font is the page's `font` call: what a font says about itself.
func (a *App) Font(in *wire.UICallInput) (out any, err error) {
	f, err := firstFile(in)
	if err != nil {
		return nil, err
	}
	if !fonts.IsFontExt(extOf(f.Name)) {
		return nil, &pluginkit.UIError{Text: wire.Text{
			"en": "This preview opens fonts: .ttf, .otf, .woff and .woff2.",
			"tr": "Bu önizleme fontları açar: .ttf, .otf, .woff ve .woff2.",
		}}
	}
	ans := &FontAnswer{}
	defer func() {
		if r := recover(); r != nil {
			ans.Info, ans.Problem, ans.Detail = nil, "damaged", fmt.Sprintf("panic: %v", r)
			out, err = ans, nil
		}
	}()
	src := a.source(f.Ref, f.Size, pageDeadline)
	defer src.Close()
	data, rerr := src.ReadAll(fontMaxBytes)
	if rerr != nil {
		ans.Problem, ans.Detail = problemOf(rerr), rerr.Error()
		return ans, nil
	}
	sf, format, rerr := fonts.ToSFNT(data, sfntMaxBytes)
	if rerr != nil {
		ans.Problem, ans.Detail = problemOf(rerr), rerr.Error()
		return ans, nil
	}
	info := fonts.ReadInfo(sf, format)
	ans.Info = &info
	return ans, nil
}

// problemOf names what went wrong, for the page.
func problemOf(err error) string {
	var nf *design.NotFormatError
	var he *pluginkit.HostError
	switch {
	case errors.As(err, &nf) && nf.Reason == "cython":
		return "cython"
	case errors.As(err, &nf):
		return "not_this_format"
	case errors.Is(err, fonts.ErrNotAFont):
		return "not_a_font"
	case errors.Is(err, design.ErrNoPreview):
		return "no_preview"
	case errors.Is(err, design.ErrEncrypted):
		return "encrypted"
	case errors.Is(err, reader.ErrTooLarge) || errors.Is(err, picture.ErrRefused):
		return "too_large"
	case errors.Is(err, reader.ErrTimeout):
		return "timeout"
	case errors.As(err, &he):
		return "unavailable"
	}
	return "damaged"
}
