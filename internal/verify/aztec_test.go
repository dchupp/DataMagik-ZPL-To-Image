package verify

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/makiuchi-d/gozxing"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/aztec"
)

func aztecRoundTrip(t *testing.T, name string, data []byte, o aztec.Options, want string) {
	t.Helper()
	m, err := aztec.Encode(data, o)
	if err != nil {
		t.Errorf("%s: encode: %v", name, err)
		return
	}
	got, err := Decode(MatrixImage(m, 4, 0, 4), gozxing.BarcodeFormat_AZTEC)
	if err != nil {
		t.Errorf("%s (%dx%d): decode: %v", name, m.Width(), m.Height(), err)
		return
	}
	if got != want {
		t.Errorf("%s: decoded %q, want %q", name, got, want)
	}
}

func latin1(b []byte) string {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}

func TestAztecTextModes(t *testing.T) {
	cases := []string{
		"A",
		"Hello World 12345",
		"lowercase only text",
		"UPPER lower MiXeD 0123456789 .,:;!?",
		"a\r\nb. c, d: e",
		"@\\^_`|~ punctuation {}[]<>=+-*/%$#\"'&()",
		"1234567890123456789012345678901234567890",
		"Lo. Test 123.",
		". x://abc/.",
		"\x01\x02\x1b\x7f mixed controls",
		strings.Repeat("The quick brown fox jumps over the lazy dog. ", 20),
		strings.Repeat("0123456789", 150),
	}
	for i, s := range cases {
		aztecRoundTrip(t, fmt.Sprintf("text%d", i), []byte(s), aztec.Options{}, s)
	}
}

func TestAztecBinary(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for _, n := range []int{1, 5, 31, 32, 62, 63, 100, 500, 1200} {
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(r.Intn(255) + 1) // gozxing's decoder drops NUL
		}
		aztecRoundTrip(t, fmt.Sprintf("bin%d", n), b, aztec.Options{}, latin1(b))
	}
	// binary interleaved with text
	b := []byte("ABC\xe9\xe8def\x80\x81GHI 123 \xff")
	aztecRoundTrip(t, "mixedbin", b, aztec.Options{}, latin1(b))
}

func TestAztecSizes(t *testing.T) {
	for l := 1; l <= 4; l++ {
		aztecRoundTrip(t, fmt.Sprintf("compact%d", l), []byte("Compact"), aztec.Options{Compact: true, Layers: l}, "Compact")
	}
	for l := 1; l <= 32; l++ {
		s := fmt.Sprintf("Full range %d layers", l)
		aztecRoundTrip(t, s, []byte(s), aztec.Options{Layers: l}, s)
	}
	// Fill symbols close to capacity.
	for _, n := range []int{10, 40, 100, 300, 800, 1500} {
		s := strings.Repeat("Z", n)
		aztecRoundTrip(t, fmt.Sprintf("fill%d", n), []byte(s), aztec.Options{}, s)
	}
}

func TestAztecECPercent(t *testing.T) {
	for _, p := range []int{1, 10, 23, 50, 80, 90} {
		s := "Error correction percentage"
		aztecRoundTrip(t, fmt.Sprintf("ec%d", p), []byte(s), aztec.Options{ECPercent: p}, s)
	}
}

func TestAztecZPLParams(t *testing.T) {
	for _, size := range []int{0, 23, 101, 104, 201, 215} {
		p := aztec.ZPLParams{Size: size}
		o, err := p.Options()
		if err != nil {
			t.Fatal(err)
		}
		aztecRoundTrip(t, fmt.Sprintf("zpl%d", size), []byte("ZPL ^B0"), o, "ZPL ^B0")
	}
}

func TestAztecMenu(t *testing.T) {
	// gozxing ignores the reader-initialisation flag in the mode message and
	// so misreads the data codeword count; menu symbols were verified with
	// zxing-cpp instead (it reports ReaderInit=true).
	t.Skip("gozxing does not support Aztec reader initialisation symbols")
	aztecRoundTrip(t, "menu-compact", []byte("MENU"), aztec.Options{Menu: true}, "MENU")
	s := strings.Repeat("menu data ", 30)
	aztecRoundTrip(t, "menu-full", []byte(s), aztec.Options{Menu: true}, s)
}

func TestAztecECI(t *testing.T) {
	// ECI 7 = ISO 8859-5: 0xE0 0xE1 are Cyrillic small er, es.
	aztecRoundTrip(t, "eci7", []byte("A\\000007\xe0\xe1"), aztec.Options{ECI: true}, "A\u0440\u0441")
	// ECI 26 = UTF-8.
	aztecRoundTrip(t, "eci26", []byte("\\000026caf\xc3\xa9"), aztec.Options{ECI: true}, "caf\u00e9")
	aztecRoundTrip(t, "backslash", []byte(`a\\b`), aztec.Options{ECI: true}, `a\b`)
	aztecRoundTrip(t, "noeci", []byte(`a\000026b`), aztec.Options{}, `a\000026b`)
}

func TestAztecStructuredAppend(t *testing.T) {
	// gozxing does not interpret structured append, so the header (" ID "
	// plus position and count letters) shows up in its decoded text.
	parts, err := aztec.EncodeStructured([]byte("ABCDEFGHI"), aztec.Options{SACount: 3, SAID: "ID42"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{" ID42 ACABC", " ID42 BCDEF", " ID42 CCGHI"}
	for i, m := range parts {
		got, err := Decode(MatrixImage(m, 4, 0, 4), gozxing.BarcodeFormat_AZTEC)
		if err != nil {
			t.Errorf("part %d: %v", i, err)
			continue
		}
		if got != want[i] {
			t.Errorf("part %d: %q want %q", i, got, want[i])
		}
	}
	aztecRoundTrip(t, "sa-noid", []byte("hello"), aztec.Options{SACount: 26, SAIndex: 26}, "ZZhello")
}
