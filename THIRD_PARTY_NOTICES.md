# Third-party notices

Media Thumb Engine is MIT-licensed (LICENSE, BRF Tech). What it is built
from, and under which terms:

## In the module (plugin.wasm)

| Component | Version | License | Used for |
|---|---|---|---|
| [filex guest SDK](https://github.com/BRF-Tech/filex) (`github.com/brf-tech/filex/backend/pkg/pluginkit`) | v0.52.0 | MIT, BRF Tech | the exports filex calls, the host functions, `thumbkit` |
| [Extism Go PDK](https://github.com/extism/go-pdk) (`github.com/extism/go-pdk`) | v1.1.3 | BSD-3-Clause, Dylibso, Inc. | the wasm ABI under the SDK |
| [golang.org/x/image](https://pkg.go.dev/golang.org/x/image) | v0.39.0 | BSD-3-Clause, The Go Authors | font parsing and drawing (`font/sfnt`, `font/opentype`), WebP and TIFF decoding, scaling |
| Go Mono (`golang.org/x/image/font/gofont/gomono`) | in x/image v0.39.0 | the Go fonts' license (BSD-3-Clause style), Bigelow & Holmes Inc. | the caption strip of a font's thumbnail |
| [golang.org/x/text](https://pkg.go.dev/golang.org/x/text) | v0.40.0 | BSD-3-Clause, The Go Authors | Mac Roman font names |
| [Brotli for Go](https://github.com/andybalholm/brotli) (`github.com/andybalholm/brotli`) | v1.2.6 | MIT, the Brotli Authors | WOFF2 decompression |

The Go standard library (BSD-3-Clause, The Go Authors) is linked into the
module too.

## In the interface bundle (ui.zip)

| Component | Version | License | Used for |
|---|---|---|---|
| [@brftech/filex-app-ui](https://www.npmjs.com/package/@brftech/filex-app-ui) (`ui/vendor/filex-app-ui.js`, the npm package's `dist/filex-app-ui.js` as published) | 0.52.0 | MIT, BRF Tech (`ui/vendor/LICENSE`) | the bridge to filex |

The bundle carries no other code: the page is this repository's own
(`ui/`), and a font is drawn by the browser itself.

## In the tests (testdata/, not shipped)

| File | License |
|---|---|
| `testdata/fonts/Go-Regular.ttf` (`golang.org/x/image/font/gofont/ttfs`) and the `.woff` / `.woff2` made from it | the Go fonts' license, Bigelow & Holmes Inc. (`testdata/fonts/Go-Regular.LICENSE.txt`) |
| `testdata/samples/*`, `internal/design/testdata/*.webp` | made for these tests (`scripts/gen-fixtures.py`); MIT, like the rest of the repository |

## Not used

[ag-psd](https://github.com/Agamnentzar/ag-psd) (MIT), which filex's own
Photoshop viewer uses in the browser, is not part of this app: the module
reads a Photoshop file's merged image and thumbnail itself (`internal/design/psd.go`,
from Adobe's published specification), and the page shows the picture the
module hands it.

No program's code was disassembled to write the readers: the Affinity and
Pixelmator Pro layouts were measured on files those programs saved and
published under open licenses (see README, "How each format is read").
