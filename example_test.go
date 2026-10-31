package zpl_test

import (
	"bytes"
	"fmt"
	"image"
	"strings"

	zpl "github.com/dchupp/DataMagik-ZPL-To-Image"
)

// Render a label to PNG.
func Example() {
	pngs, warnings, err := zpl.RenderPNG(strings.NewReader(`^XA
^FO50,50^A0N,50^FDHello, label^FS
^FO50,120^BY3^BCN,100,Y,N,N^FD123456789^FS
^XZ`), zpl.Options{DPMM: 8, LabelWidth: 4, LabelHeight: 2})
	if err != nil {
		panic(err)
	}
	fmt.Println(len(pngs), "label,", len(warnings), "warnings")
	// Output: 1 label, 0 warnings
}

// A Printer keeps its memory between calls, like a real printer: a logo
// downloaded once with ~DG and a template stored with ^DF are reused by
// later jobs. Keep one Printer per tenant or template set. A Printer is safe
// for concurrent use but runs one job at a time; use several Printers to
// render in parallel.
func ExamplePrinter() {
	p, err := zpl.NewPrinter(zpl.Options{LabelWidth: 4, LabelHeight: 2})
	if err != nil {
		panic(err)
	}
	// One-time setup: a graphic and a template with two variable fields.
	setup := `~DGR:LOGO.GRF,8,2,FFFFFFFFFFFFFFFF
^XA^DFR:SHIP.ZPL^FS
^FO20,20^XGR:LOGO.GRF,4,4^FS
^FO100,20^A0N,40^FN1^FS
^FO100,80^BY2^BCN,60,Y,N,N^FN2^FS
^XZ`
	if _, _, err := p.Render(strings.NewReader(setup)); err != nil {
		panic(err)
	}
	// Each job only sends the data.
	for _, order := range []string{"A-1001", "A-1002"} {
		job := fmt.Sprintf("^XA^XFR:SHIP.ZPL^FN1^FDOrder %s^FS^FN2^FD%s^FS^XZ", order, order)
		imgs, _, err := p.Render(strings.NewReader(job))
		if err != nil {
			panic(err)
		}
		fmt.Println(order, imgs[0].Bounds().Size())
	}
	// Output:
	// A-1001 (813,406)
	// A-1002 (813,406)
}

// RenderEach streams labels as they are produced, so a large ^PQ run with
// serial numbers does not have to fit in memory.
func ExamplePrinter_RenderEach() {
	p, _ := zpl.NewPrinter(zpl.Options{LabelWidth: 2, LabelHeight: 1, ExpandCopies: true})
	n := 0
	_, err := p.RenderEach(strings.NewReader("^XA^FO20,20^A0N,40^SN0001,1,Y^FS^PQ250^XZ"), func(img image.Image) error {
		data, err := zpl.EncodePNG(img)
		if err != nil {
			return err
		}
		n++
		_ = data // e.g. write to object storage or an HTTP response
		return nil
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(n, "labels")
	// Output: 250 labels
}

// Characters missing from the ZPL fonts (here Hebrew in a bitmap font is
// supported natively, but CJK is not) can be drawn from fallback fonts; ^PA
// turns on right-to-left layout and shaping for Arabic, Hebrew and Indic
// scripts. Import fonts/notocjk for a ready-made CJK fallback.
func ExampleOptions_fallbackFonts() {
	var cjkFont []byte // e.g. notocjk.Regular or the contents of a .ttf/.otf file
	opts := zpl.Options{LabelWidth: 4, LabelHeight: 1}
	if cjkFont != nil {
		opts.FallbackFonts = [][]byte{cjkFont}
	}
	_, warnings, err := zpl.Render(strings.NewReader("^XA^CI28^PA0,1,1,1^FO20,20^A0N,40^FD配送 Delivery^FS^XZ"), opts)
	fmt.Println(len(warnings), err)
	// Output: 0 <nil>
}

// Warnings report anything accepted but not rendered exactly, with the byte
// offset of the command.
func ExampleWarning() {
	_, warnings, _ := zpl.Render(bytes.NewReader([]byte("^XA^FO10,10^XGR:MISSING.GRF^FS^XZ")), zpl.Options{})
	for _, w := range warnings {
		fmt.Println(w)
	}
	// Output: ^XG at byte 11: image R:MISSING.GRF not found
}
