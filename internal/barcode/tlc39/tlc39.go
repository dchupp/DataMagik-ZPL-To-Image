// Package tlc39 encodes TCIF Linked Code 39 (TLC39) for the ZPL ^BT
// command: a Code 39 symbol carrying the 6-digit ECI number, a Code 39 "T"
// linkage flag to its right, and, when serial/additional data is present, a
// four-column MicroPDF417 symbol printed directly above the Code 39 symbol
// (US 2001/0045461 A1; ^BT field data "ECI,serial[,additional,...]").
//
// Placement (all distances in Code 39 narrow modules, ^BT w1):
//   - Linear: the Code 39 symbol "*" ECI "*" (no check character; the ECI
//     number carries its own check digit).
//   - Flag: drawn FlagGap narrow modules (one quiet zone) right of the last
//     bar of the Code 39 stop character, same height as the linear symbol.
//   - MicroPDF: its left edge (first bar of the left row address pattern)
//     is MicroPDFOffset narrow modules right of the first bar of the Code 39
//     start character (aligned with the first interior space), and its
//     bottom row sits directly on top of the Code 39 bars with no gap. Its
//     modules are ^BT w2 wide and its rows ^BT h2 high (one Matrix row per
//     symbol row).
package tlc39

import (
	"fmt"
	"strings"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/code39"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/micropdf"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/pdf417"
)

// Defaults and limits from the ^BT documentation.
const (
	FlagGap        = 10  // narrow modules between the stop character and the flag
	MicroPDFOffset = 1   // narrow modules from the Code 39 start to the MicroPDF417
	MaxFieldLen    = 25  // serial number and each additional field
	MaxDataLen     = 150 // serial + additional data incl. commas
)

// Symbol is an encoded TLC39 symbol.
type Symbol struct {
	Linear barcode.Linear // Code 39 ECI symbol (Ratio elements)
	Flag   barcode.Linear // "T" linkage flag character (Ratio elements)
	// MicroPDF is the 2D component, nil when the field data holds only the
	// ECI number (only the Code 39 symbol and flag print).
	MicroPDF *barcode.Matrix
}

// Options controls TLC39 encoding.
type Options struct {
	// FieldSeparator replaces the commas between the serial number and
	// additional fields inside the MicroPDF417 data. 0 keeps the commas as
	// Zebra stores the data ("appended after the serial number").
	FieldSeparator byte
}

// code39Elements returns the Code 39 elements (1 narrow, 2 wide) of s,
// which must contain only Code 39 characters.
func code39Elements(s string) []int {
	var e []int
	for i := 0; i < len(s); i++ {
		if i > 0 {
			e = append(e, 1) // inter-character gap
		}
		e = append(e, code39.Elements(code39.Index(s[i]))...)
	}
	return e
}

func isAlnum(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

// Encode encodes ^BT field data "ECI[,serial[,additional...]]".
func Encode(data string, opts Options) (Symbol, error) {
	bad := func(format string, a ...any) (Symbol, error) {
		return Symbol{}, fmt.Errorf("%w: tlc39: "+format, append([]any{barcode.ErrInvalidData}, a...)...)
	}
	if len(data) < 6 {
		return bad("ECI number must be 6 digits")
	}
	eci := data[:6]
	for i := 0; i < 6; i++ {
		if eci[i] < '0' || eci[i] > '9' {
			return bad("ECI number %q must be 6 digits", eci)
		}
	}
	sym := Symbol{
		Linear: barcode.Linear{Elements: code39Elements("*" + eci + "*"), Ratio: true, Text: eci},
		Flag:   barcode.Linear{Elements: code39Elements("T"), Ratio: true},
	}
	if len(data) == 6 {
		return sym, nil // only the Code 39 symbol prints
	}
	if data[6] != ',' {
		return bad("ECI number must be exactly 6 digits followed by a comma, got %q", data)
	}
	rest := data[7:]
	if len(rest) > MaxDataLen {
		return bad("data after the ECI number exceeds %d characters", MaxDataLen)
	}
	fields := strings.Split(rest, ",")
	for i, f := range fields {
		if len(f) > MaxFieldLen {
			return bad("field %d longer than %d characters", i+1, MaxFieldLen)
		}
		for j := 0; j < len(f); j++ {
			if !isAlnum(f[j]) {
				return bad("field %d: character %q is not alphanumeric", i+1, f[j])
			}
		}
	}
	if fields[0] == "" {
		return bad("missing serial number")
	}
	payload := rest
	if opts.FieldSeparator != 0 {
		payload = strings.ReplaceAll(rest, ",", string(opts.FieldSeparator))
	}
	cws := append([]int{pdf417.CWLinkageOther}, pdf417.CompactMicro([]byte(payload))...)
	v, ok := micropdf.Smallest(4, len(cws))
	if !ok {
		return bad("data too long for MicroPDF417")
	}
	m, err := micropdf.EncodeCodewords(cws, v)
	if err != nil {
		return Symbol{}, err
	}
	sym.MicroPDF = &m
	return sym, nil
}
