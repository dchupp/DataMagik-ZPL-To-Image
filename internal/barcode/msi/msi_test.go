package msi

import (
	"errors"
	"reflect"
	"testing"

	"github.com/datamagik/zpl-to-image/internal/barcode"
)

func TestZebraFigure(t *testing.T) {
	// ZPL II guide ^BM figure: ^BY3^BMN,B,100,Y,N,N^FD123456^FS (one
	// Mod 10 check digit, 1234566).
	want := []int{2, 1, 1, 2, 1, 2, 1, 2, 2, 1, 1, 2, 1, 2, 2, 1, 1, 2, 1, 2, 1, 2, 2, 1, 2, 1, 1, 2, 2, 1, 1, 2, 1, 2, 1, 2, 2, 1, 1, 2, 2, 1, 1, 2, 2, 1, 2, 1, 1, 2, 1, 2, 2, 1, 2, 1, 1, 2, 1, 2, 1}
	l, err := Encode("123456", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(l.Elements, want) || l.Text != "123456" || !l.Ratio {
		t.Errorf("got %v %q", l.Elements, l.Text)
	}
}

func TestCheckDigits(t *testing.T) {
	tests := []struct {
		check Check
		want  string
	}{
		{CheckNone, "1234567"},
		{CheckMod10, "12345674"},
		{CheckMod10Mod10, "123456741"},
		// Mod 11 (weights 2..7): 7*2+6*3+5*4+4*5+3*6+2*7+1*2 = 106, 11-106%11 = 4.
		{CheckMod11Mod10, "123456741"},
	}
	for _, tc := range tests {
		l, err := Encode("1234567", Options{Check: tc.check, PrintCheckDigits: true})
		if err != nil || l.Text != tc.want {
			t.Errorf("%c: %q %v, want %q", tc.check, l.Text, err, tc.want)
		}
	}
	if _, err := Encode("12a", Options{}); !errors.Is(err, barcode.ErrInvalidData) {
		t.Errorf("err = %v", err)
	}
}
