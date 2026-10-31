package databar

import (
	"errors"
	"testing"

	"github.com/datamagik/zpl-to-image/internal/barcode"
)

func TestGTINForms(t *testing.T) {
	want := "(01)00012345678905"
	for _, in := range []string{"0001234567890", "1234567890", "00012345678905", "00012345678909", "(01)00012345678905", "[01]0001234567890", "0100012345678905"} {
		s, err := Encode(TypeOmnidirectional, in, Options{})
		if err != nil || s.Text != want {
			t.Errorf("%q: text %q err %v, want %q", in, s.Text, err, want)
		}
	}
	for _, in := range []string{"", "12345x", "123456789012345", "(01)"} {
		if _, err := Encode(TypeOmnidirectional, in, Options{}); !errors.Is(err, barcode.ErrInvalidData) {
			t.Errorf("%q: err %v, want ErrInvalidData", in, err)
		}
	}
	if _, err := Limited("2001234567890", Options{}); !errors.Is(err, barcode.ErrInvalidData) {
		t.Errorf("Limited indicator 2: err %v", err)
	}
	if _, err := Encode(7, "12345678901", Options{}); !errors.Is(err, barcode.ErrInvalidData) {
		t.Errorf("type 7: err %v", err)
	}
}

func TestLinearShapes(t *testing.T) {
	sum := func(l barcode.Linear) int { return l.Width(0) }
	o, err := Omnidirectional("12345678901", Options{}) // ZPL guide ^BR example
	if err != nil {
		t.Fatal(err)
	}
	if len(o.Elements) != 45 || sum(o) != 95 || o.Ratio || o.Text != "(01)00123456789012" {
		t.Errorf("omni: %d elements, %d modules, text %q", len(o.Elements), sum(o), o.Text)
	}
	tr, _ := Truncated("12345678901", Options{})
	if len(tr.Elements) != len(o.Elements) {
		t.Errorf("truncated differs from omni")
	}
	l, err := Limited("1501234567890", Options{})
	if err != nil || len(l.Elements) != 45 || sum(l) != 73 {
		t.Errorf("limited: %v %d elements %d modules", err, len(l.Elements), sum(l))
	}
	e, err := Expanded("(01)90012345678908(3103)001750", Options{})
	if err != nil || sum(e) != 149 || len(e.Elements)%2 != 1 {
		t.Errorf("expanded: %v %d modules", err, sum(e))
	}
	s, _ := Encode(TypeOmnidirectional, "1", Options{})
	if s.LeftOffset != 1 || s.Height() != 33 || s.Matrix.Width() != 96 {
		t.Errorf("omni symbol: offset %d height %d width %d", s.LeftOffset, s.Height(), s.Matrix.Width())
	}
}

func TestStackedShapes(t *testing.T) {
	m, err := Stacked("0001234567890", Options{})
	if err != nil || m.Width() != 50 || len(m.Rows) != 3 || m.RowHeights[0] != 5 || m.RowHeights[1] != 1 || m.RowHeights[2] != 7 {
		t.Errorf("stacked: %v %v", err, m.RowHeights)
	}
	m, err = StackedOmnidirectional("0001234567890", Options{})
	if err != nil || len(m.Rows) != 5 || m.RowHeights[0] != 33 || m.RowHeights[4] != 33 {
		t.Errorf("stacked omni: %v %v", err, m.RowHeights)
	}
	data := "(01)00012345678905(10)ABCDEFGHIJKLMNOPQRSTUV(21)1234567890"
	s, err := Encode(TypeExpanded, data, Options{Segments: 4})
	if err != nil || s.Linear != nil || len(s.Matrix.Rows)%4 != 1 || len(s.SeparatorRows) != len(s.Matrix.Rows)/4*3 {
		t.Fatalf("expanded stacked: %v rows %d seps %v", err, len(s.Matrix.Rows), s.SeparatorRows)
	}
	for i, h := range s.Matrix.RowHeights {
		if (i%4 == 0) != (h == 34) {
			t.Errorf("row %d height %d", i, h)
		}
	}
	// Segments 22 (Zebra default), 0 and out-of-range values give one row.
	for _, seg := range []int{0, 22, 30, -2} {
		if s, err := Encode(TypeExpanded, data, Options{Segments: seg}); err != nil || s.Linear == nil {
			t.Errorf("segments %d: %v stacked=%v", seg, err, s.Linear == nil)
		}
	}
	if _, err := Expanded(data+"(91)ABCDEFGHIJKLMNOPQRSTUVWXYZ", Options{}); !errors.Is(err, barcode.ErrInvalidData) {
		t.Errorf("too long: %v", err)
	}
}

func TestLinkage(t *testing.T) {
	for _, ty := range []Type{TypeOmnidirectional, TypeTruncated, TypeStacked, TypeStackedOmnidirectional, TypeLimited} {
		a, _ := Encode(ty, "0361234567890", Options{})
		b, _ := Encode(ty, "0361234567890", Options{Linkage: true})
		if a.CompositeSeparator != nil || len(b.CompositeSeparator) != b.Matrix.Width() {
			t.Errorf("type %d: separator %d/%d", ty, len(b.CompositeSeparator), b.Matrix.Width())
		}
		if rowString(a.Matrix.Rows[0]) == rowString(b.Matrix.Rows[0]) {
			t.Errorf("type %d: linkage flag did not change the symbol", ty)
		}
	}
	// A composite Expanded Stacked keeps at least 4 symbol characters in
	// the first row even with 2 segments.
	s, err := Encode(TypeExpanded, "(01)00012345678905(10)ABCDEF", Options{Segments: 2, Linkage: true})
	if err != nil || s.Matrix.Width() != 102 {
		t.Errorf("linked stacked width %d err %v", s.Matrix.Width(), err)
	}
}

func TestParseElementString(t *testing.T) {
	tests := []struct{ in, reduced, text string }{
		{"(01)95012345678903(3103)000123", "01950123456789033103000123", "(01)95012345678903(3103)000123"},
		{"[10]AB(C)D[21]X", "10AB(C)D\x1d21X", "(10)AB(C)D(21)X"},
		{"(10)ABC(17)251231(21)9", "10ABC\x1d17251231219", "(10)ABC(17)251231(21)9"},
		{"\x1d0195012345678903", "0195012345678903", "0195012345678903"},
	}
	for _, tc := range tests {
		r, txt, err := ParseElementString(tc.in)
		if err != nil || r != tc.reduced || txt != tc.text {
			t.Errorf("%q: %q %q %v, want %q %q", tc.in, r, txt, err, tc.reduced, tc.text)
		}
	}
	for _, in := range []string{"", "(01)123", "(10)", "(1)23", "(17)2512"} {
		if _, _, err := ParseElementString(in); !errors.Is(err, barcode.ErrInvalidData) {
			t.Errorf("%q: err %v", in, err)
		}
	}
}

func TestExpMethod(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"(10)12A", 2},
		{"(01)00012345678905(10)ABC", 1},
		{"(01)00012345678909(10)ABC", 2}, // bad GTIN check digit: general purpose keeps data intact
		{"(01)90012345678908(3103)001750", 3},
		{"(01)90012345678908(3103)032768", 7},
		{"(01)90012345678908(3202)009999", 4},
		{"(01)90012345678908(3203)022767", 4},
		{"(01)90012345678908(3203)022768", 8},
		{"(01)90012345678908(3922)795", 5},
		{"(01)90012345678908(3932)0401234", 6},
		{"(01)90012345678908(3102)001750(11)100312", 7},
		{"(01)90012345678908(3202)001750(13)100312", 10},
		{"(01)90012345678908(3102)001750(15)100312", 11},
		{"(01)90012345678908(3202)001750(17)100312", 14},
		{"(01)90012345678908(3102)001750(17)101312", 1}, // invalid month
		{"(01)00012345678905(3103)001750", 1},           // indicator digit not 9
	}
	for _, tc := range tests {
		red, _, err := ParseElementString(tc.in)
		if err != nil {
			t.Fatal(err)
		}
		if got := expMethod(red); got != tc.want {
			t.Errorf("%q: method %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestSplitComposite(t *testing.T) {
	l, c, ok := SplitComposite("12345678901|this is composite info")
	if !ok || l != "12345678901" || c != "this is composite info" {
		t.Errorf("got %q %q %v", l, c, ok)
	}
}
