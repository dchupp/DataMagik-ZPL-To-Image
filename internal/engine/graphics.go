package engine

import (
	"math"

	"github.com/datamagik/zpl-to-image/internal/bitmap"
)

// boxMask draws ^GB: a w×h box with border thickness t and corner rounding
// index r (0–8).
func boxMask(w, h, t, r int) *bitmap.Bitmap {
	m := bitmap.New(w, h)
	rad := float64(r) / 8 * float64(min(w, h)) / 2
	if rad < 0.5 {
		m.FillRect(0, 0, w, h, true)
		if 2*t < w && 2*t < h {
			m.FillRect(t, t, w-2*t, h-2*t, false)
		}
		return m
	}
	inner := math.Max(rad-float64(t), 0)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			if !inRoundRect(px, py, 0, 0, float64(w), float64(h), rad) {
				continue
			}
			if 2*t < w && 2*t < h && inRoundRect(px, py, float64(t), float64(t), float64(w-t), float64(h-t), inner) {
				continue
			}
			m.Set(x, y, true)
		}
	}
	return m
}

func inRoundRect(px, py, x0, y0, x1, y1, r float64) bool {
	if px < x0 || py < y0 || px > x1 || py > y1 {
		return false
	}
	cx := math.Max(x0+r, math.Min(px, x1-r))
	cy := math.Max(y0+r, math.Min(py, y1-r))
	dx, dy := px-cx, py-cy
	return dx*dx+dy*dy <= r*r
}

// ellipseMask draws ^GE/^GC: an ellipse ring of thickness t in a w×h box.
func ellipseMask(w, h, t int) *bitmap.Bitmap {
	m := bitmap.New(w, h)
	rx, ry := float64(w)/2, float64(h)/2
	ix, iy := rx-float64(t), ry-float64(t)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx, dy := float64(x)+0.5-rx, float64(y)+0.5-ry
			if dx*dx/(rx*rx)+dy*dy/(ry*ry) > 1 {
				continue
			}
			if ix > 0 && iy > 0 && dx*dx/(ix*ix)+dy*dy/(iy*iy) < 1 {
				continue
			}
			m.Set(x, y, true)
		}
	}
	return m
}

// diagonalMask draws ^GD: a line of horizontal thickness t joining opposite
// corners of a w×h box, right-leaning (/) or left-leaning (\). As on the
// printer, the stroke extends t dots to the right of the box.
func diagonalMask(w, h, t int, right bool) *bitmap.Bitmap {
	m := bitmap.New(w+t, h)
	span := float64(w)
	for y := 0; y < h; y++ {
		// Left edge of the stroke at this row.
		frac := (float64(y) + 0.5) / float64(h)
		var x0 float64
		if right {
			x0 = span * (1 - frac)
		} else {
			x0 = span * frac
		}
		start := int(math.Round(x0))
		m.FillRect(start, y, t, 1, true)
	}
	return m
}

// rowsMask converts packed 1-bpp rows (MSB first, 1 = black) to a bitmap.
func rowsMask(data []byte, rowBytes int) *bitmap.Bitmap {
	if rowBytes <= 0 {
		return bitmap.New(0, 0)
	}
	h := len(data) / rowBytes
	m := bitmap.New(rowBytes*8, h)
	for y := 0; y < h; y++ {
		for x := 0; x < rowBytes*8; x++ {
			if data[y*rowBytes+x/8]&(0x80>>(x%8)) != 0 {
				m.Pix[y*m.W+x] = 1
			}
		}
	}
	return m
}
