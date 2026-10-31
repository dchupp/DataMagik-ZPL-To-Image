package font

import (
	"sync"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/bitmap"
)

// SymbolFace returns the ^GS graphic symbol font at an h×w cell. Field data
// characters select symbols: A ®, B ©, C ™, D UL mark, E CSA mark.
func SymbolFace(h, w int) Face {
	return &symbolFace{h: max(h, 1), w: max(w, 1)}
}

type symbolFace struct {
	h, w int
}

type symbolKey struct {
	r    rune
	w, h int
}

// symbolCache holds rendered symbols for all faces; rendering one means
// rasterising a large glyph and resampling it, so it is worth keeping.
var symbolCache = struct {
	sync.Mutex
	m      map[symbolKey]*bitmap.Bitmap
	pixels int
}{m: map[symbolKey]*bitmap.Bitmap{}}

// Symbol cache bounds: symbols up to 512×512 dots are cached, 16 Mpx in all.
const (
	maxCachedSymbol      = 512 * 512
	maxSymbolCachePixels = 16 << 20
)

func (f *symbolFace) Ascent() int        { return f.h }
func (f *symbolFace) Height() int        { return f.h }
func (f *symbolFace) Has(r rune) bool    { return r >= 'A' && r <= 'E' || r >= 'a' && r <= 'e' }
func (f *symbolFace) Advance(r rune) int { return f.w }

func (f *symbolFace) Draw(dst *bitmap.Bitmap, x, baseline int, r rune) {
	if r >= 'a' && r <= 'e' {
		r -= 'a' - 'A'
	}
	k := symbolKey{r, f.w, f.h}
	symbolCache.Lock()
	bm, ok := symbolCache.m[k]
	symbolCache.Unlock()
	if !ok {
		bm = f.render(r)
		// Only typical symbol sizes are kept; the cache is global and
		// long-lived, so it is bounded by total pixels, not just entries.
		if size := f.w * f.h; size <= maxCachedSymbol {
			symbolCache.Lock()
			if symbolCache.pixels+size > maxSymbolCachePixels {
				clear(symbolCache.m)
				symbolCache.pixels = 0
			}
			symbolCache.m[k] = bm
			symbolCache.pixels += size
			symbolCache.Unlock()
		}
	}
	if bm != nil {
		dst.Draw(bm, x, baseline-f.h, bitmap.Black)
	}
}

func (f *symbolFace) render(r rune) *bitmap.Bitmap {
	builtins()
	switch r {
	case 'A', 'B', 'C':
		ch := map[rune]rune{'A': '®', 'B': '©', 'C': '™'}[r]
		return fitGlyph(font0, ch, f.w, f.h)
	case 'D', 'E':
		bm := bitmap.New(f.w, f.h)
		ring(bm, max(f.w/12, 1))
		text := "UL"
		if r == 'E' {
			text = "SP"
		}
		inner := fitText(font0, text, f.w*6/10, f.h*5/10)
		bm.Draw(inner, (f.w-inner.W)/2, (f.h-inner.H)/2, bitmap.Black)
		return bm
	}
	return nil
}

// ring draws an ellipse outline of thickness t filling bm.
func ring(bm *bitmap.Bitmap, t int) {
	rx, ry := float64(bm.W)/2, float64(bm.H)/2
	ix, iy := rx-float64(t), ry-float64(t)
	for y := 0; y < bm.H; y++ {
		for x := 0; x < bm.W; x++ {
			dx, dy := float64(x)+0.5-rx, float64(y)+0.5-ry
			out := dx*dx/(rx*rx)+dy*dy/(ry*ry) <= 1
			in := ix > 0 && iy > 0 && dx*dx/(ix*ix)+dy*dy/(iy*iy) < 1
			if out && !in {
				bm.Set(x, y, true)
			}
		}
	}
}

// fitGlyph renders r from s scaled to fill a w×h box.
func fitGlyph(s *Scalable, r rune, w, h int) *bitmap.Bitmap {
	return fitText(s, string(r), w, h)
}

// fitText renders text from s with its ink bounds stretched to w×h.
func fitText(s *Scalable, text string, w, h int) *bitmap.Bitmap {
	const em = 200.0
	var gs []*glyph
	x := 0
	for _, r := range text {
		g := s.render(r, em, 1, 0x80)
		gs = append(gs, g)
		x += g.adv
	}
	tmp := bitmap.New(x+int(em), int(em)*2)
	pen := int(em) / 2
	for _, g := range gs {
		g.draw(tmp, pen, int(em)*3/2)
		pen += g.adv
	}
	return stretchInk(tmp, w, h)
}

// stretchInk crops src to its ink bounds and resamples it to w×h.
func stretchInk(src *bitmap.Bitmap, w, h int) *bitmap.Bitmap {
	x0, y0, x1, y1 := src.W, src.H, -1, -1
	for y := 0; y < src.H; y++ {
		for x := 0; x < src.W; x++ {
			if src.Get(x, y) {
				x0, y0 = min(x0, x), min(y0, y)
				x1, y1 = max(x1, x), max(y1, y)
			}
		}
	}
	out := bitmap.New(w, h)
	if x1 < 0 {
		return out
	}
	cw, ch := x1-x0+1, y1-y0+1
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			sx := x0 + (2*x+1)*cw/(2*w)
			sy := y0 + (2*y+1)*ch/(2*h)
			out.Set(x, y, src.Get(sx, sy))
		}
	}
	return out
}
