// Package barcode holds the shared result types for the symbology encoders in
// its sub-packages. Encoders are pure: they turn field data into module
// geometry and know nothing about ZPL commands, dots, rotation or fonts. The
// render package applies ^BY module widths, ratios, heights and interpretation
// lines on top of these results.
package barcode

import (
	"errors"
	"math"
)

// ErrInvalidData is returned (possibly wrapped) when field data cannot be
// encoded by a symbology.
var ErrInvalidData = errors.New("barcode: invalid data")

// Linear is a one-row symbol described as alternating bar/space elements,
// always starting with a bar.
//
// When Ratio is false each element width is a module count (1..n) and the
// renderer multiplies by the ^BY module width. When Ratio is true every
// element is one of the Elem constants and the renderer sizes it with
// ElementWidth, as Zebra does for ratio-based symbologies.
type Linear struct {
	Elements []int
	Ratio    bool

	// Text is the human readable interpretation line, including any check
	// digits the symbology prints.
	Text string

	// Guards marks elements (by index into Elements) of bars that extend
	// below the regular bar height (UPC/EAN guard patterns). Nil when unused.
	Guards []bool

	// TextGroups optionally splits Text into positioned groups (UPC/EAN
	// style). Each group is centred between the given module offsets; an
	// offset < 0 means the group sits left of the symbol, > total width
	// means right of it.
	TextGroups []TextGroup
}

// Elements of a Ratio symbol.
const (
	ElemNarrow     = 1 // one module
	ElemWide       = 2 // WideWidth
	ElemWideNarrow = 3 // wide + narrow: the Plessey termination bar on a ZD420
	ElemDash       = 4 // floor(5/3 × ratio × module): the Code 11 '-' centre bar on a ZD420
)

// ratioTenths is the ^BY ratio in tenths as a Zebra printer uses it: rounded
// to one decimal, a trailing 5 rounding down (a ZD420 draws 2.25 as 2.2 and
// 2.29 as 2.3).
func ratioTenths(ratio float64) int {
	return int(math.Ceil(ratio*10 - 0.5 - 1e-9))
}

// WideWidth is the width in dots of a wide element: floor(module × ratio),
// the ratio taken to tenths first, as measured on a ZD420 (module 1 is 2
// dots wide from ratio 2.0 to 2.9 and 3 at 3.0).
func WideWidth(module int, ratio float64) int {
	return module * ratioTenths(ratio) / 10
}

// ElementWidth is the width in dots of element e (one of the Elem
// constants) of a Ratio symbol.
func ElementWidth(e, module int, ratio float64) int {
	switch e {
	case ElemWide:
		return WideWidth(module, ratio)
	case ElemWideNarrow:
		return WideWidth(module, ratio) + module
	case ElemDash:
		return 5 * module * ratioTenths(ratio) / 30
	}
	return module
}

// TextGroup is a run of interpretation-line text centred on a module range.
type TextGroup struct {
	Text       string
	Start, End int // module offsets
	// Small renders the group with a smaller font (UPC-A/UPC-E outer digits).
	Small bool
}

// Width returns the symbol width in modules. For a Ratio symbol it is the
// width in dots at module width 1 and ratio wideUnits.
func (l Linear) Width(wideUnits int) int {
	n := 0
	for _, e := range l.Elements {
		if l.Ratio {
			n += ElementWidth(e, 1, float64(wideUnits))
		} else {
			n += e
		}
	}
	return n
}

// Matrix is a 2D module grid. Rows[y][x] is true for a dark module.
//
// RowHeights, when non-nil, gives the relative height of each row in
// row-height units (stacked symbologies use it for separator rows; the
// renderer multiplies by the commanded row height). When nil every row is one
// unit and modules are square.
type Matrix struct {
	Rows       [][]bool
	RowHeights []int
}

// Width returns the number of module columns.
func (m Matrix) Width() int {
	if len(m.Rows) == 0 {
		return 0
	}
	return len(m.Rows[0])
}

// Height returns the number of module rows.
func (m Matrix) Height() int { return len(m.Rows) }

// BarState is one bar of a height-modulated postal symbol.
type BarState uint8

const (
	BarFull      BarState = iota // ascender + tracker + descender
	BarAscender                  // ascender + tracker
	BarDescender                 // tracker + descender
	BarTracker                   // tracker only
)

// Postal is a height-modulated symbol (POSTNET, PLANET, USPS IMb).
// POSTNET/PLANET use only BarFull (tall) and BarTracker (short).
type Postal struct {
	Bars []BarState
	Text string
}
