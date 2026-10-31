package pdf417

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
)

func rowString(r []bool) string {
	var b strings.Builder
	for _, d := range r {
		if d {
			b.WriteByte('1')
		} else {
			b.WriteByte('0')
		}
	}
	return b.String()
}

func matrixStrings(m barcode.Matrix) []string {
	out := make([]string, len(m.Rows))
	for i, r := range m.Rows {
		out[i] = rowString(r)
	}
	return out
}

// TestISOVectors reproduces ISO/IEC 15438 Figure 1, Annex Q and Figure G.1.
func TestISOVectors(t *testing.T) {
	for _, v := range isoVectors {
		m, err := Encode([]byte(v.data), Options{SecurityLevel: v.level, Columns: v.cols, Truncated: v.truncated})
		if err != nil {
			t.Fatalf("%s: %v", v.name, err)
		}
		if got := matrixStrings(m); !reflect.DeepEqual(got, v.rows) {
			t.Errorf("%s:\n got %q\nwant %q", v.name, got, v.rows)
		}
	}
}

func TestGeneratorCoefficients(t *testing.T) {
	// ISO/IEC 15438 Annex F, Tables F.1 (level 0) and F.2 (level 1).
	if got := GeneratorCoefficients(2); !reflect.DeepEqual(got, []int{27, 917}) {
		t.Errorf("k=2: %v", got)
	}
	if got := GeneratorCoefficients(4); !reflect.DeepEqual(got, []int{522, 568, 723, 809}) {
		t.Errorf("k=4: %v", got)
	}
	for k := 2; k <= 512; k *= 2 {
		data := []int{5, 453, 178, 121, 239}
		cw := append(append([]int(nil), data...), ECCodewords(data, k)...)
		if !syndromesZero(cw, k) {
			t.Errorf("k=%d: non-zero syndrome", k)
		}
	}
}

// TestAnnexQ checks the worked error correction example of ISO/IEC 15438
// Annex Q: data codewords 5 453 178 121 239 ("PDF417") at level 1.
func TestAnnexQ(t *testing.T) {
	got := ECCodewords([]int{5, 453, 178, 121, 239}, 4)
	if want := []int{452, 327, 657, 619}; !reflect.DeepEqual(got, want) {
		t.Errorf("EC %v, want %v", got, want)
	}
	if got := Compact([]byte("PDF417")); !reflect.DeepEqual(got, []int{453, 178, 121, 239}) {
		t.Errorf("data %v", got)
	}
}

func TestNumericAnnexD(t *testing.T) {
	got := NumericCompact([]byte("000213298174000"))
	if want := []int{1, 624, 434, 632, 282, 200}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

// TestTables checks the symbol character table against the structural rules
// of ISO/IEC 15438 5.3.1: 4 bars and 4 spaces of 1..6 modules in 17 modules,
// cluster number (b1-b2+b3-b4+9) mod 9 = 0, 3, 6, and all patterns distinct.
func TestTables(t *testing.T) {
	seen := map[uint32]bool{}
	for cl := 0; cl < 3; cl++ {
		for cw := 0; cw < 929; cw++ {
			p := CodewordPattern(cl, cw)
			var w []int
			prev, run := uint32(2), 0
			for i := 16; i >= 0; i-- {
				b := p >> uint(i) & 1
				if b == prev {
					run++
				} else {
					if run > 0 {
						w = append(w, run)
					}
					prev, run = b, 1
				}
			}
			w = append(w, run)
			if len(w) != 8 || p>>16 != 1 || p&1 != 0 {
				t.Fatalf("cluster %d cw %d: bad pattern %017b", cl, cw, p)
			}
			for _, x := range w {
				if x > 6 {
					t.Fatalf("cluster %d cw %d: element > 6", cl, cw)
				}
			}
			if k := (w[0] - w[2] + w[4] - w[6] + 9) % 9; k != 3*cl {
				t.Fatalf("cluster %d cw %d: cluster number %d", cl, cw, k)
			}
			if seen[p] {
				t.Fatalf("duplicate pattern %017b", p)
			}
			seen[p] = true
		}
	}
	for i := 1; i <= 52; i++ {
		for _, r := range [][]bool{AppendSideRAP(nil, i), AppendCentreRAP(nil, i)} {
			if len(r) != RAPModules || !r[0] {
				t.Fatalf("RAP %d malformed", i)
			}
		}
	}
}

// TestDecodedGolden pins codeword streams of symbols that zxing-cpp decoded
// back to the input (PNG round trips run while developing). Shapes are given
// explicitly so the streams (whose padding depends on the shape) stay valid
// independently of the automatic sizing rule.
func TestDecodedGolden(t *testing.T) {
	cases := []struct {
		data  string
		opts  Options
		cols  int
		rows  int
		words []int
	}{
		{"HELLO WORLD", Options{Columns: 2}, 2, 5, []int{8, 214, 341, 446, 674, 521, 119, 900, 812, 808}},
		{"PDF417 Truncated test 0123", Options{SecurityLevel: 1, Truncated: true, Columns: 3}, 3, 7,
			[]int{17, 453, 178, 121, 236, 859, 827, 613, 60, 574, 116, 574, 559, 808, 1, 63, 900, 662, 55, 754, 612}},
		// The tail "\r\n\t~{}" (six punctuation characters) now latches to
		// Punctuation from Lower as a ZD420 does (ml pl); TestCompactRoundTrip
		// checks the text still decodes.
		{"Hello, World! 1234567890123456 abc DEF ;;;; xyz\r\n\t~{}", Options{SecurityLevel: 2, Columns: 5}, 5, 9,
			[]int{37, 237, 131, 344, 883, 807, 674, 521, 119, 326, 902, 19, 23, 229, 801, 348, 256, 900, 807, 1, 86, 813, 814, 815, 808, 750, 0, 29, 807, 714, 778, 761, 462, 296, 839, 900, 900, 713, 153, 552, 797, 173, 926, 385, 92}},
		{"\u00e9\u00e8 bytes \x01\x02\x03 mixed 99", Options{SecurityLevel: 3, Columns: 5}, 5, 8,
			[]int{24, 901, 195, 169, 195, 168, 900, 807, 54, 574, 566, 901, 1, 2, 3, 900, 807, 368, 694, 116, 849, 299, 900, 900, 905, 747, 469, 215, 781, 580, 451, 297, 901, 545, 707, 679, 206, 43, 435, 621}},
		{"12345678901234567890123456789012345678901234567890", Options{SecurityLevel: 4, Rows: 10}, 6, 10,
			[]int{28, 902, 491, 81, 137, 450, 302, 67, 15, 174, 492, 862, 667, 475, 869, 12, 434, 1, 842, 90, 900, 900, 900, 900, 900, 900, 900, 900, 835, 821, 681, 666, 158, 679, 784, 831, 86, 802, 731, 765, 365, 296, 914, 198, 572, 561, 710, 159, 19, 771, 573, 33, 137, 534, 155, 736, 330, 179, 628, 655}},
	}
	for _, c := range cases {
		s, err := EncodeSymbol([]byte(c.data), c.opts)
		if err != nil {
			t.Fatal(err)
		}
		if s.Columns != c.cols || s.Rows != c.rows || !reflect.DeepEqual(s.Codewords, c.words) {
			t.Errorf("%q: %dx%d %v", c.data, s.Columns, s.Rows, s.Codewords)
		}
	}
}

func TestSizing(t *testing.T) {
	// ^B7: 72 codewords with no rows/columns -> 6 columns x 12 rows.
	// Shapes measured on a Zebra ZD420: {codewords, columns, rows}.
	for _, m := range [][3]int{
		{16, 1, 16},
		{20, 1, 20},
		{24, 2, 12},
		{28, 2, 14},
		{32, 2, 16},
		{36, 2, 18},
		{40, 2, 20},
		{44, 3, 15},
		{48, 3, 16},
		{52, 3, 18},
		{56, 3, 19},
		{60, 3, 20},
		{64, 3, 22},
		{68, 3, 23},
		{72, 3, 24},
		{76, 4, 19},
		{80, 4, 20},
		{84, 4, 21},
		{88, 4, 22},
		{92, 4, 23},
		{96, 4, 24},
		{100, 4, 25},
		{104, 4, 26},
		{108, 5, 22},
		{112, 5, 23},
		{116, 5, 24},
		{120, 5, 24},
		{124, 5, 25},
		{128, 5, 26},
		{132, 5, 27},
		{136, 5, 28},
		{140, 5, 28},
		{144, 5, 29},
		{148, 6, 25},
		{152, 6, 26},
		{156, 6, 26},
		{160, 6, 27},
		{164, 6, 28},
		{168, 6, 28},
		{172, 6, 29},
		{176, 6, 30},
		{180, 6, 30},
		{184, 6, 31},
		{188, 6, 32},
		{192, 7, 28},
		{196, 7, 28},
		{200, 7, 29},
		{204, 7, 30},
		{208, 7, 30},
		{212, 7, 31},
		{216, 7, 31},
		{220, 7, 32},
		{224, 7, 32},
		{228, 7, 33},
		{232, 7, 34},
		{236, 7, 34},
		{240, 7, 35},
		{244, 8, 31},
		{248, 8, 31},
		{252, 8, 32},
		{256, 8, 32},
		{260, 8, 33},
		{264, 8, 33},
		{268, 8, 34},
		{272, 8, 34},
		{276, 8, 35},
		{280, 8, 35},
		{284, 8, 36},
		{288, 8, 36},
		{292, 8, 37},
		{296, 8, 37},
		{300, 9, 34},
		{304, 9, 34},
		{308, 9, 35},
		{312, 9, 35},
		{316, 9, 36},
		{320, 9, 36},
		{324, 9, 36},
		{328, 9, 37},
	} {
		if c, r, err := size(m[0], 0, 0); err != nil || c != m[1] || r != m[2] {
			t.Errorf("size(%d) = %d x %d, %v; printer used %d x %d", m[0], c, r, err, m[1], m[2])
		}
	}
	for total := 3; total <= 928; total++ {
		c, r, err := size(total, 0, 0)
		if err != nil || c*r < total || c*r > 928 || r < 3 || r > 90 || c > 30 {
			t.Fatalf("size(%d) = %d x %d, %v", total, c, r, err)
		}
	}
	if _, _, err := size(50, 3, 10); err == nil {
		t.Error("50 codewords in 3x10 accepted")
	}
	if _, _, err := size(100, 31, 0); err == nil {
		t.Error("31 columns accepted")
	}
	if _, _, err := size(929, 0, 0); err == nil {
		t.Error("929 codewords accepted")
	}
}

func TestLevels(t *testing.T) {
	for lvl := 0; lvl <= 8; lvl++ {
		s, err := EncodeSymbol([]byte("LEVEL TEST"), Options{SecurityLevel: lvl})
		if err != nil {
			t.Fatalf("level %d: %v", lvl, err)
		}
		k := 2 << lvl
		if !syndromesZero(s.Codewords, k) || s.Codewords[0] != len(s.Codewords)-k {
			t.Errorf("level %d: bad codewords", lvl)
		}
	}
	if _, err := Encode([]byte("x"), Options{SecurityLevel: 9}); !errors.Is(err, barcode.ErrInvalidData) {
		t.Error("level 9 accepted")
	}
	s, _ := EncodeSymbol([]byte("AUTO"), Options{SecurityLevel: SecurityAuto})
	if s.ECLevel != 2 {
		t.Errorf("auto level %d", s.ECLevel)
	}
}

func TestTruncatedWidth(t *testing.T) {
	full, _ := Encode([]byte("ABC"), Options{Columns: 3})
	tr, _ := Encode([]byte("ABC"), Options{Columns: 3, Truncated: true})
	if full.Width() != 17*(3+4)+1 || tr.Width() != 17*(3+2)+1 {
		t.Errorf("widths %d %d", full.Width(), tr.Width())
	}
}

func TestMacro(t *testing.T) {
	s, err := EncodeSymbol([]byte("AB"), Options{Columns: 3, Macro: &Macro{SegmentIndex: 1, FileID: []int{17, 53}, Last: true}})
	if err != nil {
		t.Fatal(err)
	}
	// LD, AB, padding..., 928, 111, 101 (segment 00001), 17, 53, 922
	got := fmt.Sprint(s.Codewords[len(s.Codewords)-2-6 : len(s.Codewords)-2])
	if got != "[928 111 101 17 53 922]" {
		t.Errorf("macro block %s", got)
	}
	data := []byte(strings.Repeat("STRUCTURED APPEND 0123456789 ", 40))
	ms, err := EncodeStructured(data, Options{Columns: 10, Rows: 20}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) < 2 {
		t.Errorf("expected several symbols, got %d", len(ms))
	}
	if _, err := EncodeStructured(data, Options{Columns: 3, Rows: 3}, 2); !errors.Is(err, barcode.ErrInvalidData) {
		t.Error("overflow not reported")
	}
}

func TestCompactModes(t *testing.T) {
	cases := []struct {
		in   string
		want []int
	}{
		{"AB", []int{1}},                    // A=0,B=1 -> 0*30+1
		{"ab", []int{27*30 + 0, 1*30 + 29}}, // ll a, b ps
		{"A;", []int{0*30 + 29, 0*30 + 29}}, // A ps ; pad
		{"\x80", []int{913, 128}},           // byte shift
	}
	for _, c := range cases {
		if got := Compact([]byte(c.in)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("Compact(%q) = %v want %v", c.in, got, c.want)
		}
	}
	if got := Compact([]byte("1234567890123")); got[0] != CWNumericLatch {
		t.Errorf("13 digits not numeric: %v", got)
	}
	if got := Compact([]byte("ABCDEF")); len(got) != 3 {
		t.Errorf("ABCDEF: %v", got)
	}
}

func TestExpandFieldEscapes(t *testing.T) {
	if got := string(ExpandFieldEscapes([]byte(`A\&B\\C\x`))); got != "A\r\nB\\C\\x" {
		t.Errorf("got %q", got)
	}
}
