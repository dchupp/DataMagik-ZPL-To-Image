package verify

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/makiuchi-d/gozxing"

	"github.com/datamagik/zpl-to-image/internal/barcode/datamatrix"
)

func dmLatin1(b []byte) string {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}

// dmNormLatin1 maps gozxing output to Latin-1 runes: gozxing returns ASCII
// upper-shifted bytes raw (invalid UTF-8) but Base 256 bytes as runes.
func dmNormLatin1(s string) string {
	var r []rune
	for len(s) > 0 {
		c, n := utf8.DecodeRuneInString(s)
		if c == utf8.RuneError && n == 1 {
			c = rune(s[0])
		}
		r = append(r, c)
		s = s[n:]
	}
	return string(r)
}

func dmRoundTrip(t *testing.T, name string, data []byte, opt datamatrix.Options, want string) {
	t.Helper()
	m, err := datamatrix.Encode(data, opt)
	if err != nil {
		t.Errorf("%s: encode: %v", name, err)
		return
	}
	got, err := Decode(MatrixImage(m, 4, 0, 2), gozxing.BarcodeFormat_DATA_MATRIX)
	if err != nil {
		t.Errorf("%s (%dx%d): decode: %v", name, m.Height(), m.Width(), err)
		return
	}
	if dmNormLatin1(got) != want {
		t.Errorf("%s (%dx%d): got %q want %q", name, m.Height(), m.Width(), got, want)
	}
}

func TestDataMatrixRoundTrip(t *testing.T) {
	q := datamatrix.Options{Quality: 200}
	cases := []string{
		"1", "12", "123456", "Hello, World!", "AIMAIMAIM", "aimaimaim", "ABC>ABC123>AB",
		".A.C1.3.DATA.123DATA.123DATA", "A", "AB", "ABCD", "ABCDE", "abcde", "ABC12",
		"THE QUICK BROWN FOX JUMPS OVER THE LAZY DOG 0123456789",
		"the quick brown fox jumps over the lazy dog 0123456789",
		"ABC*DEF>GHI\r123*456", "EDIFACT=ABC:DEF;GHI?JKL", "http://www.zebra.com/?a=1&b=2",
		"Mixed CASE text With Numbers 12345 and symbols !@#$%^&*()",
		"\x00\x01\x02 control \x1f\x7f", "~`{|}", "aA1!aA1!aA1!aA1!",
		strings.Repeat("0123456789", 30), strings.Repeat("ABCDEFGHIJ", 40),
		strings.Repeat("abc", 101), strings.Repeat("ABC>*", 50), strings.Repeat("A.B:", 60),
	}
	for _, s := range cases {
		dmRoundTrip(t, fmt.Sprintf("%.20q", s), []byte(s), q, s)
	}
	// Binary / extended ASCII.
	bin := []byte("\xe9t\xe9 \xc0 Z\xfcrich \x80\x81\xff")
	dmRoundTrip(t, "latin1", bin, q, dmLatin1(bin))
	r := rand.New(rand.NewSource(1))
	for _, n := range []int{1, 5, 50, 249, 250, 251, 600, 1000, 1555} {
		b := make([]byte, n)
		r.Read(b)
		dmRoundTrip(t, fmt.Sprintf("random%d", n), b, q, dmLatin1(b))
	}
	// Random mixtures of character classes stress mode switching.
	alpha := []string{"0123456789", "ABCDEFGHIJKLMNOPQRSTUVWXYZ", "abcdefghijklmnopqrstuvwxyz", " *>\r", ".,:;-/", "\x80\xa9\xff", "!\"#$%&'()"}
	for i := 0; i < 300; i++ {
		var sb []byte
		for len(sb) < 1+r.Intn(80) {
			set := alpha[r.Intn(len(alpha))]
			for k := r.Intn(12); k >= 0; k-- {
				sb = append(sb, set[r.Intn(len(set))])
			}
		}
		dmRoundTrip(t, fmt.Sprintf("mix%d %q", i, sb), sb, q, dmLatin1(sb))
	}
	// Escape sequences.
	e := datamatrix.Options{Quality: 200, Escape: '_'}
	dmRoundTrip(t, "escapes", []byte("A__B_d065_G"), e, "A_BA\x07")
	dmRoundTrip(t, "fnc1 mid", []byte("AB_1CD"), e, "AB\x1dCD")
}

// Every ECC 200 size, forced, filled to capacity.
func TestDataMatrixAllSizes(t *testing.T) {
	sizes := [][2]int{{10, 10}, {12, 12}, {14, 14}, {16, 16}, {18, 18}, {20, 20}, {22, 22}, {24, 24}, {26, 26},
		{32, 32}, {36, 36}, {40, 40}, {44, 44}, {48, 48}, {52, 52}, {64, 64}, {72, 72}, {80, 80}, {88, 88},
		{96, 96}, {104, 104}, {120, 120}, {132, 132}, {144, 144}, {8, 18}, {8, 32}, {12, 26}, {12, 36}, {16, 36}, {16, 48}}
	caps := []int{3, 5, 8, 12, 18, 22, 30, 36, 44, 62, 86, 114, 144, 174, 204, 280, 368, 456, 576, 696, 816, 1050, 1304, 1558, 5, 10, 16, 22, 32, 49}
	for i, sz := range sizes {
		// Lower-case letters encode one per ASCII codeword: exactly fill.
		s := make([]byte, caps[i])
		for k := range s {
			s[k] = byte('a' + k%26)
		}
		// Digits mixed in: half-capacity digit pairs.
		opt := datamatrix.Options{Quality: 200, Rows: sz[0], Columns: sz[1]}
		dmRoundTrip(t, fmt.Sprintf("%dx%d full", sz[0], sz[1]), s, opt, string(s))
		dmRoundTrip(t, fmt.Sprintf("%dx%d short", sz[0], sz[1]), []byte("12"), opt, "12")
	}
}
