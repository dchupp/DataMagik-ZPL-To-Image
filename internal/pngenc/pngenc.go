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

// Compression levels (zlib): BestSpeed is about twice as fast as
// DefaultCompression for ~20% larger files on typical labels.
const (
	BestSpeed          = zlib.BestSpeed
	DefaultCompression = zlib.DefaultCompression
	BestCompression    = zlib.BestCompression
)

// zlibPools holds reusable zlib writers per compression level.
var zlibPools sync.Map // int -> *sync.Pool

func zlibWriter(level int, dst io.Writer) (*zlib.Writer, *sync.Pool, error) {
	pi, _ := zlibPools.LoadOrStore(level, &sync.Pool{})
	pool := pi.(*sync.Pool)
	if w, ok := pool.Get().(*zlib.Writer); ok {
		w.Reset(dst)
		return w, pool, nil
	}
	w, err := zlib.NewWriterLevel(dst, level)
	return w, pool, err
}

var rawPool = sync.Pool{New: func() any { return new([]byte) }}

// Encode writes a 1-bit grayscale PNG of a w×h image at the given zlib
// level. row fills its argument ((w+7)/8 bytes) with row y packed eight
// pixels to a byte, most significant bit first, 1 = black.
func Encode(dst io.Writer, w, h int, row func(y int, dst []byte), level int) error {
	var hdr [13]byte
	binary.BigEndian.PutUint32(hdr[0:], uint32(w))
	binary.BigEndian.PutUint32(hdr[4:], uint32(h))
	hdr[8] = 1 // bit depth
	hdr[9] = 0 // grayscale: 0 = black, 1 = white
	// hdr[10..12]: deflate, adaptive filtering, no interlace

	// Each row is a filter-type byte (0 = none) and the packed pixels.
	// Filtering does not help 1-bit images, and skipping it is fast.
	rowLen := 1 + (w+7)/8
	rawp := rawPool.Get().(*[]byte)
	defer rawPool.Put(rawp)
	if cap(*rawp) < rowLen*h {
		*rawp = make([]byte, rowLen*h)
	}
	raw := (*rawp)[:rowLen*h]
	for y := range h {
		r := raw[y*rowLen : (y+1)*rowLen]
		r[0] = 0 // filter: none
		r = r[1:]
		row(y, r)
		// PNG grayscale 0 is black: invert, keeping the padding bits white.
		for i := range r {
			r[i] = ^r[i]
		}
	}

	var z bytes.Buffer
	z.Grow(len(raw) / 8)
	zw, pool, err := zlibWriter(level, &z)
	if err != nil {
		return err
	}
	if _, err := zw.Write(raw); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	pool.Put(zw)

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
