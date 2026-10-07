package design

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"

	"github.com/brf-tech/filex-media-thumbs/internal/reader"
)

// Affinity (Affinity 3's .af, and the .afphoto, .afdesign and .afpub of
// Affinity 1 and 2: one container). The format is not published, and its
// streams are zstd-compressed; this reads none of them. The program saves a
// flattened PNG of the document beside them, uncompressed, for file browsers
// (Windows Explorer, XnView), and the container's header says where:
//
//	0x00  00 FF 4B 41    every Affinity file
//	0x04  u32 LE         the container's version in its low 16 bits (11:
//	                     Affinity 2, 12: Affinity 3); the high bits are flags
//	0x08  "nsrP"
//	0x0C  "#Inf"
//	0x10  u64 LE         the newest stream table (#FT4)
//	0x18  u64 LE         the preview block, T:
//
//	T+0   FF FF FF FF
//	T+4   "Thmb"
//	T+8   u32 LE 1, the block's length (the PNG's + 13), 29, 0
//	T+24  u32 LE         the PNG's length
//	T+28  1
//	T+29  the PNG: 512 pixels on its long side
//
// Measured on documents saved by Affinity 3.2.3 and Affinity Photo 2.6.5
// (the MIT-licensed test files of the Patchy project), an incrementally
// saved one included, with two stream tables: the header still names the
// current preview while an original JPEG of a placed image sits elsewhere in
// the file - so taking the first picture found would be wrong. A file whose
// header does not lead to a PNG (an older container) is searched for one
// instead, in its first affinityScanBudget bytes.

// affinityMagic opens every Affinity file.
var affinityMagic = []byte{0x00, 0xFF, 0x4B, 0x41}

// pngSignature opens every PNG.
var pngSignature = []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}

// affinityScanBudget: how far into a file a PNG is searched for when the
// header does not lead to one. affinityPreviewMax: the most the preview may
// weigh. Variables so a test can lower them.
var (
	affinityScanBudget int64 = 64 << 20
	affinityPreviewMax int64 = 16 << 20
)

// readAffinity reads an Affinity file's preview.
func readAffinity(src *reader.Source, ext string) (*Result, error) {
	head, err := src.ReadAt(0, 40, "affinity")
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(head[:4], affinityMagic) {
		return nil, &NotFormatError{Format: "Affinity"}
	}
	res := &Result{Kind: KindAffinity, Facts: affinityFacts(head, ext)}
	png, err := affinityPNG(src, head)
	if err != nil {
		return res, err
	}
	pic, err := raw(png, "the Affinity preview", "preview")
	if err != nil {
		return res, err
	}
	res.Picture = pic
	return res, nil
}

// affinityFacts: the container's version, and the generation it names.
func affinityFacts(head []byte, ext string) Facts {
	version := int(binary.LittleEndian.Uint32(head[4:8]) & 0xFFFF)
	f := Facts{"format": "affinity", "container": version}
	switch version {
	case 11:
		f["generation"] = "2"
	case 12:
		f["generation"] = "3"
	}
	if app := strings.TrimPrefix(strings.ToLower(ext), "."); app != "" {
		f["document"] = app
	}
	return f
}

// affinityPNG is the PNG an Affinity file carries: where the header points,
// else the first whole PNG in its first affinityScanBudget bytes.
func affinityPNG(src *reader.Source, head []byte) ([]byte, error) {
	if string(head[12:16]) == "#Inf" {
		png, err := affinityPreviewAt(src, int64(binary.LittleEndian.Uint64(head[24:32])))
		if err == nil || errors.Is(err, reader.ErrTooLarge) || errors.Is(err, reader.ErrTimeout) {
			return png, err
		}
	}
	return scanForPNG(src, 40, affinityScanBudget)
}

// affinityPreviewAt reads the preview block the header names.
func affinityPreviewAt(src *reader.Source, at int64) ([]byte, error) {
	if at < 40 || at > 1<<62 || (src.Size() > 0 && at+29 > src.Size()) {
		return nil, errors.New("affinity: the header's preview is outside the file")
	}
	blk, err := src.ReadAt(at, 29, "affinity")
	if err != nil {
		return nil, err
	}
	if string(blk[4:8]) != "Thmb" {
		return nil, errors.New("affinity: no preview block where the header points")
	}
	n := int64(binary.LittleEndian.Uint32(blk[24:28]))
	if n > affinityPreviewMax {
		return nil, reader.ErrTooLarge
	}
	if n < int64(len(pngSignature))+25 {
		return nil, errors.New("affinity: the preview block is empty")
	}
	png, err := src.ReadN(int(n), "affinity")
	if err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(png, pngSignature) {
		return nil, errors.New("affinity: the preview block holds no PNG")
	}
	return png, nil
}

// scanForPNG finds the first whole PNG at or after from, in budget bytes:
// its signature, an IHDR first, and chunks up to its IEND.
func scanForPNG(src *reader.Source, from, budget int64) ([]byte, error) {
	const window = 64 << 10
	chunk := make([]byte, window)
	keep := int64(len(pngSignature) - 1)
	var carry []byte
	base := from // the position of carry's first byte
	src.SeekTo(from)
	for base-from < budget {
		n, err := io.ReadFull(src, chunk)
		if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, err
		}
		data := append(carry, chunk[:n]...)
		if i := bytes.Index(data, pngSignature); i >= 0 {
			at := base + int64(i)
			png, werr := walkPNG(src, at)
			if werr == nil || errors.Is(werr, reader.ErrTooLarge) || errors.Is(werr, reader.ErrTimeout) {
				return png, werr
			}
			// Not a PNG after all: go on after its signature.
			base, carry = at+int64(len(pngSignature)), nil
			src.SeekTo(base)
			continue
		}
		if n == 0 || err != nil {
			break
		}
		cut := int64(len(data)) - keep
		if cut < 0 {
			cut = 0
		}
		carry = append([]byte(nil), data[cut:]...)
		base += cut
	}
	return nil, ErrNoPreview
}

// walkPNG reads the PNG at at chunk by chunk, at most affinityPreviewMax.
func walkPNG(src *reader.Source, at int64) ([]byte, error) {
	sig, err := src.ReadAt(at, len(pngSignature), "affinity")
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	out.Write(sig)
	for first := true; ; first = false {
		hdr, err := src.ReadN(8, "affinity")
		if err != nil {
			return nil, err
		}
		n := int64(binary.BigEndian.Uint32(hdr[0:4]))
		kind := string(hdr[4:8])
		if first && kind != "IHDR" {
			return nil, errors.New("affinity: a PNG signature without a PNG")
		}
		if int64(out.Len())+12+n > affinityPreviewMax {
			return nil, reader.ErrTooLarge
		}
		body, err := src.ReadN(int(n)+4, "affinity")
		if err != nil {
			return nil, err
		}
		out.Write(hdr)
		out.Write(body)
		if kind == "IEND" {
			return out.Bytes(), nil
		}
	}
}
