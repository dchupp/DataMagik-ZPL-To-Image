// Package bitmap is a 1-bit-per-pixel drawing surface used for labels and for
// the per-field masks that are composited onto them.
package bitmap

import (
	"encoding/binary"
	"image"
	"image/color"
	"math/bits"
)

// Bitmap is a monochrome image packed 64 pixels to a word. Pixel x of row y
// is bit 63-x%64 (most significant first) of Pix[y*Stride+x/64]; a set bit
// is black. Bits past W in each row's last word are always zero.
type Bitmap struct {
	W, H   int
	Stride int // words per row
	Pix    []uint64
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
	stride := (w + 63) / 64
	return &Bitmap{W: w, H: h, Stride: stride, Pix: make([]uint64, stride*h)}
}

// FromRows returns a bitmap from packed rows of rowBytes bytes each (most
// significant bit first, 1 = black), as in ^GF and ~DG data. Trailing bytes
// that do not fill a row are ignored.
func FromRows(data []byte, rowBytes int) *Bitmap {
	if rowBytes <= 0 {
		return New(0, 0)
	}
	h := len(data) / rowBytes
	b := New(rowBytes*8, h)
	if b.W == 0 {
		return b
	}
	var word [8]byte
	for y := 0; y < h; y++ {
		src := data[y*rowBytes : (y+1)*rowBytes]
		row := b.row(y)
		for i := range row {
			n := copy(word[:], src[min(i*8, len(src)):])
			clear(word[n:])
			row[i] = binary.BigEndian.Uint64(word[:])
		}
	}
	return b
}

func (b *Bitmap) row(y int) []uint64 { return b.Pix[y*b.Stride : (y+1)*b.Stride] }

// bit is the mask of pixel x within its word.
func bit(x int) uint64 { return 1 << (63 - uint(x)&63) }

// span is the mask of pixels a ≤ x < c within one word (0 ≤ a < c ≤ 64).
func span(a, c int) uint64 {
	m := ^uint64(0) >> uint(a)
	if c < 64 {
		m &^= ^uint64(0) >> uint(c)
	}
	return m
}

// In reports whether (x, y) lies inside the bitmap.
func (b *Bitmap) In(x, y int) bool { return x >= 0 && y >= 0 && x < b.W && y < b.H }

// Get reports whether (x, y) is black. Out of range pixels are white.
func (b *Bitmap) Get(x, y int) bool {
	return b.In(x, y) && b.Pix[y*b.Stride+x>>6]&bit(x) != 0
}

// Set paints (x, y) black or white; out of range writes are ignored.
func (b *Bitmap) Set(x, y int, black bool) {
	if !b.In(x, y) {
		return
	}
	i := y*b.Stride + x>>6
	if black {
		b.Pix[i] |= bit(x)
	} else {
		b.Pix[i] &^= bit(x)
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

func apply(dst *uint64, m uint64, mode Mode) {
	switch mode {
	case Black:
		*dst |= m
	case White:
		*dst &^= m
	case Reverse:
		*dst ^= m
	}
}

// applySpan applies mode to pixels x0 ≤ x < x1 of row.
func applySpan(row []uint64, x0, x1 int, mode Mode) {
	if x0 >= x1 {
		return
	}
	a, c := x0>>6, (x1-1)>>6
	if a == c {
		apply(&row[a], span(x0&63, (x1-1)&63+1), mode)
		return
	}
	apply(&row[a], span(x0&63, 64), mode)
	for i := a + 1; i < c; i++ {
		apply(&row[i], ^uint64(0), mode)
	}
	apply(&row[c], span(0, (x1-1)&63+1), mode)
}

// FillMode applies mode to every pixel of r (clipped): Black sets, White
// clears, Reverse inverts.
func (b *Bitmap) FillMode(r image.Rectangle, mode Mode) {
	r = r.Intersect(image.Rect(0, 0, b.W, b.H))
	if r.Empty() {
		return
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		applySpan(b.row(y), r.Min.X, r.Max.X, mode)
	}
}

// FillRect paints a rectangle, clipped to the bitmap.
func (b *Bitmap) FillRect(x, y, w, h int, black bool) {
	mode := White
	if black {
		mode = Black
	}
	b.FillMode(image.Rect(x, y, x+w, y+h), mode)
}

// extract returns the 64 pixels of row starting at pixel pos, with pixels
// outside the row white.
func extract(row []uint64, pos int) uint64 {
	if pos <= -64 || len(row) == 0 {
		return 0
	}
	if pos < 0 {
		return row[0] >> uint(-pos)
	}
	i, o := pos>>6, uint(pos&63)
	var v uint64
	if i < len(row) {
		v = row[i] << o
	}
	if o != 0 && i+1 < len(row) {
		v |= row[i+1] >> (64 - o)
	}
	return v
}

// Draw composites the black pixels of mask onto b with its top-left at
// (x, y): Black sets them (OR), White clears them (AND NOT), Reverse
// inverts the destination under them (XOR), 64 pixels at a time.
func (b *Bitmap) Draw(mask *Bitmap, x, y int, mode Mode) {
	x0, x1 := max(x, 0), min(x+mask.W, b.W)
	if x0 >= x1 {
		return
	}
	first, last := x0>>6, (x1-1)>>6
	lastMask := span(0, (x1-1)&63+1)
	for my := max(-y, 0); my < mask.H && y+my < b.H; my++ {
		src := mask.row(my)
		dst := b.row(y + my)
		for i := first; i <= last; i++ {
			s := extract(src, i*64-x)
			if i == last {
				s &= lastMask
			}
			if s != 0 {
				apply(&dst[i], s, mode)
			}
		}
	}
}

// eachBlack calls fn for every black pixel, row by row.
func (b *Bitmap) eachBlack(fn func(x, y int)) {
	for y := 0; y < b.H; y++ {
		for i, v := range b.row(y) {
			for v != 0 {
				p := bits.LeadingZeros64(v)
				fn(i*64+p, y)
				v &^= 1 << (63 - uint(p))
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
	var out *Bitmap
	switch r & 3 {
	case R90:
		out = New(b.H, b.W)
		b.eachBlack(func(x, y int) { out.Set(b.H-1-y, x, true) })
	case R180:
		out = New(b.W, b.H)
		b.eachBlack(func(x, y int) { out.Set(b.W-1-x, b.H-1-y, true) })
	case R270:
		out = New(b.H, b.W)
		b.eachBlack(func(x, y int) { out.Set(y, b.W-1-x, true) })
	default:
		return b
	}
	return out
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
	b.eachBlack(func(x, y int) { out.Set(b.W-1-x, y, true) })
	return out
}

// Scale returns b magnified by integer factors (nearest neighbour).
func (b *Bitmap) Scale(sx, sy int) *Bitmap {
	sx, sy = max(sx, 1), max(sy, 1)
	if sx == 1 && sy == 1 {
		return b
	}
	out := New(b.W*sx, b.H*sy)
	if out.W == 0 || out.H == 0 {
		return out
	}
	for y := 0; y < b.H; y++ {
		row := out.row(y * sy)
		for i, v := range b.row(y) {
			for v != 0 {
				p := bits.LeadingZeros64(v)
				// Extend a run of black pixels in one go.
				n := bits.LeadingZeros64(^(v << uint(p)))
				x := i*64 + p
				applySpan(row, x*sx, (x+n)*sx, Black)
				if p+n >= 64 {
					break
				}
				v &^= span(p, p+n)
			}
		}
		for k := 1; k < sy; k++ {
			copy(out.row(y*sy+k), row)
		}
	}
	return out
}

// Clone returns a copy of b.
func (b *Bitmap) Clone() *Bitmap {
	c := *b
	c.Pix = append([]uint64(nil), b.Pix...)
	return &c
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
				out.Set(x-r.Min.X, y-r.Min.Y, true)
			}
		}
	}
	return out
}

// RowBytes writes row y packed eight pixels to a byte (most significant bit
// first, 1 = black) into dst, which must hold (W+7)/8 bytes.
func (b *Bitmap) RowBytes(y int, dst []byte) {
	n := (b.W + 7) / 8
	var word [8]byte
	for i, v := range b.row(y) {
		if i*8+8 <= n {
			binary.BigEndian.PutUint64(dst[i*8:], v)
			continue
		}
		binary.BigEndian.PutUint64(word[:], v)
		copy(dst[i*8:n], word[:])
	}
}

// Palette is the palette of label images: index 0 white, 1 black.
var Palette = color.Palette{color.White, color.Black}

// Image is a read-only 1-bit image.PalettedImage backed by a Bitmap.
type Image struct{ b *Bitmap }

// Image returns b as an image sharing b's pixels, so b must not be
// modified afterwards.
func (b *Bitmap) Image() *Image { return &Image{b} }

// Bitmap returns the bitmap behind the image; it must not be modified.
func (m *Image) Bitmap() *Bitmap { return m.b }

// ColorModel returns Palette.
func (m *Image) ColorModel() color.Model { return Palette }

// Bounds returns the image rectangle, with its origin at (0, 0).
func (m *Image) Bounds() image.Rectangle { return image.Rect(0, 0, m.b.W, m.b.H) }

// At returns black or white.
func (m *Image) At(x, y int) color.Color { return Palette[m.ColorIndexAt(x, y)] }

// ColorIndexAt returns 1 for black pixels and 0 for white ones.
func (m *Image) ColorIndexAt(x, y int) uint8 {
	if m.b.Get(x, y) {
		return 1
	}
	return 0
}
