package fonts

import (
	"encoding/binary"
	"math"
	"sort"
	"strings"
	"unicode/utf16"

	"golang.org/x/text/encoding/charmap"
)

// What a font says about itself, for the preview: its names (`name`), its
// weight, width and italic (`OS/2`, else `head`), its number of glyphs
// (`maxp`), its variable axes and named instances (`fvar`), and the
// characters it maps (`cmap`, the best subtable: format 12, else 4). A table
// that does not hold together leaves its fields empty; nothing is read past
// a table's end.

// MaxCodepoints is the most characters listed: a CJK font maps tens of
// thousands, and the list crosses to the browser as JSON.
const MaxCodepoints = 20_000

// maxCodepointWalk: the most mappings looked at in a cmap. A hostile format
// 12 table can promise overlapping groups of a million characters each.
const maxCodepointWalk = 2_000_000

// Axis is a variable axis (`wght`, `wdth`, ...): its range and default, and
// its name as the font gives it.
type Axis struct {
	Tag     string  `json:"tag"`
	Min     float64 `json:"min"`
	Default float64 `json:"default"`
	Max     float64 `json:"max"`
	Name    string  `json:"name,omitempty"`
}

// Instance is a named instance of a variable font ("Bold Condensed"): its
// name and its place on each axis.
type Instance struct {
	Name   string             `json:"name"`
	Coords map[string]float64 `json:"coords"`
}

// Info is what the preview shows of a font besides the font itself.
type Info struct {
	Format    Format     `json:"format"`
	Outlines  Format     `json:"outlines"`
	Family    string     `json:"family,omitempty"`
	Style     string     `json:"style,omitempty"`
	FullName  string     `json:"full_name,omitempty"`
	Version   string     `json:"version,omitempty"`
	Designer  string     `json:"designer,omitempty"`
	Maker     string     `json:"maker,omitempty"`
	License   string     `json:"license,omitempty"`
	Copyright string     `json:"copyright,omitempty"`
	Weight    int        `json:"weight,omitempty"`
	Width     int        `json:"width,omitempty"`
	Italic    bool       `json:"italic,omitempty"`
	Glyphs    int        `json:"glyphs,omitempty"`
	Axes      []Axis     `json:"axes,omitempty"`
	Instances []Instance `json:"instances,omitempty"`
	// Codepoints are the characters the font maps, rising, at most
	// MaxCodepoints of them; CodepointsTotal how many it maps (counted up to
	// a limit). Codepoints is nil when the map was not read.
	Codepoints      []int `json:"codepoints"`
	CodepointsTotal int   `json:"codepoints_total"`
}

// ReadInfo reads what sfnt (a TrueType or OpenType font, or a collection:
// its first font) says about itself. format is what the file was.
func ReadInfo(sfnt []byte, format Format) Info {
	info := Info{Format: format, Outlines: flavorOf(sfnt)}
	tables, err := tablesOf(sfnt)
	if err != nil {
		return info
	}
	names := readNames(tables["name"])
	info.Family = firstOf(names, 16, 1)
	info.Style = firstOf(names, 17, 2)
	info.FullName = names[4]
	info.Version = names[5]
	info.Maker = names[8]
	info.Designer = names[9]
	info.License = names[13]
	info.Copyright = names[0]
	if t := tables["OS/2"]; len(t) >= 8 {
		info.Weight = int(binary.BigEndian.Uint16(t[4:]))
		info.Width = int(binary.BigEndian.Uint16(t[6:]))
		if len(t) >= 64 {
			info.Italic = binary.BigEndian.Uint16(t[62:])&1 == 1
		}
	} else if t := tables["head"]; len(t) >= 46 {
		info.Italic = binary.BigEndian.Uint16(t[44:])&2 == 2
	}
	if t := tables["maxp"]; len(t) >= 6 {
		info.Glyphs = int(binary.BigEndian.Uint16(t[4:]))
	}
	info.Axes, info.Instances = readFvar(tables["fvar"], names)
	info.Codepoints, info.CodepointsTotal = readCmap(tables["cmap"])
	return info
}

// firstOf is the first of the names with these ids that the font gives.
func firstOf(names map[int]string, ids ...int) string {
	for _, id := range ids {
		if s := names[id]; s != "" {
			return s
		}
	}
	return ""
}

// readNames reads a `name` table: the best record of each name id, Windows
// English first, then any Windows or Unicode one, then a Macintosh Roman
// one.
func readNames(t []byte) map[int]string {
	out := map[int]string{}
	if len(t) < 6 {
		return out
	}
	be := binary.BigEndian
	count := int(be.Uint16(t[2:]))
	strings0 := int(be.Uint16(t[4:]))
	rank := map[int]int{}
	for i := 0; i < count; i++ {
		rec := 6 + i*12
		if rec+12 > len(t) {
			break
		}
		platform, encoding, language := be.Uint16(t[rec:]), be.Uint16(t[rec+2:]), be.Uint16(t[rec+4:])
		id := int(be.Uint16(t[rec+6:]))
		length := int(be.Uint16(t[rec+8:]))
		at := strings0 + int(be.Uint16(t[rec+10:]))
		if at+length > len(t) {
			continue
		}
		raw := t[at : at+length]
		var r int
		var text string
		switch {
		case platform == 3 || platform == 0:
			r = 2
			if platform == 3 && language == 0x409 {
				r = 3
			}
			text = utf16BE(raw)
		case platform == 1 && encoding == 0:
			r = 1
			b, err := charmap.Macintosh.NewDecoder().Bytes(raw)
			if err != nil {
				continue
			}
			text = string(b)
		default:
			continue
		}
		if text = clean(text); text == "" {
			continue
		}
		if prev, ok := rank[id]; !ok || r > prev {
			rank[id], out[id] = r, text
		}
	}
	return out
}

func utf16BE(b []byte) string {
	u := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		u = append(u, binary.BigEndian.Uint16(b[i:]))
	}
	return string(utf16.Decode(u))
}

// maxName: the most characters of one name kept (a license is pages long;
// the preview shows its start).
const maxName = 300

// clean makes a name one line: line breaks and tabs become one space, other
// control characters go, and a name longer than maxName characters is cut
// with an ellipsis.
func clean(s string) string {
	var b strings.Builder
	n := 0
	space := false
	for _, r := range s {
		switch {
		case r == ' ' || r == '\n' || r == '\r' || r == '\t':
			if !space && n > 0 {
				b.WriteByte(' ')
				n++
			}
			space = true
			continue
		case r < 0x20 || r == 0x7F || r == 0xFFFD:
			continue
		}
		if n >= maxName {
			return strings.TrimSpace(b.String()) + "…"
		}
		space = false
		b.WriteRune(r)
		n++
	}
	return strings.TrimSpace(b.String())
}

// readFvar reads the variable axes and the named instances.
func readFvar(t []byte, names map[int]string) ([]Axis, []Instance) {
	if len(t) < 16 {
		return nil, nil
	}
	be := binary.BigEndian
	axesAt := int(be.Uint16(t[4:]))
	axisCount := int(be.Uint16(t[8:]))
	axisSize := int(be.Uint16(t[10:]))
	instanceCount := int(be.Uint16(t[12:]))
	instanceSize := int(be.Uint16(t[14:]))
	if axisSize < 20 || axisCount > 64 {
		return nil, nil
	}
	fixed := func(at int) float64 { return math.Round(float64(int32(be.Uint32(t[at:])))/65536*1000) / 1000 }
	var axes []Axis
	for i := 0; i < axisCount; i++ {
		rec := axesAt + i*axisSize
		if rec+20 > len(t) {
			return nil, nil
		}
		axes = append(axes, Axis{
			Tag:     string(t[rec : rec+4]),
			Min:     fixed(rec + 4),
			Default: fixed(rec + 8),
			Max:     fixed(rec + 12),
			Name:    names[int(be.Uint16(t[rec+18:]))],
		})
	}
	var instances []Instance
	if instanceSize >= 4+4*axisCount {
		base := axesAt + axisCount*axisSize
		for i := 0; i < instanceCount && i < 256; i++ {
			rec := base + i*instanceSize
			if rec+4+4*axisCount > len(t) {
				break
			}
			in := Instance{Name: names[int(be.Uint16(t[rec:]))], Coords: map[string]float64{}}
			for a, ax := range axes {
				in.Coords[ax.Tag] = fixed(rec + 4 + 4*a)
			}
			if in.Name != "" {
				instances = append(instances, in)
			}
		}
	}
	return axes, instances
}

// readCmap is the characters the best `cmap` subtable maps to a glyph,
// rising (at most MaxCodepoints), and how many it maps.
func readCmap(t []byte) ([]int, int) {
	if len(t) < 4 {
		return nil, 0
	}
	be := binary.BigEndian
	n := int(be.Uint16(t[2:]))
	best, bestRank := -1, 0
	for i := 0; i < n; i++ {
		rec := 4 + i*8
		if rec+8 > len(t) {
			break
		}
		platform, encoding := be.Uint16(t[rec:]), be.Uint16(t[rec+2:])
		off := int(be.Uint32(t[rec+4:]))
		if off < 0 || off+4 > len(t) {
			continue
		}
		format := be.Uint16(t[off:])
		rank := 0
		switch {
		case format == 12 && (platform == 3 || platform == 0):
			rank = 3
		case format == 4 && ((platform == 3 && encoding == 1) || platform == 0):
			rank = 2
		case format == 4 && platform == 3 && encoding == 0:
			rank = 1
		}
		if rank > bestRank {
			best, bestRank = off, rank
		}
	}
	if best < 0 {
		return nil, 0
	}
	seen := map[int]bool{}
	var out []int
	total, walked := 0, 0
	// visit is told every character a subtable covers, mapped or not, so a
	// table of empty ranges is cut off as surely as one of full ones.
	visit := func(c int, mapped bool) bool {
		walked++
		if walked > maxCodepointWalk {
			return false
		}
		if !mapped || seen[c] {
			return true
		}
		total++
		if len(out) < MaxCodepoints {
			seen[c] = true
			out = append(out, c)
		}
		return true
	}
	if be.Uint16(t[best:]) == 12 {
		cmap12(t, best, visit)
	} else {
		cmap4(t, best, visit)
	}
	sort.Ints(out)
	if out == nil {
		out = []int{}
	}
	return out, total
}

// cmap4 walks a format 4 subtable at at.
func cmap4(t []byte, at int, visit func(int, bool) bool) {
	be := binary.BigEndian
	if at+14 > len(t) {
		return
	}
	segX2 := int(be.Uint16(t[at+6:]))
	ends := at + 14
	starts := ends + segX2 + 2
	deltas := starts + segX2
	ranges := deltas + segX2
	if ranges+segX2 > len(t) {
		return
	}
	for s := 0; s+1 < segX2; s += 2 {
		end := int(be.Uint16(t[ends+s:]))
		start := int(be.Uint16(t[starts+s:]))
		delta := int(be.Uint16(t[deltas+s:]))
		rangeOffset := int(be.Uint16(t[ranges+s:]))
		if start == 0xFFFF {
			break
		}
		for c := start; c <= end; c++ {
			var glyph int
			if rangeOffset == 0 {
				glyph = (c + delta) & 0xFFFF
			} else {
				g := ranges + s + rangeOffset + 2*(c-start)
				if g+2 > len(t) {
					break
				}
				if glyph = int(be.Uint16(t[g:])); glyph != 0 {
					glyph = (glyph + delta) & 0xFFFF
				}
			}
			if !visit(c, glyph != 0) {
				return
			}
		}
	}
}

// cmap12 walks a format 12 subtable at at.
func cmap12(t []byte, at int, visit func(int, bool) bool) {
	be := binary.BigEndian
	if at+16 > len(t) {
		return
	}
	groups := int(be.Uint32(t[at+12:]))
	for i := 0; i < groups; i++ {
		g := at + 16 + i*12
		if g < 0 || g+12 > len(t) {
			return
		}
		start := int(be.Uint32(t[g:]))
		end := min(int(be.Uint32(t[g+4:])), 0x10FFFF)
		glyph := int(be.Uint32(t[g+8:]))
		for c := start; c <= end; c++ {
			if !visit(c, glyph+(c-start) != 0) {
				return
			}
		}
	}
}
