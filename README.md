# Media Thumb Engine

A [filex](https://github.com/BRF-Tech/filex) app that gives design files and
fonts a **thumbnail** in every view of the explorer and a **preview** of
their own:

| Kind | Extensions | The thumbnail | The preview |
|---|---|---|---|
| Photoshop | `.psd`, `.psb` | the merged image of the document, else the thumbnail Photoshop saves | the merged image (up to 1600 px), what the file says (size, color mode, bit depth, channels, layers) |
| Affinity | `.af` (Affinity 3), `.afphoto`, `.afdesign`, `.afpub` (Affinity 1 and 2) | the PNG Affinity saves inside the file | that PNG, the container version and the program |
| Pixelmator Pro | `.pxd` (the single-file ZIP of Pixelmator Pro 2.4 and later) | the QuickLook preview inside the ZIP | that picture, and what is inside the ZIP |
| Fonts | `.ttf`, `.otf`, `.woff`, `.woff2` | the font drawn with itself: "Aa", its alphabet and digits, its name | a sample text you can change at six sizes, its weights (a variable font's named styles), every character it draws, its names, version, designer and license |

None of the design formats is opened as a document. Each program saves a
flattened picture of the document inside the file, for file browsers, and
that picture is what this app reads - so it needs no Photoshop, Affinity or
Pixelmator, and reads a 2 GB document as quickly as a small one. A font has
no picture of itself; it is drawn.

Without the app, filex is as it was: these files keep their type icons, and
open in filex's own viewer where it has one.

## Install

The app needs **filex 0.50.0 or later** (apps that draw thumbnails came with
0.50.0), with apps switched on (`FILEX_APP_PLUGINS_DISABLED` unset).

- **From the store** (filex 0.52.0 or later, which installs from a store
  link): [apps.filex.sh/apps/media-thumbs](https://apps.filex.sh/apps/media-thumbs)
  → **Install**, and give your filex's address. filex opens the same
  permission review as below, marked *From store apps.filex.sh*.
- **From GitHub:** Admin → Plugins → Apps → **Install an app** → GitHub
  repository: `BRF-Tech/filex-media-thumbs`, **Ref:** the release's tag
  (`v0.1.0`). filex reads `filex-app.json` at that tag, downloads
  `plugin.wasm` and `ui.zip` from the release and refuses either unless it
  hashes to what the manifest pins.
- **From files:** Admin → Plugins → Apps → Install an app → **Upload
  files**: `plugin.wasm`, `filex-app.json` and the **Interface bundle**
  `ui.zip`, all three from the same release (or from your own build,
  `dist/`).

### What the review asks

| Permission | Why |
|---|---|
| `files:read` | the preview reads the one file you open in it. Nothing else is read, nothing is written |
| `ui` | the preview is the app's own page, in filex's sandboxed frame, with no network |
| `ui-viewer:.psd` … `ui-viewer:.woff2` (11) | it opens those files in place of filex's preview |
| `thumbnail:.psd` … `thumbnail:.woff2` (11) | it draws their thumbnails: filex hands it the bytes of each such file, one at a time |

No network (`http:`), no settings, no state, no engine, no action in the
file menu. A thumbnail call can read only the file it is handed; filex
refuses it everything else.

The review's **File types** group asks, per kind, whether the app goes
**first**, **after** the handlers already there, or **off** - for opening and
for thumbnails. Two kinds are worth a look:

- **`.psd`**: a Photoshop file is an `image/` type, so filex's own drawer is
  in its thumbnail list too, first by default. It tries Go's image decoders,
  which do not read Photoshop files, and passes the file on to this app -
  the thumbnail comes either way. Choosing **first** saves that attempt.
- **`.pxd`**: Cython's declaration files are `.pxd` too, and are text. This
  app refuses them: a text `.pxd` gets no thumbnail from it (filex asks the
  next handler) and its preview says it is a Cython file, to be opened with
  the built-in viewer (**Open with**). Where your users keep Cython code,
  switch this app **off** for opening `.pxd` and leave it on for thumbnails.

Both can be changed later in Admin → Plugins → **Default apps**. The app's
page (Admin → Plugins → Apps → Media Thumb Engine → **Thumbnails**) holds the
limits filex keeps it to: the largest file it is sent (32 MB by default), its
time per file (10 s), its memory and how many files it draws at once.

## How each format is read

All of it is untrusted input. The file is read as a stream under a budget of
bytes and a deadline, every length a file states is checked before it is
followed, and a picture inside a file is measured from its own header before
a pixel of it is decoded (at most 16 MB, 8192 pixels a side, 8 megapixels).
A file that does not hold together is refused with a reason; the reader
never reads past what the file promised.

**Photoshop** ([Adobe's Photoshop File Formats Specification](https://www.adobe.com/devnet-apps/photoshop/fileformatashtml/)):
the header (`8BPS`, version 1 for PSD and 2 for PSB, size, channels, depth,
color mode), the image resources - **1036** (the JPEG thumbnail, at most 160
pixels), **1033** (the same from Photoshop 4, blue first) and **1057**
(whether the merged image is real) - the number of layers, and the **merged
image**: raw or PackBits, 8 or 16 bits a channel, grayscale, RGB, CMYK or
indexed color. Only the rows and the pixels that land in the size asked for
are unpacked, so memory is the picture's size, not the document's. A file
saved without "Maximize Compatibility" (1057 says the merged image is not
real), a color mode or depth not read here (Lab, duotone, multichannel,
bitmap, 32 bits) or a merged image past the limits shows its thumbnail
resource instead. Layers are never decoded.

**Affinity** (Affinity 3's `.af` and the `.afphoto` / `.afdesign` / `.afpub`
of Affinity 1 and 2 are one container, starting `00 FF 4B 41`): the format is
not published, and its streams are zstd-compressed; none of them is read. The
header's `#Inf` block points at a `Thmb` block holding an uncompressed PNG of
the document, 512 pixels on its long side, which Affinity saves for file
browsers. Measured on files saved by Affinity 3.2.3 and Affinity Photo 2.6.5
(the MIT-licensed test files of [Patchy](https://github.com/SethRobinson/Patchy/blob/main/docs/af-format.md)),
an incrementally saved one included - where the header still names the
current preview while an original JPEG of a placed image sits elsewhere in
the file, so "the first picture in the file" would be wrong. A file whose
header does not lead to a PNG (an older container) is searched for one in its
first 64 MB.

**Pixelmator Pro** (`.pxd` since Pixelmator Pro 2.4, a ZIP: `metadata.info`
SQLite, `data/` pixel tiles, `QuickLook/`): the preview is
`QuickLook/Thumbnail.webp` (up to 1024 pixels; `Preview.*`, then the 64-pixel
`Icon.*` when it is missing; TIFF before 2.4). Measured on real files from
2023 and 2026 ([pxdlib's notes](https://github.com/yunruse/pxdlib/blob/master/docs/pxd/readme.md)
describe the older package format). The ZIP's directory is read from the
file's end and only the preview member is unpacked. A `.pxd` that does not
start with a ZIP header is not Pixelmator's and is refused.

**Fonts**: TrueType and OpenType (CFF) as they are, the first font of a
collection, **WOFF** (zlib tables) and **WOFF2** (Brotli, with the `glyf` /
`loca` and `hmtx` transforms undone, as the [WOFF2 specification](https://www.w3.org/TR/WOFF2/)
describes) turned back into the font they hold. The thumbnail is drawn with
`golang.org/x/image/font/sfnt`; a font without Latin letters is drawn with the
first letters it has (Greek, Cyrillic, Hebrew, Arabic, Devanagari, Thai,
kana, CJK, Hangul) and a symbol font with its symbols. The preview draws the
font in your browser (`FontFace`, which checks every web font first) and
reads its `name`, `OS/2`, `maxp`, `fvar` and `cmap` tables in the module, so
a WOFF2's characters are listed too. A color (emoji) font has no outline to
draw: no thumbnail. CFF2 is not read by the thumbnail.

The preview page and the module never send a file anywhere: the app holds
no network permission, and its page runs with no network.

## Build

You need **Go** (the toolchain `go.mod` pins, `go1.27.1`, is downloaded by Go
itself) and **Node.js 20 or later** for the page's tests (no packages are
installed).

```bash
bash scripts/build.sh            # tests, then dist/ui.zip and dist/plugin.wasm
bash scripts/build.sh --stamp    # also writes both hashes into filex-app.json
```

The order matters and the script keeps it: the interface bundle is built
first and its hash goes into `filex-app.json` (`ui.bundle.sha256`), because
the module **describes** the interface and filex refuses a module that
describes another one; `manifest.embed.json` is written from it (`go
generate`), then the module is built (`GOOS=wasip1 GOARCH=wasm`,
`-buildmode=c-shared`), then its hash goes in (`wasm.sha256`). The module
embeds the manifest without its `wasm` block - it cannot carry its own hash.

Both builds are reproducible: the pinned toolchain, `-trimpath`,
`-buildvcs=false`, and a bundle written by `tools/bundle` (sorted, stored, one
date, LF line ends). The release workflow builds both again at the tag and
refuses to publish unless they hash to what the manifest pins
(`go run ./tools/manifest check dist/ui.zip dist/plugin.wasm`).

### Releasing

1. Bump `version` in `filex-app.json` and write its section in
   `CHANGELOG.md` (the release's notes, which filex shows an administrator
   when it offers the update: `go run ./tools/notes <version>`).
2. `bash scripts/build.sh --stamp`.
3. Commit `filex-app.json` and `manifest.embed.json`, tag `vX.Y.Z`, push the
   tag. `.github/workflows/release.yml` attaches `plugin.wasm`, `ui.zip` and
   `filex-app.json` to the release.

## Test

```bash
bash scripts/test.sh             # gofmt, go vet, go test, the page's tests
go test ./internal/fonts -run WOFF2
npm test                         # the page's tests alone (node --test)
```

The Go tests run without the module: the readers on files built in code
(each format the way its program writes it) and on the samples in
`testdata/` (`scripts/gen-fixtures.py` writes them; it needs fontTools,
brotli and Pillow), and the app against the fake filex of the SDK
(`pluginkit/plugintest`), which refuses what filex refuses. A decoded WOFF or
WOFF2 is compared with the TTF it was made from by what it draws: every
glyph's outline, advance and left side bearing.

The manifest is held to filex's install checks by `manifest_test.go`
(`plugintest.InspectManifest`; plugintest 0.52 does not know interface views
yet, and that one finding is allowed). filex itself is the final word:
its installer's dry run reads the three files and answers the review it would
show, or the refusal:

```bash
curl -sS -X POST "$FILEX/api/admin/app-plugins?dry_run=1" \
  -H "Authorization: Bearer $ADMIN_API_KEY" \
  -F wasm=@dist/plugin.wasm -F manifest=@dist/filex-app.json -F ui=@dist/ui.zip
```

## License

MIT, BRF Tech - see [LICENSE](LICENSE). What the app is built from, and
under which terms, is in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md):
the filex SDK (MIT), golang.org/x/image and golang.org/x/text (BSD-3-Clause),
the Go fonts (Go Mono draws a font thumbnail's caption; Go Regular is a test
fixture: their own BSD-style license), Brotli for Go (MIT) and the Extism Go
PDK (BSD-3-Clause). [ag-psd](https://github.com/Agamnentzar/ag-psd), which
filex's own Photoshop viewer uses, is not part of this app.
