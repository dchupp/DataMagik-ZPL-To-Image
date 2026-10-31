package aztec

// Symbol construction, ISO/IEC 24778 sections 7.2-7.4 (layout, mode message,
// bit stuffing, Reed-Solomon) and section 9 (Aztec Runes).

// wordSize returns the codeword size in bits for a symbol with the given
// number of layers.
func wordSize(layers int) int {
	switch {
	case layers <= 2:
		return 6
	case layers <= 8:
		return 8
	case layers <= 22:
		return 10
	}
	return 12
}

// totalBits returns the number of data-layer bits for a symbol.
func totalBits(compact bool, layers int) int {
	base := 112
	if compact {
		base = 88
	}
	return (base + 16*layers) * layers
}

// stuffBits splits bits into codewords of size ws, applying the Aztec bit
// stuffing rule: a codeword whose first ws-1 bits are all 0 or all 1 gets its
// last bit forced to the complement and the displaced bit carried over. The
// final codeword is padded with 1s.
func stuffBits(bits []bool, ws int) []int {
	var out []int
	n := len(bits)
	mask := (1 << ws) - 2
	for i := 0; i < n; i += ws {
		w := 0
		for j := 0; j < ws; j++ {
			if i+j >= n || bits[i+j] {
				w |= 1 << (ws - 1 - j)
			}
		}
		switch {
		case w&mask == mask:
			out = append(out, w&mask)
			i--
		case w&mask == 0:
			out = append(out, w|1)
			i--
		default:
			out = append(out, w)
		}
	}
	return out
}

// modeMessage builds the mode message bits (28 compact, 40 full).
func modeMessage(compact bool, layers, dataWords int, menu bool) []bool {
	var words []int
	var total int
	if compact {
		v := (layers-1)<<6 | (dataWords - 1)
		if menu {
			v |= 1 << 5
		}
		words = []int{v >> 4 & 15, v & 15}
		total = 7
	} else {
		v := (layers-1)<<11 | (dataWords - 1)
		if menu {
			v |= 1 << 10
		}
		words = []int{v >> 12 & 15, v >> 8 & 15, v >> 4 & 15, v & 15}
		total = 10
	}
	words = append(words, rsEncode(4, words, total-len(words))...)
	var bits []bool
	for _, w := range words {
		bits = appendBits(bits, w, 4)
	}
	return bits
}

type grid struct {
	size int
	rows [][]bool
}

func newGrid(n int) *grid {
	g := &grid{size: n, rows: make([][]bool, n)}
	for i := range g.rows {
		g.rows[i] = make([]bool, n)
	}
	return g
}

func (g *grid) set(x, y int) { g.rows[y][x] = true }

func (g *grid) drawBullsEye(center, size int) {
	for i := 0; i < size; i += 2 {
		for j := center - i; j <= center+i; j++ {
			g.set(j, center-i)
			g.set(j, center+i)
			g.set(center-i, j)
			g.set(center+i, j)
		}
	}
	// orientation marks
	g.set(center-size, center-size)
	g.set(center-size+1, center-size)
	g.set(center-size, center-size+1)
	g.set(center+size, center-size)
	g.set(center+size, center-size+1)
	g.set(center+size, center+size-1)
}

func (g *grid) drawModeMessage(compact bool, mm []bool) {
	c := g.size / 2
	if compact {
		for i := 0; i < 7; i++ {
			o := c - 3 + i
			if mm[i] {
				g.set(o, c-5)
			}
			if mm[i+7] {
				g.set(c+5, o)
			}
			if mm[20-i] {
				g.set(o, c+5)
			}
			if mm[27-i] {
				g.set(c-5, o)
			}
		}
		return
	}
	for i := 0; i < 10; i++ {
		o := c - 5 + i + i/5
		if mm[i] {
			g.set(o, c-7)
		}
		if mm[i+10] {
			g.set(c+7, o)
		}
		if mm[29-i] {
			g.set(o, c+7)
		}
		if mm[39-i] {
			g.set(c-7, o)
		}
	}
}

// symbolSize returns the side length in modules of a compact or full-range
// symbol (including reference grid lines).
func symbolSize(compact bool, layers int) int {
	if compact {
		return 11 + 4*layers
	}
	base := 14 + 4*layers
	return base + 1 + 2*((base/2-1)/15)
}

// build lays out a complete symbol from its message bits (data + check
// words, already padded to totalBits) and mode message.
func build(compact bool, layers int, msg []bool, mm []bool) [][]bool {
	base := 14 + 4*layers
	if compact {
		base = 11 + 4*layers
	}
	size := symbolSize(compact, layers)
	amap := make([]int, base)
	if compact {
		for i := range amap {
			amap[i] = i
		}
	} else {
		oc, c := base/2, size/2
		for i := 0; i < oc; i++ {
			off := i + i/15
			amap[oc-i-1] = c - off - 1
			amap[oc+i] = c + off + 1
		}
	}
	g := newGrid(size)
	rowOffset := 0
	for i := 0; i < layers; i++ {
		rowSize := (layers-i)*4 + 9
		if !compact {
			rowSize += 3
		}
		for j := 0; j < rowSize; j++ {
			co := j * 2
			for k := 0; k < 2; k++ {
				if msg[rowOffset+co+k] {
					g.set(amap[i*2+k], amap[i*2+j])
				}
				if msg[rowOffset+rowSize*2+co+k] {
					g.set(amap[i*2+j], amap[base-1-i*2-k])
				}
				if msg[rowOffset+rowSize*4+co+k] {
					g.set(amap[base-1-i*2-k], amap[base-1-i*2-j])
				}
				if msg[rowOffset+rowSize*6+co+k] {
					g.set(amap[base-1-i*2-j], amap[i*2+k])
				}
			}
		}
		rowOffset += rowSize * 8
	}
	g.drawModeMessage(compact, mm)
	if compact {
		g.drawBullsEye(size/2, 5)
	} else {
		g.drawBullsEye(size/2, 7)
		// reference grid
		for i, j := 0, 0; i < base/2-1; i, j = i+15, j+16 {
			for k := (size / 2) & 1; k < size; k += 2 {
				g.set(size/2-j, k)
				g.set(size/2+j, k)
				g.set(k, size/2-j)
				g.set(k, size/2+j)
			}
		}
	}
	return g.rows
}

// runeBits returns the 28-bit mode message of an Aztec Rune with value v.
func runeBits(v int) []bool {
	words := []int{v >> 4 & 15, v & 15}
	words = append(words, rsEncode(4, words, 5)...)
	var bits []bool
	for _, w := range words {
		bits = appendBits(bits, w, 4)
	}
	// Runes invert every other bit starting with the first (XOR 1010...).
	for i := 0; i < len(bits); i += 2 {
		bits[i] = !bits[i]
	}
	return bits
}
