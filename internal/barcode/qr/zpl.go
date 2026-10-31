package qr

import "github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"

// Field is the result of parsing ^BQ field data.
type Field struct {
	// ECLevel is the error correction level from the field data switch. It
	// overrides the ^BQ error correction parameter.
	ECLevel ECLevel
	// Manual is true for manual input ('M'): every segment then carries an
	// explicit character mode. Automatic input yields one ModeAuto segment.
	Manual bool
	// Segments holds the data to encode.
	Segments []Segment
	// StructuredAppend is set for mixed mode ("D" switch) fields.
	StructuredAppend *StructuredAppend
}

// ParseZPL parses ^BQ field data (Zebra ZPL II Programming Guide, ^BQ):
//
//	[D<ii><jj><xx>,]<ec><input>,<data>
//
// The optional mixed mode switch "Diijjxx," selects structured append: ii is
// the symbol number (01-16), jj the number of symbols (02-16) and xx the
// parity byte in hex. <ec> is H, Q, M or L (anything else means M) and
// <input> is A (automatic) or M (manual); the following character (normally
// a comma) is skipped. Like Zebra, these positions are consumed even when the
// data omits the switches, so "^FDhello" encodes "lo".
//
// Automatic input: the rest of the field is the data (commas included).
// Manual input: the rest is one or more comma separated segments, each
// starting with a character mode: N (digits), A (alphanumeric), Bnnnn
// (byte mode, nnnn = 4-digit byte count, then exactly nnnn bytes which may
// contain commas) or K (Shift-JIS Kanji pairs).
func ParseZPL(fieldData []byte) (Field, error) {
	var f Field
	d := fieldData
	if len(d) > 0 && d[0] == 'D' {
		if len(d) < 8 || d[7] != ',' {
			return f, invalid("mixed mode switch must be D<ii><jj><xx>,")
		}
		pos, ok1 := decimal(d[1:3])
		total, ok2 := decimal(d[3:5])
		parity, ok3 := hexByte(d[5:7])
		if !ok1 || !ok2 || !ok3 || total < 2 || total > 16 || pos < 1 || pos > total {
			return f, invalid("invalid mixed mode switch %q", d[:7])
		}
		f.StructuredAppend = &StructuredAppend{Position: pos, Total: total, Parity: parity}
		d = d[8:]
	}
	f.ECLevel = ECLevelM
	if len(d) > 0 {
		switch d[0] {
		case 'L':
			f.ECLevel = ECLevelL
		case 'Q':
			f.ECLevel = ECLevelQ
		case 'H':
			f.ECLevel = ECLevelH
		}
		d = d[1:]
	}
	if len(d) > 0 {
		f.Manual = d[0] == 'M'
		d = d[1:]
	}
	if len(d) > 0 {
		d = d[1:] // separator, normally ','
	}
	if !f.Manual {
		if len(d) == 0 {
			return f, invalid("no data")
		}
		f.Segments = []Segment{{Mode: ModeAuto, Data: append([]byte(nil), d...)}}
		return f, nil
	}
	for len(d) > 0 {
		var seg Segment
		mode := d[0]
		d = d[1:]
		switch mode {
		case 'N', 'A', 'K':
			end := 0
			for end < len(d) && d[end] != ',' {
				end++
			}
			seg.Mode = map[byte]Mode{'N': ModeNumeric, 'A': ModeAlphanumeric, 'K': ModeKanji}[mode]
			seg.Data = append([]byte(nil), d[:end]...)
			d = d[end:]
		case 'B':
			if len(d) < 4 {
				return f, invalid("byte mode needs a 4-digit count")
			}
			n, ok := decimal(d[:4])
			if !ok {
				return f, invalid("invalid byte count %q", d[:4])
			}
			d = d[4:]
			if n > len(d) {
				return f, invalid("byte count %d exceeds remaining data (%d bytes)", n, len(d))
			}
			seg.Mode = ModeByte
			seg.Data = append([]byte(nil), d[:n]...)
			d = d[n:]
		default:
			return f, invalid("unknown character mode %q", mode)
		}
		if err := validateSegment(seg); err != nil {
			return f, err
		}
		if len(seg.Data) > 0 {
			f.Segments = append(f.Segments, seg)
		}
		if len(d) > 0 && d[0] == ',' {
			d = d[1:]
		}
	}
	if len(f.Segments) == 0 {
		return f, invalid("no data")
	}
	return f, nil
}

func decimal(b []byte) (int, bool) {
	v := 0
	for _, c := range b {
		if c < '0' || c > '9' {
			return 0, false
		}
		v = v*10 + int(c-'0')
	}
	return v, len(b) > 0
}

func hexByte(b []byte) (byte, bool) {
	v := 0
	for _, c := range b {
		v <<= 4
		switch {
		case c >= '0' && c <= '9':
			v |= int(c - '0')
		case c >= 'A' && c <= 'F':
			v |= int(c-'A') + 10
		case c >= 'a' && c <= 'f':
			v |= int(c-'a') + 10
		default:
			return 0, false
		}
	}
	return byte(v), len(b) == 2
}

// EncodeZPL parses ^BQ field data and encodes it. opts supplies the model,
// mask and Kanji settings; its ECLevel and StructuredAppend are replaced by
// the values from the field data.
func EncodeZPL(fieldData []byte, opts Options) (barcode.Matrix, error) {
	f, err := ParseZPL(fieldData)
	if err != nil {
		return barcode.Matrix{}, err
	}
	opts.ECLevel = f.ECLevel
	opts.StructuredAppend = f.StructuredAppend
	return Encode(f.Segments, opts)
}
