# Changelog

All notable changes to this project are documented here. The project follows
[Semantic Versioning](https://semver.org/); until 1.0.0, minor versions may
change the API.

## [Unreleased]

### Changed
- Further behaviour measured on a ZD420; these symbologies now match its
  output pixel for pixel:
  - CODABLOCK: row height includes the one-module separator.
  - `^BR`: data in parentheses is encoded as literal text (DataBar Expanded,
    composite GS1-128 and 2D components). Composite symbols start 7 or 10
    modules right of the origin. UPC/EAN composites have fixed bar heights
    and a 6-module separator. Composite Code 128 prefers subset A and drops
    the linkage character when there is no 2D part.
  - PDF417/MicroPDF417: Numeric Compaction from 14 digits (all-digit data
    sooner), punctuation shifts, and MicroPDF417 Zebra padding.
  - TLC39: no flag without a 2D part, `*` field separators, the printer's
    MicroPDF417 sizing, a one-module gap and the taller flag.
- The printer harness takes `ZPL_PRINTER_ONLY` to run selected fixtures.
- Labels and field masks are packed one bit per pixel. Rendering is about
  40% faster, uses about 80% less memory (450 KB for a 4×6 label at
  8 dots/mm) and scales across cores. Rendered images are now a read-only
  1-bit `image.PalettedImage` instead of `*image.Paletted`; code that
  type-asserted `*image.Paletted` should use the `image.Image` interface.
- MaxiCode hexagons and finder rings are filled a row span at a time.
- Font caches are read without locking, and the bitmap-font glyph cache is
  bounded.

### Known limitations
- A ZD420 misencodes unbracketed DataBar Expanded data with AIs 01 and
  320x (as 393x); this library keeps the correct encoding.

## [0.1.0] - 2026-10-31

First release.

### Rendering
- Renders ZPL II label formats to images and PNGs: `Render`, `RenderPNG`,
  and a persistent `Printer` that keeps downloaded graphics, stored formats,
  fonts and settings between jobs, like a real printer.
- Every command in the Zebra ZPL II Programming Guide has a handler. Commands
  that only drive hardware (media, sensors, network, RFID, host status) are
  accepted without effect; a test enforces full coverage.
- Densities of 6, 8, 12 and 24 dots/mm; label size from `^PW`/`^LL` or
  `Options`.

### Text
- Font 0 and the bitmap fonts A–H, P–V and GS, drawn with open look-alike
  fonts calibrated against reference renders. Replace them with the real
  Zebra fonts via `Options.Fonts`.
- `^FB` and `^TB` text blocks, `^FP` direction, `^FH` hex escapes, `^CI`
  code pages 0–36 with remapping, `^SE` encoding tables.
- Downloaded fonts (`~DU`, `~DT`, `~DY`, `~DB`), `^CW` aliases and `^FL`
  font links.
- `^PA` advanced layout: bidirectional text and HarfBuzz shaping for Arabic,
  Hebrew and Indic scripts (via go-text/typesetting).
- `Options.FallbackFonts` for characters no ZPL font has, and the optional
  `fonts/notocjk` module with Noto Sans CJK.

### Graphics
- `^GB` (incl. rounding), `^GC`, `^GE`, `^GD`, `^GS`.
- `^GF`, `~DG`, `~DY` with hex, ACS compression, `:B64:` and `:Z64:` data;
  PNG, BMP and PCX downloads; `^XG`, `^IM`, `^IL`, `^IS`, `^ID`, `^TO`.

### Barcodes
- Every Zebra symbology, implemented in this repository and checked with
  independent decoders: Code 11, 2 of 5 (interleaved, industrial, standard),
  Code 39 (incl. full ASCII, LOGMARS), Code 93, Code 128 (modes N/U/A/D),
  EAN-8/13, UPC-A/E and add-ons, Codabar, MSI, Plessey, POSTNET, PLANET,
  USPS Intelligent Mail, QR (models 1 and 2), Data Matrix ECC 200, Aztec,
  MaxiCode, PDF417 (incl. Macro PDF417 with `^FM`), MicroPDF417, CODABLOCK
  A/E/F, Code 49, TLC39, GS1 DataBar and GS1 Composite.

### Formats and copies
- `^DF`/`^XF` stored formats with `^FN` fields and `^FE` insertion.
- `^SN`/`^SF` serialisation and `^PQ` copies with `Options.ExpandCopies`;
  `Printer.RenderEach` streams labels without holding them in memory.
- `^FC` real-time clock fields with `Options.Now`.

### Robustness and performance
- Untrusted input is bounded: label and field sizes, decompressed graphics,
  `^XF` nesting, copy counts and total output (`Options.MaxPixels`); internal
  errors are returned as `ErrInternal` instead of crashing.
- `Printer` is safe for concurrent use.
- About 1.4 ms to render a typical 4×6 shipping label and 2 ms including
  PNG encoding; `Options.PNGCompression` trades speed for file size.

### Verified against a printer
- A comparison harness renders fixtures on a real Zebra printer (stored with
  `^IS`, read back with `^HG`, never printed) and diffs them with this
  library. On a ZD420 (300 dpi) it settled behaviour the guide leaves open:
  QR EC level and mask handling, the PDF417 automatic shape, MaxiCode
  geometry and padding, persistent `^CF`/`^BY`/`^FW`, `^TB` line spacing,
  GS1-128 text and Code 128 auto-mode digit runs.

### Known limitations
- Data Matrix ECC 000–140 is encoded as ECC 200 (the legacy format is not
  publicly specified); `^BX` without a quality, which printers render as
  ECC 000, returns a warning.
- CODABLOCK row structure and GS1 Composite layout differ from a ZD420's
  output (the encoded data is the same).
- `^GF`/`~DY` format C (AR-compressed) and `~DS` Intellifont downloads are
  proprietary and not decoded.

[Unreleased]: https://github.com/dchupp/DataMagik-ZPL-To-Image/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/dchupp/DataMagik-ZPL-To-Image/releases/tag/v0.1.0
