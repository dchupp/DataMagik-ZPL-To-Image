package raster

import (
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/code128"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/composite"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/databar"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/upcean"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/bitmap"
)

// CompositeLinear selects the linear component of a composite symbol. The
// values are Zebra's ^BR b parameter.
type CompositeLinear int

const (
	CompositeUPCA      CompositeLinear = 7
	CompositeUPCE      CompositeLinear = 8
	CompositeEAN13     CompositeLinear = 9
	CompositeEAN8      CompositeLinear = 10
	CompositeGS1128    CompositeLinear = 11 // with a CC-A or CC-B component
	CompositeGS1128CCC CompositeLinear = 12 // with a CC-C component
)

// CompositeOptions sizes a composite symbol. All sizes are in dots.
type CompositeOptions struct {
	// Mag is the module size.
	Mag int
	// SepHeight is the number of Mag-high separator rows of a GS1-128
	// composite.
	SepHeight int
	// Height is the GS1-128 bar height; UPCHeight and EAN8Height are the
	// UPC-A/UPC-E/EAN-13 and EAN-8 bar heights.
	Height, UPCHeight, EAN8Height int
	// Lead starts the linear symbol 7 modules (UPC/EAN) or 10 modules
	// (GS1-128) right of the origin, as on a ZD420; without it the image
	// starts at the leftmost module.
	Lead bool
	// ZebraParentheses encodes bracketed 2D data literally, as Zebra
	// printers do, instead of parsing GS1 AIs (see composite.Options).
	ZebraParentheses bool
}

// Composite draws a UPC/EAN or GS1-128 composite. data is the linear data,
// optionally followed by '|' and the 2D component's data.
//
// The geometry follows a ZD420: UPC/EAN bars are separated from the 2D
// component by three 2-module rows marking the outer guard bars, and the
// GS1-128 separator is the inverse of the linear bars.
func Composite(kind CompositeLinear, data string, o CompositeOptions) (*bitmap.Bitmap, error) {
	linData, ccData, _ := databar.SplitComposite(data)
	mag := o.Mag
	height := o.Height
	var l barcode.Linear
	var err error
	var lin composite.Linear
	ccType := composite.Auto
	lead := 7 // modules between the field origin and the linear symbol
	switch kind {
	case CompositeUPCA:
		lin = composite.UPCA
		l, err = upcean.EncodeUPCA(linData, upcean.UPCOptions{PrintCheckDigit: true})
	case CompositeUPCE:
		lin = composite.UPCE
		l, err = upcean.EncodeUPCE(linData, upcean.UPCOptions{PrintCheckDigit: true})
	case CompositeEAN13:
		lin = composite.EAN13
		l, err = upcean.EncodeEAN13(linData)
	case CompositeEAN8:
		lin = composite.EAN8
		l, err = upcean.EncodeEAN8(linData)
	default: // CompositeGS1128, CompositeGS1128CCC
		lin = composite.GS1128
		lead = 10
		if kind == CompositeGS1128CCC {
			ccType = composite.CCC
		}
		link := code128.LinkNone
		switch {
		case ccData != "" && kind == CompositeGS1128CCC:
			link = code128.LinkC
		case ccData != "":
			link = code128.LinkAB
		}
		l, err = code128.EncodeComposite(linData, link)
	}
	if err != nil {
		return nil, err
	}
	if !o.Lead {
		lead = 0
	}
	linW := l.Width(2)
	var sep [][]bool
	if lin == composite.GS1128 {
		// Separator: the inverse of the linear bars, SepHeight modules high.
		row := make([]bool, linW)
		x := 0
		for i, e := range l.Elements {
			for k := 0; k < e && x+k < linW; k++ {
				row[x+k] = i%2 == 1
			}
			x += e
		}
		for range o.SepHeight {
			sep = append(sep, row)
		}
	} else {
		height = o.UPCHeight
		if lin == composite.EAN8 {
			height = o.EAN8Height
		}
		// Separator rows span modules -1 to linW: the first and last bar
		// modules, then one module further out, then the first and last.
		inner := make([]bool, linW+2)
		inner[1], inner[linW] = true, true
		outer := make([]bool, linW+2)
		outer[0], outer[linW+1] = true, true
		sep = [][]bool{inner, inner, outer, outer, inner, inner}
	}
	bars := Bars(l, mag, 2, height)
	if ccData == "" {
		out := bitmap.New(lead*mag+bars.W, bars.H)
		out.Draw(bars, lead*mag, 0, bitmap.Black)
		return out, nil
	}
	cc, err := composite.Encode(ccData, composite.Options{Linear: lin, Type: ccType, LinearWidth: linW, ZebraParentheses: o.ZebraParentheses})
	if err != nil {
		return nil, err
	}
	sepX := 0
	if lin != composite.GS1128 {
		sepX = -1
	}
	return assemble(cc, sep, sepX, bars, lead, mag), nil
}

// DataBar draws a GS1 DataBar symbol of the given type with mag-dot
// modules. data may end in '|' and a CC-A/B component's data, which is
// then drawn above the symbol. sepH (1 or 2) multiplies the height of the
// separator rows of stacked types; segments is the Expanded segments per
// row (0 means 22).
func DataBar(t databar.Type, data string, mag, sepH, segments int, zebraParentheses bool) (*bitmap.Bitmap, error) {
	linear, ccData, hasCC := databar.SplitComposite(data)
	sym, err := databar.Encode(t, linear, databar.Options{Linkage: hasCC, Segments: segments, ZebraParentheses: zebraParentheses})
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
	if !hasCC {
		return Matrix(m, mag, mag), nil
	}
	kind := map[databar.Type]composite.Linear{
		databar.TypeOmnidirectional:        composite.DataBar14,
		databar.TypeTruncated:              composite.DataBar14,
		databar.TypeStacked:                composite.DataBarStacked,
		databar.TypeStackedOmnidirectional: composite.DataBarStacked,
		databar.TypeLimited:                composite.DataBarLimited,
		databar.TypeExpanded:               composite.DataBarExpanded,
	}[t]
	cc, err := composite.Encode(ccData, composite.Options{Linear: kind, LinearWidth: m.Width(), ZebraParentheses: zebraParentheses})
	if err != nil {
		return nil, err
	}
	return assemble(cc, [][]bool{sym.CompositeSeparator}, 0, Matrix(m, mag, mag), 0, mag), nil
}

// assemble draws the 2D component, the separator rows (one module high
// each, starting at module sepX of the linear symbol) and the linear symbol
// together. The linear symbol starts lead modules right of the origin, or
// further so that the 2D component starts at least one module right of it,
// as on a ZD420.
func assemble(cc composite.Component, sep [][]bool, sepX int, linear *bitmap.Bitmap, lead, mag int) *bitmap.Bitmap {
	comp := Matrix(cc.Matrix, mag, mag)
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
	return out
}
