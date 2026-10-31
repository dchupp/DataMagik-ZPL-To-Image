// Package i2of5 encodes Interleaved 2 of 5 (ZPL ^B2).
package i2of5

import (
	"fmt"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/gs1"
)

// Options carries the parsed ^B2 parameters relevant to encoding.
type Options struct {
	// CheckDigit is the ^B2 e parameter: calculate and print a Mod 10 check
	// digit.
	CheckDigit bool
}

// patterns gives the five widths (1 = wide) of each digit.
var patterns = [10]string{
	"00110", "10001", "01001", "11000", "00101",
	"10100", "01100", "00011", "10010", "01010",
}

// Encode encodes digits as Interleaved 2 of 5. When the digit count
// (including the optional check digit) is odd, a leading 0 is added as the
// Zebra printer does. Text is the encoded digits.
func Encode(data string, opts Options) (barcode.Linear, error) {
	d := []byte(data)
	if len(d) == 0 {
		return barcode.Linear{}, fmt.Errorf("%w: i2of5: empty data", barcode.ErrInvalidData)
	}
	for _, c := range d {
		if c < '0' || c > '9' {
			return barcode.Linear{}, fmt.Errorf("%w: i2of5: character %q not encodable", barcode.ErrInvalidData, c)
		}
	}
	if opts.CheckDigit {
		d = append(d, Mod10(string(d)))
	}
	if len(d)%2 == 1 {
		d = append([]byte{'0'}, d...)
	}
	el := []int{1, 1, 1, 1} // start
	for i := 0; i < len(d); i += 2 {
		b, s := patterns[d[i]-'0'], patterns[d[i+1]-'0']
		for k := 0; k < 5; k++ {
			el = append(el, int(b[k]-'0')+1, int(s[k]-'0')+1)
		}
	}
	el = append(el, 2, 1, 1) // stop
	return barcode.Linear{Elements: el, Ratio: true, Text: string(d)}, nil
}

// Mod10 returns the Mod 10 check digit of a digit string (weight 3 on the
// rightmost digit, alternating 3 and 1).
func Mod10(digits string) byte { return gs1.CheckDigit(digits) }
