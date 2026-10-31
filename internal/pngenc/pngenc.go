// Package pngenc writes 1-bit grayscale PNGs from label bitmaps faster than
// image/png, which converts each pixel through the generic color model.
package pngenc

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"hash/crc32"
	"io"
	"sync"
)

var signature = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

var zlibPool = sync.Pool{New: func() any {
	w, _ := zlib.NewWriterLevel(io.Discard, zlib.DefaultCompression)
	return w
}}

// Encode writes a 1-bit grayscale PNG of a w×h bitmap whose pixels are one
// byte each, with stride bytes per row (non-zero = black).
func Encode(dst io.Writer, pix []byte, w, h, stride int) error {
	var hdr [13]byte
	binary.BigEndian.PutUint32(hdr[0:], uint32(w))
	binary.BigEndian.PutUint32(hdr[4:], uint32(h))
	hdr[8] = 1 // bit depth
	hdr[9] = 0 // grayscale: 0 = black, 1 = white
	// hdr[10..12]: deflate, adaptive filtering, no interlace

	// Each row is a filter-type byte (0 = none) and the packed pixels.
	// Filtering does not help 1-bit images, and skipping it is fast.
	rowLen := 1 + (w+7)/8
	raw := make([]byte, rowLen*h)
	for y := range h {
		row := raw[y*rowLen+1 : (y+1)*rowLen]
		src := pix[y*stride : y*stride+w]
		x := 0
		for ; x+8 <= w; x += 8 {
			// Black (1) pixels become 0 bits; white become 1 bits.
			s := src[x : x+8]
			b := s[0]<<7 | s[1]<<6 | s[2]<<5 | s[3]<<4 | s[4]<<3 | s[5]<<2 | s[6]<<1 | s[7]
			row[x/8] = ^b
		}
		if x < w {
			var b byte
			for k := x; k < w; k++ {
				b |= src[k] << (7 - uint(k-x))
			}
			row[x/8] = ^b
		}
	}

	var z bytes.Buffer
	zw := zlibPool.Get().(*zlib.Writer)
	zw.Reset(&z)
	if _, err := zw.Write(raw); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	zlibPool.Put(zw)

	if _, err := dst.Write(signature); err != nil {
		return err
	}
	for _, c := range []struct {
		typ  string
		data []byte
	}{{"IHDR", hdr[:]}, {"IDAT", z.Bytes()}, {"IEND", nil}} {
		if err := chunk(dst, c.typ, c.data); err != nil {
			return err
		}
	}
	return nil
}

func chunk(w io.Writer, typ string, data []byte) error {
	var head [8]byte
	binary.BigEndian.PutUint32(head[:4], uint32(len(data)))
	copy(head[4:], typ)
	crc := crc32.NewIEEE()
	crc.Write(head[4:])
	crc.Write(data)
	var tail [4]byte
	binary.BigEndian.PutUint32(tail[:], crc.Sum32())
	for _, b := range [][]byte{head[:], data, tail[:]} {
		if _, err := w.Write(b); err != nil {
			return err
		}
	}
	return nil
}
