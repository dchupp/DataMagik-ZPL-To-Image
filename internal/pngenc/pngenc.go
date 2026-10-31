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

// pack8 packs eight 0/1 bytes (little-endian in v, first pixel lowest) into
// one byte with the first pixel in the most significant bit. Each input bit
// i is moved to bit 63-i by a distinct power of two, so the products never
// overlap or carry.
func pack8(v uint64) byte { return byte((v * 0x8040201008040201) >> 56) }

// Encode writes a 1-bit grayscale PNG of a w×h bitmap whose pixels are 0 or
// 1 bytes (1 = black), with stride bytes per row, at the given zlib level.
func Encode(dst io.Writer, pix []byte, w, h, stride, level int) error {
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
		row := raw[y*rowLen : (y+1)*rowLen]
		row[0] = 0 // filter: none
		row = row[1:]
		src := pix[y*stride : y*stride+w]
		x := 0
		for ; x+8 <= w; x += 8 {
			// Black (1) pixels become 0 bits; white become 1 bits.
			row[x/8] = ^pack8(binary.LittleEndian.Uint64(src[x:]))
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
