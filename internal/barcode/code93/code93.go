// Package code93 encodes Code 93 (ZPL ^BA) with Full ASCII support and the
// two mandatory check characters C and K.
package code93

import (
	"fmt"
	"strings"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
)

// Options carries the parsed ^BA parameters relevant to encoding.
type Options struct {
	// PrintCheckDigits is the ^BA e parameter: show the C and K check
	// characters in the interpretation line. They are always encoded.
	PrintCheckDigits bool
}

// alphabet lists the 43 native characters in value order; values 43..46 are
// the shift characters ($) (%) (/) (+).
const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ-. $/+%"

const (
	shiftDollar  = 43 // ($)
	shiftPercent = 44 // (%)
	shiftSlash   = 45 // (/)
	shiftPlus    = 46 // (+)
)

// zebraShift maps the ZPL II substitute characters to the Code 93 shift
// characters: & = ($), ' = (%), ( = (/), ) = (+).
var zebraShift = map[byte]int{'&': shiftDollar, '\'': shiftPercent, '(': shiftSlash, ')': shiftPlus}

// patterns gives module widths (bar, space, bar, space, bar, space) for
// values 0..46; start/stop is "111141".
var patterns = [47]string{
	"131112", "111213", "111312", "111411", "121113", "121212", "121311", "111114", "131211", "141111",
	"211113", "211212", "211311", "221112", "221211", "231111", "112113", "112212", "112311", "122112",
	"132111", "111123", "111222", "111321", "121122", "131121", "212112", "212211", "211122", "211221",
	"221121", "222111", "112122", "112221", "122121", "123111", "121131", "311112", "311211", "321111",
	"112131", "113121", "211131", "121221", "312111", "311121", "122211",
}

const startStop = "111141"

// fullASCII returns the Code 93 Full ASCII values for ASCII character c.
func fullASCII(c byte) []int {
	ch := func(b byte) int { return strings.IndexByte(alphabet, b) }
	switch {
	case c == 0:
		return []int{shiftPercent, ch('U')}
	case c >= 1 && c <= 26:
		return []int{shiftDollar, ch('A' + c - 1)}
	case c >= 27 && c <= 31:
		return []int{shiftPercent, ch('A' + c - 27)}
	case c >= '!' && c <= ',', c == '/':
		return []int{shiftSlash, ch('A' + c - '!')}
	case c == ':':
		return []int{shiftSlash, ch('Z')}
	case c >= ';' && c <= '?':
		return []int{shiftPercent, ch('F' + c - ';')}
	case c == '@':
		return []int{shiftPercent, ch('V')}
	case c >= '[' && c <= '_':
		return []int{shiftPercent, ch('K' + c - '[')}
	case c == '`':
		return []int{shiftPercent, ch('W')}
	case c >= 'a' && c <= 'z':
		return []int{shiftPlus, ch('A' + c - 'a')}
	case c >= '{' && c <= 127:
		return []int{shiftPercent, ch('P' + c - '{')}
	}
	return nil
}

// decodePair returns the ASCII character a shift value plus a native
// character stands for, for the interpretation line.
func decodePair(shift int, c byte) (byte, bool) {
	for a := 0; a < 128; a++ {
		v := fullASCII(byte(a))
		if len(v) == 2 && v[0] == shift && alphabet[v[1]] == c {
			return byte(a), true
		}
	}
	return 0, false
}

// Encode encodes data as Code 93.
//
// Following the ZPL II guide, the characters & ' ( ) in the field data are
// the shift characters ($) (%) (/) (+); a shift character together with the
// following character prints as the ASCII character the pair stands for.
// Other characters outside the native set (e.g. lowercase letters) are
// encoded automatically with their Full ASCII pair.
func Encode(data string, opts Options) (barcode.Linear, error) {
	if data == "" {
		return barcode.Linear{}, fmt.Errorf("%w: code93: no data", barcode.ErrInvalidData)
	}
	var v []int
	var text strings.Builder
	for i := 0; i < len(data); i++ {
		c := data[i]
		if k := strings.IndexByte(alphabet, c); k >= 0 {
			v = append(v, k)
			text.WriteByte(c)
			continue
		}
		if s, ok := zebraShift[c]; ok {
			if i+1 >= len(data) || strings.IndexByte(alphabet, data[i+1]) < 0 {
				return barcode.Linear{}, fmt.Errorf("%w: code93: shift %q must be followed by A-Z or a native character", barcode.ErrInvalidData, c)
			}
			a, ok := decodePair(s, data[i+1])
			if !ok {
				return barcode.Linear{}, fmt.Errorf("%w: code93: %q%q is not a full-ASCII pair", barcode.ErrInvalidData, c, data[i+1])
			}
			i++
			v = append(v, s, strings.IndexByte(alphabet, data[i]))
			text.WriteByte(a)
			continue
		}
		if c >= 128 {
			return barcode.Linear{}, fmt.Errorf("%w: code93: character %q not encodable", barcode.ErrInvalidData, c)
		}
		v = append(v, fullASCII(c)...)
		text.WriteByte(c)
	}
	cc := checkChar(v, 20)
	v = append(v, cc)
	kc := checkChar(v, 15)
	v = append(v, kc)
	if opts.PrintCheckDigits {
		text.WriteString(checkText(cc) + checkText(kc))
	}

	var el []int
	add := func(p string) {
		for _, b := range p {
			el = append(el, int(b-'0'))
		}
	}
	add(startStop)
	for _, x := range v {
		add(patterns[x])
	}
	add(startStop)
	el = append(el, 1) // termination bar
	return barcode.Linear{Elements: el, Text: text.String()}, nil
}

// checkText prints a check value; shift values print as Zebra's substitute
// characters.
func checkText(v int) string {
	if v < 43 {
		return string(alphabet[v])
	}
	return string("&'()"[v-43])
}

// checkChar computes the C (maxWeight 20) or K (maxWeight 15) check value.
func checkChar(v []int, maxWeight int) int {
	sum := 0
	for i := len(v) - 1; i >= 0; i-- {
		w := (len(v)-1-i)%maxWeight + 1
		sum += w * v[i]
	}
	return sum % 47
}
