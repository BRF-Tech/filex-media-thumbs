#!/usr/bin/env bash
# Every check a build runs, without building: gofmt, vet, the Go tests (the
# readers, the manifest, the app against plugintest's fake filex) and the
# page's tests (Node's own test runner, no dependencies).
#
#   bash scripts/test.sh            everything
#   bash scripts/test.sh -run PSD   arguments go to `go test`
set -euo pipefail
cd "$(dirname "$0")/.."
export GOTOOLCHAIN="$(sed -n 's/^toolchain //p' go.mod)"
unformatted="$(gofmt -l . || true)"
if [ -n "$unformatted" ]; then
  echo "not gofmt-clean:" >&2
  echo "$unformatted" >&2
  exit 1
fi
go vet ./...
go test -count=1 ./... "$@"
node --test ui/test/*.test.js
