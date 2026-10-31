// Package aztec encodes Aztec Code symbols (ISO/IEC 24778): compact (1-4
// layers) and full-range (1-32 layers) symbols, reader initialisation (menu)
// symbols, structured append, ECI, and Aztec Runes. It implements the
// symbol-level behaviour of the ZPL ^B0 / ^BO command; magnification and
// orientation are applied by the renderer.
package aztec

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/datamagik/zpl-to-image/internal/barcode"
)

// DefaultECPercent is the error correction level used when none is given:
// ISO/IEC 24778's recommended 23% of symbol capacity (plus 3 codewords).
const DefaultECPercent = 23

// Options controls symbol generation.
type Options struct {
	// ECI enables ECI escapes in the data (^B0 parameter c = Y): a backslash
	// followed by exactly six digits ("\000026") emits an ECI designator
	// (FLG(n)); "\\" encodes one literal backslash. Any other backslash is
	// taken literally.
	ECI bool

	// ECPercent is the minimum error correction as a percentage of the
	// symbol capacity (1-99); 3 extra check codewords are always added in
	// automatic sizing. 0 means DefaultECPercent. Ignored when Layers > 0.
	ECPercent int

	// Layers, when > 0, forces the symbol size: 1-4 for compact symbols
	// (Compact true) or 1-32 for full-range symbols. All codewords not used
	// for data become check codewords.
	Layers  int
	Compact bool

	// Menu marks the symbol as a reader initialisation ("menu") symbol.
	// Such symbols must be compact 1-layer or full-range with <= 22 layers.
	Menu bool

	// SACount is the total number of symbols in a structured append
	// sequence (2-26); 0 or 1 disables structured append. SAIndex is this
	// symbol's 1-based position. SAID is the optional message ID (no
	// spaces, at most 24 characters).
	SACount int
	SAIndex int
	SAID    string
}

// ZPLParams holds the ^B0 parameters c..g as written in ZPL.
type ZPLParams struct {
	ECIC    bool   // c: data contains ECI escapes
	Size    int    // d: 0, 1-99 EC percent, 101-104 compact, 201-232 full, 300 rune
	Menu    bool   // e: reader initialisation symbol
	Symbols int    // f: structured append symbol count, 1-26 (1 = none)
	ID      string // g: structured append ID, up to 24 characters
}

// Options converts the ZPL parameters into encoder options. Size 300 (Rune)
// cannot be expressed as Options; use EncodeZPL or EncodeRune.
func (p ZPLParams) Options() (Options, error) {
	o := Options{ECI: p.ECIC, Menu: p.Menu, SACount: p.Symbols, SAID: p.ID}
	switch s := p.Size; {
	case s == 0:
	case s >= 1 && s <= 99:
		o.ECPercent = s
	case s >= 101 && s <= 104:
		o.Compact, o.Layers = true, s-100
	case s >= 201 && s <= 232:
		o.Layers = s - 200
	default:
		return o, fmt.Errorf("%w: aztec: bad size/error control value %d", barcode.ErrInvalidData, s)
	}
	if o.SACount > 1 {
		o.SAIndex = 1
	}
	return o, nil
}

// EncodeZPL encodes a ^B0 field. With Size 300 the field data must be a
// decimal value 0-255 and an Aztec Rune is produced. When Symbols > 1 this
// returns the first symbol of the sequence; use EncodeStructured for all.
func EncodeZPL(data []byte, p ZPLParams) (barcode.Matrix, error) {
	if p.Size == 300 {
		v, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			return barcode.Matrix{}, fmt.Errorf("%w: aztec rune: data %q is not a number 0-255", barcode.ErrInvalidData, data)
		}
		return EncodeRune(v)
	}
	o, err := p.Options()
	if err != nil {
		return barcode.Matrix{}, err
	}
	return Encode(data, o)
}

// Encode encodes data (raw bytes, ISO 8859-1 by default) as one Aztec
// symbol. The result has no quiet zone (Aztec needs none).
func Encode(data []byte, o Options) (barcode.Matrix, error) {
	items, err := parseItems(data, o.ECI)
	if err != nil {
		return barcode.Matrix{}, err
	}
	return encodeItems(items, o)
}

// EncodeStructured splits data into o.SACount symbols of nearly equal
// content length and encodes each with its structured append header. With
// SACount <= 1 it returns a single symbol.
func EncodeStructured(data []byte, o Options) ([]barcode.Matrix, error) {
	items, err := parseItems(data, o.ECI)
	if err != nil {
		return nil, err
	}
	n := o.SACount
	if n <= 1 {
		m, err := encodeItems(items, o)
		if err != nil {
			return nil, err
		}
		return []barcode.Matrix{m}, nil
	}
	if n > 26 || n > len(items) {
		return nil, fmt.Errorf("%w: aztec: cannot split %d items into %d symbols", barcode.ErrInvalidData, len(items), n)
	}
	var out []barcode.Matrix
	for i := 0; i < n; i++ {
		part := items[i*len(items)/n : (i+1)*len(items)/n]
		oo := o
		oo.SAIndex = i + 1
		m, err := encodeItems(part, oo)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

// EncodeRune returns the 11x11 Aztec Rune for value 0-255.
func EncodeRune(v int) (barcode.Matrix, error) {
	if v < 0 || v > 255 {
		return barcode.Matrix{}, fmt.Errorf("%w: aztec rune value %d out of range 0-255", barcode.ErrInvalidData, v)
	}
	g := newGrid(11)
	g.drawModeMessage(true, runeBits(v))
	g.drawBullsEye(5, 5)
	return barcode.Matrix{Rows: g.rows}, nil
}

func parseItems(data []byte, eci bool) ([]item, error) {
	items := make([]item, 0, len(data))
	for i := 0; i < len(data); i++ {
		c := data[i]
		if eci && c == '\\' {
			if i+1 < len(data) && data[i+1] == '\\' {
				items = append(items, item{b: '\\'})
				i++
				continue
			}
			if i+6 < len(data) && allDigits(data[i+1:i+7]) {
				d := strings.TrimLeft(string(data[i+1:i+7]), "0")
				if d == "" {
					d = "0"
				}
				items = append(items, item{flag: true, digits: d})
				i += 6
				continue
			}
		}
		items = append(items, item{b: c})
	}
	return items, nil
}

func allDigits(b []byte) bool {
	for _, c := range b {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

type candidate struct {
	compact bool
	layers  int
}

// candidates lists symbol sizes in increasing order of side length,
// compact before full-range at equal size.
var candidates = func() []candidate {
	c := []candidate{{true, 1}, {true, 2}, {false, 1}, {true, 3}, {false, 2}, {true, 4}, {false, 3}}
	for l := 4; l <= 32; l++ {
		c = append(c, candidate{false, l})
	}
	return c
}()

func encodeItems(items []item, o Options) (barcode.Matrix, error) {
	var pre []bool
	if o.SACount > 1 {
		if o.SACount > 26 || o.SAIndex < 1 || o.SAIndex > o.SACount {
			return barcode.Matrix{}, fmt.Errorf("%w: aztec: structured append %d of %d", barcode.ErrInvalidData, o.SAIndex, o.SACount)
		}
		if len(o.SAID) > 24 || strings.ContainsRune(o.SAID, ' ') {
			return barcode.Matrix{}, fmt.Errorf("%w: aztec: structured append ID %q", barcode.ErrInvalidData, o.SAID)
		}
		// ISO/IEC 24778 section 8: M/L U/L, optional " ID ", then the
		// position and count as letters A-Z.
		pre = appendBits(appendBits(nil, 29, 5), 29, 5)
		var hdr []item
		if o.SAID != "" {
			hdr = append(hdr, item{b: ' '})
			for i := 0; i < len(o.SAID); i++ {
				hdr = append(hdr, item{b: o.SAID[i]})
			}
			hdr = append(hdr, item{b: ' '})
		}
		hdr = append(hdr, item{b: byte('A' + o.SAIndex - 1)}, item{b: byte('A' + o.SACount - 1)})
		items = append(hdr, items...)
	}
	if len(items) == 0 {
		return barcode.Matrix{}, fmt.Errorf("%w: aztec: empty data", barcode.ErrInvalidData)
	}
	bits := append(pre, highLevelEncode(items)...)

	if o.Layers > 0 {
		max := 32
		if o.Compact {
			max = 4
		}
		if o.Layers > max {
			return barcode.Matrix{}, fmt.Errorf("%w: aztec: %d layers out of range", barcode.ErrInvalidData, o.Layers)
		}
		m, ok, why := tryBuild(bits, o.Compact, o.Layers, o.Menu, -1)
		if !ok {
			return barcode.Matrix{}, fmt.Errorf("%w: aztec: %s", barcode.ErrInvalidData, why)
		}
		return m, nil
	}
	pct := o.ECPercent
	if pct == 0 {
		pct = DefaultECPercent
	}
	if pct < 1 || pct > 99 {
		return barcode.Matrix{}, fmt.Errorf("%w: aztec: error correction %d%%", barcode.ErrInvalidData, pct)
	}
	for _, c := range candidates {
		if m, ok, _ := tryBuild(bits, c.compact, c.layers, o.Menu, pct); ok {
			return m, nil
		}
	}
	return barcode.Matrix{}, fmt.Errorf("%w: aztec: data too long (%d bits)", barcode.ErrInvalidData, len(bits))
}

// tryBuild builds the symbol if the data fits. pct < 0 means fixed size:
// any number (>= 1) of check codewords is acceptable.
func tryBuild(bits []bool, compact bool, layers int, menu bool, pct int) (barcode.Matrix, bool, string) {
	if menu && ((compact && layers > 1) || (!compact && layers > 22)) {
		return barcode.Matrix{}, false, "menu symbols must be compact 1-layer or full-range up to 22 layers"
	}
	ws := wordSize(layers)
	tb := totalBits(compact, layers)
	totalWords := tb / ws
	words := stuffBits(bits, ws)
	maxData := 2048
	if compact {
		maxData = 64
	}
	if menu {
		maxData /= 2
	}
	minCheck := 1
	if pct > 0 {
		minCheck = (totalWords*pct+99)/100 + 3
	}
	if len(words) > maxData || len(words)+minCheck > totalWords {
		return barcode.Matrix{}, false, fmt.Sprintf("data (%d codewords) does not fit %d-layer symbol", len(words), layers)
	}
	check := rsEncode(ws, words, totalWords-len(words))
	msg := make([]bool, tb%ws, tb)
	for _, w := range append(words, check...) {
		msg = appendBits(msg, w, ws)
	}
	rows := build(compact, layers, msg, modeMessage(compact, layers, len(words), menu))
	return barcode.Matrix{Rows: rows}, true, ""
}
