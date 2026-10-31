package font

import (
	"math"
	"reflect"
	"sync"

	"github.com/go-text/typesetting/bidi"
	"github.com/go-text/typesetting/di"
	gotext "github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/bitmap"
)

// Chain draws each rune with the first face that has a glyph for it (^FL
// font links and fallback fonts). When no face has the glyph, DefGlyph
// selects the primary face's default glyph (usually a box) instead of a
// space, as ^PA a does.
type Chain struct {
	Faces    []Face
	DefGlyph bool
}

func (c *Chain) Ascent() int     { return c.Faces[0].Ascent() }
func (c *Chain) Height() int     { return c.Faces[0].Height() }
func (c *Chain) Has(r rune) bool { return c.pick(r) >= 0 }

func (c *Chain) pick(r rune) int {
	for i, f := range c.Faces {
		if f.Has(r) {
			return i
		}
	}
	return -1
}

func (c *Chain) Advance(r rune) int {
	if i := c.pick(r); i >= 0 {
		return c.Faces[i].Advance(r)
	}
	if c.DefGlyph {
		return c.Faces[0].Advance(r)
	}
	return c.Faces[0].Advance(' ')
}

func (c *Chain) Draw(dst *bitmap.Bitmap, x, baseline int, r rune) {
	if i := c.pick(r); i >= 0 {
		c.Faces[i].Draw(dst, x, baseline, r)
	} else if c.DefGlyph {
		c.Faces[0].Draw(dst, x, baseline, r)
	}
}

// LineOptions control single-line layout (^PA and ^FP).
type LineOptions struct {
	Gap      int  // extra dots after each character (^FP g)
	Bidi     bool // reorder right-to-left text (^PA b)
	Shape    bool // apply script shaping, e.g. Arabic joining (^PA c)
	Features bool // apply optional OpenType features: kerning, ligatures (^PA d)
}

// Line is laid-out text ready to draw.
type Line struct {
	Width int
	items []lineItem
}

type lineItem struct {
	x, dy int // position relative to the line origin and baseline
	g     *glyph
	face  Face
	r     rune
}

// Draw paints the line with its origin (left edge, baseline) at (x, baseline).
func (l *Line) Draw(dst *bitmap.Bitmap, x, baseline int) {
	for _, it := range l.items {
		if it.g != nil {
			it.g.draw(dst, x+it.x, baseline+it.dy)
		} else {
			it.face.Draw(dst, x+it.x, baseline+it.dy, it.r)
		}
	}
}

// Layout lays out one line of text. Bidi and shaping need scalable faces;
// with bitmap fonts the text is laid out character by character, as the
// printer does.
func Layout(face Face, text []rune, o LineOptions) *Line {
	if o.Bidi || o.Shape || o.Features {
		if faces, def := scalableFaces(face); faces != nil {
			return layoutComplex(faces, def, text, o)
		}
	}
	l := &Line{}
	for i, r := range text {
		l.items = append(l.items, lineItem{x: l.Width, face: face, r: r})
		l.Width += face.Advance(r)
		if i < len(text)-1 {
			l.Width += o.Gap
		}
	}
	return l
}

// scalableFaces returns the faces of a chain (or a single face) when all of
// them are scalable.
func scalableFaces(f Face) ([]*scalableFace, bool) {
	switch f := f.(type) {
	case *scalableFace:
		return []*scalableFace{f}, false
	case *Chain:
		var out []*scalableFace
		for _, sub := range f.Faces {
			inner, _ := scalableFaces(sub)
			if inner == nil {
				return nil, false
			}
			out = append(out, inner...)
		}
		return out, f.DefGlyph
	}
	return nil, false
}

var shaperPool = sync.Pool{New: func() any { return new(shaping.HarfbuzzShaper) }}

// segment is a run of text in one direction, script and face.
type segment struct {
	start, end int
	rtl        bool
	script     language.Script
	face       int
}

// layoutComplex applies the Unicode bidirectional algorithm (when enabled),
// splits the text by script and face, and shapes each run with HarfBuzz.
func layoutComplex(faces []*scalableFace, defGlyph bool, text []rune, o LineOptions) *Line {
	l := &Line{}
	if len(text) == 0 {
		return l
	}
	for _, seg := range visualSegments(faces, text, o.Bidi) {
		if o.Shape || o.Features {
			l.appendShaped(faces[seg.face], text, seg, o)
		} else {
			l.appendRunes(faces, defGlyph, text, seg, o)
		}
	}
	if l.Width > 0 {
		l.Width -= o.Gap
	}
	return l
}

// appendRunes places characters one by one in visual order (bidi only).
func (l *Line) appendRunes(faces []*scalableFace, defGlyph bool, text []rune, seg segment, o LineOptions) {
	f := faces[seg.face]
	for k := seg.start; k < seg.end; k++ {
		i := k
		if seg.rtl {
			i = seg.end - 1 - (k - seg.start)
		}
		r := text[i]
		if seg.rtl {
			r = mirror(r)
		}
		if !f.Has(r) && !defGlyph {
			l.Width += f.Advance(' ') + o.Gap
			continue
		}
		l.items = append(l.items, lineItem{x: l.Width, face: f, r: r})
		l.Width += f.Advance(r) + o.Gap
	}
}

// appendShaped shapes one segment with HarfBuzz and appends its glyphs.
func (l *Line) appendShaped(f *scalableFace, text []rune, seg segment, o LineOptions) {
	dir := di.DirectionLTR
	if seg.rtl {
		dir = di.DirectionRTL
	}
	in := shaping.Input{
		Text:      text,
		RunStart:  seg.start,
		RunEnd:    seg.end,
		Direction: dir,
		Face:      f.s.face,
		Size:      fixed.Int26_6(math.Round(f.em * 64)),
		Script:    seg.script,
		Language:  language.DefaultLanguage(),
	}
	if !o.Features {
		in.FontFeatures = []shaping.FontFeature{
			{Tag: ot.MustNewTag("kern"), Value: 0},
			{Tag: ot.MustNewTag("liga"), Value: 0},
		}
	}
	sh := shaperPool.Get().(*shaping.HarfbuzzShaper)
	f.s.mu.Lock()
	out := sh.Shape(in)
	f.s.mu.Unlock()
	shaperPool.Put(sh)

	pen := float64(l.Width)
	for _, g := range out.Glyphs {
		x := pen + float64(g.XOffset)/64*f.sx
		l.items = append(l.items, lineItem{
			x:  int(math.Round(x)),
			dy: -int(math.Round(float64(g.YOffset) / 64)),
			g:  f.glyphByID(g.GlyphID),
		})
		pen += float64(g.Advance) / 64 * f.sx
		if g.Advance != 0 {
			pen += float64(o.Gap)
		}
	}
	l.Width = int(math.Round(pen))
}

// visualSegments splits text into runs of one direction, script and face,
// in visual (left-to-right) order.
func visualSegments(faces []*scalableFace, text []rune, useBidi bool) []segment {
	type run struct {
		start, end int
		level      int
	}
	var runs []run
	if useBidi {
		levels := bidiLevels(text)
		for i := 0; i < len(levels); {
			j := i
			for j < len(levels) && levels[j] == levels[i] {
				j++
			}
			runs = append(runs, run{i, j, levels[i]})
			i = j
		}
	}
	if len(runs) == 0 {
		runs = []run{{0, len(text), 0}}
	}
	// Rule L2: from the highest level down to the lowest odd level,
	// reverse every sequence of runs at that level or higher.
	maxLevel, minOdd := 0, math.MaxInt
	for _, r := range runs {
		maxLevel = max(maxLevel, r.level)
		if r.level%2 == 1 {
			minOdd = min(minOdd, r.level)
		}
	}
	for lvl := maxLevel; lvl >= minOdd && lvl > 0; lvl-- {
		for i := 0; i < len(runs); {
			if runs[i].level < lvl {
				i++
				continue
			}
			j := i
			for j < len(runs) && runs[j].level >= lvl {
				j++
			}
			for a, b := i, j-1; a < b; a, b = a+1, b-1 {
				runs[a], runs[b] = runs[b], runs[a]
			}
			i = j
		}
	}

	var segs []segment
	for _, r := range runs {
		rtl := r.level%2 == 1
		var sub []segment
		for i := r.start; i < r.end; i++ {
			sc := language.LookupScript(text[i])
			fi := 0
			for k, f := range faces {
				if f.Has(text[i]) {
					fi = k
					break
				}
			}
			n := len(sub)
			if n > 0 {
				last := &sub[n-1]
				common := sc == language.Common || sc == language.Inherited
				if last.face == fi && (common || last.script == sc || last.script == language.Common) {
					if last.script == language.Common && !common {
						last.script = sc
					}
					last.end = i + 1
					continue
				}
			}
			sub = append(sub, segment{start: i, end: i + 1, rtl: rtl, script: sc, face: fi})
		}
		if rtl {
			for a, b := 0, len(sub)-1; a < b; a, b = a+1, b-1 {
				sub[a], sub[b] = sub[b], sub[a]
			}
		}
		segs = append(segs, sub...)
	}
	return segs
}

// bidiLevels returns the resolved embedding level of every character
// (UAX #9, left-to-right paragraph unless the text starts right-to-left).
//
// go-text's Runs merges adjacent runs of the same direction even when their
// levels differ (e.g. level-2 digits after Arabic followed by level-0
// Latin), which loses the information rule L2 needs. Its per-character
// levels are correct but unexported, so they are read with reflection; if
// that field ever disappears the run levels are used instead.
func bidiLevels(text []rune) []int {
	var p bidi.Paragraph
	out := p.Segment(text, bidi.LeftToRight)
	levels := make([]int, len(text))
	if v := reflect.ValueOf(out).FieldByName("levels"); v.IsValid() && v.Kind() == reflect.Slice && v.Len() >= len(text) {
		for i := range levels {
			if v.Index(i).CanInt() {
				levels[i] = int(v.Index(i).Int())
			} else {
				levels[i] = int(v.Index(i).Uint())
			}
		}
		return levels
	}
	for i := 0; i < out.NumRuns(); i++ {
		r := out.Run(i)
		for k := r.Start; k < r.End; k++ {
			levels[k] = int(r.Level)
		}
	}
	return levels
}

// mirror returns the mirrored form of paired punctuation in right-to-left
// runs (Unicode Bidi_Mirroring_Glyph for the common pairs).
func mirror(r rune) rune {
	switch r {
	case '(':
		return ')'
	case ')':
		return '('
	case '[':
		return ']'
	case ']':
		return '['
	case '{':
		return '}'
	case '}':
		return '{'
	case '<':
		return '>'
	case '>':
		return '<'
	case '«':
		return '»'
	case '»':
		return '«'
	}
	return r
}

// glyphByID returns a rendered glyph by glyph index, cached per face.
func (f *scalableFace) glyphByID(gid gotext.GID) *glyph {
	if f.gidCache == nil {
		f.gidCache = map[gotext.GID]*glyph{}
	}
	if g, ok := f.gidCache[gid]; ok {
		return g
	}
	g := f.s.renderGID(gid, f.em, f.sx, 0x80)
	f.gidCache[gid] = g
	return g
}
