// Package code11 encodes Code 11 / USD-8 (ZPL ^B1).
package code11

import (
	"fmt"
	"strings"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
)

// Options carries the parsed ^B1 parameters relevant to encoding.
type Options struct {
	// OneCheckDigit is the ^B1 e parameter: Y = one check digit (C only),
	// N (the Zebra default) = two check digits (C and K).
	OneCheckDigit bool
}

const alphabet = "0123456789-"

// patterns gives the five elements (bar, space, bar, space, bar; 1 = wide)
// of each character in alphabet order.
var patterns = [11]string{
	"00001", "10001", "01001", "11000", "00101",
	"10100", "01100", "00011", "10010", "10000", "00100",
}

const startStop = "00110"

// Encode encodes data (digits and '-') as Code 11. Text is the data
// followed by the check digit(s).
func Encode(data string, opts Options) (barcode.Linear, error) {
	var v []int
	for i := 0; i < len(data); i++ {
		k := strings.IndexByte(alphabet, data[i])
		if k < 0 {
			return barcode.Linear{}, fmt.Errorf("%w: code11: character %q not encodable", barcode.ErrInvalidData, data[i])
		}
		v = append(v, k)
	}
	if len(v) == 0 {
		return barcode.Linear{}, fmt.Errorf("%w: code11: empty data", barcode.ErrInvalidData)
	}
	v = append(v, check(v, 10))
	if !opts.OneCheckDigit {
		v = append(v, check(v, 9))
	}
	text := make([]byte, len(v))
	for i, x := range v {
		text[i] = alphabet[x]
	}
	var el []int
	add := func(p string) {
		if len(el) > 0 {
			el = append(el, 1) // narrow inter-character gap
		}
		for _, b := range p {
			el = append(el, int(b-'0')+1)
		}
	}
	add(startStop)
	for _, x := range v {
		add(patterns[x])
		if alphabet[x] == '-' {
			// A ZD420 draws the '-' centre bar floor(5/3 × ratio) modules
			// wide, not as an ordinary wide bar.
			el[len(el)-3] = barcode.ElemDash
		}
	}
	add(startStop)
	return barcode.Linear{Elements: el, Ratio: true, Text: string(text)}, nil
}

// check computes a Code 11 check value with weights 1..maxWeight cycling
// from the right.
func check(v []int, maxWeight int) int {
	sum := 0
	for i := len(v) - 1; i >= 0; i-- {
		sum += ((len(v)-1-i)%maxWeight + 1) * v[i]
	}
	return sum % 11
}
