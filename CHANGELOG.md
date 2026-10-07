# Changelog

All notable changes to Media Thumb Engine are recorded here. The format
follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions
follow [Semantic Versioning](https://semver.org/). The version in
`filex-app.json` is the one a tag must match, and a version's section is its
GitHub release's notes - what filex shows an administrator when it offers the
update (`go run ./tools/notes <version>`).

## [Unreleased]

## [0.1.0] - 2026-10-07

The first version: thumbnails and a preview for design files and fonts, as a
filex app (filex 0.50.0 or later), in English and Turkish.

### Added

- **Thumbnails** for Photoshop (`.psd`, `.psb`), Affinity (`.af`,
  `.afphoto`, `.afdesign`, `.afpub`), Pixelmator Pro (`.pxd`) and fonts
  (`.ttf`, `.otf`, `.woff`, `.woff2`), drawn by the module through filex's
  `thumbnail` export:
  - Photoshop: the merged image, sampled into the thumbnail a row at a time
    (raw or PackBits; 8 or 16 bits; grayscale, RGB, CMYK, indexed color),
    else the JPEG thumbnail the file carries (resource 1036, or 1033 from
    Photoshop 4) - for a file saved without "Maximize Compatibility", a color
    mode or depth not read, or a merged image past the limits.
  - Affinity: the PNG the container's `#Inf` header points at, else the
    first whole PNG in the file's first 64 MB.
  - Pixelmator Pro: the `QuickLook/Thumbnail` member of the ZIP (then
    `Preview`, then `Icon`), read through the ZIP's directory.
  - Fonts: the font drawn with itself - "Aa", its alphabet and digits (or
    the first letters of its own script, or its symbols) and its name - on
    the page filex draws its text thumbnails on. WOFF and WOFF2 are turned
    back into the font they hold (the WOFF2 `glyf` / `loca` and `hmtx`
    transforms undone).
- **A preview** in place of filex's own for those files, the app's own page
  (`@brftech/filex-app-ui`), in filex's colors, light and dark, from a phone's
  width up:
  - a design file: its picture (a Photoshop file's merged image up to 1600
    pixels), where the picture comes from, and what the file states -
    document size, color mode, bit depth, channels, layers, whether the
    merged image was saved (Photoshop); the program and container version
    (Affinity); the items in the ZIP (Pixelmator Pro);
  - a font: a sample text to change, at six sizes; its weights - a variable
    font's named styles, else its weight axis at every hundred, a static
    font at the one weight it has (never a weight the browser would fake);
    every character it maps (the first 256, all of them up to 4096 on
    request); its names, version, designer, foundry and license.
- **Cython's `.pxd` refused**: a text `.pxd` is a Cython declaration file,
  not a Pixelmator Pro document - no thumbnail from this app (filex asks the
  next handler), and the preview says to open it with the built-in viewer.
- **Limits against hostile files**: a budget of bytes read and a deadline per
  file, at most eight reads of a file from its start, every stated length
  checked before it is followed, a picture inside a file measured from its
  header before it is decoded (16 MB, 8192 pixels a side, 8 megapixels), a
  WOFF/WOFF2 unpacked to at most 64 MB, a font's characters walked at most
  two million times, and a panic taken as the file's failure. Module memory:
  `memory_pages` 2048 (128 MiB); the page's calls: 30 s.
- **One permission**, `files:read`: the app writes nothing, has no network,
  no settings and no state.
- **Reproducible builds** of the module and the interface bundle
  (`scripts/build.sh`, `tools/bundle`, `tools/manifest`), and GitHub
  workflows that test every change and publish a tag only when both builds
  hash to what `filex-app.json` pins.
