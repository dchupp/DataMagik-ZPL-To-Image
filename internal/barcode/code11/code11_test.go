package code11

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
)

func ratioRuns(s string) []int {
	var out []int
	for _, f := range strings.Fields(s) {
		n, _ := strconv.Atoi(f[1:])
		out = append(out, 1+n/6)
	}
	return out
}

func TestZebraFigure(t *testing.T) {
	// ZPL II guide ^B1 figure: ^BY3^B1N,N,150,Y,N^FD123456^FS (two check
	// digits).
	want := ratioRuns("B3 s3 B9 s9 B3 s3 B9 s3 B3 s3 B9 s3 B3 s9 B3 s3 B9 s3 B9 s9 B3 s3 B3 s3 B3 s3 B9 s3 B9 s3 B9 s3 B9 s3 B3 s3 B3 s9 B9 s3 " +
		"B3 s3 B9 s3 B3 s3 B9 s3 B9 s3 B3 s3 B9 s3 B3 s3 B9 s9 B3")
	l, err := Encode("123456", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(l.Elements, want) {
		t.Errorf("\n got %v\nwant %v", l.Elements, want)
	}
}

func TestCheckDigits(t *testing.T) {
	// "123-45": C = (5*1+4*2+10*3+3*4+2*5+1*6) mod 11 = 71 mod 11 = 5;
	// K over "123-455" = (5*1+5*2+4*3+10*4+3*5+2*6+1*7) mod 11 = 101 mod 11 = 2.
	l, err := Encode("123-45", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if l.Text != "123-4552" {
		t.Errorf("text %q", l.Text)
	}
	l, _ = Encode("123-45", Options{OneCheckDigit: true})
	if l.Text != "123-455" {
		t.Errorf("text %q", l.Text)
	}
	if _, err := Encode("12A", Options{}); !errors.Is(err, barcode.ErrInvalidData) {
		t.Errorf("err = %v", err)
	}
}
