package design

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/brf-tech/filex-media-thumbs/internal/reader"
)

// Pixelmator Pro (.pxd). Since Pixelmator Pro 2.4 a document is one file, a
// ZIP: metadata.info (SQLite), data/ (the pixel tiles) and QuickLook/, where
// the program keeps a preview for file browsers - Thumbnail.webp (up to 1024
// pixels) and Icon.webp (64 pixels); TIFF before 2.4, when the document was a
// folder. Measured on real files (2023 and 2026): the members are stored,
// the WebP pictures carry alpha.
//
// The ZIP's directory is at its end, and filex hands an app a stream: the
// file is read through once, its last zipTail bytes kept, and the directory
// read from them; then the one preview member is read - from the kept tail
// when it is there, else by reading the file again up to it. Nothing is
// unpacked but that member.
//
// ⚠ `.pxd` is also the extension of Cython's declaration files, which are
// text. A .pxd that does not open with a ZIP's local header is not
// Pixelmator's, and is refused (NotFormatError, reason "cython" when it
// reads as text) - filex then asks the next handler, or opens it in its own
// viewer.

// pixelmatorPreviews are the members a Pixelmator Pro file's preview may be,
// by prefix, best first.
var pixelmatorPreviews = []string{"QuickLook/Thumbnail.", "QuickLook/Preview.", "QuickLook/Icon."}

const (
	// zipEOCD: the end of central directory record's fixed part.
	zipEOCD = 22
	// zipDirectoryMax: the largest central directory read.
	zipDirectoryMax = 4 << 20
	// zipTail: what is kept of the file's end - the end record with the
	// longest comment, and a directory as large as zipDirectoryMax.
	zipTail = zipEOCD + 0xFFFF + zipDirectoryMax
	// zipMembersMax: the most directory entries looked at.
	zipMembersMax = 100_000
)

// pixelmatorPreviewMax: the most the preview member may weigh, packed or not.
// A variable so a test can lower it.
var pixelmatorPreviewMax int64 = 16 << 20

// zipEntry is one member of a ZIP's central directory.
type zipEntry struct {
	name       string
	flags      uint16
	method     uint16
	compressed int64
	size       int64
	local      int64
}

// readPixelmator reads a Pixelmator Pro file's QuickLook preview.
func readPixelmator(src *reader.Source) (*Result, error) {
	head, err := src.ReadAt(0, 4, "pixelmator")
	if err != nil {
		if errors.Is(err, reader.ErrEndsEarly) {
			return nil, &NotFormatError{Format: "Pixelmator Pro"}
		}
		return nil, err
	}
	if !bytes.Equal(head, []byte("PK\x03\x04")) {
		return nil, notPixelmator(src, head)
	}
	size := src.Size()
	if size <= 0 {
		return nil, errors.New("pixelmator: the file's length is not known")
	}
	tailLen := min(size, int64(zipTail))
	tailAt := size - tailLen
	tail, err := src.ReadAt(tailAt, int(tailLen), "pixelmator")
	if err != nil {
		return nil, err
	}
	entries, err := zipDirectory(tail, tailAt, size)
	if err != nil {
		return nil, err
	}
	res := &Result{Kind: KindPixelmator, Facts: Facts{"format": "pixelmator", "members": len(entries)}}
	for _, prefix := range pixelmatorPreviews {
		for _, e := range entries {
			name := strings.TrimPrefix(e.name, "./")
			if !strings.HasPrefix(name, prefix) {
				continue
			}
			if e.flags&0x1 != 0 {
				return res, ErrEncrypted
			}
			if e.method != 0 && e.method != 8 {
				continue
			}
			if e.size > pixelmatorPreviewMax || e.compressed > pixelmatorPreviewMax {
				return res, reader.ErrTooLarge
			}
			b, err := zipMember(src, tail, tailAt, e)
			if err != nil {
				return res, err
			}
			pic, err := raw(b, "the Pixelmator Pro preview", name)
			if err != nil {
				return res, err
			}
			res.Facts["preview_member"] = name
			res.Picture = pic
			return res, nil
		}
	}
	return res, ErrNoPreview
}

// notPixelmator says why a .pxd that is not a ZIP is refused: Cython's text,
// or something else.
func notPixelmator(src *reader.Source, head []byte) error {
	sample := head
	if more, err := src.ReadAt(0, int(min(max(src.Size(), 4), 512)), "pixelmator"); err == nil {
		sample = more
	}
	if looksLikeText(sample) {
		return &NotFormatError{Format: "Pixelmator Pro", Reason: "cython"}
	}
	return &NotFormatError{Format: "Pixelmator Pro"}
}

// looksLikeText: valid UTF-8 (a cut last character allowed) with no NUL and
// few control characters.
func looksLikeText(b []byte) bool {
	// A sample cut inside a character: drop the cut bytes (at most three).
	for i := 0; i < utf8.UTFMax-1 && len(b) > 0 && !utf8.Valid(b); i++ {
		b = b[:len(b)-1]
	}
	if len(b) == 0 || !utf8.Valid(b) {
		return false
	}
	control := 0
	for _, c := range b {
		switch {
		case c == 0:
			return false
		case c < 0x20 && c != '\n' && c != '\r' && c != '\t' && c != '\f':
			control++
		}
	}
	return control*20 < len(b)
}

// zipDirectory reads a ZIP's central directory from tail, the file's last
// bytes, which start at tailAt of a file of size bytes.
func zipDirectory(tail []byte, tailAt, size int64) ([]zipEntry, error) {
	eocd := -1
	for i := len(tail) - zipEOCD; i >= 0; i-- {
		if tail[i] == 'P' && tail[i+1] == 'K' && tail[i+2] == 5 && tail[i+3] == 6 {
			eocd = i
			break
		}
	}
	if eocd < 0 {
		return nil, errors.New("pixelmator: no ZIP directory at the end of the file")
	}
	le := binary.LittleEndian
	count := int64(le.Uint16(tail[eocd+10:]))
	dirSize := int64(le.Uint32(tail[eocd+12:]))
	dirAt := int64(le.Uint32(tail[eocd+16:]))
	// A ZIP64 archive (over 4 GB, or over 65 535 members): no document's
	// preview needs one.
	if count == 0xFFFF || dirSize == 0xFFFFFFFF || dirAt == 0xFFFFFFFF {
		return nil, fmt.Errorf("pixelmator: a ZIP64 archive: %w", reader.ErrTooLarge)
	}
	if dirSize > zipDirectoryMax {
		return nil, reader.ErrTooLarge
	}
	if dirAt < tailAt || dirAt+dirSize > size || dirAt+dirSize > tailAt+int64(len(tail)) {
		return nil, errors.New("pixelmator: the ZIP directory is outside the file")
	}
	dir := tail[dirAt-tailAt : dirAt-tailAt+dirSize]
	var out []zipEntry
	for at := 0; at+46 <= len(dir) && int64(len(out)) < count && len(out) < zipMembersMax; {
		if le.Uint32(dir[at:]) != 0x02014b50 {
			break
		}
		nameLen := int(le.Uint16(dir[at+28:]))
		next := at + 46 + nameLen + int(le.Uint16(dir[at+30:])) + int(le.Uint16(dir[at+32:]))
		if next > len(dir) {
			break
		}
		out = append(out, zipEntry{
			name:       string(dir[at+46 : at+46+nameLen]),
			flags:      le.Uint16(dir[at+8:]),
			method:     le.Uint16(dir[at+10:]),
			compressed: int64(le.Uint32(dir[at+20:])),
			size:       int64(le.Uint32(dir[at+24:])),
			local:      int64(le.Uint32(dir[at+42:])),
		})
		at = next
	}
	return out, nil
}

// zipMember is a member's bytes: stored, or deflated (inflated here, at most
// pixelmatorPreviewMax).
func zipMember(src *reader.Source, tail []byte, tailAt int64, e zipEntry) ([]byte, error) {
	at := func(off int64, n int) ([]byte, error) {
		if off >= tailAt && off+int64(n) <= tailAt+int64(len(tail)) {
			return tail[off-tailAt : off-tailAt+int64(n)], nil
		}
		return src.ReadAt(off, n, "pixelmator")
	}
	local, err := at(e.local, 30)
	if err != nil {
		return nil, err
	}
	if binary.LittleEndian.Uint32(local) != 0x04034b50 {
		return nil, errors.New("pixelmator: a ZIP member without its local header")
	}
	dataAt := e.local + 30 + int64(binary.LittleEndian.Uint16(local[26:])) + int64(binary.LittleEndian.Uint16(local[28:]))
	data, err := at(dataAt, int(e.compressed))
	if err != nil {
		return nil, err
	}
	if e.method == 0 {
		return data, nil
	}
	b, err := io.ReadAll(io.LimitReader(flate.NewReader(bytes.NewReader(data)), pixelmatorPreviewMax+1))
	if err != nil {
		return nil, fmt.Errorf("pixelmator: the preview does not inflate: %w", err)
	}
	if int64(len(b)) > pixelmatorPreviewMax {
		return nil, reader.ErrTooLarge
	}
	return b, nil
}
