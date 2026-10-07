package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The release stamps both hashes into this repository's filex-app.json, whose
// addresses carry `{tag}`: each pattern must find its one place there.
func TestEachHashHasOnePlaceInTheManifest(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", manifestFile))
	if err != nil {
		t.Fatal(err)
	}
	for name, field := range map[string]*regexp.Regexp{"ui.bundle.sha256": uiSHA, "wasm.sha256": wasmSHA} {
		if n := len(field.FindAllIndex(src, -1)); n != 1 {
			t.Errorf("%s: %d places in %s, want 1", name, n, manifestFile)
		}
	}
}

// Stamping one hash leaves the other where it was and the addresses whole.
func TestStampingOneHashLeavesTheOther(t *testing.T) {
	const doc = `{
  "ui": {
    "bundle": {
      "url": "https://example.com/releases/download/{tag}/ui.zip",
      "sha256": "0000000000000000000000000000000000000000000000000000000000000000"
    }
  },
  "wasm": {
    "url": "https://example.com/releases/download/{tag}/plugin.wasm",
    "sha256": "1111111111111111111111111111111111111111111111111111111111111111"
  }
}
`
	ui := strings.Repeat("a", 64)
	out := string(uiSHA.ReplaceAll([]byte(doc), []byte("${1}"+ui+"${3}")))
	want := strings.Replace(doc, strings.Repeat("0", 64), ui, 1)
	if out != want {
		t.Fatalf("stamp-ui wrote\n%s\nwant\n%s", out, want)
	}
	wasm := strings.Repeat("b", 64)
	out = string(wasmSHA.ReplaceAll([]byte(out), []byte("${1}"+wasm+"${3}")))
	want = strings.Replace(want, strings.Repeat("1", 64), wasm, 1)
	if out != want {
		t.Fatalf("stamp-wasm wrote\n%s\nwant\n%s", out, want)
	}
}
