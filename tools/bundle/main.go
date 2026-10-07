// Command bundle builds dist/ui.zip, the preview page filex serves for this
// app, from ui/ - reproducibly, byte for byte, on any machine.
//
//	go run ./tools/bundle            -> dist/ui.zip + dist/ui.zip.sha256
//
// filex-app.json pins the bundle's sha256 and filex refuses a bundle that
// hashes to anything else, so the bundle a release builds at its tag must be
// the very one the manifest was stamped with. What would make two builds of
// the same tree differ is decided here instead:
//
//   - the files go in sorted by path, each stored (not compressed: another
//     machine's deflate may choose other bytes), with one fixed date and the
//     same attributes;
//   - text is written with LF line ends whatever the checkout made of it (a
//     Windows checkout may hold CRLF);
//   - only what the page needs goes in: ui/test/ and dot-files stay out, and
//     a file of a kind filex would not serve stops the build.
//
// The repository's LICENSE goes in at the top of the bundle.
package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

const (
	srcDir = "ui"
	outDir = "dist"
	// 1980-01-01 00:00:00 in MS-DOS date and time: the earliest a zip can say.
	dosDate = 1<<5 | 1
	dosTime = 0
)

// served are the kinds of file the page is made of, all of them kinds filex
// serves from a bundle ("" is a file with no extension, like LICENSE). The
// value is whether the file is text, whose line ends are made LF.
var served = map[string]bool{
	"": true, ".html": true, ".js": true, ".css": true, ".json": true, ".svg": true, ".txt": true, ".md": true,
	".png": false, ".woff2": false,
}

// skipped are paths under ui/ that are not part of the page.
func skipped(rel string) bool {
	if rel == "test" || strings.HasPrefix(rel, "test/") {
		return true
	}
	for _, seg := range strings.Split(rel, "/") {
		if strings.HasPrefix(seg, ".") {
			return true
		}
	}
	return false
}

type entry struct {
	name string
	data []byte
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "bundle:", err)
		os.Exit(1)
	}
}

func run() error {
	var entries []entry
	err := filepath.WalkDir(srcDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(strings.TrimPrefix(strings.TrimPrefix(p, srcDir), string(filepath.Separator)))
		if rel == "" {
			return nil
		}
		if skipped(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s is not a regular file (a link would not be served)", p)
		}
		ext := strings.ToLower(path.Ext(rel))
		text, ok := served[ext]
		if !ok {
			return fmt.Errorf("%s: filex does not serve %q files from an interface bundle", p, ext)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if text {
			b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
		}
		entries = append(entries, entry{name: rel, data: b})
		return nil
	})
	if err != nil {
		return err
	}
	license, err := os.ReadFile("LICENSE")
	if err != nil {
		return err
	}
	entries = append(entries, entry{name: "LICENSE", data: bytes.ReplaceAll(license, []byte("\r\n"), []byte("\n"))})
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
	for i := 1; i < len(entries); i++ {
		if strings.EqualFold(entries[i-1].name, entries[i].name) {
			return fmt.Errorf("%s and %s differ only in case; filex refuses such a bundle", entries[i-1].name, entries[i].name)
		}
	}
	if !hasIndex(entries) {
		return fmt.Errorf("%s/index.html is missing: the manifest's views open it", srcDir)
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		fh := &zip.FileHeader{
			Name:               e.name,
			Method:             zip.Store,
			ModifiedDate:       dosDate,
			ModifiedTime:       dosTime,
			CRC32:              crc32.ChecksumIEEE(e.data),
			CompressedSize64:   uint64(len(e.data)),
			UncompressedSize64: uint64(len(e.data)),
		}
		fh.SetMode(0o644)
		w, err := zw.CreateRaw(fh)
		if err != nil {
			return err
		}
		if _, err := w.Write(e.data); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	sum := sha256.Sum256(buf.Bytes())
	hexSum := hex.EncodeToString(sum[:])
	if err := os.WriteFile(filepath.Join(outDir, "ui.zip"), buf.Bytes(), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "ui.zip.sha256"), []byte(hexSum+"  ui.zip\n"), 0o644); err != nil {
		return err
	}
	fmt.Printf("dist/ui.zip  %d files, %d bytes, sha256 %s\n", len(entries), buf.Len(), hexSum)
	return nil
}

func hasIndex(entries []entry) bool {
	for _, e := range entries {
		if e.name == "index.html" {
			return true
		}
	}
	return false
}
