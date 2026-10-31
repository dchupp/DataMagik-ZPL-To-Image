package pdf417

import "math/big"

// High-level encodation (ISO/IEC 15438 5.4): Text, Byte and Numeric
// Compaction with mode switching based on the recommended algorithm of
// Annex P, refined so that short text runs stay in Text mode when no latch is
// needed and trailing text is never forced into Byte mode.

// Mode latch / shift codewords.
const (
	CWTextLatch    = 900
	CWByteLatch    = 901
	CWNumericLatch = 902
	CWByteShift    = 913
	CWByteLatch6   = 924
	CWMacroStart   = 928
	CWMacroOption  = 923
	CWMacroEnd     = 922
	CWLinkageGS1   = 920
	CWLinkageOther = 918
	CWReaderInit   = 921
)

const (
	modeText = iota
	modeByte
	modeNumeric
	modeNone // MicroPDF417 start: Byte Compaction implied, no latch emitted yet
)

const (
	subAlpha = iota
	subLower
	subMixed
	subPunct
)

// Text sub-mode value tables (ISO/IEC 15438 Table 4). -1 = not present.
var (
	mixedVal [128]int8
	punctVal [128]int8
)

func init() {
	for i := range mixedVal {
		mixedVal[i] = -1
		punctVal[i] = -1
	}
	mixed := "0123456789&\r\t,:#-.$/+%*=^"
	for i := 0; i < len(mixed); i++ {
		mixedVal[mixed[i]] = int8(i)
	}
	punct := ";<>@[\\]_`~!\r\t,:\n-.$/\"|*()?{}'"
	for i := 0; i < len(punct); i++ {
		punctVal[punct[i]] = int8(i)
	}
}

func isText(c byte) bool {
	return c == '\t' || c == '\n' || c == '\r' || (c >= 32 && c <= 126)
}

func isUpper(c byte) bool { return c >= 'A' && c <= 'Z' }
func isLower(c byte) bool { return c >= 'a' && c <= 'z' }
func isDigit(c byte) bool { return c >= '0' && c <= '9' }
func isMixed(c byte) bool { return c < 128 && mixedVal[c] >= 0 }
func isPunct(c byte) bool { return c < 128 && punctVal[c] >= 0 }

// punctOnly reports whether c can only be encoded in the Punctuation sub-mode.
func punctOnly(c byte) bool {
	return isPunct(c) && !isMixed(c) && !isUpper(c) && !isLower(c) && c != ' '
}

// valueIn returns c's value in sub-mode sub, or -1.
func valueIn(c byte, sub int) int {
	switch sub {
	case subAlpha:
		if isUpper(c) {
			return int(c - 'A')
		}
		if c == ' ' {
			return 26
		}
	case subLower:
		if isLower(c) {
			return int(c - 'a')
		}
		if c == ' ' {
			return 26
		}
	case subMixed:
		if c == ' ' {
			return 26
		}
		if isMixed(c) {
			return int(mixedVal[c])
		}
	case subPunct:
		if isPunct(c) {
			return int(punctVal[c])
		}
	}
	return -1
}

// textValues converts a run of text characters to sub-mode values starting
// in sub-mode sub; it returns the values and the final sub-mode.
func textValues(s []byte, sub int) ([]int, int) {
	var v []int
	run := func(i int, pred func(byte) bool) int {
		n := 0
		for i+n < len(s) && pred(s[i+n]) {
			n++
		}
		return n
	}
	for i := 0; i < len(s); {
		c := s[i]
		if x := valueIn(c, sub); x >= 0 {
			v = append(v, x)
			i++
			continue
		}
		switch sub {
		case subAlpha, subLower, subMixed:
			switch {
			case isUpper(c) && sub == subLower:
				if n := run(i, isUpper); n >= 4 || (i+n == len(s) && n >= 3) {
					v = append(v, 28, 28) // ml, al
					sub = subAlpha
				} else {
					v = append(v, 27, int(c-'A')) // as (shift)
					i++
				}
			case isUpper(c): // from mixed
				v = append(v, 28) // al
				sub = subAlpha
			case isLower(c):
				v = append(v, 27) // ll
				sub = subLower
			case isMixed(c):
				v = append(v, 28) // ml
				sub = subMixed
			default: // punctuation only
				if n := run(i, punctOnly); n >= 4 || (i+n == len(s) && n >= 3) {
					if sub != subMixed {
						v = append(v, 28) // ml
					}
					v = append(v, 25) // pl
					sub = subPunct
				} else {
					v = append(v, 29, int(punctVal[c])) // ps (shift)
					i++
				}
			}
		case subPunct:
			v = append(v, 29) // al
			sub = subAlpha
		}
	}
	return v, sub
}

type encoder struct {
	cws  []int
	mode int
	sub  int
}

// Compact performs PDF417 high-level encodation of data, starting in Text
// Compaction (Alpha) mode as PDF417 decoders do. The result holds data
// codewords only (no length descriptor or padding).
func Compact(data []byte) []int {
	e := &encoder{mode: modeText, sub: subAlpha}
	e.encode(data)
	return e.cws
}

// CompactMicro is Compact for MicroPDF417, whose decoders start in Byte
// Compaction mode (ISO/IEC 24728 5.4): Text Compaction is always entered
// with an explicit 900 latch and Byte Compaction with an explicit 901/924.
func CompactMicro(data []byte) []int {
	e := &encoder{mode: modeNone, sub: subAlpha}
	e.encode(data)
	return e.cws
}

func digitRun(s []byte, p int) int {
	n := 0
	for p+n < len(s) && isDigit(s[p+n]) {
		n++
	}
	return n
}

// textRun counts text-encodable characters from p, stopping before a run of
// 13 or more digits (which goes to Numeric Compaction).
func textRun(s []byte, p int) int {
	n := 0
	for p+n < len(s) {
		if d := digitRun(s, p+n); d >= 13 {
			break
		} else if d > 0 {
			n += d
			continue
		}
		if !isText(s[p+n]) {
			break
		}
		n++
	}
	return n
}

// byteRun counts characters from p up to the next run of >= 13 digits or
// >= 5 text characters.
func byteRun(s []byte, p int) int {
	q := p
	for q < len(s) {
		if digitRun(s, q) >= 13 {
			break
		}
		t := textRun(s, q)
		if t >= 5 {
			break
		}
		if t > 0 {
			q += t
		} else {
			q++
		}
	}
	return q - p
}

func (e *encoder) encode(s []byte) {
	textCount := 0 // values emitted in the current text segment (parity)
	for p := 0; p < len(s); {
		if d := digitRun(s, p); d >= 13 {
			e.numeric(s[p : p+d])
			p += d
			continue
		}
		t := textRun(s, p)
		end := p+t == len(s)
		if t > 0 && (t >= 5 || e.mode == modeText || (end && (e.mode != modeByte || t >= 3))) {
			if e.mode != modeText {
				e.cws = append(e.cws, CWTextLatch)
				e.mode, e.sub = modeText, subAlpha
				textCount = 0
			}
			vals, sub := textValues(s[p:p+t], e.sub)
			e.sub = sub
			textCount += len(vals)
			e.cws = appendPairs(e.cws, vals)
			p += t
			continue
		}
		b := byteRun(s, p)
		if b == 0 {
			b = 1
		}
		if b == 1 && e.mode == modeText && textCount%2 == 0 {
			e.cws = append(e.cws, CWByteShift, int(s[p]))
		} else {
			e.bytes(s[p : p+b])
			textCount = 0
		}
		p += b
	}
}

// appendPairs packs text values two per codeword (30*h + l). A trailing odd
// value is completed with the ps (29) pad value; encode never follows such a
// padded run with a byte shift (it latches instead).
func appendPairs(cws, vals []int) []int {
	for i := 0; i < len(vals); i += 2 {
		h := vals[i]
		l := 29
		if i+1 < len(vals) {
			l = vals[i+1]
		}
		cws = append(cws, 30*h+l)
	}
	return cws
}

func (e *encoder) bytes(b []byte) {
	if len(b)%6 == 0 {
		e.cws = append(e.cws, CWByteLatch6)
	} else {
		e.cws = append(e.cws, CWByteLatch)
	}
	e.cws = appendByteGroups(e.cws, b)
	e.mode = modeByte
}

// ByteCompact returns the Byte Compaction codewords for b, including the
// leading latch (924 when len(b) is a multiple of 6, else 901).
func ByteCompact(b []byte) []int {
	var cws []int
	if len(b)%6 == 0 {
		cws = append(cws, CWByteLatch6)
	} else {
		cws = append(cws, CWByteLatch)
	}
	return appendByteGroups(cws, b)
}

func appendByteGroups(cws []int, b []byte) []int {
	i := 0
	for ; i+6 <= len(b); i += 6 {
		var v uint64
		for _, c := range b[i : i+6] {
			v = v<<8 | uint64(c)
		}
		var g [5]int
		for j := 4; j >= 0; j-- {
			g[j] = int(v % 900)
			v /= 900
		}
		cws = append(cws, g[:]...)
	}
	for ; i < len(b); i++ {
		cws = append(cws, int(b[i]))
	}
	return cws
}

func (e *encoder) numeric(d []byte) {
	e.cws = append(e.cws, CWNumericLatch)
	e.cws = appendNumeric(e.cws, d)
	e.mode = modeNumeric
}

// NumericCompact returns Numeric Compaction codewords (without the 902
// latch) for a string of ASCII digits.
func NumericCompact(d []byte) []int { return appendNumeric(nil, d) }

func appendNumeric(cws []int, d []byte) []int {
	n900 := big.NewInt(900)
	for i := 0; i < len(d); i += 44 {
		j := min(i+44, len(d))
		v, _ := new(big.Int).SetString("1"+string(d[i:j]), 10)
		var g []int
		m := new(big.Int)
		for v.Sign() > 0 {
			v.DivMod(v, n900, m)
			g = append(g, int(m.Int64()))
		}
		for k := len(g) - 1; k >= 0; k-- {
			cws = append(cws, g[k])
		}
	}
	return cws
}
