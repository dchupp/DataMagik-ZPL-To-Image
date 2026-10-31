package datamatrix

import (
	"fmt"

	"github.com/datamagik/zpl-to-image/internal/barcode"
)

// itemKind classifies one unit of input after escape processing.
type itemKind uint8

const (
	kByte itemKind = iota
	kFNC1
	kFNC3
	kECI
)

// item is one input unit: a data byte, a function character or an ECI.
type item struct {
	kind itemKind
	v    int
}

// Encodation modes (ISO/IEC 16022 5.2).
const (
	mASCII = iota
	mC40
	mText
	mX12
	mEDF
	numModes
)

// Codeword values.
const (
	cwLatchC40   = 230
	cwLatchB256  = 231
	cwFNC1       = 232
	cwSA         = 233
	cwFNC3       = 234
	cwUpperShift = 235
	cwMacro05    = 236
	cwMacro06    = 237
	cwLatchX12   = 238
	cwLatchText  = 239
	cwLatchEDF   = 240
	cwECI        = 241
	cwUnlatch    = 254
	cwPad        = 129
)

// c40Values returns the C40 (text=false) or Text (text=true) values for one
// item, or nil when the item cannot be encoded in that set.
func c40Values(it item, text bool) []int {
	switch it.kind {
	case kFNC1:
		return []int{1, 27}
	case kByte:
	default:
		return nil
	}
	c := it.v
	if c >= 128 {
		return append([]int{1, 30}, c40Values(item{kByte, c - 128}, text)...)
	}
	switch {
	case c == ' ':
		return []int{3}
	case c >= '0' && c <= '9':
		return []int{c - '0' + 4}
	case !text && c >= 'A' && c <= 'Z':
		return []int{c - 'A' + 14}
	case text && c >= 'a' && c <= 'z':
		return []int{c - 'a' + 14}
	case c < 32:
		return []int{0, c}
	case c >= 33 && c <= 47:
		return []int{1, c - 33}
	case c >= 58 && c <= 64:
		return []int{1, c - 58 + 15}
	case c >= 91 && c <= 95:
		return []int{1, c - 91 + 22}
	case text && c >= 'A' && c <= 'Z':
		return []int{2, c - 64}
	default: // 96..127 (C40: ` a-z { | } ~ DEL; Text: ` { | } ~ DEL)
		return []int{2, c - 96}
	}
}

// x12Value returns the X12 value of it or -1.
func x12Value(it item) int {
	if it.kind != kByte {
		return -1
	}
	c := it.v
	switch {
	case c == 13:
		return 0
	case c == '*':
		return 1
	case c == '>':
		return 2
	case c == ' ':
		return 3
	case c >= '0' && c <= '9':
		return c - '0' + 4
	case c >= 'A' && c <= 'Z':
		return c - 'A' + 14
	}
	return -1
}

// edfValue returns the EDIFACT value of it or -1.
func edfValue(it item) int {
	if it.kind != kByte || it.v < 32 || it.v > 94 {
		return -1
	}
	return it.v & 0x3f
}

func isDigit(it item) bool { return it.kind == kByte && it.v >= '0' && it.v <= '9' }

// eciBytes encodes an ECI assignment number (ISO/IEC 16022 5.4.1).
func eciBytes(n int) []byte {
	switch {
	case n <= 126:
		return []byte{byte(n + 1)}
	case n <= 16382:
		n -= 127
		return []byte{byte(n/254 + 128), byte(n%254 + 1)}
	default:
		n -= 16383
		return []byte{byte(n/64516 + 192), byte((n/254)%254 + 1), byte(n%254 + 1)}
	}
}

func asciiCost(it item) int {
	switch it.kind {
	case kECI:
		return 1 + len(eciBytes(it.v))
	case kByte:
		if it.v >= 128 {
			return 2
		}
	}
	return 1
}

func b256Header(k int) int {
	if k <= 249 {
		return 1
	}
	return 2
}

// Edge types of the encodation graph.
const (
	eASCII1  = iota // one item in ASCII
	eASCII2         // digit pair in ASCII
	eLatch          // ASCII -> C40/Text/X12/EDIFACT
	eUnlatch        // C40/Text/X12 -> ASCII at a triplet boundary
	eC40Unit        // items completing whole C40/Text triplets
	eC40EOD         // trailing items at end of data padded with Shift 1, then unlatch
	eX12Unit        // three X12 items
	eEDFUnit        // four EDIFACT items
	eEDFExit        // 0..3 EDIFACT items followed by unlatch
	eB256           // a Base 256 segment of n bytes
)

type edge struct {
	pos, mode int // predecessor state
	typ, n    int // edge type and number of items consumed
	dst       int // destination mode (filled in when the path is built)
}

const inf = 1 << 30

// c40Unit returns how many items starting at p complete whole triplets in
// C40/Text and the number of values, or 0 when impossible. eod reports that
// the end of data was reached with two values left over (a valid
// end-of-data partial triplet).
func c40Unit(items []item, p int, text bool) (n, vals int, eod bool) {
	total := 0
	for i := p; i < len(items); i++ {
		v := c40Values(items[i], text)
		if v == nil {
			return 0, 0, false
		}
		total += len(v)
		if total%3 == 0 {
			return i - p + 1, total, false
		}
	}
	if total%3 == 2 {
		return len(items) - p, total, true
	}
	return 0, 0, false
}

// plan finds a minimal-length encodation of items starting in ASCII mode
// (shortest path over a graph whose states are input position × mode).
func plan(items []item) []edge {
	n := len(items)
	dist := make([][numModes]int, n+1)
	prev := make([][numModes]edge, n+1)
	for i := range dist {
		for m := range dist[i] {
			dist[i][m] = inf
		}
	}
	dist[0][mASCII] = 0
	relax := func(p, m, cost int, e edge) {
		if p <= n && cost < dist[p][m] {
			dist[p][m] = cost
			prev[p][m] = e
		}
	}
	for p := 0; p <= n; p++ {
		// Return to ASCII.
		for _, m := range []int{mC40, mText, mX12} {
			if d := dist[p][m]; d < inf {
				relax(p, mASCII, d+1, edge{p, m, eUnlatch, 0, 0})
			}
		}
		if d := dist[p][mEDF]; d < inf {
			for r := 0; r <= 3 && p+r <= n; r++ {
				if r > 0 && edfValue(items[p+r-1]) < 0 {
					break
				}
				relax(p+r, mASCII, d+(6*(r+1)+7)/8, edge{p, mEDF, eEDFExit, r, 0})
			}
		}
		if p == n {
			break
		}
		// Latches from ASCII.
		if d := dist[p][mASCII]; d < inf {
			for _, m := range []int{mC40, mText, mX12, mEDF} {
				relax(p, m, d+1, edge{p, mASCII, eLatch, 0, 0})
			}
		}
		// Forward edges.
		if d := dist[p][mASCII]; d < inf {
			if p+1 < n && isDigit(items[p]) && isDigit(items[p+1]) {
				relax(p+2, mASCII, d+1, edge{p, mASCII, eASCII2, 2, 0})
			}
			relax(p+1, mASCII, d+asciiCost(items[p]), edge{p, mASCII, eASCII1, 1, 0})
			for k := 1; p+k <= n && items[p+k-1].kind == kByte; k++ {
				relax(p+k, mASCII, d+1+b256Header(k)+k, edge{p, mASCII, eB256, k, 0})
			}
		}
		for _, m := range []int{mC40, mText} {
			d := dist[p][m]
			if d == inf {
				continue
			}
			k, vals, eod := c40Unit(items, p, m == mText)
			if k == 0 {
				continue
			}
			if eod {
				relax(n, mASCII, d+2*(vals+1)/3+1, edge{p, m, eC40EOD, k, 0})
			} else {
				relax(p+k, m, d+2*vals/3, edge{p, m, eC40Unit, k, 0})
			}
		}
		if d := dist[p][mX12]; d < inf && p+3 <= n &&
			x12Value(items[p]) >= 0 && x12Value(items[p+1]) >= 0 && x12Value(items[p+2]) >= 0 {
			relax(p+3, mX12, d+2, edge{p, mX12, eX12Unit, 3, 0})
		}
		if d := dist[p][mEDF]; d < inf && p+4 <= n {
			ok := true
			for i := p; i < p+4; i++ {
				ok = ok && edfValue(items[i]) >= 0
			}
			if ok {
				relax(p+4, mEDF, d+3, edge{p, mEDF, eEDFUnit, 4, 0})
			}
		}
	}
	var path []edge
	p, m := n, mASCII
	for p != 0 || m != mASCII {
		e := prev[p][m]
		e.dst = m
		path = append(path, e)
		p, m = e.pos, e.mode
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}

// stream is the codeword stream under construction.
type stream struct {
	cw   []byte
	b256 []bool // codewords subject to 255-state randomisation
	// dropUnlatch is the index of a C40/Text/X12 unlatch that may be omitted
	// if the symbol is otherwise exactly full (-1 when none).
	dropUnlatch int
}

func (s *stream) add(b ...byte) {
	s.cw = append(s.cw, b...)
	s.b256 = append(s.b256, make([]bool, len(b))...)
}

func packTriplets(vals []int) []byte {
	var out []byte
	for i := 0; i+2 < len(vals); i += 3 {
		v := 1600*vals[i] + 40*vals[i+1] + vals[i+2] + 1
		out = append(out, byte(v/256), byte(v%256))
	}
	return out
}

func packEDF(vals []int) []byte {
	var out []byte
	acc, nb := 0, 0
	for _, v := range vals {
		acc = acc<<6 | v
		nb += 6
		for nb >= 8 {
			out = append(out, byte(acc>>(nb-8)))
			nb -= 8
			acc &= 1<<nb - 1
		}
	}
	if nb > 0 {
		out = append(out, byte(acc<<(8-nb)))
	}
	return out
}

// encodeItems produces the data codewords (without padding) for items.
func encodeItems(s *stream, items []item) {
	path := plan(items)
	pos := 0
	s.dropUnlatch = -1
	for _, e := range path {
		switch e.typ {
		case eASCII1:
			it := items[pos]
			switch it.kind {
			case kFNC1:
				s.add(cwFNC1)
			case kFNC3:
				s.add(cwFNC3)
			case kECI:
				s.add(cwECI)
				s.add(eciBytes(it.v)...)
			default:
				if it.v >= 128 {
					s.add(cwUpperShift, byte(it.v-127))
				} else {
					s.add(byte(it.v + 1))
				}
			}
		case eASCII2:
			s.add(byte(130 + (items[pos].v-'0')*10 + items[pos+1].v - '0'))
		case eLatch:
			s.add([...]byte{mC40: cwLatchC40, mText: cwLatchText, mX12: cwLatchX12, mEDF: cwLatchEDF}[e.dst])
		case eUnlatch:
			s.dropUnlatch = len(s.cw)
			s.add(cwUnlatch)
		case eC40Unit, eC40EOD:
			text := e.mode == mText
			var vals []int
			for _, it := range items[pos : pos+e.n] {
				vals = append(vals, c40Values(it, text)...)
			}
			if e.typ == eC40EOD {
				vals = append(vals, 0)
				s.add(packTriplets(vals)...)
				s.dropUnlatch = len(s.cw)
				s.add(cwUnlatch)
			} else {
				s.add(packTriplets(vals)...)
			}
		case eX12Unit:
			s.add(packTriplets([]int{x12Value(items[pos]), x12Value(items[pos+1]), x12Value(items[pos+2])})...)
		case eEDFUnit, eEDFExit:
			var vals []int
			for _, it := range items[pos : pos+e.n] {
				vals = append(vals, edfValue(it))
			}
			if e.typ == eEDFExit {
				vals = append(vals, 31)
			}
			s.add(packEDF(vals)...)
		case eB256:
			start := len(s.cw)
			s.add(cwLatchB256)
			if e.n <= 249 {
				s.add(byte(e.n))
			} else {
				s.add(byte(e.n/250+249), byte(e.n%250))
			}
			for _, it := range items[pos : pos+e.n] {
				s.add(byte(it.v))
			}
			for i := start + 1; i < len(s.cw); i++ {
				s.b256[i] = true
			}
		}
		pos += e.n
	}
}

// finish pads the stream to capacity and applies randomisation.
func (s *stream) finish(capacity int) []byte {
	out := make([]byte, capacity)
	copy(out, s.cw)
	for i := range s.cw {
		if s.b256[i] {
			r := (149*(i+1))%255 + 1
			v := int(out[i]) + r
			if v > 255 {
				v -= 256
			}
			out[i] = byte(v)
		}
	}
	for i := len(s.cw); i < capacity; i++ {
		if i == len(s.cw) {
			out[i] = cwPad
			continue
		}
		r := (149*(i+1))%253 + 1
		v := cwPad + r
		if v > 254 {
			v -= 254
		}
		out[i] = byte(v)
	}
	return out
}

// fits reports whether the stream fits capacity, dropping a trailing
// optional unlatch when that makes it fit exactly.
func (s *stream) fits(capacity int) bool {
	if len(s.cw) <= capacity {
		return true
	}
	return len(s.cw) == capacity+1 && s.canDrop()
}

// canDrop reports whether the last unlatch may be omitted: it is the final
// codeword, or it is followed by a single ASCII codeword for one character.
func (s *stream) canDrop() bool {
	d := s.dropUnlatch
	if d < 0 {
		return false
	}
	if d == len(s.cw)-1 {
		return true
	}
	return d == len(s.cw)-2 && s.cw[d+1] >= 1 && s.cw[d+1] <= 128 && !s.b256[d+1]
}

func (s *stream) drop() {
	d := s.dropUnlatch
	s.cw = append(s.cw[:d], s.cw[d+1:]...)
	s.b256 = append(s.b256[:d], s.b256[d+1:]...)
	s.dropUnlatch = -1
}

func errf(format string, a ...any) error {
	return fmt.Errorf("%w: datamatrix: "+format, append([]any{barcode.ErrInvalidData}, a...)...)
}
