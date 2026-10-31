package databar

import (
	"strings"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/gs1"
)

// bitString is an MSB-first bit accumulator.
type bitString []bool

func (b *bitString) add(v, n int) {
	for i := n - 1; i >= 0; i-- {
		*b = append(*b, v>>i&1 == 1)
	}
}

func (b bitString) value(start, n int) int {
	v := 0
	for i := start; i < start+n; i++ {
		v <<= 1
		if b[i] {
			v |= 1
		}
	}
	return v
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func atoi(s string) int { return int(atoi64(s)) }

// expDate converts YYMMDD to the compressed date value, or -1.
func expDate(s string) int {
	if !isDigits(s) || len(s) != 6 {
		return -1
	}
	yy, mm, dd := atoi(s[0:2]), atoi(s[2:4]), atoi(s[4:6])
	if mm < 1 || mm > 12 || dd > 31 {
		return -1
	}
	return yy*384 + (mm-1)*32 + dd
}

// expMethod selects the encodation method (ISO/IEC 24724 7.2.5.4, Table
// 10): 1 = (01)+general, 2 = general only, 3 = (01)+(3103), 4 =
// (01)+(3202/3203), 5 = (01)+(392x), 6 = (01)+(393x), 7–14 =
// (01)+(310x/320x)[+(11/13/15/17)].
func expMethod(src string) int {
	if len(src) < 16 || src[:2] != "01" || !isDigits(src[2:16]) || gs1.CheckDigit(src[2:15]) != src[15] {
		return 2
	}
	if len(src) < 20 || src[2] != '9' || src[16] != '3' {
		return 1
	}
	switch {
	case len(src) >= 26 && (src[17:19] == "10" || src[17:19] == "20") && isDigits(src[19:26]):
		weight := atoi(src[20:26])
		if weight > 99999 {
			return 1
		}
		metric := src[17] == '1'
		if len(src) == 26 {
			if metric && src[19] == '3' && weight <= 32767 {
				return 3
			}
			if !metric && ((src[19] == '2' && weight <= 9999) || (src[19] == '3' && weight <= 22767)) {
				return 4
			}
			if metric {
				return 7
			}
			return 8
		}
		if len(src) == 34 && src[26] == '1' && strings.ContainsRune("1357", rune(src[27])) && expDate(src[28:34]) >= 0 {
			if metric {
				return 6 + int(src[27]-'0') // 7, 9, 11, 13
			}
			return 7 + int(src[27]-'0') // 8, 10, 12, 14
		}
	case src[17] == '9' && src[19] >= '0' && src[19] <= '3':
		if src[18] == '2' {
			return 5
		}
		if src[18] == '3' && len(src) >= 23 && isDigits(src[20:23]) {
			return 6
		}
	}
	return 1
}

// General-purpose field modes (7.2.5.5).
const (
	gfNumeric = iota + 1
	gfAlpha
	gfISO
)

const alphaPuncs = "*,-./"
const isoPuncs = "!\"%&'()*+,-./:;<=>?_ "

func gfType(c byte) int {
	switch {
	case c == FNC1 || (c >= '0' && c <= '9'):
		return gfNumeric
	case (c >= 'A' && c <= 'Z') || strings.IndexByte(alphaPuncs, c) >= 0:
		return gfAlpha
	case (c >= 'a' && c <= 'z') || strings.IndexByte(isoPuncs, c) >= 0:
		return gfISO
	}
	return 0
}

// gfNext reports whether the num characters from i are all of type t (or t2
// when non-zero).
func gfNext(f string, i, num, t, t2 int) bool {
	if i+num > len(f) {
		return false
	}
	for k := i; k < i+num; k++ {
		ty := gfType(f[k])
		if ty != t && (t2 == 0 || ty != t2) {
			return false
		}
	}
	return true
}

// gfNextTerminate reports whether the rest of f from i is at most maxNum
// and at least num characters of type t.
func gfNextTerminate(f string, i, num, maxNum, t int) bool {
	if i+maxNum < len(f) || len(f)-i < num {
		return false
	}
	for k := i; k < len(f); k++ {
		if gfType(f[k]) != t {
			return false
		}
	}
	return true
}

// gfNextNone reports whether none of the next num characters (or up to the
// end) are of type t.
func gfNextNone(f string, i, num, t int) bool {
	for k := i; k < len(f) && k < i+num; k++ {
		if gfType(f[k]) == t {
			return false
		}
	}
	return true
}

// encodeGeneral appends the general-purpose data field (7.2.5.5.1–3) and
// returns the final mode and a pending odd final digit (0 if none).
func encodeGeneral(f string, b *bitString) (mode int, lastDigit byte, err error) {
	mode = gfNumeric
	digit := func(c byte) int {
		if c == FNC1 {
			return 10
		}
		return int(c - '0')
	}
	for i := 0; i < len(f); {
		t := gfType(f[i])
		if t == 0 {
			return 0, 0, invalid("character %q cannot be encoded in DataBar Expanded", f[i])
		}
		c := f[i]
		switch mode {
		case gfNumeric:
			if i < len(f)-1 {
				if t != gfNumeric || gfType(f[i+1]) != gfNumeric {
					b.add(0, 4) // alphanumeric latch
					mode = gfAlpha
				} else {
					b.add(11*digit(c)+digit(f[i+1])+8, 7)
					i += 2
				}
			} else {
				if t != gfNumeric {
					b.add(0, 4)
					mode = gfAlpha
				} else {
					lastDigit = c
					i++
				}
			}
		case gfAlpha:
			switch {
			case c == FNC1:
				b.add(15, 5)
				mode = gfNumeric
				i++
			case t == gfISO:
				b.add(4, 5) // ISO/IEC 646 latch
				mode = gfISO
			case gfNext(f, i, 6, gfNumeric, 0), gfNextTerminate(f, i, 4, 5, gfNumeric):
				b.add(0, 3) // numeric latch
				mode = gfNumeric
			case c >= '0' && c <= '9':
				b.add(int(c)-43, 5)
				i++
			case c >= 'A' && c <= 'Z':
				b.add(int(c)-33, 6)
				i++
			default:
				b.add(strings.IndexByte(alphaPuncs, c)+58, 6)
				i++
			}
		case gfISO:
			if c == FNC1 {
				b.add(15, 5)
				mode = gfNumeric
				i++
				break
			}
			none := gfNextNone(f, i, 10, gfISO)
			switch {
			case none && gfNext(f, i, 4, gfNumeric, 0):
				b.add(0, 3)
				mode = gfNumeric
			case none && gfNext(f, i, 5, gfAlpha, gfNumeric):
				b.add(4, 5) // alphanumeric latch
				mode = gfAlpha
			case c >= '0' && c <= '9':
				b.add(int(c)-43, 5)
				i++
			case c >= 'A' && c <= 'Z':
				b.add(int(c)-1, 7)
				i++
			case c >= 'a' && c <= 'z':
				b.add(int(c)-7, 7)
				i++
			default:
				b.add(strings.IndexByte(isoPuncs, c)+232, 8)
				i++
			}
		}
	}
	return mode, lastDigit, nil
}

// expBinary builds the complete Expanded bit string (7.2.5) including the
// linkage flag, padding and variable length field. charsPerRow (0 for a
// single row) lets stacked symbols avoid a lone final character.
func expBinary(src string, linkage bool, charsPerRow int) (bitString, int, error) {
	if len(src) > 77 {
		return nil, 0, invalid("GS1 data too long for DataBar Expanded (%d > 77 characters)", len(src))
	}
	var b bitString
	if linkage {
		b.add(1, 1)
	} else {
		b.add(0, 1)
	}
	method := expMethod(src)
	var read int
	switch method {
	case 1:
		b.add(4, 3) // "1XX"
		read = 16
	case 2:
		b.add(0, 4) // "00XX"
		read = 0
	case 3, 4:
		b.add(4+method-3, 4) // "0100", "0101"
		read = 26
	case 5:
		b.add(0x30, 7) // "01100XX"
		read = 20
	case 6:
		b.add(0x34, 7) // "01101XX"
		read = 23
	default:
		b.add(56+method-7, 7) // "0111000".."0111111"
		read = len(src)
	}

	// Compressed data field.
	switch {
	case method == 1:
		b.add(int(src[2]-'0'), 4)
		for i := 3; i < 15; i += 3 {
			b.add(atoi(src[i:i+3]), 10)
		}
	case method >= 3:
		for i := 3; i < 15; i += 3 { // indicator digit 9 implied
			b.add(atoi(src[i:i+3]), 10)
		}
		switch {
		case method == 3:
			b.add(atoi(src[20:26]), 15)
		case method == 4:
			w := atoi(src[20:26])
			if src[19] == '3' {
				w += 10000
			}
			b.add(w, 15)
		case method == 5 || method == 6:
			b.add(int(src[19]-'0'), 2)
			if method == 6 {
				b.add(atoi(src[20:23]), 10)
			}
		default: // 7–14
			b.add(atoi(src[19:20]+src[21:26]), 20)
			date := 38400 // no date
			if len(src) == 34 {
				date = expDate(src[28:34])
			}
			b.add(date, 16)
		}
	}

	mode, last := gfNumeric, byte(0)
	if read < len(src) {
		var err error
		if mode, last, err = encodeGeneral(src[read:], &b); err != nil {
			return nil, 0, err
		}
	}

	size := func() int {
		n := (len(b)+11)/12 + 1
		if charsPerRow > 0 && n%charsPerRow == 1 {
			n++
		}
		return max(n, 4)
	}
	symChars := size()
	if last != 0 {
		rem := 12*(symChars-1) - len(b)
		if rem >= 4 && rem <= 6 {
			b.add(int(last-'0')+1, 4) // 7.2.5.5.1 c) 2)
		} else {
			b.add(int(last-'0')*11+10+8, 7) // digit + FNC1
		}
		symChars = size()
	}
	if len(b) > 252 {
		return nil, 0, invalid("GS1 data needs %d symbol characters, maximum is 22", (len(b)+11)/12+1)
	}

	// Padding (7.2.5.5.4): numeric mode first latches to alphanumeric.
	target := 12 * (symChars - 1)
	if len(b) < target && mode == gfNumeric {
		b.add(0, 4)
	}
	for len(b) < target {
		b.add(4, 5) // "00100"
	}
	b = b[:target]

	// Variable length symbol field.
	vlf := -1
	switch method {
	case 1:
		vlf = 2
	case 2:
		vlf = 3
	case 5, 6:
		vlf = 6
	}
	if vlf >= 0 {
		b[vlf] = symChars%2 == 1
		b[vlf+1] = symChars > 14
	}
	return b, symChars, nil
}

func expGroup(v int) int {
	for g := range 4 {
		if v < expGSum[g+1] {
			return g
		}
	}
	return 4
}

func expCharWidths(v int) []int {
	g := expGroup(v)
	v -= expGSum[g]
	return charWidths(v/expTEven[g], v%expTEven[g], expOddMod[g], 17-expOddMod[g], 4, expWidest[g], true)
}

// expElements lays out the symbol characters and finders (7.2.6–7.2.7):
// element 0 and 1 are the left guard, the right guard occupies the last two.
func expElements(b bitString, symChars int) (elements []int, codeblocks int) {
	dataChars := symChars - 1
	cw := make([][]int, dataChars)
	for i := range dataChars {
		cw[i] = expCharWidths(b.value(12*i, 12))
	}
	sum := 0
	wrow := expWeightRows[(dataChars-2)/2]
	for i := range dataChars {
		for j := range 8 {
			sum += cw[i][j] * expWeights[wrow[i]][j]
		}
	}
	check := expCharWidths(211*(symChars-4) + sum%211)

	codeblocks = (symChars + 1) / 2
	width := codeblocks*5 + symChars*8 + 4
	elements = make([]int, width)
	seq := expFinderSeq[(symChars-1)/2-1]
	for i := range codeblocks {
		copy(elements[21*i+10:], expFinder[seq[i]-1][:])
	}
	copy(elements[2:], check)
	for i := 1; i < dataChars; i += 2 {
		copy(elements[(i-1)/2*21+23:], cw[i])
	}
	for i := 0; i < dataChars; i += 2 {
		k := i/2*21 + 15
		for j := range 8 {
			elements[k+j] = cw[i][7-j]
		}
	}
	return elements, codeblocks
}

func encodeExpanded(data string, opt Options) (Symbol, error) {
	src, text, err := data, data, error(nil)
	if !opt.ZebraParentheses || !strings.Contains(data, "(") {
		if src, text, err = ParseElementString(data); err != nil {
			return Symbol{}, err
		}
	}
	seg := opt.Segments
	if seg <= 0 || seg > 22 {
		seg = 22
	}
	cols := max(seg/2, 1)
	if opt.Linkage && cols < 2 {
		cols = 2 // 7.2.8: at least four symbol characters in the first row of a composite
	}
	charsPerRow := 0
	if cols < 11 {
		charsPerRow = 2 * cols
	}
	b, symChars, err := expBinary(src, opt.Linkage, charsPerRow)
	if err != nil {
		return Symbol{}, err
	}
	elements, codeblocks := expElements(b, symChars)
	s := Symbol{Text: text}

	if codeblocks <= cols {
		elements[0], elements[1] = 1, 1
		elements[len(elements)-2], elements[len(elements)-1] = 1, 1
		row := expand(elements, false)
		s.Matrix.Rows = [][]bool{row}
		s.Matrix.RowHeights = []int{34}
		s.Linear, s.LeftOffset = toLinear(row, text)
		if opt.Linkage {
			s.CompositeSeparator = expSeparator(row, codeblocks, 0, true, false, nil)
		}
		return s, nil
	}

	stackRows := (codeblocks + cols - 1) / cols
	block := 0
	v2 := false
	var rows [][]bool
	var heights []int
	for r := 1; r <= stackRows; r++ {
		numCols := cols
		if r*cols > codeblocks {
			numCols = codeblocks - block
		}
		sub := make([]int, 2+21*numCols+2)
		sub[0], sub[1] = 1, 1
		special := 0
		// Partial, even-numbered last row with an even column count and an
		// odd number of finders starts with an extra space so it reads
		// left to right (7.2.8).
		if r == stackRows && numCols != cols && r%2 == 0 && cols%2 == 0 && numCols%2 == 1 {
			special = 1
			sub[0] = 2
		}
		ltr := cols%2 == 1 || r%2 == 1 || special == 1
		for reader := 0; reader < numCols; reader++ {
			i := 2 + block*21
			for j := range 21 {
				if i+j < len(elements)-2 { // guard slots are not data
					if ltr {
						sub[j+reader*21+2] = elements[i+j]
					} else {
						sub[(20-j)+(numCols-1-reader)*21+2] = elements[i+j]
					}
				}
			}
			block++
		}
		sub[len(sub)-2], sub[len(sub)-1] = 1, 1
		dark := !(r%2 == 1 || special == 1)
		row := expand(sub, dark)

		if r != 1 {
			oddLast := r == stackRows && (symChars-1)%2 == 0
			mid := make([]bool, max(len(row), len(rows[len(rows)-2])))
			for j := 5; j < 49*cols && j < len(mid); j += 2 {
				mid[j] = true
			}
			l := v2
			bottom := expSeparator(row, numCols, special, ltr, oddLast, &l)
			rows = append(rows, mid, bottom)
			heights = append(heights, 1, 1)
		}
		rows = append(rows, row)
		heights = append(heights, 34)
		if r != stackRows {
			top := expSeparator(row, numCols, 0, ltr, false, &v2)
			rows = append(rows, top)
			heights = append(heights, 1)
		}
	}
	width := 0
	for _, row := range rows {
		width = max(width, len(row))
	}
	for i, row := range rows {
		for len(row) < width {
			row = append(row, false)
		}
		rows[i] = row
		if heights[i] == 1 {
			s.SeparatorRows = append(s.SeparatorRows, i)
		}
	}
	s.Matrix.Rows = rows
	s.Matrix.RowHeights = heights
	if opt.Linkage {
		s.CompositeSeparator = expSeparator(rows[0], cols, 0, true, false, nil)
	}
	return s, nil
}

// expSeparator builds the separator row adjacent to an Expanded row
// (7.2.8 / composite separator): the row's complement inside 4X margins,
// with the 13 modules over each finder alternating over its spaces. v2
// tracks the finder version alternation across rows; when non-nil it is
// updated.
func expSeparator(row []bool, cols, special int, ltr, oddLast bool, v2 *bool) []bool {
	width := len(row)
	sep := make([]bool, width)
	for j := 4 + special; j < width-4; j++ {
		sep[j] = !row[j]
	}
	latch := false
	if v2 != nil {
		latch = *v2
	}
	set := func(i int, space *bool) {
		if i < 0 || i >= width {
			return
		}
		if row[i] {
			sep[i] = false
			*space = false
		} else {
			sep[i] = !*space
			*space = !*space
		}
	}
	space := false
	for j := range cols {
		k := 49*j + 19 + special
		if ltr {
			start, end := 0, 13
			if latch {
				start, end = 2, 15
			}
			for i := start; i < end; i++ {
				set(i+k, &space)
			}
		} else {
			if oddLast {
				k -= 17
			}
			start, end := 12, 0
			if latch {
				start, end = 14, 2
			}
			for i := start; i >= end; i-- {
				set(i+k, &space)
			}
		}
		latch = !latch
	}
	if v2 != nil {
		*v2 = latch
	}
	return sep
}
