package databar

import (
	"strings"

	"github.com/datamagik/zpl-to-image/internal/barcode/gs1"
)

// FNC1 is the field separator used in reduced GS1 data (ASCII GS).
const FNC1 = '\x1d'

// ParseElementString converts a GS1 element string into the reduced form
// encoded by DataBar Expanded (AIs and data concatenated, FNC1 after every
// variable-length field except the last) and its interpretation text.
//
// Accepted forms:
//   - "(01)95012345678903(3103)000123" (square brackets also accepted);
//     a '(' starts a new AI only when followed by 2–4 digits and ')'.
//   - raw "0195012345678903310300012310ABC\x1d21XYZ" with FNC1 (GS, 0x1D)
//     after variable-length fields; a leading FNC1 is ignored. The text is
//     then the data without FNC1s.
func ParseElementString(s string) (reduced, text string, err error) {
	if s == "" {
		return "", "", invalid("empty GS1 element string")
	}
	if s[0] != '(' && s[0] != '[' {
		r := strings.TrimPrefix(s, string(FNC1))
		r = strings.TrimSuffix(r, string(FNC1))
		if r == "" {
			return "", "", invalid("empty GS1 element string")
		}
		return r, strings.ReplaceAll(r, string(FNC1), ""), nil
	}
	var red, txt strings.Builder
	pos := 0
	for pos < len(s) {
		ai, n := aiAt(s, pos)
		if n == 0 {
			return "", "", invalid("expected (AI) at offset %d in %q", pos, s)
		}
		pos += n
		end := pos
		for end < len(s) {
			if _, m := aiAt(s, end); m > 0 {
				break
			}
			end++
		}
		val := s[pos:end]
		pos = end
		if val == "" {
			return "", "", invalid("AI (%s) has no data", ai)
		}
		fixed, isFixed := gs1.PredefinedLength(ai)
		if isFixed && len(ai)+len(val) != fixed {
			return "", "", invalid("AI (%s) needs %d data characters, got %q", ai, fixed-len(ai), val)
		}
		red.WriteString(ai)
		red.WriteString(val)
		if !isFixed && pos < len(s) {
			red.WriteByte(FNC1)
		}
		txt.WriteString("(" + ai + ")" + val)
	}
	return red.String(), txt.String(), nil
}

// aiAt reports the AI digits of a "(nn)".."(nnnn)" (or [..]) token at i.
func aiAt(s string, i int) (string, int) {
	if i >= len(s) {
		return "", 0
	}
	return gs1.AI(s[i:])
}
