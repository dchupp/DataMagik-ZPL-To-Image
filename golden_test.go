package zpl

import (
	"bytes"
	"flag"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden PNGs in testdata/golden")

// goldenDPMM reads the density from a "name@DPMM" fixture name (default 8).
func goldenDPMM(name string) int {
	if _, d, ok := strings.Cut(name, "@"); ok {
		if n, err := strconv.Atoi(d); err == nil {
			return n
		}
	}
	return 8
}

// goldenPath names label i of a golden file: name.png, name-2.png, …
func goldenPath(base string, i int) string {
	if i == 0 {
		return base + ".png"
	}
	return base + "-" + strconv.Itoa(i+1) + ".png"
}

// TestGolden renders every testdata/golden/*.zpl and compares the result
// pixel for pixel with the stored PNG (one PNG per label: name.png,
// name-2.png, …). Run with -update after intentional rendering changes.
func TestGolden(t *testing.T) {
	files, _ := filepath.Glob("testdata/golden/*.zpl")
	if len(files) == 0 {
		t.Fatal("no golden files")
	}
	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".zpl")
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			pngs, warnings, err := RenderPNG(bytes.NewReader(src), Options{Now: fixedNow, DPMM: goldenDPMM(name)})
			if err != nil {
				t.Fatal(err)
			}
			for _, w := range warnings {
				t.Log("warning:", w)
			}
			base := strings.TrimSuffix(file, ".zpl")
			if *update {
				old, _ := filepath.Glob(base + "-*.png")
				for _, o := range append(old, base+".png") {
					os.Remove(o)
				}
			}
			for i, got := range pngs {
				path := goldenPath(base, i)
				if *update {
					if err := os.WriteFile(path, got, 0o644); err != nil {
						t.Fatal(err)
					}
					continue
				}
				want, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("%v (run go test -update)", err)
				}
				a, errA := png.Decode(bytes.NewReader(got))
				b, errB := png.Decode(bytes.NewReader(want))
				if errA != nil || errB != nil {
					t.Fatalf("%s: decode: %v %v", path, errA, errB)
				}
				if !same(a, b) {
					t.Errorf("%s differs from golden image", path)
				}
			}
			// Every stored image must still be produced.
			if _, err := os.Stat(goldenPath(base, len(pngs))); err == nil && !*update {
				t.Errorf("%s: rendered %d labels, but %s exists", name, len(pngs), goldenPath(base, len(pngs)))
			}
		})
	}
}
