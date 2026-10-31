// Command zpl2png renders a ZPL II file to PNG images.
//
//	zpl2png [-dpmm 8] [-w 4 -h 6] [-copies] [-o label] file.zpl
//
// Each printed label is written to <prefix>-<n>.png. Use "-" to read stdin.
package main

import (
	"flag"
	"fmt"
	"image"
	"io"
	"os"

	zpl "github.com/datamagik/zpl-to-image"
)

func main() {
	dpmm := flag.Int("dpmm", 8, "print density in dots/mm (6, 8, 12, 24)")
	width := flag.Float64("w", 0, "label width in inches (default: ^PW or 4)")
	height := flag.Float64("h", 0, "label height in inches (default: ^LL or 6)")
	copies := flag.Bool("copies", false, "render every ^PQ copy with serialisation")
	strict := flag.Bool("strict", false, "fail on unknown commands")
	prefix := flag.String("o", "label", "output file prefix")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: zpl2png [flags] file.zpl")
		flag.PrintDefaults()
		os.Exit(2)
	}
	var in io.Reader = os.Stdin
	if name := flag.Arg(0); name != "-" {
		f, err := os.Open(name)
		if err != nil {
			fatal(err)
		}
		defer f.Close()
		in = f
	}
	printer, err := zpl.NewPrinter(zpl.Options{
		DPMM: *dpmm, LabelWidth: *width, LabelHeight: *height,
		ExpandCopies: *copies, Strict: *strict,
	})
	if err != nil {
		fatal(err)
	}
	// Stream labels to disk so large ^PQ runs need not fit in memory.
	n := 0
	warnings, err := printer.RenderEach(in, func(img image.Image) error {
		data, err := zpl.EncodePNG(img)
		if err != nil {
			return err
		}
		n++
		name := fmt.Sprintf("%s-%d.png", *prefix, n)
		if err := os.WriteFile(name, data, 0o644); err != nil {
			return err
		}
		fmt.Println(name)
		return nil
	})
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	if err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "zpl2png:", err)
	os.Exit(1)
}
