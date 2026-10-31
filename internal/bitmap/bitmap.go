// Package bitmap is a 1-bit-per-pixel drawing surface used for labels and for
// the per-field masks that are composited onto them.
package bitmap

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
)

// Bitmap is a monochrome image. Pix holds one byte per pixel (0 white,
// 1 black) for simple, fast access.
type Bitmap struct {
	W, H int
	Pix  []byte
}

// MaxPixels is the largest bitmap New allocates (64 Mpx, e.g. a 24 dots/mm
// label of about 13×8.5 inches). Larger requests yield an empty bitmap, so
// hostile parameters cannot exhaust memory; callers that must not silently
// drop content (the label canvas) check Fits first.
const MaxPixels = 64 << 20

// Fits reports whether a w×h bitmap is within MaxPixels.
func Fits(w, h int) bool {
	return w >= 0 && h >= 0 && (w == 0 || h <= MaxPixels/w)
}

// New returns a white bitmap. Negative sizes are treated as zero, and
// sizes above MaxPixels give an empty bitmap.
func New(w, h int) *Bitmap {
	w, h = max(w, 0), max(h, 0)
	if !Fits(w, h) {
		return &Bitmap{}
	}
	return &Bitmap{W: w, H: h, Pix: make([]byte, w*h)}
}

// In reports whether (x, y) lies inside the bitmap.
func (b *Bitmap) In(x, y int) bool { return x >= 0 && y >= 0 && x < b.W && y < b.H }

// Get reports whether (x, y) is black. Out of range pixels are white.
func (b *Bitmap) Get(x, y int) bool { return b.In(x, y) && b.Pix[y*b.W+x] != 0 }

// Set paints (x, y) black or white; out of range writes are ignored.
func (b *Bitmap) Set(x, y int, black bool) {
	if b.In(x, y) {
		v := byte(0)
		if black {
			v = 1
		}
		b.Pix[y*b.W+x] = v
	}
}

// ones is a row of black pixels used to fill rows with copy.
var ones = bytes.Repeat([]byte{1}, 4096)

// fill sets every byte of row to 1.
func fill(row []byte) {
	for len(row) > 0 {
		row = row[copy(row, ones):]
	}
}

// FillRect paints a rectangle, clipped to the bitmap.
func (b *Bitmap) FillRect(x, y, w, h int, black bool) {
	x0, y0 := max(x, 0), max(y, 0)
	x1, y1 := min(x+w, b.W), min(y+h, b.H)
	if x0 >= x1 {
		return
	}
	for yy := y0; yy < y1; yy++ {
		row := b.Pix[yy*b.W+x0 : yy*b.W+x1]
		if black {
			fill(row)
		} else {
			clear(row)
		}
	}
}

// Mode selects how a mask is combined with a destination.
type Mode int

const (
	// Black paints mask pixels black.
	Black Mode = iota
	// White paints mask pixels white.
	White
	// Reverse inverts destination pixels under the mask (^FR, ^LR).
	Reverse
)

// Draw composites the black pixels of mask onto b with its top-left at
// (x, y). Pixels are 0/1 bytes, so the modes are byte-wise OR (Black),
// AND-NOT (White) and XOR (Reverse), done eight pixels at a time.
func (b *Bitmap) Draw(mask *Bitmap, x, y int, mode Mode) {
	// Clip the mask's column range once.
	mx0, mx1 := max(-x, 0), min(mask.W, b.W-x)
	if mx0 >= mx1 {
		return
	}
	for my := max(-y, 0); my < mask.H && y+my < b.H; my++ {
		src := mask.Pix[my*mask.W+mx0 : my*mask.W+mx1]
		off := (y+my)*b.W + x + mx0
		dst := b.Pix[off : off+len(src)]
		i := 0
		for ; i+8 <= len(src); i += 8 {
			sv := binary.LittleEndian.Uint64(src[i:])
			if sv == 0 {
				continue
			}
			dv := binary.LittleEndian.Uint64(dst[i:])
			switch mode {
			case Black:
				dv |= sv
			case White:
				dv &^= sv
			case Reverse:
				dv ^= sv
			}
			binary.LittleEndian.PutUint64(dst[i:], dv)
		}
		for ; i < len(src); i++ {
			switch mode {
			case Black:
				dst[i] |= src[i]
			case White:
				dst[i] &^= src[i]
			case Reverse:
				dst[i] ^= src[i]
			}
		}
	}
}

// Rotation is a clockwise rotation in quarter turns.
type Rotation int

const (
	R0   Rotation = iota // N: normal
	R90                  // R: rotated 90° clockwise
	R180                 // I: inverted
	R270                 // B: read from bottom up
)

// Rotate returns b rotated clockwise.
func (b *Bitmap) Rotate(r Rotation) *Bitmap {
	switch r & 3 {
	case R90:
		out := New(b.H, b.W)
		for y := 0; y < b.H; y++ {
			for x := 0; x < b.W; x++ {
				if b.Pix[y*b.W+x] != 0 {
					out.Pix[x*out.W+(b.H-1-y)] = 1
				}
			}
		}
		return out
	case R180:
		out := New(b.W, b.H)
		n := len(b.Pix)
		for i, v := range b.Pix {
			out.Pix[n-1-i] = v
		}
		return out
	case R270:
		out := New(b.H, b.W)
		for y := 0; y < b.H; y++ {
			for x := 0; x < b.W; x++ {
				if b.Pix[y*b.W+x] != 0 {
					out.Pix[(b.W-1-x)*out.W+y] = 1
				}
			}
		}
		return out
	}
	return b
}

// RotatePoint maps a point in b's coordinate space (as a continuous position,
// e.g. a baseline origin) to the rotated bitmap's coordinate space.
func RotatePoint(r Rotation, w, h, x, y int) (int, int) {
	switch r & 3 {
	case R90:
		return h - y, x
	case R180:
		return w - x, h - y
	case R270:
		return y, w - x
	}
	return x, y
}

// MirrorX returns b flipped horizontally.
func (b *Bitmap) MirrorX() *Bitmap {
	out := New(b.W, b.H)
	for y := 0; y < b.H; y++ {
		for x := 0; x < b.W; x++ {
			out.Pix[y*b.W+(b.W-1-x)] = b.Pix[y*b.W+x]
		}
	}
	return out
}

// Scale returns b magnified by integer factors (nearest neighbour).
func (b *Bitmap) Scale(sx, sy int) *Bitmap {
	sx, sy = max(sx, 1), max(sy, 1)
	if sx == 1 && sy == 1 {
		return b
	}
	out := New(b.W*sx, b.H*sy)
	for y := 0; y < out.H; y++ {
		src := b.Pix[(y/sy)*b.W:]
		dst := out.Pix[y*out.W:]
		for x := 0; x < out.W; x++ {
			dst[x] = src[x/sx]
		}
	}
	return out
}

// Clone returns a copy of b.
func (b *Bitmap) Clone() *Bitmap {
	return &Bitmap{W: b.W, H: b.H, Pix: append([]byte(nil), b.Pix...)}
}

// FromImage converts img to a bitmap: pixels with luminance below 50% and
// alpha above 50% become black.
func FromImage(img image.Image) *Bitmap {
	r := img.Bounds()
	out := New(r.Dx(), r.Dy())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			c := color.NRGBA64Model.Convert(img.At(x, y)).(color.NRGBA64)
			if c.A < 0x8000 {
				continue
			}
			lum := (19595*uint32(c.R) + 38470*uint32(c.G) + 7471*uint32(c.B)) >> 16
			if lum < 0x8000 {
				out.Pix[(y-r.Min.Y)*out.W+(x-r.Min.X)] = 1
			}
		}
	}
	return out
}

// Image returns b as a paletted image (index 0 white, 1 black), which PNG
// encodes as a compact 1-bit image. The image shares b's pixels, so b must
// not be modified afterwards.
func (b *Bitmap) Image() *image.Paletted {
	return &image.Paletted{
		Pix:     b.Pix,
		Stride:  b.W,
		Rect:    image.Rect(0, 0, b.W, b.H),
		Palette: color.Palette{color.White, color.Black},
	}
}
