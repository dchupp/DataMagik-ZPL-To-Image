package composite

import (
	"fmt"
	"strings"

	"github.com/datamagik/zpl-to-image/internal/barcode"
	"github.com/datamagik/zpl-to-image/internal/barcode/gs1"
)

// GS (ASCII 29) stands for FNC1 inside an element string.
const GS = 0x1D

// bitWriter accumulates a bit string.
type bitWriter struct{ b []bool }

func (w *bitWriter) put(v, n int) {
	for i := n - 1; i >= 0; i-- {
		w.b = append(w.b, v>>uint(i)&1 == 1)
	}
}

func (w *bitWriter) len() int { return len(w.b) }

// ElementString converts GS1 data to the FNC1-separated element string
// that is encoded (no leading FNC1; GS after each variable-length element
// that is followed by another). Input may use bracketed AIs
// "(01)09521234543213(10)ABC" or already be an element string, optionally
// beginning with FNC1 (GS) or a "]C1"/"]e0" symbology identifier.
func ElementString(s string) (string, error) {
	if !strings.HasPrefix(s, "(") {
		for _, p := range []string{"]C1", "]e0", "]d2", "]Q3"} {
			s = strings.TrimPrefix(s, p)
		}
		s = strings.TrimLeft(s, "\x1d")
		if s == "" {
			return "", fmt.Errorf("%w: composite: empty 2D component", barcode.ErrInvalidData)
		}
		return s, nil
	}
	var b strings.Builder
	for len(s) > 0 {
		if s[0] != '(' {
			return "", fmt.Errorf("%w: composite: expected '(' in %q", barcode.ErrInvalidData, s)
		}
		end := strings.IndexByte(s, ')')
		if end < 3 || end > 5 {
			return "", fmt.Errorf("%w: composite: bad AI in %q", barcode.ErrInvalidData, s)
		}
		ai := s[1:end]
		for i := 0; i < len(ai); i++ {
			if ai[i] < '0' || ai[i] > '9' {
				return "", fmt.Errorf("%w: composite: bad AI %q", barcode.ErrInvalidData, ai)
			}
		}
		s = s[end+1:]
		// Data runs to the next "(digits)" AI.
		n := len(s)
		for i := 0; i < len(s); i++ {
			if _, m := gs1.AI(s[i:]); s[i] == '(' && m > 0 {
				n = i
				break
			}
		}
		if n == 0 {
			return "", fmt.Errorf("%w: composite: AI %s has no data", barcode.ErrInvalidData, ai)
		}
		b.WriteString(ai)
		b.WriteString(s[:n])
		s = s[n:]
		if _, fixed := gs1.PredefinedLength(ai); len(s) > 0 && !fixed {
			b.WriteByte(GS)
		}
	}
	return b.String(), nil
}

// General-purpose compaction modes (ISO/IEC 24723 5.4).
const (
	gfNumeric = iota
	gfAlpha
	gfISO
)

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
func isUpper(c byte) bool { return c >= 'A' && c <= 'Z' }
func isLower(c byte) bool { return c >= 'a' && c <= 'z' }

const alnumPunct = "*,-./"
const isoPunct = "!\"%&'()*+,-./:;<=>?_ "

func gfType(c byte) int {
	switch {
	case c == GS || isDigit(c):
		return gfNumeric
	case isUpper(c) || strings.IndexByte(alnumPunct, c) >= 0:
		return gfAlpha
	case isLower(c) || strings.IndexByte(isoPunct, c) >= 0:
		return gfISO
	}
	return -1
}

// nextAll reports whether the num characters from i exist and are all of
// type t (or t2 when t2 >= 0).
func nextAll(s string, i, num, t, t2 int) bool {
	if i+num > len(s) {
		return false
	}
	for k := i; k < i+num; k++ {
		ty := gfType(s[k])
		if ty != t && (t2 < 0 || ty != t2) {
			return false
		}
	}
	return true
}

// nextTerminate reports whether all remaining characters (at least num, at
// most maxNum) are of type t.
func nextTerminate(s string, i, num, maxNum, t int) bool {
	if i+maxNum < len(s) || len(s)-i < num {
		return false
	}
	for k := i; k < len(s); k++ {
		if gfType(s[k]) != t {
			return false
		}
	}
	return true
}

// nextNone reports whether none of the next num characters (or up to the
// end) is of type t.
func nextNone(s string, i, num, t int) bool {
	for k := i; k < len(s) && k < i+num; k++ {
		if gfType(s[k]) == t {
			return false
		}
	}
	return true
}

func digitVal(c byte) int {
	if c == GS {
		return 10
	}
	return int(c - '0')
}

// generalField encodes s with the general-purpose compaction rules of
// ISO/IEC 24723 5.4 (identical to ISO/IEC 24724 7.2.5.5), starting in mode.
// It returns the final mode and a pending final digit (0 if none).
func generalField(w *bitWriter, s string, mode int) (int, byte, error) {
	for i := 0; i < len(s); {
		t := gfType(s[i])
		if t < 0 {
			return mode, 0, fmt.Errorf("%w: composite: character %q not encodable", barcode.ErrInvalidData, s[i])
		}
		switch mode {
		case gfNumeric:
			if i < len(s)-1 {
				if t != gfNumeric || gfType(s[i+1]) != gfNumeric {
					w.put(0, 4) // alphanumeric latch
					mode = gfAlpha
				} else {
					w.put(11*digitVal(s[i])+digitVal(s[i+1])+8, 7)
					i += 2
				}
			} else {
				if t != gfNumeric {
					w.put(0, 4)
					mode = gfAlpha
				} else {
					return mode, s[i], nil // single final digit
				}
			}
		case gfAlpha:
			switch {
			case s[i] == GS:
				w.put(15, 5)
				mode = gfNumeric
				i++
			case t == gfISO:
				w.put(4, 5) // ISO/IEC 646 latch
				mode = gfISO
			case nextAll(s, i, 6, gfNumeric, -1), nextTerminate(s, i, 4, 5, gfNumeric):
				w.put(0, 3) // numeric latch
				mode = gfNumeric
			case isDigit(s[i]):
				w.put(int(s[i])-43, 5)
				i++
			case isUpper(s[i]):
				w.put(int(s[i])-33, 6)
				i++
			default:
				w.put(strings.IndexByte(alnumPunct, s[i])+58, 6)
				i++
			}
		case gfISO:
			if s[i] == GS {
				w.put(15, 5)
				mode = gfNumeric
				i++
				continue
			}
			none := nextNone(s, i, 10, gfISO)
			switch {
			case none && nextAll(s, i, 4, gfNumeric, -1):
				w.put(0, 3)
				mode = gfNumeric
			case none && nextAll(s, i, 5, gfAlpha, gfNumeric):
				w.put(4, 5) // alphanumeric latch
				mode = gfAlpha
			case isDigit(s[i]):
				w.put(int(s[i])-43, 5)
				i++
			case isUpper(s[i]):
				w.put(int(s[i])-1, 7)
				i++
			case isLower(s[i]):
				w.put(int(s[i])-7, 7)
				i++
			default:
				w.put(strings.IndexByte(isoPunct, s[i])+232, 8)
				i++
			}
		}
	}
	return mode, 0, nil
}

// dateValue returns the 16-bit YYMMDD compaction value of the 6 digits at
// s[p:], or -1 if they are not a valid date (ISO/IEC 24724 7.2.5.4.6).
func dateValue(s string, p int) int {
	if p+6 > len(s) {
		return -1
	}
	v := 0
	for i := p; i < p+6; i++ {
		if !isDigit(s[i]) {
			return -1
		}
	}
	yy := int(s[p]-'0')*10 + int(s[p+1]-'0')
	mm := int(s[p+2]-'0')*10 + int(s[p+3]-'0')
	dd := int(s[p+4]-'0')*10 + int(s[p+5]-'0')
	if mm < 1 || mm > 12 || dd > 31 {
		return -1
	}
	v = yy*384 + (mm-1)*32 + dd
	return v
}

// bitString builds the 2D component bit stream (ISO/IEC 24723 clause 5):
// encodation method field, compressed AI field and general-purpose field.
// target maps the unpadded length to the padded length (0 = too long).
func bitString(src string, target func(n int) int) ([]bool, error) {
	w := &bitWriter{}
	read := 0
	mode := gfNumeric
	fnc1 := false
	alphaPad := false
	cropAt, cropLen := -1, 0

	switch {
	case len(src) > 1 && src[0] == '1' && (src[1] == '0' || ((src[1] == '1' || src[1] == '7') && dateValue(src, 2) >= 0)):
		w.put(2, 2) // "10"
		if src[1] == '0' {
			w.put(3, 2) // no date
			read = 2
		} else {
			w.put(dateValue(src, 2), 16)
			if src[1] == '1' {
				w.put(0, 1)
			} else {
				w.put(1, 1)
			}
			read = 8
			switch {
			case read+1 < len(src) && src[read] == '1' && src[read+1] == '0':
				read += 2
			case read < len(src):
				fnc1 = true
			default:
				// FNC1 still required with nothing following: alphanumeric
				// latch then alphanumeric FNC1 (returns to numeric).
				w.put(15, 9)
			}
		}
	case len(src) > 1 && src[0] == '9' && src[1] == '0':
		ok := method90(w, src, &read, &mode, &alphaPad, &cropAt, &cropLen)
		if !ok {
			w.put(0, 1)
		}
	default:
		w.put(0, 1)
	}

	var gf strings.Builder
	if fnc1 {
		gf.WriteByte(GS)
	}
	for i := read; i < len(src); i++ {
		if i == cropAt {
			i += cropLen - 1
			continue
		}
		gf.WriteByte(src[i])
	}
	var last byte
	if gf.Len() > 0 {
		alphaPad = false
		var err error
		if mode, last, err = generalField(w, gf.String(), mode); err != nil {
			return nil, err
		}
	}
	t := target(w.len())
	if t == 0 {
		return nil, fmt.Errorf("%w: composite: 2D component data too long", barcode.ErrInvalidData)
	}
	if last != 0 {
		rem := t - w.len()
		if rem >= 4 && rem <= 6 {
			w.put(int(last-'0')+1, 4)
			w.put(0, rem-4)
		} else {
			w.put(11*int(last-'0')+18, 7)
		}
		if t = target(w.len()); t == 0 {
			return nil, fmt.Errorf("%w: composite: 2D component data too long", barcode.ErrInvalidData)
		}
	}
	if w.len() < t {
		if alphaPad {
			w.put(31, 5)
		}
		if mode == gfNumeric {
			w.put(0, 4)
		}
		for w.len() < t {
			w.put(4, 5)
		}
	}
	return w.b[:t], nil
}

// method90 applies encodation method "11" (ISO/IEC 24723 5.3.2) for data
// starting with AI 90 when its format allows; it reports whether it did.
func method90(w *bitWriter, src string, read, mode *int, alphaPad *bool, cropAt, cropLen *int) bool {
	end := strings.IndexByte(src[2:], GS)
	if end < 0 {
		end = len(src)
	} else {
		end += 2
	}
	ninety := src[2:end]
	alpha, numeric, other := 0, 0, 0
	for i := 0; i < len(ninety); i++ {
		switch c := ninety[i]; {
		case isUpper(c):
			alpha++
		case isDigit(c):
			numeric++
		default:
			other++
		}
	}
	alphaPos := -1
	if len(ninety) > 0 && ninety[0] != '0' {
		for i := 0; i < len(ninety) && i < 4; i++ {
			if isUpper(ninety[i]) {
				alphaPos = i
				break
			}
			if !isDigit(ninety[i]) {
				break
			}
		}
	}
	if alphaPos < 0 {
		return false
	}
	w.put(3, 2) // "11"
	numeric -= alphaPos
	alpha--
	ai90Alpha := false
	switch {
	case other == 0 && alpha > numeric:
		w.put(3, 2) // alpha mode
		ai90Alpha = true
	case other == 0 && alpha == 0:
		w.put(2, 2) // numeric mode
	default:
		w.put(0, 1) // alphanumeric
		*mode = gfAlpha
	}
	crop := 0
	if end < len(src) && src[end] == GS {
		if end+2 < len(src) && src[end+1] == '2' && src[end+2] == '1' {
			crop = 1
		} else if end+4 < len(src) && src[end+1:end+5] == "8004" {
			crop = 3
		}
	}
	switch crop {
	case 0:
		w.put(0, 1)
	case 1:
		w.put(2, 2)
		*cropAt, *cropLen = end+1, 2
	case 3:
		w.put(3, 2)
		*cropAt, *cropLen = end+1, 4
	}
	num := 0
	for i := 0; i < alphaPos; i++ {
		num = num*10 + int(ninety[i]-'0')
	}
	letter := -1
	if num < 31 {
		letter = strings.IndexByte("BDHIJKLNPQRSTVWZ", ninety[alphaPos])
	}
	if letter >= 0 {
		w.put(num, 5)
		w.put(letter, 4)
	} else {
		w.put(31, 5)
		w.put(num, 10)
		w.put(int(ninety[alphaPos])-65, 5)
	}
	*read = alphaPos + 3
	if ai90Alpha {
		for {
			c := byte(0)
			if *read < len(src) {
				c = src[*read]
			}
			switch {
			case isUpper(c):
				w.put(int(c)-65, 5)
			case isDigit(c):
				w.put(int(c)+4, 6)
			case c == GS:
				w.put(31, 5)
			}
			*read++
			if c == GS || c == 0 {
				break
			}
		}
		if *read > len(src) {
			*read = len(src)
		}
		*alphaPad = true
	}
	return true
}
