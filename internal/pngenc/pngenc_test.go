package pngenc

import (
	"bytes"
	"image"
	"image/png"
	"math/rand"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for _, size := range [][2]int{{1, 1}, {7, 3}, {8, 2}, {9, 5}, {813, 1218}, {17, 0}} {
		w, h := size[0], size[1]
		pix := make([]byte, w*h)
		for i := range pix {
			pix[i] = byte(r.Intn(2))
		}
		var buf bytes.Buffer
		row := func(y int, dst []byte) {
			clear(dst)
			for x := range w {
				dst[x/8] |= pix[y*w+x] << (7 - x%8)
			}
		}
		if err := Encode(&buf, w, h, row, DefaultCompression); err != nil {
			t.Fatal(err)
		}
		if h == 0 {
			continue // zero-height PNGs are invalid; image/png rejects them too
		}
		img, err := png.Decode(&buf)
		if err != nil {
			t.Fatalf("%dx%d: %v", w, h, err)
		}
		if img.Bounds() != image.Rect(0, 0, w, h) {
			t.Fatalf("bounds %v", img.Bounds())
		}
		for y := range h {
			for x := range w {
				g, _, _, _ := img.At(x, y).RGBA()
				if (g < 0x8000) != (pix[y*w+x] != 0) {
					t.Fatalf("%dx%d: pixel %d,%d differs", w, h, x, y)
				}
			}
		}
	}
}
