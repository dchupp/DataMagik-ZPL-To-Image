package engine

import (
	"errors"
	"image"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/codabar"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/code11"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/code128"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/code39"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/code49"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/code93"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/databar"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/i2of5"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/msi"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/plessey"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/postal"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/twoof5"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/upcean"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/bitmap"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/font"
)

// hriGap is the space between bars and the interpretation line, in dots.
const hriGap = 5

// linearParams are the common ^B parameters of linear symbologies.
type linearParams struct {
	height int
	hri    bool
	above  bool
}

// lp reads height/print/above parameters at the given indexes.
func (f *format) lp(ps params, st fieldState, hi, fi, gi int, defHRI, defAbove bool) linearParams {
	return linearParams{
		height: f.fit(ps.int(hi, st.by.height), 1),
		hri:    ps.yes(fi, defHRI),
		above:  ps.yes(gi, defAbove),
	}
}

// otherBarcode encodes the linear, postal and stacked symbologies. It
// returns one shape per symbol (several for Macro PDF417 with ^FM).
func (f *format) otherBarcode(fld *field, st fieldState, b *barcodeSpec, data []byte) ([]shape, error) {
	ps, s := b.ps, string(data)
	var l barcode.Linear
	var err error
	var p linearParams
	kind := hriDefault
	switch b.cmd {
	case "^B1":
		p = f.lp(ps, st, 2, 3, 4, true, false)
		l, err = code11.Encode(s, code11.Options{OneCheckDigit: ps.yes(1, false)})
	case "^B2":
		p = f.lp(ps, st, 1, 2, 3, true, false)
		l, err = i2of5.Encode(s, i2of5.Options{CheckDigit: ps.yes(4, false)})
	case "^B3":
		p = f.lp(ps, st, 2, 3, 4, true, false)
		l, err = code39.Encode(s, code39.Options{CheckDigit: ps.yes(1, false)})
	case "^BL":
		p = linearParams{height: f.fit(ps.int(1, st.by.height), 1), hri: true, above: ps.yes(2, false)}
		l, err = code39.EncodeLOGMARS(s)
	case "^BA":
		p = f.lp(ps, st, 1, 2, 3, true, false)
		l, err = code93.Encode(s, code93.Options{PrintCheckDigits: ps.yes(4, false)})
	case "^BC":
		p = f.lp(ps, st, 1, 2, 3, true, false)
		opts := code128.Options{UCCCheckDigit: ps.yes(4, false)}
		switch ps.char(5, 'N') {
		case 'U':
			opts.Mode = code128.ModeUCC
		case 'A':
			opts.Mode = code128.ModeAuto
		case 'D':
			opts.Mode = code128.ModeGS1
			kind = hriGS1
		}
		l, err = code128.Encode(s, opts)
	case "^BI":
		p = f.lp(ps, st, 1, 2, 3, true, false)
		l, err = twoof5.EncodeIndustrial(s)
	case "^BJ":
		p = f.lp(ps, st, 1, 2, 3, true, false)
		l, err = twoof5.EncodeStandard(s)
	case "^BK":
		p = f.lp(ps, st, 2, 3, 4, true, false)
		l, err = codabar.Encode(s, codabar.Options{Start: ps.char(5, 'A'), Stop: ps.char(6, 'A')})
	case "^BM":
		p = f.lp(ps, st, 2, 3, 4, true, false)
		l, err = msi.Encode(s, msi.Options{Check: msi.Check(ps.char(1, 'B')), PrintCheckDigits: ps.yes(5, false)})
	case "^BP":
		p = f.lp(ps, st, 2, 3, 4, true, false)
		l, err = plessey.Encode(s, plessey.Options{PrintCheckDigit: ps.yes(1, false)})
	case "^B8":
		p, kind = f.lp(ps, st, 1, 2, 3, true, false), hriUPC
		l, err = upcean.EncodeEAN8(s)
	case "^BE":
		p, kind = f.lp(ps, st, 1, 2, 3, true, false), hriUPC
		l, err = upcean.EncodeEAN13(s)
	case "^BU":
		p, kind = f.lp(ps, st, 1, 2, 3, true, false), hriUPC
		l, err = upcean.EncodeUPCA(s, upcean.UPCOptions{PrintCheckDigit: ps.yes(4, true)})
	case "^B9":
		p, kind = f.lp(ps, st, 1, 2, 3, true, false), hriUPC
		l, err = upcean.EncodeUPCE(s, upcean.UPCOptions{PrintCheckDigit: ps.yes(4, true)})
	case "^BS":
		p, kind = f.lp(ps, st, 1, 2, 3, true, true), hriUPC
		l, err = upcean.EncodeAddOn(s)
	case "^B5":
		p = f.lp(ps, st, 1, 2, 3, false, false)
		pc, err := postal.EncodePLANET(s)
		if err != nil {
			return nil, err
		}
		return []shape{f.postalShape(fld, st, pc, p, false)}, nil
	case "^BZ":
		p = f.lp(ps, st, 1, 2, 3, false, false)
		var pc barcode.Postal
		switch ps.int(4, 0) {
		case 1:
			pc, err = postal.EncodePLANET(s)
		case 3:
			pc, err = postal.EncodeIMb(s)
		default:
			pc, err = postal.EncodePOSTNET(s)
		}
		if err != nil {
			return nil, err
		}
		return []shape{f.postalShape(fld, st, pc, p, ps.int(4, 0) == 3)}, nil
	default:
		return f.stackedBarcode(fld, st, b, data)
	}
	if err != nil {
		return nil, err
	}
	return []shape{f.linearShape(fld, st, l, p, kind)}, nil
}

// hriKind selects the interpretation line style.
type hriKind int

const (
	hriDefault hriKind = iota // font A magnified by the module width, centred
	hriUPC                    // UPC/EAN digit groups between guard bars
	hriGS1                    // font 0 sized to the symbol width
)

// linearShape draws bars and the interpretation line.
func (f *format) linearShape(fld *field, st fieldState, l barcode.Linear, p linearParams, kind hriKind) shape {
	m := st.by.module
	wide := max(int(float64(m)*st.by.ratio+0.5), m+1)
	if !l.Ratio {
		wide = 0
	}
	ew := func(e int) int {
		if l.Ratio && e == 2 {
			return wide
		}
		return e * m
	}
	width := 0
	for _, e := range l.Elements {
		width += ew(e)
	}

	guardExt := 0
	if l.Guards != nil && p.hri {
		guardExt = 20 * f.dpmm() / 8
	}
	bars := bitmap.New(width, p.height+guardExt)
	x := 0
	for i, e := range l.Elements {
		w := ew(e)
		if i%2 == 0 {
			h := p.height
			if l.Guards != nil && i < len(l.Guards) && l.Guards[i] {
				h += guardExt
			}
			bars.FillRect(x, 0, w, h, true)
		}
		x += w
	}
	if !p.hri || l.Text == "" {
		return boxShape(bars)
	}

	if kind == hriUPC && fld.font == nil && len(l.TextGroups) > 0 {
		return f.upcShape(bars, l, p, m)
	}

	var face font.Face
	centred := true
	switch {
	case fld.font != nil:
		face, _ = f.resolveFace(fld.font, st)
		centred = false
	case kind == hriGS1:
		h := 16 * m
		probe := font.Font0().Face(h, h)
		tw := runeWidth(probe, []rune(l.Text), 0)
		w := h
		if tw > 0 {
			w = clamp(h*width*95/100/tw, h/2, h)
		}
		face = font.Font0().Face(h, w)
	default:
		face = font.BitmapFont('A', f.dpmm()).Face(9*m, 5*m)
	}
	text := []rune(f.pr.p.charset.Decode([]byte(l.Text)))
	tw := runeWidth(face, text, 0)
	th := face.Height()
	tx := 0
	if centred {
		tx = (width - tw) / 2
	}
	left := min(tx, 0)
	right := max(width, tx+tw)
	out := bitmap.New(right-left, bars.H+hriGap+th)
	barY, textY := 0, bars.H+hriGap
	if p.above {
		barY, textY = th+hriGap, 0
	}
	out.Draw(bars, -left, barY, bitmap.Black)
	pen := tx - left
	for _, r := range text {
		face.Draw(out, pen, textY+face.Ascent(), r)
		pen += face.Advance(r)
	}
	s := boxShape(out)
	s.box = image.Rect(-left, barY, -left+width, barY+bars.H)
	s.anchor = image.Pt(-left, barY+p.height)
	s.ranchor = image.Pt(-left+width, barY+p.height)
	return s
}

// upcShape lays out UPC/EAN digit groups. Below the OCR-B threshold the
// printer uses font A, magnified by the module width.
func (f *format) upcShape(bars *bitmap.Bitmap, l barcode.Linear, p linearParams, m int) shape {
	threshold := map[int]int{6: 2, 8: 3, 12: 5, 24: 9}[f.dpmm()]
	small := font.BitmapFont('A', f.dpmm()).Face(9*m, 5*m)
	// Main digits are OCR-B with a cap height of about 8.7 modules.
	ocrb := font.BitmapFont('E', 8)
	scale := 8.67 * float64(m) / float64(ocrb.Ascent)
	main := ocrb.ScaledFace(scale, scale)
	if m < threshold {
		main = font.BitmapFont('A', f.dpmm()).ScaledFace(float64(m)*1.2, float64(m)*1.2)
	}
	th := max(main.Height(), small.Height())
	left, right := 0, bars.W
	type placed struct {
		face font.Face
		text []rune
		x    int
	}
	var groups []placed
	for _, g := range l.TextGroups {
		face := main
		if g.Small {
			face = small
		} else if avail := (g.End - g.Start) * m; avail > 0 {
			// Narrow the digits if the group would cover the guard bars.
			if tw := runeWidth(main, []rune(g.Text), 0); tw > avail {
				spec := ocrb
				sy := scale
				if m < threshold {
					spec, sy = font.BitmapFont('A', f.dpmm()), float64(m)*1.2
				}
				face = spec.ScaledFace(sy*float64(avail)/float64(tw), sy)
			}
		}
		rs := []rune(g.Text)
		tw := runeWidth(face, rs, 0)
		x := ((g.Start+g.End)*m - tw) / 2
		if g.End <= 0 {
			x = g.End*m - tw // right-aligned left of the symbol
		}
		groups = append(groups, placed{face, rs, x})
		left, right = min(left, x), max(right, x+tw)
	}
	textTop := p.height + 1
	if p.above {
		textTop = 0
	}
	barY := 0
	if p.above {
		barY = th + hriGap
	}
	h := max(barY+bars.H, textTop+th)
	if !p.above {
		h = max(bars.H, textTop+th)
	}
	out := bitmap.New(right-left, h)
	out.Draw(bars, -left, barY, bitmap.Black)
	for _, g := range groups {
		base := textTop + main.Ascent()
		if p.above {
			base = textTop + g.face.Ascent()
		}
		pen := g.x - left
		for _, r := range g.text {
			g.face.Draw(out, pen, base, r)
			pen += g.face.Advance(r)
		}
	}
	s := boxShape(out)
	s.box = image.Rect(-left, barY, -left+bars.W, barY+bars.H)
	s.anchor = image.Pt(-left, barY+p.height)
	s.ranchor = image.Pt(-left+bars.W, barY+p.height)
	return s
}

// postalShape draws POSTNET, PLANET or Intelligent Mail bars.
func (f *format) postalShape(fld *field, st fieldState, pc barcode.Postal, p linearParams, imb bool) shape {
	m := st.by.module
	pitch := 2 * m
	if !imb {
		pitch = max(int(float64(m)*st.by.ratio+0.5), m+1) + m
	}
	h := p.height
	bm := bitmap.New(max(len(pc.Bars)*pitch-(pitch-m), 1), h)
	for i, b := range pc.Bars {
		x := i * pitch
		top, bot := 0, h
		switch b {
		case barcode.BarTracker:
			if imb {
				top, bot = h/3, h-h/3
			} else {
				top = h - h*2/5
			}
		case barcode.BarAscender:
			bot = h - h/3
		case barcode.BarDescender:
			top = h / 3
		}
		bm.FillRect(x, top, m, bot-top, true)
	}
	if !p.hri || pc.Text == "" {
		return boxShape(bm)
	}
	face := font.BitmapFont('A', f.dpmm()).Face(9*m, 5*m)
	if fld.font != nil {
		face, _ = f.resolveFace(fld.font, st)
	}
	text := []rune(pc.Text)
	tw, th := runeWidth(face, text, 0), face.Height()
	out := bitmap.New(max(bm.W, tw), h+hriGap+th)
	barY, textY := 0, h+hriGap
	if p.above {
		barY, textY = th+hriGap, 0
	}
	out.Draw(bm, 0, barY, bitmap.Black)
	pen := (bm.W - tw) / 2
	pen = max(pen, 0)
	for _, r := range text {
		face.Draw(out, pen, textY+face.Ascent(), r)
		pen += face.Advance(r)
	}
	s := boxShape(out)
	s.anchor = image.Pt(0, barY+h)
	return s
}

var errNotImplemented = errors.New("symbology not available")

// stackedBarcode handles PDF417, MicroPDF417, CODABLOCK, Code 49, TLC39 and
// GS1 DataBar.
func (f *format) stackedBarcode(fld *field, st fieldState, b *barcodeSpec, data []byte) ([]shape, error) {
	ps := b.ps
	switch b.cmd {
	case "^BR":
		return f.dataBar(fld, st, ps, string(data))
	case "^B4":
		m := st.by.module
		rowMod := ps.int(1, 0)
		if rowMod <= 0 {
			rowMod = max(st.by.height/m, 1)
		}
		rowMod = f.fit(rowMod*m, 16) / m
		sym, err := code49.Encode(string(data), code49.Options{Mode: ps.char(3, 'A'), RowHeight: rowMod})
		if err != nil {
			return nil, err
		}
		s := matrixShape(sym.Matrix, m, m)
		if hri := ps.char(2, 'N'); hri == 'A' || hri == 'B' {
			s = f.addText(fld, st, s, sym.Text, hri == 'A', false)
		}
		return []shape{s}, nil
	}
	return f.pdfFamily(fld, st, b, data)
}

// addText adds an interpretation line (font A magnified by the module width,
// or the field's ^A font) above or below a symbol shape.
func (f *format) addText(fld *field, st fieldState, s shape, text string, above, centred bool) shape {
	m := st.by.module
	face := font.BitmapFont('A', f.dpmm()).Face(9*m, 5*m)
	if fld.font != nil {
		face, _ = f.resolveFace(fld.font, st)
	}
	rs := []rune(text)
	tw, th := runeWidth(face, rs, 0), face.Height()
	w := s.mask.W
	tx := 0
	if centred {
		tx = max((w-tw)/2, 0)
	}
	out := bitmap.New(max(w, tx+tw), s.mask.H+hriGap+th)
	symY, textY := 0, s.mask.H+hriGap
	if above {
		symY, textY = th+hriGap, 0
	}
	out.Draw(s.mask, 0, symY, bitmap.Black)
	pen := tx
	for _, r := range rs {
		face.Draw(out, pen, textY+face.Ascent(), r)
		pen += face.Advance(r)
	}
	n := boxShape(out)
	n.box = s.box.Add(image.Pt(0, symY))
	n.anchor = s.anchor.Add(image.Pt(0, symY))
	n.ranchor = s.ranchor.Add(image.Pt(0, symY))
	return n
}

// dataBar renders ^BR: GS1 DataBar types 1–6 and the composite types 7–12.
func (f *format) dataBar(fld *field, st fieldState, ps params, data string) ([]shape, error) {
	typ := clamp(ps.int(1, 1), 1, 12)
	mag := clamp(ps.int(2, map[int]int{12: 3, 24: 6}[f.dpmm()]), 1, 10)
	if !ps.has(2) && mag == 0 {
		mag = 2
	}
	sepH := clamp(ps.int(3, 1), 1, 2)
	if typ >= 7 {
		return f.composite(fld, st, ps, typ, mag, sepH, data)
	}
	linear, cc, hasCC := databar.SplitComposite(data)
	sym, err := databar.Encode(databar.Type(typ), linear, databar.Options{Linkage: hasCC, Segments: ps.int(5, 22)})
	if err != nil {
		return nil, err
	}
	m := sym.Matrix
	if sepH > 1 {
		m.RowHeights = append([]int(nil), m.RowHeights...)
		for _, i := range sym.SeparatorRows {
			m.RowHeights[i] *= sepH
		}
	}
	if hasCC {
		return f.compositeAbove(fld, st, m, sym.CompositeSeparator, cc, databar.Type(typ), mag)
	}
	return []shape{matrixShape(m, mag, mag)}, nil
}
