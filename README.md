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
- `Options.Rotation` (0, 90, 180 or 270) turns every label image clockwise, like Labelary's `X-Rotation` header; other values return `ErrInvalidRotation`.

## Barcodes without ZPL

The `barcode` package draws a single symbol with the same encoders, for callers that place barcodes themselves (e.g. in a document designer):

```go
import "github.com/dchupp/DataMagik-ZPL-To-Image/barcode"

img, err := barcode.Encode("qr", "https://example.com", barcode.Options{ECLevel: "H"}) // *image.Gray
```

```go
func Encode(symbology, data string, o Options) (*image.Gray, error)
func Symbologies() []Info // Name, Aliases, TwoD, Description, Options
var ErrUnknownSymbology, ErrInvalidData, ErrCheckDigit, ErrInvalidOption error
```

- The image is black (0) on white (255), one pixel per module (`ModuleWidth` scales it), with no quiet zone and no human readable text. Scale it with nearest neighbour and recolour it as needed.
- Linear symbols are one module high unless `Options.Height` (in modules) is set; stacked symbols use `RowHeight`; GS1 DataBar keeps its ISO heights; MaxiCode is drawn at 10 pixels per module column.
- Names are matched ignoring case, spaces, `-` and `_`. Symbologies: `code128`, `gs1-128`, `code39`, `logmars`, `code93`, `code11`, `codabar`, `itf`, `industrial2of5`, `standard2of5`, `msi`, `plessey`, `ean13`, `ean8`, `upca`, `upce`, `postnet`, `planet`, `imb`, `qr`, `gs1-qr`, `datamatrix`, `gs1-datamatrix`, `aztec`, `pdf417`, `micropdf417`, `maxicode`, `codablock-f`, `code49`, `tlc39`, the GS1 DataBar family (`databar`, `-truncated`, `-stacked`, `-stacked-omni`, `-limited`, `-expanded`, `-expanded-stacked`) and composites (`upca-cc`, `upce-cc`, `ean13-cc`, `ean8-cc`, `gs1-128-cc`, `gs1-128-ccc`; data `linear|2D`).
- Options (each ignored where it does not apply): `Ratio` (wide:narrow, 2.0–3.0, default 3.0), `CheckDigit` (Code 39, ITF, Code 128), `GS1` (Code 128, Data Matrix, QR, CODABLOCK), `ECLevel` (QR L/M/Q/H, default M), `ECPercent`/`Layers`/`Compact` (Aztec), `Security` (PDF417 1–8, 0 = automatic), `Columns`/`Rows` (PDF417, MicroPDF417, CODABLOCK-F, Data Matrix, DataBar Expanded Stacked), `Rectangular` (Data Matrix), `Mode` (MaxiCode, default 4).
- EAN/UPC compute their check digit; data that includes one must be correct (`ErrCheckDigit`). UPC-E also takes 6–8 digit UPC-E forms. QR and Aztec add ECI 26 for non-ASCII UTF-8 data.

CLI:

```
go run ./cmd/zpl2png -dpmm 8 -w 4 -h 6 -o label label.zpl   # writes label-1.png, …
```

## Rescaling and tokenizing

`Rescale` rewrites ZPL written for one density so it prints the same physical size at another, for example a template designed at 8 dots/mm (203 dpi) sent to a 12 dots/mm (300 dpi) printer:

```go
out, warnings, err := zpl.Rescale(src, 8, 12)
```

Dot-valued parameters are multiplied by `to/from` and rounded; everything else (field data, comments, whitespace, `^CC`/`^CD` changes, untouched commands) is copied byte for byte. Each limitation below is returned as a `Warning` with the byte offset of the command.

| Commands | Treatment |
|---|---|
| `^FO` `^FT` `^LH` `^FM`, `^LS` `^LT` (clamped to ±9999, ±240), `^PW` `^LL` (≤ 32000), `^ML`, `^PF` `~TA`, `^MN` offset, `^RS` position in dots (`F`n or n; `B`n mm is kept) | scaled |
| `^GB` `^GE` `^GD` width, height, thickness; `^GC` diameter, thickness; `^FB` width, line spacing, indent; `^TB` `^GS` `^FP` gap | scaled (`^GB` rounding is an index and is kept) |
| `^A` `^A@` `^CF` height and width | scaled; a `^A` with no size and no `^CF` gets the scaled power-up default (9×5 dots) written out; bitmap fonts A–H warn when whole magnifications can't come within 15% |
| `^BY` module width (1–10) and height; every `^B*` height or row height (`^B7` `^BF` `^BB` rows in dots) | scaled; module widths warn beyond 15% rounding error or when clamped |
| `^BQ` `^BO`/`^B0` `^BR` magnification, `^BX` module size, `^BT` widths and heights | scaled within range; density-dependent defaults are written out when the target's default differs |
| `^B4` row multiplier, `^BR` separator and composite heights (in modules), `^BY` ratio, `^BD` (fixed 25 mm) | unchanged |
| `^GF`, `~DG`, `~DY` GRF and PNG | decoded (hex, ACS, `:B64:`, `:Z64:`, binary), resampled nearest-neighbour, re-encoded (hex, or `:Z64:` for compressed or binary sources) with new byte counts |
| `^XG` of a graphic not in the input | magnification multiplied when `to/from` is whole, warned otherwise; `^IM`/`^IL`/`^XF` of objects not in the input warn |
| `^DF` stored formats | rescaled like the rest of the stream |
| `^MU` | after `^MU I`/`^MU M` measurements are physical and not scaled; a dpi conversion is left in place and warned |
| `~DB` bitmap fonts, BMP/PCX downloads, format C data | unchanged, warned |
| Fields that rely on the power-up `^CF` or `^BY` sizes | unchanged, warned |
| `^PR` `^MD` `^PQ` `^SN` `^FW` `^CI`, RFID, media, network and host commands | unchanged |

`Tokenize` exposes the renderer's lexer for tools such as label importers: it returns each command's canonical prefix and name, parameters, field data or payload, and its `[Offset, End)` span in the source, so commands a tool cannot model can be kept verbatim.

```go
cmds, err := zpl.Tokenize(strings.NewReader(src))
for _, c := range cmds {
    raw := src[c.Offset:c.End] // exactly as written
    _ = raw
}
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

A typical 4×6-inch shipping label (`testdata/samples/shipping_label.zpl`: text, boxes, MaxiCode, two Code 128s and a QR code) takes about 0.6 ms to render and 1.5 ms including PNG encoding, on a modern x86 core, and about 450 KB of memory. Labels are kept packed at one bit per pixel. Rendering scales across cores: one `Printer` per goroutine reaches about 6,000–7,000 labels per second on a 12-core laptop CPU (`BenchmarkParallelShippingLabel`). Rendered labels use a dedicated 1-bit PNG encoder; `Options.PNGCompression` trades speed for size (`PNGFast`, the default, is about twice as fast as `PNGBalanced` for files ~20% larger). With `ExpandCopies`, fields that do not change between `^PQ` copies are rendered once and reused. Glyphs are cached per font, so a long-lived `Printer` gets faster as it warms up. Run `go test -bench . -run XXX .` to measure.

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
- **Verified against a real printer:** `internal/verify` can compare renders with a Zebra printer's own output (stored with `^IS`, read back with `^HG`, nothing printed). On a ZD420 at 300 dpi, CODABLOCK, GS1 DataBar, GS1 Composite and TLC39 match pixel for pixel; QR, PDF417, MaxiCode, graphics, interpretation lines and Code 128 match to within about 0.3–5% of ink area, mostly glyph-shape differences from the substitute fonts. Behaviour learned from the printer, where it differs from the guide or Labelary:
  - `^BQ` encodes exactly the requested error-correction level and ignores the mask parameter.
  - PDF417 picks columns so rows ÷ (columns + 4) is closest to 20/7; `^B7`, `^BF` and `^BB` row heights are in dots.
  - MaxiCode is drawn on a whole-dot hexagon grid and ends its data with codeword 63 before padding.
  - `^CF`, `^BY` and `^FW` persist across labels.
  - `^TB` steps font-0 lines by 1.25× the height.
  - GS1-128 text prints as typed.
  - Code 128 auto mode puts an odd digit after the subset C pairs.
  - `^BR` data in parentheses is encoded as the literal text, parentheses included, without parsing the AIs (DataBar Expanded, the GS1-128 of types 11–12 and the 2D components). Scanners therefore return "(01)…" as text, not as a GS1 element string; leave out the parentheses for real GS1 data.
  - `^BR` types 7–12 start the linear symbol 7 (UPC/EAN) or 10 (GS1-128) modules right of the field origin. UPC/EAN bars ignore the height parameter (74 modules, 60 for EAN-8) and sit under a 6-module separator. In composites, Code 128 prefers subset A, and there is no linkage character without a 2D part.
  - PDF417 and MicroPDF417 use Numeric Compaction for runs of 14 or more digits (all-digit MicroPDF417 data always, all-digit PDF417 data from 13). A single punctuation character between letters is shifted, not latched. MicroPDF417 pads with a fixed cycle of Text Compaction latches instead of 900s.
  - TLC39 separates fields with `*` in the MicroPDF417, which is at least 4×6 and sized from the field length. The MicroPDF417 sits one module above the bars, and the flag extends above and below them.
- **Calibration against Labelary:**

## Known limitations

- **Data Matrix ECC 000–140:** the legacy convolutional variants aren't publicly specified. Printers print ECC 000 when `^BX` has no quality parameter; this library renders ECC 200 instead and returns a warning (add quality 200 to match the printer).
- **Printer bug not reproduced:** a ZD420 encodes unbracketed DataBar Expanded data with AI 01 followed by 320x as if it were 393x, so scanners read different data. This library encodes the data correctly.
- **Proprietary compressed formats:** `^GF` format C and `~DY` format C (AR-compressed) are treated as raw binary. `~DS` Intellifont downloads fall back to font 0.
- **Script coverage:** shaped output is only as good as the fonts supplied; other scripts need `Options.FallbackFonts` (or `~DU`/`~DY` fonts).
- **`^CI26`:** decodes as UTF-8 when the data is valid UTF-8, otherwise as GB18030.
- **Undocumented Zebra behaviour:** where neither Zebra's documentation nor a printer measurement settles a choice, the encoders follow the relevant ISO/AIM specification. Each package documents its choice.
- **Clock default:** `^FC` uses `time.Now` unless `Options.Now` is set.

## Testing

```
go test ./...                                   # unit, feature and golden-image tests
go test -run TestGolden -update .               # regenerate testdata/golden/*.png after intended changes
(cd internal/verify && go test ./...)           # decode round-trips (gozxing) for encoders and full labels
ZPL_LABELARY=1 go test -tags labelary -run Labelary -v ./internal/verify   # optional: diff golden fixtures against api.labelary.com
ZPL_PRINTER=host:9100 go test -tags zebra -run Zebra -v ./internal/verify  # optional: compare against a real Zebra printer (nothing is printed)
go test -fuzz FuzzRender -fuzztime 60s .        # fuzz the whole pipeline
go test -fuzz FuzzRescale -fuzztime 60s .       # fuzz Rescale (also FuzzTokenize)
go test -fuzz . -fuzztime 30s ./internal/barcode/qr   # fuzz one encoder (each 2D/stacked encoder and Code 128 has a fuzz test)
go test -run XXX -fuzz FuzzEncode -fuzztime 60s ./barcode   # fuzz the public barcode package
```

Golden fixtures named `name@DPMM.zpl` render at that density (6, 8, 12 or 24 dots/mm); the rest render at 8. The Labelary harness sends only the files in `testdata/golden` to Labelary's public API, and runs only when explicitly enabled.

To test against your own labels, drop `.zpl` files into `testdata/golden` and run with `-update`. Keep real customer data out of the repository.
