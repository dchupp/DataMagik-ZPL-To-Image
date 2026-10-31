package maxicode

// Text compaction (ISO/IEC 16023 section 4.9 and Annex A): five code sets of
// 64 symbol values, switched with shifts, latches and locks, plus Numeric
// Shift which packs nine digits into six codewords. The encoder below finds a
// minimal-length codeword sequence with dynamic programming.

const (
	setA = iota
	setB
	setC
	setD
	setE
	numSets
)

// Function values inside the code set tables (anything >= 0 is a byte).
const (
	fnECI = -1 - iota
	fnPad
	fnNS
	fnShiftA
	fnShiftB
	fnShiftC
	fnShiftD
	fnShiftE
	fn2ShiftA
	fn3ShiftA
	fnLatchA
	fnLatchB
	fnLock
)

const (
	cwPad = 33 // PAD in code sets A and B
	cwNS  = 31 // Numeric Shift, all code sets
	cwECI = 27 // ECI, all code sets
)

func chars(s string) []int16 {
	out := make([]int16, 0, len(s))
	for i := 0; i < len(s); i++ {
		out = append(out, int16(s[i]))
	}
	return out
}

func cat(parts ...[]int16) [64]int16 {
	var t [64]int16
	n := 0
	for _, p := range parts {
		n += copy(t[n:], p)
	}
	if n != 64 {
		panic("maxicode: code set table size")
	}
	return t
}

func fn(f ...int16) []int16 { return f }

func rng(lo, hi int) []int16 {
	var out []int16
	for c := lo; c <= hi; c++ {
		out = append(out, int16(c))
	}
	return out
}

// codeSets is ISO/IEC 16023 Table 3 (code sets A to E) with characters given
// by their ISO/IEC 8859-1 byte values.
var codeSets = [numSets][64]int16{
	setA: cat(chars("\rABCDEFGHIJKLMNOPQRSTUVWXYZ"), fn(fnECI, 0x1c, 0x1d, 0x1e, fnNS, ' ', fnPad),
		chars("\"#$%&'()*+,-./0123456789:"), fn(fnShiftB, fnShiftC, fnShiftD, fnShiftE, fnLatchB)),
	setB: cat(chars("`abcdefghijklmnopqrstuvwxyz"), fn(fnECI, 0x1c, 0x1d, 0x1e, fnNS, '{', fnPad),
		chars("}~\x7f;<=>?[\\]^_ ,./:@!|"), fn(fnPad, fn2ShiftA, fn3ShiftA, fnPad, fnShiftA, fnShiftC, fnShiftD, fnShiftE, fnLatchA)),
	setC: cat(rng(0xc0, 0xda), fn(fnECI, 0x1c, 0x1d, 0x1e, fnNS),
		fn(0xdb, 0xdc, 0xdd, 0xde, 0xdf, 0xaa, 0xac, 0xb1, 0xb2, 0xb3, 0xb5, 0xb9, 0xba, 0xbc, 0xbd, 0xbe), rng(0x80, 0x89),
		fn(fnLatchA, ' ', fnLock, fnShiftD, fnShiftE, fnLatchB)),
	setD: cat(rng(0xe0, 0xfa), fn(fnECI, 0x1c, 0x1d, 0x1e, fnNS),
		fn(0xfb, 0xfc, 0xfd, 0xfe, 0xff, 0xa1, 0xa8, 0xab, 0xaf, 0xb0, 0xb4, 0xb7, 0xb8, 0xbb, 0xbf), rng(0x8a, 0x94),
		fn(fnLatchA, ' ', fnShiftC, fnLock, fnShiftE, fnLatchB)),
	setE: cat(rng(0x00, 0x1a), fn(fnECI, fnPad, fnPad, 0x1b, fnNS, 0x1c, 0x1d, 0x1e, 0x1f, 0x9f, 0xa0),
		fn(0xa2, 0xa3, 0xa4, 0xa5, 0xa6, 0xa7, 0xa9, 0xad, 0xae, 0xb6), rng(0x95, 0x9e),
		fn(fnLatchA, ' ', fnShiftC, fnShiftD, fnLock, fnLatchB)),
}

// charValue[set][b] is the symbol value of byte b in set, or -1.
var charValue [numSets][256]int8

// funcValue[set][f] is the symbol value of function -f-1 in set, or -1.
var funcValue [numSets][16]int8

func init() {
	for s := range codeSets {
		for i := range charValue[s] {
			charValue[s][i] = -1
		}
		for i := range funcValue[s] {
			funcValue[s][i] = -1
		}
		for v, c := range codeSets[s] {
			if c >= 0 {
				charValue[s][c] = int8(v)
			} else if funcValue[s][-c-1] < 0 {
				funcValue[s][-c-1] = int8(v)
			}
		}
	}
}

func fval(set int, f int16) byte {
	v := funcValue[set][-f-1]
	if v < 0 {
		panic("maxicode: function not in code set")
	}
	return byte(v)
}

// latchSeq returns the codewords that move the latched state from p to s.
func latchSeq(p, s int) []byte {
	switch {
	case p == s:
		return nil
	case s == setA:
		return []byte{fval(p, fnLatchA)}
	case s == setB:
		return []byte{fval(p, fnLatchB)}
	case s == setC:
		return []byte{fval(p, fnShiftC), fval(setC, fnLock)}
	case s == setD:
		return []byte{fval(p, fnShiftD), fval(setD, fnLock)}
	default:
		return []byte{fval(p, fnShiftE), fval(setE, fnLock)}
	}
}

var shiftFn = [numSets]int16{fnShiftA, fnShiftB, fnShiftC, fnShiftD, fnShiftE}

// canShift reports whether a single-character shift from set p to set t exists.
func canShift(p, t int) bool {
	return p != t && funcValue[p][-shiftFn[t]-1] >= 0
}

type op uint8

const (
	opChar   op = iota // character in the current set
	opShift            // shift to another set for one character
	op2Shift           // B: 2 Shift A
	op3Shift           // B: 3 Shift A
	opNS               // numeric shift, 9 digits
)

type step struct {
	from    int  // input position before the step
	prev    int  // latched state at `from` before latching
	latched int  // state after latching (and during the op)
	o       op   // operation
	target  int  // shift target set for opShift
	valid   bool // reachable
}

const inf = 1 << 30

// encodeText returns the minimal codeword sequence for data, starting in code
// set A, together with the final latched code set.
func encodeText(data []byte) ([]byte, int) {
	n := len(data)
	cost := make([][numSets]int, n+1)
	back := make([][numSets]step, n+1)
	for i := range cost {
		for s := range cost[i] {
			cost[i][s] = inf
		}
	}
	cost[0][setA] = 0
	back[0][setA].valid = true

	isDigit := func(b byte) bool { return b >= '0' && b <= '9' }
	digitRun := make([]int, n+1) // digits starting at i
	for i := n - 1; i >= 0; i-- {
		if isDigit(data[i]) {
			digitRun[i] = digitRun[i+1] + 1
		}
	}

	// On equal cost, prefer encoding characters in the current set and
	// single shifts over 2/3 Shift A, as Zebra printers do (a ZD420 encodes
	// "e 4 s" in set B as e, space, Shift A 4, space rather than 3 Shift A).
	rank := func(o op) int {
		if o == op2Shift || o == op3Shift {
			return 1
		}
		return 0
	}
	relax := func(j, s, c int, st step) {
		if c < cost[j][s] || c == cost[j][s] && rank(st.o) < rank(back[j][s].o) {
			cost[j][s] = c
			st.valid = true
			back[j][s] = st
		}
	}

	for i := 0; i < n; i++ {
		// Latch relaxation at position i.
		var lc [numSets]int
		var lp [numSets]int
		for s := 0; s < numSets; s++ {
			lc[s] = inf
			for p := 0; p < numSets; p++ {
				if cost[i][p] == inf {
					continue
				}
				if c := cost[i][p] + len(latchSeq(p, s)); c < lc[s] {
					lc[s], lp[s] = c, p
				}
			}
		}
		ch := data[i]
		for s := 0; s < numSets; s++ {
			if lc[s] == inf {
				continue
			}
			base := step{from: i, prev: lp[s], latched: s}
			if digitRun[i] >= 9 {
				st := base
				st.o = opNS
				relax(i+9, s, lc[s]+6, st)
			}
			if charValue[s][ch] >= 0 {
				st := base
				st.o = opChar
				relax(i+1, s, lc[s]+1, st)
			}
			for t := 0; t < numSets; t++ {
				if canShift(s, t) && charValue[t][ch] >= 0 {
					st := base
					st.o, st.target = opShift, t
					relax(i+1, s, lc[s]+2, st)
				}
			}
			if s == setB && charValue[setA][ch] >= 0 && i+1 < n && charValue[setA][data[i+1]] >= 0 {
				st := base
				st.o = op2Shift
				relax(i+2, s, lc[s]+3, st)
				if i+2 < n && charValue[setA][data[i+2]] >= 0 {
					st.o = op3Shift
					relax(i+3, s, lc[s]+4, st)
				}
			}
		}
	}

	// Choose the cheapest final state, preferring ones that can pad directly.
	best := -1
	for _, s := range []int{setA, setB, setE, setC, setD} {
		if cost[n][s] < inf && (best < 0 || cost[n][s] < cost[n][best]) {
			best = s
		}
	}

	// Backtrack into reverse-ordered chunks.
	var chunks [][]byte
	pos, state := n, best
	for pos > 0 {
		st := back[pos][state]
		var cw []byte
		cw = append(cw, latchSeq(st.prev, st.latched)...)
		seg := data[st.from:pos]
		switch st.o {
		case opChar:
			cw = append(cw, byte(charValue[st.latched][seg[0]]))
		case opShift:
			cw = append(cw, fval(st.latched, shiftFn[st.target]), byte(charValue[st.target][seg[0]]))
		case op2Shift, op3Shift:
			f := int16(fn2ShiftA)
			if st.o == op3Shift {
				f = fn3ShiftA
			}
			cw = append(cw, fval(setB, f))
			for _, b := range seg {
				cw = append(cw, byte(charValue[setA][b]))
			}
		case opNS:
			v := 0
			for _, b := range seg {
				v = v*10 + int(b-'0')
			}
			cw = append(cw, cwNS, byte(v>>24&0x3f), byte(v>>18&0x3f), byte(v>>12&0x3f), byte(v>>6&0x3f), byte(v&0x3f))
		}
		chunks = append(chunks, cw)
		pos, state = st.from, st.prev
	}
	out := make([]byte, 0, cost[n][best])
	for i := len(chunks) - 1; i >= 0; i-- {
		out = append(out, chunks[i]...)
	}
	if n == 0 {
		best = setA
	}
	return out, best
}
