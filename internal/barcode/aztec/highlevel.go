package aztec

// High-level (character) encodation, ISO/IEC 24778 section 7.3.1.
//
// The encoder runs a dynamic search over the five character modes (Upper,
// Lower, Mixed, Punct, Digit) plus Binary Shift, keeping every state that is
// not dominated by another one, and finally picks the shortest bit stream.
// This is the well known near-optimal algorithm also used by ZXing.

const (
	modeUpper = iota
	modeLower
	modeDigit
	modeMixed
	modePunct
	nModes
)

// latchTable[from][to] = (bitCount << 16) | code bits for the shortest latch
// sequence between modes.
var latchTable = [nModes][nModes]int{
	{0, (5 << 16) + 28, (5 << 16) + 30, (5 << 16) + 29, (10 << 16) + (29 << 5) + 30},                                      // Upper
	{(9 << 16) + (30 << 4) + 14, 0, (5 << 16) + 30, (5 << 16) + 29, (10 << 16) + (29 << 5) + 30},                          // Lower
	{(4 << 16) + 14, (9 << 16) + (14 << 5) + 28, 0, (9 << 16) + (14 << 5) + 29, (14 << 16) + (14 << 10) + (29 << 5) + 30}, // Digit
	{(5 << 16) + 29, (5 << 16) + 28, (10 << 16) + (29 << 5) + 30, 0, (5 << 16) + 30},                                      // Mixed
	{(5 << 16) + 31, (10 << 16) + (31 << 5) + 28, (10 << 16) + (31 << 5) + 30, (10 << 16) + (31 << 5) + 29, 0},            // Punct
}

// shiftTable[from][to] is the shift code, or -1 when no shift exists.
var shiftTable = func() [nModes][nModes]int {
	var t [nModes][nModes]int
	for i := range t {
		for j := range t[i] {
			t[i][j] = -1
		}
	}
	t[modeUpper][modePunct] = 0
	t[modeLower][modePunct] = 0
	t[modeLower][modeUpper] = 28
	t[modeMixed][modePunct] = 0
	t[modeDigit][modePunct] = 0
	t[modeDigit][modeUpper] = 15
	return t
}()

// charMap[mode][byte] is the code of the byte in that mode, 0 if absent.
var charMap = func() [nModes][256]int {
	var m [nModes][256]int
	m[modeUpper][' '] = 1
	for c := 'A'; c <= 'Z'; c++ {
		m[modeUpper][c] = int(c-'A') + 2
	}
	m[modeLower][' '] = 1
	for c := 'a'; c <= 'z'; c++ {
		m[modeLower][c] = int(c-'a') + 2
	}
	m[modeDigit][' '] = 1
	for c := '0'; c <= '9'; c++ {
		m[modeDigit][c] = int(c-'0') + 2
	}
	m[modeDigit][','] = 12
	m[modeDigit]['.'] = 13
	mixed := []byte{0, ' ', 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 27, 28, 29, 30, 31, '@', '\\', '^', '_', '`', '|', '~', 127}
	for i, c := range mixed {
		if i > 0 {
			m[modeMixed][c] = i
		}
	}
	punct := []byte{0, '\r', 0, 0, 0, 0, '!', '"', '#', '$', '%', '&', '\'', '(', ')', '*', '+', ',', '-', '.', '/', ':', ';', '<', '=', '>', '?', '[', ']', '{', '}'}
	for i, c := range punct {
		if c > 0 {
			m[modePunct][c] = i
		}
	}
	return m
}()

// item is one input element: a byte, or (when flag >= 0) an FLG(n) escape
// carrying digits (ECI designator) or FNC1 when digits is empty.
type item struct {
	b      byte
	flag   bool
	digits string
}

// token is an immutable linked list node of output bits.
type token struct {
	prev *token
	// simple token
	value, bits int
	// binary shift token (bits == -1)
	start, count int
}

func (t *token) add(value, bits int) *token { return &token{prev: t, value: value, bits: bits} }
func (t *token) addBinary(start, count int) *token {
	return &token{prev: t, bits: -1, start: start, count: count}
}

type state struct {
	tok        *token
	mode       int
	shiftBytes int // bytes in the open binary shift
	shiftCost  int
	bitCount   int
}

func binaryShiftCost(n int) int {
	switch {
	case n > 62:
		return 21
	case n > 31:
		return 20
	case n > 0:
		return 10
	}
	return 0
}

func newState(tok *token, mode, shiftBytes, bitCount int) *state {
	return &state{tok: tok, mode: mode, shiftBytes: shiftBytes, shiftCost: binaryShiftCost(shiftBytes), bitCount: bitCount}
}

func (s *state) latchAndAppend(mode, value int) *state {
	bc := s.bitCount
	t := s.tok
	if mode != s.mode {
		l := latchTable[s.mode][mode]
		t = t.add(l&0xFFFF, l>>16)
		bc += l >> 16
	}
	n := 5
	if mode == modeDigit {
		n = 4
	}
	t = t.add(value, n)
	return newState(t, mode, 0, bc+n)
}

func (s *state) shiftAndAppend(mode, value int) *state {
	n := 5
	if s.mode == modeDigit {
		n = 4
	}
	t := s.tok.add(shiftTable[s.mode][mode], n).add(value, 5)
	return newState(t, s.mode, 0, s.bitCount+n+5)
}

func (s *state) addBinaryShiftChar(index int) *state {
	t, mode, bc := s.tok, s.mode, s.bitCount
	if mode == modePunct || mode == modeDigit {
		l := latchTable[mode][modeUpper]
		t = t.add(l&0xFFFF, l>>16)
		bc += l >> 16
		mode = modeUpper
	}
	delta := 8
	switch s.shiftBytes {
	case 0, 31:
		delta = 18
	case 62:
		delta = 9
	}
	r := newState(t, mode, s.shiftBytes+1, bc+delta)
	if r.shiftBytes == 2047+31 {
		r = r.endBinaryShift(index + 1)
	}
	return r
}

func (s *state) endBinaryShift(index int) *state {
	if s.shiftBytes == 0 {
		return s
	}
	return newState(s.tok.addBinary(index-s.shiftBytes, s.shiftBytes), s.mode, 0, s.bitCount)
}

// appendFlag emits FLG(n) (via P/S unless already in Punct) followed by the
// designator digits. The mode is unchanged.
func (s *state) appendFlag(digits string, latch bool) *state {
	t, bc, mode := s.tok, s.bitCount, s.mode
	if mode != modePunct {
		if latch {
			l := latchTable[mode][modePunct]
			t = t.add(l&0xFFFF, l>>16)
			bc += l >> 16
			mode = modePunct
		} else {
			n := 5
			if mode == modeDigit {
				n = 4
			}
			t = t.add(0, n)
			bc += n
		}
	}
	t = t.add(0, 5).add(len(digits), 3)
	bc += 8
	for _, d := range digits {
		t = t.add(int(d-'0')+2, 4)
		bc += 4
	}
	return newState(t, mode, 0, bc)
}

func (s *state) betterOrEqual(o *state) bool {
	n := s.bitCount + latchTable[s.mode][o.mode]>>16
	if s.shiftBytes < o.shiftBytes {
		n += o.shiftCost - s.shiftCost
	} else if s.shiftBytes > o.shiftBytes && o.shiftBytes > 0 {
		n += 10
	}
	return n <= o.bitCount
}

func simplify(states []*state) []*state {
	var out []*state
	for _, s := range states {
		add := true
		for i := 0; i < len(out); {
			o := out[i]
			if o.betterOrEqual(s) {
				add = false
				break
			}
			if s.betterOrEqual(o) {
				out = append(out[:i], out[i+1:]...)
				continue
			}
			i++
		}
		if add {
			out = append(out, s)
		}
	}
	return out
}

func updateForChar(s *state, items []item, index int, out []*state) []*state {
	ch := items[index].b
	inCurrent := charMap[s.mode][ch] > 0
	var noBinary *state
	for mode := 0; mode < nModes; mode++ {
		c := charMap[mode][ch]
		if c <= 0 {
			continue
		}
		if noBinary == nil {
			noBinary = s.endBinaryShift(index)
		}
		if !inCurrent || mode == s.mode || mode == modeDigit {
			out = append(out, noBinary.latchAndAppend(mode, c))
		}
		if !inCurrent && shiftTable[s.mode][mode] >= 0 {
			out = append(out, noBinary.shiftAndAppend(mode, c))
		}
	}
	if s.shiftBytes > 0 || charMap[s.mode][ch] == 0 {
		out = append(out, s.addBinaryShiftChar(index))
	}
	return out
}

func updateForPair(s *state, index, pair int, out []*state) []*state {
	nb := s.endBinaryShift(index)
	out = append(out, nb.latchAndAppend(modePunct, pair))
	if s.mode != modePunct {
		out = append(out, nb.shiftAndAppend(modePunct, pair))
	}
	if pair == 3 || pair == 4 {
		// ". " or ", " in Digit mode
		out = append(out, nb.latchAndAppend(modeDigit, 16-pair).latchAndAppend(modeDigit, 1))
	}
	if s.shiftBytes > 0 {
		out = append(out, s.addBinaryShiftChar(index).addBinaryShiftChar(index+1))
	}
	return out
}

// highLevelEncode converts items into the Aztec bit stream, starting in
// Upper mode with the given preamble bits already emitted.
func highLevelEncode(items []item) []bool {
	states := []*state{newState(nil, modeUpper, 0, 0)}
	for i := 0; i < len(items); i++ {
		it := items[i]
		var out []*state
		if it.flag {
			for _, s := range states {
				nb := s.endBinaryShift(i)
				out = append(out, nb.appendFlag(it.digits, false))
				if nb.mode != modePunct {
					out = append(out, nb.appendFlag(it.digits, true))
				}
			}
			states = simplify(out)
			continue
		}
		pair := 0
		if i+1 < len(items) && !items[i+1].flag {
			next := items[i+1].b
			switch {
			case it.b == '\r' && next == '\n':
				pair = 2
			case it.b == '.' && next == ' ':
				pair = 3
			case it.b == ',' && next == ' ':
				pair = 4
			case it.b == ':' && next == ' ':
				pair = 5
			}
		}
		if pair > 0 {
			for _, s := range states {
				out = updateForPair(s, i, pair, out)
			}
			i++
		} else {
			for _, s := range states {
				out = updateForChar(s, items, i, out)
			}
		}
		states = simplify(out)
	}
	best := states[0]
	for _, s := range states[1:] {
		if s.bitCount < best.bitCount {
			best = s
		}
	}
	best = best.endBinaryShift(len(items))
	// collect tokens in order
	var toks []*token
	for t := best.tok; t != nil; t = t.prev {
		toks = append(toks, t)
	}
	var bits []bool
	for i := len(toks) - 1; i >= 0; i-- {
		t := toks[i]
		if t.bits >= 0 {
			bits = appendBits(bits, t.value, t.bits)
			continue
		}
		n := t.count
		for j := 0; j < n; j++ {
			if j == 0 || (j == 31 && n <= 62) {
				bits = appendBits(bits, 31, 5) // B/S
				switch {
				case n > 62:
					bits = appendBits(bits, n-31, 16)
				case j == 0:
					bits = appendBits(bits, min(n, 31), 5)
				default:
					bits = appendBits(bits, n-31, 5)
				}
			}
			bits = appendBits(bits, int(items[t.start+j].b), 8)
		}
	}
	return bits
}

func appendBits(b []bool, v, n int) []bool {
	for i := n - 1; i >= 0; i-- {
		b = append(b, v>>uint(i)&1 == 1)
	}
	return b
}
