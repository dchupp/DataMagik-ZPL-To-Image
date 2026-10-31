# zpl-to-image

A pure-Go renderer that turns Zebra **ZPL II** label formats into PNG images.

```go
import zpl "github.com/dchupp/DataMagik-ZPL-To-Image"

pngs, warnings, err := zpl.RenderPNG(strings.NewReader(`^XA
^FO50,50^A0N,50^FDHello, label^FS
^FO50,120^BY3^BCN,100,Y,N,N^FD123456789^FS
^FO400,120^BQN,2,6^FDQA,https://example.com^FS
^XZ`), zpl.Options{DPMM: 8, LabelWidth: 4, LabelHeight: 6})
```

- A `Printer` is safe for concurrent use; it runs one job at a time like a real printer, so use several `Printer`s to render in parallel.
- One image per `^XA…^XZ` format (or one per `^PQ` copy with `ExpandCopies`).
- `Render`/`RenderPNG` hold all images in memory and stop with `ErrTooMuchOutput` beyond `Options.MaxPixels` (default 256 Mpx). `Printer.RenderEach` streams labels to a callback with no total limit; the CLI uses it.
- Untrusted input is bounded:
  - Labels over 64 Mpx return `ErrLabelTooLarge`.
  - Fields and graphics larger than the label are dropped.
  - `^XF` nesting stops at 8 levels.
  - `^PQ` expansion stops at 10,000 copies.
- `zpl.NewPrinter` keeps printer memory between calls, as a real printer does: `~DG`/`~DY` graphics, `^DF` formats, `~DU`/`~DB` fonts, and persistent settings such as `^CI`, `^LH`, `^PW`, `^PO`, `^MC`.
- `Warning`s report anything accepted but not reproducible exactly, and unknown commands. `Strict` turns unknown commands into errors.

CLI:

```
go run ./cmd/zpl2png -dpmm 8 -w 4 -h 6 -o label label.zpl   # writes label-1.png, …
```

## Dependencies

- `golang.org/x/image`: glyph rasterisation and BMP decoding.
- `golang.org/x/text`: code page tables for `^CI`.
- `github.com/go-text/typesetting` (BSD-3, pure Go): font parsing (TTF, OTF/CFF, CFF2, TTC, variable fonts), HarfBuzz shaping and the Unicode bidi algorithm.

Every barcode symbology is implemented here. `internal/verify` is a separate module that uses gozxing for decode tests, so it is not a dependency of the library.

## Complex scripts and CJK

`^PA` turns on Zebra's advanced text layout: b = bidirectional ordering, c = character shaping, d = OpenType features. With it, Arabic and Hebrew are ordered right-to-left and joined, and Indic scripts form conjuncts and reorder vowel signs. Shaping applies to single-line fields and `^TB` blocks; `^FB` does not support complex text on Zebra printers either.

The built-in look-alike fonts cover Latin, Greek and Cyrillic (the bitmap fonts also Hebrew). For other scripts, pass fonts that cover them as `Options.FallbackFonts`; they are tried in order for any character the selected ZPL font lacks. For Chinese, Japanese and Korean, add the optional `fonts/notocjk` module (Noto Sans CJK, SIL OFL, ~16 MB). It is a separate Go module, so the library's own download stays small:

```go
// go get github.com/dchupp/DataMagik-ZPL-To-Image/fonts/notocjk
import "github.com/dchupp/DataMagik-ZPL-To-Image/fonts/notocjk"

opts := zpl.Options{FallbackFonts: [][]byte{notocjk.Regular}}
```

## Performance

A typical 4×6-inch shipping label (`testdata/samples/shipping_label.zpl`: text, boxes, MaxiCode, two Code 128s and a QR code) takes about 1.7 ms to render and 4 ms including PNG encoding, on a modern x86 core. Rendered labels use a dedicated 1-bit PNG encoder. Glyphs are cached per font, so a long-lived `Printer` gets faster as it warms up. Run `go test -bench . -run XXX .` to measure.

## Coverage

Every command in the Zebra ZPL II Programming Guide index has a handler. `internal/engine/coverage_test.go` enforces this. Commands that only drive hardware (media, sensors, network, RFID, host status) are accepted as no-ops because they don't change the printed image.

| Area | Commands |
|---|---|
| Fields & layout | `^FO ^FT` (incl. chained `^FT` and z-justification), `^FD ^FV ^FS ^FH ^FB ^TB ^FP ^FR ^FW ^FN ^FE ^FM ^FX ^LH ^LL ^PW ^LS ^LT ^LR ^PO ^PM ^JM ^MC` |
| Text | `^A ^A@ ^CF ^CW ^FL ^PA ^CI` (0–36 with remapping) `^SE ~DE ^GS ~DB ~DU ~DT ~DY`(TTF/OTF) |
| Graphics | `^GB` (incl. rounding) `^GC ^GE ^GD ^GF` (A/B, ACS, `:B64:`/`:Z64:` + CRC) `~DG ~DY` (GRF/PNG/BMP/PCX) `^XG ^IM ^IL ^IS ^ID ~EG ^TO` |
| Formats & serialisation | `^DF ^XF ^FN ^SN ^SF ^PQ` (copies/replicates) `^FC ^SO ^ST` |
| Linear barcodes | Code 11, Interleaved/Industrial/Standard 2 of 5, Code 39 (+ full ASCII, LOGMARS), Code 93, Code 128 (modes N/U/A/D, invocation codes), EAN-8/13, UPC-A/E, UPC/EAN add-ons, Codabar, MSI, Plessey, POSTNET, PLANET, USPS Intelligent Mail |
| 2D & stacked | QR (models 1 & 2, mixed modes, Kanji, structured append), Data Matrix ECC 200 (square/rectangular, escapes, GS1), Aztec (compact, full, rune, menu), MaxiCode (modes 2–6), PDF417 (incl. truncated and Macro PDF417 with `^FM`), MicroPDF417, CODABLOCK A/E/F, Code 49, TLC39, GS1 DataBar and GS1 Composite (every `^BR` type 1–12: CC-A/B/C with DataBar, UPC/EAN and GS1-128) |

## Fidelity

- **Fonts:** Zebra's fonts are proprietary, so the package embeds open look-alikes:
  - **Roboto Condensed Bold** for font 0 and P–V. It is calibrated against reference renders: caps are 0.75 h tall and start at the field origin.
  - **Liberation Mono** for the bitmap fonts A–H, drawn into each font's native dot matrix with Zebra's integer magnification.

  Both are SIL OFL 1.1; licences are in `internal/font/data`. If you are licensed for the real fonts, pass them with `Options.Fonts`, or download them with `~DU`/`~DY`.
- **Interpretation lines** follow printer behaviour:
  - Default: font A magnified by the `^BY` module width, centred below the bars.
  - A preceding `^A` font is used as given and left-aligned.
  - UPC/EAN digits are laid out between the guard bars.
- **Calibration against Labelary:** QR placement (10 dots below the origin), QR EC-level upgrading and mask choice, `^GD` geometry, MaxiCode size, and `^B7`/`^BF`/`^BR` height units were checked against Labelary's renders. QR codes and four of the five DataBar types match pixel for pixel.

## Known limitations

- **Data Matrix ECC 000–140:** the legacy convolutional variants aren't publicly specified. `^BX` without a quality parameter is encoded as ECC 200 (Zebra's documented default is 0); an explicit quality below 200 is encoded as ECC 200 with a warning.
- **Proprietary compressed formats:** `^GF` format C and `~DY` format C (AR-compressed) are treated as raw binary. `~DS` Intellifont downloads fall back to font 0.
- **Script coverage:** shaped output is only as good as the fonts supplied; other scripts need `Options.FallbackFonts` (or `~DU`/`~DY` fonts).
- **`^CI26`:** decodes as UTF-8 when the data is valid UTF-8, otherwise as GB18030.
- **Undocumented Zebra behaviour:** where Zebra's own behaviour isn't documented (exact Code 128 auto-mode subset choices, PDF417 automatic aspect ratio, TLC39 2D content), the encoders follow the relevant ISO/AIM specification. Each package documents its choice.
- **Clock default:** `^FC` uses `time.Now` unless `Options.Now` is set.

## Testing

```
go test ./...                                   # unit, feature and golden-image tests
go test -run TestGolden -update .               # regenerate testdata/golden/*.png after intended changes
(cd internal/verify && go test ./...)           # decode round-trips (gozxing) for encoders and full labels
ZPL_LABELARY=1 go test -tags labelary -run Labelary -v ./internal/verify   # optional: diff golden fixtures against api.labelary.com
go test -fuzz FuzzRender -fuzztime 60s .        # fuzz the whole pipeline
go test -fuzz . -fuzztime 30s ./internal/barcode/qr   # fuzz one encoder (each 2D/stacked encoder and Code 128 has a fuzz test)
```

Golden fixtures named `name@DPMM.zpl` render at that density (6, 8, 12 or 24 dots/mm); the rest render at 8. The Labelary harness sends only the files in `testdata/golden` to Labelary's public API, and runs only when explicitly enabled.

To test against your own labels, drop `.zpl` files into `testdata/golden` and run with `-update`. Keep real customer data out of the repository.
