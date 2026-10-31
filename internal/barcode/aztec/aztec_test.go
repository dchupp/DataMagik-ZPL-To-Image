package aztec

import (
	"errors"
	"strings"
	"testing"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
)

func bitString(b []bool) string {
	var sb strings.Builder
	for _, v := range b {
		if v {
			sb.WriteByte('X')
		} else {
			sb.WriteByte('.')
		}
	}
	return sb.String()
}

func items(s string) []item {
	it, _ := parseItems([]byte(s), false)
	return it
}

// Hand-checked vectors (style of ZXing's Aztec EncoderTest), annotated
// token by token.
func TestHighLevelEncode(t *testing.T) {
	cases := []struct{ in, want string }{
		// 'A'  P/S   '. ' L/L    b    D/L    '.'
		{"A. b.", "...X. ..... ...XX XXX.. ...XX XXXX. XX.X"},
		// 'L'  L/L   'o'   'r'   'e'   'm'   ' '   'i'   'p'   's'   'u'   'm'   D/L   '.'
		{"Lorem ipsum.", ".XX.X XXX.. X.... X..XX ..XX. .XXX. ....X .X.X. X...X X.X.. X.XX. .XXX. XXXX. XX.X"},
		// 'L'  L/L   'o'   P/S   '. '  U/S   'T'   'e'   's'   't'    D/L   ' '  '1'  '2'  '3'  '.'
		{"Lo. Test 123.", ".XX.X XXX.. X.... ..... ...XX XXX.. X.X.X ..XX. X.X.. X.X.X XXXX. ...X ..XX .X.. .X.X XX.X"},
		// 'L'  L/L   'o'   D/L   '.'  '.'  '.'  U/L  L/L   'x'
		{"Lo...x", ".XX.X XXX.. X.... XXXX. XX.X XX.X XX.X XXX. XXX.. XX..X"},
		// P/S   '. '  L/L   'x'   P/S   ':'   P/S   '/'   P/S   '/'   'a'   'b'   'c'   P/S   '/'   D/L   '.'
		{". x://abc/.", "..... ...XX XXX.. XX..X ..... X.X.X ..... X.X.. ..... X.X.. ...X. ...XX ..X.. ..... X.X.. XXXX. XX.X"},
		// 'N'  B/S    =1   '\0'      N
		{"N\x00N", ".XXXX XXXXX ....X ........ .XXXX"},
		// 'N'  B/S    =2   '\0'       'n'
		{"N\x00n", ".XXXX XXXXX ...X. ........ .XX.XXX."},
	}
	for _, c := range cases {
		got := bitString(highLevelEncode(items(c.in)))
		want := strings.ReplaceAll(c.want, " ", "")
		if got != want {
			t.Errorf("%q:\n got %s\nwant %s", c.in, got, want)
		}
	}
}

func TestBinaryShiftLengths(t *testing.T) {
	// Binary shift header sizes: 1-31 bytes => 10 bits, 32-62 => two
	// 10-bit headers, >62 => 21 bits (B/S, 00000, 11-bit length-31).
	for _, n := range []int{1, 31, 32, 62, 63, 2000} {
		in := strings.Repeat("\x80", n)
		bits := highLevelEncode(items(in))
		hdr := 10
		switch {
		case n > 62:
			hdr = 21
		case n > 31:
			hdr = 20
		}
		if len(bits) != hdr+8*n {
			t.Errorf("n=%d: %d bits, want %d", n, len(bits), hdr+8*n)
		}
	}
}

func TestECIFlag(t *testing.T) {
	// "\000026A": P/S FLG(2) '2' '6' then 'A' in Upper.
	it, _ := parseItems([]byte(`\000026A`), true)
	got := bitString(highLevelEncode(it))
	want := "....." + "....." + ".X." + ".X.." + "X..." + "...X."
	if got != want {
		t.Errorf("got %s want %s", got, want)
	}
	it, _ = parseItems([]byte(`a\\b`), true)
	if len(it) != 3 || it[1].b != '\\' {
		t.Errorf("escaped backslash parsed as %+v", it)
	}
}

func TestReedSolomonGF16(t *testing.T) {
	// Mode message of a compact 1-layer symbol with 1 data word: data
	// nibbles 0,0 give all-zero check words (RS is linear).
	if c := rsEncode(4, []int{0, 0}, 5); c[0]|c[1]|c[2]|c[3]|c[4] != 0 {
		t.Errorf("zero message gave %v", c)
	}
	// The codeword must evaluate to zero at a^1..a^n.
	f := field(4)
	data := []int{5, 3}
	cw := append(append([]int{}, data...), rsEncode(4, data, 5)...)
	for i := 1; i <= 5; i++ {
		r := f.exp[i]
		v := 0
		for _, c := range cw {
			v = f.mul(v, r) ^ c
		}
		if v != 0 {
			t.Errorf("syndrome %d = %d", i, v)
		}
	}
}

func TestReedSolomonSyndromes(t *testing.T) {
	for _, bits := range []int{6, 8, 10, 12} {
		f := field(bits)
		data := []int{1, 2, 3, f.size - 1, 7, 0, 9}
		cw := append(append([]int{}, data...), rsEncode(bits, data, 6)...)
		for i := 1; i <= 6; i++ {
			v := 0
			for _, c := range cw {
				v = f.mul(v, f.exp[i]) ^ c
			}
			if v != 0 {
				t.Errorf("GF(2^%d) syndrome %d = %d", bits, i, v)
			}
		}
	}
}

func TestStuffBits(t *testing.T) {
	// 6-bit words: "000000" -> 000001 + carry, "111111" -> 111110 + carry.
	in := []bool{false, false, false, false, false, false, true, true, true, true, true, true}
	got := stuffBits(in, 6)
	if got[0] != 0b000001 {
		t.Fatalf("first word %06b", got[0])
	}
	// second word starts at bit 5: 0 11111 -> 011111 (no stuffing)
	if got[1] != 0b011111 {
		t.Fatalf("second word %06b", got[1])
	}
	// remainder "1" padded with ones -> 111111 -> stuffed 111110
	if got[2] != 0b111110 {
		t.Fatalf("third word %06b", got[2])
	}
}

func TestSizes(t *testing.T) {
	want := map[[2]int]int{{1, 1}: 15, {1, 4}: 27, {0, 1}: 19, {0, 4}: 31, {0, 5}: 37, {0, 32}: 151, {0, 22}: 109}
	for k, v := range want {
		if got := symbolSize(k[0] == 1, k[1]); got != v {
			t.Errorf("%v: size %d want %d", k, got, v)
		}
	}
	for _, s := range []int{101, 102, 103, 104, 201, 210, 232} {
		m, err := EncodeZPL([]byte("ABC"), ZPLParams{Size: s})
		if err != nil {
			t.Fatal(err)
		}
		compact := s < 200
		if m.Width() != symbolSize(compact, s%100) || m.Height() != m.Width() {
			t.Errorf("size %d: %dx%d", s, m.Width(), m.Height())
		}
	}
}

func TestRune(t *testing.T) {
	m, err := EncodeRune(0)
	if err != nil || m.Width() != 11 || m.Height() != 11 {
		t.Fatalf("rune: %v %d", err, m.Width())
	}
	// Rune 0: data nibbles 0 -> check words 0; every even bit inverted.
	b := runeBits(0)
	if bitString(b) != strings.Repeat("X.", 14) {
		t.Errorf("rune 0 bits %s", bitString(b))
	}
	if _, err := EncodeRune(256); !errors.Is(err, barcode.ErrInvalidData) {
		t.Error("rune 256 accepted")
	}
	if _, err := EncodeZPL([]byte("x"), ZPLParams{Size: 300}); !errors.Is(err, barcode.ErrInvalidData) {
		t.Error("non-numeric rune accepted")
	}
}

func TestModeMessageMenu(t *testing.T) {
	mm := modeMessage(true, 1, 5, true)
	// layers-1 = 00, menu bit, data words-1 = 00100
	if got := bitString(mm[:8]); got != "..X..X.." {
		t.Errorf("compact menu mode message %s", got)
	}
	mm = modeMessage(false, 3, 2, true)
	if got := bitString(mm[:16]); got != "...X."+"X"+".........X" {
		t.Errorf("full menu mode message %s", got)
	}
}

func TestErrors(t *testing.T) {
	bad := []struct {
		data string
		p    ZPLParams
	}{
		{"", ZPLParams{}},
		{"x", ZPLParams{Size: 105}},
		{"x", ZPLParams{Size: 233}},
		{"x", ZPLParams{Size: 100}},
		{strings.Repeat("A", 100), ZPLParams{Size: 101}},
		{strings.Repeat("A", 4000), ZPLParams{}},
		{"x", ZPLParams{Symbols: 27}},
		{"x", ZPLParams{Symbols: 2, ID: "has space"}},
		{"x", ZPLParams{Size: 102, Menu: true}},
	}
	for _, b := range bad {
		if _, err := EncodeZPL([]byte(b.data), b.p); !errors.Is(err, barcode.ErrInvalidData) {
			t.Errorf("%q %+v: err %v", b.data, b.p, err)
		}
	}
}

func TestMenuAutoSize(t *testing.T) {
	// Too much data for compact 1 layer: menu symbols skip compact 2-4.
	m, err := Encode([]byte(strings.Repeat("A", 20)), Options{Menu: true})
	if err != nil {
		t.Fatal(err)
	}
	if m.Width() == 19 || m.Width() == 23 || m.Width() == 27 {
		// 19/23/27 are also full-range 1-3 sizes; ensure not compact by
		// checking the mode message ring position (full: ring at c-7).
		c := m.Width() / 2
		if !m.Rows[c-7][c-7] {
			t.Errorf("expected full-range bullseye corner")
		}
	}
}
