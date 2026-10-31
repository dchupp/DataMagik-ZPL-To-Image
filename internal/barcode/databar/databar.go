// Package databar encodes the GS1 DataBar family (ISO/IEC 24724:2011) as
// selected by the ZPL ^BR command.
//
// Zebra's ^BRa,b,c,d,e,f symbology type b maps to:
//
//	1  GS1 DataBar Omnidirectional   -> Omnidirectional / Encode(TypeOmnidirectional)
//	2  GS1 DataBar Truncated         -> Truncated
//	3  GS1 DataBar Stacked           -> Stacked
//	4  GS1 DataBar Stacked Omni      -> StackedOmnidirectional
//	5  GS1 DataBar Limited           -> Limited
//	6  GS1 DataBar Expanded          -> Expanded / ExpandedStacked (f < segments needed)
//	7  UPC-A, 8 UPC-E, 9 EAN-13, 10 EAN-8, 11 UCC/EAN-128 & CC-A/B, 12 UCC/EAN-128 & CC-C
//
// Types 7–12 are not DataBar: they are the linear parts of GS1 Composite
// symbols and are produced by the UPC/EAN and Code 128 encoders (with the
// GS1-128 FNC1 start for 11/12); the renderer must encode the part after '|'
// as a CC-A/B (types 7–11) or CC-C (type 12) composite component above the
// linear symbol, separated by the composite separator row(s). Their ^BR
// e parameter (bar height, dots) applies only to that linear part.
//
// ZPL field data for ^BR has the form "linear|composite"; use
// SplitComposite and set Options.Linkage when a composite part is present so
// the linear symbol carries the 2D linkage flag.
//
// Units: all widths and heights are in modules (X). Zebra's ^BR c
// (magnification) is the module width in dots. Heights are the nominal ISO
// heights (Omni 33X, Truncated 13X, Stacked 5X/7X, Stacked Omni 33X rows,
// Limited 10X, Expanded 34X rows); separator rows are 1X high and are listed
// in Symbol.SeparatorRows so the renderer can apply ^BR d (separator height).
package databar

import (
	"fmt"
	"strings"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/gs1"
)

// Type is a ^BR symbology type (only the DataBar types 1–6).
type Type int

// DataBar symbology types, numbered as Zebra's ^BR b parameter.
const (
	TypeOmnidirectional        Type = 1
	TypeTruncated              Type = 2
	TypeStacked                Type = 3
	TypeStackedOmnidirectional Type = 4
	TypeLimited                Type = 5
	TypeExpanded               Type = 6
)

// Options control DataBar encoding.
type Options struct {
	// Linkage sets the 2D composite linkage flag: a CC-A/B composite
	// component will be printed above the symbol (ZPL data contained '|').
	Linkage bool

	// ZebraParentheses encodes Expanded data containing parentheses as the
	// literal text, in general-purpose mode, instead of parsing the AIs.
	// Zebra printers (checked on a ZD420) do this, so "(01)…" is scanned as
	// the characters "(01)…" rather than as a GS1 element string.
	ZebraParentheses bool

	// Segments is Zebra's ^BR f: Expanded segments (symbol characters) per
	// row, 2–22 and even. 0 means 22 (single row for all valid data).
	// Fewer segments than the data needs produce GS1 DataBar Expanded
	// Stacked.
	Segments int
}

// Symbol is a fully laid out DataBar symbol.
type Symbol struct {
	// Matrix holds every module row of the symbol, including separator rows,
	// at its full nominal width (including the 1X light module of the left
	// guard). RowHeights are in modules (X).
	Matrix barcode.Matrix

	// SeparatorRows lists the indexes of separator rows in Matrix.
	SeparatorRows []int

	// Linear is set for single-row symbols. Its elements omit the light
	// guard module(s) at the left (always 1X) and right edges, which belong
	// to the quiet zone; LeftOffset gives the omitted left width.
	Linear     *barcode.Linear
	LeftOffset int

	// CompositeSeparator is the separator row (width Matrix.Width()) to put
	// directly between a composite component and the top row of the symbol.
	// Only set when Options.Linkage is true.
	CompositeSeparator []bool

	// Text is the human readable interpretation, e.g. "(01)00012345678905".
	Text string
}

// Height returns the nominal height in modules.
func (s Symbol) Height() int {
	h := 0
	for _, rh := range s.Matrix.RowHeights {
		h += rh
	}
	return h
}

// Encode encodes data as the given ^BR DataBar type. For TypeTruncated the
// geometry is the Omnidirectional one with a 13X height.
func Encode(t Type, data string, opt Options) (Symbol, error) {
	switch t {
	case TypeOmnidirectional, TypeTruncated, TypeStacked, TypeStackedOmnidirectional:
		return encodeOmni(t, data, opt.Linkage)
	case TypeLimited:
		return encodeLimited(data, opt.Linkage)
	case TypeExpanded:
		return encodeExpanded(data, opt)
	}
	return Symbol{}, fmt.Errorf("databar: type %d is not a DataBar type: %w", t, barcode.ErrInvalidData)
}

// Omnidirectional encodes GS1 DataBar Omnidirectional (^BR type 1): 96X
// wide (95X in Linear), 33X high. data is a GTIN (see Encode for forms).
func Omnidirectional(data string, opt Options) (barcode.Linear, error) {
	return linear(Encode(TypeOmnidirectional, data, opt))
}

// Truncated encodes GS1 DataBar Truncated (^BR type 2): Omnidirectional
// elements, 13X high.
func Truncated(data string, opt Options) (barcode.Linear, error) {
	return linear(Encode(TypeTruncated, data, opt))
}

// Limited encodes GS1 DataBar Limited (^BR type 5): 79X wide including the
// 1X left and 5X right light guards (73X in Linear), 10X high. The GTIN
// must start with 0 or 1 (indicator digit).
func Limited(data string, opt Options) (barcode.Linear, error) {
	return linear(Encode(TypeLimited, data, opt))
}

// Stacked encodes GS1 DataBar Stacked (^BR type 3): two 50X wide rows of
// 5X and 7X separated by a 1X separator row (RowHeights {5,1,7}).
func Stacked(data string, opt Options) (barcode.Matrix, error) {
	s, err := Encode(TypeStacked, data, opt)
	return s.Matrix, err
}

// StackedOmnidirectional encodes GS1 DataBar Stacked Omnidirectional (^BR
// type 4): two 33X rows with a three-row separator (RowHeights
// {33,1,1,1,33}).
func StackedOmnidirectional(data string, opt Options) (barcode.Matrix, error) {
	s, err := Encode(TypeStackedOmnidirectional, data, opt)
	return s.Matrix, err
}

// Expanded encodes single-row GS1 DataBar Expanded (^BR type 6 with the
// default 22 segments). data is a GS1 element string (see ParseElementString).
func Expanded(data string, opt Options) (barcode.Linear, error) {
	opt.Segments = 22
	return linear(Encode(TypeExpanded, data, opt))
}

// ExpandedStacked encodes GS1 DataBar Expanded Stacked with the given
// segments per row (2–22, even): 34X rows separated by three 1X separator
// rows (RowHeights {34,1,1,1,34,...}).
func ExpandedStacked(data string, segments int, opt Options) (barcode.Matrix, error) {
	opt.Segments = segments
	s, err := Encode(TypeExpanded, data, opt)
	return s.Matrix, err
}

// SplitComposite splits ZPL ^BR field data "linear|composite" into its parts.
func SplitComposite(data string) (linearPart, compositePart string, hasComposite bool) {
	return strings.Cut(data, "|")
}

func linear(s Symbol, err error) (barcode.Linear, error) {
	if err != nil {
		return barcode.Linear{}, err
	}
	if s.Linear == nil {
		return barcode.Linear{}, fmt.Errorf("databar: symbol is stacked: %w", barcode.ErrInvalidData)
	}
	return *s.Linear, nil
}

func invalid(format string, a ...any) error {
	return fmt.Errorf("databar: "+format+": %w", append(a, barcode.ErrInvalidData)...)
}

// combins returns n choose r.
func combins(n, r int) int {
	if r < 0 || n < r {
		return 0
	}
	if r > n-r {
		r = n - r
	}
	v := 1
	for i := 1; i <= r; i++ {
		v = v * (n - r + i) / i
	}
	return v
}

// getWidths is getRSSwidths() of ISO/IEC 24724 Annex B: the element widths
// of the val'th n-module, `elements`-element pattern whose elements are at
// most maxWidth wide. requireNarrow excludes patterns without a 1X element
// (the Annex's noNarrow == 0).
func getWidths(val, n, elements, maxWidth int, requireNarrow bool) []int {
	widths := make([]int, elements)
	narrowMask := 0
	bar := 0
	for ; bar < elements-1; bar++ {
		elmWidth := 1
		narrowMask |= 1 << bar
		var subVal int
		for {
			subVal = combins(n-elmWidth-1, elements-bar-2)
			if requireNarrow && narrowMask == 0 && n-elmWidth-(elements-bar-1) >= elements-bar-1 {
				subVal -= combins(n-elmWidth-(elements-bar), elements-bar-2)
			}
			if elements-bar-1 > 1 {
				lessVal := 0
				for mxw := n - elmWidth - (elements - bar - 2); mxw > maxWidth; mxw-- {
					lessVal += combins(n-elmWidth-mxw-1, elements-bar-3)
				}
				subVal -= lessVal * (elements - 1 - bar)
			} else if n-elmWidth > maxWidth {
				subVal--
			}
			val -= subVal
			if val < 0 {
				break
			}
			elmWidth++
			narrowMask &^= 1 << bar
		}
		val += subVal
		n -= elmWidth
		widths[bar] = elmWidth
	}
	widths[bar] = n
	return widths
}

// charWidths interleaves the odd (even positions) and even element widths
// of a character.
func charWidths(vOdd, vEven, nOdd, nEven, elements, maxOdd int, oddRequireNarrow bool) []int {
	odd := getWidths(vOdd, nOdd, elements, maxOdd, oddRequireNarrow)
	even := getWidths(vEven, nEven, elements, 9-maxOdd, !oddRequireNarrow)
	w := make([]int, 2*elements)
	for i := range elements {
		w[2*i] = odd[i]
		w[2*i+1] = even[i]
	}
	return w
}

// expand turns element widths into modules, the first element dark when
// dark is true.
func expand(widths []int, dark bool) []bool {
	var row []bool
	for _, w := range widths {
		for range w {
			row = append(row, dark)
		}
		dark = !dark
	}
	return row
}

// toLinear run-length encodes a module row, trimming light edge modules.
func toLinear(row []bool, text string) (*barcode.Linear, int) {
	start, end := 0, len(row)
	for start < end && !row[start] {
		start++
	}
	for end > start && !row[end-1] {
		end--
	}
	var el []int
	for i := start; i < end; {
		j := i
		for j < end && row[j] == row[i] {
			j++
		}
		el = append(el, j-i)
		i = j
	}
	return &barcode.Linear{Elements: el, Text: text}, start
}

// parseGTIN accepts 1–14 digits (optionally prefixed by "(01)", "[01]" or,
// for 15/16-digit input, "01") and returns the 13 significant digits,
// zero-padded on the left. A 14th digit is taken as the check digit and is
// recomputed rather than verified (the encoded value never includes it).
func parseGTIN(data string) (string, error) {
	d := strings.TrimSpace(data)
	switch {
	case strings.HasPrefix(d, "(01)") || strings.HasPrefix(d, "[01]"):
		d = d[4:]
	case (len(d) == 15 || len(d) == 16) && strings.HasPrefix(d, "01"):
		d = d[2:]
	}
	if len(d) == 0 || len(d) > 14 {
		return "", invalid("GTIN must have 1 to 14 digits, got %q", data)
	}
	for i := 0; i < len(d); i++ {
		if d[i] < '0' || d[i] > '9' {
			return "", invalid("GTIN %q is not numeric", data)
		}
	}
	if len(d) == 14 {
		d = d[:13]
	}
	return strings.Repeat("0", 13-len(d)) + d, nil
}

func gtinText(gtin13 string) string {
	return "(01)" + gtin13 + string(gs1.CheckDigit(gtin13))
}

func atoi64(s string) int64 {
	var v int64
	for i := 0; i < len(s); i++ {
		v = v*10 + int64(s[i]-'0')
	}
	return v
}
