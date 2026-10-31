package zpl

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

// A 2x1-inch label at 8 dots/mm is 406x203 dots. A 10x10 box at (20,30)
// covers the dots (20..29, 30..39).
const rotationLabel = "^XA^FO20,30^GB10,10,10^FS^XZ"

func isBlack(img image.Image, x, y int) bool {
	g := color.GrayModel.Convert(img.At(x, y)).(color.Gray)
	return g.Y < 128
}

func TestOptionsRotation_Rotates(t *testing.T) {
	const w, h = 406, 203
	tests := []struct {
		rotation int
		wantW    int
		wantH    int
		blackX   int // a dot inside the rotated box
		blackY   int
		whiteX   int // the box's unrotated position, now white
		whiteY   int
	}{
		{0, w, h, 25, 35, h - 1 - 35, 25},
		{90, h, w, h - 1 - 35, 25, 25, 35},
		{180, w, h, w - 1 - 25, h - 1 - 35, 25, 35},
		{270, h, w, 35, w - 1 - 25, 25, 35},
	}
	for _, tc := range tests {
		imgs, _, err := Render(strings.NewReader(rotationLabel), Options{LabelWidth: 2, LabelHeight: 1, Rotation: tc.rotation})
		if err != nil {
			t.Fatalf("rotation %d: %v", tc.rotation, err)
		}
		b := imgs[0].Bounds()
		if b.Dx() != tc.wantW || b.Dy() != tc.wantH {
			t.Errorf("rotation %d: size %dx%d, want %dx%d", tc.rotation, b.Dx(), b.Dy(), tc.wantW, tc.wantH)
			continue
		}
		if !isBlack(imgs[0], tc.blackX, tc.blackY) {
			t.Errorf("rotation %d: (%d,%d) is white, want black", tc.rotation, tc.blackX, tc.blackY)
		}
		if isBlack(imgs[0], tc.whiteX, tc.whiteY) {
			t.Errorf("rotation %d: (%d,%d) is black, want white", tc.rotation, tc.whiteX, tc.whiteY)
		}
	}
}

func TestOptionsRotation_EveryCopyRotated(t *testing.T) {
	imgs, _, err := Render(strings.NewReader("^XA^PQ2^FO20,30^GB10,10,10^FS^XZ"), Options{LabelWidth: 2, LabelHeight: 1, Rotation: 90, ExpandCopies: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(imgs) != 2 {
		t.Fatalf("got %d images, want 2", len(imgs))
	}
	for i, img := range imgs {
		if b := img.Bounds(); b.Dx() != 203 || b.Dy() != 406 {
			t.Errorf("copy %d: size %dx%d, want 203x406", i, b.Dx(), b.Dy())
		}
	}
}

func TestOptionsRotation_PNG(t *testing.T) {
	pngs, _, err := RenderPNG(strings.NewReader(rotationLabel), Options{LabelWidth: 2, LabelHeight: 1, Rotation: 270})
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(pngs[0]))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 203 || b.Dy() != 406 {
		t.Errorf("size %dx%d, want 203x406", b.Dx(), b.Dy())
	}
}

func TestOptionsRotation_Invalid(t *testing.T) {
	for _, r := range []int{-90, 45, 360, 1} {
		if _, err := NewPrinter(Options{Rotation: r}); !errors.Is(err, ErrInvalidRotation) {
			t.Errorf("NewPrinter rotation %d: err = %v, want ErrInvalidRotation", r, err)
		}
		if _, _, err := Render(strings.NewReader(rotationLabel), Options{Rotation: r}); !errors.Is(err, ErrInvalidRotation) {
			t.Errorf("Render rotation %d: err = %v, want ErrInvalidRotation", r, err)
		}
	}
}
