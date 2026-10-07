// Package mediathumbs is the root of Media Thumb Engine, a filex app that
// draws the thumbnails of design files and fonts and opens them in a
// preview of its own. It holds the manifest the module describes and
// nothing else: the reading is in internal/design and internal/fonts, the
// answers filex asks for in internal/app, the wasm entry point in
// cmd/plugin and the preview page in ui/.
package mediathumbs

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/brf-tech/filex/backend/pkg/pluginkit/wire"
)

// manifest.embed.json is filex-app.json without its `wasm` block, written
// by `go generate` (tools/manifest). The module cannot carry its own hash,
// so it embeds the manifest without it; everything else - the `ui` block
// with the interface bundle's hash included - is what filex compares with
// the manifest the administrator approved. Edit filex-app.json, never this
// copy.
//
//go:generate go run ./tools/manifest embed
//go:embed manifest.embed.json
var manifestJSON []byte

// ManifestJSON is the embedded manifest as it is.
func ManifestJSON() []byte { return manifestJSON }

// Manifest is what the `describe` export answers. A manifest that does not
// parse is a build mistake, not something a file can cause, so it panics.
func Manifest() wire.Manifest {
	m, err := ParseManifest(manifestJSON)
	if err != nil {
		panic(err)
	}
	return m
}

// ParseManifest reads a filex-app.json strictly: a field filex does not know
// is an error here for the reason filex refuses it at install (a typo such
// as "permisions" would install an app whose every call is refused).
func ParseManifest(b []byte) (wire.Manifest, error) {
	var m wire.Manifest
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return wire.Manifest{}, fmt.Errorf("filex-app.json: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return wire.Manifest{}, errors.New("filex-app.json: trailing data after the manifest")
	}
	if m.Name == "" || m.Version == "" {
		return wire.Manifest{}, errors.New("filex-app.json: name and version are required")
	}
	if m.ManifestVersion == 0 {
		m.ManifestVersion = wire.ProtocolVersion
	}
	return m, nil
}
