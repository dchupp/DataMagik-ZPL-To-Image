package plessey

import (
	"errors"
	"reflect"
	"testing"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
)

func TestZebraFigure(t *testing.T) {
	// ZPL II guide ^BP figure: ^BY3^BPN,N,100,Y,N^FD12345^FS.
	want := []int{2, 1, 2, 1, 1, 2, 2, 1, 2, 1, 1, 2, 1, 2, 1, 2, 1, 2, 2, 1, 1, 2, 1, 2, 2, 1, 2, 1, 1, 2, 1, 2, 1, 2, 1, 2, 2, 1, 1, 2, 2, 1, 1, 2, 2, 1, 1, 2, 1, 2, 2, 1, 2, 1, 1, 2, 1, 2, 2, 1, 2, 1, 2, 1, 2, 2, 1, 2, 1, 1, 2, 1, 2}
	l, err := Encode("12345", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(l.Elements, want) || l.Text != "12345" || !l.Ratio {
		t.Errorf("got %v %q", l.Elements, l.Text)
	}
	l, _ = Encode("12345", Options{PrintCheckDigit: true})
	// CRC bits 0110 0111 -> nibbles 6, E (LSB first).
	if l.Text != "123456E" {
		t.Errorf("text %q", l.Text)
	}
	if _, err := Encode("12G", Options{}); !errors.Is(err, barcode.ErrInvalidData) {
		t.Errorf("err = %v", err)
	}
}
