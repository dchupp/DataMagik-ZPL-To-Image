package engine

import (
	"errors"
	"fmt"
	"image"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/bitmap"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/lexer"
)

// byDefaults is the ^BY state.
type byDefaults struct {
	module int
	ratio  float64
	height int
}

// cfDefaults is the ^CF state.
type cfDefaults struct {
	font byte
	h, w int
}

// fwDefaults is the ^FW state.
type fwDefaults struct {
	rot  bitmap.Rotation
	just int
}

// fieldState is the format-level state a field is rendered with. It is
// copied into deferred (^FN template) fields.
type fieldState struct {
	by    byDefaults
	cf    cfDefaults
	fw    fwDefaults
	fePos byte // ^FE character, 0 when unset
}

// op is one composited drawing operation.
type op struct {
	mask     *bitmap.Bitmap  // nil for a rectangle fill
	rect     image.Rectangle // label coordinates, when mask is nil
	x, y     int
	mode     bitmap.Mode
	variable bool // ^FV field: not retained by ^MC N
}

// format renders one label.
type format struct {
	pr         *Printer
	global     bool
	copyIndex  int
	replicates int

	st     fieldState
	fld    *field
	ops    []op
	nextFT image.Point
	hasFT  bool

	// ^DF capture.
	capturing bool
	capName   string
	captured  []lexer.Command

	// ^FN handling.
	fnData    map[int][]byte
	templates []template

	opPixels int // total size of ops
	opsFull  bool

	fieldSeq int        // fields rendered so far
	memo     *fieldMemo // shared by the copies of a ^PQ run

	recallDepth int // current ^XF nesting
	recalled    int // commands replayed by ^XF in this format

	store      string // ^IS target
	storePrint bool
	noPrint    bool
	lastA      string // last ^A@ font name, carried over
}

// template is a deferred ^FN field without data.
type template struct {
	n      int
	render func(data []byte)
}

func (pr *Printer) newFormat(copyIndex, replicates int) *format {
	f := &format{
		pr:         pr,
		copyIndex:  copyIndex,
		replicates: replicates,
		fnData:     map[int][]byte{},
		storePrint: true,
	}
	f.st.by, f.st.cf, f.st.fw = pr.p.by, pr.p.cf, pr.p.fw
	f.fld = newField()
	return f
}

// dpmm is the effective density for sizing (halved by ^JM B).
func (f *format) dpmm() int {
	if f.pr.p.halfDense {
		return max(f.pr.opt.DPMM/2, 3)
	}
	return f.pr.opt.DPMM
}

// exec runs one command.
func (f *format) exec(c lexer.Command) error {
	if f.capturing {
		if c.Key() == "^FS" && len(f.captured) == 0 {
			return nil // ^DFname^FS
		}
		f.captured = append(f.captured, c)
		return nil
	}
	h, ok := handlers[c.Key()]
	if !ok {
		if f.pr.opt.Strict {
			return &UnknownCommandError{Command: c.Key(), Offset: c.Offset}
		}
		f.pr.warn(c, "unknown command ignored")
		return nil
	}
	return h(f, c, params(c.Split()))
}

// UnknownCommandError is returned in strict mode for unrecognised commands.
type UnknownCommandError struct {
	Command string
	Offset  int
}

func (e *UnknownCommandError) Error() string {
	return "engine: unknown command " + e.Command
}

// addOp records a drawing operation.
func (f *format) addOp(o op) {
	// Bound the work and memory of one format's pending fields: a mask
	// counts its size, a rectangle fill its full area (it costs compositing
	// time, though no memory). The area is not clipped to the label, whose
	// size can still change with a later ^PW/^LL, but capped at the largest
	// label bitmap, which is all one fill can ever touch.
	var area int
	if o.mask == nil {
		if o.rect.Empty() {
			return
		}
		area = min(o.rect.Dx()*o.rect.Dy(), bitmap.MaxPixels)
	} else {
		if o.mask.W == 0 || o.mask.H == 0 {
			return
		}
		area = o.mask.W * o.mask.H
	}
	if f.opPixels += area; f.opPixels > maxFormatPixels {
		if !f.opsFull {
			f.opsFull = true
			f.pr.warnings = append(f.pr.warnings, Warning{Message: "format too large: remaining fields dropped"})
		}
		return
	}
	f.ops = append(f.ops, o)
}

// apply composites the operation onto dst.
func (o op) apply(dst *bitmap.Bitmap) {
	if o.mask == nil {
		dst.FillMode(o.rect, o.mode)
		return
	}
	dst.Draw(o.mask, o.x, o.y, o.mode)
}

// maxFormatPixels bounds the total size of the field bitmaps one format may
// hold before compositing.
const maxFormatPixels = 2 * bitmap.MaxPixels

// ErrLabelTooLarge is returned when ^PW/^LL (or the options) describe a
// label larger than bitmap.MaxPixels.
var ErrLabelTooLarge = errors.New("engine: label larger than the supported maximum of 64 megapixels")

// finish ends the format at ^XZ and returns the label image, or nil when
// nothing is printed (^DF, ^IS with print N, global context).
func (f *format) finish() (*bitmap.Bitmap, error) {
	f.endField()
	if f.capturing {
		f.pr.st.put(f.capName, "ZPL", &object{format: f.captured})
		return nil, nil
	}
	for _, t := range f.templates {
		if d, ok := f.fnData[t.n]; ok {
			t.render(d)
		}
	}
	w, h := f.labelSize()
	if !bitmap.Fits(w, h) {
		return nil, fmt.Errorf("%w (%d×%d dots)", ErrLabelTooLarge, w, h)
	}
	scale := 1
	if f.pr.p.halfDense {
		scale = 2
		w, h = (w+1)/2, (h+1)/2
	}
	canvas := bitmap.New(w, h)
	// The ^MC N background keeps only ^FD fields, so it needs its own
	// canvas; it is only built when it will be used.
	var retained *bitmap.Bitmap
	if f.pr.p.keepImage {
		retained = bitmap.New(w, h)
	}
	if f.pr.bg != nil {
		canvas.Draw(f.pr.bg, 0, 0, bitmap.Black)
		if retained != nil {
			retained.Draw(f.pr.bg, 0, 0, bitmap.Black)
		}
	}
	for _, o := range f.ops {
		o.apply(canvas)
		if retained != nil && !o.variable {
			o.apply(retained)
		}
	}
	f.pr.bg = retained
	if f.store != "" {
		f.pr.st.put(f.store, "GRF", &object{graphic: canvas.Clone()})
	}
	if f.noPrint || (f.store != "" && !f.storePrint) {
		return nil, nil
	}
	out := canvas.Scale(scale, scale)
	if f.pr.p.mirror {
		out = out.MirrorX()
	}
	if f.pr.p.invert {
		out = out.Rotate(bitmap.R180)
	}
	return out, nil
}

// maxDim bounds the size of any single field in dots. Fields larger than
// the label cannot print anyway; the bound keeps hostile parameters (e.g. a
// 32000-dot font) from exhausting memory.
func (f *format) maxDim() int {
	w, h := f.labelSize()
	return 2 * max(w, h)
}

// fit clamps a per-unit size (module size, row height) so that units of it
// stay within maxDim.
func (f *format) fit(v, units int) int {
	return clamp(v, 1, max(f.maxDim()/max(units, 1), 1))
}

// labelSize returns the label size in dots.
func (f *format) labelSize() (int, int) {
	dotsPerInch := float64(f.pr.opt.DPMM) * 25.4
	w, h := f.pr.opt.Width, f.pr.opt.Height
	if w <= 0 {
		w = f.pr.p.pw
	}
	if h <= 0 {
		h = f.pr.p.ll
	}
	if w <= 0 {
		w = int(4*dotsPerInch + 0.5)
	}
	if h <= 0 {
		h = int(6*dotsPerInch + 0.5)
	}
	return w, h
}

// Limits on ^XF replay: stored formats may recall other formats, but a
// format that (directly or indirectly) recalls itself must not recurse
// forever or blow up exponentially.
const (
	maxRecallDepth    = 8
	maxRecallCommands = 100000
)

// recall replays a stored format (^XF).
func (f *format) recall(c lexer.Command, name string) error {
	o := f.pr.st.get(name, "ZPL")
	if o == nil || o.format == nil {
		f.pr.warn(c, "format %s not found", name)
		return nil
	}
	if f.recallDepth >= maxRecallDepth {
		f.pr.warn(c, "format %s not recalled: ^XF nested more than %d deep", name, maxRecallDepth)
		return nil
	}
	f.recallDepth++
	defer func() { f.recallDepth-- }()
	for _, rc := range o.format {
		if k := rc.Key(); k == "^XA" || k == "^XZ" {
			continue
		}
		if f.recalled >= maxRecallCommands {
			if f.recalled == maxRecallCommands {
				f.pr.warn(c, "format %s not fully recalled: more than %d recalled commands", name, maxRecallCommands)
				f.recalled++ // warn once
			}
			return nil
		}
		f.recalled++
		if err := f.exec(rc); err != nil {
			return err
		}
	}
	return nil
}
