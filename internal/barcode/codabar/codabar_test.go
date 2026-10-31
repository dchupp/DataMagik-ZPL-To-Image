package codabar

import (
	"errors"
	"reflect"
	"testing"

	"github.com/datamagik/zpl-to-image/internal/barcode"
)

func TestZebraFigure(t *testing.T) {
	// ZPL II guide ^BK figure: ^BY3^BKN,N,150,Y,N,A,A^FD123456^FS.
	want := []int{1, 1, 2, 2, 1, 2, 1, 1, 1, 1, 1, 1, 2, 2, 1, 1, 1, 1, 1, 2, 1, 1, 2, 1, 2, 2, 1, 1, 1, 1, 1, 1, 1, 1, 2, 1, 1, 2, 1, 1, 2, 1, 1, 1, 1, 2, 1, 1, 1, 2, 1, 1, 1, 1, 2, 1, 1, 1, 2, 2, 1, 2, 1}
	l, err := Encode("123456", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(l.Elements, want) || l.Text != "A123456A" || !l.Ratio {
		t.Errorf("got %v %q", l.Elements, l.Text)
	}
}

func TestStartStop(t *testing.T) {
	l, err := Encode("40156", Options{Start: 'b', Stop: 'D'})
	if err != nil || l.Text != "B40156D" {
		t.Errorf("%q %v", l.Text, err)
	}
	if _, err := Encode("12", Options{Start: 'E'}); !errors.Is(err, barcode.ErrInvalidData) {
		t.Errorf("err = %v", err)
	}
	if _, err := Encode("1A2", Options{}); !errors.Is(err, barcode.ErrInvalidData) {
		t.Errorf("err = %v", err)
	}
}
