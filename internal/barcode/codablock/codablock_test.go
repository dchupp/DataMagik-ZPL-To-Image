package codablock

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/code39"
)

func bitsString(r []bool) string {
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

func dataRows(m barcode.Matrix) []string {
	var out []string
	for y := 1; y < len(m.Rows); y += 2 {
		out = append(out, bitsString(m.Rows[y]))
	}
	return out
}

// TestCodablockFVectors reproduces AIM ISS-X-24 figures and reference
// symbols (row content incl. row indicators, K1/K2, row checks).
func TestCodablockFVectors(t *testing.T) {
	for _, v := range codablockFVectors {
		cols := (len(v.rows[0]) - 57) / 11
		m, err := Encode([]byte(v.data), Options{Mode: ModeF, Columns: cols})
		if err != nil {
			t.Fatalf("%s: %v", v.name, err)
		}
		if got := dataRows(m); !reflect.DeepEqual(got, v.rows) {
			t.Errorf("%s:\n got %q\nwant %q", v.name, got, v.rows)
		}
	}
}

// decodeCode39Row reads a Codablock A data row (Narrow 1, ratio 3) back to
// its Code 39 characters.
func decodeCode39Row(t *testing.T, row []bool) string {
	var widths []int
	for i := 0; i < len(row); {
		j := i
		for j < len(row) && row[j] == row[i] {
			j++
		}
		widths = append(widths, j-i)
		i = j
	}
	var s strings.Builder
	for k := 0; k+9 <= len(widths); k += 10 {
		var got []int
		for _, w := range widths[k : k+9] {
			if w == 3 {
				got = append(got, 2)
			} else {
				got = append(got, 1)
			}
		}
		found := false
		for i := 0; i <= code39.StartStop; i++ {
			if slices.Equal(code39.Elements(i), got) {
				if i == code39.StartStop {
					s.WriteByte('*')
				} else {
					s.WriteByte(code39.Chars[i])
				}
				found = true
			}
		}
		if !found {
			t.Fatalf("bad Code 39 character %v", got)
		}
	}
	return s.String()
}

// TestCodablockA checks row indicators, filler and the K1/K2 characters.
// The same rows were decoded as Code 39 by zxing-cpp ("PTHE QUICK P",
// "0BROWN FOX 0", "1JUMPS OVER1", "2 THE LAZY 2", "3DOG     FN3").
func TestCodablockA(t *testing.T) {
	m, err := Encode([]byte("THE QUICK BROWN FOX JUMPS OVER THE LAZY DOG"), Options{Mode: ModeA, Columns: 10})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"*PTHE QUICK P*", "*0BROWN FOX 0*", "*1JUMPS OVER1*", "*2 THE LAZY 2*", "*3DOG     FN3*"}
	var got []string
	for y := 1; y < len(m.Rows); y += 2 {
		got = append(got, decodeCode39Row(t, m.Rows[y]))
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rows %q want %q", got, want)
	}
	if len(m.RowHeights) != 11 || m.RowHeights[0] != 1 || m.RowHeights[1] != 8 {
		t.Errorf("row heights %v", m.RowHeights)
	}
	// Separators: top/bottom solid, interior solid between start and stop.
	for _, y := range []int{0, len(m.Rows) - 1} {
		if strings.Contains(bitsString(m.Rows[y]), "0") {
			t.Errorf("separator %d not solid", y)
		}
	}
	inner := bitsString(m.Rows[2])
	first := bitsString(m.Rows[1])
	sw := 16 // '*' width at ratio 3 (3 wide + 6 narrow) + gap
	if inner[:sw] != first[:sw] || inner[len(inner)-sw:] != first[len(first)-sw:] ||
		strings.Contains(inner[sw:len(inner)-sw], "0") {
		t.Errorf("interior separator %s", inner)
	}

	// Single row: no right indicator and no check characters.
	m, _ = Encode([]byte("ABC"), Options{Mode: ModeA})
	if got := decodeCode39Row(t, m.Rows[1]); got != "*LABC*" {
		t.Errorf("single row %q", got)
	}
	// A single row is not padded to Columns (a ZD420 prints
	// ^BBN,20,Y,6,,A^FDAB12 with four data characters).
	m, _ = Encode([]byte("AB12"), Options{Mode: ModeA, Columns: 6})
	if got := decodeCode39Row(t, m.Rows[1]); got != "*LAB12*" {
		t.Errorf("single row with columns %q", got)
	}
	// Wide elements follow barcode.WideWidth: at Narrow 1 and ratio 2.5
	// that row is 7 characters of 12 cells plus 6 gaps, 90 wide, and the
	// separators reach one cell further: 91 as on a ZD420.
	m, _ = Encode([]byte("AB12"), Options{Mode: ModeA, Columns: 6, Narrow: 1, Ratio: 2.5, RowDots: 17})
	if w := m.Width(); w != 91 || m.Rows[1][90] || !m.Rows[0][90] || !m.Rows[2][90] {
		t.Errorf("single row width %d, want 91 with separators reaching the last cell", w)
	}
	if m.RowHeights[1] != 17 {
		t.Errorf("RowDots: row height %d, want 17", m.RowHeights[1])
	}
	// s=N: no K1/K2, filler only.
	m, _ = Encode([]byte("ABCDEFG"), Options{Mode: ModeA, Columns: 4, NoCheck: true})
	if got := decodeCode39Row(t, m.Rows[3]); got != "*0EFG 0*" {
		t.Errorf("no-check row %q", got)
	}
	// Rows given: columns derived as (len + 2) / rows.
	m, _ = Encode([]byte("ABCDEFGHIJ"), Options{Mode: ModeA, Rows: 3})
	if got := decodeCode39Row(t, m.Rows[1]); got != "*WABCD W*" && len(m.Rows) != 7 {
		t.Errorf("rows=3: %q, %d matrix rows", got, len(m.Rows))
	}
	if _, err := Encode([]byte("abc"), Options{Mode: ModeA}); !errors.Is(err, barcode.ErrInvalidData) {
		t.Error("lower case accepted in Codablock A")
	}
	if _, err := Encode([]byte(strings.Repeat("A", 100)), Options{Mode: ModeA, Columns: 4}); !errors.Is(err, barcode.ErrInvalidData) {
		t.Error("23 rows accepted")
	}
}

// TestCodablockEF pins symbols whose rows zxing-cpp decoded as valid Code
// 128 (row check characters correct).
func TestCodablockEF(t *testing.T) {
	m, err := Encode([]byte("0112345678901231abc\x01def"), Options{Mode: ModeE, Columns: 10})
	if err != nil {
		t.Fatal(err)
	}
	// Row 1: Start A, Code C selector (FNC1 followed by digits), ...
	rows := dataRows(m)
	if len(rows) != 2 || !strings.HasPrefix(rows[0], "11010000100"+"10111011110") {
		t.Errorf("E rows %q", rows)
	}
	w := 11*(10+4) + 13
	for _, r := range rows {
		if len(r) != w {
			t.Errorf("row width %d want %d", len(r), w)
		}
	}
	// Zebra ^FH function characters and extended ASCII.
	for _, d := range []string{"A\x80B\x81C\x82D\x83E\x84F", "Caf\xe9 \xff"} {
		if _, err := Encode([]byte(d), Options{Mode: ModeF, Columns: 6}); err != nil {
			t.Errorf("%q: %v", d, err)
		}
	}
	if _, err := Encode([]byte("Caf\xe9"), Options{Mode: ModeE}); !errors.Is(err, barcode.ErrInvalidData) {
		t.Error("extended byte accepted in Codablock E")
	}
	// Defaults: at least two rows; Narrow scales the matrix.
	m1, _ := Encode([]byte("HELLO"), Options{})
	m2, _ := Encode([]byte("HELLO"), Options{Narrow: 3, RowHeight: 10, SepHeight: 2})
	if len(m1.Rows) < 5 || m2.Width() != 3*m1.Width() || m2.RowHeights[0] != 6 || m2.RowHeights[1] != 30 {
		t.Errorf("defaults/scaling: %d rows, widths %d %d, heights %v", len(m1.Rows), m1.Width(), m2.Width(), m2.RowHeights)
	}
	if _, err := Encode([]byte("x"), Options{Columns: 63}); !errors.Is(err, barcode.ErrInvalidData) {
		t.Error("63 columns accepted")
	}
	if _, err := Encode([]byte(strings.Repeat("ab", 400)), Options{Columns: 4}); !errors.Is(err, barcode.ErrInvalidData) {
		t.Error("45 rows accepted")
	}
}

func TestDerivedColumnsCapped(t *testing.T) {
	data := []byte(strings.Repeat("ABC123", 34)) // 204 characters
	for _, rows := range []int{0, 2} {
		m, err := Encode(data, Options{Mode: ModeA, Rows: rows})
		if err != nil {
			t.Fatalf("rows=%d: %v", rows, err)
		}
		// Each Code 39 character is 13 narrow modules wide (incl. the gap);
		// a row has start, row indicator, ≤62 data characters and stop.
		if w := m.Width(); w > (62+4)*16 {
			t.Errorf("rows=%d: width %d modules exceeds 62 columns", rows, w)
		}
	}
}
