// The Media Thumb Engine wasm module. Build (scripts/build.sh does it, after
// the tests and the interface bundle):
//
//	GOOS=wasip1 GOARCH=wasm go build -trimpath -buildvcs=false -ldflags="-s -w" -buildmode=c-shared -o dist/plugin.wasm ./cmd/plugin
//
// A c-shared wasip1 module is a reactor: filex calls its exports and main()
// never runs, so the app registers from init().
package main

import (
	mediathumbs "github.com/brf-tech/filex-media-thumbs"
	"github.com/brf-tech/filex-media-thumbs/internal/app"
	"github.com/brf-tech/filex/backend/pkg/pluginkit"
)

func main() {}

func init() {
	pluginkit.Run(app.New(app.SDKHost{}, mediathumbs.Manifest()).Plugin())
}
