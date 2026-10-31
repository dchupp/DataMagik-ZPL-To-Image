// Package codablock encodes CODABLOCK A (Code 39 based) and CODABLOCK E/F
// (Code 128 based) stacked symbols for the ZPL ^BB command, following the
// AIM Codablock specifications (row indicators, K1/K2 symbol check
// characters, Code 128 row check characters and separator bars).
//
// Geometry: Encode returns a Matrix whose cells are square; one module
// (narrow element) is Options.Narrow cells wide. Matrix rows alternate
// separator and data rows: sep, row 1, sep, row 2, ..., row n, sep (2n+1
// rows). RowHeights holds each row's height in cells: separators
// SepHeight*Narrow, data rows RowHeight*Narrow. The top and bottom
// separators span the whole symbol; interior separators span the area
// between the start and stop characters, which continue through them.
// Quiet zones are not included.
package codablock

import (
	"fmt"
	"math"

	"github.com/datamagik/zpl-to-image/internal/barcode"
	"github.com/datamagik/zpl-to-image/internal/barcode/code128"
	"github.com/datamagik/zpl-to-image/internal/barcode/code39"
)

// Mode is the ^BB m parameter.
type Mode byte

const (
	ModeA Mode = 'A' // Code 39 character set
	ModeE Mode = 'E' // Code 128 with automatic leading FNC1 (GS1/UCC)
	ModeF Mode = 'F' // Code 128 (^BB default)
)

// Options controls Codablock encoding (^BB s, c, r, m and h).
type Options struct {
	Mode Mode // default ModeF
	// Columns is the number of data characters per row (^BB c, 2..62);
	// 0 = derived from Rows, or a single row (A) / two rows (E, F).
	Columns int
	// Rows (^BB r) is used to derive Columns when Columns is 0. The symbol
	// gets as many rows as the data needs (Zebra may print fewer or more
	// rows than requested).
	Rows int
	// NoCheck disables the K1/K2 check characters (^BB s=N); only honoured
	// for Codablock A. Single-row symbols never have them.
	NoCheck bool
	// RowHeight is the data row height in modules (^BB h; default 8).
	RowHeight int
	// SepHeight is the separator bar height in modules (default 1).
	SepHeight int
	// Narrow is the number of matrix cells per module (default 1).
	Narrow int
	// Ratio is the Codablock A wide:narrow ratio (2.0..3.0, default 3.0);
	// wide elements are round(Ratio*Narrow) cells.
	Ratio float64
}

// Zebra ^FH function characters for Codablock E and F.
const (
	ZebraFNC1 = 0x80
	ZebraFNC2 = 0x81
	ZebraFNC3 = 0x82
	ZebraFNC4 = 0x83
	ZebraNUL  = 0x84
)

func errf(format string, a ...any) error {
	return fmt.Errorf("%w: codablock: "+format, append([]any{barcode.ErrInvalidData}, a...)...)
}

// Encode encodes data. For modes E and F the bytes 0x80..0x84 stand for
// FNC1, FNC2, FNC3, FNC4 and NUL as with Zebra's ^FH; in mode F other bytes
// >= 0x85 are encoded as extended ASCII (FNC4 + char), in mode E they are
// rejected.
func Encode(data []byte, opts Options) (barcode.Matrix, error) {
	if opts.Mode == 0 {
		opts.Mode = ModeF
	}
	if opts.RowHeight <= 0 {
		opts.RowHeight = 8
	}
	if opts.SepHeight <= 0 {
		opts.SepHeight = 1
	}
	if opts.Narrow <= 0 {
		opts.Narrow = 1
	}
	if opts.Columns != 0 && (opts.Columns < 2 || opts.Columns > 62) {
		return barcode.Matrix{}, errf("columns %d out of range 2..62", opts.Columns)
	}
	switch opts.Mode {
	case ModeA:
		return encodeA(data, opts)
	case ModeE, ModeF:
		return encodeF(data, opts)
	}
	return barcode.Matrix{}, errf("unknown mode %q", opts.Mode)
}

// MinColumnsF is the minimum number of characters per row in CODABLOCK E
// and F. Smaller Columns values are widened to it.
const MinColumnsF = 4

// assemble stacks data rows (module bit rows, all the same width) with
// separators. sepL and sepR are the module widths at the left and right
// that interior separators copy from the first row (start and stop
// characters).
func assemble(rows [][]bool, sepL, sepR int, opts Options) barcode.Matrix {
	w := len(rows[0])
	full := make([]bool, w)
	for i := range full {
		full[i] = true
	}
	inner := make([]bool, w)
	for i := range inner {
		inner[i] = i >= sepL && i < w-sepR || rows[0][i]
	}
	n := opts.Narrow
	scale := func(r []bool) []bool {
		out := make([]bool, 0, len(r)*n)
		for _, d := range r {
			for k := 0; k < n; k++ {
				out = append(out, d)
			}
		}
		return out
	}
	var m barcode.Matrix
	add := func(r []bool, h int) {
		m.Rows = append(m.Rows, scale(r))
		m.RowHeights = append(m.RowHeights, h*n)
	}
	add(full, opts.SepHeight)
	for i, r := range rows {
		if i > 0 {
			add(inner, opts.SepHeight)
		}
		add(r, opts.RowHeight)
	}
	add(full, opts.SepHeight)
	return m
}

// ---- Codablock A ----

func encodeA(data []byte, opts Options) (barcode.Matrix, error) {
	vals := make([]int, len(data))
	for i, c := range data {
		k := -1
		for j := 0; j < len(code39.Chars); j++ {
			if code39.Chars[j] == c {
				k = j
				break
			}
		}
		if k < 0 {
			return barcode.Matrix{}, errf("character %q not in the Code 39 set", c)
		}
		vals[i] = k
	}
	n := len(vals)
	if n == 0 {
		return barcode.Matrix{}, errf("no data")
	}
	checks := !opts.NoCheck
	c := opts.Columns
	if c == 0 {
		if opts.Rows > 0 {
			extra := 0
			if checks && opts.Rows > 1 {
				extra = 2
			}
			c = (n + extra + opts.Rows - 1) / opts.Rows
		} else {
			c = n // single row
		}
		// A derived width never exceeds the 62-column maximum; longer data
		// continues on further rows.
		c = min(max(c, 1), 62)
	}
	r := (n + c - 1) / c
	if r > 1 && checks {
		r = (n + 2 + c - 1) / c
	}
	if r == 1 {
		checks = false
	}
	if r > 22 {
		return barcode.Matrix{}, errf("data needs %d rows (max 22)", r)
	}
	space := code39.Index(' ')
	cells := make([]int, r*c)
	copy(cells, vals)
	for i := n; i < len(cells); i++ {
		cells[i] = space
	}
	if checks {
		k1, k2 := 0, 0
		for i := 0; i < r*c-2; i++ {
			k1 = (k1 + cells[i]*(i+1)) % 43
			k2 = (k2 + cells[i]*i) % 43
		}
		cells[r*c-2], cells[r*c-1] = k1, k2
	}
	ratio := opts.Ratio
	if ratio == 0 {
		ratio = 3
	}
	if ratio < 2 || ratio > 3 {
		return barcode.Matrix{}, errf("ratio %.1f out of range 2.0..3.0", ratio)
	}
	// Elements are built at Narrow resolution directly, so assemble with
	// Narrow = 1 and scale heights afterwards.
	nc := opts.Narrow
	wide := int(math.Round(ratio * float64(nc)))
	narrow := nc
	var rows [][]bool
	for i := 0; i < r; i++ {
		ind := r + 20
		if i > 0 {
			ind = i - 1
		}
		chars := []int{code39.StartStop, ind}
		chars = append(chars, cells[i*c:(i+1)*c]...)
		if r > 1 {
			chars = append(chars, ind)
		}
		chars = append(chars, code39.StartStop)
		var row []bool
		for j, ch := range chars {
			if j > 0 {
				row = appendRun(row, false, narrow) // inter-character gap
			}
			for e, p := range code39.Elements(ch) {
				w := narrow
				if p == 2 {
					w = wide
				}
				row = appendRun(row, e%2 == 0, w)
			}
		}
		rows = append(rows, row)
	}
	starW := 3*wide + 6*narrow
	o := opts
	o.Narrow = 1
	o.RowHeight = opts.RowHeight * nc
	o.SepHeight = opts.SepHeight * nc
	return assemble(rows, starW+narrow, starW+narrow, o), nil
}

func appendRun(row []bool, dark bool, n int) []bool {
	for i := 0; i < n; i++ {
		row = append(row, dark)
	}
	return row
}

// ---- Codablock E / F ----

// item is one message element: a 7-bit character (ext = extended ASCII
// high half via FNC4) or a function character (fnc 1..4).
type item struct {
	v    int // 0..127
	ext  bool
	fnc  int // 1..4 for FNC1..FNC4
	orig int // value for K1/K2 (-1: not counted)
}

const (
	setA = iota
	setB
	setC
)

func isDigitItem(it item) bool { return it.fnc == 0 && !it.ext && it.v >= '0' && it.v <= '9' }
func aOnly(it item) bool       { return it.fnc == 0 && it.v < 32 }
func bOnly(it item) bool       { return it.fnc == 0 && it.v >= 96 }

func encodeF(data []byte, opts Options) (barcode.Matrix, error) {
	var msg []item
	if opts.Mode == ModeE {
		msg = append(msg, item{fnc: 1, orig: -1})
	}
	for _, b := range data {
		switch {
		case b < 0x80:
			msg = append(msg, item{v: int(b), orig: int(b)})
		case b >= ZebraFNC1 && b <= ZebraFNC4:
			msg = append(msg, item{fnc: int(b-ZebraFNC1) + 1, orig: -1})
		case b == ZebraNUL:
			msg = append(msg, item{v: 0, orig: 0})
		case opts.Mode == ModeE:
			return barcode.Matrix{}, errf("byte 0x%02X not allowed in Codablock E", b)
		default:
			msg = append(msg, item{v: int(b - 128), ext: true, orig: int(b)})
		}
	}
	// K1/K2 values: data bytes; FNC1 other than in first position counts
	// as GS (29); other function characters are not counted.
	var kvals []int
	for i, it := range msg {
		switch {
		case it.fnc == 1 && i > 0:
			kvals = append(kvals, 29)
		case it.orig >= 0:
			kvals = append(kvals, it.orig)
		}
	}
	c := opts.Columns
	if c == 0 {
		r := opts.Rows
		if r < 2 {
			r = 2
		}
		c = (len(msg) + 2 + r - 1) / r
	}
	// CODABLOCK E/F rows hold at least 4 characters (AIM ISS-X-24); a
	// smaller ^BB c is widened to 4 (MinColumnsF).
	c = min(max(c, MinColumnsF), 62)
	rows, final, err := layoutF(msg, c)
	if err != nil {
		return barcode.Matrix{}, err
	}
	r := len(rows)
	// Symbol check characters in the last two data positions.
	k1, k2 := 0, 0
	for i, v := range kvals {
		k1 = (k1 + v*(i+1)) % 86
		k2 = (k2 + v*i) % 86
	}
	last := rows[r-1]
	last[len(last)-2] = mapValue(k1, final)
	last[len(last)-1] = mapValue(k2, final)
	// Row indicators.
	for i, row := range rows {
		v := i + 42
		if i == 0 {
			v = r - 2
		}
		set := setA
		if row[1] == c128CodeC {
			set = setC
		}
		row[2] = mapValue(v, set)
	}
	var bits [][]bool
	for _, row := range rows {
		sum := row[0]
		for k := 1; k < len(row); k++ {
			sum += k * row[k]
		}
		row = append(row, sum%103, c128Stop)
		var b []bool
		for _, v := range row {
			for e, ch := range code128.Pattern(v) {
				b = appendRun(b, e%2 == 0, int(ch-'0'))
			}
		}
		bits = append(bits, b)
	}
	return assemble(bits, 11, 13, opts), nil
}

// mapValue maps a row indicator / check value 0..85 to a codeword: in code
// set C directly, in A/B skipping the digit codewords 16..25.
func mapValue(v, set int) int {
	if set == setC {
		return v
	}
	switch {
	case v < 32:
		return v + 64
	case v < 48:
		return v - 32
	default:
		return v - 22
	}
}

// layoutF distributes msg over rows of c data codewords. Each returned row
// holds [start A, subset selector, row indicator placeholder, c data
// codewords]; the last row's final two data positions are reserved for
// K1/K2. It also returns the code set in force at the end.
func layoutF(msg []item, c int) ([][]int, int, error) {
	n := len(msg)
	digitRun := func(i int) int {
		k := 0
		for i+k < n && isDigitItem(msg[i+k]) {
			k++
		}
		return k
	}
	aBeforeB := func(i int) bool {
		for ; i < n; i++ {
			if aOnly(msg[i]) {
				return true
			}
			if bOnly(msg[i]) {
				return false
			}
		}
		return false
	}
	bBeforeA := func(i int) bool {
		for ; i < n; i++ {
			if bOnly(msg[i]) {
				return true
			}
			if aOnly(msg[i]) {
				return false
			}
		}
		return false
	}
	var rows [][]int
	i := 0
	set := setB
	for {
		if len(rows) >= 44 {
			return nil, 0, errf("data needs more than 44 rows")
		}
		row := []int{c128StartA}
		switch {
		case i < n && (digitRun(i) >= 2 || (msg[i].fnc == 1 && digitRun(i+1) >= 2)):
			set = setC
			row = append(row, c128CodeC)
		case i < n && aBeforeB(i):
			set = setA
			row = append(row, c128Shift) // selector for set A
		default:
			set = setB
			row = append(row, c128CodeB)
		}
		row = append(row, 0) // row indicator
		rem := c
		emit := func(v ...int) {
			row = append(row, v...)
			rem -= len(v)
		}
		start := i
	row:
		for i < n && rem > 0 {
			it := msg[i]
			switch set {
			case setC:
				switch {
				case digitRun(i) >= 2:
					emit((it.v-'0')*10 + msg[i+1].v - '0')
					i += 2
				case it.fnc == 1:
					emit(c128FNC1)
					i++
				case rem < 2:
					break row
				case aBeforeB(i):
					emit(c128CodeA)
					set = setA
				default:
					emit(c128CodeB)
					set = setB
				}
			default:
				nums := min(digitRun(i), 2*rem)
				if nums >= 4 && it.fnc != 1 {
					if nums%2 == 0 && rem >= 3 {
						emit(c128CodeC)
						set = setC
						continue
					}
					if nums%2 == 1 && rem >= 4 {
						emit(charValue(it, set))
						i++
						emit(c128CodeC)
						set = setC
						continue
					}
				}
				if fits(it, set) {
					if it.ext {
						if rem < 2 {
							break row
						}
						emit(fnc4(set))
					}
					emit(charValue(it, set))
					i++
					continue
				}
				// Character of the other set (A <-> B).
				other := setA
				if set == setA {
					other = setB
				}
				shift := !it.ext && i+1 < n && ((other == setA && bBeforeA(i+1)) || (other == setB && aBeforeB(i+1)))
				need := 2
				if it.ext {
					need = 3
				}
				if rem < need {
					break row
				}
				if shift {
					emit(c128Shift, charValue(it, other))
					i++
					continue
				}
				if other == setA {
					emit(c128CodeA)
				} else {
					emit(c128CodeB)
				}
				set = other
			}
		}
		if i == start && i < n && rem == c {
			return nil, 0, errf("%d columns too narrow", c)
		}
		final := i == n && rem >= 2 && len(rows) >= 1
		pad := rem
		if final {
			pad = rem - 2
		}
		for k := 0; k < pad; k++ {
			if set == setC {
				row = append(row, c128CodeB)
				set = setB
			} else {
				row = append(row, c128CodeC)
				set = setC
			}
		}
		if final {
			row = append(row, 0, 0) // K1, K2
			rows = append(rows, row)
			return rows, set, nil
		}
		rows = append(rows, row)
	}
}

func fits(it item, set int) bool {
	if it.fnc != 0 {
		return true
	}
	if set == setA {
		return it.v < 96
	}
	return it.v >= 32
}

func fnc4(set int) int {
	if set == setA {
		return c128CodeA
	}
	return c128CodeB
}

// charValue returns the Code 128 value of it in code set A or B.
func charValue(it item, set int) int {
	switch it.fnc {
	case 1:
		return c128FNC1
	case 2:
		return c128FNC2
	case 3:
		return c128FNC3
	case 4:
		return fnc4(set)
	}
	if set == setA && it.v < 32 {
		return it.v + 64
	}
	return it.v - 32
}
