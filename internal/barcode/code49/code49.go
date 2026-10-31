// Package code49 encodes Code 49 (ANSI/AIM BC6-2000, USS Code 49) as
// produced by the ZPL ^B4 command.
//
// A symbol has 2–8 rows. Each row is a 1X bar + 1X space start pattern, four
// 16-module symbol characters (each encoding two code characters, the last
// of the row being the row check character) and a 4X bar stop pattern: 70X
// wide. Rows are separated by 1X high separator bars, and per the AIM spec
// the symbol also has 1X separator bars above the first and below the last
// row (Options.NoBorder drops these two).
//
// Units: the Matrix is in modules. RowHeights gives data rows
// Options.RowHeight modules (Zebra's ^B4 h: "multiplied by the module width
// gives the row height in dots") and separator rows 1 module, so the renderer
// scales every RowHeights entry by the ^BY module width in dots.
//
// The ^B4 f parameter (interpretation line N/A/B) is a renderer concern:
// Symbol.Text is the decoded data to print (Zebra notes it may run past the
// right edge for symbols over two rows).
package code49

import (
	"fmt"
	"strings"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
)

// internalSet is the Code 49 code character set in value order (0–48) as
// sent in ZPL field data for starting modes 0–5: '<' Shift 1, '>' Shift 2,
// ':' ';' '?' FNC1–FNC3 (listed "N.A." by Zebra) and '=' Numeric Shift.
const internalSet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ-. $/+%<>:;?="

// Code character values of the non-data characters.
const (
	shift1  = 43
	shift2  = 44
	fnc1    = 45
	fnc3    = 47
	numeric = 48 // Numeric Shift <NS>, also the pad character
)

// Options control Code 49 encoding.
type Options struct {
	// Mode is Zebra's ^B4 m starting mode: '0' regular alphanumeric, '1'
	// multiple read (concatenate) alphanumeric, '2' regular numeric, '3'
	// group alphanumeric, '4' alphanumeric Shift 1, '5' alphanumeric Shift 2,
	// or 'A' automatic (the default, also used for 0).
	//
	// In automatic mode the data is any ASCII text. In modes 0–5 the data
	// is written in the internal character set (see internalSet); as on a
	// Zebra printer, an invalid sequence ends the data and the symbol holds
	// only what precedes it.
	Mode byte

	// RowHeight is the data row height in modules (^B4 h). 0 means 8, the
	// AIM minimum.
	RowHeight int

	// NoBorder omits the 1X separator bars above the first and below the
	// last row.
	NoBorder bool
}

// Symbol is an encoded Code 49 symbol.
type Symbol struct {
	Matrix barcode.Matrix
	// SeparatorRows lists the separator (and border) rows of Matrix.
	SeparatorRows []int
	// Codewords holds the 8 code characters of each row.
	Codewords [][8]int
	// Mode is the starting mode value M (0–5) encoded in the last row.
	Mode int
	// Text is the human readable interpretation (lower case letters print
	// as lower case, as Zebra notes; function characters are omitted).
	Text string
}

// Encode encodes data as a Code 49 symbol.
func Encode(data string, opt Options) (Symbol, error) {
	var cw []int
	var mode int
	switch opt.Mode {
	case 0, 'A', 'a':
		var err error
		if cw, mode, err = autoCodewords(data); err != nil {
			return Symbol{}, err
		}
	case '0', '1', '2', '3', '4', '5':
		mode = int(opt.Mode - '0')
		cw = manualCodewords(data, mode)
	default:
		return Symbol{}, invalid("unknown starting mode %q", opt.Mode)
	}
	if len(cw) == 0 {
		return Symbol{}, invalid("no encodable data in %q", data)
	}
	if len(cw) > 49 {
		return Symbol{}, invalid("data needs %d code characters, maximum is 49", len(cw))
	}
	grid := layout(cw, mode)

	h := opt.RowHeight
	if h <= 0 {
		h = 8
	}
	s := Symbol{Codewords: grid, Mode: mode, Text: decodeCodewords(cw, mode)}
	if opt.Mode == 0 || opt.Mode == 'A' || opt.Mode == 'a' {
		s.Text = data
	}
	sep := func() {
		bar := make([]bool, 70)
		for i := range bar {
			bar[i] = true
		}
		s.SeparatorRows = append(s.SeparatorRows, len(s.Matrix.Rows))
		s.Matrix.Rows = append(s.Matrix.Rows, bar)
		s.Matrix.RowHeights = append(s.Matrix.RowHeights, 1)
	}
	if !opt.NoBorder {
		sep()
	}
	for r := range grid {
		if r > 0 {
			sep()
		}
		s.Matrix.Rows = append(s.Matrix.Rows, rowModules(grid, r))
		s.Matrix.RowHeights = append(s.Matrix.RowHeights, h)
	}
	if !opt.NoBorder {
		sep()
	}
	return s, nil
}

func invalid(format string, a ...any) error {
	return fmt.Errorf("code49: "+format+": %w", append(a, barcode.ErrInvalidData)...)
}

// asciiCodes returns the code characters (one, or a shift and a
// character) for an ASCII character (Table 7).
func asciiCodes(c byte) []int {
	idx := func(ch byte) int { return strings.IndexByte(internalSet, ch) }
	s1 := func(ch byte) []int { return []int{shift1, idx(ch)} }
	s2 := func(ch byte) []int { return []int{shift2, idx(ch)} }
	switch {
	case c == 0:
		return s1(' ')
	case c >= 1 && c <= 26:
		return s1('A' + c - 1)
	case c >= 27 && c <= 31:
		return s1('1' + c - 27)
	case c >= 'a' && c <= 'z':
		return s2(c - 'a' + 'A')
	case c == 127:
		return s2(' ')
	}
	if c < 127 && idx(c) >= 0 && idx(c) <= 42 {
		return []int{idx(c)}
	}
	shifted := map[byte][]int{
		'!': s1('6'), '"': s1('7'), '#': s1('8'), '&': s1('9'), '\'': s1('0'),
		'(': s1('-'), ')': s1('.'), '*': s1('$'), ',': s1('/'), ':': s1('+'),
		';': s2('1'), '<': s2('2'), '=': s2('3'), '>': s2('4'), '?': s2('5'), '@': s2('6'),
		'[': s2('7'), '\\': s2('8'), ']': s2('9'), '^': s2('0'), '_': s2('-'), '`': s2('.'),
		'{': s2('$'), '|': s2('/'), '}': s2('+'), '~': s2('%'),
	}
	return shifted[c]
}

// packDigits appends the numeric encodation (2.2.2) of a run of digits.
// len(d) must not be 2.
func packDigits(cw []int, d string) []int {
	three := func(v int) []int { return []int{v / 2304, v / 48 % 48, v % 48} }
	for len(d) > 0 {
		switch {
		case len(d) == 7 || len(d) == 4: // 5n+2 -> 4 then 3; 5n+4 -> 4
			cw = append(cw, three(100000+atoi(d[:4]))...)
			d = d[4:]
		case len(d) == 3:
			v := atoi(d)
			cw = append(cw, v/48, v%48)
			d = ""
		case len(d) == 1:
			cw = append(cw, int(d[0]-'0'))
			d = ""
		default:
			cw = append(cw, three(atoi(d[:5]))...)
			d = d[5:]
		}
	}
	return cw
}

func atoi(s string) int {
	v := 0
	for i := 0; i < len(s); i++ {
		v = v*10 + int(s[i]-'0')
	}
	return v
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// autoCodewords implements Zebra's automatic mode: full ASCII via shifts,
// numeric encodation for runs of five or more digits, and the starting mode
// chosen from the first code character (2.3).
func autoCodewords(data string) ([]int, int, error) {
	var cw []int
	for i := 0; i < len(data); {
		if isDigit(data[i]) {
			j := i
			for j < len(data) && isDigit(data[j]) {
				j++
			}
			if j-i >= 5 {
				cw = append(cw, numeric)
				cw = packDigits(cw, data[i:j])
				if j < len(data) {
					cw = append(cw, numeric)
				}
			} else {
				for k := i; k < j; k++ {
					cw = append(cw, int(data[k]-'0'))
				}
			}
			i = j
			continue
		}
		codes := asciiCodes(data[i])
		if codes == nil {
			return nil, 0, invalid("character %q is not ASCII", data[i])
		}
		cw = append(cw, codes...)
		i++
	}
	mode := 0
	if len(cw) > 0 {
		switch cw[0] {
		case numeric:
			mode = 2
		case shift1:
			mode = 4
		case shift2:
			mode = 5
		}
	}
	if mode != 0 {
		cw = cw[1:]
	}
	return cw, mode, nil
}

// manualCodewords converts ZPL internal-character-set data for starting
// modes 0–5, stopping at the first invalid sequence as Zebra does.
func manualCodewords(data string, mode int) []int {
	var cw []int
	num := mode == 2
	shift := 0
	switch mode {
	case 4:
		shift = 1
	case 5:
		shift = 2
	}
	for i := 0; i < len(data); {
		if num {
			j := i
			for j < len(data) && isDigit(data[j]) {
				j++
			}
			if j-i == 2 {
				return cw // invalid: numeric encodation has no 2-digit form
			}
			cw = packDigits(cw, data[i:j])
			if j < len(data) && data[j] != '=' {
				return cw // invalid: numeric mode ended without <NS>
			}
			i = j
			if i < len(data) { // '=' returns to alphanumeric
				cw = append(cw, numeric)
				i++
				num = false
			}
			continue
		}
		v := strings.IndexByte(internalSet, data[i])
		switch {
		case v < 0:
			return cw
		case shift != 0:
			if v > 42 || (shift == 1 && v == 42) { // S1 % is reserved
				return cw
			}
			cw = append(cw, v)
			shift = 0
		case v == shift1 || v == shift2:
			if i+1 >= len(data) {
				return cw
			}
			cw = append(cw, v)
			shift = v - shift1 + 1
		case v == numeric:
			cw = append(cw, v)
			num = true
		default:
			cw = append(cw, v)
		}
		i++
	}
	return cw
}

// layout places the code characters in rows of 7 plus a row check
// character, pads with <NS>, and adds the row count/mode character and the
// symbol check characters (2.1.2–2.1.4).
func layout(cw []int, mode int) [][8]int {
	var grid [][8]int
	pads := 0
	for r := 0; r == 0 || r*7 < len(cw); r++ {
		var row [8]int
		for i := range 7 {
			if k := r*7 + i; k < len(cw) {
				row[i] = cw[k]
			} else {
				row[i] = numeric
				pads++
			}
		}
		grid = append(grid, row)
	}
	// The last row needs 5 free positions (6 or more rows: all 7) for the
	// check and row-count characters; symbols have at least two rows.
	if (len(grid) <= 6 && pads < 5) || len(grid) > 6 || len(grid) == 1 {
		grid = append(grid, [8]int{numeric, numeric, numeric, numeric, numeric, numeric, numeric})
	}
	rows := len(grid)
	last := &grid[rows-1]
	last[6] = 7*(rows-2) + mode

	for i := range rows - 1 {
		sum := 0
		for j := range 7 {
			sum += grid[i][j]
		}
		grid[i][7] = sum % 49
	}

	x, y, z := last[6]*20, last[6]*16, last[6]*38
	pos := 0
	for i := range rows - 1 {
		for j := range 4 {
			w := grid[i][2*j]*49 + grid[i][2*j+1]
			x += xWeight[pos] * w
			y += yWeight[pos] * w
			z += zWeight[pos] * w
			pos++
		}
	}
	if rows > 6 {
		z %= 2401
		last[0], last[1] = z/49, z%49
	}
	w := last[0]*49 + last[1]
	x += xWeight[pos] * w
	y += yWeight[pos] * w
	pos++
	y %= 2401
	last[2], last[3] = y/49, y%49
	x += xWeight[pos] * (last[2]*49 + last[3])
	x %= 2401
	last[4], last[5] = x/49, x%49

	sum := 0
	for j := range 7 {
		sum += last[j]
	}
	last[7] = sum % 49
	return grid
}

// Table 5 check character weights (Cr7 weights 20, 16, 38 are applied
// separately).
var (
	xWeight = [32]int{1, 9, 31, 26, 2, 12, 17, 23, 37, 18, 22, 6, 27, 44, 15, 43, 39, 11, 13, 5, 41, 33, 36, 8, 4, 32, 3, 19, 40, 25, 29, 10}
	yWeight = [32]int{9, 31, 26, 2, 12, 17, 23, 37, 18, 22, 6, 27, 44, 15, 43, 39, 11, 13, 5, 41, 33, 36, 8, 4, 32, 3, 19, 40, 25, 29, 10, 24}
	zWeight = [32]int{31, 26, 2, 12, 17, 23, 37, 18, 22, 6, 27, 44, 15, 43, 39, 11, 13, 5, 41, 33, 36, 8, 4, 32, 3, 19, 40, 25, 29, 10, 24, 30}
)

// rowParity is Table 2 (true = odd parity) for rows 1–7; the last row is
// always even.
var rowParity = [7][4]bool{
	{true, false, false, true},
	{false, true, false, true},
	{true, true, false, false},
	{false, false, true, true},
	{true, false, true, false},
	{false, true, true, false},
	{true, true, true, true},
}

// rowModules renders row r: start "10", four symbol characters, stop "1111".
func rowModules(grid [][8]int, r int) []bool {
	row := []bool{true, false}
	for j := range 4 {
		w := grid[r][2*j]*49 + grid[r][2*j+1]
		p := evenPatterns[w]
		if r != len(grid)-1 && rowParity[r][j] {
			p = oddPatterns[w]
		}
		for b := 15; b >= 0; b-- {
			row = append(row, p>>b&1 == 1)
		}
	}
	return append(row, true, true, true, true)
}

// shiftTables map code characters 0–42 to ASCII under Shift 1 / Shift 2
// (Table 7); 0 marks the reserved S1 %.
var shiftTables = func() [3][43]byte {
	var t [3][43]byte
	for c := 0; c < 128; c++ {
		codes := asciiCodes(byte(c))
		switch len(codes) {
		case 1:
			t[0][codes[0]] = byte(c)
		case 2:
			t[codes[0]-shift1+1][codes[1]] = byte(c)
		}
	}
	return t
}()

// decodeCodewords converts code characters back to text (the
// interpretation line).
func decodeCodewords(cw []int, mode int) string {
	var b strings.Builder
	num := mode == 2
	shift := 0
	switch mode {
	case 4:
		shift = 1
	case 5:
		shift = 2
	}
	for i := 0; i < len(cw); {
		if num {
			j := i
			for j < len(cw) && cw[j] != numeric {
				j++
			}
			seg := cw[i:j]
			for len(seg) >= 3 {
				v := seg[0]*2304 + seg[1]*48 + seg[2]
				if v >= 100000 {
					fmt.Fprintf(&b, "%04d", v-100000)
				} else {
					fmt.Fprintf(&b, "%05d", v)
				}
				seg = seg[3:]
			}
			switch len(seg) {
			case 2:
				fmt.Fprintf(&b, "%03d", seg[0]*48+seg[1])
			case 1:
				b.WriteByte(byte('0' + seg[0]))
			}
			num = false
			i = j + 1
			continue
		}
		c := cw[i]
		i++
		switch {
		case c == numeric:
			num = true
		case c == shift1 || c == shift2:
			shift = c - shift1 + 1
		case c >= fnc1 && c <= fnc3:
		default:
			if ch := shiftTables[shift][c]; ch != 0 || (shift == 1 && c == 38) {
				b.WriteByte(ch)
			}
			shift = 0
		}
	}
	return b.String()
}
