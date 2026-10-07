package design

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"

	"github.com/brf-tech/filex-media-thumbs/internal/picture"
	"github.com/brf-tech/filex-media-thumbs/internal/reader"
)

// Photoshop (.psd, and .psb, the large document format), from Adobe's
// Photoshop File Formats Specification. What is read, in file order - every
// step forward, so a stream is read once:
//
//	header (26 bytes)           8BPS, version 1 (PSD) or 2 (PSB), channels,
//	                            height, width, depth, colour mode
//	colour mode data            the palette of an indexed image, else skipped
//	image resources             8BIM blocks: 1036 (the thumbnail, a JPEG), 1033
//	                            (the same from Photoshop 4, blue first) and
//	                            1057 (version info: is the merged image real?)
//	layer and mask information  its length (4 bytes, 8 in a PSB), and the
//	                            number of layers at its start; then skipped
//	image data                  the merged image: raw or PackBits (RLE), one
//	                            channel after another
//
// The merged image is drawn first: Photoshop's thumbnail is at most 160
// pixels. Only the rows that land in the box asked for are unpacked, and
// only the pixels that land in it are kept, so the memory is the box's
// whatever the document's size. 8 and 16 bits a channel (the high byte of
// 16); grayscale, RGB, CMYK and indexed colour.
//
// The thumbnail resource is the answer when there is no merged image to
// draw: a file saved without "Maximize compatibility" (1057 says its merged
// image is not real), a colour mode or a depth not read here (Lab, duotone,
// multichannel, bitmap, 32 bits), a compression not read here (ZIP), a merged
// image past the reading limits, or a damaged one. It comes before the layers
// in the file, so it is in hand by then. A file with neither is ErrNoPreview.

// The resources read.
const (
	psdResThumbnail4 = 1033 // Photoshop 4.0: JPEG, blue first
	psdResThumbnail  = 1036 // Photoshop 5.0 and later: JPEG
	psdResVersion    = 1057 // version info: hasRealMergedData
)

// The colour modes, by the number the header carries.
const (
	psdModeBitmap       = 0
	psdModeGray         = 1
	psdModeIndexed      = 2
	psdModeRGB          = 3
	psdModeCMYK         = 4
	psdModeMultichannel = 7
	psdModeDuotone      = 8
	psdModeLab          = 9
)

// psdModeNames are the colour modes as the facts name them.
var psdModeNames = map[int]string{
	psdModeBitmap: "bitmap", psdModeGray: "grayscale", psdModeIndexed: "indexed", psdModeRGB: "rgb",
	psdModeCMYK: "cmyk", psdModeMultichannel: "multichannel", psdModeDuotone: "duotone", psdModeLab: "lab",
}

// psdCountsBudget: the RLE row lengths of the channels drawn (2 bytes a row
// in a PSD, 4 in a PSB). Past it the merged image is not read. A variable so
// a test can lower it.
var psdCountsBudget int64 = 16 << 20

// psdThumbnailMax: the most a thumbnail resource may weigh.
const psdThumbnailMax = 16 << 20

// psdFile is what the head of a Photoshop file says.
type psdFile struct {
	large                                bool // a PSB (version 2)
	channels, width, height, depth, mode int
	palette                              []byte // 768 bytes: an indexed image's colours
	thumbnail                            []byte // the JPEG of 1036, else of 1033
	thumbnailBGR                         bool   // it came from 1033
	// merged: 1057 says the merged image is real (1), is not (0), or the
	// file has no 1057 (-1, and it is drawn).
	merged int
	// layers: how many layers the file says it has; -1 when it was not read.
	layers int
	// dataAt is where the image data section starts.
	dataAt int64
}

func (f *psdFile) facts() Facts {
	out := Facts{
		"format":   "psd",
		"width":    f.width,
		"height":   f.height,
		"channels": f.channels,
		"depth":    f.depth,
	}
	if f.large {
		out["format"] = "psb"
	}
	if name, ok := psdModeNames[f.mode]; ok {
		out["mode"] = name
	}
	if f.layers >= 0 {
		out["layers"] = f.layers
	}
	switch f.merged {
	case 1:
		out["merged"] = "real"
	case 0:
		out["merged"] = "not_real"
	}
	return out
}

// readPSD reads a Photoshop file's picture: its merged image sampled into
// box x box, else its thumbnail resource.
func readPSD(src *reader.Source, box int) (*Result, error) {
	f, err := readPSDHead(src)
	if err != nil {
		return nil, err
	}
	res := &Result{Kind: KindPSD, Facts: f.facts()}
	var mergedErr error
	if f.mergedDrawable() {
		img, err := f.readMerged(src, box)
		if err == nil {
			b := img.Bounds()
			res.Picture = &Picture{Image: img, Width: b.Dx(), Height: b.Dy(), Source: "merged"}
			return res, nil
		}
		if errors.Is(err, reader.ErrTimeout) {
			return res, err
		}
		mergedErr = err
	}
	if f.thumbnail != nil {
		pic, err := f.thumbnailPicture()
		if err != nil {
			return res, err
		}
		res.Picture = pic
		return res, nil
	}
	if mergedErr != nil {
		return res, mergedErr
	}
	return res, ErrNoPreview
}

// readPSDHead reads the header, the colour mode data, the image resources
// and the number of layers, and notes where the image data starts.
func readPSDHead(src *reader.Source) (*psdFile, error) {
	h, err := src.ReadAt(0, 26, "psd")
	if err != nil {
		return nil, err
	}
	if string(h[0:4]) != "8BPS" {
		return nil, &NotFormatError{Format: "Photoshop"}
	}
	f := &psdFile{merged: -1, layers: -1}
	switch v := binary.BigEndian.Uint16(h[4:6]); v {
	case 1:
	case 2:
		f.large = true
	default:
		return nil, fmt.Errorf("psd: unknown version %d", v)
	}
	f.channels = int(binary.BigEndian.Uint16(h[12:14]))
	f.height = int(binary.BigEndian.Uint32(h[14:18]))
	f.width = int(binary.BigEndian.Uint32(h[18:22]))
	f.depth = int(binary.BigEndian.Uint16(h[22:24]))
	f.mode = int(binary.BigEndian.Uint16(h[24:26]))
	maxSide := 30_000
	if f.large {
		maxSide = 300_000
	}
	if f.channels < 1 || f.channels > 56 || f.width < 1 || f.height < 1 || f.width > maxSide || f.height > maxSide {
		return nil, fmt.Errorf("psd: a %dx%d image of %d channels is outside the format", f.width, f.height, f.channels)
	}

	// Colour mode data: only an indexed image's palette is kept.
	n, err := src.BE32("psd")
	if err != nil {
		return nil, err
	}
	at := src.Pos()
	if f.mode == psdModeIndexed && n >= 768 {
		if f.palette, err = src.ReadN(768, "psd"); err != nil {
			return nil, err
		}
	}
	src.SeekTo(at + n)

	// Image resources.
	n, err = src.BE32("psd")
	if err != nil {
		return nil, err
	}
	end := src.Pos() + n
	if err := f.readResources(src, end); err != nil {
		return nil, err
	}
	src.SeekTo(end)

	// Layer and mask information: its length, the number of layers at the
	// start of the layer info inside it, and past it the image data.
	width := 4
	if f.large {
		width = 8
	}
	layers, err := readLength(src, width)
	if err != nil {
		return nil, err
	}
	start := src.Pos()
	f.dataAt = start + layers
	if layers >= int64(width)+2 {
		info, err := readLength(src, width)
		if err != nil {
			return nil, err
		}
		if info >= 2 {
			b, err := src.ReadN(2, "psd")
			if err != nil {
				return nil, err
			}
			// Negative: the first alpha channel holds the merged image's
			// transparency. The count is its absolute value.
			c := int(int16(binary.BigEndian.Uint16(b)))
			if c < 0 {
				c = -c
			}
			f.layers = c
		} else {
			f.layers = 0
		}
	} else if layers == 0 {
		f.layers = 0
	}
	return f, nil
}

// readLength reads a big-endian length of width bytes (4, or 8 in a PSB).
func readLength(src *reader.Source, width int) (int64, error) {
	b, err := src.ReadN(width, "psd")
	if err != nil {
		return 0, err
	}
	if width == 4 {
		return int64(binary.BigEndian.Uint32(b)), nil
	}
	v := binary.BigEndian.Uint64(b)
	if v > 1<<62 {
		return 0, errors.New("psd: a section's length is outside the file")
	}
	return int64(v), nil
}

// readResources walks the image resource blocks up to end and keeps the
// thumbnail and the version info. A block that does not hold together ends
// the walk, not the file: what was found before it stands, and the merged
// image does not depend on it.
func (f *psdFile) readResources(src *reader.Source, end int64) error {
	for src.Pos()+12 <= end {
		start := src.Pos()
		b, err := src.ReadN(7, "psd")
		if err != nil {
			return err
		}
		switch string(b[0:4]) {
		case "8BIM", "MeSa", "AgHg", "PHUT", "DCSR":
		default:
			return nil
		}
		id := int(binary.BigEndian.Uint16(b[4:6]))
		// The name: a Pascal string padded to an even length, its length
		// byte included.
		nameLen := int64(b[6])
		src.SeekTo(start + 6 + ((1 + nameLen + 1) &^ 1))
		size, err := src.BE32("psd")
		if err != nil {
			return err
		}
		data := src.Pos()
		if size > end-data {
			return nil
		}
		switch {
		case (id == psdResThumbnail || (id == psdResThumbnail4 && f.thumbnail == nil)) && size > 28 && size-28 <= psdThumbnailMax:
			d, err := src.ReadN(int(size), "psd")
			if err != nil {
				return err
			}
			// Format 1 is kJpegRGB; 0 (raw) is not written by Photoshop.
			if binary.BigEndian.Uint32(d[0:4]) == 1 {
				f.thumbnail, f.thumbnailBGR = d[28:], id == psdResThumbnail4
			}
		case id == psdResVersion && size >= 5:
			d, err := src.ReadN(5, "psd")
			if err != nil {
				return err
			}
			f.merged = 0
			if d[4] != 0 {
				f.merged = 1
			}
		}
		src.SeekTo(data + size + size&1)
	}
	return nil
}

// mergedChannels is how many channels the colour mode draws from.
func (f *psdFile) mergedChannels() int {
	switch f.mode {
	case psdModeGray, psdModeIndexed:
		return 1
	case psdModeRGB:
		return 3
	case psdModeCMYK:
		return 4
	}
	return 0
}

// mergedDrawable: the merged image is real and of a kind read here.
func (f *psdFile) mergedDrawable() bool {
	need := f.mergedChannels()
	return f.merged != 0 && need > 0 && f.channels >= need &&
		(f.depth == 8 || f.depth == 16) &&
		(f.mode != psdModeIndexed || len(f.palette) == 768)
}

// readMerged draws the merged image into box x box: the sampled rows of each
// channel drawn, the sampled pixels of each row.
func (f *psdFile) readMerged(src *reader.Source, box int) (image.Image, error) {
	need := f.mergedChannels()
	tw, th := picture.Fit(f.width, f.height, box, box)
	rows := samplePositions(f.height, th)
	cols := samplePositions(f.width, tw)
	bps := f.depth / 8
	rowBytes := int64(f.width) * int64(bps)
	planes := make([][]byte, need)
	for c := range planes {
		planes[c] = make([]byte, tw*th)
	}
	row := make([]byte, rowBytes)
	keep := func(c, i int) {
		dst := planes[c][i*tw : (i+1)*tw]
		for j, x := range cols {
			dst[j] = row[x*bps]
		}
	}

	b, err := src.ReadAt(f.dataAt, 2, "psd")
	if err != nil {
		return nil, err
	}
	switch compression := binary.BigEndian.Uint16(b); compression {
	case 0:
		base := src.Pos()
		for c := 0; c < need; c++ {
			for i, y := range rows {
				src.SeekTo(base + (int64(c)*int64(f.height)+int64(y))*rowBytes)
				if err := src.Fill(row, "psd"); err != nil {
					return nil, err
				}
				keep(c, i)
			}
		}
	case 1:
		width := int64(4)
		if !f.large {
			width = 2
		}
		tableLen := int64(need) * int64(f.height) * width
		if tableLen > psdCountsBudget {
			return nil, reader.ErrTooLarge
		}
		table, err := src.ReadN(int(tableLen), "psd")
		if err != nil {
			return nil, err
		}
		// The row lengths of the channels not drawn come next; the rows
		// follow them.
		off := src.Pos() + int64(f.channels-need)*int64(f.height)*width
		// The longest a PackBits row can be: a header byte every 128 bytes.
		maxPacked := rowBytes + rowBytes/128 + 16
		packed := make([]byte, 0, 4096)
		for c := 0; c < need; c++ {
			i := 0
			for y := 0; y < f.height; y++ {
				k := int64(c*f.height + y)
				var n int64
				if f.large {
					n = int64(binary.BigEndian.Uint32(table[k*4:]))
				} else {
					n = int64(binary.BigEndian.Uint16(table[k*2:]))
				}
				if n > maxPacked {
					return nil, fmt.Errorf("psd: a row of %d packed bytes for %d pixels", n, f.width)
				}
				if i < len(rows) && rows[i] == y {
					if int64(cap(packed)) < n {
						packed = make([]byte, n)
					}
					packed = packed[:n]
					src.SeekTo(off)
					if err := src.Fill(packed, "psd"); err != nil {
						return nil, err
					}
					unpackBits(row, packed)
					keep(c, i)
					i++
				}
				off += n
			}
		}
	default:
		// ZIP (2, 3): not written by Photoshop for the merged image.
		return nil, fmt.Errorf("%w: merged image compression %d", ErrNoPreview, compression)
	}
	return f.compose(planes, tw, th), nil
}

// compose turns the sampled channels into a picture.
func (f *psdFile) compose(planes [][]byte, tw, th int) image.Image {
	if f.mode == psdModeGray {
		g := image.NewGray(image.Rect(0, 0, tw, th))
		copy(g.Pix, planes[0])
		return g
	}
	out := image.NewRGBA(image.Rect(0, 0, tw, th))
	for i := 0; i < tw*th; i++ {
		var r, g, b byte
		switch f.mode {
		case psdModeRGB:
			r, g, b = planes[0][i], planes[1][i], planes[2][i]
		case psdModeCMYK:
			// Stored inverted (255 is no ink), so the product of the channel
			// and the black is the light left.
			k := int(planes[3][i])
			r = byte(int(planes[0][i]) * k / 255)
			g = byte(int(planes[1][i]) * k / 255)
			b = byte(int(planes[2][i]) * k / 255)
		case psdModeIndexed:
			v := int(planes[0][i])
			r, g, b = f.palette[v], f.palette[256+v], f.palette[512+v]
		}
		out.Pix[i*4], out.Pix[i*4+1], out.Pix[i*4+2], out.Pix[i*4+3] = r, g, b, 0xFF
	}
	return out
}

// thumbnailPicture is the thumbnail resource: as it is from 1036, decoded
// with red and blue swapped back from 1033.
func (f *psdFile) thumbnailPicture() (*Picture, error) {
	if !f.thumbnailBGR {
		return raw(f.thumbnail, "the Photoshop thumbnail", "thumbnail")
	}
	img, _, err := picture.Decode(f.thumbnail, "the Photoshop thumbnail", picture.Embedded())
	if err != nil {
		if errors.Is(err, picture.ErrRefused) {
			return nil, fmt.Errorf("%w: %v", reader.ErrTooLarge, err)
		}
		return nil, err
	}
	b := img.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			r, g, bl, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			i := out.PixOffset(x, y)
			out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = byte(bl>>8), byte(g>>8), byte(r>>8), 0xFF
		}
	}
	return &Picture{Image: out, Width: b.Dx(), Height: b.Dy(), Source: "thumbnail"}, nil
}

// unpackBits expands one PackBits row into dst. A row that runs past dst is
// cut, one that ends short leaves the rest black: a preview, not a check of
// the file.
func unpackBits(dst, src []byte) {
	o := 0
	for i := 0; i < len(src) && o < len(dst); {
		n := int(int8(src[i]))
		i++
		switch {
		case n >= 0:
			k := n + 1
			if i+k > len(src) {
				k = len(src) - i
			}
			o += copy(dst[o:], src[i:i+k])
			i += k
		case n != -128:
			if i >= len(src) {
				break
			}
			v := src[i]
			i++
			for k := 1 - n; k > 0 && o < len(dst); k-- {
				dst[o] = v
				o++
			}
		}
	}
	clear(dst[o:])
}

// samplePositions are the n source positions (of size) a picture n wide
// takes its pixels from, nearest first, rising.
func samplePositions(size, n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = int(int64(i) * int64(size) / int64(n))
	}
	return out
}
