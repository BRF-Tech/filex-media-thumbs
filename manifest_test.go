package mediathumbs

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/brf-tech/filex/backend/pkg/pluginkit/plugintest"
	"github.com/brf-tech/filex/backend/pkg/pluginkit/wire"

	"github.com/brf-tech/filex-media-thumbs/internal/design"
	"github.com/brf-tech/filex-media-thumbs/internal/fonts"
)

// filex-app.json is the only file an administrator reviews. These tests hold
// it to what filex refuses at install (as far as plugintest knows it) and to
// what this app does: the kinds it draws and opens are the kinds its readers
// read, its one permission is reading the file it is handed, and the module
// describes exactly that manifest.

func readManifestFile(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("filex-app.json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestManifest_ParsesStrictly(t *testing.T) {
	m, err := ParseManifest(readManifestFile(t))
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "media-thumbs" || m.Version == "" || m.Wasm == nil || m.UI == nil || m.Thumbnails == nil {
		t.Fatalf("manifest %+v", m)
	}
	if _, err := ParseManifest([]byte(`{"name":"x","version":"1.0.0","permisions":[]}`)); err == nil {
		t.Fatal("a misspelt field is refused, as filex refuses it")
	}
	if _, err := ParseManifest(append(readManifestFile(t), []byte(`{}`)...)); err == nil {
		t.Fatal("trailing data is refused")
	}
}

// manifest.embed.json is filex-app.json without its `wasm` block, member for
// member, in the same order (go generate writes it; CI compares the bytes).
func TestManifest_EmbedIsFresh(t *testing.T) {
	full := readManifestFile(t)
	embedded, err := os.ReadFile("manifest.embed.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(embedded, ManifestJSON()) {
		t.Fatal("the module embeds another manifest.embed.json than the one on disk")
	}
	var a, b map[string]any
	if err := json.Unmarshal(full, &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(embedded, &b); err != nil {
		t.Fatal(err)
	}
	delete(a, "wasm")
	if !reflect.DeepEqual(a, b) {
		t.Fatal("manifest.embed.json is not filex-app.json without its wasm block: run go generate ./...")
	}
	if !slices.Equal(topKeys(t, embedded), slices.DeleteFunc(topKeys(t, full), func(k string) bool { return k == "wasm" })) {
		t.Fatal("manifest.embed.json has the members in another order: run go generate ./...")
	}
}

func topKeys(t *testing.T, b []byte) []string {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(b))
	if _, err := dec.Token(); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, tok.(string))
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			t.Fatal(err)
		}
	}
	return keys
}

// What `describe` answers is the manifest the administrator approved: filex
// refuses the module otherwise (name, version, permissions, and the
// interface it describes).
func TestManifest_DescribeIsTheManifest(t *testing.T) {
	onDisk, err := ParseManifest(readManifestFile(t))
	if err != nil {
		t.Fatal(err)
	}
	onDisk.Wasm = nil
	a, _ := json.Marshal(onDisk)
	b, _ := json.Marshal(Manifest())
	if !bytes.Equal(a, b) {
		t.Fatalf("describe answers another manifest:\n%s\n%s", b, a)
	}
}

// What plugintest knows of filex's install checks. plugintest 0.52 predates
// app interfaces: it calls a `viewer` placement unknown. That finding, for a
// view with an interface, is the one allowed.
func TestManifest_WhatFilexChecks(t *testing.T) {
	m := Manifest()
	for _, f := range plugintest.InspectManifest(m).Errors() {
		if strings.HasPrefix(f.Where, "manifest.views[") && strings.Contains(f.Message, `"viewer"`) {
			continue
		}
		t.Errorf("filex would refuse it: %s", f)
	}
	for _, f := range plugintest.InspectManifestLanguages(m, plugintest.LangOpts{SameAllowed: []string{"Media Thumb Engine"}}).Errors() {
		t.Errorf("a text is missing a language: %s", f)
	}
	if !slices.Equal(m.Languages, []string{"en", "tr"}) {
		t.Fatalf("languages %v", m.Languages)
	}
	if m.Filex != ">=0.50.0" {
		t.Fatalf("filex %q: thumbnails drawn by apps came in 0.50.0, and filex before it refuses the block", m.Filex)
	}
	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(m.Version) {
		t.Fatalf("version %q is not MAJOR.MINOR.PATCH", m.Version)
	}
	if m.Limits.MemoryPages < 1024 || m.Limits.MemoryPages > 4096 || m.Limits.CallTimeoutS < 1 || m.Limits.CallTimeoutS > 60 {
		t.Fatalf("limits %+v are outside what filex allows", m.Limits)
	}
}

// One permission: reading the file it is handed. Nothing written, no
// network, no state; the interface's and the kinds' grants are derived by
// filex and never listed.
func TestManifest_TheNarrowestGrant(t *testing.T) {
	m := Manifest()
	if !slices.Equal(m.Permissions, []string{"files:read"}) {
		t.Fatalf("permissions %v, want only files:read", m.Permissions)
	}
	for k, reason := range m.PermissionReasons {
		if k != "files:read" && k != "ui" {
			t.Fatalf("a reason for %q, which the app does not ask for", k)
		}
		if reason["en"] == "" || reason["tr"] == "" {
			t.Fatalf("the reason for %q is not in both languages", k)
		}
	}
	if m.UI == nil || m.UI.PackageFetch || m.UI.Download || len(m.UI.CSP) > 0 || len(m.UI.External) > 0 {
		t.Fatalf("the interface asks for more than its own files: %+v", m.UI)
	}
	if len(m.Actions) > 0 || len(m.Settings) > 0 || len(m.PublicPages) > 0 || len(m.NewDocuments) > 0 {
		t.Fatal("the app has no actions, settings, public pages or new documents")
	}
}

func sorted(s []string) []string {
	out := slices.Clone(s)
	slices.Sort(out)
	return out
}

// The kinds it draws and opens are the kinds its readers read.
func TestManifest_TheKinds(t *testing.T) {
	m := Manifest()
	all := sorted(append(design.Extensions(), fonts.Extensions()...))
	a := m.Thumbnails.Applies
	if a.Kind != "file" || len(a.Mime) > 0 || !slices.Equal(sorted(a.Ext), all) {
		t.Fatalf("thumbnails.applies %+v, want the extensions %v", a, all)
	}
	views := map[string]wire.View{}
	for _, v := range m.Views {
		views[v.ID] = v
		if v.Placement != "viewer" || v.UI != "index.html" || v.Applies.Kind != "file" || len(v.Applies.Mime) > 0 {
			t.Fatalf("view %q: %+v", v.ID, v)
		}
		if _, err := os.Stat(filepath.Join("ui", v.UI)); err != nil {
			t.Fatalf("view %q opens %s, which ui/ does not have", v.ID, v.UI)
		}
	}
	if len(views) != 2 || !slices.Equal(sorted(views["design"].Applies.Ext), sorted(design.Extensions())) || !slices.Equal(sorted(views["font"].Applies.Ext), sorted(fonts.Extensions())) {
		t.Fatalf("views %+v", m.Views)
	}
	for _, e := range all {
		for _, lang := range []string{"en", "tr"} {
			if !strings.Contains(m.Description[lang], "."+e) {
				t.Fatalf("the %s description does not name .%s", lang, e)
			}
		}
	}
}

// The module and the interface are downloaded from this repository's GitHub
// release of the tag being installed, and pinned by their hashes.
func TestManifest_Downloads(t *testing.T) {
	m, err := ParseManifest(readManifestFile(t))
	if err != nil {
		t.Fatal(err)
	}
	const release = "https://github.com/BRF-Tech/filex-media-thumbs/releases/download/{tag}/"
	hex := regexp.MustCompile(`^[0-9a-f]{64}$`)
	if m.Wasm.URL != release+"plugin.wasm" || !hex.MatchString(m.Wasm.SHA256) {
		t.Fatalf("wasm %+v", m.Wasm)
	}
	if m.UI.Bundle.URL != release+"ui.zip" || !hex.MatchString(m.UI.Bundle.SHA256) {
		t.Fatalf("ui.bundle %+v", m.UI.Bundle)
	}
	if m.Homepage != "https://github.com/BRF-Tech/filex-media-thumbs" {
		t.Fatalf("homepage %q", m.Homepage)
	}
}
