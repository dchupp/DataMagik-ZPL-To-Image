package engine

import (
	"math"
	"math/rand"
	"testing"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/bitmap"
)

// TestSpanFillsMatchPixelTests checks the row-span hexagon and ring fills
// against the per-pixel definitions they replace.
func TestSpanFillsMatchPixelTests(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for n := 0; n < 2000; n++ {
		cx, cy := 5+r.Float64()*40, 5+r.Float64()*40
		rad := 0.3 + r.Float64()*25
		got, want := bitmap.New(60, 60), bitmap.New(60, 60)
		if n%2 == 0 {
			fillHexagon(got, cx, cy, rad)
			for y := 0; y < 60; y++ {
				for x := 0; x < 60; x++ {
					if hexInside(math.Abs(float64(x)+0.5-cx), math.Abs(float64(y)+0.5-cy), rad) {
						want.Set(x, y, true)
					}
				}
			}
		} else {
			rin := r.Float64() * rad
			fillRing(got, cx, cy, rin, rad)
			for y := 0; y < 60; y++ {
				for x := 0; x < 60; x++ {
					dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
					if d := math.Sqrt(dx*dx + dy*dy); d >= rin && d <= rad {
						want.Set(x, y, true)
					}
				}
			}
		}
		for y := 0; y < 60; y++ {
			for x := 0; x < 60; x++ {
				if got.Get(x, y) != want.Get(x, y) {
					t.Fatalf("case %d: pixel %d,%d differs", n, x, y)
				}
			}
		}
	}
}
