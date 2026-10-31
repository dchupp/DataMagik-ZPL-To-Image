// Package plessey encodes Plessey (ZPL ^BP), including its mandatory 8-bit
// CRC check.
package plessey

import (
	"fmt"
	"strings"

	"github.com/datamagik/zpl-to-image/internal/barcode"
)

// Options carries the parsed ^BP parameters relevant to encoding.
type Options struct {
	// PrintCheckDigit is the ^BP e parameter: print the CRC (as two hex
	// characters) in the interpretation line. The CRC is always encoded.
	PrintCheckDigit bool
}

const hexDigits = "0123456789ABCDEF"

// Encode encodes data (0-9, A-F) as Plessey. Each character is four bits,
// least significant first; a 1 bit is a wide bar and narrow space, a 0 bit a
// narrow bar and wide space. The CRC uses the polynomial
// x^8+x^7+x^6+x^5+x^3+1. Start and termination patterns match the ZPL II
// guide figure.
func Encode(data string, opts Options) (barcode.Linear, error) {
	data = strings.ToUpper(data)
	if data == "" {
		return barcode.Linear{}, fmt.Errorf("%w: plessey: empty data", barcode.ErrInvalidData)
	}
	var bits []byte
	for i := 0; i < len(data); i++ {
		v := strings.IndexByte(hexDigits, data[i])
		if v < 0 {
			return barcode.Linear{}, fmt.Errorf("%w: plessey: character %q not encodable", barcode.ErrInvalidData, data[i])
		}
		for b := 0; b < 4; b++ {
			bits = append(bits, byte(v>>b&1))
		}
	}
	crc := CRC(bits)
	el := []int{2, 1, 2, 1, 1, 2, 2, 1} // start: bits 1101
	for _, b := range append(bits, crc[:]...) {
		if b == 1 {
			el = append(el, 2, 1)
		} else {
			el = append(el, 1, 2)
		}
	}
	el = append(el, 2, 2, 1, 2, 1, 1, 2, 1, 2) // termination bar + stop
	text := data
	if opts.PrintCheckDigit {
		lo := crc[0] | crc[1]<<1 | crc[2]<<2 | crc[3]<<3
		hi := crc[4] | crc[5]<<1 | crc[6]<<2 | crc[7]<<3
		text += string(hexDigits[lo]) + string(hexDigits[hi])
	}
	return barcode.Linear{Elements: el, Ratio: true, Text: text}, nil
}

// CRC computes the Plessey check bits (in transmission order) over the data
// bits.
func CRC(bits []byte) [8]byte {
	poly := [9]byte{1, 1, 1, 1, 0, 1, 0, 0, 1}
	work := append(append([]byte(nil), bits...), make([]byte, 8)...)
	for i := range bits {
		if work[i] == 1 {
			for j, p := range poly {
				work[i+j] ^= p
			}
		}
	}
	var crc [8]byte
	copy(crc[:], work[len(bits):])
	return crc
}
