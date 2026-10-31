package zpl

import (
	"image"
	"testing"
)

// zebraInk returns the ink bounding box of src rendered at 12 dots/mm (the
// ZD420 300 dpi the measurements below come from) and the ink box of row y
// alone (for bar widths without the interpretation line).
func zebraInk(t *testing.T, src string, row int) (box, rowBox image.Rectangle) {
	t.Helper()
	imgs := render(t, src, Options{DPMM: 12, LabelWidth: 4, LabelHeight: 2})
	img := imgs[0]
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if black(img, x, y) {
				box = box.Union(image.Rect(x, y, x+1, y+1))
				if y == row {
					rowBox = rowBox.Union(image.Rect(x, y, x+1, y+1))
				}
			}
		}
	}
	return box, rowBox
}

// TestZebraDivergences pins the sizes a ZD420 (300 dpi) draws for the
// fixtures testdata/printer/q16–q20, measured with the zebra harness.
func TestZebraDivergences(t *testing.T) {
	tests := []struct {
		name string
		zpl  string
		w, h int // ink box of row 50 (bars) when h == 0, else the whole ink box
	}{
		// Code 128 m=A: Start B, A, B, 1, Code C, 23, 45, 67, check, stop.
		{"code128 auto", "^XA^FO40,40^BY2,3.0,56^BCN,56,Y,N,N,A^FDAB1234567^FS^XZ", 224, 0},
		// Wide elements are floor(module × ratio): 2 dots at ^BY1,2.5.
		{"code39 ratio", "^XA^FO40,40^BY1,2.5,56^B3N,N,56,Y,N^FDAB12^FS^XZ", 77, 0},
		{"itf ratio", "^XA^FO40,40^BY1,2.5,56^B2N,56,Y,N,N^FD123456^FS^XZ", 50, 0},
		// Data Matrix without a module size: floor(^BY height / rows),
		// 40/14 = 2 dots.
		{"datamatrix default size", "^XA^FO40,40^BY3,3.0,40^BXN,,200,,,6,_,1^FDS2026101301^FS^XZ", 28, 28},
		{"pdf417", "^XA^FO40,40^BY2^B7N,6,,,,N^FDhttps://data-magik.com/label-designer?serial=S2026101301^FS^XZ", 206, 114},
	}
	for _, tc := range tests {
		box, row := zebraInk(t, tc.zpl, 50)
		if tc.h == 0 {
			if row.Dx() != tc.w {
				t.Errorf("%s: bars %d wide, want %d", tc.name, row.Dx(), tc.w)
			}
			continue
		}
		if box.Dx() != tc.w || box.Dy() != tc.h {
			t.Errorf("%s: ink %dx%d, want %dx%d", tc.name, box.Dx(), box.Dy(), tc.w, tc.h)
		}
	}
}
