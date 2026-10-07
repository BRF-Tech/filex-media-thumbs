package fonts

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/brf-tech/filex-media-thumbs/internal/reader"
)

// WOFF 1.0 (W3C Recommendation, 2012): an SFNT font whose tables are stored
// one by one, each compressed with zlib or not. The header is 44 bytes, the
// directory 20 bytes a table:
//
//	tag, offset, compLength, origLength, origChecksum
//
// A table whose compLength equals its origLength is stored as it is; a
// shorter one is zlib data that must inflate to exactly origLength. Extended
// metadata and private data are not read.

// DecodeWOFF turns a WOFF file into the SFNT font it holds, at most maxOut
// bytes of it.
func DecodeWOFF(b []byte, maxOut int) ([]byte, error) {
	if len(b) < 44 || string(b[:4]) != "wOFF" {
		return nil, errors.New("woff: not a WOFF file")
	}
	be := binary.BigEndian
	flavor := be.Uint32(b[4:])
	if length := int64(be.Uint32(b[8:])); length > int64(len(b)) {
		return nil, fmt.Errorf("woff: %w", reader.ErrEndsEarly)
	}
	n := int(be.Uint16(b[12:]))
	if n == 0 || n > maxTables {
		return nil, fmt.Errorf("woff: %d tables", n)
	}
	if total := int64(be.Uint32(b[16:])); total > int64(maxOut) {
		return nil, reader.ErrTooLarge
	}
	if 44+20*n > len(b) {
		return nil, fmt.Errorf("woff: the table directory %w", reader.ErrEndsEarly)
	}
	tables := make(map[string][]byte, n)
	sum := int64(0)
	for i := 0; i < n; i++ {
		rec := b[44+20*i:]
		tag := string(rec[:4])
		off := int64(be.Uint32(rec[4:]))
		comp := int64(be.Uint32(rec[8:]))
		orig := int64(be.Uint32(rec[12:]))
		if off+comp > int64(len(b)) {
			return nil, fmt.Errorf("woff: the %q table is outside the file", tag)
		}
		if comp > orig {
			return nil, fmt.Errorf("woff: the %q table is larger packed than unpacked", tag)
		}
		sum += orig
		if sum > int64(maxOut) {
			return nil, reader.ErrTooLarge
		}
		if _, dup := tables[tag]; dup {
			return nil, fmt.Errorf("woff: the %q table twice", tag)
		}
		data := b[off : off+comp]
		if comp < orig {
			var err error
			if data, err = inflateExactly(data, orig); err != nil {
				return nil, fmt.Errorf("woff: the %q table: %w", tag, err)
			}
		}
		tables[tag] = data
	}
	return buildSFNT(flavor, tables)
}

// inflateExactly inflates zlib data that must come to exactly n bytes.
func inflateExactly(data []byte, n int64) ([]byte, error) {
	zr, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	out := make([]byte, n)
	if _, err := io.ReadFull(zr, out); err != nil {
		return nil, fmt.Errorf("it inflates to less than its %d bytes", n)
	}
	var one [1]byte
	if k, _ := zr.Read(one[:]); k > 0 {
		return nil, fmt.Errorf("it inflates to more than its %d bytes", n)
	}
	return out, nil
}
