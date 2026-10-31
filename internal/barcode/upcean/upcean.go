// Package upcean encodes the UPC/EAN family: EAN-8 (ZPL ^B8), EAN-13 (^BE),
// UPC-A (^BU), UPC-E (^B9) and the 2/5-digit add-ons (^BS).
//
// All results use module widths (Ratio false). Guards marks the bars that
// extend below the main bars, and TextGroups positions the interpretation
// line digits the way UPC/EAN symbols print them.
package upcean

import (
	"fmt"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/gs1"
)

// lCodes are the odd-parity (set A / L) widths, starting with a space. The
// right-hand (R) characters use the same widths starting with a bar, and
// the even-parity (set B / G) characters use them reversed.
var lCodes = [10][4]int{
	{3, 2, 1, 1}, {2, 2, 2, 1}, {2, 1, 2, 2}, {1, 4, 1, 1}, {1, 1, 3, 2},
	{1, 2, 3, 1}, {1, 1, 1, 4}, {1, 3, 1, 2}, {1, 2, 1, 3}, {3, 1, 1, 2},
}

// ean13Parity gives the L/G pattern of the left half for each leading digit
// (bit i set = character i uses G).
var ean13Parity = [10]string{
	"LLLLLL", "LLGLGG", "LLGGLG", "LLGGGL", "LGLLGG",
	"LGGLLG", "LGGGLL", "LGLGLG", "LGLGGL", "LGGLGL",
}

// upceParity gives the UPC-E parity for number system 0 by check digit; for
// number system 1 every character is inverted.
var upceParity = [10]string{
	"GGGLLL", "GGLGLL", "GGLLGL", "GGLLLG", "GLGGLL",
	"GLLGGL", "GLLLGG", "GLGLGL", "GLGLLG", "GLLGLG",
}

// ean5Parity gives the 5-digit add-on parity by checksum.
var ean5Parity = [10]string{
	"GGLLL", "GLGLL", "GLLGL", "GLLLG", "LGGLL",
	"LLGGL", "LLLGG", "LGLGL", "LGLLG", "LLGLG",
}

// builder accumulates elements and guard flags.
type builder struct {
	el    []int
	guard []bool
	mod   int // modules so far
}

func (b *builder) add(guard bool, w ...int) {
	for _, x := range w {
		b.el = append(b.el, x)
		b.guard = append(b.guard, guard && len(b.el)%2 == 1)
		b.mod += x
	}
}

// char adds digit d with parity p ('L', 'G' or 'R').
func (b *builder) char(d byte, p byte, guard bool) {
	w := lCodes[d-'0']
	if p == 'G' {
		w = [4]int{w[3], w[2], w[1], w[0]}
	}
	b.add(guard, w[:]...)
}

func (b *builder) linear(text string, groups []barcode.TextGroup) barcode.Linear {
	return barcode.Linear{Elements: b.el, Guards: b.guard, Text: text, TextGroups: groups}
}

// normalize checks data is all digits and pads it on the left with zeros
// (or truncates it, keeping the leftmost n digits) to exactly n digits, as
// the ZPL II guide describes for fixed-length UPC/EAN fields. Keeping the
// leftmost digits means a supplied check digit is dropped and recomputed.
func normalize(sym, data string, n int) (string, error) {
	for i := 0; i < len(data); i++ {
		if data[i] < '0' || data[i] > '9' {
			return "", fmt.Errorf("%w: %s: character %q not encodable", barcode.ErrInvalidData, sym, data[i])
		}
	}
	if len(data) > n {
		return data[:n], nil
	}
	for len(data) < n {
		data = "0" + data
	}
	return data, nil
}

// CheckDigit returns the UPC/EAN Mod 10 check digit of digits (weight 3 on
// the rightmost digit).
func CheckDigit(digits string) byte { return gs1.CheckDigit(digits) }

// EncodeEAN13 encodes ^BE field data: 12 digits (padded on the left with
// zeros or truncated) plus a computed check digit.
func EncodeEAN13(data string) (barcode.Linear, error) {
	d, err := normalize("ean13", data, 12)
	if err != nil {
		return barcode.Linear{}, err
	}
	d += string(CheckDigit(d))
	var b builder
	b.add(true, 1, 1, 1)
	par := ean13Parity[d[0]-'0']
	for i := 1; i <= 6; i++ {
		b.char(d[i], par[i-1], false)
	}
	b.add(true, 1, 1, 1, 1, 1)
	for i := 7; i <= 12; i++ {
		b.char(d[i], 'R', false)
	}
	b.add(true, 1, 1, 1)
	return b.linear(d, []barcode.TextGroup{
		{Text: d[:1], Start: -7, End: -1},
		{Text: d[1:7], Start: 3, End: 45},
		{Text: d[7:], Start: 50, End: 92},
	}), nil
}

// EncodeEAN8 encodes ^B8 field data: 7 digits (padded on the left with
// zeros or truncated) plus a computed check digit.
func EncodeEAN8(data string) (barcode.Linear, error) {
	d, err := normalize("ean8", data, 7)
	if err != nil {
		return barcode.Linear{}, err
	}
	d += string(CheckDigit(d))
	var b builder
	b.add(true, 1, 1, 1)
	for i := 0; i < 4; i++ {
		b.char(d[i], 'L', false)
	}
	b.add(true, 1, 1, 1, 1, 1)
	for i := 4; i < 8; i++ {
		b.char(d[i], 'R', false)
	}
	b.add(true, 1, 1, 1)
	return b.linear(d, []barcode.TextGroup{
		{Text: d[:4], Start: 3, End: 31},
		{Text: d[4:], Start: 36, End: 64},
	}), nil
}

// UPCOptions carries the ^BU / ^B9 parameters relevant to encoding.
type UPCOptions struct {
	// PrintCheckDigit is the e parameter (Zebra default Y): show the check
	// digit in the interpretation line. It is always encoded.
	PrintCheckDigit bool
}

// EncodeUPCA encodes ^BU field data: 11 digits (padded on the left with
// zeros or truncated) plus a computed check digit. The bars of the first and
// last digits extend with the guards; those digits print small outside the
// symbol.
func EncodeUPCA(data string, opts UPCOptions) (barcode.Linear, error) {
	d, err := normalize("upca", data, 11)
	if err != nil {
		return barcode.Linear{}, err
	}
	d += string(CheckDigit(d))
	var b builder
	b.add(true, 1, 1, 1)
	for i := 0; i < 6; i++ {
		b.char(d[i], 'L', i == 0)
	}
	b.add(true, 1, 1, 1, 1, 1)
	for i := 6; i < 12; i++ {
		b.char(d[i], 'R', i == 11)
	}
	b.add(true, 1, 1, 1)
	text := d
	groups := []barcode.TextGroup{
		{Text: d[:1], Start: -7, End: -1, Small: true},
		{Text: d[1:6], Start: 10, End: 45},
		{Text: d[6:11], Start: 50, End: 85},
	}
	if opts.PrintCheckDigit {
		groups = append(groups, barcode.TextGroup{Text: d[11:], Start: 96, End: 102, Small: true})
	} else {
		text = d[:11]
	}
	return b.linear(text, groups), nil
}

// EncodeUPCE encodes ^B9 field data. Zebra expects the 10-digit
// manufacturer + product code of a number system 0 UPC-A and zero-suppresses
// it; this also accepts 11 digits (number system 0 or 1 first) and 12 digits
// (with a check digit, which is recomputed). Shorter data is padded on the
// left with zeros. Codes that cannot be zero-suppressed (see the ^B9 product
// code rules) are rejected.
//
// Text is the number system, the six UPC-E digits and the check digit; the
// number system and check digit print small outside the symbol.
func EncodeUPCE(data string, opts UPCOptions) (barcode.Linear, error) {
	if len(data) < 10 {
		var err error
		if data, err = normalize("upce", data, 10); err != nil {
			return barcode.Linear{}, err
		}
	}
	if _, err := normalize("upce", data, len(data)); err != nil {
		return barcode.Linear{}, err
	}
	var a string // the 11-digit UPC-A message
	switch len(data) {
	case 10:
		a = "0" + data
	case 11:
		a = data
	default:
		a = data[:11]
	}
	ns := a[0]
	if ns != '0' && ns != '1' {
		return barcode.Linear{}, fmt.Errorf("%w: upce: number system %c not supported", barcode.ErrInvalidData, ns)
	}
	e, err := suppress(a[1:6], a[6:11])
	if err != nil {
		return barcode.Linear{}, err
	}
	check := CheckDigit(a)
	par := upceParity[check-'0']
	var b builder
	b.add(true, 1, 1, 1)
	for i := 0; i < 6; i++ {
		p := par[i]
		if ns == '1' {
			p = 'L' + 'G' - p
		}
		b.char(e[i], p, false)
	}
	b.add(true, 1, 1, 1, 1, 1, 1)
	text := string(ns) + e
	groups := []barcode.TextGroup{
		{Text: string(ns), Start: -7, End: -1, Small: true},
		{Text: e, Start: 3, End: 45},
	}
	if opts.PrintCheckDigit {
		text += string(check)
		groups = append(groups, barcode.TextGroup{Text: string(check), Start: 52, End: 58, Small: true})
	}
	return b.linear(text, groups), nil
}

// suppress converts a 5-digit manufacturer and 5-digit product code to the
// six UPC-E digits.
func suppress(m, p string) (string, error) {
	switch {
	case (m[2:] == "000" || m[2:] == "100" || m[2:] == "200") && p[:2] == "00":
		return m[:2] + p[2:] + m[2:3], nil
	case m[3:] == "00" && p[:3] == "000":
		return m[:3] + p[3:] + "3", nil
	case m[4] == '0' && p[:4] == "0000":
		return m[:4] + p[4:] + "4", nil
	case m[4] != '0' && p[:4] == "0000" && p[4] >= '5':
		return m + p[4:], nil
	}
	return "", fmt.Errorf("%w: upce: manufacturer %s / product %s cannot be zero-suppressed", barcode.ErrInvalidData, m, p)
}

// EncodeEAN2 encodes a 2-digit UPC/EAN add-on (^BS with 2 digits). The
// interpretation line spans the whole add-on.
func EncodeEAN2(data string) (barcode.Linear, error) {
	d, err := normalize("ean2", data, 2)
	if err != nil {
		return barcode.Linear{}, err
	}
	v := int(d[0]-'0')*10 + int(d[1]-'0')
	par := [4]string{"LL", "LG", "GL", "GG"}[v%4]
	return addOn(d, par), nil
}

// EncodeEAN5 encodes a 5-digit UPC/EAN add-on (^BS with 5 digits).
func EncodeEAN5(data string) (barcode.Linear, error) {
	d, err := normalize("ean5", data, 5)
	if err != nil {
		return barcode.Linear{}, err
	}
	sum := 0
	for i := 0; i < 5; i++ {
		w := 3
		if i%2 == 1 {
			w = 9
		}
		sum += w * int(d[i]-'0')
	}
	return addOn(d, ean5Parity[sum%10]), nil
}

// EncodeAddOn encodes ^BS field data: 1-2 digits give a 2-digit add-on,
// 3-5 digits a 5-digit add-on (padded on the left with zeros); longer data
// is truncated to 5 digits.
func EncodeAddOn(data string) (barcode.Linear, error) {
	if len(data) <= 2 {
		return EncodeEAN2(data)
	}
	return EncodeEAN5(data)
}

func addOn(d, par string) barcode.Linear {
	var b builder
	b.add(false, 1, 1, 2)
	for i := 0; i < len(d); i++ {
		if i > 0 {
			b.add(false, 1, 1)
		}
		b.char(d[i], par[i], false)
	}
	l := b.linear(d, []barcode.TextGroup{{Text: d, Start: 0, End: b.mod}})
	l.Guards = nil
	return l
}
