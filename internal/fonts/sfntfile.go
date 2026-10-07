package fonts

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
)

// The table directory of an SFNT font (TrueType or OpenType), read and
// written. Every offset a file states is checked against its bytes before it
// is followed: a table that runs past the end is left out, never read past.

// maxTables: more tables than any font has (a font has a few dozen).
const maxTables = 1024

// tablesOf is the tables of a TrueType or OpenType font, each a slice of b;
// of a collection (ttcf), the first font's.
func tablesOf(b []byte) (map[string][]byte, error) {
	at := 0
	if len(b) >= 16 && string(b[:4]) == "ttcf" {
		if binary.BigEndian.Uint32(b[8:12]) == 0 {
			return nil, errors.New("font: an empty collection")
		}
		at = int(binary.BigEndian.Uint32(b[12:16]))
	}
	if at < 0 || at+12 > len(b) {
		return nil, errors.New("font: the table directory is outside the file")
	}
	n := int(binary.BigEndian.Uint16(b[at+4:]))
	if n == 0 || n > maxTables {
		return nil, fmt.Errorf("font: %d tables", n)
	}
	out := make(map[string][]byte, n)
	for i := 0; i < n; i++ {
		rec := at + 12 + i*16
		if rec+16 > len(b) {
			break
		}
		tag := string(b[rec : rec+4])
		off := int64(binary.BigEndian.Uint32(b[rec+8:]))
		length := int64(binary.BigEndian.Uint32(b[rec+12:]))
		if off+length > int64(len(b)) {
			continue
		}
		out[tag] = b[off : off+length]
	}
	if len(out) == 0 {
		return nil, errors.New("font: no table inside the file")
	}
	return out, nil
}

// buildSFNT writes a TrueType or OpenType font of flavor (its sfntVersion:
// 0x00010000, "OTTO", "true") from its tables: the directory sorted by tag,
// each table padded to four bytes, the checksums and head's
// checkSumAdjustment computed.
func buildSFNT(flavor uint32, tables map[string][]byte) ([]byte, error) {
	tags := make([]string, 0, len(tables))
	total := 12
	for tag, data := range tables {
		if len(tag) != 4 {
			return nil, fmt.Errorf("font: a table tag %q", tag)
		}
		tags = append(tags, tag)
		total += 16 + (len(data)+3)&^3
	}
	if len(tags) == 0 || len(tags) > maxTables {
		return nil, fmt.Errorf("font: %d tables", len(tags))
	}
	sort.Strings(tags)
	n := len(tags)
	searchRange, entrySelector := 1, 0
	for searchRange*2 <= n {
		searchRange *= 2
		entrySelector++
	}
	searchRange *= 16

	out := make([]byte, 0, total)
	be := binary.BigEndian
	out = be.AppendUint32(out, flavor)
	out = be.AppendUint16(out, uint16(n))
	out = be.AppendUint16(out, uint16(searchRange))
	out = be.AppendUint16(out, uint16(entrySelector))
	out = be.AppendUint16(out, uint16(n*16-searchRange))

	offset := 12 + 16*n
	headAt := -1
	for _, tag := range tags {
		data := tables[tag]
		if tag == "head" && len(data) >= 12 {
			// checkSumAdjustment is zero while the checksums are summed.
			data = append([]byte(nil), data...)
			be.PutUint32(data[8:], 0)
			tables[tag] = data
			headAt = offset
		}
		out = append(out, tag...)
		out = be.AppendUint32(out, checksum(data))
		out = be.AppendUint32(out, uint32(offset))
		out = be.AppendUint32(out, uint32(len(data)))
		offset += (len(data) + 3) &^ 3
	}
	for _, tag := range tags {
		data := tables[tag]
		out = append(out, data...)
		for pad := (4 - len(data)%4) % 4; pad > 0; pad-- {
			out = append(out, 0)
		}
	}
	if headAt >= 0 {
		be.PutUint32(out[headAt+8:], 0xB1B0AFBA-checksum(out))
	}
	return out, nil
}

// checksum is the sum of b's big-endian 32-bit words, the last one padded
// with zeros.
func checksum(b []byte) uint32 {
	var sum uint32
	for i := 0; i < len(b); i += 4 {
		var w [4]byte
		copy(w[:], b[i:])
		sum += binary.BigEndian.Uint32(w[:])
	}
	return sum
}
