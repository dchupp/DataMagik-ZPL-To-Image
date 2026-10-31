package engine

import (
	"image"
	"path"
	"strings"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/bitmap"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/encoding"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/font"
)

// Default cell sizes (h, w) of fonts P–V at 8 dots/mm.
var pvSizes = map[byte][2]int{
	'P': {20, 18}, 'Q': {28, 24}, 'R': {35, 31}, 'S': {40, 35},
	'T': {48, 42}, 'U': {59, 53}, 'V': {80, 71},
}

// residentScalable names Zebra's resident TrueType files; they map to the
// font 0 look-alike unless the file has been downloaded.
func residentScalable(name string) bool {
	n := strings.ToUpper(name)
	return strings.Contains(n, "TT0003M_") || strings.Contains(n, "SWISS") ||
		strings.Contains(n, "0.FNT") || strings.HasSuffix(n, ".TTF") || strings.HasSuffix(n, ".TTE")
}

// resolveFace picks the face and rotation for a text field.
func (f *format) resolveFace(sel *fontSel, st fieldState) (font.Face, bitmap.Rotation) {
	name, objPath := st.cf.font, ""
	h, w := 0, 0
	rot := st.fw.rot
	if sel != nil {
		name, objPath, h, w = sel.name, sel.path, sel.h, sel.w
		if sel.rotSet {
			rot = sel.rot
		}
	}
	if h <= 0 && w <= 0 {
		h, w = st.cf.h, st.cf.w
	}
	return f.faceFor(name, objPath, h, w), rot
}

// faceFor returns the face for a font letter or stored object name.
func (f *format) faceFor(name byte, objPath string, h, w int) font.Face {
	pr := f.pr
	if objPath == "" {
		if alias, ok := pr.p.cwAlias[name]; ok {
			objPath = alias
		}
	}
	if objPath != "" {
		if face := f.objectFace(objPath, h, w); face != nil {
			return f.withLinks(face, objPath, h, w)
		}
		if !residentScalable(objPath) {
			pr.warnings = append(pr.warnings, Warning{Command: "^A@", Message: "font " + objPath + " not found; using font 0"})
		}
		return f.withLinks(scalableFace(font.Font0(), h, w), objPath, h, w)
	}
	if s, ok := pr.fontOver[name]; ok {
		return f.withLinks(scalableFace(s, h, w), string(name), h, w)
	}
	if spec := font.BitmapFont(name, f.dpmm()); spec != nil {
		face := spec.Face(h, w)
		return f.withFallbacks(face, face.Height(), face.Height())
	}
	if sz, ok := pvSizes[name]; ok {
		if h <= 0 && w <= 0 {
			h, w = sz[0], sz[1]
		}
		return f.withLinks(scalableFace(font.Font0(), h, w), "0", h, w)
	}
	if name != '0' {
		// Unknown fonts fall back to font A, as documented for ^CF.
		face := font.BitmapFont('A', f.dpmm()).Face(h, w)
		return f.withFallbacks(face, face.Height(), face.Height())
	}
	return f.withLinks(scalableFace(font.Font0(), h, w), "0", h, w)
}

func (f *format) objectFace(name string, h, w int) font.Face {
	o := f.pr.st.get(name, "FNT")
	if o == nil {
		for _, ext := range []string{"TTF", "TTE", "OTF"} {
			if o = f.pr.st.get(strings.TrimSuffix(name, ".FNT")+"."+ext, ext); o != nil {
				break
			}
		}
	}
	switch {
	case o == nil:
		return nil
	case o.scalable != nil:
		return scalableFace(o.scalable, h, w)
	case o.bitmapFt != nil:
		return o.bitmapFt.Face(h, w)
	}
	return nil
}

func scalableFace(s *font.Scalable, h, w int) font.Face {
	switch {
	case h <= 0 && w <= 0:
		h, w = 15, 15
	case h <= 0:
		h = w
	case w <= 0:
		w = h
	}
	return s.Face(h, w)
}

// withLinks adds ^FL linked fonts and the configured fallback fonts.
func (f *format) withLinks(primary font.Face, base string, h, w int) font.Face {
	chain := &font.Chain{Faces: []font.Face{primary}, DefGlyph: f.pr.p.pa.defGlyph}
	key := f.pr.st.normalize(base, "")
	for pat, exts := range f.pr.p.links {
		if ok, _ := path.Match(pat, key); ok || pat == key {
			for _, e := range exts {
				if face := f.objectFace(e, h, w); face != nil {
					chain.Faces = append(chain.Faces, face)
				}
			}
		}
	}
	return f.withFallbacks(chain, h, w)
}

// withFallbacks appends the Options.FallbackFonts (e.g. a CJK font) to a
// face so characters missing from every ZPL font still print.
func (f *format) withFallbacks(face font.Face, h, w int) font.Face {
	if len(f.pr.fallbacks) == 0 {
		return face
	}
	chain, ok := face.(*font.Chain)
	if !ok {
		chain = &font.Chain{Faces: []font.Face{face}, DefGlyph: f.pr.p.pa.defGlyph}
	}
	if h <= 0 && w <= 0 {
		h, w = face.Height(), face.Height()
	}
	for _, s := range f.pr.fallbacks {
		chain.Faces = append(chain.Faces, scalableFace(s, h, w))
	}
	return chain
}

// lineOptions returns the ^PA advanced-layout options for a field.
func (f *format) lineOptions(fld *field) font.LineOptions {
	pa := f.pr.p.pa
	return font.LineOptions{Gap: fld.gap, Bidi: pa.bidi, Shape: pa.shape, Features: pa.features}
}

// line is one laid-out line of text.
type line struct {
	runes []rune
	x     int // left offset within the block
	extra int // extra space per word gap (full justification)
}

func runeWidth(face font.Face, rs []rune, gap int) int {
	w := 0
	for _, r := range rs {
		w += face.Advance(r) + gap
	}
	if len(rs) > 0 {
		w -= gap
	}
	return w
}

// drawText renders a text field.
func (f *format) drawText(fld *field, st fieldState, data []byte) {
	face, rot := f.resolveFace(fld.font, st)
	if fld.tb != nil && fld.tb.rotSet {
		rot = fld.tb.rot
	}
	text := f.decodeText(data)
	switch {
	case fld.block != nil:
		f.drawBlock(fld, st, face, rot, fbEscapes(text))
	case fld.tb != nil:
		f.drawTB(fld, st, face, rot, text)
	default:
		f.drawLine(fld, st, face, rot, text)
	}
}

// decodeText converts field bytes to runes using ^CI, or the ^SE encoding
// table for multi-byte character sets.
func (f *format) decodeText(data []byte) []rune {
	p := f.pr.p
	if p.encTable == nil || !encoding.IsMultiByte(p.charset.ID) || p.charset.ID == 28 {
		return []rune(p.charset.Decode(data))
	}
	var out []rune
	for i := 0; i < len(data); i++ {
		if data[i] < 0x80 || i+1 >= len(data) {
			out = append(out, rune(data[i]))
			continue
		}
		code := uint16(data[i])<<8 | uint16(data[i+1])
		if r, ok := p.encTable[code]; ok {
			out = append(out, r)
		} else {
			out = append(out, rune(code))
		}
		i++
	}
	return out
}

// drawSymbol renders a ^GS field.
func (f *format) drawSymbol(fld *field, st fieldState, data []byte) {
	sel := fld.symbol
	h, w := sel.h, sel.w
	if h <= 0 && w <= 0 {
		h, w = st.cf.h, st.cf.w
	}
	if h <= 0 {
		h = w
	}
	if w <= 0 {
		w = h
	}
	rot := st.fw.rot
	if sel.rotSet {
		rot = sel.rot
	}
	f.drawLine(fld, st, font.SymbolFace(h, w), rot, []rune(string(data)))
}

// drawLine renders single-line text (including ^FP vertical/reverse).
func (f *format) drawLine(fld *field, st fieldState, face font.Face, rot bitmap.Rotation, text []rune) {
	if fld.dir == 'R' {
		text = reverseRunes(text)
	}
	h, asc := face.Height(), face.Ascent()
	pad := h/2 + 2
	if fld.dir == 'V' {
		cw := 0
		for _, r := range text {
			cw = max(cw, face.Advance(r))
		}
		step := h + fld.gap
		bh := max(len(text)*step-fld.gap, h)
		m := bitmap.New(cw+2*pad, bh+2*pad)
		for i, r := range text {
			face.Draw(m, pad, pad+i*step+asc, r)
		}
		s := shape{mask: m, box: image.Rect(pad, pad, pad+cw, pad+bh)}
		s.anchor = image.Pt(pad, pad+asc)
		s.ranchor = image.Pt(pad+cw, pad+asc)
		s.advance = image.Pt(0, len(text)*step)
		f.place(fld, st, s, rot)
		return
	}
	line := font.Layout(face, text, f.lineOptions(fld))
	width := line.Width
	m := bitmap.New(width+2*pad, h+2*pad)
	line.Draw(m, pad, pad+asc)
	s := shape{mask: m, box: image.Rect(pad, pad, pad+width, pad+h)}
	s.anchor = image.Pt(pad, pad+asc)
	s.ranchor = image.Pt(pad+width, pad+asc)
	s.advance = image.Pt(width+fld.gap, 0)
	f.place(fld, st, s, rot)
}

func reverseRunes(rs []rune) []rune {
	out := make([]rune, len(rs))
	for i, r := range rs {
		out[len(rs)-1-i] = r
	}
	return out
}

// fbEscapes applies ^FB's backslash codes: \& line break, \\ backslash,
// \(*) soft hyphen (printed as a hyphen).
func fbEscapes(rs []rune) []rune {
	var out []rune
	for i := 0; i < len(rs); i++ {
		if rs[i] != '\\' || i+1 >= len(rs) {
			out = append(out, rs[i])
			continue
		}
		switch rs[i+1] {
		case '&':
			out = append(out, '\n')
			i++
		case '\\':
			out = append(out, '\\')
			i++
		case '(':
			if i+3 < len(rs) && rs[i+3] == ')' {
				out = append(out, '-')
				i += 3
			} else {
				out = append(out, rs[i])
			}
		default:
			out = append(out, rs[i])
		}
	}
	return out
}

// wrap breaks text into lines no wider than width. hyphenate adds a hyphen
// when a word must be split. indent applies to every line after the first.
func wrap(measure func([]rune) int, text []rune, width, indent int, hyphenate bool) [][]rune {
	var lines [][]rune
	avail := func() int {
		if len(lines) > 0 {
			return width - indent
		}
		return width
	}
	for _, para := range strings.Split(string(text), "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			lines = append(lines, nil)
			continue
		}
		var cur []rune
		for _, word := range words {
			wr := []rune(word)
			cand := wr
			if len(cur) > 0 {
				cand = append(append(append([]rune{}, cur...), ' '), wr...)
			}
			if measure(cand) <= avail() {
				cur = cand
				continue
			}
			if len(cur) > 0 {
				lines = append(lines, cur)
				cur = nil
			}
			// Split words that cannot fit on a line of their own.
			for measure(wr) > avail() && len(wr) > 1 {
				n := len(wr) - 1
				for n > 1 {
					piece := wr[:n]
					if hyphenate {
						piece = append(append([]rune{}, wr[:n]...), '-')
					}
					if measure(piece) <= avail() {
						break
					}
					n--
				}
				piece := append([]rune{}, wr[:n]...)
				if hyphenate {
					piece = append(piece, '-')
				}
				lines = append(lines, piece)
				wr = wr[n:]
			}
			cur = wr
		}
		lines = append(lines, cur)
	}
	return lines
}

// drawBlock renders a ^FB text block.
func (f *format) drawBlock(fld *field, st fieldState, face font.Face, rot bitmap.Rotation, text []rune) {
	b := fld.block
	h, asc := face.Height(), face.Ascent()
	if b.width <= 0 {
		return // documented: no width, no print
	}
	if fld.dir == 'R' {
		text = reverseRunes(text)
	}
	measure := func(rs []rune) int { return runeWidth(face, rs, fld.gap) }
	lines := wrap(measure, text, b.width, b.indent, true)
	maxLines := max(b.lines, 1)
	step := h + b.spacing
	bh := max((maxLines-1)*step+h, 1)
	pad := h/2 + 2
	m := bitmap.New(b.width+2*pad, bh+2*pad)
	just := b.just
	for i, ln := range lines {
		row := min(i, maxLines-1) // excess lines overwrite the last line
		top := pad + row*step
		indent := 0
		if i > 0 {
			indent = b.indent
		}
		avail := b.width - indent
		lw := runeWidth(face, ln, fld.gap)
		x := indent
		extra, spaces := 0, 0
		switch just {
		case 'C':
			x += (avail - lw) / 2
		case 'R':
			x += avail - lw
		case 'J':
			lastOfPara := i == len(lines)-1 || len(lines[i+1]) == 0
			if !lastOfPara {
				for _, r := range ln {
					if r == ' ' {
						spaces++
					}
				}
				if spaces > 0 {
					extra = avail - lw
				}
			}
		}
		pen := pad + x
		used := 0
		for _, r := range ln {
			face.Draw(m, pen, top+asc, r)
			pen += face.Advance(r) + fld.gap
			if r == ' ' && spaces > 0 {
				add := extra * (used + 1) / spaces
				add -= extra * used / spaces
				pen += add
				used++
			}
		}
	}
	s := shape{mask: m, box: image.Rect(pad, pad, pad+b.width, pad+bh)}
	last := pad + (maxLines-1)*step + asc
	s.anchor = image.Pt(pad, last)
	s.ranchor = image.Pt(pad+b.width, last)
	f.place(fld, st, s, rot)
}

// drawTB renders a ^TB text block: word wrapped and truncated to the box.
func (f *format) drawTB(fld *field, st fieldState, face font.Face, rot bitmap.Rotation, text []rune) {
	tb := fld.tb
	h, asc := face.Height(), face.Ascent()
	width, height := max(tb.width, 1), max(tb.height, 1)
	// ^TB supports complex text: lines are measured and drawn with the
	// ^PA layout options (bidi, shaping).
	opts := f.lineOptions(fld)
	measure := func(rs []rune) int { return font.Layout(face, rs, opts).Width }
	lines := wrap(measure, text, width, 0, false)
	m := bitmap.New(width, height)
	pitch := font.TextBlockLineHeight(face)
	for i, ln := range lines {
		top := i * pitch
		if top+h > height {
			break
		}
		font.Layout(face, ln, opts).Draw(m, 0, top+asc)
	}
	s := shape{mask: m, box: image.Rect(0, 0, width, height)}
	s.anchor = image.Pt(0, asc)
	s.ranchor = image.Pt(width, asc)
	f.place(fld, st, s, rot)
}
