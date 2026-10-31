// Package msi encodes MSI (Modified Plessey, ZPL ^BM).
package msi

import (
	"fmt"
	"strconv"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
)

// Check is the ^BM e parameter.
type Check byte

const (
	CheckNone       Check = 'A' // no check digits
	CheckMod10      Check = 'B' // one Mod 10 check digit (Zebra default)
	CheckMod10Mod10 Check = 'C' // two Mod 10 check digits
	CheckMod11Mod10 Check = 'D' // one Mod 11 and one Mod 10 check digit
)

// Options carries the parsed ^BM parameters relevant to encoding.
type Options struct {
	// Check selects the check digits; zero means CheckMod10 (Zebra default).
	Check Check
	// PrintCheckDigits is the ^BM e2 parameter: insert the check digit(s)
	// into the interpretation line.
	PrintCheckDigits bool
}

// Encode encodes digits as MSI. The ZPL II guide limits valid symbols to 13
// (no check) or 14 digits, but like the printer this encoder does not
// enforce the limit.
func Encode(data string, opts Options) (barcode.Linear, error) {
	if data == "" {
		return barcode.Linear{}, fmt.Errorf("%w: msi: empty data", barcode.ErrInvalidData)
	}
	for i := 0; i < len(data); i++ {
		if data[i] < '0' || data[i] > '9' {
			return barcode.Linear{}, fmt.Errorf("%w: msi: character %q not encodable", barcode.ErrInvalidData, data[i])
		}
	}
	full := data
	switch opts.Check {
	case 0, CheckMod10:
		full += Mod10(full)
	case CheckNone:
	case CheckMod10Mod10:
		full += Mod10(full)
		full += Mod10(full)
	case CheckMod11Mod10:
		full += Mod11(full)
		full += Mod10(full)
	default:
		return barcode.Linear{}, fmt.Errorf("%w: msi: unknown check option %q", barcode.ErrInvalidData, opts.Check)
	}
	text := data
	if opts.PrintCheckDigits {
		text = full
	}
	el := []int{2, 1} // start
	for i := 0; i < len(full); i++ {
		d := full[i] - '0'
		for b := 3; b >= 0; b-- {
			if d>>b&1 == 1 {
				el = append(el, 2, 1)
			} else {
				el = append(el, 1, 2)
			}
		}
	}
	el = append(el, 1, 2, 1) // stop
	return barcode.Linear{Elements: el, Ratio: true, Text: text}, nil
}

// Mod10 returns the MSI Mod 10 (Luhn) check digit: digits are doubled
// alternately starting with the rightmost one.
func Mod10(digits string) string {
	sum := 0
	for i := len(digits) - 1; i >= 0; i-- {
		d := int(digits[i] - '0')
		if (len(digits)-1-i)%2 == 0 {
			d *= 2
			d = d/10 + d%10
		}
		sum += d
	}
	return strconv.Itoa((10 - sum%10) % 10)
}

// Mod11 returns the MSI Mod 11 check (IBM weights 2..7 from the right). A
// remainder giving 10 is emitted as the two digits "10".
func Mod11(digits string) string {
	sum := 0
	for i := len(digits) - 1; i >= 0; i-- {
		sum += int(digits[i]-'0') * ((len(digits)-1-i)%6 + 2)
	}
	return strconv.Itoa((11 - sum%11) % 11)
}
