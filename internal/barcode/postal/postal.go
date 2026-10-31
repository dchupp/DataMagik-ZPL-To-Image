// Package postal encodes the USPS height-modulated symbologies: POSTNET
// (ZPL ^BZ type 0), PLANET (^BZ type 1 and ^B5) and the Intelligent Mail
// barcode (^BZ type 3, USPS-B-3200).
package postal

import (
	"fmt"
	"math/big"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
)

// postnetCodes gives the five bars of each digit for POSTNET (1 = full
// bar); PLANET uses the complement.
var postnetCodes = [10]string{
	"11000", "00011", "00101", "00110", "01001",
	"01010", "01100", "10001", "10010", "10100",
}

// EncodePOSTNET encodes digits as POSTNET: frame bar, five bars per digit,
// a Mod 10 check digit and a closing frame bar. Text is the data as given.
func EncodePOSTNET(data string) (barcode.Postal, error) {
	return encode5("postnet", data, '1')
}

// EncodePLANET encodes digits as PLANET (the POSTNET structure with
// inverted bar heights). Text is the data as given.
func EncodePLANET(data string) (barcode.Postal, error) {
	return encode5("planet", data, '0')
}

func encode5(sym, data string, tall byte) (barcode.Postal, error) {
	if data == "" {
		return barcode.Postal{}, fmt.Errorf("%w: %s: empty data", barcode.ErrInvalidData, sym)
	}
	sum := 0
	for i := 0; i < len(data); i++ {
		c := data[i]
		if c < '0' || c > '9' {
			return barcode.Postal{}, fmt.Errorf("%w: %s: character %q not encodable", barcode.ErrInvalidData, sym, c)
		}
		sum += int(c - '0')
	}
	digits := data + string(rune('0'+(10-sum%10)%10))
	bars := []barcode.BarState{barcode.BarFull}
	for i := 0; i < len(digits); i++ {
		for _, b := range []byte(postnetCodes[digits[i]-'0']) {
			if b == tall {
				bars = append(bars, barcode.BarFull)
			} else {
				bars = append(bars, barcode.BarTracker)
			}
		}
	}
	bars = append(bars, barcode.BarFull)
	return barcode.Postal{Bars: bars, Text: data}, nil
}

// EncodeIMb encodes a USPS Intelligent Mail barcode. data is the 20-digit
// tracking code (barcode ID, service type, mailer ID and serial number)
// followed by an optional 5, 9 or 11 digit routing (ZIP) code, as Zebra's
// ^BZ type 3 takes it; '-' and ' ' separators are ignored. Text is the data
// as given.
func EncodeIMb(data string) (barcode.Postal, error) {
	var d []byte
	for i := 0; i < len(data); i++ {
		switch c := data[i]; {
		case c >= '0' && c <= '9':
			d = append(d, c)
		case c == '-' || c == ' ':
		default:
			return barcode.Postal{}, fmt.Errorf("%w: imb: character %q not encodable", barcode.ErrInvalidData, c)
		}
	}
	if len(d) < 20 {
		return barcode.Postal{}, fmt.Errorf("%w: imb: tracking code needs 20 digits", barcode.ErrInvalidData)
	}
	track, zip := d[:20], d[20:]
	if track[1] > '4' {
		return barcode.Postal{}, fmt.Errorf("%w: imb: second barcode ID digit must be 0-4", barcode.ErrInvalidData)
	}

	// Step 1: binary data.
	v := new(big.Int)
	switch len(zip) {
	case 0:
	case 5, 9, 11:
		v.SetString(string(zip), 10)
		off := map[int]int64{5: 1, 9: 100001, 11: 1000100001}[len(zip)]
		v.Add(v, big.NewInt(off))
	default:
		return barcode.Postal{}, fmt.Errorf("%w: imb: routing code must have 0, 5, 9 or 11 digits", barcode.ErrInvalidData)
	}
	mulAdd := func(m, a byte) {
		v.Mul(v, big.NewInt(int64(m)))
		v.Add(v, big.NewInt(int64(a-'0')))
	}
	mulAdd(10, track[0])
	mulAdd(5, track[1])
	for _, c := range track[2:] {
		mulAdd(10, c)
	}

	// Step 2: CRC-11 over the 102 bits held in 13 bytes.
	var bytes [13]byte
	v.FillBytes(bytes[:])
	fcs := crc11(bytes)

	// Step 3: codewords A..J.
	var cw [10]int
	r := new(big.Int)
	v.DivMod(v, big.NewInt(636), r)
	cw[9] = int(r.Int64())
	for i := 8; i >= 1; i-- {
		v.DivMod(v, big.NewInt(1365), r)
		cw[i] = int(r.Int64())
	}
	cw[0] = int(v.Int64())

	// Step 4: orientation and FCS bit.
	cw[9] *= 2
	if fcs&0x400 != 0 {
		cw[0] += 659
	}

	// Step 5: codewords to characters.
	var chars [10]int
	for i, c := range cw {
		if c < len(table5of13) {
			chars[i] = table5of13[c]
		} else {
			chars[i] = table2of13[c-len(table5of13)]
		}
		if fcs>>i&1 != 0 {
			chars[i] = ^chars[i] & 0x1fff
		}
	}

	// Step 6: characters to bars.
	var bit [130]bool
	for i := 0; i < 10; i++ {
		for j := 0; j < 13; j++ {
			bit[barMap[13*i+j]-1] = chars[i]>>j&1 != 0
		}
	}
	bars := make([]barcode.BarState, 65)
	for k := range bars {
		desc, asc := bit[k], bit[k+65]
		switch {
		case asc && desc:
			bars[k] = barcode.BarFull
		case asc:
			bars[k] = barcode.BarAscender
		case desc:
			bars[k] = barcode.BarDescender
		default:
			bars[k] = barcode.BarTracker
		}
	}
	return barcode.Postal{Bars: bars, Text: data}, nil
}

// crc11 is USPS_MSB_Math_CRC11GenerateFrameCheckSequence from USPS-B-3200.
func crc11(b [13]byte) int {
	const poly = 0x0F35
	fcs := 0x07FF
	data := int(b[0]) << 5
	for bit := 2; bit < 8; bit++ {
		if (fcs^data)&0x400 != 0 {
			fcs = fcs<<1 ^ poly
		} else {
			fcs <<= 1
		}
		fcs &= 0x7FF
		data <<= 1
	}
	for _, x := range b[1:] {
		data = int(x) << 3
		for bit := 0; bit < 8; bit++ {
			if (fcs^data)&0x400 != 0 {
				fcs = fcs<<1 ^ poly
			} else {
				fcs <<= 1
			}
			fcs &= 0x7FF
			data <<= 1
		}
	}
	return fcs
}

// table5of13 and table2of13 are USPS-B-3200 Appendix D tables I and II,
// generated with the Appendix C algorithm.
var table5of13, table2of13 = nOf13(5, 1287), nOf13(2, 78)

func nOf13(n, size int) []int {
	t := make([]int, size)
	lo, hi := 0, size-1
	for c := 0; c < 8192; c++ {
		if popcount(c) != n {
			continue
		}
		rev := reverse13(c)
		if rev < c {
			continue
		}
		if rev == c {
			t[hi] = c
			hi--
		} else {
			t[lo], t[lo+1] = c, rev
			lo += 2
		}
	}
	return t
}

func popcount(x int) int {
	n := 0
	for ; x != 0; x &= x - 1 {
		n++
	}
	return n
}

func reverse13(x int) int {
	r := 0
	for i := 0; i < 13; i++ {
		r = r<<1 | x>>i&1
	}
	return r
}

// barMap is USPS-B-3200 Appendix D table IV as a reverse lookup: entry
// 13*char+bit is the 1-based bar position (1..65 descenders, 66..130
// ascenders) driven by that bit of character A..J.
var barMap = [130]int{
	67, 6, 78, 16, 86, 95, 34, 40, 45, 113, 117, 121, 62, 87, 18, 104, 41, 76, 57, 119,
	115, 72, 97, 2, 127, 26, 105, 35, 122, 52, 114, 7, 24, 82, 68, 63, 94, 44, 77, 112,
	70, 100, 39, 30, 107, 15, 125, 85, 10, 65, 54, 88, 20, 106, 46, 66, 8, 116, 29, 61,
	99, 80, 90, 37, 123, 51, 25, 84, 129, 56, 4, 109, 96, 28, 36, 47, 11, 71, 33, 102,
	21, 9, 17, 49, 124, 79, 64, 91, 42, 69, 53, 60, 14, 1, 27, 103, 126, 75, 89, 50,
	120, 19, 32, 110, 92, 111, 130, 59, 31, 12, 81, 43, 55, 5, 74, 22, 101, 128, 58, 118,
	48, 108, 38, 98, 93, 23, 83, 13, 73, 3,
}
