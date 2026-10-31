package engine

import (
	"image"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/bitmap"
)

// fontSel is a font selection made by ^A or ^A@ inside a field.
type fontSel struct {
	name   byte   // font letter/digit; 0 when selected by path
	path   string // ^A@ object name
	rot    bitmap.Rotation
	rotSet bool
	h, w   int
}

// blockSpec is ^FB.
type blockSpec struct {
	width, lines, spacing int
	just                  byte
	indent                int
}

// tbSpec is ^TB.
type tbSpec struct {
	rot           bitmap.Rotation
	rotSet        bool
	width, height int
}

// field accumulates the commands between ^FO/^FT and ^FS.
type field struct {
	originSet bool
	x, y      int
	typeset   bool
	ftNoPos   bool // ^FT without coordinates: continue after last field
	just      int
	justSet   bool

	font     *fontSel
	data     []byte
	hasData  bool
	variable bool
	hexInd   byte
	block    *blockSpec
	tb       *tbSpec
	dir      byte
	gap      int
	reverse  bool
	barcode  *barcodeSpec
	symbol   *fontSel // ^GS
	graphics []graphic
	fn       int
	hasFN    bool
	serial   *serialSpec
	clock    *[3]byte // ^FC indicators
	fm       []image.Point
	fmSkip   []bool
}

// graphic is a pending graphic item (^GB, ^GF, ^XG …).
type graphic struct {
	mask  *bitmap.Bitmap
	white bool
	// rects, when mask is nil, describe the graphic as non-overlapping
	// rectangles within a w×h box (square-cornered ^GB), drawn without a
	// mask.
	rects []image.Rectangle
	w, h  int
}

func newField() *field { return &field{dir: 'H'} }

func (fld *field) empty() bool {
	return !fld.hasData && !fld.hasFN && fld.barcode == nil && len(fld.graphics) == 0 && fld.symbol == nil && fld.serial == nil
}

// shape is something ready to place: a mask with a logical box (used for
// ^FO placement) and a typeset anchor (used for ^FT).
type shape struct {
	mask    *bitmap.Bitmap
	rects   []image.Rectangle // instead of mask: unrotated rectangle fills
	size    image.Point       // shape size when mask is nil
	box     image.Rectangle
	anchor  image.Point // left typeset origin
	ranchor image.Point // right typeset origin (right justification)
	advance image.Point // pen position after the field, for chained ^FT
	white   bool
}

// endField finishes the current field (^FS, or implicitly at ^XZ).
func (f *format) endField() {
	fld := f.fld
	f.fld = newField()
	if f.global || fld.empty() {
		return
	}
	st := f.st
	data, has := fld.data, fld.hasData
	if fld.hasFN {
		if has {
			f.fnData[fld.fn] = data
		}
		if !fld.originSet && len(fld.graphics) == 0 {
			// A data-only ^FN field supplies data to templates.
			return
		}
		if !has {
			n := fld.fn
			f.templates = append(f.templates, template{n: n, render: func(d []byte) {
				c := *fld
				c.data, c.hasData = d, true
				c.graphics = nil // already drawn below
				f.renderField(&c, st)
			}})
			// Graphics in the template field still print.
			if len(fld.graphics) > 0 {
				g := *fld
				g.barcode, g.symbol, g.hasData = nil, nil, false
				f.renderField(&g, st)
			}
			return
		}
	}
	f.renderField(fld, st)
}

// renderField draws a complete field.
func (f *format) renderField(fld *field, st fieldState) {
	data := f.fieldData(fld, st)
	seq := f.fieldSeq
	f.fieldSeq++
	// Copies of a format run the same commands in the same order, so a
	// field with the same sequence number and data as in an earlier copy
	// renders identically: replay its drawing operations.
	var key fieldKey
	if f.memo != nil {
		key = fieldKey{seq, string(data)}
		if m, ok := f.memo.fields[key]; ok {
			for _, o := range m.ops {
				f.addOp(o)
			}
			f.nextFT, f.hasFT = m.nextFT, m.hasFT
			return
		}
	}
	start := len(f.ops)

	for _, g := range fld.graphics {
		w, h := g.w, g.h
		if g.mask != nil {
			w, h = g.mask.W, g.mask.H
		}
		s := shape{mask: g.mask, rects: g.rects, size: image.Pt(w, h), box: image.Rect(0, 0, w, h), white: g.white}
		s.anchor = image.Pt(0, h)
		s.ranchor = image.Pt(w, h)
		f.place(fld, st, s, bitmap.R0)
	}
	switch {
	case fld.barcode != nil:
		if fld.hasData || fld.serial != nil {
			f.drawBarcode(fld, st, data)
		}
	case fld.symbol != nil:
		f.drawSymbol(fld, st, data)
	case fld.hasData || fld.serial != nil:
		f.drawText(fld, st, data)
	}

	if f.memo != nil && len(f.memo.fields) < maxMemoFields {
		// Cached masks stay alive for the whole ^PQ run, so the cache has
		// the same pixel budget as one label's fields.
		ops := f.ops[start:]
		size := 0
		for _, o := range ops {
			if o.mask != nil {
				size += o.mask.W * o.mask.H
			}
		}
		if f.memo.pixels+size <= maxFormatPixels {
			f.memo.pixels += size
			f.memo.fields[key] = fieldResult{
				ops:    append([]op(nil), ops...),
				nextFT: f.nextFT,
				hasFT:  f.hasFT,
			}
		}
	}
}

// fieldKey identifies a field's output across the copies of one format.
type fieldKey struct {
	seq  int
	data string
}

// fieldResult is a field's recorded output.
type fieldResult struct {
	ops    []op
	nextFT image.Point
	hasFT  bool
}

// fieldMemo is the replay cache shared by the copies of one ^PQ run.
type fieldMemo struct {
	fields map[fieldKey]fieldResult
	pixels int // total size of the masks it keeps alive
}

// maxMemoFields bounds the replay cache of one ^PQ run.
const maxMemoFields = 4096

// place composites a shape onto the label according to the field origin,
// justification and rotation.
func (f *format) place(fld *field, st fieldState, s shape, rot bitmap.Rotation) {
	var mask *bitmap.Bitmap
	w, h := s.size.X, s.size.Y
	switch {
	case s.mask != nil:
		mask = s.mask.Rotate(rot)
		w, h = s.mask.W, s.mask.H
	case s.rects != nil:
		rot = bitmap.R0 // graphics are never rotated
	default:
		return
	}
	rp := func(p image.Point) image.Point {
		x, y := bitmap.RotatePoint(rot, w, h, p.X, p.Y)
		return image.Pt(x, y)
	}
	a, b := rp(s.box.Min), rp(s.box.Max)
	box := image.Rectangle{Min: a, Max: b}.Canon()

	just := st.fw.just
	if fld.justSet {
		just = fld.just
	}
	right := just == 1

	ox, oy := fld.x, fld.y
	if !fld.originSet {
		ox, oy = f.pr.p.lhX, f.pr.p.lhY
	}
	if fld.typeset && fld.ftNoPos && f.hasFT {
		ox, oy = f.nextFT.X, f.nextFT.Y
	}
	var dx, dy int
	if fld.typeset {
		anchor := s.anchor
		if right {
			anchor = s.ranchor
		}
		ra := rp(anchor)
		dx, dy = ox-ra.X, oy-ra.Y
	} else {
		dx, dy = ox-box.Min.X, oy-box.Min.Y
		if right {
			dx = ox - box.Max.X
		}
	}
	dx -= f.pr.p.ls
	dy += f.pr.p.lt

	if s.advance != (image.Point{}) {
		adv := rp(s.anchor.Add(s.advance))
		f.nextFT = image.Pt(dx+adv.X+f.pr.p.ls, dy+adv.Y-f.pr.p.lt)
		f.hasFT = true
	}

	mode := bitmap.Black
	if s.white {
		mode = bitmap.White
	}
	if fld.reverse || f.pr.p.reverse {
		mode = bitmap.Reverse
	}
	if mask == nil {
		for _, r := range s.rects {
			f.addOp(op{rect: r.Add(image.Pt(dx, dy)), mode: mode, variable: fld.variable})
		}
		return
	}
	f.addOp(op{mask: mask, x: dx, y: dy, mode: mode, variable: fld.variable})
}
