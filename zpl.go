// Package zpl renders Zebra ZPL II label formats to images.
//
// A Printer models a Zebra printer: graphics, formats and fonts downloaded
// with ~DG, ^DF, ~DU and similar commands, and persistent settings such as
// ^CI, ^LH or ^PW, survive between calls, exactly as on a real printer.
// Render and RenderPNG are conveniences that use a fresh Printer.
//
//	images, warnings, err := zpl.Render(strings.NewReader("^XA^FO50,50^A0N,40^FDHello^FS^XZ"), zpl.Options{})
package zpl

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"time"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/bitmap"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/engine"
)

// Options configure rendering.
type Options struct {
	// DPMM is the print density in dots per millimetre: 6 (152 dpi),
	// 8 (203 dpi, the default), 12 (300 dpi) or 24 (600 dpi).
	DPMM int
	// LabelWidth and LabelHeight set the label size in inches. When zero,
	// ^PW and ^LL are used, falling back to 4×6 inches.
	LabelWidth, LabelHeight float64
	// Fonts replaces fonts by name ('0', 'A'–'Z', '1'–'9') with TrueType or
	// OpenType data, e.g. the real Zebra fonts if you are licensed for them.
	Fonts map[rune][]byte
	// Now supplies the real-time clock for ^FC fields. Defaults to time.Now.
	Now func() time.Time
	// ExpandCopies renders every copy requested by ^PQ, applying ^SN/^SF
	// serialisation. By default each format yields one image.
	ExpandCopies bool
	// Strict makes unknown commands an error instead of a warning.
	Strict bool
	// FallbackFonts are TrueType/OpenType fonts (TTF, OTF, TTC) used for
	// characters no ZPL font has, tried in order. Import
	// github.com/dchupp/DataMagik-ZPL-To-Image/fonts/notocjk for a CJK font.
	FallbackFonts [][]byte
	// MaxPixels limits the total size of the images Render and RenderPNG
	// return (default DefaultMaxPixels). RenderEach is not limited.
	MaxPixels int
}

// DefaultMaxPixels is the default Options.MaxPixels: 256 megapixels, about
// 260 4×6-inch labels at 8 dots/mm.
const DefaultMaxPixels = 256 << 20

var (
	// ErrTooMuchOutput is returned by Render and RenderPNG when the labels
	// exceed Options.MaxPixels.
	ErrTooMuchOutput = errors.New("zpl: rendered output exceeds Options.MaxPixels")
	// ErrLabelTooLarge is returned when a single label exceeds 64 megapixels.
	ErrLabelTooLarge = engine.ErrLabelTooLarge
)

// Warning reports something that was accepted but could not be rendered
// exactly, or an unrecognised command.
type Warning = engine.Warning

// Printer is a virtual Zebra printer with persistent memory.
type Printer struct {
	p         *engine.Printer
	maxPixels int
}

// NewPrinter returns a printer with power-up defaults.
func NewPrinter(o Options) (*Printer, error) {
	eo := engine.Options{
		DPMM:          o.DPMM,
		Now:           o.Now,
		ExpandCopies:  o.ExpandCopies,
		Strict:        o.Strict,
		FallbackFonts: o.FallbackFonts,
	}
	dpmm := o.DPMM
	if dpmm == 0 {
		dpmm = 8
	}
	eo.Width = int(o.LabelWidth*25.4*float64(dpmm) + 0.5)
	eo.Height = int(o.LabelHeight*25.4*float64(dpmm) + 0.5)
	if len(o.Fonts) > 0 {
		eo.Fonts = map[byte][]byte{}
		for k, v := range o.Fonts {
			eo.Fonts[byte(k)] = v
		}
	}
	p, err := engine.New(eo)
	if err != nil {
		return nil, err
	}
	maxPixels := o.MaxPixels
	if maxPixels <= 0 {
		maxPixels = DefaultMaxPixels
	}
	return &Printer{p: p, maxPixels: maxPixels}, nil
}

// Render interprets ZPL from r and returns one image per printed label.
// All images are held in memory, so the total output is limited to
// Options.MaxPixels; use RenderEach for unbounded output such as large ^PQ
// runs with ExpandCopies.
func (p *Printer) Render(r io.Reader) ([]image.Image, []Warning, error) {
	var out []image.Image
	total := 0
	warnings, err := p.RenderEach(r, func(img image.Image) error {
		b := img.Bounds()
		if total += b.Dx() * b.Dy(); total > p.maxPixels {
			return fmt.Errorf("%w: over %d pixels; use RenderEach to stream", ErrTooMuchOutput, p.maxPixels)
		}
		out = append(out, img)
		return nil
	})
	return out, warnings, err
}

// RenderEach interprets ZPL from r and calls fn with each label as soon as
// it is rendered. An error from fn stops rendering and is returned.
func (p *Printer) RenderEach(r io.Reader, fn func(image.Image) error) ([]Warning, error) {
	src, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return p.p.Run(src, func(b *bitmap.Bitmap) error { return fn(b.Image()) })
}

// RenderPNG is like Render but returns PNG-encoded images.
func (p *Printer) RenderPNG(r io.Reader) ([][]byte, []Warning, error) {
	imgs, warnings, err := p.Render(r)
	out := make([][]byte, 0, len(imgs))
	for _, img := range imgs {
		b, e := EncodePNG(img)
		if e != nil {
			return out, warnings, e
		}
		out = append(out, b)
	}
	return out, warnings, err
}

// EncodePNG encodes a rendered label as a compact 1-bit PNG.
func EncodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.DefaultCompression}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Render renders ZPL with a fresh printer.
func Render(r io.Reader, o Options) ([]image.Image, []Warning, error) {
	p, err := NewPrinter(o)
	if err != nil {
		return nil, nil, err
	}
	return p.Render(r)
}

// RenderPNG renders ZPL to PNG images with a fresh printer.
func RenderPNG(r io.Reader, o Options) ([][]byte, []Warning, error) {
	p, err := NewPrinter(o)
	if err != nil {
		return nil, nil, err
	}
	return p.RenderPNG(r)
}
