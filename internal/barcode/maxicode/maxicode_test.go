package maxicode

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
)

// ISO/IEC 16023:2000 Figure 2 (mode 4, 93 code set A characters).
var isoFigure2 = []string{
	"011111010000001000001000100111",
	"000100000001000000001010000000",
	"001011001100100110110010010010",
	"100000010001100010010000000000",
	"001011000000101000001010110011",
	"111010001000001011001000111100",
	"100000000110000010010000000000",
	"000010100010010010001001111100",
	"111011100000001000000110000000",
	"000000011011000000010100011000",
	"101111000001010110001100000011",
	"001110001010000000111010001110",
	"000111100000000000100001011000",
	"100010000000000000000111001000",
	"100000001000000000011000001000",
	"000010111000000000000010000010",
	"111000001000000000001000001101",
	"011000000000000000001000100100",
	"000000101100000000001001010001",
	"101010001000000000100111001100",
	"001000011000000000011100001010",
	"000000000000000000110000100000",
	"101011001010100001000101010001",
	"100011110010101001101010001010",
	"011010000000000101011010011111",
	"000001110011111111111100010100",
	"001110100111000101011000011100",
	"110111011100100001101001010110",
	"000001011011101010010111001100",
	"111000110111100010001111011110",
	"101111010111111000010110111001",
	"001001101111101101101010011100",
	"001011000000111101100100001000",
}

// ISO/IEC 16023:2000 Annex B.1 example primary message (mode 3, postal code
// "B1050", country 056, class 999) with secondary data "CEN".
var isoB1 = []string{
	"000000010101010101010101010111",
	"001011000000000000000000000010",
	"111001101010101010101010101000",
	"010101010101010101010101010110",
	"000000000000000000000000000001",
	"101010101010101010101010101010",
	"010101010101010101010101010100",
	"000000000000000000000000000010",
	"101010101010101010101010101000",
	"010101010111100000100101010110",
	"000000000001110000010100000010",
	"101010101000000011010010101010",
	"010101111100000000100001010110",
	"000000001100000000011000000010",
	"101010100100000000011110101001",
	"010101011000000000000001010110",
	"000000101000000000001000000001",
	"101010110000000000001010101010",
	"010101010000000000011101010101",
	"000000101000000000011100000000",
	"101010111100000000001010101001",
	"010101011110000000011101010110",
	"000000001110000001111000000011",
	"101010101011011001000110101010",
	"010101010101010101011100100010",
	"000000000000000000000011011100",
	"101010101010101010101001110100",
	"111011101110000110101011010110",
	"100001011111111101000011100111",
	"110100001000001101100010100110",
	"110110111111011110000011011111",
	"010010001001110010000101000010",
	"010001011010000011010010011100",
}

func matrixString(m barcode.Matrix) []string {
	out := make([]string, len(m.Rows))
	for y, r := range m.Rows {
		var b strings.Builder
		for _, d := range r {
			if d {
				b.WriteByte('1')
			} else {
				b.WriteByte('0')
			}
		}
		out[y] = b.String()
	}
	return out
}

func compare(t *testing.T, got barcode.Matrix, want []string) {
	t.Helper()
	g := matrixString(got)
	if len(g) != Rows || got.Width() != Cols {
		t.Fatalf("size %dx%d", got.Width(), len(g))
	}
	for y := range want {
		if g[y] != want[y] {
			t.Errorf("row %2d got  %s\n       want %s", y, g[y], want[y])
		}
	}
}

func TestISOFigure2(t *testing.T) {
	m, err := Encode([]byte("THIS IS A 93 CHARACTER CODE SET A MESSAGE THAT FILLS A MODE 4, UNAPPENDED, MAXICODE SYMBOL..."), Options{Mode: 4})
	if err != nil {
		t.Fatal(err)
	}
	compare(t, m, isoFigure2)
}

func TestISOB1(t *testing.T) {
	m, err := EncodeMessage(Message{Mode: 3, PostalCode: "B1050", CountryCode: 56, ServiceClass: 999, Data: []byte("CEN")}, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	compare(t, m, isoB1)
	// Same symbol through Zebra's mode 3 field data layout.
	m2, err := Encode([]byte("999056B1050 CEN"), Options{Mode: 3})
	if err != nil {
		t.Fatal(err)
	}
	compare(t, m2, isoB1)
}

// decodeText is a reference decoder for the codeword stream (after the
// mode/primary), modelled on ISO/IEC 16023 4.9 / zxing.
func decodeText(t *testing.T, cw []byte) []byte {
	t.Helper()
	var out []byte
	set, last, shift := setA, setA, -1
	for i := 0; i < len(cw); i++ {
		c := codeSets[set][cw[i]]
		switch c {
		case fnLatchA:
			set, shift = setA, -1
		case fnLatchB:
			set, shift = setB, -1
		case fnShiftA, fnShiftB, fnShiftC, fnShiftD, fnShiftE:
			last = set
			set = int(fnShiftA - c)
			shift = 1
		case fn2ShiftA, fn3ShiftA:
			last = set
			set = setA
			shift = 2
			if c == fn3ShiftA {
				shift = 3
			}
		case fnNS:
			if i+5 >= len(cw) {
				t.Fatalf("truncated NS")
			}
			v := int(cw[i+1])<<24 | int(cw[i+2])<<18 | int(cw[i+3])<<12 | int(cw[i+4])<<6 | int(cw[i+5])
			i += 5
			out = append(out, []byte(strings.Repeat("0", 9-len(itoa(v)))+itoa(v))...)
		case fnLock:
			shift = -1
		case fnPad:
			out = append(out, 0xff, 0xfe) // marker, trimmed below
		case fnECI:
			t.Fatalf("unexpected ECI")
		default:
			out = append(out, byte(c))
		}
		if shift == 0 {
			set = last
		}
		shift--
	}
	for len(out) >= 2 && out[len(out)-2] == 0xff && out[len(out)-1] == 0xfe {
		out = out[:len(out)-2]
	}
	return out
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var b []byte
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	return string(b)
}

func TestCodeSetsCoverAllBytes(t *testing.T) {
	for b := 0; b < 256; b++ {
		ok := false
		for s := 0; s < numSets; s++ {
			ok = ok || charValue[s][b] >= 0
		}
		if !ok {
			t.Errorf("byte %#x not encodable", b)
		}
	}
}

func TestTextRoundTrip(t *testing.T) {
	all := make([]byte, 256)
	for i := range all {
		all[i] = byte(i)
	}
	cases := [][]byte{
		nil,
		[]byte("A"),
		[]byte("abc"),
		[]byte("aBc"),
		[]byte("abcDEFghi"),
		[]byte("123456789"),
		[]byte("12345678901234567890X"),
		[]byte("ÀÁÂàáâ€"),
		[]byte("\x00\x01\x02\x1b"),
		[]byte("[)>\x1e01\x1d961Z00004951\x1dUPSN\x1d06X610\x1d159\x1d1234567\x1d1/1\x1d\x1dY\x1d634 ALPHA DR\x1dPITTSBURGH\x1dPA\x1e\x04"),
		all[:64], all[64:128], all[128:192], all[192:],
		[]byte("ab,c.d/e:f g"),
		[]byte("éaéaé"),
		[]byte("\xc0\xe0\xc0\xe0\x80\x8a"),
	}
	for _, in := range cases {
		cw, _ := encodeText(in)
		got := decodeText(t, cw)
		if !bytes.Equal(got, in) {
			t.Errorf("round trip %q: got %q (cw %v)", in, got, cw)
		}
	}
}

func TestTextOptimal(t *testing.T) {
	for _, c := range []struct {
		in   string
		want int
	}{
		{"ABC", 3},
		{"abc", 4},              // latch B + 3
		{"aBc", 5},              // latch B, a, shift A, B, c
		{"123456789", 6},        // NS
		{"abc12d", 8},           // latch B, abc, 2 shift A 1 2, d
		{"\xe0\xe1\xe2\xe3", 6}, // lock D (2) + 4
	} {
		cw, _ := encodeText([]byte(c.in))
		if len(cw) != c.want {
			t.Errorf("%q: %d codewords %v, want %d", c.in, len(cw), cw, c.want)
		}
	}
}

func TestRSGenerator(t *testing.T) {
	// Every check symbol sequence must make the codeword polynomial vanish
	// at alpha^1..alpha^n.
	data := []byte{2, 31, 0, 5, 63, 1, 7, 9, 44, 13}
	ec := rsEncode(data, 10)
	all := append(append([]byte{}, data...), ec...)
	for i := 1; i <= 10; i++ {
		var s byte
		for _, c := range all {
			s = gfMul(s, gfExp[i]) ^ c
		}
		if s != 0 {
			t.Errorf("syndrome %d = %d", i, s)
		}
	}
}

func TestParseZPL(t *testing.T) {
	lpm := "[)>\x1e01\x1d961Z00004951\x1dUPSN\x1e\x04"
	m, err := ParseZPL(2, []byte("001840152382802"+lpm))
	if err != nil {
		t.Fatal(err)
	}
	if m.ServiceClass != 1 || m.CountryCode != 840 || m.PostalCode != "152382802" || string(m.Data) != lpm {
		t.Errorf("mode 2: %+v", m)
	}
	m, err = ParseZPL(3, []byte("999056B1050 hello"))
	if err != nil || m.PostalCode != "B1050 " || m.CountryCode != 56 || m.ServiceClass != 999 || string(m.Data) != "hello" {
		t.Errorf("mode 3: %+v %v", m, err)
	}
	// UPS standard layout.
	m, err = ParseZPL(2, []byte("[)>\x1e01\x1d9615238\x1d840\x1d001\x1d1Z123\x1e\x04"))
	if err != nil || m.PostalCode != "152380000" || m.CountryCode != 840 || m.ServiceClass != 1 ||
		string(m.Data) != "[)>\x1e01\x1d961Z123\x1e\x04" {
		t.Errorf("scm layout: %+v %v", m, err)
	}
	m, err = ParseZPL(4, []byte("anything"))
	if err != nil || m.Mode != 4 || string(m.Data) != "anything" {
		t.Errorf("mode 4: %+v %v", m, err)
	}
	m, err = ParseZPL(0, []byte("001840152382802"))
	if err != nil || m.Mode != 2 {
		t.Errorf("default mode: %+v %v", m, err)
	}
}

func TestErrors(t *testing.T) {
	bad := []struct {
		data string
		opts Options
	}{
		{"00184015238", Options{Mode: 2}},       // short hpm
		{"0018401523A2802", Options{Mode: 2}},   // non-numeric zip
		{"0A1840152382802", Options{Mode: 2}},   // class
		{"001840B1050_", Options{Mode: 3}},      // invalid postal char
		{"x", Options{Mode: 7}},                 // mode
		{"x", Options{Mode: 4, SymbolCount: 9}}, // SA count
		{"x", Options{Mode: 4, SymbolNumber: 3, SymbolCount: 2}},
		{strings.Repeat("A", 94), Options{Mode: 4}}, // too long
		{strings.Repeat("A", 78), Options{Mode: 5}},
		{"001840152382802" + strings.Repeat("A", 85), Options{Mode: 2}},
	}
	for _, c := range bad {
		if _, err := Encode([]byte(c.data), c.opts); !errors.Is(err, barcode.ErrInvalidData) {
			t.Errorf("%q %+v: err %v", c.data, c.opts, err)
		}
	}
	// Capacity limits that must fit.
	for _, c := range []struct {
		data string
		opts Options
	}{
		{strings.Repeat("A", 93), Options{Mode: 4}},
		{strings.Repeat("A", 77), Options{Mode: 5}},
		{strings.Repeat("A", 91), Options{Mode: 6, SymbolNumber: 2, SymbolCount: 3}},
		{"001840152382802" + strings.Repeat("A", 84), Options{Mode: 2}},
		{strings.Repeat("1", 138), Options{Mode: 4}},
	} {
		if _, err := Encode([]byte(c.data), c.opts); err != nil {
			t.Errorf("%d chars %+v: %v", len(c.data), c.opts, err)
		}
	}
}

func TestStructuredAppendCodewords(t *testing.T) {
	cw, err := Codewords(Message{Mode: 4, Data: []byte("AB")}, 3, 5)
	if err != nil {
		t.Fatal(err)
	}
	if cw[0] != 4 || cw[1] != cwPad || cw[2] != (5-1)|(3-1)<<3 || cw[3] != 1 || cw[4] != 2 {
		t.Errorf("codewords %v", cw[:6])
	}
}

func TestGeometry(t *testing.T) {
	if x, y := FinderCenter(); x != 14.5 || y-SymbolHeight/2 > 1e-9 || SymbolHeight/2-y > 1e-9 {
		t.Errorf("finder centre %v,%v", x, y)
	}
	r := FinderRings()
	if r[0][0] != HexHeight || r[2][1]-9 > 1e-9 || 9-r[2][1] > 1e-9 {
		t.Errorf("rings %v", r)
	}
	m, _ := Encode([]byte("x"), Options{Mode: 4})
	for y := 1; y < Rows; y += 2 {
		if m.Rows[y][Cols-1] {
			t.Errorf("odd row %d column 29 dark", y)
		}
	}
}

// TestZebraPadding pins codewords read back from a Zebra ZD420's own
// rendering of ^BD4^FDMaxiCode mode 4 sample: the printer ends the data with
// codeword 63, pads with 33, and encodes " 4 " in set B with a single
// Shift A for the digit.
func TestZebraPadding(t *testing.T) {
	want := []byte{4, 13, 63, 1, 24, 9, 59, 3, 15, 4, 50, 2, 42, 51, 53, 34, 22, 20, 5, 16,
		5, 47, 13, 15, 4, 5, 47, 59, 52, 47, 19, 1, 13, 16, 12, 5, 63, 33}
	m, err := ParseZPL(4, []byte("MaxiCode mode 4 sample"))
	if err != nil {
		t.Fatal(err)
	}
	m.ZebraPadding = true
	cw, err := Codewords(m, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(cw[:len(want)], want) {
		t.Errorf("codewords\n got %v\nwant %v", cw[:len(want)], want)
	}
	for i := len(want); i < 104; i++ {
		if cw[i] != 33 {
			t.Fatalf("codeword %d = %d, want PAD 33", i, cw[i])
		}
	}
}
