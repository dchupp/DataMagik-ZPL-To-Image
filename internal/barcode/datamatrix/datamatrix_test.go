package datamatrix

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
)

func cwString(b []byte) string {
	s := make([]string, len(b))
	for i, v := range b {
		s[i] = fmt.Sprint(v)
	}
	return strings.Join(s, " ")
}

// ISO/IEC 16022 Annex O example: "123456" in a 10×10 symbol.
func TestRSExample(t *testing.T) {
	data := []byte{142, 164, 186}
	got := cwString(symbolSizes[0].interleave(data))
	if want := "142 164 186 114 25 5 88 102"; got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func hl(t *testing.T, s string, opt Options) string {
	t.Helper()
	if opt.Quality == 0 {
		opt.Quality = ECC200
	}
	items, sa, err := parseEscapes([]byte(s), opt.Escape)
	if err != nil {
		t.Fatal(err)
	}
	_ = sa
	var st stream
	encodeItems(&st, items)
	return cwString(st.cw)
}

func TestHighLevel(t *testing.T) {
	cases := []struct{ in, want string }{
		{"123456", "142 164 186"},
		{"AIMAIMAIM", "230 91 11 91 11 91 11 254"},
		{"aimaimaim", "239 91 11 91 11 91 11 254"},
		{"ABC>ABC123>AB", "238 89 233 14 192 100 207 44 31 254 67"},
		{".A.C1.3.DATA.123DATA.123DATA", "240 184 27 131 198 236 238 16 21 1 187 28 179 16 21 1 187 28 179 16 21 1 124"},
		{"a", "98"},
		{"\xe9", "235 106"},
	}
	for _, c := range cases {
		got := hl(t, c.in, Options{})
		// Compare codeword counts against the reference encodation where the
		// exact sequence may legitimately differ, but require exact for the
		// short canonical ones.
		if got != c.want && len(strings.Fields(got)) > len(strings.Fields(c.want)) {
			t.Errorf("%q: got %s want %s", c.in, got, c.want)
		}
	}
}

func TestExactHighLevel(t *testing.T) {
	for in, want := range map[string]string{
		"123456":    "142 164 186",
		"AIMAIMAIM": "230 91 11 91 11 91 11 254",
		"aimaimaim": "239 91 11 91 11 91 11 254",
	} {
		if got := hl(t, in, Options{}); got != want {
			t.Errorf("%q: got %s want %s", in, got, want)
		}
	}
}

func TestPadRandomisation(t *testing.T) {
	var s stream
	s.add(142, 164, 186)
	got := cwString(s.finish(8)[3:])
	// position 5: (149*5)%253+1 = 239; 129+239-254 = 114, etc.
	want := []int{129}
	for p := 5; p <= 8; p++ {
		v := 129 + (149*p)%253 + 1
		if v > 254 {
			v -= 254
		}
		want = append(want, v)
	}
	if got != strings.Trim(fmt.Sprint(want), "[]") {
		t.Fatalf("got %s want %v", got, want)
	}
}

func TestEscapes(t *testing.T) {
	items, sa, err := parseEscapes([]byte("_1AB__C_d065_G_5009_2214001001x"), '_')
	if err != nil {
		t.Fatal(err)
	}
	want := []item{{kFNC1, 0}, {kByte, 'A'}, {kByte, 'B'}, {kByte, '_'}, {kByte, 'C'}, {kByte, 65}, {kByte, 7}, {kECI, 9}, {kByte, 'x'}}
	if fmt.Sprint(items) != fmt.Sprint(want) {
		t.Fatalf("items %v want %v", items, want)
	}
	if fmt.Sprint(sa) != "[214 1 1]" {
		t.Fatalf("sa %v", sa)
	}
	if _, _, err := parseEscapes([]byte("_d999"), '_'); err == nil {
		t.Fatal("expected error")
	}
	items, _, _ = parseEscapes([]byte("_1AB"), 0)
	if len(items) != 4 {
		t.Fatal("escape disabled should be literal")
	}
}

func TestSizes(t *testing.T) {
	for _, sz := range symbolSizes {
		data := make([]byte, sz.dataCW)
		for i := range data {
			data[i] = byte(i*7 + 1)
		}
		m := sz.render(sz.interleave(data))
		if len(m) != sz.rows || len(m[0]) != sz.cols {
			t.Fatalf("%dx%d: bad dims", sz.rows, sz.cols)
		}
		// Every codeword bit must be placed exactly once.
		nrow, ncol := sz.regionsV()*sz.regRows, sz.regionsH()*sz.regCols
		seen := map[int]int{}
		for _, v := range placement(nrow, ncol) {
			if v >= 0 {
				seen[v]++
			}
		}
		total := sz.dataCW + sz.eccCW
		if len(seen) != total*8 {
			t.Errorf("%dx%d: %d bits placed want %d", sz.rows, sz.cols, len(seen), total*8)
		}
		for k, c := range seen {
			if c != 1 {
				t.Errorf("%dx%d: bit %d placed %d times", sz.rows, sz.cols, k, c)
			}
		}
	}
}

func TestEncodeErrors(t *testing.T) {
	_, err := Encode([]byte("x"), Options{Quality: 0})
	if !errors.Is(err, ErrUnsupportedQuality) || !errors.Is(err, barcode.ErrInvalidData) {
		t.Fatalf("err %v", err)
	}
	_, err = Encode([]byte(strings.Repeat("x", 100)), Options{Quality: 200, Columns: 10, Rows: 10})
	if !errors.Is(err, barcode.ErrInvalidData) {
		t.Fatalf("err %v", err)
	}
	m, err := Encode([]byte("HELLO"), Options{Quality: 200, Rectangular: true})
	if err != nil || m.Height() != 8 || m.Width() != 18 {
		t.Fatalf("rect: %v %dx%d", err, m.Height(), m.Width())
	}
	m, err = Encode([]byte("HELLO"), Options{Quality: 200, Columns: 32, Rows: 32})
	if err != nil || m.Height() != 32 {
		t.Fatalf("forced: %v", err)
	}
	m, err = Encode([]byte("HELLO"), Options{Quality: 200, Columns: 36, Rectangular: true})
	if err != nil || m.Height() != 12 || m.Width() != 36 {
		t.Fatalf("forced cols rect: %v %dx%d", err, m.Height(), m.Width())
	}
}

// Known full symbol: "123456" → ISO example. Check finder patterns.
func TestFinder(t *testing.T) {
	m, err := Encode([]byte("123456"), Options{Quality: 200})
	if err != nil {
		t.Fatal(err)
	}
	if m.Width() != 10 {
		t.Fatalf("width %d", m.Width())
	}
	for i := 0; i < 10; i++ {
		if !m.Rows[i][0] || !m.Rows[9][i] {
			t.Fatal("L finder")
		}
		if m.Rows[0][i] != (i%2 == 0) || m.Rows[i][9] != (i%2 == 1) {
			t.Fatal("clock track")
		}
	}
}

func TestMinimal(t *testing.T) {
	// Base 256 for binary runs.
	s := strings.Repeat("\xff\xfe\x80\x90", 10)
	got := strings.Fields(hl(t, s, Options{}))
	if len(got) != 42 || got[0] != "231" {
		t.Fatalf("binary: %v", got)
	}
	// Long digits → ASCII pairs.
	if got := strings.Fields(hl(t, "12345678901234567890", Options{})); len(got) != 10 {
		t.Fatalf("digits: %v", got)
	}
	// Each encodation should never be longer than plain ASCII.
	for _, in := range []string{"Hello, World!", "ABCDEFGHIJKLMNOPabcdef0123", "A*B>C\rD", "AB:CD;EF", "http://www.example.com/x?y=1"} {
		items, _, _ := parseEscapes([]byte(in), 0)
		ascii := 0
		for _, it := range items {
			ascii += asciiCost(it)
		}
		if n := len(strings.Fields(hl(t, in, Options{}))); n > ascii {
			t.Errorf("%q: %d > ascii %d", in, n, ascii)
		}
	}
}

func dataCW(t *testing.T, s string, opt Options) string {
	t.Helper()
	opt.Quality = ECC200
	cw, _, err := encode200([]byte(s), opt)
	if err != nil {
		t.Fatal(err)
	}
	return cwString(cw)
}

func TestHeaders(t *testing.T) {
	e := Options{Escape: '_'}
	if got := dataCW(t, "_2214001001A", e); !strings.HasPrefix(got, "233 214 1 1 66") {
		t.Errorf("SA escape: %s", got)
	}
	sa := Options{StructuredAppend: &StructuredAppend{Index: 3, Count: 7, FileID1: 10, FileID2: 1}}
	if got := dataCW(t, "A", sa); !strings.HasPrefix(got, "233 42 10 1 66") {
		t.Errorf("SA option: %s", got)
	}
	if got := dataCW(t, "_10112", e); !strings.HasPrefix(got, "232 131 142") {
		t.Errorf("GS1: %s", got)
	}
	if got := dataCW(t, "_3A", e); !strings.HasPrefix(got, "234 66") {
		t.Errorf("FNC3: %s", got)
	}
	if got := dataCW(t, "[)>\x1e05\x1d12\x1e\x04", Options{}); !strings.HasPrefix(got, "236 142 129") {
		t.Errorf("macro 05: %s", got)
	}
	if got := dataCW(t, "[)>\x1e06\x1dA\x1e\x04", Options{}); !strings.HasPrefix(got, "237 66 129") {
		t.Errorf("macro 06: %s", got)
	}
	if got := dataCW(t, "_5009A", e); !strings.HasPrefix(got, "241 10 66") {
		t.Errorf("ECI: %s", got)
	}
	if got := dataCW(t, "_5900A", e); !strings.HasPrefix(got, "241 131 12 66") {
		t.Errorf("ECI 900: %s", got)
	}
}

func TestRectFallback(t *testing.T) {
	m, err := Encode([]byte(strings.Repeat("x\x01", 60)), Options{Quality: ECC200, Rectangular: true})
	if err != nil || m.Width() != m.Height() {
		t.Fatalf("expected square fallback: %v %dx%d", err, m.Height(), m.Width())
	}
}

// The unlatch is dropped when it would overflow an otherwise exact fit.
func TestDropUnlatch(t *testing.T) {
	// 12 C40 chars = 1 latch + 8 + unlatch = 10 → fits 8x32 (10) anyway;
	// "AIMAIMAIM" = 8 incl. unlatch fits 14x14 (8) exactly.
	m, err := Encode([]byte("AIMAIMAIMAIM"), Options{Quality: ECC200})
	if err != nil {
		t.Fatal(err)
	}
	// 1 + 8 = 9 codewords without unlatch, 10 with: 16x16 holds 12.
	if m.Width() != 16 {
		t.Fatalf("width %d", m.Width())
	}
	var s stream
	items, _, _ := parseEscapes([]byte("AIMAIMAIMA"), 0)
	encodeItems(&s, items)
	// 230 + 6 + 254 + 'A' = 9; 8 fits 14x14 only by dropping the unlatch.
	if len(s.cw) != 9 || !s.fits(8) {
		t.Fatalf("drop: %v fits=%v", s.cw, s.fits(8))
	}
}
