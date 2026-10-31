package qr

import "github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"

// symbol is a QR module grid under construction.
type symbol struct {
	size     int
	dark     [][]bool
	function [][]bool // function patterns (never masked, never data)
	dataPos  [][2]int // data module coordinates in bit order (x, y)
}

func newSymbol(size int) *symbol {
	s := &symbol{size: size, dark: make([][]bool, size), function: make([][]bool, size)}
	for y := range size {
		s.dark[y] = make([]bool, size)
		s.function[y] = make([]bool, size)
	}
	return s
}

func (s *symbol) setFn(x, y int, dark bool) {
	s.dark[y][x] = dark
	s.function[y][x] = true
}

// drawFinder draws a finder pattern centred at (cx, cy) with its separator.
func (s *symbol) drawFinder(cx, cy int) {
	for dy := -4; dy <= 4; dy++ {
		for dx := -4; dx <= 4; dx++ {
			x, y := cx+dx, cy+dy
			if x < 0 || y < 0 || x >= s.size || y >= s.size {
				continue
			}
			d := max(abs(dx), abs(dy))
			s.setFn(x, y, d != 2 && d != 4)
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// drawCommon draws finders, separators, timing patterns, the dark module and
// reserves the format information areas.
func (s *symbol) drawCommon() {
	n := s.size
	s.drawFinder(3, 3)
	s.drawFinder(n-4, 3)
	s.drawFinder(3, n-4)
	for i := 8; i < n-8; i++ {
		s.setFn(6, i, i%2 == 0)
		s.setFn(i, 6, i%2 == 0)
	}
	for i := 0; i < 9; i++ {
		if !s.function[8][i] {
			s.setFn(i, 8, false)
		}
		if !s.function[i][8] {
			s.setFn(8, i, false)
		}
	}
	for i := 0; i < 8; i++ {
		s.setFn(n-1-i, 8, false)
		s.setFn(8, n-1-i, false)
	}
	s.setFn(8, n-8, true)
}

// drawFormat writes the 15-bit format information (both copies).
func (s *symbol) drawFormat(level ECLevel, mask, model int) {
	ecBits := [4]int{1, 0, 3, 2}[level]
	data := ecBits<<3 | mask
	rem := data
	for range 10 {
		rem = rem<<1 ^ (rem>>9)*0x537
	}
	bits := (data<<10 | rem) ^ 0x5412
	if model == 1 {
		bits = (data<<10 | rem) ^ 0x2825
	}
	bit := func(i int) bool { return bits>>uint(i)&1 == 1 }
	n := s.size
	for i := 0; i <= 5; i++ {
		s.setFn(8, i, bit(i))
	}
	s.setFn(8, 7, bit(6))
	s.setFn(8, 8, bit(7))
	s.setFn(7, 8, bit(8))
	for i := 9; i < 15; i++ {
		s.setFn(14-i, 8, bit(i))
	}
	for i := 0; i < 8; i++ {
		s.setFn(n-1-i, 8, bit(i))
	}
	for i := 8; i < 15; i++ {
		s.setFn(8, n-15+i, bit(i))
	}
	s.setFn(8, n-8, true)
}

// drawModel2 adds alignment patterns and version information.
func (s *symbol) drawModel2(version int) {
	pos := alignmentPositions(version)
	last := len(pos) - 1
	for i, cy := range pos {
		for j, cx := range pos {
			if (i == 0 && j == 0) || (i == 0 && j == last) || (i == last && j == 0) {
				continue
			}
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					s.setFn(cx+dx, cy+dy, max(abs(dx), abs(dy)) != 1)
				}
			}
		}
	}
	if version >= 7 {
		rem := version
		for range 12 {
			rem = rem<<1 ^ (rem>>11)*0x1F25
		}
		bits := version<<12 | rem
		for i := 0; i < 18; i++ {
			d := bits>>uint(i)&1 == 1
			a, b := s.size-11+i%3, i/3
			s.setFn(a, b, d)
			s.setFn(b, a, d)
		}
	}
	// Zigzag data placement.
	n := s.size
	for right := n - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5
		}
		upward := (right+1)&2 == 0
		for vert := 0; vert < n; vert++ {
			y := vert
			if upward {
				y = n - 1 - vert
			}
			for j := 0; j < 2; j++ {
				x := right - j
				if !s.function[y][x] {
					s.dataPos = append(s.dataPos, [2]int{x, y})
				}
			}
		}
	}
}

// drawModel1 lays out the Model 1 codeword positions and extension
// patterns. Model 1 places each codeword as a 2x4 (vertical) or 4x2
// (horizontal) block: two columns of vertical blocks along the right edge,
// rows of horizontal blocks across the middle, and four columns of vertical
// blocks left of the vertical timing pattern. The first codeword sits in the
// bottom-right corner. Extension patterns are the 2x4 / 4x2 cells along the
// right and bottom edges that are not used for codewords: their outer line is
// dark and their inner line light.
func (s *symbol) drawModel1() {
	n := s.size
	columns := n/4 + 1 + 2
	add := func(x, y, w int) {
		for b := 0; b < 8; b++ {
			s.dataPos = append(s.dataPos, [2]int{x - b%w, y - b/w})
		}
	}
	for j := 0; j < columns; j++ {
		switch {
		case j <= 1: // vertical blocks on the right side
			rows := (n - 8) / 4
			for i := 0; i < rows; i++ {
				x, y := n-1-j*2, n-1-i*4
				if j == 0 && i%2 == 0 && i > 0 && i < rows-1 { // extension
					for k := 0; k < 4; k++ {
						s.setFn(x, y-k, true)
						s.setFn(x-1, y-k, false)
					}
					continue
				}
				add(x, y, 2)
			}
		case columns-j <= 4: // vertical blocks on the left side
			rows := (n - 16) / 4
			for i := 0; i < rows; i++ {
				x := (columns-j-1)*2 + 1
				if columns-j == 4 {
					x++ // skip the timing column
				}
				add(x, n-1-8-i*4, 2)
			}
		default: // horizontal blocks
			rows := n / 2
			for i := 0; i < rows; i++ {
				if j == 2 && i >= rows-4 {
					continue
				}
				x := n - 1 - 4 - (j-2)*4
				y := n - 1 - i*2
				if i >= rows-3 {
					y-- // skip the timing row
				}
				if i == 0 && j%2 == 1 && j+1 != columns-4 { // extension
					for k := 0; k < 4; k++ {
						s.setFn(x-k, y, true)
						s.setFn(x-k, y-1, false)
					}
					continue
				}
				add(x, y, 4)
			}
		}
	}
	// The top four bits of the first codeword fall on the bottom-right 2x2
	// corner. They are always zero in Model 1 (the bit stream starts with
	// four 0 bits) and readers ignore them; the corner is drawn unmasked with
	// only the outermost module dark, as in reference symbols.
	s.setFn(n-1, n-1, true)
	s.setFn(n-2, n-1, false)
	s.setFn(n-1, n-2, false)
	s.setFn(n-2, n-2, false)
}

// maskBit reports whether data mask pattern m inverts module (x, y).
func maskBit(m, x, y int) bool {
	switch m {
	case 0:
		return (x+y)%2 == 0
	case 1:
		return y%2 == 0
	case 2:
		return x%3 == 0
	case 3:
		return (x+y)%3 == 0
	case 4:
		return (x/3+y/2)%2 == 0
	case 5:
		return x*y%2+x*y%3 == 0
	case 6:
		return (x*y%2+x*y%3)%2 == 0
	default:
		return ((x+y)%2+x*y%3)%2 == 0
	}
}

// buildMatrix places the final codewords and applies the best (or forced)
// mask.
func buildMatrix(cw []byte, model, version int, opts Options) barcode.Matrix {
	s := newSymbol(version*4 + 17)
	s.drawCommon()
	if model == 1 {
		s.drawModel1()
	} else {
		s.drawModel2(version)
	}
	// Reserve format info positions (drawn later per mask).
	s.drawFormat(opts.ECLevel, 0, model)
	for i, p := range s.dataPos {
		if i/8 < len(cw) && !s.function[p[1]][p[0]] {
			s.dark[p[1]][p[0]] = cw[i/8]>>uint(7-i%8)&1 == 1
		}
	}
	apply := func(m int) {
		for _, p := range s.dataPos {
			if maskBit(m, p[0], p[1]) && !s.function[p[1]][p[0]] {
				s.dark[p[1]][p[0]] = !s.dark[p[1]][p[0]]
			}
		}
	}
	mask := opts.Mask
	if !opts.ForceMask {
		best := -1
		for m := 0; m < 8; m++ {
			apply(m)
			s.drawFormat(opts.ECLevel, m, model)
			if p := penalty(s.dark); best < 0 || p < best {
				best, mask = p, m
			}
			apply(m)
		}
	}
	apply(mask)
	s.drawFormat(opts.ECLevel, mask, model)
	return barcode.Matrix{Rows: s.dark}
}

// penalty computes the ISO/IEC 18004 mask evaluation score (rules N1-N4).
func penalty(g [][]bool) int {
	n := len(g)
	score := 0
	at := func(x, y int, vertical bool) bool {
		if vertical {
			return g[x][y]
		}
		return g[y][x]
	}
	for _, vertical := range []bool{false, true} {
		for y := 0; y < n; y++ {
			// N1: runs of five or more same-colour modules.
			run := 1
			for x := 1; x <= n; x++ {
				if x < n && at(x, y, vertical) == at(x-1, y, vertical) {
					run++
					continue
				}
				if run >= 5 {
					score += 3 + run - 5
				}
				run = 1
			}
			// N3: 1:1:3:1:1 finder-like pattern with four light modules on
			// either side (outside the symbol counts as light).
			light := func(x int) bool { return x < 0 || x >= n || !at(x, y, vertical) }
			for x := 0; x+7 <= n; x++ {
				if !(at(x, y, vertical) && !at(x+1, y, vertical) && at(x+2, y, vertical) &&
					at(x+3, y, vertical) && at(x+4, y, vertical) && !at(x+5, y, vertical) && at(x+6, y, vertical)) {
					continue
				}
				before := light(x-1) && light(x-2) && light(x-3) && light(x-4)
				after := light(x+7) && light(x+8) && light(x+9) && light(x+10)
				if before || after {
					score += 40
				}
			}
		}
	}
	// N2: 2x2 blocks of one colour.
	dark := 0
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if g[y][x] {
				dark++
			}
			if x+1 < n && y+1 < n {
				c := g[y][x]
				if g[y][x+1] == c && g[y+1][x] == c && g[y+1][x+1] == c {
					score += 3
				}
			}
		}
	}
	// N4: dark module proportion.
	total := n * n
	k := (abs(dark*20-total*10)+total-1)/total - 1
	score += k * 10
	return score
}
