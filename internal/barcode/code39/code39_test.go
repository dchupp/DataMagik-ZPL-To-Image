package code39

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
)

// ratioRuns converts pixel runs measured from a Zebra figure into 1/2
// (narrow/wide) elements.
func ratioRuns(s string, threshold int) []int {
	var out []int
	for _, f := range strings.Fields(s) {
		n, _ := strconv.Atoi(f[1:])
		if n > threshold {
			out = append(out, 2)
		} else {
			out = append(out, 1)
		}
	}
	return out
}

func TestZebraFigures(t *testing.T) {
	// ZPL II guide ^B3 figure: ^BY3^B3N,N,100,Y,N^FD123ABC^FS.
	want := ratioRuns("B3 s9 B3 s3 B9 s3 B9 s3 B4 s2 B10 s2 B4 s8 B4 s2 B4 s2 B10 s2 B4 s2 B10 s8 B4 s2 B4 s2 "+
		"B10 s3 B9 s3 B9 s9 B3 s3 B3 s3 B3 s3 B9 s3 B3 s3 B3 s9 B3 s3 B9 s3 B3 s3 B9 s3 B3 s9 B3 s3 B9 s3 B9 "+
		"s3 B9 s3 B3 s9 B3 s3 B3 s3 B3 s9 B3 s3 B9 s3 B9 s3 B3", 6)
	l, err := Encode("123ABC", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(l.Elements, want) || l.Text != "*123ABC*" || !l.Ratio {
		t.Errorf("123ABC:\n got %v %q\nwant %v", l.Elements, l.Text, want)
	}

	// ^BL figure: ^BY3^BLN,100,N^FD12AB^FS (mandatory Mod 43 check).
	want = ratioRuns("B4 s8 B4 s2 B10 s2 B10 s2 B4 s2 B10 s2 B4 s8 B4 s2 B4 s2 B10 s2 B4 s2 B10 s8 B4 s2 "+
		"B4 s2 B10 s2 B10 s2 B4 s2 B4 s8 B4 s2 B10 s2 B4 s2 B10 s2 B4 s8 B4 s2 B10 s2 B10 s2 B4 s2 B10 s2 B4 "+
		"s8 B4 s2 B4 s8 B4 s2 B10 s2 B10 s2 B4", 6)
	l, err = EncodeLOGMARS("12ab")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(l.Elements, want) {
		t.Errorf("LOGMARS 12AB:\n got %v\nwant %v", l.Elements, want)
	}
	// 1+2+10+11 = 24 -> 'O'.
	if l.Text != "12ABO" {
		t.Errorf("LOGMARS text %q", l.Text)
	}
}

func TestCheckDigit(t *testing.T) {
	// "CODE 39": 12+24+13+14+38+3+9 = 113, 113 mod 43 = 27 -> 'R'.
	l, err := Encode("CODE 39", Options{CheckDigit: true})
	if err != nil {
		t.Fatal(err)
	}
	if l.Text != "*CODE 39R*" {
		t.Errorf("text %q", l.Text)
	}
	if n := (len(l.Elements) + 1) / 10; n != 10 {
		t.Errorf("%d characters, want 10", n)
	}
}

func TestFullASCII(t *testing.T) {
	v, err := values("a\r~!")
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, x := range v {
		b.WriteByte(alphabet[x])
	}
	if b.String() != "+A$M%S/A" {
		t.Errorf("full ASCII %q", b.String())
	}
	if _, err := Encode("é", Options{}); !errors.Is(err, barcode.ErrInvalidData) {
		t.Errorf("err = %v", err)
	}
	if _, err := EncodeLOGMARS("a!"); !errors.Is(err, barcode.ErrInvalidData) {
		t.Errorf("err = %v", err)
	}
}

func TestZebraFullASCIIFigure(t *testing.T) {
	// ZPL II guide ^B3 "Full ASCII" example: ^B3N,N,100,Y^FDTEST+$$M$J-$^FS.
	// Native characters ($ + etc.) are encoded as themselves; the scanner
	// does the Full ASCII interpretation.
	fig := "1211212111111121221121112211111121112211111121221112111212111212121111121212111121211112111212121111111122211112111121211212121111121121211"
	l, err := Encode("TEST+$$M$J-$", Options{})
	if err != nil {
		t.Fatal(err)
	}
	var got strings.Builder
	for _, e := range l.Elements {
		got.WriteByte(byte('0' + e))
	}
	if got.String() != fig {
		t.Errorf("\n got %s\nwant %s", got.String(), fig)
	}
}
