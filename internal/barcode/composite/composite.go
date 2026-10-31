// Package composite encodes the 2D component of GS1 Composite symbols
// (ISO/IEC 24723): CC-A, CC-B (MicroPDF417 based) and CC-C (PDF417 based),
// including the encodation method / general-purpose bit field encodation of
// the GS1 element string. It is used for ZPL ^BR (types 1-12).
//
// The linear component (GS1 DataBar, EAN/UPC, GS1-128) and the separator
// pattern between the components are produced elsewhere; Encode reports
// the horizontal Offset at which the component must be drawn relative to the
// linear symbol.
//
// Geometry: Matrix rows are component rows; RowHeights gives each row's
// height in modules (X): 2 for CC-A/CC-B and 3 for CC-C, the minimum row
// heights of ISO/IEC 24723 (^BR has no 2D row height parameter, so the
// renderer multiplies RowHeights by the module width). The component sits
// directly on top of the separator row (one module high, or ^BR d) which
// sits directly on top of the linear symbol.
//
// Horizontal alignment (Component.Offset = x of the component's first module
// minus x of linear module 0; see Component.Offset for module 0), verified
// against the ISO/IEC 24723 / GS1 General Specifications figures:
//
//	DataBar Omni/Truncated (CC-A/B, 4 col)   -4
//	DataBar Stacked/Stacked Omni (2 col)     +1
//	DataBar Limited (3 col)                  +1 (CC-A), -9 (CC-B)
//	DataBar Expanded / Exp. Stacked (4 col)  +2
//	UPC-A, EAN-13 (4 col), UPC-E (2 col)     -3
//	EAN-8 (3 col)                            -4 (CC-A), -14 (CC-B)
//	GS1-128 CC-A/B (4 col)                   right-aligned per 12.3 g (depends on LinearWidth)
//	GS1-128 CC-C                             -7 (columns from LinearWidth)
//
// Linkage the linear encoder must provide (ISO/IEC 24723 7.4): GS1 DataBar
// symbols set their linkage flag; GS1-128 adds an extra code set character
// between its last data character and the check character: final set B ->
// value 99 (CC-A/B) or 101 (CC-C); final set C -> 101 (CC-A/B) or 100
// (CC-C). EAN/UPC carry no flag.
package composite

import (
	"fmt"
	"math/big"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/micropdf"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/pdf417"
)

// Type is the composite component type.
type Type int

const (
	// Auto tries CC-A, then CC-B (^BR types 1-11). CC-C must be requested.
	Auto Type = iota
	CCA
	CCB
	CCC
)

func (t Type) String() string {
	return [...]string{"auto", "CC-A", "CC-B", "CC-C"}[t]
}

// Linear identifies the linear component the 2D component is linked to.
type Linear int

const (
	DataBar14       Linear = iota // GS1 DataBar Omnidirectional / Truncated (^BR 1, 2)
	DataBarStacked                // GS1 DataBar Stacked / Stacked Omnidirectional (^BR 3, 4)
	DataBarLimited                // ^BR 5
	DataBarExpanded               // GS1 DataBar Expanded (and Expanded Stacked) (^BR 6)
	UPCA                          // UPC-A and EAN-13 (^BR 7, 9)
	EAN13                         // same geometry as UPCA
	EAN8                          // ^BR 10
	UPCE                          // ^BR 8
	GS1128                        // UCC/EAN-128 (^BR 11, 12)
)

// Columns returns the CC-A/CC-B column count for the linear component
// (ISO/IEC 24723 Table 1).
func (l Linear) Columns() int {
	switch l {
	case DataBarStacked, UPCE:
		return 2
	case DataBarLimited, EAN8:
		return 3
	default:
		return 4
	}
}

// Options controls composite component encoding.
type Options struct {
	Linear Linear
	Type   Type
	// LinearWidth is the width in modules of the linear component; required
	// for GS1-128 (it sets the CC-C column count and the CC-A/B offset):
	// 11 * (number of Code 128 symbol characters incl. start, linkage,
	// check and stop) + 2.
	LinearWidth int
}

// Component is an encoded 2D component.
type Component struct {
	Matrix  barcode.Matrix
	Type    Type // CCA, CCB or CCC
	Columns int  // data columns
	// Offset is the x position (in modules) of Matrix column 0 relative to
	// module 0 of the linear symbol, where module 0 is: for GS1 DataBar the
	// leading light module of the left guard (as in the ISO/IEC 24724
	// element widths); for EAN/UPC the first bar of the left guard pattern
	// (no quiet zone); for GS1-128 the first bar of the start character.
	// Negative values mean the component starts left of the linear symbol
	// (the linear is then shifted right by -Offset in the combined symbol).
	Offset int
	// Codewords are the data and error correction codewords (for tests).
	Codewords []int
}

// Encode builds the 2D component for GS1 data (bracketed "(01)..." form or
// an FNC1/GS-separated element string).
func Encode(data string, opts Options) (Component, error) {
	src, err := ElementString(data)
	if err != nil {
		return Component{}, err
	}
	cols := opts.Linear.Columns()
	switch opts.Type {
	case CCA:
		return encodeA(src, cols, opts)
	case CCB:
		return encodeB(src, cols, opts)
	case CCC:
		if opts.Linear != GS1128 {
			return Component{}, fmt.Errorf("%w: composite: CC-C requires a GS1-128 linear component", barcode.ErrInvalidData)
		}
		return encodeC(src, opts)
	}
	if c, err := encodeA(src, cols, opts); err == nil {
		return c, nil
	}
	return encodeB(src, cols, opts)
}

// offset returns the component offset for the linear type (ISO/IEC 24723
// 12.3 alignment, as implemented by the reference encoders).
func offset(opts Options, t Type, width int) int {
	switch opts.Linear {
	case DataBar14:
		return -4
	case DataBarStacked:
		return 1
	case DataBarLimited:
		if t == CCA {
			return 1
		}
		return -9
	case DataBarExpanded:
		return 2
	case UPCA, EAN13, UPCE:
		return -3
	case EAN8:
		if t == CCA {
			return -4
		}
		return -14
	case GS1128:
		if t == CCC {
			return -7
		}
		// 12.3 g): align with the last space module of symbol character
		// number (n-9) div 2 counted from the right (0 = stop).
		n := (opts.LinearWidth - 2) / 11
		pos := (n - 9) / 2
		c := opts.LinearWidth - pos*11 - 1 - width
		if pos > 0 {
			c -= 2
		}
		return c
	}
	return 0
}

// ccaVariant holds a CC-A size (ISO/IEC 24723 Table 9) and its row address
// pattern start values (Tables 10, 11).
type ccaVariant struct {
	cols, rows, ec, left, centre, right, cluster int
}

var ccaVariants = []ccaVariant{
	{2, 5, 4, 39, 0, 19, 2}, {2, 6, 4, 1, 0, 33, 0}, {2, 7, 5, 32, 0, 12, 1}, {2, 8, 5, 8, 0, 40, 1},
	{2, 9, 6, 14, 0, 46, 1}, {2, 10, 6, 43, 0, 23, 0}, {2, 12, 7, 20, 0, 52, 1},
	{3, 4, 4, 11, 43, 23, 1}, {3, 5, 5, 1, 33, 13, 0}, {3, 6, 6, 5, 37, 17, 1}, {3, 7, 7, 15, 47, 27, 2},
	{3, 8, 7, 21, 1, 33, 2},
	{4, 3, 4, 40, 20, 52, 0}, {4, 4, 5, 43, 23, 3, 0}, {4, 5, 6, 46, 26, 6, 0}, {4, 6, 7, 34, 14, 46, 0},
	{4, 7, 8, 29, 9, 41, 1},
}

func (v ccaVariant) dataCW() int { return v.cols*v.rows - v.ec }

// ccaBits is the bit capacity of m base-928 codewords (69 bits per 7
// codewords, then 10 bits per codeword less one).
func ccaBits(m int) int {
	b := 69 * (m / 7)
	if r := m % 7; r > 0 {
		b += 10*r - 1
	}
	return b
}

func encodeA(src string, cols int, opts Options) (Component, error) {
	var vs []ccaVariant
	for _, v := range ccaVariants {
		if v.cols == cols {
			vs = append(vs, v)
		}
	}
	target := func(n int) int {
		for _, v := range vs {
			if b := ccaBits(v.dataCW()); n <= b {
				return b
			}
		}
		return 0
	}
	bits, err := bitString(src, target)
	if err != nil {
		return Component{}, err
	}
	data := base928(bits)
	var v ccaVariant
	for _, x := range vs {
		if x.dataCW() == len(data) {
			v = x
		}
	}
	all := append(data, pdf417.ECCodewords(data, v.ec)...)
	mv := micropdf.Variant{Columns: v.cols, Rows: v.rows, ECCodewords: v.ec,
		LeftRAP: v.left, CentreRAP: v.centre, RightRAP: v.right, Cluster: v.cluster}
	m := micropdf.Layout(all, mv, cols != 3) // 3-column CC-A has no left RAP
	m.RowHeights = heights(v.rows, 2)
	return Component{Matrix: m, Type: CCA, Columns: cols, Offset: offset(opts, CCA, m.Width()), Codewords: all}, nil
}

// base928 converts the CC-A bit string to codewords: each 69-bit group
// gives 7 base-928 codewords, a final group of n bits n/10+1 codewords.
func base928(bits []bool) []int {
	var out []int
	for i := 0; i < len(bits); i += 69 {
		end := min(i+69, len(bits))
		v := new(big.Int)
		for _, b := range bits[i:end] {
			v.Lsh(v, 1)
			if b {
				v.SetBit(v, 0, 1)
			}
		}
		n := (end-i)/10 + 1
		if end-i == 69 {
			n = 7
		}
		g := make([]int, n)
		m := new(big.Int)
		b928 := big.NewInt(928)
		for k := n - 1; k >= 0; k-- {
			v.DivMod(v, b928, m)
			g[k] = int(m.Int64())
		}
		out = append(out, g...)
	}
	return out
}

func heights(n, h int) []int {
	r := make([]int, n)
	for i := range r {
		r[i] = h
	}
	return r
}

// byteCW is the Byte Compaction codeword count for n bytes.
func byteCW(n int) int { return n/6*5 + n%6 }

// maxBytes returns the largest byte count fitting d codewords.
func maxBytes(d int) int {
	b := 0
	for byteCW(b+1) <= d {
		b++
	}
	return b
}

func toBytes(bits []bool) []byte {
	out := make([]byte, len(bits)/8)
	for i, b := range bits {
		if b {
			out[i/8] |= 0x80 >> uint(i%8)
		}
	}
	return out
}

func encodeB(src string, cols int, opts Options) (Component, error) {
	var vs []micropdf.Variant
	for _, v := range micropdf.Variants() {
		if v.Columns == cols {
			vs = append(vs, v)
		}
	}
	target := func(n int) int {
		for _, v := range vs {
			if b := 8 * maxBytes(v.DataCapacity()-2); n <= b {
				return b
			}
		}
		return 0
	}
	bits, err := bitString(src, target)
	if err != nil {
		return Component{}, err
	}
	data := append([]int{pdf417.CWLinkageGS1}, pdf417.ByteCompact(toBytes(bits))...)
	v, _ := micropdf.Smallest(cols, len(data))
	all := append(data, pdf417.ECCodewords(data, v.ECCodewords)...)
	m := micropdf.Layout(all, v, true)
	m.RowHeights = heights(v.Rows, 2)
	return Component{Matrix: m, Type: CCB, Columns: cols, Offset: offset(opts, CCB, m.Width()), Codewords: all}, nil
}

func encodeC(src string, opts Options) (Component, error) {
	if opts.LinearWidth < 68 {
		return Component{}, fmt.Errorf("%w: composite: CC-C needs the GS1-128 linear width (got %d)", barcode.ErrInvalidData, opts.LinearWidth)
	}
	var cols, level int
	target := func(n int) int {
		used := byteCW((n + 7) / 8)
		switch {
		case used <= 40:
			level = 2
		case used <= 160:
			level = 3
		case used <= 320:
			level = 4
		case used <= 833:
			level = 5
		case used <= 865:
			level = 4
		default:
			return 0
		}
		k := 2 << level
		used += k + 3
		// 52 = 7 (left shift) + 10 (right quiet zone) - 17 (start)
		//    + 2*17 (row indicators) + 18 (stop)
		cols = (opts.LinearWidth - 52) / 17
		if opts.LinearWidth == 68 {
			cols = 1
		}
		cols = min(cols, 30)
		rows := (used + cols - 1) / cols
		for rows > 30 && cols < 30 {
			cols++
			rows = (used + cols - 1) / cols
		}
		rows = max(rows, 3)
		return 8 * maxBytes(cols*rows-k-3)
	}
	bits, err := bitString(src, target)
	if err != nil {
		return Component{}, err
	}
	// cols and level were fixed by the last target call (unpadded length).
	k := 2 << level
	data := []int{0, pdf417.CWLinkageGS1}
	data = append(data, pdf417.ByteCompact(toBytes(bits))...)
	data[0] = len(data)
	all := append(data, pdf417.ECCodewords(data, k)...)
	rows := len(all) / cols
	m := pdf417.Layout(all, cols, rows, level, false)
	m.RowHeights = heights(rows, 3)
	return Component{Matrix: m, Type: CCC, Columns: cols, Offset: offset(opts, CCC, m.Width()), Codewords: all}, nil
}
