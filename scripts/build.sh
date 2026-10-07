#!/usr/bin/env bash
# Builds Media Thumb Engine: the preview page (dist/ui.zip) and the module
# (dist/plugin.wasm) - after the tests pass.
#
#   bash scripts/build.sh            dist/ui.zip, dist/plugin.wasm, their .sha256 files and dist/filex-app.json
#   bash scripts/build.sh --stamp    also writes both hashes into filex-app.json (before tagging a release)
#
# The order is the contract's (tools/manifest says why): the bundle first,
# its hash into filex-app.json, manifest.embed.json from it, then the module,
# which describes that bundle, then the module's hash. Without --stamp a
# hash that differs from filex-app.json's is reported, not written: the
# module built then describes the bundle filex-app.json names, and a release
# refuses it (tools/manifest check).
#
# Both builds are REPRODUCIBLE: filex checks each download against the hash
# the manifest pins, so the release workflow, building again at the tag, must
# get the same bytes:
#
#   - the Go toolchain is the one go.mod pins (`toolchain goX.Y.Z`), forced
#     through GOTOOLCHAIN - another compiler emits other bytes;
#   - -trimpath drops this machine's paths, -buildvcs=false the git commit
#     (the commit that carries the hash would change the hash);
#   - the bundle is written by tools/bundle: sorted, stored, one date, LF.
#
# SKIP_TESTS=1 skips the tests: for bisecting a build problem, never for a
# release.
set -euo pipefail
cd "$(dirname "$0")/.."

stamp=0
case "${1:-}" in
  "") ;;
  --stamp) stamp=1 ;;
  *) echo "usage: bash scripts/build.sh [--stamp]" >&2; exit 2 ;;
esac

pinned="$(sed -n 's/^toolchain //p' go.mod)"
if [ -z "$pinned" ]; then
  echo "go.mod has no 'toolchain goX.Y.Z' line: the build could not be reproduced elsewhere" >&2
  exit 1
fi
export GOTOOLCHAIN="$pinned"

if [ "$stamp" = 1 ] && grep -q '^replace ' go.mod; then
  echo "go.mod replaces a module with a local copy; a release builds against published modules" >&2
  exit 1
fi

# The module's size gate: filex's default FILEX_APP_PLUGIN_MAX_WASM_MB is 64,
# and every install compiles the module. Measure the build before raising it.
MAX_MB="${MAX_WASM_MB:-12}"

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  else
    shasum -a 256 "$1" | cut -d' ' -f1
  fi
}

# pin ui|wasm: the hash filex-app.json pins.
pin() {
  go run ./tools/manifest pins | sed -n "s/^$1 //p"
}

mkdir -p dist

echo "== the preview page =="
go run ./tools/bundle
ui_sum="$(sha256 dist/ui.zip)"
if [ "$stamp" = 1 ]; then
  go run ./tools/manifest stamp-ui dist/ui.zip >/dev/null
elif [ "$(pin ui)" != "$ui_sum" ]; then
  echo "note: filex-app.json pins another ui.zip ($(pin ui)); this tree builds $ui_sum - run with --stamp to write it" >&2
fi

# manifest.embed.json follows filex-app.json (tools/manifest embed).
go generate ./...

if [ "${SKIP_TESTS:-0}" = 1 ]; then
  echo "tests skipped (SKIP_TESTS=1) - do not release this build" >&2
else
  echo "== tests ($(go version)) =="
  unformatted="$(gofmt -l . || true)"
  if [ -n "$unformatted" ]; then
    echo "not gofmt-clean:" >&2
    echo "$unformatted" >&2
    exit 1
  fi
  go vet ./...
  if ! go test -count=1 ./...; then
    echo "build refused: the Go tests do not pass, so plugin.wasm is not produced" >&2
    exit 1
  fi
  if ! node --test ui/test/*.test.js; then
    echo "build refused: the page's tests do not pass" >&2
    exit 1
  fi
fi

echo "== the module =="
GOOS=wasip1 GOARCH=wasm go build -trimpath -buildvcs=false -ldflags="-s -w" -buildmode=c-shared -o dist/plugin.wasm ./cmd/plugin
wasm_sum="$(sha256 dist/plugin.wasm)"
size="$(wc -c < dist/plugin.wasm | tr -d ' ')"
mb="$(awk -v b="$size" 'BEGIN { printf "%.2f", b / 1048576 }')"
echo "dist/plugin.wasm  ${size} bytes (${mb} MB)  sha256 ${wasm_sum}"
if [ "$size" -gt $((MAX_MB * 1048576)) ]; then
  echo "size gate: plugin.wasm is ${mb} MB, over ${MAX_MB} MB" >&2
  exit 1
fi

if [ "$stamp" = 1 ]; then
  go run ./tools/manifest stamp-wasm dist/plugin.wasm >/dev/null
  go run ./tools/manifest check dist/ui.zip dist/plugin.wasm
  echo "stamped ui.bundle.sha256 and wasm.sha256 into filex-app.json - commit it (and manifest.embed.json), then tag v$(sed -n 's/^  "version": "\([^"]*\)".*/\1/p' filex-app.json | head -1)"
elif [ "$(pin wasm)" != "$wasm_sum" ]; then
  echo "note: filex-app.json pins another plugin.wasm; run with --stamp before a release" >&2
fi

echo "$ui_sum  ui.zip" > dist/ui.zip.sha256
echo "$wasm_sum  plugin.wasm" > dist/plugin.wasm.sha256
cp filex-app.json dist/filex-app.json
