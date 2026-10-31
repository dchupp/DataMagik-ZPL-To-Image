package datamatrix

// symbolSize describes one ECC 200 symbol size (ISO/IEC 16022 Table 7).
type symbolSize struct {
	rows, cols       int // overall size in modules (no quiet zone)
	regRows, regCols int // data region size (without finder/clock patterns)
	dataCW, eccCW    int // total data and error correction codewords
	blocks           int // number of interleaved Reed-Solomon blocks
}

func (s symbolSize) rect() bool    { return s.rows != s.cols }
func (s symbolSize) regionsV() int { return s.rows / (s.regRows + 2) }
func (s symbolSize) regionsH() int { return s.cols / (s.regCols + 2) }

// symbolSizes lists every ECC 200 size, squares then rectangles, each group
// in ascending capacity order.
var symbolSizes = []symbolSize{
	{10, 10, 8, 8, 3, 5, 1},
	{12, 12, 10, 10, 5, 7, 1},
	{14, 14, 12, 12, 8, 10, 1},
	{16, 16, 14, 14, 12, 12, 1},
	{18, 18, 16, 16, 18, 14, 1},
	{20, 20, 18, 18, 22, 18, 1},
	{22, 22, 20, 20, 30, 20, 1},
	{24, 24, 22, 22, 36, 24, 1},
	{26, 26, 24, 24, 44, 28, 1},
	{32, 32, 14, 14, 62, 36, 1},
	{36, 36, 16, 16, 86, 42, 1},
	{40, 40, 18, 18, 114, 48, 1},
	{44, 44, 20, 20, 144, 56, 1},
	{48, 48, 22, 22, 174, 68, 1},
	{52, 52, 24, 24, 204, 84, 2},
	{64, 64, 14, 14, 280, 112, 2},
	{72, 72, 16, 16, 368, 144, 4},
	{80, 80, 18, 18, 456, 192, 4},
	{88, 88, 20, 20, 576, 224, 4},
	{96, 96, 22, 22, 696, 272, 4},
	{104, 104, 24, 24, 816, 336, 6},
	{120, 120, 18, 18, 1050, 408, 6},
	{132, 132, 20, 20, 1304, 496, 8},
	{144, 144, 22, 22, 1558, 620, 10},
	// Rectangular.
	{8, 18, 6, 16, 5, 7, 1},
	{8, 32, 6, 14, 10, 11, 1},
	{12, 26, 10, 24, 16, 14, 1},
	{12, 36, 10, 16, 22, 18, 1},
	{16, 36, 14, 16, 32, 24, 1},
	{16, 48, 14, 22, 49, 28, 1},
}

// interleave computes the error correction for s and returns the complete
// interleaved codeword stream (data then ECC) ready for placement.
func (s symbolSize) interleave(data []byte) []byte {
	out := make([]byte, s.dataCW+s.eccCW)
	copy(out, data)
	eccPer := s.eccCW / s.blocks
	for b := 0; b < s.blocks; b++ {
		var blk []byte
		for i := b; i < s.dataCW; i += s.blocks {
			blk = append(blk, data[i])
		}
		ecc := rsECC(blk, eccPer)
		// The 144×144 symbol interleaves its ECC rotated by two blocks.
		// This is the de-facto standard layout produced by zint and BWIPP
		// and expected by zxing (see zxing-cpp issue #259).
		col := b
		if s.rows == 144 {
			col = (b + 2) % s.blocks
		}
		for j, e := range ecc {
			out[s.dataCW+j*s.blocks+col] = e
		}
	}
	return out
}

// placement implements the ECC 200 module placement algorithm of
// ISO/IEC 16022 Annex F. It returns, for every mapping-matrix module, the
// codeword index*8 + bit (bit 0 = MSB), or -1 for a fixed light module and -2
// for a fixed dark module.
func placement(nrow, ncol int) []int {
	arr := make([]int, nrow*ncol)
	set := make([]bool, nrow*ncol)
	module := func(row, col, chr, bit int) {
		if row < 0 {
			row += nrow
			col += 4 - ((nrow + 4) % 8)
		}
		if col < 0 {
			col += ncol
			row += 4 - ((ncol + 4) % 8)
		}
		arr[row*ncol+col] = chr*8 + bit
		set[row*ncol+col] = true
	}
	utah := func(row, col, chr int) {
		module(row-2, col-2, chr, 0)
		module(row-2, col-1, chr, 1)
		module(row-1, col-2, chr, 2)
		module(row-1, col-1, chr, 3)
		module(row-1, col, chr, 4)
		module(row, col-2, chr, 5)
		module(row, col-1, chr, 6)
		module(row, col, chr, 7)
	}
	corner := func(chr int, pos [8][2]int) {
		for i, p := range pos {
			module(p[0], p[1], chr, i)
		}
	}
	chr, row, col := 0, 4, 0
	for {
		if row == nrow && col == 0 {
			corner(chr, [8][2]int{{nrow - 1, 0}, {nrow - 1, 1}, {nrow - 1, 2}, {0, ncol - 2}, {0, ncol - 1}, {1, ncol - 1}, {2, ncol - 1}, {3, ncol - 1}})
			chr++
		}
		if row == nrow-2 && col == 0 && ncol%4 != 0 {
			corner(chr, [8][2]int{{nrow - 3, 0}, {nrow - 2, 0}, {nrow - 1, 0}, {0, ncol - 4}, {0, ncol - 3}, {0, ncol - 2}, {0, ncol - 1}, {1, ncol - 1}})
			chr++
		}
		if row == nrow-2 && col == 0 && ncol%8 == 4 {
			corner(chr, [8][2]int{{nrow - 3, 0}, {nrow - 2, 0}, {nrow - 1, 0}, {0, ncol - 2}, {0, ncol - 1}, {1, ncol - 1}, {2, ncol - 1}, {3, ncol - 1}})
			chr++
		}
		if row == nrow+4 && col == 2 && ncol%8 == 0 {
			corner(chr, [8][2]int{{nrow - 1, 0}, {nrow - 1, ncol - 1}, {0, ncol - 3}, {0, ncol - 2}, {0, ncol - 1}, {1, ncol - 3}, {1, ncol - 2}, {1, ncol - 1}})
			chr++
		}
		for {
			if row < nrow && col >= 0 && !set[row*ncol+col] {
				utah(row, col, chr)
				chr++
			}
			row -= 2
			col += 2
			if row < 0 || col >= ncol {
				break
			}
		}
		row++
		col += 3
		for {
			if row >= 0 && col < ncol && !set[row*ncol+col] {
				utah(row, col, chr)
				chr++
			}
			row += 2
			col -= 2
			if row >= nrow || col < 0 {
				break
			}
		}
		row += 3
		col++
		if row >= nrow && col >= ncol {
			break
		}
	}
	if !set[nrow*ncol-1] {
		arr[(nrow-1)*ncol+ncol-1] = -2
		arr[(nrow-2)*ncol+ncol-2] = -2
		arr[(nrow-1)*ncol+ncol-2] = -1
		arr[(nrow-2)*ncol+ncol-1] = -1
	}
	return arr
}

// render places the codewords into s and adds finder and clock patterns.
func (s symbolSize) render(cw []byte) [][]bool {
	nrow := s.regionsV() * s.regRows
	ncol := s.regionsH() * s.regCols
	pl := placement(nrow, ncol)
	bit := func(r, c int) bool {
		v := pl[r*ncol+c]
		switch {
		case v == -2:
			return true
		case v < 0:
			return false
		}
		return cw[v/8]&(0x80>>(v%8)) != 0
	}
	rows := make([][]bool, s.rows)
	for y := range rows {
		rows[y] = make([]bool, s.cols)
	}
	for ry := 0; ry < s.regionsV(); ry++ {
		for rx := 0; rx < s.regionsH(); rx++ {
			y0 := ry * (s.regRows + 2)
			x0 := rx * (s.regCols + 2)
			h, w := s.regRows+2, s.regCols+2
			for i := 0; i < w; i++ {
				rows[y0+h-1][x0+i] = true // solid bottom
				rows[y0][x0+i] = i%2 == 0 // alternating top
			}
			for j := 0; j < h; j++ {
				rows[y0+j][x0] = true         // solid left
				rows[y0+j][x0+w-1] = j%2 == 1 // alternating right
			}
			for j := 0; j < s.regRows; j++ {
				for i := 0; i < s.regCols; i++ {
					rows[y0+1+j][x0+1+i] = bit(ry*s.regRows+j, rx*s.regCols+i)
				}
			}
		}
	}
	return rows
}
