// Package font renders ZPL text: Zebra's bitmap fonts (A–H, GS), the
// scalable font 0 and P–V, and user TrueType/OpenType or downloaded bitmap
// fonts.
//
// Zebra's own font files are proprietary, so the built-in fonts are open
// look-alikes: Roboto Condensed Bold for font 0, and glyphs rasterised from
// Liberation Mono into each bitmap font's dot matrix (both SIL OFL 1.1).
package font

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"math"
	"sync"

	gotext "github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"golang.org/x/image/vector"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/bitmap"
)

//go:embed data/RobotoCondensed-Bold.ttf
var robotoCondensedBold []byte

//go:embed data/LiberationMono-Regular.ttf
var liberationMono []byte

// Face draws text at a fixed size.
type Face interface {
	// Ascent is the distance from the top of the character cell to the
	// baseline.
	Ascent() int
	// Height is the character cell height (line height without spacing).
	Height() int
	// Advance returns the horizontal advance of r.
	Advance(r rune) int
	// Has reports whether the face has a glyph for r.
	Has(r rune) bool
	// Draw paints r with its origin (left edge, baseline) at (x, baseline).
	Draw(dst *bitmap.Bitmap, x, baseline int, r rune)
}

// Glyph is a rendered glyph positioned relative to the pen.
type glyph struct {
	bm     *bitmap.Bitmap
	dx, dy int // top-left of bm relative to (pen x, baseline)
	adv    int
}

func (g *glyph) draw(dst *bitmap.Bitmap, x, baseline int) {
	if g.bm != nil {
		dst.Draw(g.bm, x+g.dx, baseline+g.dy, bitmap.Black)
	}
}

// Scalable is a parsed TrueType/OpenType font (TrueType or CFF outlines,
// including variable fonts at their default instance and the first font of
// a collection).
type Scalable struct {
	mu     sync.Mutex // guards face, which is not safe for concurrent use
	face   *gotext.Face
	upem   float64
	gids   cache[rune, gidEntry]
	glyphs cache[glyphCacheKey, *glyph] // rendered glyphs shared by all faces
}

type gidEntry struct {
	gid gotext.GID
	ok  bool
}

type glyphCacheKey struct {
	gid       gotext.GID
	em, sx    float64
	threshold uint8
}

// maxCachedGlyphs bounds each font's rendered-glyph cache, maxCachedGIDs
// its character-to-glyph cache.
const (
	maxCachedGlyphs = 8192
	maxCachedGIDs   = 65536
)

// ParseScalable parses TTF, OTF or TTC/OTC data.
func ParseScalable(data []byte) (*Scalable, error) {
	face, err := gotext.ParseTTF(bytes.NewReader(data))
	if err != nil {
		faces, cerr := gotext.ParseTTC(bytes.NewReader(data))
		if cerr != nil || len(faces) == 0 {
			return nil, fmt.Errorf("font: %w", err)
		}
		face = faces[0]
	}
	s := &Scalable{face: face, upem: float64(face.Upem())}
	s.gids.max = maxCachedGIDs
	return s, nil
}

// gid returns the glyph for r (0, the .notdef glyph, when missing).
func (s *Scalable) gid(r rune) (gotext.GID, bool) {
	if e, ok := s.gids.get(r); ok {
		return e.gid, e.ok
	}
	s.mu.Lock()
	gid, ok := s.face.NominalGlyph(r)
	s.mu.Unlock()
	s.gids.put(r, gidEntry{gid, ok})
	return gid, ok
}

var (
	builtinOnce sync.Once
	font0       *Scalable
	mono        *Scalable
)

func builtins() {
	builtinOnce.Do(func() {
		var err error
		if font0, err = ParseScalable(robotoCondensedBold); err != nil {
			panic(err)
		}
		if mono, err = ParseScalable(liberationMono); err != nil {
			panic(err)
		}
	})
}

// Font0 returns the built-in look-alike for Zebra font 0.
func Font0() *Scalable { builtins(); return font0 }

// Zebra font 0 calibration, measured against reference renders: capital
// letters are 0.75h tall and start at the top of the cell, so the baseline
// is 0.75h below the field origin. The em is sized to give that cap height
// with Roboto Condensed (cap height 0.711 em), and glyphs are narrowed to
// match CG Triumvirate Bold Condensed widths when w == h.
const (
	font0Em       = 1.055
	font0Baseline = 0.75
	font0WidthFit = 0.95
)

// Face returns s sized to an h-dot cell with w-dot nominal width
// (w == h gives the font's natural proportions).
func (s *Scalable) Face(h, w int) Face {
	h, w = max(h, 1), max(w, 1)
	em := float64(h) * font0Em
	return &scalableFace{
		s:      s,
		em:     em,
		sx:     float64(w) / float64(h) * font0WidthFit,
		h:      h,
		ascent: int(math.Round(float64(h) * font0Baseline)),
		cache:  map[rune]*glyph{},
	}
}

type scalableFace struct {
	s        *Scalable
	em       float64
	sx       float64
	h        int
	ascent   int
	cache    map[rune]*glyph
	gidCache map[gotext.GID]*glyph
}

func (f *scalableFace) Ascent() int { return f.ascent }
func (f *scalableFace) Height() int { return f.h }

func (f *scalableFace) Has(r rune) bool {
	_, ok := f.s.gid(r)
	return ok
}

func (f *scalableFace) Advance(r rune) int { return f.glyph(r).adv }

func (f *scalableFace) Draw(dst *bitmap.Bitmap, x, baseline int, r rune) {
	f.glyph(r).draw(dst, x, baseline)
}

func (f *scalableFace) glyph(r rune) *glyph {
	if g, ok := f.cache[r]; ok {
		return g
	}
	gid, _ := f.s.gid(r)
	g := f.s.renderGID(gid, f.em, f.sx, 0x80)
	f.cache[r] = g
	return g
}

// render rasterises r at an em of em pixels with horizontal scale sx.
func (s *Scalable) render(r rune, em, sx float64, threshold uint8) *glyph {
	gid, _ := s.gid(r)
	return s.renderGID(gid, em, sx, threshold)
}

// renderGID returns glyph gid rendered at an em of em pixels with
// horizontal scale sx, from the font's cache when possible. Rendered glyphs
// are immutable, so they are shared between faces, fields and labels.
func (s *Scalable) renderGID(gid gotext.GID, em, sx float64, threshold uint8) *glyph {
	k := glyphCacheKey{gid, em, sx, threshold}
	if g, ok := s.glyphs.get(k); ok {
		return g
	}
	g := s.rasterize(gid, em, sx, threshold)
	s.glyphs.put(k, g)
	return g
}

// rasterize renders glyph gid. Pixels whose coverage reaches threshold
// (0–255) are set.
func (s *Scalable) rasterize(gid gotext.GID, em, sx float64, threshold uint8) *glyph {
	s.mu.Lock()
	scale := em / s.upem
	adv := float64(s.face.HorizontalAdvance(gid)) * scale
	outline, ok := s.face.GlyphData(gid).(gotext.GlyphOutline)
	s.mu.Unlock()
	g := &glyph{adv: int(math.Round(adv * sx))}
	if !ok || len(outline.Segments) == 0 {
		return g
	}
	// Font units have Y up; the bitmap has Y down.
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, seg := range outline.Segments {
		for _, p := range seg.ArgsSlice() {
			x, y := float64(p.X)*scale*sx, -float64(p.Y)*scale
			minX, maxX = math.Min(minX, x), math.Max(maxX, x)
			minY, maxY = math.Min(minY, y), math.Max(maxY, y)
		}
	}
	x0, y0 := int(math.Floor(minX)), int(math.Floor(minY))
	w, h := int(math.Ceil(maxX))-x0, int(math.Ceil(maxY))-y0
	if w <= 0 || h <= 0 || !bitmap.Fits(w, h) {
		return g
	}
	ras := vector.NewRasterizer(w, h)
	pt := func(p ot.SegmentPoint) (float32, float32) {
		return float32(float64(p.X)*scale*sx - float64(x0)), float32(-float64(p.Y)*scale - float64(y0))
	}
	for _, seg := range outline.Segments {
		switch seg.Op {
		case ot.SegmentOpMoveTo:
			ras.MoveTo(pt(seg.Args[0]))
		case ot.SegmentOpLineTo:
			ras.LineTo(pt(seg.Args[0]))
		case ot.SegmentOpQuadTo:
			ax, ay := pt(seg.Args[0])
			bx, by := pt(seg.Args[1])
			ras.QuadTo(ax, ay, bx, by)
		case ot.SegmentOpCubeTo:
			ax, ay := pt(seg.Args[0])
			bx, by := pt(seg.Args[1])
			cx, cy := pt(seg.Args[2])
			ras.CubeTo(ax, ay, bx, by, cx, cy)
		}
	}
	ras.ClosePath()
	alpha := image.NewAlpha(image.Rect(0, 0, w, h))
	ras.Draw(alpha, alpha.Bounds(), image.Opaque, image.Point{})
	bm := bitmap.New(w, h)
	for i, a := range alpha.Pix {
		if a >= threshold {
			bm.Set(i%w, i/w, true)
		}
	}
	g.bm, g.dx, g.dy = bm, x0, y0
	return g
}
