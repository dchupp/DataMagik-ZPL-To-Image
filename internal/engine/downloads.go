package engine

import (
	"bytes"
	"image"
	"image/png"
	"strconv"
	"strings"

	"golang.org/x/image/bmp"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/bitmap"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/compress"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/font"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/lexer"
)

// splitHead splits off n delimiter-separated header parameters and returns
// them with the remaining data untouched.
func splitHead(c lexer.Command, n int) (params, string) {
	parts := strings.SplitN(string(c.Params), string(c.Delim), n+1)
	if len(parts) <= n {
		return params(parts), ""
	}
	return params(parts[:n]), parts[n]
}

// ~DGd:o.x,t,w,data
func cmdDG(f *format, c lexer.Command, _ params) error {
	ps, data := splitHead(c, 3)
	total, row := ps.int(1, 0), ps.int(2, 0)
	if row <= 0 {
		f.pr.warn(c, "missing bytes per row")
		return nil
	}
	raw, err := compress.DecodeGraphic(data, row, total)
	if err != nil {
		f.pr.warn(c, "%v", err)
	}
	f.pr.st.put(ps.str(0), "GRF", &object{graphic: rowsMask(raw, row)})
	return nil
}

// ~DYd:f,b,x,t,w,data
func cmdDY(f *format, c lexer.Command, _ params) error {
	ps, data := splitHead(c, 5)
	name := ps.str(0)
	format, ext := ps.char(1, 'A'), strings.ToUpper(ps.str(2))
	total, row := ps.int(3, 0), ps.int(4, 0)

	var raw []byte
	var err error
	switch format {
	case 'B':
		raw = []byte(data)
	case 'C':
		f.pr.warn(c, "AR-compressed (format C) downloads are proprietary; data stored as is")
		raw = []byte(data)
	default: // A (hex or ZB64) and P (ZB64 PNG)
		d := strings.TrimSpace(data)
		if strings.HasPrefix(d, ":B64:") || strings.HasPrefix(d, ":Z64:") {
			raw, err = compress.DecodeGraphic(d, 0, 0)
		} else if ext == "G" || ext == "GRF" || ext == "" {
			raw, err = compress.DecodeGraphic(d, row, total)
		} else {
			raw = compress.DecodeHex(d)
		}
	}
	if err != nil {
		f.pr.warn(c, "%v", err)
	}

	switch ext {
	case "P", "PNG":
		img, err := png.Decode(bytes.NewReader(raw))
		if err != nil {
			f.pr.warn(c, "PNG: %v", err)
			return nil
		}
		f.pr.st.put(withExt(name, "PNG"), "PNG", &object{graphic: bitmap.FromImage(img), raw: raw})
	case "B", "BMP":
		img, err := bmp.Decode(bytes.NewReader(raw))
		if err != nil {
			f.pr.warn(c, "BMP: %v", err)
			return nil
		}
		f.pr.st.put(withExt(name, "BMP"), "BMP", &object{graphic: bitmap.FromImage(img), raw: raw})
	case "X", "PCX":
		img, err := decodePCX(raw)
		if err != nil {
			f.pr.warn(c, "PCX: %v", err)
			return nil
		}
		f.pr.st.put(withExt(name, "PCX"), "PCX", &object{graphic: bitmap.FromImage(img), raw: raw})
	case "T", "E", "TTF", "TTE", "OTF":
		x := map[string]string{"T": "TTF", "E": "TTE"}[ext]
		if x == "" {
			x = ext
		}
		s, err := font.ParseScalable(raw)
		if err != nil {
			f.pr.warn(c, "%v", err)
			return nil
		}
		f.pr.st.put(withExt(name, x), x, &object{scalable: s, raw: raw})
	case "G", "GRF", "":
		if row <= 0 {
			f.pr.warn(c, "missing bytes per row")
			return nil
		}
		f.pr.st.put(withExt(name, "GRF"), "GRF", &object{graphic: rowsMask(raw, row)})
	default:
		f.pr.st.put(withExt(name, ext), ext, &object{raw: raw})
	}
	return nil
}

func withExt(name, ext string) string {
	if strings.Contains(name[strings.IndexByte(name, ':')+1:], ".") {
		return name
	}
	return name + "." + ext
}

// ~DUd:o.x,s,data and ~DTd:o.x,s,data: TrueType font as hex.
func cmdDU(f *format, c lexer.Command, _ params) error { return downloadTTF(f, c) }
func cmdDT(f *format, c lexer.Command, _ params) error { return downloadTTF(f, c) }

func downloadTTF(f *format, c lexer.Command) error {
	ps, data := splitHead(c, 2)
	raw := compress.DecodeHex(data)
	if d := strings.TrimSpace(data); strings.HasPrefix(d, ":B64:") || strings.HasPrefix(d, ":Z64:") {
		raw, _ = compress.DecodeGraphic(d, 0, 0)
	}
	s, err := font.ParseScalable(raw)
	if err != nil {
		f.pr.warn(c, "%v", err)
		f.pr.st.put(ps.str(0), "FNT", &object{raw: raw})
		return nil
	}
	f.pr.st.put(ps.str(0), "FNT", &object{scalable: s, raw: raw})
	return nil
}

// ~DSd:o.x,s,data: Intellifont download. The format is proprietary; fields
// using it fall back to font 0.
func cmdDS(f *format, c lexer.Command, _ params) error {
	ps, data := splitHead(c, 2)
	f.pr.st.put(ps.str(0), "FNT", &object{raw: compress.DecodeHex(data)})
	f.pr.warn(c, "Intellifont data cannot be rendered; font 0 will be used")
	return nil
}

// ~DEd:o.x,s,data: encoding table.
func cmdDE(f *format, c lexer.Command, _ params) error {
	ps, data := splitHead(c, 2)
	f.pr.st.put(ps.str(0), "DAT", &object{raw: compress.DecodeHex(data)})
	return nil
}

// parseDAT reads a .DAT table of (font code, input code) 16-bit pairs.
func parseDAT(raw []byte) map[uint16]rune {
	m := map[uint16]rune{}
	for i := 0; i+3 < len(raw); i += 4 {
		fontCode := uint16(raw[i])<<8 | uint16(raw[i+1])
		input := uint16(raw[i+2])<<8 | uint16(raw[i+3])
		m[input] = rune(fontCode)
	}
	return m
}

// ~DBd:o.x,a,h,w,base,space,#char,©,data
func cmdDB(f *format, c lexer.Command, _ params) error {
	ps, data := splitHead(c, 8)
	d := &font.Downloaded{
		H:      max(ps.int(2, 0), 1),
		W:      max(ps.int(3, 0), 1),
		Ascent: ps.int(4, 0),
		Space:  ps.int(5, 0),
		Glyphs: map[rune]font.DownloadedGlyph{},
	}
	for _, chunk := range strings.Split(data, "#")[1:] {
		fields := strings.SplitN(chunk, ".", 7)
		if len(fields) < 7 {
			continue
		}
		code, err := strconv.ParseUint(strings.TrimSpace(fields[0]), 16, 32)
		if err != nil {
			continue
		}
		num := func(i int) int { v, _ := strconv.Atoi(strings.TrimSpace(fields[i])); return v }
		h, w, x, y, adv := num(1), num(2), num(3), num(4), num(5)
		hex := strings.NewReplacer("O", "0", "o", "0").Replace(fields[6])
		bits := compress.DecodeHex(hex)
		rowBytes := (w + 7) / 8
		bm := bitmap.New(w, h)
		for yy := 0; yy < h; yy++ {
			for xx := 0; xx < w; xx++ {
				i := yy*rowBytes + xx/8
				if i < len(bits) && bits[i]&(0x80>>(xx%8)) != 0 {
					bm.Set(xx, yy, true)
				}
			}
		}
		// y is the offset from the baseline up to the bitmap top.
		d.Glyphs[rune(code)] = font.DownloadedGlyph{Bm: bm, X: x, Y: d.Ascent - y, Adv: adv}
	}
	f.pr.st.put(ps.str(0), "FNT", &object{bitmapFt: d})
	return nil
}

// decodePCX decodes 1-, 8- and 24-bit RLE PCX images.
func decodePCX(b []byte) (image.Image, error) {
	if len(b) < 128 || b[0] != 0x0A {
		return nil, errPCX
	}
	bpp := int(b[3])
	le := func(i int) int { return int(b[i]) | int(b[i+1])<<8 }
	w := le(8) - le(4) + 1
	h := le(10) - le(6) + 1
	planes := int(b[65])
	stride := le(66)
	if w <= 0 || h <= 0 || stride <= 0 || planes <= 0 {
		return nil, errPCX
	}
	lineLen := stride * planes
	buf := make([]byte, 0, lineLen*h)
	for i := 128; i < len(b) && len(buf) < lineLen*h; {
		v := b[i]
		i++
		n := 1
		if b[2] == 1 && v&0xC0 == 0xC0 {
			n = int(v & 0x3F)
			if i >= len(b) {
				break
			}
			v = b[i]
			i++
		}
		for range n {
			buf = append(buf, v)
		}
	}
	for len(buf) < lineLen*h {
		buf = append(buf, 0)
	}
	img := image.NewGray(image.Rect(0, 0, w, h))
	var pal []byte
	if bpp == 8 && planes == 1 && len(b) >= 769 && b[len(b)-769] == 0x0C {
		pal = b[len(b)-768:]
	}
	for y := 0; y < h; y++ {
		line := buf[y*lineLen:]
		for x := 0; x < w; x++ {
			var lum int
			switch {
			case bpp == 1 && planes == 1:
				if line[x/8]&(0x80>>(x%8)) != 0 {
					lum = 255
				}
			case bpp == 8 && planes == 1:
				v := int(line[x])
				if pal != nil {
					lum = (299*int(pal[v*3]) + 587*int(pal[v*3+1]) + 114*int(pal[v*3+2])) / 1000
				} else {
					lum = v
				}
			case bpp == 8 && planes >= 3:
				r, g, bl := line[x], line[stride+x], line[2*stride+x]
				lum = (299*int(r) + 587*int(g) + 114*int(bl)) / 1000
			default:
				return nil, errPCX
			}
			img.Pix[y*w+x] = byte(lum)
		}
	}
	return img, nil
}

type pcxError struct{}

func (pcxError) Error() string { return "unsupported or invalid PCX data" }

var errPCX error = pcxError{}
