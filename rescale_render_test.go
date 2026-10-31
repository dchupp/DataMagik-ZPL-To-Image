package zpl

import (
	"bytes"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Render-comparison thresholds. A golden label rendered at its own density
// is compared with the same label rescaled and rendered at another density.
//
// Both images are reduced to a common physical grid of 0.5 mm cells (3, 4,
// 6 or 12 dots), each cell holding its fraction of black dots, the cells
// are box-blurred over 2.5 mm (±1 mm), and the two are compared with the
// weighted Jaccard index Σmin(a,b)/Σmax(a,b): 1 for identical coverage.
// The blur tolerates what the renderer itself does differently at each
// density (text stroke rasterisation, MaxiCode's whole-dot hexagon grid,
// the fixed 10-dot QR offset, ±1-dot rounding) while a field moved or
// resized by a few millimetres, or a label rendered without rescaling,
// loses most of its overlap: unscaled renders score at most about 0.25,
// and the test requires every rescaled render to beat its unscaled one
// by minRescaleGain.
//
// Measured (see the test log): rescales without warnings score 0.73–1.00,
// the lowest being 2d at 8→6, where 2D module sizes round up by 7–11%
// (inside the 15% Rescale tolerates silently); rescales Rescale reported as
// inexact (module widths or bitmap fonts that cannot match at the new
// density, mostly 8→6 and the bitmap-font fixtures) score 0.46–0.94 and
// are held to the lower bound only; unscaled renders score 0.00–0.23.
const (
	minRescaleIoU       = 0.70 // rescales without warnings
	minRescaleIoUWarned = 0.40 // rescales Rescale reported as inexact
	minRescaleGain      = 2.0  // rescaled score ÷ unscaled score
	rescaleCellsPerMM   = 2
	rescaleBlurCells    = 2
)

// coverage reduces a label to cells of cell×cell dots holding the fraction
// of black dots.
func coverage(img image.Image, cell int) ([]float64, int, int) {
	b := img.Bounds()
	cw, ch := b.Dx()/cell, b.Dy()/cell
	out := make([]float64, cw*ch)
	pal, _ := img.(image.PalettedImage)
	for y := 0; y < ch*cell; y++ {
		for x := 0; x < cw*cell; x++ {
			black := false
			if pal != nil {
				black = pal.ColorIndexAt(b.Min.X+x, b.Min.Y+y) == 1
			} else {
				r, _, _, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
				black = r < 0x8000
			}
			if black {
				out[(y/cell)*cw+x/cell]++
			}
		}
	}
	for i := range out {
		out[i] /= float64(cell * cell)
	}
	return out, cw, ch
}

// blurCells box-blurs a w×h grid over (2r+1)² cells.
func blurCells(c []float64, w, h, r int) []float64 {
	out := make([]float64, len(c))
	for y := range h {
		for x := range w {
			s := 0.0
			for yy := max(y-r, 0); yy <= min(y+r, h-1); yy++ {
				for xx := max(x-r, 0); xx <= min(x+r, w-1); xx++ {
					s += c[yy*w+xx]
				}
			}
			out[y*w+x] = s / float64((2*r+1)*(2*r+1))
		}
	}
	return out
}

// softIoU compares two labels of densities da and db on the common grid.
func softIoU(a image.Image, da int, b image.Image, db int) float64 {
	ca, aw, ah := coverage(a, da/rescaleCellsPerMM)
	cb, bw, bh := coverage(b, db/rescaleCellsPerMM)
	w, h := max(aw, bw), max(ah, bh)
	pad := func(c []float64, cw, ch int) []float64 {
		o := make([]float64, w*h)
		for y := range ch {
			copy(o[y*w:y*w+cw], c[y*cw:(y+1)*cw])
		}
		return blurCells(o, w, h, rescaleBlurCells)
	}
	pa, pb := pad(ca, aw, ah), pad(cb, bw, bh)
	var inter, union float64
	for i := range pa {
		inter += min(pa[i], pb[i])
		union += max(pa[i], pb[i])
	}
	if union == 0 {
		return 1
	}
	return inter / union
}

func renderAt(t *testing.T, src []byte, dpmm int) []image.Image {
	t.Helper()
	imgs, _, err := Render(bytes.NewReader(src), Options{DPMM: dpmm, Now: fixedNow})
	if err != nil {
		t.Fatal(err)
	}
	return imgs
}

// TestRescale_RenderMatches renders each golden label at its own density
// and, rescaled, at every other density, and compares them on a common
// physical grid.
func TestRescale_RenderMatches(t *testing.T) {
	if testing.Short() {
		t.Skip("renders every golden label at four densities")
	}
	files, _ := filepath.Glob("testdata/golden/*.zpl")
	if len(files) == 0 {
		t.Fatal("no golden files")
	}
	var report strings.Builder
	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".zpl")
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		from := goldenDPMM(name)
		want := renderAt(t, src, from)
		for _, to := range []int{6, 8, 12, 24} {
			if to == from {
				continue
			}
			t.Run(fmt.Sprintf("%s/%d→%d", name, from, to), func(t *testing.T) {
				out, warnings, err := Rescale(src, from, to)
				if err != nil {
					t.Fatal(err)
				}
				got := renderAt(t, out, to)
				raw := renderAt(t, src, to)
				if len(got) != len(want) || len(raw) != len(want) {
					t.Fatalf("%d and %d labels, want %d", len(got), len(raw), len(want))
				}
				limit := minRescaleIoU
				if len(warnings) > 0 {
					limit = minRescaleIoUWarned
				}
				for i := range want {
					iou := softIoU(want[i], from, got[i], to)
					base := softIoU(want[i], from, raw[i], to)
					fmt.Fprintf(&report, "%-16s %2d→%2d  rescaled %.3f  unscaled %.3f  warnings %d\n", name, from, to, iou, base, len(warnings))
					if iou < limit {
						t.Errorf("label %d: soft IoU %.3f < %.2f (warnings: %v)", i+1, iou, limit, warnings)
					}
					// The metric must tell a rescaled label from an unscaled one.
					if iou < minRescaleGain*base {
						t.Errorf("label %d: rescaled %.3f is not %.0f× the unscaled %.3f", i+1, iou, minRescaleGain, base)
					}
				}
			})
		}
	}
	t.Logf("soft IoU per label:\n%s", report.String())
}
