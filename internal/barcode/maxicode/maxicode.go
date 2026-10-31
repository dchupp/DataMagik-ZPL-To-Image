// Package maxicode encodes MaxiCode (ISO/IEC 16023) symbols as produced by
// the ZPL II ^BD command.
//
// MaxiCode is a fixed-size symbol of 884 hexagonal modules in 33 rows around
// a central bullseye finder. Encode returns the module grid as a
// barcode.Matrix of Rows x Cols (33 x 30); see the geometry section below
// for how the rectangular grid maps onto hexagons. The bullseye is not part
// of the matrix: the renderer draws it (see FinderCenter and FinderRings).
package maxicode

import (
	"bytes"
	"fmt"
	"strconv"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
)

// Geometry.
//
// Rows[y][x] of the returned matrix is the hexagon in row y (0 = top) and
// column x (0 = left). Hexagons are "pointy-top" (vertices up and down) with
// width across the flats of 1 module W. In even rows (0, 2, ... 32) the
// hexagon centres are at x+0.5 W; odd rows (1, 3, ... 31) are offset right by
// half a module, centres at x+1.0 W, and only use columns 0..28 (column 29 of
// an odd row is always light). Row centres are RowPitch apart, the first at
// HexHeight/2. The whole symbol (excluding quiet zone) is SymbolWidth x
// SymbolHeight modules.
const (
	// Rows is the number of module rows.
	Rows = 33
	// Cols is the number of module columns (even rows; odd rows use 29).
	Cols = 30

	// HexWidth is the hexagon width across flats (W), the unit of the
	// geometry constants below.
	HexWidth = 1.0
	// HexHeight is the vertex-to-vertex height of a regular hexagon, 2/sqrt(3) W.
	HexHeight = 1.1547005383792515
	// RowPitch is the vertical distance between row centres, sqrt(3)/2 W.
	RowPitch = 0.8660254037844386
	// SymbolWidth is the overall symbol width in W.
	SymbolWidth = 30.0
	// SymbolHeight is the overall symbol height in W (32*RowPitch + HexHeight).
	SymbolHeight = 32*RowPitch + HexHeight

	// SymbolWidthMM and SymbolHeightMM are the ISO/IEC 16023 nominal printed
	// symbol dimensions in millimetres. Renderers that print at the nominal
	// size scale the geometry above by about SymbolWidthMM/SymbolWidth
	// horizontally and SymbolHeightMM/SymbolHeight vertically.
	SymbolWidthMM  = 28.14
	SymbolHeightMM = 26.91
	// ModuleWidthMM is the nominal W at the nominal symbol width.
	ModuleWidthMM = SymbolWidthMM / SymbolWidth
)

// ModuleCenter returns the centre of hexagon (x, y) in units of W from the
// top-left corner of the symbol.
func ModuleCenter(x, y int) (cx, cy float64) {
	cx = float64(x) + 0.5
	if y%2 == 1 {
		cx += 0.5
	}
	return cx, float64(y)*RowPitch + HexHeight/2
}

// FinderCenter returns the centre of the bullseye in units of W (the centre
// of the virtual module at row 16, column 14).
func FinderCenter() (cx, cy float64) { return ModuleCenter(14, 16) }

// FinderRings returns the inner and outer diameters, in W, of the three dark
// concentric rings of the bullseye, innermost first. The rings and the light
// gaps between them have equal width; the light centre has diameter
// HexHeight and the outer dark ring an outer diameter of 9 W.
func FinderRings() [3][2]float64 {
	step := (9 - HexHeight) / 5
	var r [3][2]float64
	for i := range r {
		r[i][0] = HexHeight + float64(2*i)*step
		r[i][1] = HexHeight + float64(2*i+1)*step
	}
	return r
}

// Message is the content of one MaxiCode symbol.
type Message struct {
	// Mode is the MaxiCode mode, 2 to 6.
	Mode int
	// PostalCode, CountryCode and ServiceClass form the primary
	// ("high priority") message in modes 2 and 3. Mode 2 postal codes are
	// 1-9 digits; mode 3 postal codes are up to 6 code set A characters
	// (space padded).
	PostalCode   string
	CountryCode  int
	ServiceClass int
	// Data is the secondary ("low priority") message in modes 2 and 3, or
	// the whole message in modes 4 to 6. Bytes are ISO/IEC 8859-1.
	Data []byte
}

// Options are the ^BD parameters.
type Options struct {
	// Mode is the ^BD mode m, 2 to 6 (0 means the ZPL default 2).
	Mode int
	// SymbolNumber (n) and SymbolCount (t) give the structured append
	// position, each 1 to 8 (0 means 1). With SymbolCount 1 no structured
	// append header is encoded.
	SymbolNumber int
	SymbolCount  int
}

func errf(format string, a ...any) error {
	return fmt.Errorf("%w: maxicode: "+format, append([]any{barcode.ErrInvalidData}, a...)...)
}

var scmHeader = []byte("[)>\x1e01\x1d")

// ParseZPL splits ^BD field data into a Message for the given mode (0 means
// 2).
//
// Modes 4, 5 and 6 take the field data as is. In modes 2 and 3 the field
// data starts with Zebra's fixed-format high-priority message:
//
//	mode 2: aaabbbcccccdddd  class of service, country, 5-digit ZIP, +4
//	mode 3: aaabbbcccccc     class of service, country, 6-char postal code
//
// and the rest is the low-priority message (typically "[)>RS01GS96..."),
// encoded verbatim as the secondary message.
//
// As an extension, data in the standard UPS/ANSI MH10.8.3 layout
// "[)>RS01GSyy<postal>GS<country>GS<class>GS<rest>" is also accepted: the
// three fields are moved to the primary message and the secondary message
// becomes "[)>RS01GSyy<rest>", as ISO/IEC 16023 Annex B prescribes. Readers
// reinsert the fields, so the decoded text equals the input.
func ParseZPL(mode int, fieldData []byte) (Message, error) {
	if mode == 0 {
		mode = 2
	}
	if mode < 2 || mode > 6 {
		return Message{}, errf("mode %d out of range 2-6", mode)
	}
	m := Message{Mode: mode}
	if mode > 3 {
		m.Data = fieldData
		return m, nil
	}
	if len(fieldData) >= 9 && bytes.HasPrefix(fieldData, scmHeader) && isDigits(fieldData[7:9]) {
		parts := bytes.SplitN(fieldData[9:], []byte{0x1d}, 4)
		if len(parts) < 3 {
			return Message{}, errf("structured carrier message lacks postal code, country and class fields")
		}
		m.PostalCode = string(parts[0])
		var err error
		if m.CountryCode, err = threeDigits(parts[1]); err != nil {
			return Message{}, errf("country code %q: must be 3 digits", parts[1])
		}
		if m.ServiceClass, err = threeDigits(parts[2]); err != nil {
			return Message{}, errf("class of service %q: must be 3 digits", parts[2])
		}
		m.Data = append([]byte{}, fieldData[:9]...)
		if len(parts) == 4 {
			m.Data = append(m.Data, parts[3]...)
		}
		if mode == 2 && m.CountryCode == 840 && len(m.PostalCode) == 5 {
			m.PostalCode += "0000" // ISO/IEC 16023 B.1.4a: unknown +4 is zero filled
		}
		return m, nil
	}
	hpm := 15
	if mode == 3 {
		hpm = 12
	}
	if len(fieldData) < hpm {
		return Message{}, errf("mode %d high priority message needs %d characters, have %d", mode, hpm, len(fieldData))
	}
	var err error
	if m.ServiceClass, err = threeDigits(fieldData[0:3]); err != nil {
		return Message{}, errf("class of service %q: must be 3 digits", fieldData[0:3])
	}
	if m.CountryCode, err = threeDigits(fieldData[3:6]); err != nil {
		return Message{}, errf("country code %q: must be 3 digits", fieldData[3:6])
	}
	m.PostalCode = string(fieldData[6:hpm])
	m.Data = fieldData[hpm:]
	return m, nil
}

func isDigits(b []byte) bool {
	for _, c := range b {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(b) > 0
}

func threeDigits(b []byte) (int, error) {
	if len(b) != 3 || !isDigits(b) {
		return 0, strconv.ErrSyntax
	}
	return strconv.Atoi(string(b))
}

// Encode encodes ^BD field data with the given parameters.
func Encode(fieldData []byte, opts Options) (barcode.Matrix, error) {
	m, err := ParseZPL(opts.Mode, fieldData)
	if err != nil {
		return barcode.Matrix{}, err
	}
	return EncodeMessage(m, opts.SymbolNumber, opts.SymbolCount)
}

// EncodeMessage encodes m as symbol symbolNumber of symbolCount (structured
// append; 0 means 1).
func EncodeMessage(m Message, symbolNumber, symbolCount int) (barcode.Matrix, error) {
	cw, err := Codewords(m, symbolNumber, symbolCount)
	if err != nil {
		return barcode.Matrix{}, err
	}
	return Place(cw), nil
}

// Codewords returns all 144 codewords of the symbol for m, including error
// correction, in ISO/IEC 16023 codeword order.
func Codewords(m Message, symbolNumber, symbolCount int) ([144]byte, error) {
	var cw [144]byte
	if symbolNumber == 0 {
		symbolNumber = 1
	}
	if symbolCount == 0 {
		symbolCount = 1
	}
	if symbolCount < 1 || symbolCount > 8 || symbolNumber < 1 || symbolNumber > symbolCount {
		return cw, errf("structured append symbol %d of %d out of range (1-8)", symbolNumber, symbolCount)
	}

	var prefix []byte
	if symbolCount > 1 {
		prefix = []byte{cwPad, byte((symbolCount - 1) | (symbolNumber-1)<<3)}
	}
	text, final := encodeText(m.Data)
	stream := append(prefix, text...)

	var capacity int
	switch m.Mode {
	case 2, 3:
		if err := primarySCM(&cw, m); err != nil {
			return cw, err
		}
		capacity = 84
	case 4, 6:
		capacity = 93
	case 5:
		capacity = 77
	default:
		return cw, errf("mode %d out of range 2-6", m.Mode)
	}
	if len(stream) > capacity {
		return cw, errf("data needs %d codewords, mode %d holds %d", len(stream), m.Mode, capacity)
	}
	if len(stream) < capacity {
		pad := byte(cwPad)
		switch final {
		case setC, setD:
			stream = append(stream, fval(final, fnLatchA))
		case setE:
			pad = fval(setE, fnPad)
		}
		for len(stream) < capacity {
			stream = append(stream, pad)
		}
	}

	var secondary []byte
	if m.Mode <= 3 {
		secondary = stream
	} else {
		cw[0] = byte(m.Mode)
		copy(cw[1:10], stream[:9])
		secondary = stream[9:]
	}
	copy(cw[10:20], rsEncode(cw[:10], 10))

	nd := len(secondary) // 84 (SEC) or 68 (EEC)
	ne := 144 - 20 - nd
	copy(cw[20:], secondary)
	for parity := 0; parity < 2; parity++ {
		half := make([]byte, 0, nd/2)
		for i := parity; i < nd; i += 2 {
			half = append(half, secondary[i])
		}
		for j, e := range rsEncode(half, ne/2) {
			cw[20+nd+2*j+parity] = e
		}
	}
	return cw, nil
}

// primarySCM fills codewords 0-9 with the structured carrier message of
// modes 2 and 3 (ISO/IEC 16023 4.9.1 and Annex B).
func primarySCM(cw *[144]byte, m Message) error {
	if m.CountryCode < 0 || m.CountryCode > 999 {
		return errf("country code %d out of range", m.CountryCode)
	}
	if m.ServiceClass < 0 || m.ServiceClass > 999 {
		return errf("class of service %d out of range", m.ServiceClass)
	}
	country, service := m.CountryCode, m.ServiceClass
	pc := []byte(m.PostalCode)
	if m.Mode == 2 {
		if len(pc) < 1 || len(pc) > 9 || !isDigits(pc) {
			return errf("mode 2 postal code %q must be 1-9 digits", pc)
		}
		v, _ := strconv.Atoi(string(pc))
		n := len(pc)
		cw[0] = byte((v&0x03)<<4 | 2)
		cw[1] = byte(v >> 2 & 0x3f)
		cw[2] = byte(v >> 8 & 0x3f)
		cw[3] = byte(v >> 14 & 0x3f)
		cw[4] = byte(v >> 20 & 0x3f)
		cw[5] = byte(v>>26&0x0f | (n&0x03)<<4)
		cw[6] = byte(n>>2&0x0f | (country&0x03)<<4)
	} else {
		if len(pc) > 6 {
			return errf("mode 3 postal code %q longer than 6 characters", pc)
		}
		for len(pc) < 6 {
			pc = append(pc, ' ')
		}
		var p [6]int
		for i, c := range pc {
			if c >= 'a' && c <= 'z' {
				c -= 'a' - 'A'
			}
			v := charValue[setA][c]
			if c < ' ' || v < 0 {
				return errf("mode 3 postal code %q: invalid character %q", m.PostalCode, c)
			}
			p[i] = int(v)
		}
		cw[0] = byte((p[5]&0x03)<<4 | 3)
		cw[1] = byte((p[4]&0x03)<<4 | p[5]>>2&0x0f)
		cw[2] = byte((p[3]&0x03)<<4 | p[4]>>2&0x0f)
		cw[3] = byte((p[2]&0x03)<<4 | p[3]>>2&0x0f)
		cw[4] = byte((p[1]&0x03)<<4 | p[2]>>2&0x0f)
		cw[5] = byte((p[0]&0x03)<<4 | p[1]>>2&0x0f)
		cw[6] = byte(p[0]>>2&0x0f | (country&0x03)<<4)
	}
	cw[7] = byte(country >> 2 & 0x3f)
	cw[8] = byte(country>>8&0x03 | (service&0x0f)<<2)
	cw[9] = byte(service >> 4 & 0x3f)
	return nil
}

// orientation lists the always-dark orientation modules (row, column).
var orientation = [][2]int{
	{0, 28}, {0, 29}, // top right filler
	{9, 10}, {9, 11}, {10, 11}, // top left
	{15, 7}, {16, 8}, // left
	{16, 20}, {17, 20}, // right
	{22, 10}, {23, 10}, // bottom left
	{22, 17}, {23, 17}, // bottom right
}

// Place lays out 144 codewords on the module grid per ISO/IEC 16023
// Figure 5 and adds the orientation modules. The finder area stays light.
func Place(cw [144]byte) barcode.Matrix {
	rows := make([][]bool, Rows)
	for y := range rows {
		rows[y] = make([]bool, Cols)
		for x := range rows[y] {
			if n := int(moduleSequence[y][x]); n > 0 {
				n--
				rows[y][x] = cw[n/6]>>(5-n%6)&1 == 1
			}
		}
	}
	for _, p := range orientation {
		rows[p[0]][p[1]] = true
	}
	return barcode.Matrix{Rows: rows}
}
