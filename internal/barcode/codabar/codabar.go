// Package codabar encodes ANSI Codabar (ZPL ^BK).
package codabar

import (
	"fmt"
	"strings"

	"github.com/datamagik/zpl-to-image/internal/barcode"
)

// Options carries the parsed ^BK parameters relevant to encoding. ^BK's
// check digit parameter is fixed at N, so no check digit is supported.
type Options struct {
	// Start and Stop are the k and l parameters (A, B, C or D; Zebra's
	// default is A). Zero means A. Lowercase is accepted.
	Start, Stop byte
}

const alphabet = "0123456789-$:/.+ABCD"

// patterns gives the seven elements (bar, space, ...; 1 = wide) of each
// character in alphabet order.
var patterns = [20]string{
	"0000011", "0000110", "0001001", "1100000", "0010010",
	"1000010", "0100001", "0100100", "0110000", "1001000",
	"0001100", "0011000", "1000101", "1010001", "1010100",
	"0010101", "0011010", "0101001", "0001011", "0001110",
}

// Encode encodes data (0-9 - $ : / . +) between the start and stop
// characters. Text is start + data + stop.
func Encode(data string, opts Options) (barcode.Linear, error) {
	start, err := startStop(opts.Start)
	if err != nil {
		return barcode.Linear{}, err
	}
	stop, err := startStop(opts.Stop)
	if err != nil {
		return barcode.Linear{}, err
	}
	for i := 0; i < len(data); i++ {
		if k := strings.IndexByte(alphabet, data[i]); k < 0 || k >= 16 {
			return barcode.Linear{}, fmt.Errorf("%w: codabar: character %q not encodable", barcode.ErrInvalidData, data[i])
		}
	}
	text := string(start) + data + string(stop)
	var el []int
	for i := 0; i < len(text); i++ {
		if i > 0 {
			el = append(el, 1) // narrow inter-character gap
		}
		for _, b := range patterns[strings.IndexByte(alphabet, text[i])] {
			el = append(el, int(b-'0')+1)
		}
	}
	return barcode.Linear{Elements: el, Ratio: true, Text: text}, nil
}

func startStop(c byte) (byte, error) {
	if c == 0 {
		return 'A', nil
	}
	if c >= 'a' && c <= 'd' {
		c -= 'a' - 'A'
	}
	if c < 'A' || c > 'D' {
		return 0, fmt.Errorf("%w: codabar: invalid start/stop %q", barcode.ErrInvalidData, c)
	}
	return c, nil
}
