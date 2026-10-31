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
			if !mask.Get(mx, my) || !b.In(x+mx, y+my) {
				continue
			}
			switch mode {
			case Black:
				b.Set(x+mx, y+my, true)
			case White:
				b.Set(x+mx, y+my, false)
			case Reverse:
				b.Set(x+mx, y+my, !b.Get(x+mx, y+my))
			}
		}
	}
}

func random(r *rand.Rand, w, h int) *Bitmap {
	b := New(w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			b.Set(x, y, r.Intn(2) == 1)
		}
	}
	return b
}

// same compares two bitmaps pixel by pixel and checks that the padding
// bits past W stay white.
func same(t *testing.T, want, got *Bitmap) bool {
	t.Helper()
	if want.W != got.W || want.H != got.H {
		t.Errorf("size %dx%d, want %dx%d", got.W, got.H, want.W, want.H)
		return false
	}
	for y := 0; y < want.H; y++ {
		for x := 0; x < want.W; x++ {
			if want.Get(x, y) != got.Get(x, y) {
				t.Errorf("pixel %d,%d differs", x, y)
				return false
			}
		}
		if r := got.W & 63; r != 0 && got.Pix[y*got.Stride+got.Stride-1]&^span(0, r) != 0 {
			t.Errorf("row %d: padding bits set", y)
			return false
		}
	}
	return true
}

func TestDrawMatchesReference(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for n := 0; n < 3000; n++ {
		dst := random(r, 1+r.Intn(200), 1+r.Intn(20))
		mask := random(r, 1+r.Intn(150), 1+r.Intn(20))
		x, y := r.Intn(400)-200, r.Intn(40)-20
		mode := Mode(r.Intn(3))
		want := dst.Clone()
		drawSlow(want, mask, x, y, mode)
		dst.Draw(mask, x, y, mode)
		if !same(t, want, dst) {
			t.Fatalf("case %d: mode %d, pos %d,%d", n, mode, x, y)
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
	for n := 0; n < 2000; n++ {
		dst := random(r, 1+r.Intn(200), 1+r.Intn(20))
		x, y := r.Intn(260)-30, r.Intn(40)-10
		w, h := r.Intn(150), r.Intn(20)
		mode := Mode(r.Intn(3))
		mask := New(w, h)
		mask.FillRect(0, 0, w, h, true)
		want := dst.Clone()
		drawSlow(want, mask, x, y, mode)
		dst.FillMode(image.Rect(x, y, x+w, y+h), mode)
		if !same(t, want, dst) {
			t.Fatalf("case %d: mode %d", n, mode)
		}
	}
}

func TestTransformsMatchReference(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	for n := 0; n < 300; n++ {
		b := random(r, 1+r.Intn(150), 1+r.Intn(70))
		for rot := R0; rot <= R270; rot++ {
			got := b.Rotate(rot)
			w, h := b.W, b.H
			if rot&1 == 1 {
				w, h = h, w
			}
			want := New(w, h)
			for y := 0; y < b.H; y++ {
				for x := 0; x < b.W; x++ {
					var X, Y int
					switch rot {
					case R0:
						X, Y = x, y
					case R90:
						X, Y = b.H-1-y, x
					case R180:
						X, Y = b.W-1-x, b.H-1-y
					case R270:
						X, Y = y, b.W-1-x
					}
					want.Set(X, Y, b.Get(x, y))
				}
			}
			if !same(t, want, got) {
				t.Fatalf("case %d rotation %d", n, rot)
			}
		}
		want := New(b.W, b.H)
		for y := 0; y < b.H; y++ {
			for x := 0; x < b.W; x++ {
				want.Set(b.W-1-x, y, b.Get(x, y))
			}
		}
		if !same(t, want, b.MirrorX()) {
			t.Fatalf("case %d mirror", n)
		}
		sx, sy := 1+r.Intn(4), 1+r.Intn(3)
		want = New(b.W*sx, b.H*sy)
		for y := 0; y < want.H; y++ {
			for x := 0; x < want.W; x++ {
				want.Set(x, y, b.Get(x/sx, y/sy))
			}
		}
		if !same(t, want, b.Scale(sx, sy)) {
			t.Fatalf("case %d scale %d,%d", n, sx, sy)
		}
	}
}

func TestFromRowsAndRowBytes(t *testing.T) {
	r := rand.New(rand.NewSource(4))
	for n := 0; n < 200; n++ {
		rowBytes, h := 1+r.Intn(20), r.Intn(10)
		data := make([]byte, rowBytes*h+r.Intn(rowBytes))
		r.Read(data)
		b := FromRows(data, rowBytes)
		if b.W != rowBytes*8 || b.H != h {
			t.Fatalf("size %dx%d", b.W, b.H)
		}
		row := make([]byte, rowBytes)
		for y := 0; y < h; y++ {
			for x := 0; x < b.W; x++ {
				if b.Get(x, y) != (data[y*rowBytes+x/8]&(0x80>>(x%8)) != 0) {
					t.Fatalf("pixel %d,%d", x, y)
				}
			}
			b.RowBytes(y, row)
			if string(row) != string(data[y*rowBytes:(y+1)*rowBytes]) {
				t.Fatalf("RowBytes row %d", y)
			}
		}
	}
	// RowBytes on widths that are not a multiple of 8.
	b := New(13, 1)
	b.Set(0, 0, true)
	b.Set(12, 0, true)
	row := make([]byte, 2)
	b.RowBytes(0, row)
	if row[0] != 0x80 || row[1] != 0x08 {
		t.Errorf("RowBytes = %08b %08b", row[0], row[1])
	}
}
