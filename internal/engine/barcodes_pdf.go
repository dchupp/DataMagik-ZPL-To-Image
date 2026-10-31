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
		s, err := micropdf.Encode(data, micropdf.Options{Mode: clamp(ps.int(2, 0), 0, 33), ZebraPadding: true})
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
			// h is the row pitch in dots, including the one-module separator
			// (measured on a ZD420: h=10 at ^BY2 gives 8-dot rows and 2-dot
			// separators). The encoder takes heights in narrow modules.
			RowHeight: max((f.fit(max(ps.int(1, 8), 2), 44)-m)/m, 1),
			SepHeight: 1,
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
	sym, err := tlc39.Encode(string(data), tlc39.Options{ZebraPadding: true, FieldSeparator: '*'})
	if err != nil {
		return nil, err
	}
	lin := barsBitmap(sym.Linear, w1, r1, h1)
	var micro *bitmap.Bitmap
	if sym.MicroPDF != nil {
		micro = matrixShape(*sym.MicroPDF, w2, h2).mask
	}
	// The "T" linkage flag only marks a 2D part: with just the ECI number a
	// ZD420 prints the Code 39 symbol alone.
	if micro == nil {
		return []shape{boxShape(lin)}, nil
	}
	// The MicroPDF417 top is one dot below the field origin (as on a
	// ZD420); the flag, centred on the bars, may reach above it.
	ext := tlc39.FlagExtension(h1, r1)
	flag := barsBitmap(sym.Flag, w1, r1, h1+2*ext)
	linY := micro.H + w1
	pad := max(ext-linY, 0)
	flagX := lin.W + tlc39.FlagGap*w1
	w := max(flagX+flag.W, tlc39.MicroPDFOffset*w1+micro.W)
	out := bitmap.New(w, pad+linY+h1+ext)
	out.Draw(micro, tlc39.MicroPDFOffset*w1, pad, bitmap.Black)
	out.Draw(lin, 0, pad+linY, bitmap.Black)
	out.Draw(flag, flagX, pad+linY-ext, bitmap.Black)
	s := boxShape(out)
	s.box.Min.Y = pad - 1
	return []shape{s}, nil
}

// composite renders ^BR types 7–12: UPC/EAN or GS1-128 with a 2D component.
//
// The geometry follows a ZD420: the linear symbol starts 7 modules (UPC/EAN)
// or 10 modules (GS1-128) right of the field origin, and further when the
// 2D component overhangs it on the left; UPC/EAN bars ignore the height e
// (74 modules, 60 for EAN-8) and are separated from the 2D component by
// three 2-module rows marking the outer guard bars.
func (f *format) composite(fld *field, st fieldState, ps params, typ, mag, sepH int, data string) ([]shape, error) {
	linData, ccData, _ := databar.SplitComposite(data)
	// The linear height e is in modules of the magnification.
	height := f.fit(max(ps.int(4, 25), 1)*mag, 1)
	var l barcode.Linear
	var err error
	var kind composite.Linear
	ccType := composite.Auto
	lead := 7 // modules between the field origin and the linear symbol
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
		lead = 10
		if typ == 12 {
			ccType = composite.CCC
		}
		link := code128.LinkNone
		switch {
		case ccData != "" && typ == 12:
			link = code128.LinkC
		case ccData != "":
			link = code128.LinkAB
		}
		l, err = code128.EncodeComposite(linData, link)
	}
	if err != nil {
		return nil, err
	}
	linW := l.Width(2)
	var sep [][]bool
	if kind == composite.GS1128 {
		// Separator: the inverse of the linear bars, d modules high.
		row := make([]bool, linW)
		x := 0
		for i, e := range l.Elements {
			for k := 0; k < e && x+k < linW; k++ {
				row[x+k] = i%2 == 1
			}
			x += e
		}
		for range sepH {
			sep = append(sep, row)
		}
	} else {
		height = f.fit(74*mag, 1)
		if kind == composite.EAN8 {
			height = f.fit(60*mag, 1)
		}
		// Separator rows span modules -1 to linW: the first and last bar
		// modules, then one module further out, then the first and last.
		inner := make([]bool, linW+2)
		inner[1], inner[linW] = true, true
		outer := make([]bool, linW+2)
		outer[0], outer[linW+1] = true, true
		sep = [][]bool{inner, inner, outer, outer, inner, inner}
	}
	bars := barsBitmap(l, mag, 2, height)
	if ccData == "" {
		out := bitmap.New(lead*mag+bars.W, bars.H)
		out.Draw(bars, lead*mag, 0, bitmap.Black)
		return []shape{boxShape(out)}, nil
	}
	cc, err := composite.Encode(ccData, composite.Options{Linear: kind, Type: ccType, LinearWidth: linW, ZebraParentheses: true})
	if err != nil {
		return nil, err
	}
	sepX := 0
	if kind != composite.GS1128 {
		sepX = -1
	}
	return []shape{assembleComposite(cc, sep, sepX, bars, lead, mag)}, nil
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
	cc, err := composite.Encode(ccData, composite.Options{Linear: kind, LinearWidth: m.Width(), ZebraParentheses: true})
	if err != nil {
		return nil, err
	}
	return []shape{assembleComposite(cc, [][]bool{sep}, 0, matrixShape(m, mag, mag).mask, 0, mag)}, nil
}

// assembleComposite draws the 2D component, the separator rows (one module
// high each, starting at module sepX of the linear symbol) and the linear
// symbol together. The linear symbol starts lead modules right of the
// origin, or further so that the 2D component starts at least one module
// right of it, as on a ZD420.
func assembleComposite(cc composite.Component, sep [][]bool, sepX int, linear *bitmap.Bitmap, lead, mag int) shape {
	comp := matrixShape(cc.Matrix, mag, mag).mask
	linX := max(lead, 1-cc.Offset) * mag
	ccX := linX + cc.Offset*mag
	w := max(linX+linear.W, ccX+comp.W)
	sepY := comp.H
	out := bitmap.New(w, sepY+len(sep)*mag+linear.H)
	out.Draw(comp, ccX, 0, bitmap.Black)
	for r, row := range sep {
		for i, dark := range row {
			if dark {
				out.FillRect(linX+(sepX+i)*mag, sepY+r*mag, mag, mag, true)
			}
		}
	}
	out.Draw(linear, linX, sepY+len(sep)*mag, bitmap.Black)
	return boxShape(out)
}
