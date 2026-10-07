// Command manifest keeps filex-app.json, the interface bundle and the module
// in step.
//
//	go run ./tools/manifest embed                 filex-app.json -> manifest.embed.json (go generate runs it)
//	go run ./tools/manifest stamp-ui dist/ui.zip  writes the bundle's sha256 into ui.bundle.sha256
//	go run ./tools/manifest stamp-wasm dist/plugin.wasm
//	                                              writes the module's sha256 into wasm.sha256
//	go run ./tools/manifest check dist/ui.zip dist/plugin.wasm
//	                                              fails unless filex-app.json pins exactly these two
//	go run ./tools/manifest pins                  prints the two hashes filex-app.json pins
//
// Why the order matters (scripts/build.sh follows it): the module DESCRIBES
// the interface - filex refuses a module whose `ui` block names another
// bundle than the manifest - so the bundle's hash goes into filex-app.json
// first, manifest.embed.json is written from it, and only then is the module
// built. The module's own hash cannot be inside the module (writing it would
// change the module, which would change the hash), so manifest.embed.json is
// filex-app.json WITHOUT its `wasm` block, and stamping wasm.sha256 changes
// no byte the module embeds.
//
// Edit filex-app.json only; manifest.embed.json is never edited by hand.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"

	"github.com/brf-tech/filex/backend/pkg/pluginkit/wire"
)

const (
	manifestFile = "filex-app.json"
	embedFile    = "manifest.embed.json"
)

func main() {
	var err error
	switch {
	case len(os.Args) == 2 && os.Args[1] == "embed":
		err = embed()
	case len(os.Args) == 3 && os.Args[1] == "stamp-ui":
		err = stamp(uiSHA, os.Args[2])
	case len(os.Args) == 3 && os.Args[1] == "stamp-wasm":
		err = stamp(wasmSHA, os.Args[2])
	case len(os.Args) == 4 && os.Args[1] == "check":
		err = check(os.Args[2], os.Args[3])
	case len(os.Args) == 2 && os.Args[1] == "pins":
		err = pins()
	default:
		err = errors.New("usage: go run ./tools/manifest embed | stamp-ui <ui.zip> | stamp-wasm <plugin.wasm> | check <ui.zip> <plugin.wasm> | pins")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "manifest:", err)
		os.Exit(1)
	}
}

// embed writes manifest.embed.json: every top-level member of
// filex-app.json, in its order and with its own formatting, except `wasm`.
func embed() error {
	src, err := os.ReadFile(manifestFile)
	if err != nil {
		return err
	}
	if err := validate(src); err != nil {
		return err
	}
	out, err := withoutWasm(src)
	if err != nil {
		return err
	}
	return os.WriteFile(embedFile, out, 0o644)
}

// withoutWasm is src with its top-level `wasm` member left out; every other
// member keeps its bytes.
func withoutWasm(src []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(src))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, fmt.Errorf("%s is not a JSON object", manifestFile)
	}
	var out bytes.Buffer
	out.WriteString("{")
	first := true
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, _ := tok.(string)
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, fmt.Errorf("%s: %q: %w", manifestFile, key, err)
		}
		if key == "wasm" {
			continue
		}
		if !first {
			out.WriteString(",")
		}
		first = false
		k, _ := json.Marshal(key)
		fmt.Fprintf(&out, "\n  %s: %s", k, raw)
	}
	out.WriteString("\n}\n")
	return out.Bytes(), nil
}

// validate decodes the manifest the way filex does, refusing a key filex
// does not know.
func validate(src []byte) error {
	dec := json.NewDecoder(bytes.NewReader(src))
	dec.DisallowUnknownFields()
	var m wire.Manifest
	if err := dec.Decode(&m); err != nil {
		return fmt.Errorf("%s: %w", manifestFile, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%s: trailing data after the manifest", manifestFile)
	}
	if m.Name == "" || m.Version == "" {
		return fmt.Errorf("%s: name and version are required", manifestFile)
	}
	if m.UI == nil || m.Wasm == nil {
		return fmt.Errorf("%s: this app has a module and an interface: both `wasm` and `ui` are required", manifestFile)
	}
	return nil
}

// The two hashes, each found inside its own block. Neither block holds a
// nested object, but both addresses carry the `{tag}` placeholder, so a
// match may cross a brace pair with no brace inside it and never an unpaired
// brace: it stays inside the block it starts in.
var (
	uiSHA   = regexp.MustCompile(`("bundle"\s*:\s*\{(?:[^{}]|\{[^{}]*\})*"sha256"\s*:\s*")([0-9a-f]{64})(")`)
	wasmSHA = regexp.MustCompile(`("wasm"\s*:\s*\{(?:[^{}]|\{[^{}]*\})*"sha256"\s*:\s*")([0-9a-f]{64})(")`)
)

func sumOf(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:]), nil
}

// stamp replaces one 64-hex value in place, leaving every other byte of the
// file as it was.
func stamp(field *regexp.Regexp, path string) error {
	sum, err := sumOf(path)
	if err != nil {
		return err
	}
	src, err := os.ReadFile(manifestFile)
	if err != nil {
		return err
	}
	if n := len(field.FindAllIndex(src, -1)); n != 1 {
		return fmt.Errorf("%s: expected exactly one place for the hash of %s, found %d", manifestFile, path, n)
	}
	out := field.ReplaceAll(src, []byte("${1}"+sum+"${3}"))
	if err := validate(out); err != nil {
		return err
	}
	if err := os.WriteFile(manifestFile, out, 0o644); err != nil {
		return err
	}
	fmt.Println(sum)
	return nil
}

// check fails unless filex-app.json pins the bundle and the module given -
// the release refuses to publish anything else.
func check(uiZip, wasm string) error {
	src, err := os.ReadFile(manifestFile)
	if err != nil {
		return err
	}
	for _, c := range []struct {
		field *regexp.Regexp
		path  string
		what  string
	}{{uiSHA, uiZip, "ui.bundle.sha256"}, {wasmSHA, wasm, "wasm.sha256"}} {
		m := c.field.FindSubmatch(src)
		if m == nil {
			return fmt.Errorf("%s: no %s", manifestFile, c.what)
		}
		sum, err := sumOf(c.path)
		if err != nil {
			return err
		}
		if string(m[2]) != sum {
			return fmt.Errorf("%s pins %s %s, but %s hashes to %s: run bash scripts/build.sh --stamp and commit", manifestFile, c.what, m[2], c.path, sum)
		}
	}
	return nil
}

// pins prints the two hashes filex-app.json pins, "ui <sha>" and
// "wasm <sha>".
func pins() error {
	src, err := os.ReadFile(manifestFile)
	if err != nil {
		return err
	}
	for _, c := range []struct {
		name  string
		field *regexp.Regexp
	}{{"ui", uiSHA}, {"wasm", wasmSHA}} {
		m := c.field.FindSubmatch(src)
		if m == nil {
			return fmt.Errorf("%s: no %s hash", manifestFile, c.name)
		}
		fmt.Printf("%s %s\n", c.name, m[2])
	}
	return nil
}
