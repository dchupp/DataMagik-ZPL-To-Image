package qr

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
)

func codewords(t *testing.T, segs []Segment, opts Options, model, version int) []byte {
	t.Helper()
	b, err := buildBits(segs, opts, model, version)
	if err != nil {
		t.Fatal(err)
	}
	data := finishBits(b, dataCodewords(model, version, opts.ECLevel)*8)
	return addECC(data, model, version, opts.ECLevel)
}

func hexs(b []byte) string { return fmt.Sprintf("% X", b) }

// ISO/IEC 18004 Annex I example: "01234567" in version 1-M.
func TestISOExample01234567(t *testing.T) {
	got := codewords(t, []Segment{{ModeNumeric, []byte("01234567")}}, Options{ECLevel: ECLevelM}, 2, 1)
	want := "10 20 0C 56 61 80 EC 11 EC 11 EC 11 EC 11 EC 11 A5 24 D4 C1 ED 36 C7 87 2C 55"
	if hexs(got) != want {
		t.Errorf("got  %s\nwant %s", hexs(got), want)
	}
}

// Well known "HELLO WORLD" 1-M example (alphanumeric).
func TestHelloWorld1M(t *testing.T) {
	got := codewords(t, []Segment{{ModeAuto, []byte("HELLO WORLD")}}, Options{ECLevel: ECLevelM}, 2, 1)
	want := "20 5B 0B 78 D1 72 DC 4D 43 40 EC 11 EC 11 EC 11 C4 23 27 77 EB D7 E7 E2 5D 17"
	if hexs(got) != want {
		t.Errorf("got  %s\nwant %s", hexs(got), want)
	}
}

func TestRSGenerator(t *testing.T) {
	// Degree-7 generator: alpha exponents 0 87 229 146 149 238 102 21.
	g := rsGenerator(7)
	want := []int{87, 229, 146, 149, 238, 102, 21}
	for i, c := range g {
		if gfLog[c] != want[i] {
			t.Fatalf("generator coefficient %d = a^%d, want a^%d", i, gfLog[c], want[i])
		}
	}
}

func TestFormatBits(t *testing.T) {
	// L / mask 4 -> 110011000101111 (ISO Table C.1).
	s := newSymbol(21)
	s.drawFormat(ECLevelL, 4, 2)
	got := 0
	for i := 0; i <= 5; i++ {
		if s.dark[i][8] {
			got |= 1 << uint(i)
		}
	}
	if s.dark[7][8] {
		got |= 1 << 6
	}
	if s.dark[8][8] {
		got |= 1 << 7
	}
	if s.dark[8][7] {
		got |= 1 << 8
	}
	for i := 9; i < 15; i++ {
		if s.dark[8][14-i] {
			got |= 1 << uint(i)
		}
	}
	if got != 0b110011000101111 {
		t.Errorf("format bits %015b", got)
	}
}

func TestCapacities(t *testing.T) {
	// Data codewords, ISO/IEC 18004 Table 7 spot checks.
	cases := []struct {
		v    int
		l    ECLevel
		want int
	}{{1, ECLevelL, 19}, {1, ECLevelH, 9}, {7, ECLevelM, 124}, {10, ECLevelQ, 154}, {40, ECLevelL, 2956}, {40, ECLevelH, 1276}, {27, ECLevelM, 1128}}
	for _, c := range cases {
		if got := dataCodewords(2, c.v, c.l); got != c.want {
			t.Errorf("v%d-%s: %d data codewords, want %d", c.v, c.l, got, c.want)
		}
	}
	if got := alignmentPositions(32); fmt.Sprint(got) != "[6 34 60 86 112 138]" {
		t.Errorf("v32 alignment %v", got)
	}
	if got := alignmentPositions(7); fmt.Sprint(got) != "[6 22 38]" {
		t.Errorf("v7 alignment %v", got)
	}
	for v := 1; v <= 14; v++ {
		s := newSymbol(v*4 + 17)
		s.drawCommon()
		s.drawModel1()
		if len(s.dataPos) != model1TotalCodewords[v]*8 {
			t.Errorf("model 1 v%d: %d data modules", v, len(s.dataPos))
		}
		for l := ECLevelL; l <= ECLevelH; l++ {
			ec, nb := blockLayout(1, v, l)
			if nb*ec+dataCodewords(1, v, l) > model1TotalCodewords[v] {
				t.Errorf("model 1 v%d-%s overflows", v, l)
			}
		}
	}
	for v := 1; v <= 40; v++ {
		s := newSymbol(v*4 + 17)
		s.drawCommon()
		s.drawModel2(v)
		if got := len(s.dataPos) / 8; got != totalCodewords(2, v) {
			t.Errorf("model 2 v%d: %d codeword positions, want %d", v, got, totalCodewords(2, v))
		}
	}
}

func TestAutoSegments(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"0123456789", "N:0123456789"},
		{"HELLO WORLD", "A:HELLO WORLD"},
		{"hello", "B:hello"},
		{"ABC123456789012345xyz", "A:ABC|N:123456789012345|B:xyz"},
		{"a1", "B:a1"},
		{"\x93\x5f\x96\x7b", "K:\x93\x5f\x96\x7b"},
	}
	for _, c := range cases {
		segs := autoSegments([]byte(c.in), 1, true)
		var s string
		for i, g := range segs {
			if i > 0 {
				s += "|"
			}
			s += string("?NABK"[g.Mode]) + ":" + string(g.Data)
		}
		if s != c.want {
			t.Errorf("%q: got %s want %s", c.in, s, c.want)
		}
	}
	if segs := autoSegments([]byte("\x93\x5f"), 1, false); segs[0].Mode != ModeByte {
		t.Errorf("DisableAutoKanji ignored")
	}
}

func TestParseZPL(t *testing.T) {
	cases := []struct {
		in     string
		ec     ECLevel
		manual bool
		segs   string
		sa     string
	}{
		{"QA,0123456789ABCD 2D code", ECLevelQ, false, "?:0123456789ABCD 2D code", ""},
		{"LA,a,b,c", ECLevelL, false, "?:a,b,c", ""},
		{"HM,N123456789012345", ECLevelH, true, "N:123456789012345", ""},
		{"MM,AAC-42", ECLevelM, true, "A:AC-42", ""},
		{"D03048F,LM,N0123456789,A12AABB,B0006qrcode", ECLevelL, true, "N:0123456789|A:12AABB|B:qrcode", "3/4/8F"},
		{"D03040C,LA,012345678912AABBqrcode", ECLevelL, false, "?:012345678912AABBqrcode", "3/4/0C"},
		{"HM,B0003a,bK\x93\x5f", ECLevelH, true, "B:a,b|K:\x93\x5f", ""},
		{"hello", ECLevelM, false, "?:lo", ""},
		{"XA,data", ECLevelM, false, "?:data", ""},
	}
	for _, c := range cases {
		f, err := ParseZPL([]byte(c.in))
		if err != nil {
			t.Errorf("%q: %v", c.in, err)
			continue
		}
		var s string
		for i, g := range f.Segments {
			if i > 0 {
				s += "|"
			}
			s += string("?NABK"[g.Mode]) + ":" + string(g.Data)
		}
		sa := ""
		if f.StructuredAppend != nil {
			sa = fmt.Sprintf("%d/%d/%02X", f.StructuredAppend.Position, f.StructuredAppend.Total, f.StructuredAppend.Parity)
		}
		if f.ECLevel != c.ec || f.Manual != c.manual || s != c.segs || sa != c.sa {
			t.Errorf("%q: got ec=%s manual=%v segs=%q sa=%q", c.in, f.ECLevel, f.Manual, s, sa)
		}
	}
	for _, bad := range []string{"QM,Nabc", "QM,X1", "QM,B00", "QM,B0009abc", "D1,QA,x", "D17168F,QA,x", "QA,", "QM,Aabc"} {
		if _, err := ParseZPL([]byte(bad)); !errors.Is(err, barcode.ErrInvalidData) {
			t.Errorf("%q: expected ErrInvalidData, got %v", bad, err)
		}
	}
}

func TestEncodeSizes(t *testing.T) {
	m, err := Encode([]Segment{{ModeAuto, []byte("01234567")}}, Options{ECLevel: ECLevelM})
	if err != nil || m.Width() != 21 || m.Height() != 21 {
		t.Fatalf("got %dx%d err %v", m.Width(), m.Height(), err)
	}
	// Largest numeric payload of version 40-L: 7089 digits.
	big := bytes.Repeat([]byte("1"), 7089)
	m, err = Encode([]Segment{{ModeAuto, big}}, Options{})
	if err != nil || m.Width() != 177 {
		t.Fatalf("7089 digits: %d %v", m.Width(), err)
	}
	if _, err := Encode([]Segment{{ModeAuto, append(big, '1')}}, Options{}); !errors.Is(err, barcode.ErrInvalidData) {
		t.Errorf("7090 digits should not fit: %v", err)
	}
	// Byte capacity 40-H: 1273.
	if _, err := Encode([]Segment{{ModeByte, bytes.Repeat([]byte("a"), 1273)}}, Options{ECLevel: ECLevelH}); err != nil {
		t.Errorf("1273 bytes at H: %v", err)
	}
	if _, err := Encode([]Segment{{ModeByte, bytes.Repeat([]byte("a"), 1274)}}, Options{ECLevel: ECLevelH}); err == nil {
		t.Errorf("1274 bytes at H should not fit")
	}
	m, err = Encode([]Segment{{ModeAuto, []byte("QR Code Model 1 ")}}, Options{Model: 1, ECLevel: ECLevelM})
	if err != nil || m.Width() != 25 {
		t.Fatalf("model 1: %d %v", m.Width(), err)
	}
	if _, err := Encode(nil, Options{}); err == nil {
		t.Errorf("empty data should fail")
	}
}

// The Model 1 sample from the zxing-cpp test suite
// (test/samples/qrcode-1/qr-model-1.png: version 2-M, "QR Code Model 1 ")
// must be reproduced exactly with one of the eight masks. This pins down the
// Model 1 codeword placement, block structure, extension patterns and format
// information mask.
func TestModel1Sample(t *testing.T) {
	rows := []string{
		"#######.#######...#######",
		"#.....#..#.##.###.#.....#",
		"#.###.#....###.#..#.###.#",
		"#.###.#.###.###.#.#.###.#",
		"#.###.#.##..#...#.#.###.#",
		"#.....#.#..#..###.#.....#",
		"#######.#.#.#.#.#.#######",
		"........#..#.#...........",
		".####.#.#.#.##.#.#####..#",
		"...#.#..##..#.##..##.#.##",
		"##.##.##....#.#.#..#..#.#",
		".##.#...###..#...###..###",
		"#....##...#...####..#.#.#",
		".##.##....#.#..##.##.##.#",
		".###..##.#.##.#.#..#.#..#",
		"#......#....#.#.##.#..#.#",
		"###...####..#..#...#....#",
		"........#..#.....####...#",
		"#######..#...#.####.#####",
		"#.....#..#..##....#...###",
		"#.###.#.#...####.....#..#",
		"#.###.#.#.##..#.#####...#",
		"#.###.#.####..##...#.#.##",
		"#.....#.##..#.....##.#...",
		"#######...########..#.#.#",
	}
	want := ""
	for _, r := range rows {
		want += r + "\n"
	}
	for mask := 0; mask < 8; mask++ {
		m, err := Encode([]Segment{{ModeAuto, []byte("QR Code Model 1 ")}}, Options{Model: 1, ECLevel: ECLevelM, ForceMask: true, Mask: mask})
		if err != nil {
			t.Fatal(err)
		}
		got := ""
		for _, r := range m.Rows {
			for _, d := range r {
				if d {
					got += "#"
				} else {
					got += "."
				}
			}
			got += "\n"
		}
		if got == want {
			return
		}
		if mask == 7 {
			t.Errorf("no mask reproduces the sample; mask 7 gives\n%s", got)
		}
	}
}
