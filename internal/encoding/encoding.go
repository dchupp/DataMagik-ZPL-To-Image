// Package encoding converts field data bytes to Unicode text according to the
// ^CI character set in effect, including ^CI character remapping.
package encoding

import (
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/encoding/unicode"
)

// Charset is a ^CI selection.
type Charset struct {
	// ID is the ^CI value (0–36).
	ID int
	// Remap holds ^CI byte remappings: input byte → output byte.
	Remap map[byte]byte
}

// national holds the ISO 646 style substitutions for ^CI 0–12 at the code
// points # $ @ [ \ ] ^ ` { | } ~.
var nationalPositions = [12]byte{0x23, 0x24, 0x40, 0x5B, 0x5C, 0x5D, 0x5E, 0x60, 0x7B, 0x7C, 0x7D, 0x7E}

var national = map[int][12]rune{
	0:  {'#', '$', '@', '[', '\\', ']', '^', '`', '{', '|', '}', '~'}, // USA 1
	1:  {'#', '$', '@', '[', '\\', ']', '^', '`', '{', '|', '}', '~'}, // USA 2
	2:  {'£', '$', '@', '[', '\\', ']', '^', '`', '{', '|', '}', '~'}, // UK
	3:  {'£', '$', '¾', 'ĳ', '½', '|', '^', '`', '¨', 'ƒ', '¼', '´'},  // Holland
	4:  {'#', '$', '@', 'Æ', 'Ø', 'Å', '^', '`', 'æ', 'ø', 'å', '~'},  // Denmark/Norway
	5:  {'#', '¤', 'É', 'Ä', 'Ö', 'Å', 'Ü', 'é', 'ä', 'ö', 'å', 'ü'},  // Sweden/Finland
	6:  {'#', '$', '§', 'Ä', 'Ö', 'Ü', '^', '`', 'ä', 'ö', 'ü', 'ß'},  // Germany
	7:  {'£', '$', 'à', '°', 'ç', '§', '^', '`', 'é', 'ù', 'è', '¨'},  // France 1
	8:  {'#', '$', 'à', 'â', 'ç', 'ê', 'î', 'ô', 'é', 'ù', 'è', 'û'},  // France 2
	9:  {'£', '$', '§', '°', 'ç', 'é', '^', 'ù', 'à', 'ò', 'è', 'ì'},  // Italy
	10: {'₧', '$', '@', '¡', 'Ñ', '¿', '^', '`', '¨', 'ñ', '}', '~'},  // Spain
	11: {'#', '$', '@', '[', '\\', ']', '^', '`', '{', '|', '}', '~'}, // Miscellaneous
	12: {'#', '$', '@', '[', '¥', ']', '^', '`', '{', '|', '}', '~'},  // Japan
}

func single(e *charmap.Charmap) encoding.Encoding { return e }

// tables maps ^CI values to x/text decoders. Values not listed fall back to
// code page 850 (the Zebra default upper half).
var tables = map[int]encoding.Encoding{
	13: single(charmap.CodePage850),
	14: unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM), // 16-bit Unicode / double-byte
	15: japanese.ShiftJIS,
	16: japanese.EUCJP,
	17: unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM), // UCS-2 big endian (deprecated)
	24: single(charmap.ISO8859_1),
	26: simplifiedchinese.GB18030,
	27: single(charmap.Windows1252),
	29: unicode.UTF16(unicode.BigEndian, unicode.UseBOM),
	30: unicode.UTF16(unicode.LittleEndian, unicode.UseBOM),
	31: single(charmap.Windows1250),
	33: single(charmap.Windows1251),
	34: single(charmap.Windows1253),
	35: single(charmap.Windows1254),
	36: single(charmap.Windows1255),
}

// Multibyte Asian encodings selectable by name for ^CI 26 / ^CI 14 callers
// that know their data's encoding.
var (
	ShiftJIS = japanese.ShiftJIS
	EUCJP    = japanese.EUCJP
	GB18030  = simplifiedchinese.GB18030
	Big5     = traditionalchinese.Big5
	EUCKR    = korean.EUCKR
)

// Decode converts field data to text.
func (c Charset) Decode(data []byte) string {
	if len(c.Remap) > 0 {
		data = append([]byte(nil), data...)
		for i, b := range data {
			if r, ok := c.Remap[b]; ok {
				data[i] = r
			}
		}
	}
	switch {
	case c.ID == 28:
		return decodeUTF8(data)
	case c.ID >= 0 && c.ID <= 12:
		return decodeNational(c.ID, data)
	case c.ID == 26:
		// Multibyte Asian with ASCII transparency: UTF-8 is common in
		// practice, otherwise assume GB18030 (a superset of GBK/GB2312).
		if utf8.Valid(data) {
			return string(data)
		}
	}
	enc, ok := tables[c.ID]
	if !ok {
		enc = charmap.CodePage850
	}
	out, err := enc.NewDecoder().Bytes(data)
	if err != nil {
		return decodeNational(0, data)
	}
	return string(out)
}

func decodeUTF8(data []byte) string {
	if utf8.Valid(data) {
		return string(data)
	}
	// Replace invalid bytes using CP850 like the printer's fallback glyphs.
	var out []rune
	for len(data) > 0 {
		r, n := utf8.DecodeRune(data)
		if r == utf8.RuneError && n == 1 {
			r = charmap.CodePage850.DecodeByte(data[0])
		}
		out = append(out, r)
		data = data[n:]
	}
	return string(out)
}

func decodeNational(id int, data []byte) string {
	subs := national[id]
	out := make([]rune, 0, len(data))
	for _, b := range data {
		r := rune(b)
		if b >= 0x80 {
			r = charmap.CodePage850.DecodeByte(b)
		} else {
			for i, p := range nationalPositions {
				if p == b {
					r = subs[i]
					break
				}
			}
		}
		out = append(out, r)
	}
	return string(out)
}

// IsMultiByte reports whether a ^CI value uses multi-byte input, which
// matters for ^FH hex escapes and barcode byte counts.
func IsMultiByte(id int) bool {
	switch id {
	case 14, 15, 16, 17, 26, 28, 29, 30:
		return true
	}
	return false
}
