// Package barcode holds the shared result types for the symbology encoders in
// its sub-packages. Encoders are pure: they turn field data into module
// geometry and know nothing about ZPL commands, dots, rotation or fonts. The
// render package applies ^BY module widths, ratios, heights and interpretation
// lines on top of these results.
package barcode

import "errors"

// ErrInvalidData is returned (possibly wrapped) when field data cannot be
// encoded by a symbology.
var ErrInvalidData = errors.New("barcode: invalid data")

// Linear is a one-row symbol described as alternating bar/space elements,
// always starting with a bar.
//
// When Ratio is false each element width is a module count (1..n) and the
// renderer multiplies by the ^BY module width. When Ratio is true every
// element is 1 (narrow) or 2 (wide) and the renderer maps wide elements to
// round(ratio*moduleWidth) as Zebra does for ratio-based symbologies.
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

// TextGroup is a run of interpretation-line text centred on a module range.
type TextGroup struct {
	Text       string
	Start, End int // module offsets
	// Small renders the group with a smaller font (UPC-A/UPC-E outer digits).
	Small bool
}

// Width returns the symbol width in modules (narrow units when Ratio is set;
// wide elements count as wideUnits).
func (l Linear) Width(wideUnits int) int {
	n := 0
	for _, e := range l.Elements {
		if l.Ratio && e == 2 {
			n += wideUnits
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
