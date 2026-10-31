// Package verify is a separate module holding decode round-trip tests for the
// barcode encoders. It depends on gozxing, which keeps the main module free of
// third-party dependencies.
package verify

import (
	"image"
	"image/color"

	"github.com/datamagik/zpl-to-image/internal/barcode"
)

// MatrixImage draws m with square modules of scale pixels and a quiet zone of
// quiet modules on every side. Row heights (if any) are multiplied by rowUnit
// pixels; pass rowUnit 0 to use scale.
func MatrixImage(m barcode.Matrix, scale, rowUnit, quiet int) *image.Gray {
	if rowUnit == 0 {
		rowUnit = scale
	}
	h := 0
	for y := range m.Rows {
		h += rowHeight(m, y) * rowUnit
	}
	w := m.Width() * scale
	img := blank(w+2*quiet*scale, h+2*quiet*scale)
	oy := quiet * scale
	for y, row := range m.Rows {
		rh := rowHeight(m, y) * rowUnit
		for x, dark := range row {
			if dark {
				fill(img, quiet*scale+x*scale, oy, scale, rh)
			}
		}
		oy += rh
	}
	return img
}

func rowHeight(m barcode.Matrix, y int) int {
	if m.RowHeights == nil {
		return 1
	}
	return m.RowHeights[y]
}

// LinearImage draws l with the given narrow module width, wide:narrow ratio
// (ignored unless l.Ratio) and bar height, plus a 10-module quiet zone.
func LinearImage(l barcode.Linear, module int, ratio float64, height int) *image.Gray {
	wide := int(float64(module)*ratio + 0.5)
	width := 0
	for _, e := range l.Elements {
		width += elemWidth(l, e, module, wide)
	}
	q := 10 * module
	img := blank(width+2*q, height+2*q)
	x := q
	for i, e := range l.Elements {
		ew := elemWidth(l, e, module, wide)
		if i%2 == 0 {
			fill(img, x, q, ew, height)
		}
		x += ew
	}
	return img
}

func elemWidth(l barcode.Linear, e, module, wide int) int {
	if l.Ratio && e == 2 {
		return wide
	}
	return e * module
}

// PostalImage draws a height-modulated symbol with bars of width module,
// gaps of width module, and a full height of height pixels.
func PostalImage(p barcode.Postal, module, height int) *image.Gray {
	q := 10 * module
	img := blank(len(p.Bars)*2*module+2*q, height+2*q)
	third := height / 3
	for i, b := range p.Bars {
		x := q + i*2*module
		top, bot := q+third, q+2*third
		switch b {
		case barcode.BarFull:
			top, bot = q, q+height
		case barcode.BarAscender:
			top = q
		case barcode.BarDescender:
			bot = q + height
		}
		fill(img, x, top, module, bot-top)
	}
	return img
}

func blank(w, h int) *image.Gray {
	img := image.NewGray(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	return img
}

func fill(img *image.Gray, x, y, w, h int) {
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			img.SetGray(xx, yy, color.Gray{})
		}
	}
}
