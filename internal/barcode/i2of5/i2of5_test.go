package i2of5

import (
	"errors"
	"reflect"
	"testing"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
)

func TestZebraFigure(t *testing.T) {
	// ZPL II guide ^B2 figure: ^BY3^B2N,150,Y,N,N^FD123456^FS.
	want := []int{1, 1, 1, 1, 2, 1, 1, 2, 1, 1, 1, 1, 2, 2, 2, 1, 2, 1, 1, 2, 1, 1, 1, 2, 2, 1, 1, 2, 2, 2, 1, 1, 1, 1, 2, 1, 1}
	l, err := Encode("123456", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(l.Elements, want) || l.Text != "123456" || !l.Ratio {
		t.Errorf("got %v %q", l.Elements, l.Text)
	}
}

func TestPaddingAndCheck(t *testing.T) {
	l, _ := Encode("123", Options{})
	if l.Text != "0123" {
		t.Errorf("text %q", l.Text)
	}
	// GTIN-14 style: 1234567890123 -> check 1 (3*(3+1+9+7+5+3+1)+(2+0+8+6+4+2) = 109).
	l, _ = Encode("1234567890123", Options{CheckDigit: true})
	if l.Text != "12345678901231" {
		t.Errorf("text %q", l.Text)
	}
	l, _ = Encode("12", Options{CheckDigit: true})
	if l.Text != "0123" { // 2*3+1 = 7 -> check 3
		t.Errorf("text %q", l.Text)
	}
	if _, err := Encode("12A4", Options{}); !errors.Is(err, barcode.ErrInvalidData) {
		t.Errorf("err = %v", err)
	}
}
