// Package code39 encodes Code 39 (ZPL ^B3) and LOGMARS (ZPL ^BL).
package code39

import (
	"fmt"
	"strings"

	"github.com/datamagik/zpl-to-image/internal/barcode"
)

// Options carries the parsed ^B3 parameters relevant to encoding.
type Options struct {
	// CheckDigit is the ^B3 e parameter: append a Mod 43 check character.
	CheckDigit bool
}

// alphabet lists the native characters in Mod 43 value order.
const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ-. $/+%"

// patterns gives the nine elements (bar, space, ...; 1 = wide) of each
// character in alphabet order; the last entry is the '*' start/stop.
var patterns = [44]string{
	"000110100", "100100001", "001100001", "101100000", "000110001",
	"100110000", "001110000", "000100101", "100100100", "001100100",
	"100001001", "001001001", "101001000", "000011001", "100011000",
	"001011000", "000001101", "100001100", "001001100", "000011100",
	"100000011", "001000011", "101000010", "000010011", "100010010",
	"001010010", "000000111", "100000110", "001000110", "000010110",
	"110000001", "011000001", "111000000", "010010001", "110010000",
	"011010000", "010000101", "110000100", "011000100", "010101000",
	"010100010", "010001010", "000101010", "010010100",
}

const star = 43

// fullASCII maps ASCII 0..127 to its Code 39 Full ASCII representation.
var fullASCII [128]string

func init() {
	for c := 0; c < 128; c++ {
		var s string
		switch {
		case c == 0:
			s = "%U"
		case c >= 1 && c <= 26:
			s = "$" + string(rune('A'+c-1))
		case c >= 27 && c <= 31:
			s = "%" + string(rune('A'+c-27))
		case c == ' ', c == '-', c == '.', c >= '0' && c <= '9', c >= 'A' && c <= 'Z':
			s = string(rune(c))
		case c >= '!' && c <= ',', c == '/':
			s = "/" + string(rune('A'+c-'!'))
		case c == ':':
			s = "/Z"
		case c >= ';' && c <= '?':
			s = "%" + string(rune('F'+c-';'))
		case c == '@':
			s = "%V"
		case c >= '[' && c <= '_':
			s = "%" + string(rune('K'+c-'['))
		case c == '`':
			s = "%W"
		case c >= 'a' && c <= 'z':
			s = "+" + string(rune('A'+c-'a'))
		case c >= '{' && c <= 127:
			s = "%" + string(rune('P'+c-'{'))
		}
		fullASCII[c] = s
	}
}

// values converts data to Mod 43 values. Native characters (including
// $ / + %) encode as themselves; any other ASCII character is encoded with
// its Full ASCII pair, as Zebra does for e.g. lowercase letters.
func values(data string) ([]int, error) {
	var v []int
	for i := 0; i < len(data); i++ {
		c := data[i]
		if k := strings.IndexByte(alphabet, c); k >= 0 {
			v = append(v, k)
			continue
		}
		if c >= 128 {
			return nil, fmt.Errorf("%w: code39: character %q not encodable", barcode.ErrInvalidData, c)
		}
		for _, p := range []byte(fullASCII[c]) {
			v = append(v, strings.IndexByte(alphabet, p))
		}
	}
	return v, nil
}

// Encode encodes data as Code 39. Text is the interpretation line as Zebra
// prints it: the data (and check character, if any) between '*' start/stop
// characters.
func Encode(data string, opts Options) (barcode.Linear, error) {
	v, err := values(data)
	if err != nil {
		return barcode.Linear{}, err
	}
	text := data
	if opts.CheckDigit {
		c := check(v)
		v = append(v, c)
		text += string(alphabet[c])
	}
	return build(v, "*"+text+"*"), nil
}

// EncodeLOGMARS encodes data as LOGMARS (^BL): Code 39 with a mandatory
// Mod 43 check character. Lowercase letters are converted to uppercase as
// the ZPL II guide specifies. Text is the data followed by the check
// character, without '*' delimiters.
func EncodeLOGMARS(data string) (barcode.Linear, error) {
	data = strings.ToUpper(data)
	var v []int
	for i := 0; i < len(data); i++ {
		k := strings.IndexByte(alphabet, data[i])
		if k < 0 {
			return barcode.Linear{}, fmt.Errorf("%w: logmars: character %q not encodable", barcode.ErrInvalidData, data[i])
		}
		v = append(v, k)
	}
	c := check(v)
	v = append(v, c)
	return build(v, data+string(alphabet[c])), nil
}

func check(v []int) int {
	sum := 0
	for _, x := range v {
		sum += x
	}
	return sum % 43
}

func build(v []int, text string) barcode.Linear {
	all := append(append([]int{star}, v...), star)
	var el []int
	for i, x := range all {
		if i > 0 {
			el = append(el, 1) // narrow inter-character gap
		}
		for _, b := range patterns[x] {
			el = append(el, int(b-'0')+1)
		}
	}
	return barcode.Linear{Elements: el, Ratio: true, Text: text}
}

// Chars lists the native Code 39 characters in check-value (mod 43) order.
const Chars = alphabet

// StartStop is the character index of the '*' start/stop character.
const StartStop = star

// Index returns the character index of c (StartStop for '*'), or -1.
func Index(c byte) int {
	if c == '*' {
		return star
	}
	return strings.IndexByte(alphabet, c)
}

// Elements returns the nine bar/space elements of character index i:
// 1 = narrow, 2 = wide, starting with a bar.
func Elements(i int) []int {
	e := make([]int, 9)
	for k, c := range patterns[i] {
		e[k] = 1 + int(c-'0')
	}
	return e
}
