package verify

import (
	"bytes"
	"encoding/hex"
	"image"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/maxicode"
)

// gozxing has no MaxiCode reader, so these round trips use an external
// zxing-cpp decoder. Set MAXICODE_DECODER to a command taking "IMAGE.png
// MaxiCode" and printing "<format>\t<text repr>\t<bytes hex>..." (the
// zxing-cpp python wrapper output); the test is skipped otherwise.
// MAXICODE_PNG_DIR optionally keeps the rendered images.

// maxiImage renders m as hexagons of scale pixels across the flats plus the
// bullseye, with a quiet zone of one module.
func maxiImage(m [][]bool, scale float64) *image.Gray {
	q := scale
	w := int(math.Ceil(maxicode.SymbolWidth*scale + 2*q))
	h := int(math.Ceil(maxicode.SymbolHeight*scale + 2*q))
	img := blank(w, h)
	set := func(x, y int) { img.Pix[y*img.Stride+x] = 0 }
	hw, hh := scale/2, maxicode.HexHeight*scale/2
	for y, row := range m {
		for x, dark := range row {
			if !dark {
				continue
			}
			cx, cy := maxicode.ModuleCenter(x, y)
			cx, cy = cx*scale+q, cy*scale+q
			for py := int(cy - hh); py <= int(cy+hh)+1; py++ {
				for px := int(cx - hw); px <= int(cx+hw)+1; px++ {
					dx, dy := math.Abs(float64(px)+0.5-cx), math.Abs(float64(py)+0.5-cy)
					if dx <= hw && dy <= hh-dx/math.Sqrt(3) {
						set(px, py)
					}
				}
			}
		}
	}
	fx, fy := maxicode.FinderCenter()
	fx, fy = fx*scale+q, fy*scale+q
	rings := maxicode.FinderRings()
	for py := 0; py < h; py++ {
		for px := 0; px < w; px++ {
			d := 2 * math.Hypot(float64(px)+0.5-fx, float64(py)+0.5-fy) / scale
			for _, r := range rings {
				if d >= r[0] && d < r[1] {
					set(px, py)
				}
			}
		}
	}
	return img
}

type maxiCase struct {
	name string
	data string // UTF-8 literal; runes are Latin-1 bytes
	opts maxicode.Options
	want string // expected decoded bytes, as data
}

func maxiCases() []maxiCase {
	ups := "[)>\x1e01\x1d961Z00004951\x1dUPSN\x1d06X610\x1d159\x1d1234567\x1d1/1\x1d\x1dY\x1d634 ALPHA DR\x1dPITTSBURGH\x1dPA\x1e\x04"
	var allb strings.Builder
	for i := 1; i < 256; i++ {
		allb.WriteRune(rune(i))
	}
	all := []rune(allb.String())
	return []maxiCase{
		{"zebra-mode2", "001840152382802" + ups, maxicode.Options{Mode: 2},
			"[)>\x1e01\x1d96152382802\x1d840\x1d001\x1d1Z00004951\x1dUPSN\x1d06X610\x1d159\x1d1234567\x1d1/1\x1d\x1dY\x1d634 ALPHA DR\x1dPITTSBURGH\x1dPA\x1e\x04"},
		{"zebra-mode2-plain", "003840100010000HELLO world", maxicode.Options{Mode: 2},
			"100010000\x1d840\x1d003\x1dHELLO world"},
		{"zebra-mode3", "999056B1050 CEN", maxicode.Options{Mode: 3}, "B1050 \x1d056\x1d999\x1dCEN"},
		{"zebra-mode3-ups", "066826W1A1AA" + ups, maxicode.Options{Mode: 3},
			"[)>\x1e01\x1d96W1A1AA\x1d826\x1d066\x1d1Z00004951\x1dUPSN\x1d06X610\x1d159\x1d1234567\x1d1/1\x1d\x1dY\x1d634 ALPHA DR\x1dPITTSBURGH\x1dPA\x1e\x04"},
		{"scm-layout", "[)>\x1e01\x1d96123456789\x1d840\x1d001\x1d1Z12345\x1dUPSN\x1e\x04", maxicode.Options{Mode: 2},
			"[)>\x1e01\x1d96123456789\x1d840\x1d001\x1d1Z12345\x1dUPSN\x1e\x04"},
		{"mode4", "THIS IS A 93 CHARACTER CODE SET A MESSAGE THAT FILLS A MODE 4, UNAPPENDED, MAXICODE SYMBOL...", maxicode.Options{Mode: 4},
			"THIS IS A 93 CHARACTER CODE SET A MESSAGE THAT FILLS A MODE 4, UNAPPENDED, MAXICODE SYMBOL..."},
		{"mode4-mixed", "MaxiCode (19 chars) 123456789012 abc", maxicode.Options{Mode: 4}, "MaxiCode (19 chars) 123456789012 abc"},
		{"mode4-digits", strings.Repeat("0123456789", 13), maxicode.Options{Mode: 4}, strings.Repeat("0123456789", 13)},
		{"mode5", "Enhanced error correction mode 5 ÀÉÎõü", maxicode.Options{Mode: 5}, "Enhanced error correction mode 5 ÀÉÎõü"},
		{"mode4-latin1-a", string(all[:60]), maxicode.Options{Mode: 4}, string(all[:60])},
		{"mode4-latin1-b", string(all[60:120]), maxicode.Options{Mode: 4}, string(all[60:120])},
		{"mode4-latin1-c", string(all[120:170]), maxicode.Options{Mode: 4}, string(all[120:170])},
		{"mode4-latin1-d", string(all[170:220]), maxicode.Options{Mode: 4}, string(all[170:220])},
		{"mode4-latin1-e", string(all[220:]), maxicode.Options{Mode: 4}, string(all[220:])},
		{"mode6", "READER PROGRAM", maxicode.Options{Mode: 6}, "READER PROGRAM"},
		{"mode4-sa", "PART TWO OF THREE", maxicode.Options{Mode: 4, SymbolNumber: 2, SymbolCount: 3}, "PART TWO OF THREE"},
		{"mode2-sa", "001840152382802" + ups, maxicode.Options{Mode: 2, SymbolNumber: 1, SymbolCount: 2},
			"[)>\x1e01\x1d96152382802\x1d840\x1d001\x1d1Z00004951\x1dUPSN\x1d06X610\x1d159\x1d1234567\x1d1/1\x1d\x1dY\x1d634 ALPHA DR\x1dPITTSBURGH\x1dPA\x1e\x04"},
	}
}

func TestMaxiCodeZXingCpp(t *testing.T) {
	dec := os.Getenv("MAXICODE_DECODER")
	if dec == "" {
		t.Skip("MAXICODE_DECODER not set")
	}
	dir := os.Getenv("MAXICODE_PNG_DIR")
	if dir == "" {
		dir = t.TempDir()
	}
	for _, c := range maxiCases() {
		t.Run(c.name, func(t *testing.T) {
			data := maxiLatin1(c.data)
			m, err := maxicode.Encode(data, c.opts)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "maxi-"+c.name+".png")
			f, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(f, maxiImage(m.Rows, 10)); err != nil {
				t.Fatal(err)
			}
			f.Close()
			out, err := exec.Command(dec, path, "MaxiCode").CombinedOutput()
			if err != nil {
				t.Fatalf("decode: %v\n%s", err, out)
			}
			fields := strings.Split(strings.TrimSpace(string(out)), "\t")
			if len(fields) < 3 {
				t.Fatalf("decoder output %q", out)
			}
			got, err := hex.DecodeString(fields[2])
			if err != nil {
				t.Fatal(err)
			}
			if want := maxiLatin1(c.want); !bytes.Equal(got, want) {
				t.Errorf("decoded %q\nwant    %q\n%s", got, want, out)
			}
		})
	}
}

// maxiLatin1 converts a UTF-8 Go string whose runes are all < 256 to bytes.
func maxiLatin1(s string) []byte {
	b := make([]byte, 0, len(s))
	for _, r := range s {
		b = append(b, byte(r))
	}
	return b
}
