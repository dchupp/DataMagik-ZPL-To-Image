package qr

import "strings"

const alnumChars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ $%*+-./:"

// bitBuf is an MSB-first bit accumulator.
type bitBuf struct{ bits []bool }

func (b *bitBuf) put(v, n int) {
	for i := n - 1; i >= 0; i-- {
		b.bits = append(b.bits, v>>uint(i)&1 == 1)
	}
}

func (b *bitBuf) len() int { return len(b.bits) }

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isAlnum(c byte) bool { return strings.IndexByte(alnumChars, c) >= 0 }

// kanjiValue returns the 13-bit Kanji mode value of a Shift-JIS pair.
func kanjiValue(hi, lo byte) (int, bool) {
	if lo < 0x40 || lo > 0xFC || lo == 0x7F {
		return 0, false
	}
	v := int(hi)<<8 | int(lo)
	switch {
	case v >= 0x8140 && v <= 0x9FFC:
		v -= 0x8140
	case v >= 0xE040 && v <= 0xEBBF:
		v -= 0xC140
	default:
		return 0, false
	}
	return (v>>8)*0xC0 + v&0xFF, true
}

func validateSegment(s Segment) error {
	switch s.Mode {
	case ModeAuto, ModeByte:
	case ModeNumeric:
		for _, c := range s.Data {
			if !isDigit(c) {
				return invalid("non-digit %q in numeric segment", c)
			}
		}
	case ModeAlphanumeric:
		for _, c := range s.Data {
			if !isAlnum(c) {
				return invalid("character %q not allowed in alphanumeric segment", c)
			}
		}
	case ModeKanji:
		if len(s.Data)%2 != 0 {
			return invalid("Kanji segment has an odd byte count")
		}
		for i := 0; i < len(s.Data); i += 2 {
			if _, ok := kanjiValue(s.Data[i], s.Data[i+1]); !ok {
				return invalid("invalid Shift-JIS Kanji %02X%02X", s.Data[i], s.Data[i+1])
			}
		}
	default:
		return invalid("unknown mode %d", s.Mode)
	}
	return nil
}

// modeIndicator returns the 4-bit mode indicator.
func modeIndicator(m Mode) int {
	switch m {
	case ModeNumeric:
		return 1
	case ModeAlphanumeric:
		return 2
	case ModeByte:
		return 4
	case ModeKanji:
		return 8
	}
	return 0
}

// charCount returns the number of characters (count indicator units).
func charCount(s Segment) int {
	if s.Mode == ModeKanji {
		return len(s.Data) / 2
	}
	return len(s.Data)
}

// buildBits produces the data bit stream (without terminator/padding).
func buildBits(segs []Segment, opts Options, model, version int) (*bitBuf, error) {
	b := &bitBuf{}
	if model == 1 {
		b.put(0, 4) // Model 1 symbols start with four zero bits
	}
	if sa := opts.StructuredAppend; sa != nil {
		b.put(3, 4)
		b.put(sa.Position-1, 4)
		b.put(sa.Total-1, 4)
		b.put(int(sa.Parity), 8)
	}
	if opts.ECI > 0 {
		b.put(7, 4)
		switch e := opts.ECI; {
		case e < 1<<7:
			b.put(e, 8)
		case e < 1<<14:
			b.put(2<<14|e, 16)
		default:
			b.put(6<<21|e, 24)
		}
	}
	if opts.GS1 {
		b.put(5, 4)
	}
	total := 0
	for _, s := range segs {
		var parts []Segment
		if s.Mode == ModeAuto {
			parts = autoSegments(s.Data, version, !opts.DisableAutoKanji)
		} else {
			parts = []Segment{s}
		}
		for _, p := range parts {
			total += len(p.Data)
			writeSegment(b, p, version)
		}
	}
	if total == 0 {
		return nil, invalid("no data")
	}
	return b, nil
}

// writeSegment appends one segment, splitting it if its length exceeds the
// character count indicator range.
func writeSegment(b *bitBuf, s Segment, version int) {
	ccBits := charCountBits(s.Mode, version)
	maxChars := 1<<uint(ccBits) - 1
	unit := 1
	if s.Mode == ModeKanji {
		unit = 2
	}
	data := s.Data
	for len(data) > 0 {
		n := len(data) / unit
		if n > maxChars {
			n = maxChars
		}
		chunk := data[:n*unit]
		data = data[n*unit:]
		b.put(modeIndicator(s.Mode), 4)
		b.put(n, ccBits)
		switch s.Mode {
		case ModeNumeric:
			for i := 0; i < len(chunk); i += 3 {
				j := min(i+3, len(chunk))
				v := 0
				for _, c := range chunk[i:j] {
					v = v*10 + int(c-'0')
				}
				b.put(v, []int{0, 4, 7, 10}[j-i])
			}
		case ModeAlphanumeric:
			for i := 0; i < len(chunk); i += 2 {
				a := strings.IndexByte(alnumChars, chunk[i])
				if i+1 < len(chunk) {
					b.put(a*45+strings.IndexByte(alnumChars, chunk[i+1]), 11)
				} else {
					b.put(a, 6)
				}
			}
		case ModeByte:
			for _, c := range chunk {
				b.put(int(c), 8)
			}
		case ModeKanji:
			for i := 0; i < len(chunk); i += 2 {
				v, _ := kanjiValue(chunk[i], chunk[i+1])
				b.put(v, 13)
			}
		}
	}
}

// autoSegments splits data into the segment sequence with the smallest bit
// length for the given version's character count indicator sizes. Costs are
// tracked in sixths of a bit so numeric (10/3) and alphanumeric (11/2) per
// character costs are exact.
func autoSegments(data []byte, version int, kanji bool) []Segment {
	modes := [4]Mode{ModeNumeric, ModeAlphanumeric, ModeByte, ModeKanji}
	n := len(data)
	if n == 0 {
		return nil
	}
	const inf = int(^uint(0) >> 2)
	type state struct {
		cost int
		prev int // previous mode index, 4 = start
		step int // bytes consumed by the last character
	}
	st := make([][5]state, n+1)
	for i := range st {
		for m := range st[i] {
			st[i][m].cost = inf
		}
	}
	st[0][4].cost = 0
	var header [4]int
	for m := range modes {
		header[m] = (4 + charCountBits(modes[m], version)) * 6
	}
	charCost := [4]int{20, 33, 48, 78}
	for i := 0; i < n; i++ {
		c := data[i]
		var can [4]bool
		can[0] = isDigit(c)
		can[1] = isAlnum(c)
		can[2] = true
		if kanji && i+1 < n {
			_, can[3] = kanjiValue(c, data[i+1])
		}
		for from := 0; from < 5; from++ {
			cur := st[i][from].cost
			if cur == inf {
				continue
			}
			for to := 0; to < 4; to++ {
				if !can[to] {
					continue
				}
				step := 1
				if to == 3 {
					step = 2
				}
				cost := cur + charCost[to]
				if from != to {
					cost += header[to]
				}
				if cost < st[i+step][to].cost {
					st[i+step][to] = state{cost, from, step}
				}
			}
		}
	}
	best := 0
	for m := 1; m < 4; m++ {
		if st[n][m].cost < st[n][best].cost {
			best = m
		}
	}
	// Backtrack.
	var rev []Segment
	pos, m := n, best
	for pos > 0 {
		s := st[pos][m]
		start := pos - s.step
		if len(rev) > 0 && rev[len(rev)-1].Mode == modes[m] {
			last := &rev[len(rev)-1]
			last.Data = append(append([]byte{}, data[start:pos]...), last.Data...)
		} else {
			rev = append(rev, Segment{Mode: modes[m], Data: append([]byte{}, data[start:pos]...)})
		}
		pos, m = start, s.prev
	}
	segs := make([]Segment, len(rev))
	for i := range rev {
		segs[i] = rev[len(rev)-1-i]
	}
	return segs
}

// finishBits adds the terminator and pad codewords and returns the data
// codewords.
func finishBits(b *bitBuf, capacity int) []byte {
	b.put(0, min(4, capacity-b.len()))
	for b.len()%8 != 0 {
		b.bits = append(b.bits, false)
	}
	for pad := 0xEC; b.len() < capacity; pad ^= 0xEC ^ 0x11 {
		b.put(pad, 8)
	}
	out := make([]byte, capacity/8)
	for i, bit := range b.bits {
		if bit {
			out[i/8] |= 0x80 >> uint(i%8)
		}
	}
	return out
}

// addECC splits data into blocks, appends Reed-Solomon codewords and returns
// the final codeword sequence in placement order. Model 2 interleaves the
// blocks; Model 1 concatenates them (all data blocks, then all EC blocks).
func addECC(data []byte, model, version int, level ECLevel) []byte {
	ecLen, nb := blockLayout(model, version, level)
	total := totalCodewords(model, version)
	nData := len(data)
	shortLen := nData / nb
	numLong := nData % nb
	var blocks, ecs [][]byte
	off := 0
	for i := 0; i < nb; i++ {
		l := shortLen
		if i >= nb-numLong {
			l++
		}
		blk := data[off : off+l]
		off += l
		blocks = append(blocks, blk)
		ecs = append(ecs, rsEncode(blk, ecLen))
	}
	out := make([]byte, 0, total)
	if model == 1 {
		for _, b := range blocks {
			out = append(out, b...)
		}
		for _, e := range ecs {
			out = append(out, e...)
		}
	} else {
		for i := 0; i <= shortLen; i++ {
			for _, b := range blocks {
				if i < len(b) {
					out = append(out, b[i])
				}
			}
		}
		for i := 0; i < ecLen; i++ {
			for _, e := range ecs {
				out = append(out, e[i])
			}
		}
	}
	for len(out) < total {
		out = append(out, 0)
	}
	return out
}
