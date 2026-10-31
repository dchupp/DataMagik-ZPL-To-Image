package code49

import (
	"errors"
	"fmt"
	"math/bits"
	"strings"
	"testing"

	"github.com/datamagik/zpl-to-image/internal/barcode"
)

func flat(g [][8]int) string {
	var s []string
	for _, r := range g {
		for _, v := range r {
			s = append(s, fmt.Sprint(v))
		}
	}
	return strings.Join(s, " ")
}

// Code character grids for the ANSI/AIM BC6-2000 worked examples (2.2.2
// numeric examples, 2.3.7 "EXAMPLE 2") and shift/mode cases, as tabulated
// in zint's backend/tests/test_code49.c (long grids are compared by prefix).
func TestCodewords(t *testing.T) {
	tests := []struct{ in, want string }{
		{"EXAMPLE 2", "14 33 10 22 25 21 14 41 38 2 35 14 18 13 0 22"},
		{"12345", "5 17 9 48 48 48 48 27 48 48 13 23 0 13 2 0"},
		{"123456", "5 17 9 6 48 48 48 34 48 48 36 9 23 41 2 11"},
		{"12345678", "5 17 9 14 6 48 48 0 48 48 25 42 2 17 2 37"},
		{"123456789", "5 17 9 46 16 37 48 31 48 48 7 26 9 39 2 32"},
		{"1234567", "43 45 2 11 39 48 48 40 48 48 33 36 38 6 2 15"},
		{"\037", "5 48 48 48 48 48 48 48 48 48 4 33 13 15 4 18"},
		{"\000\037", "38 43 5 48 48 48 48 33 48 48 45 7 38 43 4 37"},
		{"a\000", "10 43 38 48 48 48 48 38 48 48 32 33 14 15 5 48"},
		{"ab", "10 44 11 48 48 48 48 12 48 48 27 39 42 0 5 13"},
		{"\000A\000a\000", "38 10 43 38 44 10 43 30 38 48 25 23 38 32 4 12"},
		{"1234\037aA12345A", "1 2 3 4 43 5 44 4 10 10 48 5 17 9 48 0 10 48 19 2 13 32 7 33"},
		{"1234567890123456789012345678901234567890", "5 17 9 29 22 18 5 7 17 9 29 22 18 5 17 19 9 29 22 18 5 17 9 11 29 22 18 48 48 48 48 16"},
	}
	for _, tc := range tests {
		s, err := Encode(tc.in, Options{})
		if err != nil {
			t.Fatalf("%q: %v", tc.in, err)
		}
		if got := flat(s.Codewords); !strings.HasPrefix(got, tc.want) {
			t.Errorf("%q:\n got %s\nwant %s", tc.in, got, tc.want)
		}
		if s.Text != tc.in || decodeCodewords(dataCodewords(s), s.Mode) != tc.in {
			t.Errorf("%q: text %q, decoded %q", tc.in, s.Text, decodeCodewords(dataCodewords(s), s.Mode))
		}
	}
}

// dataCodewords returns the data area code characters of a symbol.
func dataCodewords(s Symbol) []int {
	var cw []int
	n := len(s.Codewords)
	for r, row := range s.Codewords {
		k := 7
		if r == n-1 {
			k = 0
			if n <= 6 {
				k = 2
			}
		}
		cw = append(cw, row[:k]...)
	}
	return cw
}

// Rows of the AIM specification's Figure 1 and Figure 3 symbols.
func TestSpecFigures(t *testing.T) {
	tests := []struct {
		in   string
		rows []string
	}{
		{"MULTIPLE ROWS IN CODE 49", []string{
			"1011111011001011101011100110000110111101011011111010111101000100001111",
			"1010100001000010001001111000101110100110001111010010001011100011001111",
			"1011001100000101101101110111000010110010110000111011101011110001101111",
			"1010011001100100001111010010001100101011101111110011010001001111101111",
			"1011001111001011101000000101001110111110111010001011010001101111101111",
		}},
		{"EXAMPLE 2", []string{
			"1011000111011100101111001001000110110011110010100010001111000100101111",
			"1011000100110010001100010110010000100001101001111010000001001011101111",
		}},
	}
	for _, tc := range tests {
		s, err := Encode(tc.in, Options{RowHeight: 10})
		if err != nil {
			t.Fatal(err)
		}
		m := s.Matrix
		if want := 2*len(tc.rows) + 1; len(m.Rows) != want {
			t.Fatalf("%q: %d matrix rows, want %d", tc.in, len(m.Rows), want)
		}
		for i, want := range tc.rows {
			var b strings.Builder
			for _, d := range m.Rows[2*i+1] {
				b.WriteByte("01"[btoi(d)])
			}
			if b.String() != want {
				t.Errorf("%q row %d:\n got %s\nwant %s", tc.in, i, b.String(), want)
			}
		}
		for i, h := range m.RowHeights {
			sep := i%2 == 0
			if (sep && (h != 1 || strings.Contains(fmt.Sprint(m.Rows[i]), "false"))) || (!sep && h != 10) {
				t.Errorf("%q: row %d height %d", tc.in, i, h)
			}
		}
	}
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// EXAMPLE 2 symbol character values and parities from AIM 2.3.7 steps 11–12.
func TestExample2Values(t *testing.T) {
	s, _ := Encode("EXAMPLE 2", Options{})
	var w []int
	for _, r := range s.Codewords {
		for j := range 4 {
			w = append(w, r[2*j]*49+r[2*j+1])
		}
	}
	if fmt.Sprint(w) != "[719 512 1246 727 1864 1729 895 22]" {
		t.Errorf("symbol characters %v", w)
	}
}

// Every pattern is a 4-bar/4-space character starting with a bar, ending
// with a space, elements 1–6 modules, of the right parity, and all 4802
// patterns are distinct.
func TestPatternTables(t *testing.T) {
	seen := map[uint16]bool{}
	for parity, table := range [][2401]uint16{evenPatterns, oddPatterns} {
		for v, p := range table {
			if p&0x8000 == 0 || p&1 != 0 || seen[p] {
				t.Fatalf("parity %d value %d: bad pattern %016b", parity, v, p)
			}
			seen[p] = true
			var runs []int
			prev, n := true, 0
			for b := 15; b >= 0; b-- {
				d := p>>b&1 == 1
				if d != prev {
					runs = append(runs, n)
					n = 0
					prev = d
				}
				n++
			}
			runs = append(runs, n)
			barSum := 0
			for i, r := range runs {
				if r < 1 || r > 6 {
					t.Fatalf("value %d: element width %d", v, r)
				}
				if i%2 == 0 {
					barSum += r
				}
			}
			if len(runs) != 8 || barSum%2 != parity || bits.OnesCount16(p) != barSum {
				t.Fatalf("parity %d value %d: %d elements, bars %d", parity, v, len(runs), barSum)
			}
		}
	}
}

// Round trip: read the modules back through the pattern tables and check
// row parity, row checks, mode/row count and the decoded text.
func TestRoundTrip(t *testing.T) {
	rev := map[uint16][2]int{}
	for v, p := range evenPatterns {
		rev[p] = [2]int{v, 0}
	}
	for v, p := range oddPatterns {
		rev[p] = [2]int{v, 1}
	}
	inputs := []string{"A", "Hello, World!", "0123456789", "ABC12345678901DEF", "x~\x7f\x00{}|", strings.Repeat("9", 81), "MULTIPLE ROWS IN CODE 49", strings.Repeat("Ab1", 8)}
	for _, in := range inputs {
		s, err := Encode(in, Options{NoBorder: true})
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		rows := len(s.Codewords)
		if rows < 2 || rows > 8 || len(s.Matrix.Rows) != 2*rows-1 || s.Matrix.Width() != 70 {
			t.Fatalf("%q: %d rows", in, rows)
		}
		var cw []int
		for r := range rows {
			m := s.Matrix.Rows[2*r]
			for j := range 4 {
				var p uint16
				for b := range 16 {
					p = p<<1 | uint16(btoi(m[2+16*j+b]))
				}
				vp, ok := rev[p]
				wantOdd := r != rows-1 && rowParity[r][j]
				if !ok || (vp[1] == 1) != wantOdd {
					t.Fatalf("%q row %d char %d: pattern %v", in, r, j, vp)
				}
				cw = append(cw, vp[0]/49, vp[0]%49)
			}
			row := cw[8*r : 8*r+8]
			sum := 0
			for _, c := range row[:7] {
				sum += c
			}
			if sum%49 != row[7] {
				t.Errorf("%q row %d: bad row check", in, r)
			}
		}
		last := cw[8*(rows-1):]
		if last[6] != 7*(rows-2)+s.Mode {
			t.Errorf("%q: row count char %d", in, last[6])
		}
		if got := decodeCodewords(dataCodewords(s), s.Mode); got != in {
			t.Errorf("%q: decoded %q", in, got)
		}
	}
}

func TestZebraModes(t *testing.T) {
	tests := []struct {
		mode      byte
		in, text  string
		mValue    int
		codewords string // data code characters (first ones)
	}{
		{'0', "ABC 12", "ABC 12", 0, "10 11 12 38 1 2"},
		{'0', ">A>B", "ab", 0, "44 10 44 11"},
		{'0', "<A", "\x01", 0, "43 10"},
		{'0', "AB=12345", "AB12345", 0, "10 11 48 5 17 9"},
		{'0', "ABc", "AB", 0, "10 11"}, // lower case invalid: data stops
		{'0', "A<>B", "A", 0, "10"},    // two shifts invalid
		{'0', "A>%B", "A~B", 0, "10 44 42 11"},
		{'0', "A<%B", "A", 0, "10"}, // S1 % reserved
		{'1', "ABC", "ABC", 1, "10 11 12"},
		{'2', "12345=AB", "12345AB", 2, "5 17 9 48 10 11"},
		{'2', "1234567", "1234567", 2, "43 45 2 11 39"},
		{'2', "12345AB", "12345", 2, "5 17 9"}, // numeric ended by non-NS
		{'3', "1ABC", "1ABC", 3, "1 10 11 12"},
		{'4', "AB", "\x01B", 4, "10 11"},
		{'5', "AB", "aB", 5, "10 11"},
		{'0', "A:B;C?D", "ABCD", 0, "10 45 11 46 12 47 13"},
	}
	for _, tc := range tests {
		s, err := Encode(tc.in, Options{Mode: tc.mode})
		if err != nil {
			t.Fatalf("mode %c %q: %v", tc.mode, tc.in, err)
		}
		cw := dataCodewords(s)
		got := fmt.Sprint(cw)
		got = strings.Trim(got, "[]")
		if s.Text != tc.text || s.Mode != tc.mValue || !strings.HasPrefix(got, tc.codewords) {
			t.Errorf("mode %c %q: text %q mode %d codewords %s; want %q %d %s", tc.mode, tc.in, s.Text, s.Mode, got, tc.text, tc.mValue, tc.codewords)
		}
	}
}

func TestErrors(t *testing.T) {
	for _, tc := range []struct {
		in   string
		mode byte
	}{
		{"", 'A'},
		{"é", 'A'},
		{strings.Repeat("A", 50), 'A'},
		{"abc", '0'},
		{"A", 'X'},
	} {
		if _, err := Encode(tc.in, Options{Mode: tc.mode}); !errors.Is(err, barcode.ErrInvalidData) {
			t.Errorf("%q mode %c: err %v", tc.in, tc.mode, err)
		}
	}
	if _, err := Encode(strings.Repeat("A", 49), Options{}); err != nil {
		t.Errorf("49 code characters: %v", err)
	}
}
