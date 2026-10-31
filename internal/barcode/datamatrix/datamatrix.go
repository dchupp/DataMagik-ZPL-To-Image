// Package datamatrix encodes Data Matrix symbols (ISO/IEC 16022) as used by
// the ZPL ^BX command.
//
// ECC 200 is fully supported: all 24 square and 6 rectangular sizes,
// ASCII/C40/Text/X12/EDIFACT/Base 256 encodation with a minimal-length mode
// selection (shortest path over input position × encodation mode), FNC1/GS1,
// ECI, Structured Append, reader programming (FNC3), 05/06 macros and
// Zebra's ^BX escape sequences. The legacy convolutional ECC 000–140 symbols
// are not implemented: Encode returns an error wrapping ErrUnsupportedQuality.
package datamatrix

import (
	"errors"
	"fmt"

	"github.com/datamagik/zpl-to-image/internal/barcode"
)

// ErrUnsupportedQuality is returned (wrapped, together with
// barcode.ErrInvalidData) for the legacy ECC 000–140 quality levels, which
// this package does not implement.
var ErrUnsupportedQuality = errors.New("datamatrix: ECC 000-140 symbols are not supported")

// DefaultEscape is the escape character Zebra firmware V60.13.0.12 and later
// uses for ^BX quality 200 field data (older firmware uses '~').
const DefaultEscape = '_'

// MaxInput is the field data length (bytes) beyond which Zebra truncates
// quality 200 data.
const MaxInput = 3072

// Quality levels of the ^BX s parameter.
const (
	ECC000 = 0
	ECC050 = 50
	ECC080 = 80
	ECC100 = 100
	ECC140 = 140
	ECC200 = 200
)

// StructuredAppend places a symbol in a Structured Append sequence.
type StructuredAppend struct {
	Index   int // 1-based position of this symbol (1..16)
	Count   int // total number of symbols (2..16)
	FileID1 int // file identification, first byte (1..254)
	FileID2 int // file identification, second byte (1..254)
}

// Options controls encoding. The zero value is not useful for ZPL (Zebra's
// default quality is 0, which is unsupported); set Quality to ECC200.
type Options struct {
	// Quality is the ^BX s parameter: 0, 50, 80, 100, 140 or 200.
	Quality int
	// Columns and Rows force a symbol size (0 = automatic). When only one is
	// set the smallest symbol with that dimension that fits is used. The
	// ZPL layer should zero values above 49 (Zebra resets them to automatic).
	Columns, Rows int
	// FormatID is the ^BX f parameter (ECC 000–140 only, ignored for 200).
	FormatID int
	// Escape is the escape sequence control character (^BX g). 0 disables
	// escape processing so data is encoded literally.
	Escape byte
	// Rectangular selects rectangular symbols (^BX a=2). If the data does
	// not fit the largest rectangular symbol a square symbol is used.
	Rectangular bool
	// StructuredAppend, when non-nil, adds a Structured Append header.
	// It is an error to combine it with a "_2" escape sequence.
	StructuredAppend *StructuredAppend
}

// Encode encodes data as a Data Matrix symbol without quiet zone.
func Encode(data []byte, opt Options) (barcode.Matrix, error) {
	switch opt.Quality {
	case ECC200:
	case ECC000, ECC050, ECC080, ECC100, ECC140:
		return barcode.Matrix{}, fmt.Errorf("%w: %w (quality %d)", barcode.ErrInvalidData, ErrUnsupportedQuality, opt.Quality)
	default:
		return barcode.Matrix{}, errf("invalid quality %d", opt.Quality)
	}
	cw, size, err := encode200(data, opt)
	if err != nil {
		return barcode.Matrix{}, err
	}
	return barcode.Matrix{Rows: size.render(size.interleave(cw))}, nil
}

// encode200 returns the padded data codewords and the chosen symbol size.
func encode200(data []byte, opt Options) ([]byte, symbolSize, error) {
	if len(data) > MaxInput {
		data = data[:MaxInput]
	}
	items, sa, err := parseEscapes(data, opt.Escape)
	if err != nil {
		return nil, symbolSize{}, err
	}
	var s stream
	if opt.StructuredAppend != nil {
		if sa != nil {
			return nil, symbolSize{}, errf("structured append given twice")
		}
		a := opt.StructuredAppend
		if a.Count < 2 || a.Count > 16 || a.Index < 1 || a.Index > a.Count ||
			a.FileID1 < 1 || a.FileID1 > 254 || a.FileID2 < 1 || a.FileID2 > 254 {
			return nil, symbolSize{}, errf("invalid structured append %+v", *a)
		}
		sa = []byte{byte((a.Index-1)<<4 | (17 - a.Count)), byte(a.FileID1), byte(a.FileID2)}
	}
	if sa != nil {
		s.add(cwSA)
		s.add(sa...)
	}
	// FNC1 (GS1 / AIM) or FNC3 (reader programming) in first position.
	if len(items) > 0 && (items[0].kind == kFNC1 || items[0].kind == kFNC3) {
		if items[0].kind == kFNC1 {
			s.add(cwFNC1)
		} else {
			s.add(cwFNC3)
		}
		items = items[1:]
	}
	items = macro(&s, items)
	encodeItems(&s, items)

	size, err := chooseSize(&s, opt)
	if err != nil {
		return nil, symbolSize{}, err
	}
	if len(s.cw) > size.dataCW {
		s.drop()
	}
	return s.finish(size.dataCW), size, nil
}

// macro detects the ISO/IEC 15434 05/06 envelopes and replaces them with the
// macro codewords 236/237.
func macro(s *stream, items []item) []item {
	if len(items) < 9 {
		return items
	}
	head := []int{'[', ')', '>', 0x1e, '0', -1, 0x1d}
	var kind int
	for i, h := range head {
		it := items[i]
		if it.kind != kByte {
			return items
		}
		if h == -1 {
			kind = it.v
			continue
		}
		if it.v != h {
			return items
		}
	}
	t := items[len(items)-2:]
	if t[0].kind != kByte || t[0].v != 0x1e || t[1].kind != kByte || t[1].v != 0x04 {
		return items
	}
	switch kind {
	case '5':
		s.add(cwMacro05)
	case '6':
		s.add(cwMacro06)
	default:
		return items
	}
	return items[7 : len(items)-2]
}

func chooseSize(s *stream, opt Options) (symbolSize, error) {
	n := len(s.cw)
	if opt.Columns > 0 || opt.Rows > 0 {
		var matched bool
		for _, sz := range symbolSizes {
			if (opt.Columns > 0 && sz.cols != opt.Columns) || (opt.Rows > 0 && sz.rows != opt.Rows) {
				continue
			}
			if opt.Columns == 0 || opt.Rows == 0 {
				// Single dimension: honour the aspect preference when the
				// dimension alone is ambiguous.
				if sz.rect() != opt.Rectangular && hasShape(opt, opt.Rectangular) {
					continue
				}
			}
			matched = true
			if s.fits(sz.dataCW) {
				return sz, nil
			}
		}
		if !matched {
			return symbolSize{}, errf("no ECC 200 symbol with %d columns × %d rows", opt.Columns, opt.Rows)
		}
		return symbolSize{}, errf("data (%d codewords) does not fit the forced symbol size", n)
	}
	for _, rect := range []bool{opt.Rectangular, false} {
		for _, sz := range symbolSizes {
			if sz.rect() == rect && s.fits(sz.dataCW) {
				return sz, nil
			}
		}
	}
	return symbolSize{}, errf("data too long (%d codewords, max 1558)", n)
}

// hasShape reports whether some size with the forced dimension has the
// requested shape.
func hasShape(opt Options, rect bool) bool {
	for _, sz := range symbolSizes {
		if sz.rect() == rect && ((opt.Columns > 0 && sz.cols == opt.Columns) || (opt.Rows > 0 && sz.rows == opt.Rows)) {
			return true
		}
	}
	return false
}

// parseEscapes applies Zebra's ^BX escape sequences (quality 200):
//
//	EE      the escape character itself
//	E1      FNC1 (GS1 in first position, group separator elsewhere)
//	E2nnnnnnnnn  Structured Append: FNC2 followed by three 3-digit codeword
//	        values 001–254 (symbol sequence indicator, file ID 1, file ID 2)
//	E3      FNC3 (reader programming)
//	E5nnn   ECI / code page nnn
//	EdNNN   byte with decimal value NNN (000–255)
//	E@ … E^ control characters NUL … RS (E followed by c gives c-64)
//	E0      PAD: ends the encoded data (remaining field data is ignored)
//
// An escape character followed by anything else is kept literally.
func parseEscapes(data []byte, esc byte) (items []item, sa []byte, err error) {
	digits := func(i, n int) (int, bool) {
		if i+n > len(data) {
			return 0, false
		}
		v := 0
		for _, c := range data[i : i+n] {
			if c < '0' || c > '9' {
				return 0, false
			}
			v = v*10 + int(c-'0')
		}
		return v, true
	}
	for i := 0; i < len(data); i++ {
		c := data[i]
		if esc == 0 || c != esc || i+1 >= len(data) {
			items = append(items, item{kByte, int(c)})
			continue
		}
		n := data[i+1]
		switch {
		case n == esc:
			items = append(items, item{kByte, int(esc)})
			i++
		case n == '1':
			items = append(items, item{kFNC1, 0})
			i++
		case n == '3':
			items = append(items, item{kFNC3, 0})
			i++
		case n == '2':
			if sa != nil {
				return nil, nil, errf("more than one structured append sequence")
			}
			sa = make([]byte, 3)
			for k := 0; k < 3; k++ {
				v, ok := digits(i+2+3*k, 3)
				if !ok || v < 1 || v > 254 {
					return nil, nil, errf("structured append needs nine digits (three values 001-254)")
				}
				sa[k] = byte(v)
			}
			i += 10
		case n == '5':
			v, ok := digits(i+2, 3)
			if !ok {
				return nil, nil, errf("ECI escape needs three digits")
			}
			items = append(items, item{kECI, v})
			i += 4
		case n == 'd':
			v, ok := digits(i+2, 3)
			if !ok || v > 255 {
				return nil, nil, errf("%cd escape needs a three-digit value 000-255", esc)
			}
			items = append(items, item{kByte, v})
			i += 4
		case n == '4':
			return nil, nil, errf("explicit FNC4 is not allowed")
		case n == '0':
			return items, sa, nil
		case n >= '@' && n <= '_':
			items = append(items, item{kByte, int(n) - 64})
			i++
		default:
			items = append(items, item{kByte, int(c)})
		}
	}
	return items, sa, nil
}
