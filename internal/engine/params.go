package engine

import (
	"strconv"
	"strings"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/bitmap"
)

// params are a command's delimiter-separated parameters.
type params []string

func (ps params) raw(i int) string {
	if i < len(ps) {
		return ps[i]
	}
	return ""
}

// str returns parameter i with surrounding spaces removed.
func (ps params) str(i int) string { return strings.TrimSpace(ps.raw(i)) }

func (ps params) has(i int) bool { return ps.str(i) != "" }

// int parses parameter i, returning def when it is missing or not a number.
// Like the printer, it accepts a leading integer and ignores trailing junk.
func (ps params) int(i, def int) int {
	v, ok := parseInt(ps.str(i))
	if !ok {
		return def
	}
	return v
}

func parseInt(s string) (int, bool) {
	end := 0
	if end < len(s) && (s[end] == '-' || s[end] == '+') {
		end++
	}
	digits := end
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	if end == digits {
		return 0, false
	}
	v, err := strconv.Atoi(s[:end])
	return v, err == nil
}

func (ps params) float(i int, def float64) float64 {
	v, err := strconv.ParseFloat(ps.str(i), 64)
	if err != nil {
		return def
	}
	return v
}

// char returns the upper-cased first byte of parameter i, or def.
func (ps params) char(i int, def byte) byte {
	s := ps.str(i)
	if s == "" {
		return def
	}
	c := s[0]
	if c >= 'a' && c <= 'z' {
		c -= 'a' - 'A'
	}
	return c
}

// yes parses a Y/N parameter.
func (ps params) yes(i int, def bool) bool {
	switch ps.char(i, 0) {
	case 'Y':
		return true
	case 'N':
		return false
	}
	return def
}

// rotation parses an N/R/I/B orientation.
func parseRotation(c byte) (bitmap.Rotation, bool) {
	switch c {
	case 'N':
		return bitmap.R0, true
	case 'R':
		return bitmap.R90, true
	case 'I':
		return bitmap.R180, true
	case 'B':
		return bitmap.R270, true
	}
	return bitmap.R0, false
}

func clamp(v, lo, hi int) int { return min(max(v, lo), hi) }
