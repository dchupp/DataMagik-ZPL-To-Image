// Package barcode draws barcode symbols as images, using the same encoders
// as the ZPL renderer.
//
// Encode returns the bare symbol: black (0) modules on white (255), no
// quiet zone and no human readable text. Callers scale the image to the size
// they need (nearest neighbour keeps the edges sharp) and recolour it.
//
//	img, err := barcode.Encode("qr", "https://example.com", barcode.Options{ECLevel: "H"})
//
// # Size
//
// One module (the narrow bar or space of a linear symbol, one cell of a 2D
// symbol) is ModuleWidth pixels, 1 by default. Linear and postal symbols are
// Height modules high (1 by default for linear symbols, so the image is one
// pixel high and meant to be stretched). Stacked symbols (PDF417,
// MicroPDF417, CODABLOCK-F, Code 49) use RowHeight modules per row, and
// GS1 DataBar keeps its ISO heights. MaxiCode, whose hexagons need several
// pixels each, is drawn at 10×ModuleWidth pixels per module column (about
// 300×290 pixels at ModuleWidth 1).
//
// # Data
//
// Data is the text to encode. Symbologies limited to digits, or to a
// character set, return an error wrapping ErrInvalidData for anything else.
// GS1 data (gs1-128, gs1-datamatrix, gs1-qr, DataBar Expanded, composite 2D
// components) is written as an element string with parenthesised AIs:
// "(01)09501101530003(10)ABC123". Composite symbols take the linear and 2D
// data separated by '|': "(01)09501101530003|(10)ABC123".
package barcode

import (
	"errors"
	"fmt"
	"image"
	"strings"

	ibarcode "github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/bitmap"
)

var (
	// ErrUnknownSymbology is returned by Encode for a name Symbologies does
	// not list (as a name or alias).
	ErrUnknownSymbology = errors.New("barcode: unknown symbology")
	// ErrInvalidData is wrapped by errors for data the symbology cannot
	// encode (bad characters, wrong length, too long for the symbol).
	ErrInvalidData = ibarcode.ErrInvalidData
	// ErrCheckDigit is returned (wrapping ErrInvalidData too) when EAN/UPC
	// data includes a check digit that is wrong.
	ErrCheckDigit = errors.New("barcode: check digit mismatch")
	// ErrInvalidOption is wrapped by errors for an option out of range.
	ErrInvalidOption = errors.New("barcode: invalid option")
)

// Options adjust a symbol. The zero value gives sensible defaults for every
// symbology; options that do not apply to the requested symbology are
// ignored. Info.Options lists the options each symbology honours.
type Options struct {
	// ModuleWidth is the size of one module in pixels, 1–100 (0 means 1).
	ModuleWidth int

	// Height is the bar height in modules of linear and postal symbols, and
	// of the linear part of composite symbols (0–10000). 0 means 1 for
	// linear symbols; 10 for POSTNET and PLANET and 9 for IMb (so the short
	// bars are 2/5 and 1/3 of the height); for composites 74 (UPC-A,
	// UPC-E, EAN-13), 60 (EAN-8) or 25 (GS1-128), as a Zebra printer draws
	// them.
	Height int

	// Ratio is the wide:narrow ratio of two-width symbologies (Code 39,
	// LOGMARS, Code 11, Codabar, ITF, industrial and standard 2 of 5, MSI,
	// Plessey, TLC39), 2.0–3.0 (0 means 3.0, the ZPL ^BY default). A wide
	// element is floor(Ratio×ModuleWidth) pixels, the ratio taken to
	// tenths, as a Zebra printer draws it: at ModuleWidth 1 every ratio
	// below 3.0 gives 2.
	Ratio float64

	// CheckDigit adds the optional check character: Mod 43 for Code 39,
	// Mod 10 for ITF, a Mod 10 (GS1) check digit for Code 128. Symbologies
	// with mandatory check characters always have them.
	CheckDigit bool

	// GS1 encodes the data as a GS1 element string: FNC1 first for Code 128
	// (GS1-128), Data Matrix (GS1 DataMatrix) and QR Code (GS1 QR), and
	// CODABLOCK-F becomes CODABLOCK-E (FNC1 first).
	GS1 bool

	// ECLevel is the QR Code error correction level: "L", "M", "Q" or "H"
	// (empty means "M").
	ECLevel string

	// ECPercent is the Aztec error correction as a percentage of the symbol,
	// 1–99 (0 means 23, the ISO recommendation). Ignored when Layers is set.
	ECPercent int

	// Layers forces the Aztec symbol size: 1–4 with Compact, else 1–32.
	// 0 picks the smallest symbol that fits.
	Layers int

	// Compact selects a compact Aztec symbol (with Layers), or Compact
	// (truncated) PDF417, which drops the right row indicator.
	Compact bool

	// Security is the PDF417 error correction level 1–8 (2^(level+1)
	// check codewords). 0 picks the ISO/IEC 15438 recommended level for the
	// data length (2–5). Level 0, which only detects errors, is not offered.
	Security int

	// Columns and Rows set the symbol size, 0 meaning automatic:
	//   - PDF417: data columns 1–30 and rows 3–90.
	//   - MicroPDF417: columns 1–4 (Rows is ignored; the fewest rows that
	//     hold the data are used).
	//   - CODABLOCK-F: characters per row 4–62 and rows 2–44.
	//   - Data Matrix: the symbol's module columns and rows (10–144);
	//     setting one picks the smallest symbol with that dimension.
	//   - GS1 DataBar Expanded Stacked: Columns is the segments per row,
	//     2–22 and even (0 means 4).
	Columns, Rows int

	// Rectangular selects a rectangular Data Matrix symbol when the data
	// fits one.
	Rectangular bool

	// RowHeight is the row height in modules of stacked symbols, 1–100:
	// PDF417 (0 means 3), MicroPDF417 (0 means 2), CODABLOCK-F (0 means 8),
	// Code 49 (0 means 8), and the MicroPDF417 part of TLC39 (0 means 2).
	RowHeight int

	// Mode is the MaxiCode mode, 2–6 (0 means 4, a standard symbol for any
	// data). Modes 2 and 3 are structured carrier messages; their data is
	// written as for ZPL ^BD: class of service, country code and postal
	// code ("001840152382802" for mode 2) followed by the secondary
	// message, or a "[)>␞01␝96…" ANSI MH10.8.3 message.
	Mode int
}

// Info describes a symbology.
type Info struct {
	// Name is the canonical name to pass to Encode.
	Name string
	// Aliases are other names Encode accepts.
	Aliases []string
	// TwoD is true for 2D matrix, stacked and composite symbologies, whose
	// image height is set by the symbol rather than by Options.Height
	// alone. Single-row GS1 DataBar is linear but keeps its ISO height.
	TwoD bool
	// Description is a short human readable description.
	Description string
	// Options names the Options fields the symbology honours, besides
	// ModuleWidth.
	Options []string
}

// Symbologies returns every supported symbology, in a stable order.
func Symbologies() []Info {
	out := make([]Info, len(symbologies))
	for i, s := range symbologies {
		out[i] = s.Info
		out[i].Aliases = append([]string(nil), s.Aliases...)
		out[i].Options = append([]string(nil), s.Options...)
	}
	return out
}

// Encode draws data as the named symbology. Names are matched without
// regard to case, spaces, hyphens and underscores, so "GS1-128", "gs1_128"
// and "gs1128" are the same.
//
// The image has one pixel per module at the default ModuleWidth, no quiet
// zone and no human readable text. EAN-13, EAN-8, UPC-A and UPC-E compute
// their check digit as a Zebra printer does; when the data already includes
// one (13, 8, 12 or 8 digits respectively) it must be correct or Encode
// returns ErrCheckDigit.
func Encode(symbology, data string, o Options) (*image.Gray, error) {
	s, ok := lookup[normalize(symbology)]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownSymbology, symbology)
	}
	r, err := resolve(o)
	if err != nil {
		return nil, err
	}
	bm, err := s.encode(data, r)
	if err != nil {
		return nil, err
	}
	if bm.W == 0 || bm.H == 0 {
		return nil, fmt.Errorf("barcode: %s: %w: image larger than %d pixels", s.Name, ErrInvalidOption, bitmap.MaxPixels)
	}
	return gray(bm), nil
}

func normalize(name string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '-', '_', ' ':
			return -1
		}
		if r >= 'A' && r <= 'Z' {
			return r + 'a' - 'A'
		}
		return r
	}, name)
}

// opts is Options with defaults applied.
type opts struct {
	Options
	mw int // ModuleWidth
}

func resolve(o Options) (opts, error) {
	bad := func(name string, v any) (opts, error) {
		return opts{}, fmt.Errorf("%w: %s %v out of range", ErrInvalidOption, name, v)
	}
	switch {
	case o.ModuleWidth < 0 || o.ModuleWidth > 100:
		return bad("ModuleWidth", o.ModuleWidth)
	case o.Height < 0 || o.Height > 10000:
		return bad("Height", o.Height)
	case o.Ratio != 0 && (o.Ratio < 2 || o.Ratio > 3):
		return bad("Ratio", o.Ratio)
	case o.ECPercent < 0 || o.ECPercent > 99:
		return bad("ECPercent", o.ECPercent)
	case o.Security < 0 || o.Security > 8:
		return bad("Security", o.Security)
	case o.Layers < 0 || o.Layers > 32:
		return bad("Layers", o.Layers)
	case o.Columns < 0 || o.Rows < 0:
		return bad("Columns/Rows", fmt.Sprint(o.Columns, "/", o.Rows))
	case o.RowHeight < 0 || o.RowHeight > 100:
		return bad("RowHeight", o.RowHeight)
	case o.Mode != 0 && (o.Mode < 2 || o.Mode > 6):
		return bad("Mode", o.Mode)
	}
	switch o.ECLevel {
	case "", "L", "M", "Q", "H", "l", "m", "q", "h":
	default:
		return bad("ECLevel", o.ECLevel)
	}
	r := opts{Options: o, mw: max(o.ModuleWidth, 1)}
	if r.Ratio == 0 {
		r.Ratio = 3
	}
	return r, nil
}

// height returns Height (or def when 0) in pixels.
func (o opts) height(def int) int {
	if o.Height > 0 {
		return o.Height * o.mw
	}
	return def * o.mw
}

// rowHeight returns RowHeight (or def when 0) in modules.
func (o opts) rowHeight(def int) int {
	if o.RowHeight > 0 {
		return o.RowHeight
	}
	return def
}

// gray converts a bitmap to black-on-white grey pixels.
func gray(bm *bitmap.Bitmap) *image.Gray {
	img := image.NewGray(image.Rect(0, 0, bm.W, bm.H))
	for y := 0; y < bm.H; y++ {
		row := img.Pix[y*img.Stride : y*img.Stride+bm.W]
		for x := range row {
			if !bm.Get(x, y) {
				row[x] = 255
			}
		}
	}
	return img
}
