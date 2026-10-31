package engine

import (
	"bytes"
	"fmt"
	"image"
	"math"

	"github.com/datamagik/zpl-to-image/internal/barcode"
	"github.com/datamagik/zpl-to-image/internal/barcode/aztec"
	"github.com/datamagik/zpl-to-image/internal/barcode/datamatrix"
	"github.com/datamagik/zpl-to-image/internal/barcode/maxicode"
	"github.com/datamagik/zpl-to-image/internal/barcode/qr"
	"github.com/datamagik/zpl-to-image/internal/bitmap"
	"github.com/datamagik/zpl-to-image/internal/font"
	"github.com/datamagik/zpl-to-image/internal/lexer"
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
	// The guide documents a default mask of 7, but printers choose the mask
	// by penalty score unless one is given explicitly.
	if ps.has(4) {
		opts.ForceMask, opts.Mask = true, clamp(ps.int(4, 7), 0, 7)
	}
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
	m, err := qr.Encode(fd.Segments, opts)
	if err != nil {
		return shape{}, err
	}
	// Like the printer, use the smallest version for the requested level,
	// then raise the error correction as far as that version allows.
	opts.Version = (m.Width() - 17) / 4
	for lvl := opts.ECLevel + 1; lvl <= qr.ECLevelH; lvl++ {
		o := opts
		o.ECLevel = lvl
		if better, err := qr.Encode(fd.Segments, o); err == nil {
			m = better
		}
	}
	mag := f.fit(clamp(ps.int(2, f.defaultMagnification()), 1, 100), m.Width())
	// Zebra printers place QR symbols 10 dots below the field origin.
	s := matrixShape(m, mag, mag)
	return offsetShape(s, 0, 10), nil
}

func (f *format) dataMatrixShape(b *barcodeSpec, st fieldState, data []byte) (shape, error) {
	ps := b.ps
	opts := datamatrix.Options{
		Quality:     ps.int(2, 200),
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
	// Zebra documents quality 0 as the default, but ECC 000–140 is a
	// closed legacy format; an omitted quality is treated as ECC 200 and
	// only an explicit request for ECC 000–140 is reported.
	if opts.Quality != 200 {
		f.pr.warnings = append(f.pr.warnings, Warning{Offset: b.offset, Command: "^BX", Message: fmt.Sprintf("ECC %03d is not supported; encoding as ECC 200", opts.Quality)})
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
	})
	if err != nil {
		return shape{}, err
	}
	const widthMM = 25.0
	unit := widthMM / maxicode.SymbolWidth * float64(f.dpmm()) // dots per W
	w := int(math.Ceil(maxicode.SymbolWidth * unit))
	h := int(math.Ceil(maxicode.SymbolHeight * unit))
	bm := bitmap.New(w, h)
	r := maxicode.HexHeight / 2 // circumradius in W
	for y, row := range m.Rows {
		for x, dark := range row {
			if !dark {
				continue
			}
			cx, cy := maxicode.ModuleCenter(x, y)
			fillHexagon(bm, cx*unit, cy*unit, r*unit*0.92)
		}
	}
	fcx, fcy := maxicode.FinderCenter()
	for _, ring := range maxicode.FinderRings() {
		fillRing(bm, fcx*unit, fcy*unit, ring[0]/2*unit, ring[1]/2*unit)
	}
	return boxShape(bm), nil
}

// fillHexagon paints a pointy-top regular hexagon with circumradius r.
func fillHexagon(bm *bitmap.Bitmap, cx, cy, r float64) {
	half := r * math.Sqrt(3) / 2
	for y := int(cy - r); y <= int(cy+r)+1; y++ {
		for x := int(cx - half); x <= int(cx+half)+1; x++ {
			px, py := math.Abs(float64(x)+0.5-cx), math.Abs(float64(y)+0.5-cy)
			if hexInside(px, py, r) {
				bm.Set(x, y, true)
			}
		}
	}
}

// hexInside tests a point (folded to the first quadrant) against a
// pointy-top hexagon of circumradius r.
func hexInside(px, py, r float64) bool {
	half := r * math.Sqrt(3) / 2
	return px <= half && py <= r && py <= r-px/math.Sqrt(3)
}

func fillRing(bm *bitmap.Bitmap, cx, cy, rin, rout float64) {
	for y := int(cy - rout); y <= int(cy+rout)+1; y++ {
		for x := int(cx - rout); x <= int(cx+rout)+1; x++ {
			dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
			d := math.Sqrt(dx*dx + dy*dy)
			if d >= rin && d <= rout {
				bm.Set(x, y, true)
			}
		}
	}
}
