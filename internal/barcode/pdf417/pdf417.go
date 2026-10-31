// Package pdf417 encodes PDF417 (ISO/IEC 15438) symbols for the ZPL ^B7
// command, including Compact (truncated) PDF417 and Macro PDF417 (the ^FM
// structured append), and exports the low-level pieces (symbol character
// patterns, row address patterns, GF(929) Reed-Solomon, high-level
// compaction) used by the micropdf, composite and tlc39 packages.
//
// Encode returns one Matrix row per symbol row; every row is one module
// high and the renderer applies the commanded row height (^B7 h, or the ^BY
// height divided by the number of rows) to each. Quiet zones are not
// included.
package pdf417

import (
	"bytes"
	"fmt"
	"math"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
)

// SecurityAuto selects the ISO/IEC 15438 Annex E recommended minimum error
// correction level for the data length. Zebra's ^B7 default is level 0.
const SecurityAuto = -1

// Limits of the symbology.
const (
	MaxCodewords = 928
	MaxRows      = 90
	MinRows      = 3
	MaxColumns   = 30
)

// Options controls PDF417 encoding (the ^B7 s, c, r and t parameters).
type Options struct {
	// SecurityLevel is the error correction level 0..8 (2^(level+1) EC
	// codewords), or SecurityAuto. ^B7 default: 0.
	SecurityLevel int
	// Columns is the number of data columns 1..30; 0 = automatic.
	Columns int
	// Rows is the number of rows 3..90; 0 = automatic.
	Rows int
	// Truncated selects Compact PDF417 (^B7 t=Y): right row indicator
	// removed and stop pattern reduced to a single module bar.
	Truncated bool
	// Macro, when non-nil, appends a Macro PDF417 control block.
	Macro *Macro
}

// Macro describes a Macro PDF417 control block (structured append).
type Macro struct {
	// SegmentIndex is the 0-based index of this symbol (0..99998).
	SegmentIndex int
	// FileID is the file identifier as codewords (each 0..899). Empty
	// uses DefaultFileID.
	FileID []int
	// SegmentCount, when > 0, is encoded as the optional segment count field.
	SegmentCount int
	// Last marks the final segment (terminator codeword 922).
	Last bool
}

// DefaultFileID is the file ID used for Macro PDF417 when none is given.
var DefaultFileID = []int{0}

// codewords returns the Macro PDF417 control block codewords.
func (m *Macro) codewords() []int {
	// Segment index and count are 5-digit Numeric Compaction values
	// (ISO/IEC 15438 H.2): codewords of 1 followed by the 5 digits.
	cws := []int{CWMacroStart, (100000 + m.SegmentIndex) / 900, (100000 + m.SegmentIndex) % 900}
	id := m.FileID
	if len(id) == 0 {
		id = DefaultFileID
	}
	cws = append(cws, id...)
	if m.SegmentCount > 0 {
		cws = append(cws, CWMacroOption, 1, (100000+m.SegmentCount)/900, (100000+m.SegmentCount)%900)
	}
	if m.Last {
		cws = append(cws, CWMacroEnd)
	}
	return cws
}

// ECLevelFor returns the ISO/IEC 15438 Annex E recommended minimum error
// correction level for n data codewords.
func ECLevelFor(n int) int {
	switch {
	case n <= 40:
		return 2
	case n <= 160:
		return 3
	case n <= 320:
		return 4
	default:
		return 5
	}
}

// Symbol is an encoded PDF417 symbol with its geometry.
type Symbol struct {
	Matrix    barcode.Matrix
	Rows      int
	Columns   int
	ECLevel   int
	Codewords []int // complete codeword sequence (rows*cols)
}

// Encode encodes data as a PDF417 symbol.
func Encode(data []byte, opts Options) (barcode.Matrix, error) {
	s, err := EncodeSymbol(data, opts)
	if err != nil {
		return barcode.Matrix{}, err
	}
	return s.Matrix, nil
}

// EncodeSymbol is Encode returning the symbol geometry and codewords too.
func EncodeSymbol(data []byte, opts Options) (Symbol, error) {
	cws := Compact(data)
	if opts.Macro != nil {
		if opts.Macro.SegmentIndex < 0 || opts.Macro.SegmentIndex > 99998 {
			return Symbol{}, fmt.Errorf("%w: pdf417: macro segment index %d out of range", barcode.ErrInvalidData, opts.Macro.SegmentIndex)
		}
		for _, c := range opts.Macro.FileID {
			if c < 0 || c > 899 {
				return Symbol{}, fmt.Errorf("%w: pdf417: macro file ID codeword %d out of range", barcode.ErrInvalidData, c)
			}
		}
	}
	return EncodeCodewords(cws, opts)
}

// EncodeCodewords lays out already-compacted data codewords (without the
// symbol length descriptor) as a PDF417 symbol, adding the length
// descriptor, padding, optional macro block and error correction.
func EncodeCodewords(data []int, opts Options) (Symbol, error) {
	var macro []int
	if opts.Macro != nil {
		macro = opts.Macro.codewords()
	}
	n := 1 + len(data) + len(macro) // incl. length descriptor
	level := opts.SecurityLevel
	if level == SecurityAuto {
		level = ECLevelFor(n)
	}
	if level < 0 || level > 8 {
		return Symbol{}, fmt.Errorf("%w: pdf417: security level %d out of range", barcode.ErrInvalidData, level)
	}
	k := 2 << level
	total := n + k
	cols, rows, err := size(total, opts.Columns, opts.Rows)
	if err != nil {
		return Symbol{}, err
	}
	all := make([]int, 0, rows*cols)
	all = append(all, rows*cols-k) // length descriptor
	all = append(all, data...)
	for len(all)+len(macro) < rows*cols-k {
		all = append(all, CWTextLatch) // padding
	}
	all = append(all, macro...)
	all = append(all, ECCodewords(all, k)...)
	return Symbol{
		Matrix:    Layout(all, cols, rows, level, opts.Truncated),
		Rows:      rows,
		Columns:   cols,
		ECLevel:   level,
		Codewords: all,
	}, nil
}

// size chooses the symbol dimensions for total codewords following the ^B7
// rules: both given -> must fit; one given -> the other is derived; neither
// -> the shape closest to Zebra's 1:2 column-to-row ratio (rows = 2*cols,
// e.g. 72 codewords -> 6 columns x 12 rows).
func size(total, cols, rows int) (int, int, error) {
	tooLong := func() (int, int, error) {
		return 0, 0, fmt.Errorf("%w: pdf417: %d codewords do not fit (columns %d, rows %d)", barcode.ErrInvalidData, total, cols, rows)
	}
	if cols < 0 || cols > MaxColumns || rows < 0 || (rows != 0 && (rows < MinRows || rows > MaxRows)) {
		return 0, 0, fmt.Errorf("%w: pdf417: columns %d / rows %d out of range", barcode.ErrInvalidData, cols, rows)
	}
	if total > MaxCodewords {
		return tooLong()
	}
	switch {
	case cols > 0 && rows > 0:
		if rows*cols > MaxCodewords || total > rows*cols {
			return tooLong()
		}
		return cols, rows, nil
	case cols > 0:
		r := max(MinRows, (total+cols-1)/cols)
		if r > MaxRows || r*cols > MaxCodewords {
			return tooLong()
		}
		return cols, r, nil
	case rows > 0:
		c := (total + rows - 1) / rows
		if c > MaxColumns || c*rows > MaxCodewords {
			return tooLong()
		}
		return max(c, 1), rows, nil
	}
	bestC, bestR, bestD := 0, 0, math.MaxInt
	for c := 1; c <= MaxColumns; c++ {
		r := max(MinRows, (total+c-1)/c)
		if r > MaxRows || r*c > MaxCodewords {
			continue
		}
		d := r - 2*c
		if d < 0 {
			d = -d
		}
		if d < bestD || (d == bestD && r*c < bestR*bestC) {
			bestC, bestR, bestD = c, r, d
		}
	}
	if bestC == 0 {
		return tooLong()
	}
	return bestC, bestR, nil
}

// EncodeStructured splits data over up to maxSymbols Macro PDF417 symbols
// (the ^FM structured append): each symbol is filled to capacity before the
// next is started, as the ^FM documentation describes. All symbols share
// opts (dimensions, security level, truncation); opts.Macro supplies the
// file ID (segment fields are set per symbol). A single symbol is returned
// without a macro block when the data fits in one.
func EncodeStructured(data []byte, opts Options, maxSymbols int) ([]barcode.Matrix, error) {
	if maxSymbols <= 1 {
		m, err := Encode(data, opts)
		if err != nil {
			return nil, err
		}
		return []barcode.Matrix{m}, nil
	}
	if m, err := Encode(data, Options{SecurityLevel: opts.SecurityLevel, Columns: opts.Columns, Rows: opts.Rows, Truncated: opts.Truncated}); err == nil {
		return []barcode.Matrix{m}, nil
	}
	var fileID []int
	if opts.Macro != nil {
		fileID = opts.Macro.FileID
	}
	var out []barcode.Matrix
	for idx := 0; len(data) > 0; idx++ {
		if idx >= maxSymbols {
			return nil, fmt.Errorf("%w: pdf417: data needs more than %d structured append symbols", barcode.ErrInvalidData, maxSymbols)
		}
		o := opts
		mk := func(n int) Options {
			o.Macro = &Macro{SegmentIndex: idx, FileID: fileID, Last: n == len(data)}
			return o
		}
		// Largest prefix that fits (binary search; compaction length is
		// monotonic in the prefix length to within a codeword or two, so
		// verify downwards after the search).
		lo, hi := 0, len(data)
		for lo < hi {
			mid := (lo + hi + 1) / 2
			if _, err := Encode(data[:mid], mk(mid)); err == nil {
				lo = mid
			} else {
				hi = mid - 1
			}
		}
		for lo > 0 {
			if _, err := Encode(data[:lo], mk(lo)); err == nil {
				break
			}
			lo--
		}
		if lo == 0 {
			return nil, fmt.Errorf("%w: pdf417: symbol too small for structured append", barcode.ErrInvalidData)
		}
		m, _ := Encode(data[:lo], mk(lo))
		out = append(out, m)
		data = data[lo:]
	}
	return out, nil
}

// ExpandFieldEscapes applies the ^FD escapes Zebra defines for PDF417 data:
// `\&` is carriage return + line feed and `\\` is a backslash.
func ExpandFieldEscapes(data []byte) []byte {
	if !bytes.Contains(data, []byte{'\\'}) {
		return data
	}
	out := make([]byte, 0, len(data)+4)
	for i := 0; i < len(data); i++ {
		if data[i] == '\\' && i+1 < len(data) {
			switch data[i+1] {
			case '&':
				out = append(out, '\r', '\n')
				i++
				continue
			case '\\':
				out = append(out, '\\')
				i++
				continue
			}
		}
		out = append(out, data[i])
	}
	return out
}
