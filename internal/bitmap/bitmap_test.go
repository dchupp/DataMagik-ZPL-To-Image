package bitmap

import (
	"image"
	"math/rand"
	"testing"
)

// drawSlow is the reference per-pixel implementation of Draw.
func drawSlow(b, mask *Bitmap, x, y int, mode Mode) {
	for my := 0; my < mask.H; my++ {
		for mx := 0; mx < mask.W; mx++ {
			if mask.Pix[my*mask.W+mx] == 0 || !b.In(x+mx, y+my) {
				continue
			}
			i := (y+my)*b.W + x + mx
			switch mode {
			case Black:
				b.Pix[i] = 1
			case White:
				b.Pix[i] = 0
			case Reverse:
				b.Pix[i] ^= 1
			}
		}
	}
}

func random(r *rand.Rand, w, h int) *Bitmap {
	b := New(w, h)
	for i := range b.Pix {
		b.Pix[i] = byte(r.Intn(2))
	}
	return b
}

func TestDrawMatchesReference(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for n := 0; n < 2000; n++ {
		dst := random(r, 1+r.Intn(40), 1+r.Intn(40))
		mask := random(r, 1+r.Intn(30), 1+r.Intn(30))
		x, y := r.Intn(80)-40, r.Intn(80)-40
		mode := Mode(r.Intn(3))
		want := dst.Clone()
		drawSlow(want, mask, x, y, mode)
		dst.Draw(mask, x, y, mode)
		for i := range want.Pix {
			if want.Pix[i] != dst.Pix[i] {
				t.Fatalf("case %d: mismatch at %d (mode %d, pos %d,%d)", n, i, mode, x, y)
			}
		}
	}
}

func TestFillRect(t *testing.T) {
	b := New(5000, 3)
	b.FillRect(-10, 1, 4800, 5, true)
	if b.Get(0, 0) || !b.Get(0, 1) || !b.Get(4789, 2) || b.Get(4790, 2) {
		t.Error("FillRect clipping wrong")
	}
	b.FillRect(100, 0, 10, 3, false)
	if b.Get(105, 1) || !b.Get(99, 1) {
		t.Error("white FillRect wrong")
	}
}

func TestFillModeMatchesDraw(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	for n := 0; n < 1000; n++ {
		dst := random(r, 1+r.Intn(40), 1+r.Intn(40))
		x, y := r.Intn(60)-20, r.Intn(60)-20
		w, h := r.Intn(30), r.Intn(30)
		mode := Mode(r.Intn(3))
		mask := New(w, h)
		mask.FillRect(0, 0, w, h, true)
		want := dst.Clone()
		drawSlow(want, mask, x, y, mode)
		dst.FillMode(image.Rect(x, y, x+w, y+h), mode)
		for i := range want.Pix {
			if want.Pix[i] != dst.Pix[i] {
				t.Fatalf("case %d: mismatch (mode %d)", n, mode)
			}
		}
	}
}
