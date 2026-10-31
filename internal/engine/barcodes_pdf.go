package engine

import (
	"fmt"
	"math"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/codablock"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/micropdf"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/pdf417"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/raster"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/tlc39"
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
			// separators; at ^BY3 h=20 gives 17-dot rows, so rows are
			// h − module dots, not whole modules).
			RowHeight: max((f.fit(max(ps.int(1, 8), 2), 44)-m)/m, 1),
			RowDots:   max(f.fit(max(ps.int(1, 8), 2), 44)-m, 1),
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
	bm, pad := raster.TLC39(sym, w1, r1, h1, w2, h2)
	if sym.MicroPDF == nil {
		return []shape{boxShape(bm)}, nil
	}
	s := boxShape(bm)
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
func (f *format) composite(typ, mag, sepH int, ps params, data string) ([]shape, error) {
	bm, err := raster.Composite(raster.CompositeLinear(typ), data, raster.CompositeOptions{
		Mag:       mag,
		SepHeight: sepH,
		// The linear height e is in modules of the magnification.
		Height:           f.fit(max(ps.int(4, 25), 1)*mag, 1),
		UPCHeight:        f.fit(74*mag, 1),
		EAN8Height:       f.fit(60*mag, 1),
		Lead:             true,
		ZebraParentheses: true,
	})
	if err != nil {
		return nil, err
	}
	return []shape{boxShape(bm)}, nil
}
