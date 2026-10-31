package composite

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
)

type zintExample struct {
	idx     int
	linear  Linear
	typ     Type
	data    string
	primary string
	rows    []string
}

func firstOne(s string) int { return strings.IndexByte(s, '1') }

// TestZintExamples compares the 2D component and its alignment with the
// reference symbols (GS1 General Specifications / ISO/IEC 24723 figures as
// reproduced by zint and cross-checked with BWIPP).
func TestZintExamples(t *testing.T) {
	for _, ex := range zintExamples {
		opts := Options{Linear: ex.linear, Type: ex.typ}
		n := len(ex.rows)
		last := ex.rows[n-1]
		if ex.linear == GS1128 {
			opts.LinearWidth = strings.LastIndexByte(last, '1') - firstOne(last) + 1
		}
		c, err := Encode(ex.data, opts)
		if err != nil {
			t.Errorf("#%d %s: %v", ex.idx, ex.data, err)
			continue
		}
		if ex.typ != Auto && c.Type != ex.typ {
			t.Errorf("#%d: type %v, want %v", ex.idx, c.Type, ex.typ)
		}
		r := c.Matrix.Height()
		if r+2 > n {
			t.Errorf("#%d: %d component rows, symbol has %d", ex.idx, r, n)
			continue
		}
		cx := len(ex.rows[0])
		for _, row := range ex.rows[:r] {
			cx = min(cx, firstOne(row))
		}
		for y := 0; y < r; y++ {
			var b strings.Builder
			b.WriteString(strings.Repeat("0", cx))
			for _, d := range c.Matrix.Rows[y] {
				if d {
					b.WriteByte('1')
				} else {
					b.WriteByte('0')
				}
			}
			got := b.String()
			want := ex.rows[y]
			if len(got) < len(want) {
				got += strings.Repeat("0", len(want)-len(got))
			}
			if got != want {
				t.Errorf("#%d %s row %d:\n got %s\nwant %s", ex.idx, ex.data, y, got, want)
				break
			}
		}
		// Alignment: module 0 of the linear symbol.
		var lx int
		switch ex.linear {
		case GS1128, UPCA, EAN13, EAN8, UPCE:
			lx = firstOne(last)
		default: // DataBar: leading light guard module before the first bar
			lx = firstOne(ex.rows[r+1]) - 1
		}
		if got, want := c.Offset, cx-lx; got != want {
			t.Errorf("#%d %v %v: offset %d, want %d", ex.idx, ex.linear, c.Type, got, want)
		}
		if c.Matrix.RowHeights == nil || len(c.Matrix.RowHeights) != r {
			t.Errorf("#%d: bad RowHeights", ex.idx)
		}
	}
}

func TestElementString(t *testing.T) {
	cases := []struct{ in, want string }{
		{"(01)09521234543213(10)ABC(21)XYZ", "0109521234543213" + "10ABC\x1d" + "21XYZ"},
		{"(17)010615(10)A123456", "1701061510A123456"},
		{"(91)1A(92)2B", "911A\x1d922B"},
		{"\x1d10ABC\x1d21X", "10ABC\x1d21X"},
		{"]C110ABC", "10ABC"},
	}
	for _, c := range cases {
		got, err := ElementString(c.in)
		if err != nil || got != c.want {
			t.Errorf("ElementString(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
	for _, bad := range []string{"(01", "(0A)12", "()1", "(10)"} {
		if _, err := ElementString(bad); !errors.Is(err, barcode.ErrInvalidData) {
			t.Errorf("ElementString(%q) err = %v", bad, err)
		}
	}
}

func TestErrors(t *testing.T) {
	if _, err := Encode("(10)ABC", Options{Linear: UPCA, Type: CCC}); !errors.Is(err, barcode.ErrInvalidData) {
		t.Error("CC-C with UPC-A accepted")
	}
	if _, err := Encode("(10)ABC", Options{Linear: GS1128, Type: CCC}); !errors.Is(err, barcode.ErrInvalidData) {
		t.Error("CC-C without linear width accepted")
	}
	if _, err := Encode("(91)"+strings.Repeat("x", 400), Options{Linear: UPCE}); !errors.Is(err, barcode.ErrInvalidData) {
		t.Error("oversized CC-B accepted")
	}
	if _, err := Encode("(91)\x01", Options{Linear: UPCA}); !errors.Is(err, barcode.ErrInvalidData) {
		t.Error("invalid character accepted")
	}
}

// TestZXingDecoded pins the byte payloads zxing-cpp returned when reading
// the CC-B component as a MicroPDF417 symbol and the CC-C component as a
// PDF417 symbol (it skips the 920 linkage codeword and returns the bit
// field bytes), confirming symbol structure and error correction.
func TestZXingDecoded(t *testing.T) {
	cases := []struct {
		data string
		opts Options
		hex  string
	}{
		{"(91)abcdefghijklmnopqrstuvwxyz", Options{Linear: UPCA, Type: CCB}, "6c025ab772edebf830e2c7932e6cfa34ead7b36eedfc38f2e642"},
		{"(10)ABCD123456(410)3898765432108", Options{Linear: GS1128, Type: CCC, LinearWidth: 154}, "b08218a30556c5f44d8f3b70d593d4"},
	}
	for _, c := range cases {
		src, _ := ElementString(c.data)
		comp, err := Encode(c.data, c.opts)
		if err != nil {
			t.Fatal(err)
		}
		bits, err := bitString(src, func(int) int { return 8 * len(c.hex) / 2 })
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprintf("%x", toBytes(bits)); got != c.hex {
			t.Errorf("%s: payload %s want %s", c.data, got, c.hex)
		}
		if comp.Codewords[0] != 920 && comp.Codewords[1] != 920 {
			t.Errorf("%s: no 920 linkage codeword", c.data)
		}
	}
}
