package font

import (
	"math"
	"sync"
	"unicode"

	"github.com/datamagik/zpl-to-image/internal/bitmap"
)

// BitmapSpec describes a fixed dot-matrix font.
type BitmapSpec struct {
	Name      byte
	H, W      int // character matrix in dots
	Gap       int // inter-character gap in dots
	Ascent    int // rows above the baseline
	UpperOnly bool

	mu     sync.Mutex
	glyphs map[glyphKey]*glyph
}

// Zebra's resident bitmap fonts at 6 and 8 dots/mm. Fonts E and H have
// higher resolution matrices on 12 and 24 dots/mm printers; see BitmapFont.
var bitmapSpecs = map[byte]*BitmapSpec{
	'A': {Name: 'A', H: 9, W: 5, Gap: 1, Ascent: 7},
	'B': {Name: 'B', H: 11, W: 7, Gap: 2, Ascent: 11, UpperOnly: true},
	'C': {Name: 'C', H: 18, W: 10, Gap: 2, Ascent: 14},
	'D': {Name: 'D', H: 18, W: 10, Gap: 2, Ascent: 14},
	'E': {Name: 'E', H: 28, W: 15, Gap: 5, Ascent: 23},
	'F': {Name: 'F', H: 26, W: 13, Gap: 3, Ascent: 21},
	'G': {Name: 'G', H: 60, W: 40, Gap: 8, Ascent: 48},
	'H': {Name: 'H', H: 21, W: 13, Gap: 6, Ascent: 21, UpperOnly: true},
}

var hiResSpecs = map[[2]int]*BitmapSpec{
	{'E', 12}: {Name: 'E', H: 42, W: 20, Gap: 6, Ascent: 34},
	{'E', 24}: {Name: 'E', H: 84, W: 40, Gap: 12, Ascent: 68},
	{'H', 12}: {Name: 'H', H: 34, W: 22, Gap: 9, Ascent: 34, UpperOnly: true},
	{'H', 24}: {Name: 'H', H: 68, W: 44, Gap: 18, Ascent: 68, UpperOnly: true},
}

// BitmapFont returns the resident bitmap font spec for name at the given
// dots/mm, or nil if name is not a bitmap font.
func BitmapFont(name byte, dpmm int) *BitmapSpec {
	if s, ok := hiResSpecs[[2]int{int(name), dpmm}]; ok {
		return s
	}
	return bitmapSpecs[name]
}

// IsBitmap reports whether name is a resident bitmap font (A–H).
func IsBitmap(name byte) bool { return bitmapSpecs[name] != nil }

// Magnification converts a commanded size to an integer magnification of a
// native size, as Zebra does for bitmap fonts (1–10, rounding to nearest,
// never below 1).
func Magnification(commanded, native int) int {
	if commanded <= 0 || native <= 0 {
		return 1
	}
	m := int(math.Round(float64(commanded) / float64(native)))
	return min(max(m, 1), 10)
}

type glyphKey struct {
	r    rune
	w, h int // cell size in dots
}

// glyph returns r rendered into a w×h dot cell (the native matrix scaled by
// the magnification). Rendering at the final size from the outline keeps
// magnified text smooth.
func (s *BitmapSpec) glyph(r rune, w, h int) *glyph {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.glyphs == nil {
		s.glyphs = map[glyphKey]*glyph{}
	}
	k := glyphKey{r, w, h}
	if g, ok := s.glyphs[k]; ok {
		return g
	}
	g := s.outlineGlyph(r, w, h)
	s.glyphs[k] = g
	return g
}

// outlineGlyph rasterises r from Liberation Mono so that capital letters
// fill the rows above the baseline and the glyph fills the cell width.
func (s *BitmapSpec) outlineGlyph(r rune, w, h int) *glyph {
	builtins()
	const capHeight = 1349.0 / 2048 // Liberation Mono cap height / em
	const advance = 1229.0 / 2048   // monospace advance / em
	ascent := int(math.Round(float64(s.Ascent*h) / float64(s.H)))
	em := float64(ascent) / capHeight
	sx := float64(w) / (advance * em)
	// Small cells need a lower coverage threshold to keep thin strokes.
	threshold := uint8(0x80)
	if h < 20 {
		threshold = 0x40
	}
	g := mono.render(r, em, sx, threshold)
	if g.bm == nil {
		return &glyph{}
	}
	// Clip into the cell: rows [-ascent, h-ascent) around the baseline.
	cell := bitmap.New(w, h)
	cell.Draw(g.bm, g.dx, ascent+g.dy, bitmap.Black)
	return &glyph{bm: cell, dx: 0, dy: -ascent}
}

// Face returns the font magnified to approximate an h×w request, using
// integer magnification as the printer does.
func (s *BitmapSpec) Face(h, w int) Face {
	mh := Magnification(h, s.H)
	mw := mh
	if w > 0 {
		mw = Magnification(w, s.W)
	}
	if h <= 0 && w > 0 {
		mh = mw
	}
	return &bitmapFace{s: s, sx: float64(mw), sy: float64(mh)}
}

// ScaledFace returns the font scaled by arbitrary factors, for
// interpretation lines that the printer sizes freely.
func (s *BitmapSpec) ScaledFace(sx, sy float64) Face {
	return &bitmapFace{s: s, sx: sx, sy: sy}
}

type bitmapFace struct {
	s      *BitmapSpec
	sx, sy float64
}

func (f *bitmapFace) cell() (int, int) {
	return max(int(math.Round(float64(f.s.W)*f.sx)), 1), max(int(math.Round(float64(f.s.H)*f.sy)), 1)
}

func (f *bitmapFace) Ascent() int { return int(math.Round(float64(f.s.Ascent) * f.sy)) }
func (f *bitmapFace) Height() int { _, h := f.cell(); return h }

// Has reports whether the font's character set includes r; fallback fonts
// supply anything else.
func (f *bitmapFace) Has(r rune) bool {
	builtins()
	_, ok := mono.gid(f.fold(r))
	return ok || r == ' '
}

func (f *bitmapFace) Advance(r rune) int {
	w, _ := f.cell()
	return w + int(math.Round(float64(f.s.Gap)*f.sx))
}

func (f *bitmapFace) fold(r rune) rune {
	if f.s.UpperOnly {
		return unicode.ToUpper(r)
	}
	return r
}

func (f *bitmapFace) Draw(dst *bitmap.Bitmap, x, baseline int, r rune) {
	if !f.Has(r) {
		return // characters outside the font print as spaces
	}
	w, h := f.cell()
	g := f.s.glyph(f.fold(r), w, h)
	if g.bm != nil {
		dst.Draw(g.bm, x, baseline-f.Ascent(), bitmap.Black)
	}
}

// Downloaded is a bitmap font sent with ~DB.
type Downloaded struct {
	H, W    int // default cell size
	Ascent  int // baseline position from top
	Space   int // width of space character
	Glyphs  map[rune]DownloadedGlyph
	Default rune
}

// DownloadedGlyph is one ~DB character bitmap.
type DownloadedGlyph struct {
	Bm   *bitmap.Bitmap
	X, Y int // offsets of the bitmap from the cell origin
	Adv  int
}

// Face returns the downloaded font scaled by integer magnification.
func (d *Downloaded) Face(h, w int) Face {
	mh := Magnification(h, d.H)
	mw := mh
	if w > 0 {
		mw = Magnification(w, d.W)
	}
	return &downloadedFace{d: d, mx: mw, my: mh}
}

type downloadedFace struct {
	d      *Downloaded
	mx, my int
}

func (f *downloadedFace) Ascent() int { return f.d.Ascent * f.my }
func (f *downloadedFace) Height() int { return f.d.H * f.my }
func (f *downloadedFace) Has(r rune) bool {
	_, ok := f.d.Glyphs[r]
	return ok || r == ' '
}

func (f *downloadedFace) Advance(r rune) int {
	if g, ok := f.d.Glyphs[r]; ok {
		return g.Adv * f.mx
	}
	return f.d.Space * f.mx
}

func (f *downloadedFace) Draw(dst *bitmap.Bitmap, x, baseline int, r rune) {
	g, ok := f.d.Glyphs[r]
	if !ok || g.Bm == nil {
		return
	}
	top := baseline - f.d.Ascent*f.my
	dst.Draw(g.Bm.Scale(f.mx, f.my), x+g.X*f.mx, top+g.Y*f.my, bitmap.Black)
}
