// Package micropdf encodes MicroPDF417 (ISO/IEC 24728) symbols for the ZPL
// ^BF command and exposes the variant table and layout used by the GS1
// CC-B composite component and TLC39.
//
// Encode returns one Matrix row per symbol row, each one module high; the
// renderer applies the ^BF h row height to every row. Quiet zones are not
// included.
package micropdf

import (
	"fmt"

	"github.com/datamagik/zpl-to-image/internal/barcode"
	"github.com/datamagik/zpl-to-image/internal/barcode/pdf417"
)

// Variant is one of the 34 MicroPDF417 symbol sizes of ISO/IEC 24728
// Table 1 with its row address pattern start values (Tables 10-12).
type Variant struct {
	Columns     int // data columns 1..4
	Rows        int // 4..44
	ECCodewords int // number of error correction codewords
	LeftRAP     int // starting left RAP number (1..52)
	CentreRAP   int // starting centre RAP number (3 and 4 column only)
	RightRAP    int // starting right RAP number
	Cluster     int // starting cluster index (0, 1, 2 = clusters 0, 3, 6)
}

// DataCapacity returns the number of data codewords the variant holds.
func (v Variant) DataCapacity() int { return v.Columns*v.Rows - v.ECCodewords }

// Modules returns the symbol width in modules.
func (v Variant) Modules() int {
	w := 2*pdf417.RAPModules + v.Columns*pdf417.CodewordModules + 1
	if v.Columns >= 3 {
		w += pdf417.RAPModules
	}
	return w
}

// variants in ISO/IEC 24728 Table 1 order.
var variants = func() []Variant {
	cols := []int{1, 1, 1, 1, 1, 1, 2, 2, 2, 2, 2, 2, 2, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4}
	rows := []int{11, 14, 17, 20, 24, 28, 8, 11, 14, 17, 20, 23, 26, 6, 8, 10, 12, 15, 20, 26, 32, 38, 44, 4, 6, 8, 10, 12, 15, 20, 26, 32, 38, 44}
	ec := []int{7, 7, 7, 8, 8, 8, 8, 9, 9, 10, 11, 13, 15, 12, 14, 16, 18, 21, 26, 32, 38, 44, 50, 8, 12, 14, 16, 18, 21, 26, 32, 38, 44, 50}
	left := []int{1, 8, 36, 19, 9, 25, 1, 1, 8, 36, 19, 9, 27, 1, 7, 15, 25, 37, 1, 1, 21, 15, 1, 47, 1, 7, 15, 25, 37, 1, 1, 21, 15, 1}
	centre := []int{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 7, 15, 25, 37, 17, 9, 29, 31, 25, 19, 1, 7, 15, 25, 37, 17, 9, 29, 31, 25}
	right := []int{9, 8, 36, 19, 17, 33, 1, 9, 8, 36, 19, 17, 35, 1, 7, 15, 25, 37, 33, 17, 37, 47, 49, 43, 1, 7, 15, 25, 37, 33, 17, 37, 47, 49}
	cluster := []int{0, 1, 2, 0, 2, 0, 0, 0, 1, 2, 0, 2, 2, 0, 0, 2, 0, 0, 0, 0, 2, 2, 0, 1, 0, 0, 2, 0, 0, 0, 0, 2, 2, 0}
	v := make([]Variant, len(cols))
	for i := range v {
		v[i] = Variant{cols[i], rows[i], ec[i], left[i], centre[i], right[i], cluster[i]}
	}
	return v
}()

// Variants returns the 34 variants in ISO/IEC 24728 Table 1 order.
func Variants() []Variant { return append([]Variant(nil), variants...) }

// ZebraMode returns the variant selected by ^BF mode m (0..33). Zebra's
// table lists the ISO variants in order except that the 4-column 4-row
// variant (ISO's first 4-column entry) is moved to the end as mode 33.
func ZebraMode(m int) (Variant, error) {
	switch {
	case m < 0 || m > 33:
		return Variant{}, fmt.Errorf("%w: micropdf: mode %d out of range 0..33", barcode.ErrInvalidData, m)
	case m < 23:
		return variants[m], nil
	case m < 33:
		return variants[m+1], nil
	default:
		return variants[23], nil
	}
}

// Smallest returns the smallest variant (fewest rows) with the given column
// count (1..4) holding n data codewords. With columns 0 any column count is
// allowed and the variant with the smallest area (then fewest columns) wins.
func Smallest(columns, n int) (Variant, bool) {
	best, ok := Variant{}, false
	for _, v := range variants {
		if (columns != 0 && v.Columns != columns) || v.DataCapacity() < n {
			continue
		}
		if !ok || v.Columns*v.Rows < best.Columns*best.Rows ||
			(v.Columns*v.Rows == best.Columns*best.Rows && v.Columns < best.Columns) {
			best, ok = v, true
		}
	}
	return best, ok
}

// ModeAuto selects the smallest variant that holds the data.
const ModeAuto = -1

// Options controls MicroPDF417 encoding.
type Options struct {
	// Mode is the ^BF mode 0..33 (Zebra's column/row/EC table), or ModeAuto.
	// The ^BF default is 0 (1 column x 11 rows).
	Mode int
}

// Encode encodes data (Text/Numeric/Byte compaction) as a MicroPDF417
// symbol of the variant selected by opts.Mode. Data that does not fit the
// selected variant is an error (Zebra prints no symbol).
func Encode(data []byte, opts Options) (barcode.Matrix, error) {
	cws := pdf417.CompactMicro(data)
	var v Variant
	if opts.Mode == ModeAuto {
		var ok bool
		if v, ok = Smallest(0, len(cws)); !ok {
			return barcode.Matrix{}, fmt.Errorf("%w: micropdf: data too long (%d codewords)", barcode.ErrInvalidData, len(cws))
		}
	} else {
		var err error
		if v, err = ZebraMode(opts.Mode); err != nil {
			return barcode.Matrix{}, err
		}
	}
	return EncodeCodewords(cws, v)
}

// EncodeCodewords pads data codewords to the variant's capacity with 900,
// adds error correction and lays out the symbol.
func EncodeCodewords(data []int, v Variant) (barcode.Matrix, error) {
	if len(data) > v.DataCapacity() {
		return barcode.Matrix{}, fmt.Errorf("%w: micropdf: %d data codewords exceed %d-column %d-row capacity %d",
			barcode.ErrInvalidData, len(data), v.Columns, v.Rows, v.DataCapacity())
	}
	all := make([]int, 0, v.Columns*v.Rows)
	all = append(all, data...)
	for len(all) < v.DataCapacity() {
		all = append(all, pdf417.CWTextLatch)
	}
	all = append(all, pdf417.ECCodewords(all, v.ECCodewords)...)
	return Layout(all, v, true), nil
}

// Layout draws the complete codeword sequence (rows*columns codewords) in
// MicroPDF417 row structure: left RAP, data columns with a centre RAP
// (after column 1 for 3 columns, after column 2 for 4 columns), right RAP
// and a one-module stop bar. When leftRAP is false the left RAP is omitted
// (used by the 3-column CC-A composite component).
func Layout(cws []int, v Variant, leftRAP bool) barcode.Matrix {
	m := barcode.Matrix{Rows: make([][]bool, v.Rows)}
	l, c, r, cl := v.LeftRAP, v.CentreRAP, v.RightRAP, v.Cluster
	for y := 0; y < v.Rows; y++ {
		row := make([]bool, 0, v.Modules())
		if leftRAP {
			row = pdf417.AppendSideRAP(row, l)
		}
		for x := 0; x < v.Columns; x++ {
			if (v.Columns == 3 && x == 1) || (v.Columns == 4 && x == 2) {
				row = pdf417.AppendCentreRAP(row, c)
			}
			row = pdf417.AppendCodeword(row, cl, cws[y*v.Columns+x])
		}
		row = pdf417.AppendSideRAP(row, r)
		row = append(row, true) // stop: one-module bar
		m.Rows[y] = row
		l, c, r = l%52+1, c%52+1, r%52+1
		cl = (cl + 1) % 3
	}
	return m
}
