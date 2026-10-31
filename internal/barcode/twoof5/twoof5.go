// Package twoof5 encodes the bar-only 2 of 5 symbologies: Industrial 2 of 5
// (ZPL ^BI) and Standard 2 of 5 (ZPL ^BJ). Neither has a check digit in ZPL.
package twoof5

import (
	"fmt"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
)

// patterns gives the five bar widths (1 = wide) of each digit.
var patterns = [10]string{
	"00110", "10001", "01001", "11000", "00101",
	"10100", "01100", "00011", "10010", "01010",
}

// EncodeIndustrial encodes digits as Industrial 2 of 5 (^BI): start bars
// wide-wide-narrow, stop bars wide-narrow-wide, all spaces narrow (as in the
// ZPL II guide figure).
func EncodeIndustrial(data string) (barcode.Linear, error) {
	return encode(data, []int{2, 1, 2, 1, 1, 1}, []int{2, 1, 1, 1, 2})
}

// EncodeStandard encodes digits as Standard 2 of 5 (^BJ): start bars
// narrow-narrow, stop bars wide-narrow, all spaces narrow (as in the ZPL II
// guide figure).
func EncodeStandard(data string) (barcode.Linear, error) {
	return encode(data, []int{1, 1, 1, 1}, []int{2, 1, 1})
}

func encode(data string, start, stop []int) (barcode.Linear, error) {
	if data == "" {
		return barcode.Linear{}, fmt.Errorf("%w: 2 of 5: empty data", barcode.ErrInvalidData)
	}
	el := append([]int(nil), start...)
	for i := 0; i < len(data); i++ {
		c := data[i]
		if c < '0' || c > '9' {
			return barcode.Linear{}, fmt.Errorf("%w: 2 of 5: character %q not encodable", barcode.ErrInvalidData, c)
		}
		for _, b := range patterns[c-'0'] {
			el = append(el, int(b-'0')+1, 1)
		}
	}
	el = append(el, stop...)
	return barcode.Linear{Elements: el, Ratio: true, Text: data}, nil
}
