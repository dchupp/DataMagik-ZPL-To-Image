package twoof5

import (
	"errors"
	"reflect"
	"testing"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
)

func TestZebraFigures(t *testing.T) {
	// ZPL II guide figures, ^BY3 ... ^FD123456^FS.
	industrial := []int{2, 1, 2, 1, 1, 1, 2, 1, 1, 1, 1, 1, 1, 1, 2, 1, 1, 1, 2, 1, 1, 1, 1, 1, 2, 1, 2, 1, 2, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 2, 1, 1, 1, 2, 1, 2, 1, 1, 1, 2, 1, 1, 1, 1, 1, 1, 1, 2, 1, 2, 1, 1, 1, 1, 1, 2, 1, 1, 1, 2}
	standard := []int{1, 1, 1, 1, 2, 1, 1, 1, 1, 1, 1, 1, 2, 1, 1, 1, 2, 1, 1, 1, 1, 1, 2, 1, 2, 1, 2, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 2, 1, 1, 1, 2, 1, 2, 1, 1, 1, 2, 1, 1, 1, 1, 1, 1, 1, 2, 1, 2, 1, 1, 1, 1, 1, 2, 1, 1}
	l, err := EncodeIndustrial("123456")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(l.Elements, industrial) || l.Text != "123456" || !l.Ratio {
		t.Errorf("industrial %v", l.Elements)
	}
	l, err = EncodeStandard("123456")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(l.Elements, standard) {
		t.Errorf("standard %v", l.Elements)
	}
	if _, err := EncodeStandard("1-2"); !errors.Is(err, barcode.ErrInvalidData) {
		t.Errorf("err = %v", err)
	}
}
