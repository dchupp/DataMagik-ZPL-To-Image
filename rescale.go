package zpl

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"strconv"
	"strings"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/bitmap"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/compress"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/font"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/lexer"
)

// Rescale rewrites ZPL written for one print density so it prints the same
// physical size at another. Densities are in dots per millimetre: 6, 8, 12
// or 24. When fromDPMM equals toDPMM, src is returned unchanged.
//
// Every parameter measured in dots is multiplied by toDPMM/fromDPMM and
// rounded to the nearest dot: field origins, label size and shifts, box,
// circle, ellipse and line sizes, text block sizes, font heights and
// widths, barcode heights, module widths and magnifications. ^GF, ~DG and
// ~DY graphics (hex, ACS, :B64:, :Z64:, binary, and PNG downloads) are
// decoded, resampled with nearest-neighbour sampling and re-encoded with
// correct byte counts. Everything else, including field data, comments,
// whitespace, prefix and delimiter changes and the commands it does not
// change, is copied byte for byte. Stored formats (^DF) are rescaled like
// any other ZPL since their body is in the stream.
//
// Some things cannot be scaled exactly; each is reported as a Warning with
// the command's byte offset in src:
//
//   - integer parameters (module widths, magnifications) whose rounded value
//     differs from the exact one by more than 15%, or that had to be clamped
//     to their legal range;
//   - resident bitmap fonts (A–H), which only print at whole multiples of
//     their native size, when the nearest size differs by more than 15%;
//   - sizes that a format leaves to the printer's power-up defaults, which
//     are in dots (^CF A,9,5; ^BY 2,3,10): Rescale assumes the printer is at
//     power-up defaults, and appends the scaled default size to a ^A that
//     has none, but cannot add sizes to fields without ^A or to barcodes;
//   - graphics and formats recalled (^XG, ^IM, ^IL, ^XF) but not defined
//     in src, downloaded bitmap fonts (~DB), BMP/PCX downloads, and
//     AR-compressed (format C) data, which are left as they are;
//   - ^MU: after ^MU I or ^MU M, measurements are in inches or millimetres
//     and are not scaled (module widths and magnifications, which stay in
//     dots, still are); a ^MU dots-per-inch conversion is left in place.
func Rescale(src []byte, fromDPMM, toDPMM int) ([]byte, []Warning, error) {
	for _, d := range []int{fromDPMM, toDPMM} {
		switch d {
		case 6, 8, 12, 24:
		default:
			return nil, nil, fmt.Errorf("zpl: unsupported density %d dots/mm (want 6, 8, 12 or 24)", d)
		}
	}
	if fromDPMM == toDPMM {
		return src, nil, nil
	}
	r := &rescaler{
		src: src, from: fromDPMM, to: toDPMM,
		format: '^', control: '~',
		cfFont: 'A', srcCF: [2]int{9, 5}, dstCF: [2]int{9, 5},
		graphics: map[string]bool{}, formats: map[string]bool{},
	}
	out := r.run()
	if r.overBudget {
		return nil, r.warnings, ErrRescaleTooLarge
	}
	return out, r.warnings, nil
}

// ErrRescaleTooLarge is returned by Rescale when the graphics in the ZPL add
// up to more than RescalePixelBudget pixels to decode and resample.
var ErrRescaleTooLarge = errors.New("zpl: graphics too large to rescale")

// RescalePixelBudget bounds the pixels one Rescale call decodes and writes
// across all its graphics, so a small job of compressed graphics cannot run
// for seconds: it is one label's worth, bitmap.MaxPixels.
const RescalePixelBudget = bitmap.MaxPixels

// spend charges n pixels to the call's budget, reporting whether it holds.
func (r *rescaler) spend(n int) bool {
	if r.overBudget || n < 0 || r.spent+n > RescalePixelBudget {
		r.overBudget = true
		return false
	}
	r.spent += n
	return true
}

// maxRelErr is the relative size error above which a rounded or matched
// size is reported.
const maxRelErr = 0.15

type rescaler struct {
	src      []byte
	from, to int
	warnings []Warning

	physical bool // ^MU I/M in effect: measurements are not in dots

	spent      int  // pixels decoded and resampled so far
	overBudget bool // RescalePixelBudget was exceeded

	// Font and barcode defaults, as set by the source so far.
	cfSized      bool
	cfFont       byte
	srcCF, dstCF [2]int // ^CF height, width before and after scaling
	byModule     bool
	byHeight     bool

	// The current field.
	fieldFont, fieldBarcode bool

	format, control byte // current prefixes, which rewritten text must not contain

	graphics map[string]bool // graphics defined in src (normalised names)
	formats  map[string]bool // formats stored in src with ^DF
}

func (r *rescaler) run() []byte {
	var out bytes.Buffer
	out.Grow(len(r.src))
	prev := 0
	for _, c := range lexer.All(r.src) {
		if r.overBudget {
			return nil // Rescale fails; decoding the rest would only spend more
		}
		out.Write(r.src[prev:c.Offset])
		params := r.command(c)
		if params != nil && (bytes.IndexByte(params, r.format) >= 0 || bytes.IndexByte(params, r.control) >= 0) {
			// ^CC/^CT made a digit (or a letter of the graphic encoding) a
			// prefix: the rewritten parameters would split the command.
			r.warn(c, "not rescaled: the new parameters would contain the command prefix %q or %q", r.format, r.control)
			params = nil
		}
		if params != nil {
			out.Write(r.src[c.Offset:c.ParamStart])
			out.Write(params)
		} else {
			out.Write(r.src[c.Offset:c.End])
		}
		prev = c.End
	}
	out.Write(r.src[prev:])
	return out.Bytes()
}

func (r *rescaler) warn(c lexer.Command, format string, args ...any) {
	r.warnings = append(r.warnings, Warning{Offset: c.Offset, Command: c.Key(), Message: fmt.Sprintf(format, args...)})
}

// scale converts a dot count, rounding half away from zero.
func (r *rescaler) scale(v int) int {
	n, d := int64(v)*int64(r.to), int64(r.from)
	if n >= 0 {
		return int((2*n + d) / (2 * d))
	}
	return -int((-2*n + d) / (2 * d))
}

// Parameter lists for commands whose dot parameters need no special care.
// pos parameters are coordinates or offsets (0 and negative values are
// kept as scaled); size parameters stay at least 1 when they were.
var (
	posParams = map[string][]int{
		"^FO": {0, 1}, "^FT": {0, 1}, "^LH": {0, 1},
		"^FB": {2, 4}, // line spacing (may be negative), hanging indent
		"^FP": {1},    // character gap
		"^PF": {0},    // slew dot rows
		"~TA": {0},    // tear-off adjustment
		"^MN": {1},    // black mark offset
	}
	sizeParams = map[string][]int{
		"^GB": {0, 1, 2}, // width, height, thickness; rounding is an index
		"^GC": {0, 1},
		"^GE": {0, 1, 2},
		"^GD": {0, 1, 2},
		"^FB": {0},
		"^TB": {1, 2},
		"^GS": {1, 2},
		"^ML": {0},
	}
	// limits are the ranges (in dots) the renderer accepts.
	limits = map[string]struct{ idx, lo, hi int }{
		"^PW": {0, 1, 32000}, "^LL": {0, 1, 32000},
		"^LS": {0, -9999, 9999}, "^LT": {0, -240, 240},
	}
	// barHeight is the index of each barcode's height (or row height)
	// parameter, which falls back to the ^BY height when omitted.
	barHeight = map[string]int{
		"^B1": 2, "^B2": 1, "^B3": 2, "^BL": 1, "^BA": 1, "^BC": 1, "^BI": 1,
		"^BJ": 1, "^BK": 2, "^BM": 2, "^BP": 2, "^B8": 1, "^BE": 1, "^BU": 1,
		"^B9": 1, "^BS": 1, "^B5": 1, "^BZ": 1, "^B7": 1, "^BF": 1, "^BB": 1,
	}
	// moduleBarcodes size their bars with the ^BY module width.
	moduleBarcodes = map[string]bool{
		"^B1": true, "^B2": true, "^B3": true, "^B4": true, "^BL": true, "^BA": true,
		"^BC": true, "^BI": true, "^BJ": true, "^BK": true, "^BM": true, "^BP": true,
		"^B8": true, "^BE": true, "^BU": true, "^B9": true, "^BS": true, "^B5": true,
		"^BZ": true, "^B7": true, "^BF": true, "^BB": true,
	}
	// magnification parameters: module sizes in whole dots, legal range.
	magParams = map[string][]struct{ idx, lo, hi int }{
		"^BQ": {{2, 1, 10}},
		"^BO": {{1, 1, 10}}, "^B0": {{1, 1, 10}},
		"^BR": {{2, 1, 10}},
		"^BT": {{1, 1, 10}, {4, 1, 10}},
		"^BX": {{1, 1, 32000}},
		"^BY": {{0, 1, 10}},
	}
)

// command returns the rewritten parameter bytes of c, or nil to copy it.
func (r *rescaler) command(c lexer.Command) []byte {
	key := c.Key()
	switch key {
	case "^GF":
		return r.graphicField(c)
	case "~DG":
		r.graphics[graphicName(paramStr(c, 0))] = true
		return r.downloadGraphic(c)
	case "~DY":
		r.graphics[graphicName(paramStr(c, 0))] = true
		return r.downloadObject(c)
	case "~DB":
		r.warn(c, "downloaded bitmap font is not rescaled: its glyphs are in dots of the source density")
		return nil
	case "^IS":
		r.graphics[graphicName(paramStr(c, 0))] = true
		return nil
	case "^DF":
		r.formats[graphicName(paramStr(c, 0))] = true
		return nil
	case "^XF":
		if !r.formats[graphicName(paramStr(c, 0))] {
			r.warn(c, "format %s is not defined in this ZPL, so it is not rescaled", paramStr(c, 0))
		}
		return nil
	case "^IM", "^IL":
		if !r.graphics[graphicName(paramStr(c, 0))] {
			r.warn(c, "graphic %s is not defined in this ZPL, so it is not rescaled", paramStr(c, 0))
		}
		return nil
	case "^MU":
		r.units(c)
		return nil
	case "^FS":
		r.fieldFont, r.fieldBarcode = false, false
		return nil
	case "^FD", "^FV":
		if !r.fieldFont && !r.fieldBarcode && !r.cfSized && !r.physical {
			r.warn(c, "text uses the power-up default font size (^CF A,9,5, in dots), which cannot be rescaled; give the field a ^A size or set ^CF")
		}
		return nil
	case "^A":
		return r.font(c)
	}
	if c.Name == "CC" || c.Name == "CT" || c.Name == "CD" {
		if len(c.Params) == 1 && c.Name == "CC" {
			r.format = c.Params[0]
		} else if len(c.Params) == 1 && c.Name == "CT" {
			r.control = c.Params[0]
		}
		return nil
	}
	raw := r.src[c.ParamStart:c.End]
	p := splitRaw(raw, c.Delim)
	orig := len(p.f)
	switch key {
	case "^CF":
		r.defaultFont(c, p)
	case "^BY":
		r.byModule = r.byModule || p.has(0)
		r.byHeight = r.byHeight || p.has(2)
		r.sizes(p, 2)
	case "^XG":
		r.recallGraphic(c, p)
	case "^FM":
		for i := range p.f {
			r.positions(p, i)
		}
	case "^RS":
		r.rfidPosition(p)
	}
	r.positions(p, posParams[key]...)
	r.sizes(p, sizeParams[key]...)
	if l, ok := limits[key]; ok && p.has(l.idx) && !r.physical {
		if v, _ := p.int(l.idx); v > 0 || l.lo < 0 {
			s := r.scale(v)
			if s < l.lo || s > l.hi {
				r.warn(c, "%d dots scale to %d, outside %d to %d; clamped", v, s, l.lo, l.hi)
			}
			p.set(l.idx, min(max(s, l.lo), l.hi))
		}
	}
	// Parameters whose default depends on the density: when omitted, write
	// the source density's default (scaled below) unless the target's own
	// default is the same.
	type omitted struct {
		idx int
		def int
	}
	var defaults []omitted
	for _, d := range densityDefaults[key] {
		if !p.has(d.idx) && (d.mag || !r.physical) {
			defaults = append(defaults, omitted{d.idx, d.def(r.to)})
			p.set(d.idx, d.def(r.from))
		}
	}
	for _, m := range magParams[key] {
		r.magnification(c, p, m.idx, m.lo, m.hi)
	}
	if strings.HasPrefix(key, "^B") && key != "^BY" {
		r.barcode(c, p, key)
	}
	for _, d := range defaults {
		if v, _ := p.int(d.idx); v == d.def {
			p.f[d.idx] = nil
		}
	}
	for len(p.f) > 0 && len(p.f[len(p.f)-1]) == 0 && len(p.f) > orig {
		p.f = p.f[:len(p.f)-1]
	}
	if out := p.bytes(); !bytes.Equal(out, raw) {
		return out
	}
	return nil
}

// densityDefaults are barcode parameters whose default, when omitted,
// depends on the density (as the renderer implements them).
var densityDefaults = map[string][]struct {
	idx int
	mag bool
	def func(dpmm int) int
}{
	"^BQ": {{2, true, defaultMag}},
	"^BO": {{1, true, defaultMag}},
	"^B0": {{1, true, defaultMag}},
	"^BR": {{2, true, func(d int) int { return max(map[int]int{12: 3, 24: 6}[d], 1) }}},
	"^BT": {
		{1, true, func(d int) int { return map[bool]int{true: 4, false: 2}[d >= 24] }},
		{3, false, func(d int) int { return max(map[int]int{24: 120, 12: 60}[d], 40) }},
		{4, true, func(d int) int { return map[bool]int{true: 4, false: 2}[d >= 24] }},
		{5, false, func(d int) int { return map[bool]int{true: 8, false: 4}[d >= 24] }},
	},
}

// defaultMagnification is the QR and Aztec default by density.
func defaultMag(dpmm int) int {
	return map[int]int{6: 1, 8: 2, 12: 3, 24: 6}[dpmm]
}

func paramStr(c lexer.Command, i int) string {
	ps := c.Split()
	if i < len(ps) {
		return strings.TrimSpace(ps[i])
	}
	return ""
}

// graphicName normalises a stored object name: no drive, no extension.
func graphicName(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	if i := strings.IndexByte(s, ':'); i >= 0 {
		s = s[i+1:]
	}
	if i := strings.IndexByte(s, '.'); i >= 0 {
		s = s[:i]
	}
	return s
}

func (r *rescaler) positions(p *rawParams, idx ...int) {
	if r.physical {
		return
	}
	for _, i := range idx {
		if v, ok := p.int(i); ok {
			p.set(i, r.scale(v))
		}
	}
}

func (r *rescaler) sizes(p *rawParams, idx ...int) {
	if r.physical {
		return
	}
	for _, i := range idx {
		if v, ok := p.int(i); ok {
			s := r.scale(v)
			if v > 0 {
				s = max(s, 1)
			}
			p.set(i, s)
		}
	}
}

// magnification scales a whole-dot module size within [lo, hi], reporting
// a large rounding error or a clamp.
func (r *rescaler) magnification(c lexer.Command, p *rawParams, i, lo, hi int) {
	v, ok := p.int(i)
	if !ok || v <= 0 {
		return
	}
	exact := float64(v) * float64(r.to) / float64(r.from)
	s := min(max(r.scale(v), lo), hi)
	p.set(i, s)
	switch err := float64(s)/exact - 1; {
	case r.scale(v) > hi || r.scale(v) < lo:
		r.warn(c, "module size %d scales to %.2f dots, outside %d to %d; clamped to %d, so the symbol is %+.0f%% the size", v, exact, lo, hi, s, 100*err)
	case math.Abs(err) > maxRelErr:
		r.warn(c, "module size %d scales to %.2f dots; whole dots only, so %d makes the symbol %+.0f%% the size", v, exact, s, 100*err)
	}
}

// barcode scales a barcode's height and reports sizes left to defaults.
func (r *rescaler) barcode(c lexer.Command, p *rawParams, key string) {
	r.fieldBarcode = true
	if i, ok := barHeight[key]; ok {
		if p.has(i) {
			r.sizes(p, i)
			if key == "^BB" {
				if v, ok := p.int(i); ok && v > 0 && v < 2 {
					p.set(i, 2)
				}
			}
		} else if !r.byHeight {
			r.warn(c, "barcode uses the power-up ^BY height (10 dots), which cannot be rescaled; set a height")
		}
	}
	if key == "^BX" && !p.has(1) && !r.byHeight {
		r.warn(c, "Data Matrix size comes from the power-up ^BY height (10 dots), which cannot be rescaled; set a module size")
	}
	if key == "^BT" {
		r.sizes(p, 3)
		if v, ok := p.int(5); ok && v > 0 && !r.physical {
			s := r.scale(v)
			if s > 255 {
				r.warn(c, "MicroPDF417 row height %d scales to %d; clamped to 255", v, s)
			}
			p.set(5, min(max(s, 1), 255))
		}
	}
	if moduleBarcodes[key] && !r.byModule {
		r.warn(c, "barcode uses the power-up ^BY module width (2 dots), which cannot be rescaled; set ^BY")
	}
}

// units handles ^MU: inches and millimetres are physical and must not be
// scaled; a dots-per-inch conversion is applied by the printer on top.
func (r *rescaler) units(c lexer.Command) {
	ps := c.Split()
	if len(ps) > 0 {
		switch u := strings.ToUpper(strings.TrimSpace(ps[0])); {
		case strings.HasPrefix(u, "I"), strings.HasPrefix(u, "M"):
			if !r.physical {
				r.warn(c, "measurements after ^MU %s are physical units and are not rescaled", u[:1])
			}
			r.physical = true
		case strings.HasPrefix(u, "D"):
			r.physical = false
		}
	}
	if len(ps) > 1 && (strings.TrimSpace(ps[1]) != "" || len(ps) > 2 && strings.TrimSpace(ps[2]) != "") {
		r.warn(c, "^MU dots-per-inch conversion is left in place: the printer applies it on top of the rescaled values")
	}
}

// font handles ^A: parameters after the font name are orientation, height,
// width (and for ^A@ the font file).
func (r *rescaler) font(c lexer.Command) []byte {
	raw := r.src[c.ParamStart:c.End]
	i := 0
	for i < len(raw) && (raw[i] == '\r' || raw[i] == '\n') {
		i++
	}
	if i >= len(raw) {
		return nil
	}
	name := upper(raw[i])
	p := splitRaw(raw[i+1:], c.Delim)
	r.fieldFont = true
	if r.physical {
		return nil
	}
	h, _ := p.int(1)
	w, _ := p.int(2)
	src, dst := [2]int{h, w}, [2]int{r.scale(h), r.scale(w)}
	switch {
	case h <= 0 && w <= 0 && !r.cfSized && !p.has(1) && !p.has(2):
		// No size and no ^CF: the power-up 9×5-dot default. Make the
		// scaled default explicit so the field keeps its physical size.
		src, dst = [2]int{9, 5}, [2]int{r.scale(9), r.scale(5)}
		p.set(1, dst[0])
		p.set(2, dst[1])
	case h <= 0 && w <= 0:
		src, dst = r.srcCF, r.dstCF
		r.sizes(p, 1, 2)
	default:
		r.sizes(p, 1, 2)
	}
	r.checkBitmap(c, name, src, dst)
	if !p.changed {
		return nil
	}
	return append(bytes.Clone(raw[:i+1]), p.bytes()...)
}

func upper(b byte) byte {
	if b >= 'a' && b <= 'z' {
		return b - 'a' + 'A'
	}
	return b
}

func (r *rescaler) defaultFont(c lexer.Command, p *rawParams) {
	if f := strings.TrimSpace(p.str(0)); f != "" {
		r.cfFont = upper(f[0])
	}
	if !p.has(1) && !p.has(2) {
		return
	}
	h, _ := p.int(1)
	w, _ := p.int(2)
	if !p.has(1) {
		h = 0
	}
	if !p.has(2) {
		w = 0
	}
	r.cfSized = true
	if r.physical {
		r.srcCF, r.dstCF = [2]int{h, w}, [2]int{h, w}
		return
	}
	r.srcCF, r.dstCF = [2]int{h, w}, [2]int{r.scale(h), r.scale(w)}
	r.sizes(p, 1, 2)
	r.checkBitmap(c, r.cfFont, r.srcCF, r.dstCF)
}

// checkBitmap reports a resident bitmap font whose nearest magnification at
// the new density is not close to the original physical size.
func (r *rescaler) checkBitmap(c lexer.Command, name byte, src, dst [2]int) {
	if !font.IsBitmap(name) {
		return
	}
	sh, sw := bitmapCell(name, r.from, src[0], src[1])
	dh, dw := bitmapCell(name, r.to, dst[0], dst[1])
	eh := float64(dh)*float64(r.from)/(float64(sh)*float64(r.to)) - 1
	ew := float64(dw)*float64(r.from)/(float64(sw)*float64(r.to)) - 1
	if math.Abs(eh) > maxRelErr || math.Abs(ew) > maxRelErr {
		r.warn(c, "bitmap font %c prints %.2f×%.2f mm at %d dots/mm but %.2f×%.2f mm at %d (whole magnifications only); a scalable font such as ^A0 rescales exactly",
			name, float64(sh)/float64(r.from), float64(sw)/float64(r.from), r.from,
			float64(dh)/float64(r.to), float64(dw)/float64(r.to), r.to)
	}
}

// bitmapCell is the printed character cell of a bitmap font, as the
// renderer magnifies it.
func bitmapCell(name byte, dpmm, h, w int) (int, int) {
	s := font.BitmapFont(name, dpmm)
	mh := font.Magnification(h, s.H)
	mw := mh
	if w > 0 {
		mw = font.Magnification(w, s.W)
	}
	if h <= 0 && w > 0 {
		mh = mw
	}
	return mh * s.H, mw * s.W
}

// recallGraphic handles ^XG. A graphic downloaded in src is rescaled where
// it is defined, so its magnification stays. A graphic stored on the
// printer is assumed to be at the source density: its magnification is
// scaled when the density ratio is a whole number.
func (r *rescaler) recallGraphic(c lexer.Command, p *rawParams) {
	name := p.str(0)
	if r.graphics[graphicName(name)] {
		return
	}
	if r.to%r.from != 0 {
		r.warn(c, "graphic %s is not defined in this ZPL and %d/%d is not a whole magnification; not rescaled", strings.TrimSpace(name), r.to, r.from)
		return
	}
	k := r.to / r.from
	for _, i := range []int{1, 2} {
		m := 1
		if v, ok := p.int(i); ok && v > 0 {
			m = v
		}
		if m*k > 10 {
			r.warn(c, "graphic %s magnification %d×%d exceeds 10; clamped", strings.TrimSpace(name), m, k)
		}
		p.set(i, min(m*k, 10))
	}
	r.warn(c, "graphic %s is not defined in this ZPL; assuming it is stored at %d dots/mm, its magnification was multiplied by %d", strings.TrimSpace(name), r.from, k)
}

// rfidPosition scales the ^RS read/write position when it is in dot rows
// (a number or F + number); B + number is millimetres from the bottom.
func (r *rescaler) rfidPosition(p *rawParams) {
	if r.physical || len(p.f) < 2 {
		return
	}
	s := strings.TrimSpace(p.str(1))
	prefix := ""
	if s != "" && (s[0] == 'F' || s[0] == 'f') {
		prefix, s = s[:1], s[1:]
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return
	}
	p.f[1] = []byte(prefix + strconv.Itoa(r.scale(v)))
	p.changed = true
}

// Graphics.

// resample scales a packed 1-bit image of rowBytes bytes per row with
// nearest-neighbour sampling. It returns nil when the result would be too
// large to download.
func (r *rescaler) resample(raw []byte, rowBytes int) ([]byte, int) {
	h := len(raw) / rowBytes
	w := rowBytes * 8
	nw, nh := max(r.scale(w), 1), max(r.scale(h), 1)
	nrow := (nw + 7) / 8
	if !bitmap.Fits(nw, nh) || !r.spend(w*h+nw*nh) {
		return nil, 0
	}
	out := make([]byte, nrow*nh)
	xs := make([]int, nw)
	for x := range xs {
		xs[x] = min((2*x+1)*r.from/(2*r.to), w-1)
	}
	for y := range nh {
		sy := min((2*y+1)*r.from/(2*r.to), h-1)
		srow := raw[sy*rowBytes : (sy+1)*rowBytes]
		drow := out[y*nrow : (y+1)*nrow]
		for x, sx := range xs {
			if srow[sx>>3]&(0x80>>(sx&7)) != 0 {
				drow[x>>3] |= 0x80 >> (x & 7)
			}
		}
	}
	return out, nrow
}

// grfFits reports whether a downloaded bitmap of row bytes per row and
// total bytes is one the renderer would draw, so it is worth rescaling.
func (r *rescaler) grfFits(c lexer.Command, row, total int) bool {
	if row <= 0 {
		return false
	}
	if row > compress.MaxBytes || total > compress.MaxBytes || !bitmap.Fits(row*8, total/row) {
		r.warn(c, "graphic too large to rescale")
		return false
	}
	return true
}

// zplHexOrZ64 encodes rescaled data: plain hex when the
// source was plain hex, compressed :Z64: otherwise (ACS, base64, binary).
func zplHexOrZ64(data string, binary bool, raw []byte) string {
	d := strings.TrimSpace(data)
	plain := !binary && !strings.HasPrefix(d, ":")
	for i := 0; plain && i < len(d); i++ {
		c := d[i]
		plain = c >= '0' && c <= '9' || c >= 'A' && c <= 'F' || c >= 'a' && c <= 'f' || c == ' '
	}
	if plain {
		return strings.ToUpper(hex.EncodeToString(raw))
	}
	var z bytes.Buffer
	zw, _ := zlib.NewWriterLevel(&z, zlib.BestCompression)
	zw.Write(raw)
	zw.Close()
	return ":Z64:" + withCRC(base64.StdEncoding.EncodeToString(z.Bytes()))
}

func withCRC(body string) string {
	return fmt.Sprintf("%s:%04x", body, compress.CRC16(body))
}

// trailingSpace returns the CR/LF and spaces ending b.
func trailingSpace(b []byte) []byte {
	i := len(b)
	for i > 0 && (b[i-1] == '\r' || b[i-1] == '\n' || b[i-1] == ' ') {
		i--
	}
	return b[i:]
}

// decodeGRF decodes a ^GF/~DG/~DY bitmap payload.
func decodeGRF(kind byte, data string, row, total int) ([]byte, error) {
	switch kind {
	case 'B':
		raw := []byte(data)
		if total > 0 && len(raw) > total {
			raw = raw[:total]
		}
		return raw, nil
	case 'C':
		return nil, fmt.Errorf("AR-compressed (format C) data is proprietary and was not rescaled")
	}
	raw, err := compress.DecodeGraphic(data, row, total)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

// graphicField handles ^GFa,b,c,d,data.
func (r *rescaler) graphicField(c lexer.Command) []byte {
	parts := strings.SplitN(string(c.Params), string(c.Delim), 5)
	if len(parts) < 5 {
		return nil
	}
	ps := params(parts)
	kind := upper(ps.char(0, 'A'))
	total, row := ps.int(2), ps.int(3)
	if !r.grfFits(c, row, total) {
		return nil
	}
	raw, err := decodeGRF(kind, parts[4], row, total)
	if err != nil {
		r.warn(c, "graphic not rescaled: %v", err)
		return nil
	}
	if len(raw) < row {
		return nil
	}
	out, nrow := r.resample(raw[:len(raw)/row*row], row)
	if out == nil {
		r.warn(c, "graphic too large to rescale")
		return nil
	}
	d := string(c.Delim)
	n := strconv.Itoa(len(out))
	s := "A" + d + n + d + n + d + strconv.Itoa(nrow) + d + zplHexOrZ64(parts[4], kind == 'B', out)
	if kind == 'A' {
		s += string(trailingSpace(r.src[c.ParamStart:c.End]))
	}
	return []byte(s)
}

// downloadGraphic handles ~DGd:o.x,t,w,data.
func (r *rescaler) downloadGraphic(c lexer.Command) []byte {
	parts := strings.SplitN(string(c.Params), string(c.Delim), 4)
	if len(parts) < 4 {
		return nil
	}
	ps := params(parts)
	total, row := ps.int(1), ps.int(2)
	if !r.grfFits(c, row, total) {
		return nil
	}
	raw, err := compress.DecodeGraphic(parts[3], row, total)
	if err != nil || len(raw) < row {
		r.warn(c, "graphic not rescaled: %v", err)
		return nil
	}
	out, nrow := r.resample(raw[:len(raw)/row*row], row)
	if out == nil {
		r.warn(c, "graphic too large to rescale")
		return nil
	}
	d := string(c.Delim)
	s := parts[0] + d + strconv.Itoa(len(out)) + d + strconv.Itoa(nrow) + d + zplHexOrZ64(parts[3], false, out)
	return append([]byte(s), trailingSpace(r.src[c.ParamStart:c.End])...)
}

// downloadObject handles ~DYd:f,b,x,t,w,data: GRF bitmaps and PNG images
// are rescaled; fonts and other objects are not images and stay.
func (r *rescaler) downloadObject(c lexer.Command) []byte {
	parts := strings.SplitN(string(c.Params), string(c.Delim), 6)
	if len(parts) < 6 {
		return nil
	}
	ps := params(parts)
	kind := upper(ps.char(1, 'A'))
	ext := strings.ToUpper(strings.TrimSpace(parts[2]))
	total, row := ps.int(3), ps.int(4)
	data := parts[5]
	d := string(c.Delim)
	tail := []byte(nil)
	if kind != 'B' && kind != 'C' {
		tail = trailingSpace(r.src[c.ParamStart:c.End])
	}
	switch ext {
	case "G", "GRF", "":
		if !r.grfFits(c, row, total) {
			return nil
		}
		var raw []byte
		var err error
		if kind == 'B' || kind == 'C' {
			raw, err = decodeGRF(kind, data, row, total)
		} else {
			raw, err = compress.DecodeGraphic(strings.TrimSpace(data), row, total)
		}
		if err != nil || len(raw) < row {
			r.warn(c, "graphic not rescaled: %v", err)
			return nil
		}
		out, nrow := r.resample(raw[:len(raw)/row*row], row)
		if out == nil {
			r.warn(c, "graphic too large to rescale")
			return nil
		}
		s := parts[0] + d + "A" + d + parts[2] + d + strconv.Itoa(len(out)) + d + strconv.Itoa(nrow) + d +
			zplHexOrZ64(data, kind == 'B', out)
		return append([]byte(s), tail...)
	case "P", "PNG":
		var raw []byte
		var err error
		switch kind {
		case 'B':
			raw = []byte(data)
		case 'C':
			r.warn(c, "AR-compressed (format C) image was not rescaled")
			return nil
		default:
			t := strings.TrimSpace(data)
			if strings.HasPrefix(t, ":B64:") || strings.HasPrefix(t, ":Z64:") {
				raw, err = compress.DecodeGraphic(t, 0, 0)
			} else {
				raw = compress.DecodeHex(t)
			}
		}
		img, derr := bitmap.DecodePNG(raw)
		if err != nil || derr != nil {
			r.warn(c, "PNG not rescaled: %v", firstErr(err, derr))
			return nil
		}
		out, ok := r.resampleImage(img)
		if !ok {
			r.warn(c, "PNG too large to rescale")
			return nil
		}
		f := "A"
		if kind == 'P' {
			f = "P"
		}
		s := parts[0] + d + f + d + parts[2] + d + strconv.Itoa(len(out)) + d + parts[4] + d +
			":B64:" + withCRC(base64.StdEncoding.EncodeToString(out))
		return append([]byte(s), tail...)
	case "B", "BMP", "X", "PCX":
		r.warn(c, "%s image was not rescaled; download it as PNG or GRF to rescale it", ext)
	}
	return nil
}

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

// resampleImage scales an image with nearest-neighbour sampling to an
// 8-bit grayscale PNG.
func (r *rescaler) resampleImage(img image.Image) ([]byte, bool) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	nw, nh := max(r.scale(w), 1), max(r.scale(h), 1)
	if !bitmap.Fits(nw, nh) || w == 0 || h == 0 || !r.spend(w*h+nw*nh) {
		return nil, false
	}
	out := image.NewGray(image.Rect(0, 0, nw, nh))
	for y := range nh {
		sy := b.Min.Y + min((2*y+1)*r.from/(2*r.to), h-1)
		for x := range nw {
			sx := b.Min.X + min((2*x+1)*r.from/(2*r.to), w-1)
			out.SetGray(x, y, color.GrayModel.Convert(img.At(sx, sy)).(color.Gray))
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return nil, false
	}
	return buf.Bytes(), true
}

// params reads header parameters of payload commands.
type params []string

func (ps params) char(i int, def byte) byte {
	if i < len(ps) {
		if s := strings.TrimSpace(ps[i]); s != "" {
			return s[0]
		}
	}
	return def
}

func (ps params) int(i int) int {
	if i < len(ps) {
		v, _ := leadingInt(strings.TrimSpace(ps[i]))
		return v
	}
	return 0
}

// leadingInt parses a leading integer and ignores trailing junk, as the
// renderer does.
func leadingInt(s string) (int, bool) {
	end := 0
	if end < len(s) && (s[end] == '-' || s[end] == '+') {
		end++
	}
	digits := end
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	if end == digits {
		return 0, false
	}
	v, err := strconv.Atoi(s[:end])
	return v, err == nil
}

// rawParams are a command's parameters as written in the source (CR/LF
// included), so that untouched parameters are copied exactly.
type rawParams struct {
	f       [][]byte
	delim   byte
	changed bool
}

func splitRaw(b []byte, delim byte) *rawParams {
	p := &rawParams{delim: delim}
	if len(b) > 0 {
		p.f = bytes.Split(b, []byte{delim})
	}
	return p
}

// str returns parameter i without CR/LF.
func (p *rawParams) str(i int) string {
	if i >= len(p.f) {
		return ""
	}
	return strings.NewReplacer("\r", "", "\n", "").Replace(string(p.f[i]))
}

func (p *rawParams) has(i int) bool { return strings.TrimSpace(p.str(i)) != "" }

func (p *rawParams) int(i int) (int, bool) { return leadingInt(strings.TrimSpace(p.str(i))) }

// set replaces the number in parameter i (keeping surrounding spaces and
// trailing text) or fills an empty or missing parameter.
func (p *rawParams) set(i, v int) {
	for len(p.f) <= i {
		p.f = append(p.f, nil)
	}
	f := p.f[i]
	num := []byte(strconv.Itoa(v))
	a := 0
	for a < len(f) && (f[a] == ' ' || f[a] == '\r' || f[a] == '\n') {
		a++
	}
	if a == len(f) {
		p.f[i] = append(bytes.Clone(f[:a]), num...)
		p.changed = true
		return
	}
	b := a
	if f[b] == '-' || f[b] == '+' {
		b++
	}
	for b < len(f) && (f[b] >= '0' && f[b] <= '9' || f[b] == '\r' || f[b] == '\n') {
		b++
	}
	for b > a && (f[b-1] == '\r' || f[b-1] == '\n') {
		b--
	}
	if b == a || b == a+1 && (f[a] == '-' || f[a] == '+') {
		return // not a number: leave it
	}
	if old := string(f[a:b]); old == string(num) {
		return
	}
	nf := append(append(bytes.Clone(f[:a]), num...), f[b:]...)
	p.f[i] = nf
	p.changed = true
}

func (p *rawParams) bytes() []byte { return bytes.Join(p.f, []byte{p.delim}) }
