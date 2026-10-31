package engine

import (
	"bytes"
	"fmt"
	"image"
	"math"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/aztec"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/datamatrix"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/maxicode"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/qr"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/bitmap"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/font"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/lexer"
)

// BarcodeCommands lists every barcode command.
var BarcodeCommands = []string{
	"^B0", "^B1", "^B2", "^B3", "^B4", "^B5", "^B7", "^B8", "^B9", "^BA", "^BB", "^BC",
	"^BD", "^BE", "^BF", "^BI", "^BJ", "^BK", "^BL", "^BM", "^BO", "^BP", "^BQ", "^BR",
	"^BS", "^BT", "^BU", "^BX", "^BZ",
}

// barcodeSpec is a barcode command waiting for its field data.
type barcodeSpec struct {
	cmd    string
	ps     params
	offset int // byte offset of the command, for warnings
}

func cmdBarcode(f *format, c lexer.Command, ps params) error {
	f.fld.barcode = &barcodeSpec{cmd: c.Key(), ps: ps, offset: c.Offset}
	return nil
}

// drawBarcode encodes and places a barcode field.
func (f *format) drawBarcode(fld *field, st fieldState, data []byte) {
	b := fld.barcode
	rot := st.fw.rot
	if r, ok := parseRotation(b.ps.char(0, 0)); ok {
		rot = r
	}
	var s shape
	var err error
	switch b.cmd {
	case "^BQ":
		rot = bitmap.R0 // ^FW has no effect on QR rotation
		s, err = f.qrShape(b.ps, data)
	case "^BX":
		s, err = f.dataMatrixShape(b, st, data)
	case "^B0", "^BO":
		s, err = f.aztecShape(b.ps, data)
	case "^BD":
		rot = st.fw.rot
		s, err = f.maxiShape(b.ps, data)
	default:
		var multi []shape
		multi, err = f.otherBarcode(fld, st, b, data)
		if err == nil {
			f.placeMulti(fld, st, multi, rot)
			return
		}
	}
	if err != nil {
		f.barcodeError(fld, st, b, err)
		return
	}
	f.place(fld, st, s, rot)
}

// placeMulti places one or more symbols; extra symbols go to ^FM origins.
func (f *format) placeMulti(fld *field, st fieldState, shapes []shape, rot bitmap.Rotation) {
	for i, s := range shapes {
		c := *fld
		if len(fld.fm) > 0 {
			if i >= len(fld.fm) {
				break
			}
			if fld.fmSkip[i] {
				continue
			}
			c.originSet, c.x, c.y = true, fld.fm[i].X, fld.fm[i].Y
		} else if i > 0 {
			break
		}
		f.place(&c, st, s, rot)
	}
}

func (f *format) barcodeError(fld *field, st fieldState, b *barcodeSpec, err error) {
	cmd := b.cmd
	f.pr.warnings = append(f.pr.warnings, Warning{Offset: b.offset, Command: cmd, Message: err.Error()})
	if !f.pr.p.validate {
		return
	}
	c := *fld
	c.barcode, c.font = nil, nil
	f.drawLine(&c, st, font.BitmapFont('A', f.dpmm()).Face(0, 0), st.fw.rot, []rune("INVALID - "+cmd[1:]))
}

// defaultMagnification is the 2D magnification default by density.
func (f *format) defaultMagnification() int {
	switch f.pr.opt.DPMM {
	case 6:
		return 1
	case 12:
		return 3
	case 24:
		return 6
	}
	return 2
}

// matrixShape scales a module matrix: modules are mw dots wide and each
// row unit rh dots tall.
func matrixShape(m barcode.Matrix, mw, rh int) shape {
	mw, rh = max(mw, 1), max(rh, 1)
	h := 0
	for y := range m.Rows {
		h += rowUnits(m, y) * rh
	}
	bm := bitmap.New(m.Width()*mw, h)
	top := 0
	for y, row := range m.Rows {
		rhy := rowUnits(m, y) * rh
		for x, dark := range row {
			if dark {
				bm.FillRect(x*mw, top, mw, rhy, true)
			}
		}
		top += rhy
	}
	return boxShape(bm)
}

func rowUnits(m barcode.Matrix, y int) int {
	if m.RowHeights == nil {
		return 1
	}
	return m.RowHeights[y]
}

// boxShape wraps a bitmap whose box is the whole bitmap and whose typeset
// origin is its bottom-left corner.
func boxShape(bm *bitmap.Bitmap) shape {
	return shape{
		mask:    bm,
		box:     image.Rect(0, 0, bm.W, bm.H),
		anchor:  image.Pt(0, bm.H),
		ranchor: image.Pt(bm.W, bm.H),
	}
}

// offsetShape shifts a shape's content right/down within its placement box.
func offsetShape(s shape, dx, dy int) shape {
	bm := bitmap.New(s.mask.W+dx, s.mask.H+dy)
	bm.Draw(s.mask, dx, dy, bitmap.Black)
	out := boxShape(bm)
	out.box = image.Rect(0, 0, bm.W, bm.H)
	return out
}

func qrECLetter(c byte) (qr.ECLevel, bool) {
	switch c {
	case 'H':
		return qr.ECLevelH, true
	case 'Q':
		return qr.ECLevelQ, true
	case 'M':
		return qr.ECLevelM, true
	case 'L':
		return qr.ECLevelL, true
	}
	return 0, false
}

// qrFieldHasEC reports whether ^BQ field data starts (after an optional
// "Diijjxx," mixed-mode switch) with a valid error correction letter.
func qrFieldHasEC(data []byte) bool {
	if len(data) > 0 && data[0] == 'D' {
		i := bytes.IndexByte(data, ',')
		if i < 0 {
			return false
		}
		data = data[i+1:]
	}
	if len(data) == 0 {
		return false
	}
	_, ok := qrECLetter(data[0])
	return ok
}

func (f *format) qrShape(ps params, data []byte) (shape, error) {
	opts := qr.Options{Model: ps.int(1, 2)}
	if opts.Model != 1 {
		opts.Model = 2
	}
	// The ^BQ d parameter: Q when empty, M when invalid.
	paramEC := qr.ECLevelM
	if !ps.has(3) {
		paramEC = qr.ECLevelQ
	} else if l, ok := qrECLetter(ps.char(3, 0)); ok {
		paramEC = l
	}
	// The ^BQ mask parameter is ignored: a ZD420 produced identical symbols
	// for masks 0–7 and no mask, always choosing by penalty score (which
	// this encoder reproduces exactly).
	switch f.pr.p.charset.ID {
	case 28, 29, 30:
		opts.DisableAutoKanji = true
	}
	fd, err := qr.ParseZPL(data)
	if err != nil {
		return shape{}, err
	}
	// A valid level switch in the field data wins over the parameter.
	opts.ECLevel, opts.StructuredAppend = paramEC, fd.StructuredAppend
	if qrFieldHasEC(data) {
		opts.ECLevel = fd.ECLevel
	}
	// The printer encodes at exactly the requested level (verified on a
	// ZD420; Labelary instead raises it as far as the version allows).
	m, err := qr.Encode(fd.Segments, opts)
	if err != nil {
		return shape{}, err
	}
	mag := f.fit(clamp(ps.int(2, f.defaultMagnification()), 1, 100), m.Width())
	// Zebra printers place QR symbols 10 dots below the field origin.
	s := matrixShape(m, mag, mag)
	return offsetShape(s, 0, 10), nil
}

func (f *format) dataMatrixShape(b *barcodeSpec, st fieldState, data []byte) (shape, error) {
	ps := b.ps
	opts := datamatrix.Options{
		Quality:     ps.int(2, 0),
		Columns:     ps.int(3, 0),
		Rows:        ps.int(4, 0),
		FormatID:    ps.int(5, 6),
		Escape:      '_',
		Rectangular: ps.int(7, 1) == 2,
	}
	if s := ps.raw(6); s != "" {
		opts.Escape = s[0]
	}
	if opts.Columns > 49 {
		opts.Columns = 0
	}
	if opts.Rows > 49 {
		opts.Rows = 0
	}
	// The default quality is 0, and printers really print ECC 000 then (a
	// ZD420's ^BX symbol without a quality parameter is not ECC 200). ECC
	// 000–140 is not implemented, so it is rendered as ECC 200 and reported:
	// the image will differ from the printer's.
	if opts.Quality != 200 {
		msg := fmt.Sprintf("ECC %03d is not supported; rendered as ECC 200", opts.Quality)
		if !ps.has(2) {
			msg = "no quality given, so a printer would print ECC 000, which is not supported; rendered as ECC 200 (add quality 200 to ^BX to match)"
		}
		f.pr.warnings = append(f.pr.warnings, Warning{Offset: b.offset, Command: "^BX", Message: msg})
		opts.Quality = 200
	}
	m, err := datamatrix.Encode(data, opts)
	if err != nil {
		return shape{}, err
	}
	h := ps.int(1, 0)
	if h <= 0 {
		h = max(int(math.Round(float64(st.by.height)/float64(max(m.Height(), 1)))), 1)
	}
	h = f.fit(h, max(m.Width(), m.Height()))
	return matrixShape(m, h, h), nil
}

func (f *format) aztecShape(ps params, data []byte) (shape, error) {
	p := aztec.ZPLParams{
		ECIC:    ps.yes(2, false),
		Size:    ps.int(3, 0),
		Menu:    ps.yes(4, false),
		Symbols: clamp(ps.int(5, 1), 1, 26),
		ID:      ps.str(6),
	}
	m, err := aztec.EncodeZPL(data, p)
	if err != nil {
		return shape{}, err
	}
	mag := clamp(ps.int(1, f.defaultMagnification()), 1, 10)
	return matrixShape(m, mag, mag), nil
}

// maxiShape draws MaxiCode hexagons and the bullseye. Zebra printers print
// the symbol 25 mm wide (200 dots at 8 dots/mm), a little under the ISO
// nominal 28.14 mm.
func (f *format) maxiShape(ps params, data []byte) (shape, error) {
	m, err := maxicode.Encode(data, maxicode.Options{
		Mode:         clamp(ps.int(0, 2), 2, 6),
		SymbolNumber: clamp(ps.int(1, 1), 1, 8),
		SymbolCount:  clamp(ps.int(2, 1), 1, 8),
		ZebraPadding: true,
	})
	if err != nil {
		return shape{}, err
	}
	// The printer lays hexagons on a whole-dot grid: on a 300 dpi ZD420 the
	// symbol is 25 mm wide with 10-dot columns and 9-dot rows (299×298 dots),
	// i.e. the row pitch is the column pitch times √3/2, rounded.
	const widthMM = 25.0
	col := max(math.Round(widthMM/maxicode.SymbolWidth*float64(f.dpmm())), 3) // dots per column (W)
	row := max(math.Round(col*maxicode.RowPitch), 2)                          // dots per row
	center := func(x, y int) (float64, float64) {
		cx, _ := maxicode.ModuleCenter(x, y)
		return cx * col, float64(y)*row + maxicode.HexHeight/2*col
	}
	w := int(math.Ceil(maxicode.SymbolWidth * col))
	h := int(math.Ceil(float64(maxicode.Rows-1)*row + maxicode.HexHeight*col))
	bm := bitmap.New(w, h)
	r := maxicode.HexHeight / 2 * col // circumradius in dots
	for y, cells := range m.Rows {
		for x, dark := range cells {
			if dark {
				cx, cy := center(x, y)
				fillHexagon(bm, cx, cy, r*0.92)
			}
		}
	}
	fcx, fcy := center(14, 16)
	for _, ring := range maxicode.FinderRings() {
		fillRing(bm, fcx, fcy, ring[0]/2*col, ring[1]/2*col)
	}
	return boxShape(bm), nil
}

// fillHexagon paints a pointy-top regular hexagon with circumradius r.
func fillHexagon(bm *bitmap.Bitmap, cx, cy, r float64) {
	half := r * math.Sqrt(3) / 2
	for y := int(cy - r); y <= int(cy+r)+1; y++ {
		py := math.Abs(float64(y) + 0.5 - cy)
		if py > r {
			continue
		}
		m := min(half, math.Sqrt(3)*(r-py))
		fillSpan(bm, y, cx-m, cx+m, func(x int) bool {
			return hexInside(math.Abs(float64(x)+0.5-cx), py, r)
		})
	}
}

// hexInside tests a point (folded to the first quadrant) against a
// pointy-top hexagon of circumradius r.
func hexInside(px, py, r float64) bool {
	half := r * math.Sqrt(3) / 2
	return px <= half && py <= r && py <= r-px/math.Sqrt(3)
}

// fillRing paints the pixels whose centres lie between radii rin and rout
// of (cx, cy).
func fillRing(bm *bitmap.Bitmap, cx, cy, rin, rout float64) {
	for y := int(cy - rout); y <= int(cy+rout)+1; y++ {
		dy := float64(y) + 0.5 - cy
		if dy*dy > rout*rout {
			continue
		}
		in := func(x int) bool {
			dx := float64(x) + 0.5 - cx
			d := math.Sqrt(dx*dx + dy*dy)
			return d >= rin && d <= rout
		}
		mo := math.Sqrt(rout*rout - dy*dy)
		if dy*dy >= rin*rin {
			fillSpan(bm, y, cx-mo, cx+mo, in)
			continue
		}
		mi := math.Sqrt(rin*rin - dy*dy)
		fillSpan(bm, y, cx-mo, cx-mi, in)
		fillSpan(bm, y, cx+mi, cx+mo, in)
	}
}

// fillSpan fills the run of pixels in row y whose centres lie in [a, b],
// correcting the computed ends with the exact per-pixel test in.
func fillSpan(bm *bitmap.Bitmap, y int, a, b float64, in func(x int) bool) {
	lo, hi := int(math.Ceil(a-0.5)), int(math.Floor(b-0.5))
	for in(lo - 1) {
		lo--
	}
	for lo <= hi && !in(lo) {
		lo++
	}
	for in(hi + 1) {
		hi++
	}
	for hi >= lo && !in(hi) {
		hi--
	}
	if lo <= hi {
		bm.FillRect(lo, y, hi-lo+1, 1, true)
	}
}
