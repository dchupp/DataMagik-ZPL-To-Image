package engine

import (
	"fmt"
	"math"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/codablock"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/code128"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/composite"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/databar"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/micropdf"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/pdf417"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/tlc39"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/upcean"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/bitmap"
)

// pdfFamily handles ^B7, ^BF, ^BB and ^BT.
func (f *format) pdfFamily(fld *field, st fieldState, b *barcodeSpec, data []byte) ([]shape, error) {
	ps, m := b.ps, st.by.module
	switch b.cmd {
	case "^B7":
		opts := pdf417.Options{
			SecurityLevel: clamp(ps.int(2, 0), 0, 8),
			Columns:       clamp(ps.int(3, 0), 0, 30),
			Rows:          clamp(ps.int(4, 0), 0, 90),
			Truncated:     ps.yes(5, false),
		}
		data = pdf417.ExpandFieldEscapes(data)
		var syms []barcode.Matrix
		var err error
		if len(fld.fm) > 1 {
			syms, err = pdf417.EncodeStructured(data, opts, len(fld.fm))
		} else {
			var s barcode.Matrix
			s, err = pdf417.Encode(data, opts)
			syms = []barcode.Matrix{s}
		}
		if err != nil {
			return nil, err
		}
		var out []shape
		for _, s := range syms {
			// h is the row height in dots; without it the ^BY height is
			// divided between the rows.
			rh := ps.int(1, 0)
			if rh <= 0 {
				rh = max(int(math.Round(float64(st.by.height)/float64(max(s.Height(), 1)))), 1)
			}
			out = append(out, matrixShape(s, m, f.fit(rh, s.Height())))
		}
		return out, nil
	case "^BF":
		s, err := micropdf.Encode(data, micropdf.Options{Mode: clamp(ps.int(2, 0), 0, 33)})
		if err != nil {
			return nil, err
		}
		// As with ^B7, h is the row height in dots; otherwise the ^BY
		// height is divided between the rows.
		rh := ps.int(1, 0)
		if rh <= 0 {
			rh = max(int(math.Round(float64(st.by.height)/float64(max(s.Height(), 1)))), 1)
		}
		return []shape{matrixShape(s, m, f.fit(rh, s.Height()))}, nil
	case "^BB":
		mode := codablock.Mode(ps.char(5, 'F'))
		if mode != codablock.ModeA && mode != codablock.ModeE {
			mode = codablock.ModeF
		}
		if c := ps.int(3, 0); mode != codablock.ModeA && c > 0 && c < codablock.MinColumnsF {
			f.pr.warnings = append(f.pr.warnings, Warning{Offset: b.offset, Command: "^BB",
				Message: fmt.Sprintf("CODABLOCK %c needs at least %d columns; using %d", mode, codablock.MinColumnsF, codablock.MinColumnsF)})
		}
		s, err := codablock.Encode(data, codablock.Options{
			Mode:    mode,
			Columns: ps.int(3, 0),
			Rows:    ps.int(4, 0),
			NoCheck: !ps.yes(2, true),
			// h is the row height in dots (as for ^B7; measured on a ZD420);
			// the encoder takes it in modules of the narrow width.
			RowHeight: max(f.fit(max(ps.int(1, 8), 2), 44)/m, 1),
			SepHeight: m,
			Narrow:    m,
			Ratio:     st.by.ratio,
		})
		if err != nil {
			return nil, err
		}
		return []shape{matrixShape(s, 1, 1)}, nil
	case "^BT":
		return f.tlc39(ps, data)
	}
	return nil, errNotImplemented
}

// barsBitmap draws a linear symbol's bars.
func barsBitmap(l barcode.Linear, module int, ratio float64, height int) *bitmap.Bitmap {
	wide := max(int(float64(module)*ratio+0.5), module+1)
	ew := func(e int) int {
		if l.Ratio && e == 2 {
			return wide
		}
		return e * module
	}
	w := 0
	for _, e := range l.Elements {
		w += ew(e)
	}
	bm := bitmap.New(w, height)
	x := 0
	for i, e := range l.Elements {
		if i%2 == 0 {
			bm.FillRect(x, 0, ew(e), height, true)
		}
		x += ew(e)
	}
	return bm
}

// tlc39 renders ^BT: Code 39 with a MicroPDF417 above it and a linkage flag.
func (f *format) tlc39(ps params, data []byte) ([]shape, error) {
	hiRes := f.dpmm() >= 24
	w1 := clamp(ps.int(1, map[bool]int{true: 4, false: 2}[hiRes]), 1, 10)
	r1 := min(max(ps.float(2, 2.0), 2), 3)
	h1 := clamp(ps.int(3, map[int]int{24: 120, 12: 60}[f.dpmm()]), 0, f.maxDim())
	if h1 == 0 {
		h1 = 40
	}
	w2 := clamp(ps.int(4, map[bool]int{true: 4, false: 2}[hiRes]), 1, 10)
	h2 := clamp(ps.int(5, map[bool]int{true: 8, false: 4}[hiRes]), 1, 255)
	sym, err := tlc39.Encode(string(data), tlc39.Options{})
	if err != nil {
		return nil, err
	}
	lin := barsBitmap(sym.Linear, w1, r1, h1)
	flag := barsBitmap(sym.Flag, w1, r1, h1)
	var micro *bitmap.Bitmap
	if sym.MicroPDF != nil {
		micro = matrixShape(*sym.MicroPDF, w2, h2).mask
	}
	top := 0
	if micro != nil {
		top = micro.H
	}
	flagX := lin.W + tlc39.FlagGap*w1
	w := flagX + flag.W
	if micro != nil {
		w = max(w, tlc39.MicroPDFOffset*w1+micro.W)
	}
	out := bitmap.New(w, top+h1)
	if micro != nil {
		out.Draw(micro, tlc39.MicroPDFOffset*w1, 0, bitmap.Black)
	}
	out.Draw(lin, 0, top, bitmap.Black)
	out.Draw(flag, flagX, top, bitmap.Black)
	return []shape{boxShape(out)}, nil
}

// composite renders ^BR types 7–12: UPC/EAN or GS1-128 with a 2D component.
func (f *format) composite(fld *field, st fieldState, ps params, typ, mag, sepH int, data string) ([]shape, error) {
	linData, ccData, _ := databar.SplitComposite(data)
	// The linear height e is in modules of the magnification.
	height := f.fit(max(ps.int(4, 25), 1)*mag, 1)
	var l barcode.Linear
	var err error
	var kind composite.Linear
	ccType := composite.Auto
	switch typ {
	case 7:
		kind = composite.UPCA
		l, err = upcean.EncodeUPCA(linData, upcean.UPCOptions{PrintCheckDigit: true})
	case 8:
		kind = composite.UPCE
		l, err = upcean.EncodeUPCE(linData, upcean.UPCOptions{PrintCheckDigit: true})
	case 9:
		kind = composite.EAN13
		l, err = upcean.EncodeEAN13(linData)
	case 10:
		kind = composite.EAN8
		l, err = upcean.EncodeEAN8(linData)
	case 11, 12:
		kind = composite.GS1128
		if typ == 12 {
			ccType = composite.CCC
		}
		l, err = code128.EncodeComposite(linData, typ == 12)
	}
	if err != nil {
		return nil, err
	}
	bars := barsBitmap(l, mag, 2, height)
	linW := l.Width(2)
	// Separator: inverse of the linear bars for GS1-128, the guard bars for
	// EAN/UPC (ISO/IEC 24723 separator patterns).
	sep := make([]bool, linW)
	x := 0
	for i, e := range l.Elements {
		for k := 0; k < e && x+k < linW; k++ {
			bar := i%2 == 0
			if kind == composite.GS1128 {
				sep[x+k] = !bar
			} else {
				sep[x+k] = bar && l.Guards != nil && i < len(l.Guards) && l.Guards[i]
			}
		}
		x += e
	}
	if ccData == "" {
		return []shape{boxShape(bars)}, nil
	}
	cc, err := composite.Encode(ccData, composite.Options{Linear: kind, Type: ccType, LinearWidth: linW})
	if err != nil {
		return nil, err
	}
	return []shape{assembleComposite(cc, sep, sepH, bars, 0, mag)}, nil
}

// compositeAbove stacks a CC-A/B component above a GS1 DataBar symbol.
func (f *format) compositeAbove(fld *field, st fieldState, m barcode.Matrix, sep []bool, ccData string, t databar.Type, mag int) ([]shape, error) {
	kind := map[databar.Type]composite.Linear{
		databar.TypeOmnidirectional:        composite.DataBar14,
		databar.TypeTruncated:              composite.DataBar14,
		databar.TypeStacked:                composite.DataBarStacked,
		databar.TypeStackedOmnidirectional: composite.DataBarStacked,
		databar.TypeLimited:                composite.DataBarLimited,
		databar.TypeExpanded:               composite.DataBarExpanded,
	}[t]
	cc, err := composite.Encode(ccData, composite.Options{Linear: kind, LinearWidth: m.Width()})
	if err != nil {
		return nil, err
	}
	sepH := 1
	return []shape{assembleComposite(cc, sep, sepH, matrixShape(m, mag, mag).mask, 0, mag)}, nil
}

// assembleComposite draws the 2D component, the separator row and the
// linear symbol (whose module 0 is at linX0 dots in linear) together.
func assembleComposite(cc composite.Component, sep []bool, sepH int, linear *bitmap.Bitmap, linX0, mag int) shape {
	comp := matrixShape(cc.Matrix, mag, mag).mask
	ccX := cc.Offset * mag
	minX := min(0, ccX)
	w := max(linear.W, ccX+comp.W, len(sep)*mag) - minX
	sepRows := sepH * mag
	out := bitmap.New(w, comp.H+sepRows+linear.H)
	out.Draw(comp, ccX-minX, 0, bitmap.Black)
	for i, dark := range sep {
		if dark {
			out.FillRect(i*mag-minX, comp.H, mag, sepRows, true)
		}
	}
	out.Draw(linear, -minX-linX0, comp.H+sepRows, bitmap.Black)
	s := boxShape(out)
	return s
}
