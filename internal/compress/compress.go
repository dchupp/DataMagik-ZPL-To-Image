// Package compress decodes ZPL graphic data: ASCII hex with Zebra's ACS
// run-length compression, and the :B64: and :Z64: base64 encodings used by
// ^GF, ~DG, ~DY and ~DU.
package compress

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// MaxBytes bounds the decoded size of any graphic or download (64 MiB, a
// 512-megapixel bitmap), protecting against huge row counts, ACS fill codes
// and zlib bombs.
const MaxBytes = 64 << 20

// ErrTooLarge is returned when decoded data would exceed MaxBytes.
var ErrTooLarge = errors.New("compress: decoded data too large")

// ErrCRC is returned when a :B64:/:Z64: payload's CRC does not match. The
// decoded data is still returned alongside it.
var ErrCRC = errors.New("compress: CRC mismatch")

// DecodeGraphic decodes ASCII graphic data (hex, ACS-compressed hex, :B64: or
// :Z64:) into raw bytes. rowBytes is the number of bytes per row, which ACS
// needs for the ',', '!' and ':' row codes; total is the expected byte count
// (the result is padded or truncated to it when total > 0).
func DecodeGraphic(data string, rowBytes, total int) ([]byte, error) {
	if rowBytes < 0 || rowBytes > MaxBytes || total > MaxBytes {
		return nil, ErrTooLarge
	}
	data = strings.TrimSpace(data)
	var out []byte
	var err error
	if p, ok := base64Payload(data); ok {
		out, err = p.decode()
	} else {
		out = DecodeACS(data, rowBytes)
	}
	if total > 0 {
		if len(out) < total {
			out = append(out, make([]byte, total-len(out))...)
		} else {
			out = out[:total]
		}
	}
	return out, err
}

type b64payload struct {
	zlib bool
	body string
	crc  string
}

func base64Payload(s string) (b64payload, bool) {
	var p b64payload
	switch {
	case strings.HasPrefix(s, ":Z64:"):
		p.zlib = true
	case strings.HasPrefix(s, ":B64:"):
	default:
		return p, false
	}
	s = s[5:]
	if i := strings.LastIndexByte(s, ':'); i >= 0 {
		p.body, p.crc = s[:i], s[i+1:]
	} else {
		p.body = s
	}
	return p, true
}

func (p b64payload) decode() ([]byte, error) {
	body := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, p.body)
	raw, err := base64.StdEncoding.DecodeString(body)
	if err != nil {
		raw, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(body, "="))
		if err != nil {
			return nil, fmt.Errorf("compress: base64: %w", err)
		}
	}
	var crcErr error
	if p.crc != "" {
		want, perr := strconv.ParseUint(strings.TrimSpace(p.crc), 16, 16)
		if perr == nil && uint16(want) != CRC16(body) {
			crcErr = ErrCRC
		}
	}
	if !p.zlib {
		return raw, crcErr
	}
	out, err := inflate(raw)
	if err != nil {
		return out, err
	}
	return out, crcErr
}

func inflate(raw []byte) ([]byte, error) {
	limited := func(r io.Reader) ([]byte, error) {
		out, err := io.ReadAll(io.LimitReader(r, MaxBytes+1))
		if len(out) > MaxBytes {
			return nil, ErrTooLarge
		}
		return out, err
	}
	if r, err := zlib.NewReader(bytes.NewReader(raw)); err == nil {
		out, err := limited(r)
		if err == ErrTooLarge {
			return nil, err
		}
		if err == nil || len(out) > 0 {
			return out, nil
		}
	}
	out, err := limited(flate.NewReader(bytes.NewReader(raw)))
	if err == ErrTooLarge {
		return nil, err
	}
	if err != nil && len(out) == 0 {
		return nil, fmt.Errorf("compress: inflate: %w", err)
	}
	return out, nil
}

// CRC16 computes the CRC-16/XMODEM (CCITT, poly 0x1021, init 0) of s, which
// Zebra appends to :B64: and :Z64: data, over the base64 text.
func CRC16(s string) uint16 {
	var crc uint16
	for i := 0; i < len(s); i++ {
		crc ^= uint16(s[i]) << 8
		for range 8 {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

// DecodeACS expands ASCII hex data with Zebra's Alternative Compression
// Scheme:
//
//	G..Y   repeat the following hex digit 1..19 times
//	g..z   repeat 20, 40, ... 400 times (combinable with G..Y, e.g. "hB")
//	,      fill the rest of the row with 0
//	!      fill the rest of the row with 1 (F)
//	:      repeat the previous row
//
// rowBytes is the row width in bytes; when it is 0 the row codes are ignored.
// Output stops at MaxBytes.
func DecodeACS(s string, rowBytes int) []byte {
	rowBytes = min(max(rowBytes, 0), MaxBytes)
	rowDigits := rowBytes * 2
	const maxNibbles = 2 * MaxBytes
	var nibbles []byte // completed nibbles
	var prevRow []byte
	rowStart := 0
	count := 0

	endRow := func(fill byte) {
		if rowDigits == 0 || rowStart+rowDigits > maxNibbles {
			return
		}
		for len(nibbles)-rowStart < rowDigits {
			nibbles = append(nibbles, fill)
		}
		prevRow = append(prevRow[:0], nibbles[rowStart:rowStart+rowDigits]...)
		rowStart += rowDigits
	}
	checkRow := func() {
		if rowDigits > 0 {
			for len(nibbles)-rowStart >= rowDigits {
				prevRow = append(prevRow[:0], nibbles[rowStart:rowStart+rowDigits]...)
				rowStart += rowDigits
			}
		}
	}

	for i := 0; i < len(s) && len(nibbles) < maxNibbles; i++ {
		c := s[i]
		switch {
		case c >= 'G' && c <= 'Y':
			count += int(c-'G') + 1
		case c >= 'g' && c <= 'z':
			count += (int(c-'g') + 1) * 20
		case isHex(c):
			n := min(max(count, 1), maxNibbles-len(nibbles))
			count = 0
			v := hexVal(c)
			for range n {
				nibbles = append(nibbles, v)
			}
			checkRow()
		case c == ',':
			count = 0
			endRow(0)
		case c == '!':
			count = 0
			endRow(0xF)
		case c == ':':
			count = 0
			if rowDigits > 0 && rowStart+rowDigits <= maxNibbles {
				// Discard any partial row and repeat the previous one.
				nibbles = nibbles[:rowStart]
				row := prevRow
				if row == nil {
					row = make([]byte, rowDigits)
				}
				nibbles = append(nibbles, row...)
				rowStart += rowDigits
			}
		}
	}
	out := make([]byte, (len(nibbles)+1)/2)
	for i, n := range nibbles {
		if i%2 == 0 {
			out[i/2] = n << 4
		} else {
			out[i/2] |= n
		}
	}
	return out
}

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'A' && c <= 'F' || c >= 'a' && c <= 'f'
}

func hexVal(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	default:
		return c - 'A' + 10
	}
}

// DecodeHex decodes plain hex digits, ignoring any other characters.
func DecodeHex(s string) []byte {
	out := make([]byte, 0, len(s)/2)
	var hi byte
	half := false
	for i := 0; i < len(s); i++ {
		if !isHex(s[i]) {
			continue
		}
		if half {
			out = append(out, hi<<4|hexVal(s[i]))
		} else {
			hi = hexVal(s[i])
		}
		half = !half
	}
	return out
}
