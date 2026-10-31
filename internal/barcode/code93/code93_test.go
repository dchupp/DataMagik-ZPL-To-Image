package code93

import (
	"errors"
	"testing"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
)

func TestCheckChars(t *testing.T) {
	// Wikipedia worked example: "TEST93" has check characters "+6".
	l, err := Encode("TEST93", Options{PrintCheckDigits: true})
	if err != nil {
		t.Fatal(err)
	}
	if l.Text != "TEST93+6" {
		t.Errorf("text %q", l.Text)
	}
	// start + 6 data + 2 check + stop = 10 characters of 6 elements + bar.
	if len(l.Elements) != 61 || l.Width(0) != 10*9+1 {
		t.Errorf("%d elements, %d modules", len(l.Elements), l.Width(0))
	}
	l, _ = Encode("TEST93", Options{})
	if l.Text != "TEST93" {
		t.Errorf("text %q", l.Text)
	}
}

func TestShifts(t *testing.T) {
	// Zebra substitutes: &A = ($)A = SOH, )A = (+)A = 'a'; raw lowercase is
	// encoded the same way.
	a, err := Encode(")AB", Options{})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Encode("aB", Options{})
	if a.Text != "aB" || b.Text != "aB" || len(a.Elements) != len(b.Elements) {
		t.Errorf("%q %q", a.Text, b.Text)
	}
	for i := range a.Elements {
		if a.Elements[i] != b.Elements[i] {
			t.Fatalf("element %d differs", i)
		}
	}
	if _, err := Encode("A&", Options{}); !errors.Is(err, barcode.ErrInvalidData) {
		t.Errorf("err = %v", err)
	}
}

func TestPatternsAreNineModules(t *testing.T) {
	for i, p := range patterns {
		n := 0
		for _, c := range p {
			n += int(c - '0')
		}
		if n != 9 {
			t.Errorf("value %d: %d modules", i, n)
		}
	}
}

func TestZebraFigure(t *testing.T) {
	// ZPL II guide ^BA figure: ^BY3^BAN,100,Y,N,N^FD12345ABCDE^FS (widths
	// measured from the figure bitmap).
	want := []int{1, 1, 1, 1, 4, 1, 1, 1, 1, 2, 1, 3, 1, 1, 1, 3, 1, 2, 1, 1, 1, 4, 1, 1, 1, 2, 1, 1, 1, 3, 1, 2, 1, 2, 1, 2, 2, 1, 1, 1, 1, 3, 2, 1, 1, 2, 1, 2, 2, 1, 1, 3, 1, 1, 2, 2, 1, 1, 1, 2, 2, 2, 1, 2, 1, 1, 3, 1, 1, 1, 2, 1, 1, 2, 1, 1, 2, 2, 1, 1, 1, 1, 4, 1, 1}
	l, err := Encode("12345ABCDE", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Elements) != len(want) {
		t.Fatalf("%d elements, want %d", len(l.Elements), len(want))
	}
	for i := range want {
		if l.Elements[i] != want[i] {
			t.Errorf("element %d = %d, want %d\n got %v", i, l.Elements[i], want[i], l.Elements)
			break
		}
	}
}

func TestRejectsEmptyAndInvalidPairs(t *testing.T) {
	for _, d := range []string{"", "&1", "A(9"} {
		if _, err := Encode(d, Options{}); !errors.Is(err, barcode.ErrInvalidData) {
			t.Errorf("Encode(%q) = %v, want ErrInvalidData", d, err)
		}
	}
}
