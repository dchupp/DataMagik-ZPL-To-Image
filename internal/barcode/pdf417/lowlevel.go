package pdf417

import "github.com/datamagik/zpl-to-image/internal/barcode"

// Low-level building blocks shared with the micropdf and composite packages.

// Pattern widths in modules.
const (
	CodewordModules = 17 // one symbol character
	StartModules    = 17 // start pattern 81111113
	StopModules     = 18 // stop pattern 711311121
	RAPModules      = 10 // MicroPDF417 / CC-A/B row address pattern
)

const (
	startPattern = 0x1FEA8 // 17 modules
	stopPattern  = 0x3FA29 // 18 modules
)

// appendBits appends the n low bits of v, most significant first.
func appendBits(row []bool, v uint32, n int) []bool {
	for i := n - 1; i >= 0; i-- {
		row = append(row, v>>uint(i)&1 == 1)
	}
	return row
}

// CodewordPattern returns the 17-module bar/space pattern of codeword cw
// (0..928) in the given cluster index (0, 1, 2 for clusters 0, 3, 6), most
// significant bit = first (dark) module.
func CodewordPattern(cluster, cw int) uint32 {
	return uint32(clusterPatterns[cluster%3][cw]) << 1
}

// AppendCodeword appends the 17 modules of codeword cw in cluster index
// cluster (0, 1, 2 for clusters 0, 3, 6).
func AppendCodeword(row []bool, cluster, cw int) []bool {
	return appendBits(row, CodewordPattern(cluster, cw), CodewordModules)
}

// AppendStart appends the PDF417 start pattern.
func AppendStart(row []bool) []bool { return appendBits(row, startPattern, StartModules) }

// AppendStop appends the PDF417 stop pattern.
func AppendStop(row []bool) []bool { return appendBits(row, stopPattern, StopModules) }

// AppendSideRAP appends left/right row address pattern number n (1..52, as
// numbered in ISO/IEC 24728 Table 2; values wrap modulo 52).
func AppendSideRAP(row []bool, n int) []bool {
	return appendBits(row, uint32(rapSide[(n-1)%52]), RAPModules)
}

// AppendCentreRAP appends centre row address pattern number n (1..52).
func AppendCentreRAP(row []bool, n int) []bool {
	return appendBits(row, uint32(rapCentre[(n-1)%52]), RAPModules)
}

// RowIndicators returns the left and right row indicator codeword values
// for row r (0-based) of a PDF417 symbol (ISO/IEC 15438 5.3.2).
func RowIndicators(r, rows, cols, ecLevel int) (left, right int) {
	base := 30 * (r / 3)
	x := (rows - 1) / 3
	y := ecLevel*3 + (rows-1)%3
	z := cols - 1
	switch r % 3 {
	case 0:
		return base + x, base + z
	case 1:
		return base + y, base + x
	default:
		return base + z, base + y
	}
}

// Layout draws a PDF417 symbol from its complete codeword sequence (length
// descriptor, data, padding and error correction, rows*cols codewords).
// When compact is set the right row indicator is dropped and the stop
// pattern is reduced to a single dark module (Compact/truncated PDF417).
// Each Matrix row is one symbol row, one module high.
func Layout(cws []int, cols, rows, ecLevel int, compact bool) barcode.Matrix {
	m := barcode.Matrix{Rows: make([][]bool, rows)}
	for r := 0; r < rows; r++ {
		cl := r % 3
		left, right := RowIndicators(r, rows, cols, ecLevel)
		row := make([]bool, 0, StartModules+(cols+2)*CodewordModules+StopModules)
		row = AppendStart(row)
		row = AppendCodeword(row, cl, left)
		for c := 0; c < cols; c++ {
			row = AppendCodeword(row, cl, cws[r*cols+c])
		}
		if compact {
			row = append(row, true)
		} else {
			row = AppendCodeword(row, cl, right)
			row = AppendStop(row)
		}
		m.Rows[r] = row
	}
	return m
}
