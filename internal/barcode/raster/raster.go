// Package raster draws encoded symbols (barcode.Linear, Matrix, Postal and
// the multi-part symbologies) as bitmaps. It is shared by the ZPL engine,
// which sizes symbols from ^BY and command parameters, and by the public
// barcode package, which sizes them from its Options.
package raster

import (
	"math"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/maxicode"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/tlc39"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/bitmap"
)

// WideWidth is the width in dots of a wide element: round(ratio*module),
// but always at least one dot wider than a narrow element, as on a Zebra
// printer.
func WideWidth(module int, ratio float64) int {
	return max(int(float64(module)*ratio+0.5), module+1)
}

// Bars draws a linear symbol's bars, height dots high. Elements are module
// dots per unit; wide elements of a Ratio symbol are WideWidth dots.
func Bars(l barcode.Linear, module int, ratio float64, height int) *bitmap.Bitmap {
	wide := WideWidth(module, ratio)
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

// Matrix scales a module matrix: modules are mw dots wide and each row unit
// rh dots tall.
func Matrix(m barcode.Matrix, mw, rh int) *bitmap.Bitmap {
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
	return bm
}

func rowUnits(m barcode.Matrix, y int) int {
	if m.RowHeights == nil {
		return 1
	}
	return m.RowHeights[y]
}

// Postal draws POSTNET, PLANET or Intelligent Mail bars: bars module dots
// wide every pitch dots, full bars height dots high. POSTNET/PLANET short
// bars are the bottom 2/5; IMb trackers are the middle third and
// ascenders/descenders reach a third from the bottom/top.
func Postal(pc barcode.Postal, module, pitch, height int, imb bool) *bitmap.Bitmap {
	h := height
	bm := bitmap.New(max(len(pc.Bars)*pitch-(pitch-module), 1), h)
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
		bm.FillRect(x, top, module, bot-top, true)
	}
	return bm
}

// MaxiCode draws MaxiCode hexagons and the bullseye with col dots per
// module column. The hexagons lie on a whole-dot grid: the row pitch is the
// column pitch times √3/2, rounded, as on a Zebra printer.
func MaxiCode(m barcode.Matrix, col float64) *bitmap.Bitmap {
	row := max(math.Round(col*maxicode.RowPitch), 2) // dots per row
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
	return bm
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

// TLC39 draws a TLC39 symbol: the Code 39 bars (w1-dot modules, ratio r1,
// h1 dots high), the MicroPDF417 (w2-dot modules, h2-dot rows) above them
// and the linkage flag. pad is the number of dots the flag reaches above
// the MicroPDF417's top. Without a MicroPDF417 only the Code 39 bars are
// drawn.
func TLC39(sym tlc39.Symbol, w1 int, r1 float64, h1, w2, h2 int) (bm *bitmap.Bitmap, pad int) {
	lin := Bars(sym.Linear, w1, r1, h1)
	// The "T" linkage flag only marks a 2D part: with just the ECI number a
	// ZD420 prints the Code 39 symbol alone.
	if sym.MicroPDF == nil {
		return lin, 0
	}
	micro := Matrix(*sym.MicroPDF, w2, h2)
	// The MicroPDF417 top is one dot below the field origin (as on a
	// ZD420); the flag, centred on the bars, may reach above it.
	ext := tlc39.FlagExtension(h1, r1)
	flag := Bars(sym.Flag, w1, r1, h1+2*ext)
	linY := micro.H + w1
	pad = max(ext-linY, 0)
	flagX := lin.W + tlc39.FlagGap*w1
	w := max(flagX+flag.W, tlc39.MicroPDFOffset*w1+micro.W)
	out := bitmap.New(w, pad+linY+h1+ext)
	out.Draw(micro, tlc39.MicroPDFOffset*w1, pad, bitmap.Black)
	out.Draw(lin, 0, pad+linY, bitmap.Black)
	out.Draw(flag, flagX, pad+linY-ext, bitmap.Black)
	return out, pad
}
