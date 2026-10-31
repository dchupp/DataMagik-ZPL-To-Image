// Package engine interprets ZPL II commands and renders labels.
package engine

import (
	"errors"
	"fmt"
	"time"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/bitmap"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/encoding"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/font"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/lexer"
)

// Options configure a Printer.
type Options struct {
	// DPMM is the print density in dots per millimetre: 6, 8, 12 or 24.
	DPMM int
	// Width and Height override the label size in dots. When zero, ^PW/^LL
	// are used, falling back to 4×6 inches.
	Width, Height int
	// Fonts maps font names (e.g. '0', 'A', 'Z') to TrueType/OpenType data
	// that replaces the built-in look-alikes.
	Fonts map[byte][]byte
	// Now returns the real-time clock value used by ^FC fields.
	Now func() time.Time
	// ExpandCopies renders one image per label copy requested by ^PQ, with
	// ^SN/^SF serialisation applied.
	ExpandCopies bool
	// Strict turns unknown commands into errors instead of warnings.
	Strict bool
	// FallbackFonts are TrueType/OpenType fonts used for characters that
	// no ZPL font has (e.g. CJK), tried in order.
	FallbackFonts [][]byte
}

// ErrInternal is returned when rendering hits an internal error (a bug);
// the input is rejected instead of crashing the caller.
var ErrInternal = errors.New("engine: internal error while rendering")

// MaxCopies caps the labels rendered for one format when expanding ^PQ.
const MaxCopies = 10000

// Warning describes a command that was accepted but could not be rendered
// exactly, or was not recognised.
type Warning struct {
	Offset  int
	Command string
	Message string
}

func (w Warning) String() string {
	return fmt.Sprintf("%s at byte %d: %s", w.Command, w.Offset, w.Message)
}

// persistent is printer state that survives across label formats, as on a
// real printer until it is changed or power-cycled.
type persistent struct {
	pw, ll    int
	lhX, lhY  int
	ls, lt    int
	invert    bool // ^PO I
	mirror    bool // ^PM Y
	reverse   bool // ^LR Y
	charset   encoding.Charset
	halfDense bool // ^JM B
	keepImage bool // ^MC N
	validate  bool // ^CV Y
	cwAlias   map[byte]string
	links     map[string][]string // ^FL base → extension fonts
	pa        advancedLayout      // ^PA
	encTable  map[uint16]rune     // ^SE table
	clockOff  time.Duration       // ^ST
	clockOffs [4]clockOffset      // ^SO per indicator (index 2, 3)
}

// advancedLayout is the ^PA state.
type advancedLayout struct {
	defGlyph bool // missing glyphs print the font's default glyph, not a space
	bidi     bool // bidirectional text layout
	shape    bool // character shaping
	features bool // OpenType features
}

type clockOffset struct {
	months, days, years     int
	hours, minutes, seconds int
}

// Printer is a virtual Zebra printer. It keeps stored objects (graphics,
// formats, fonts) and persistent settings between Run calls.
type Printer struct {
	opt       Options
	st        *storage
	p         persistent
	fontOver  map[byte]*font.Scalable
	fallbacks []*font.Scalable
	bg        *bitmap.Bitmap // retained image for ^MC N
	warnings  []Warning
}

// New returns a printer with power-up defaults.
func New(opt Options) (*Printer, error) {
	switch opt.DPMM {
	case 0:
		opt.DPMM = 8
	case 6, 8, 12, 24:
	default:
		return nil, fmt.Errorf("engine: unsupported density %d dots/mm", opt.DPMM)
	}
	if opt.Now == nil {
		opt.Now = time.Now
	}
	pr := &Printer{
		opt:      opt,
		st:       newStorage(),
		fontOver: map[byte]*font.Scalable{},
	}
	pr.p.cwAlias = map[byte]string{}
	pr.p.links = map[string][]string{}
	for i, data := range opt.FallbackFonts {
		s, err := font.ParseScalable(data)
		if err != nil {
			return nil, fmt.Errorf("engine: fallback font %d: %w", i, err)
		}
		pr.fallbacks = append(pr.fallbacks, s)
	}
	for name, data := range opt.Fonts {
		s, err := font.ParseScalable(data)
		if err != nil {
			return nil, fmt.Errorf("engine: font %q: %w", name, err)
		}
		pr.fontOver[upperByte(name)] = s
	}
	return pr, nil
}

func upperByte(c byte) byte {
	if c >= 'a' && c <= 'z' {
		return c - 'a' + 'A'
	}
	return c
}

func (pr *Printer) warn(c lexer.Command, format string, args ...any) {
	pr.warnings = append(pr.warnings, Warning{Offset: c.Offset, Command: c.Key(), Message: fmt.Sprintf(format, args...)})
}

// Run interprets a ZPL stream and passes each printed label to emit as
// soon as it is rendered, so callers can stream any number of labels. An
// error from emit stops processing and is returned.
func (pr *Printer) Run(src []byte, emit func(*bitmap.Bitmap) error) (warnings []Warning, err error) {
	// Defence in depth: hostile input must never crash the host process.
	defer func() {
		if r := recover(); r != nil {
			warnings, err = pr.warnings, fmt.Errorf("%w: %v", ErrInternal, r)
		}
	}()
	pr.warnings = nil
	cmds := lexer.All(src)
	for i := 0; i < len(cmds); i++ {
		c := cmds[i]
		if c.Key() != "^XA" {
			if err := pr.global(c); err != nil {
				return pr.warnings, err
			}
			continue
		}
		end := i + 1
		for end < len(cmds) && cmds[end].Key() != "^XZ" {
			end++
		}
		if end == len(cmds) {
			pr.warnings = append(pr.warnings, Warning{Offset: c.Offset, Command: "^XA", Message: "format not terminated by ^XZ"})
		}
		if err := pr.runFormat(cmds[i+1:end], emit); err != nil {
			return pr.warnings, err
		}
		i = end
	}
	return pr.warnings, nil
}

// global executes a command outside ^XA…^XZ. Download, storage and
// printer-setting commands work there; field commands are ignored.
func (pr *Printer) global(c lexer.Command) error {
	f := pr.newFormat(0, 0)
	f.global = true
	return f.exec(c)
}

// runFormat renders one ^XA…^XZ block, once per copy when expanding copies.
func (pr *Printer) runFormat(cmds []lexer.Command, emit func(*bitmap.Bitmap) error) error {
	copies, replicates := 1, 0
	if pr.opt.ExpandCopies {
		if c, ok := pr.findPQ(cmds, 0, map[string]bool{}); ok {
			ps := params(c.Split())
			copies = max(ps.int(0, 1), 1)
			replicates = max(ps.int(2, 0), 0)
			if copies > MaxCopies {
				pr.warn(c, "^PQ quantity %d capped at %d", copies, MaxCopies)
				copies = MaxCopies
			}
		}
	}
	var memo *fieldMemo
	if copies > 1 {
		memo = &fieldMemo{fields: map[fieldKey]fieldResult{}}
	}
	for k := range copies {
		f := pr.newFormat(k, replicates)
		f.memo = memo
		for _, c := range cmds {
			if err := f.exec(c); err != nil {
				return err
			}
		}
		img, err := f.finish()
		if err != nil {
			return err
		}
		if img != nil {
			if err := emit(img); err != nil {
				return err
			}
		}
	}
	return nil
}

// findPQ returns the last ^PQ in a format, including ^PQ commands inside
// formats it recalls with ^XF.
func (pr *Printer) findPQ(cmds []lexer.Command, depth int, seen map[string]bool) (lexer.Command, bool) {
	var found lexer.Command
	ok := false
	for _, c := range cmds {
		switch c.Key() {
		case "^PQ":
			found, ok = c, true
		case "^XF":
			name := pr.st.normalize(params(c.Split()).str(0), "ZPL")
			if depth >= maxRecallDepth || seen[name] {
				continue
			}
			seen[name] = true
			if o := pr.st.get(name, "ZPL"); o != nil {
				if pq, has := pr.findPQ(o.format, depth+1, seen); has {
					found, ok = pq, true
				}
			}
		}
	}
	return found, ok
}
