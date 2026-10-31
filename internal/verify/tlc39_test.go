package verify

import (
	"testing"

	"github.com/makiuchi-d/gozxing"

	"github.com/datamagik/zpl-to-image/internal/barcode/tlc39"
)

// The TLC39 linear part is a plain Code 39 symbol of the ECI number.
func TestTLC39LinearDecode(t *testing.T) {
	s, err := tlc39.Encode("123456,ABCd12345678901234,5551212,88899", tlc39.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, ratio := range []float64{2, 3} {
		got, err := Decode(LinearImage(s.Linear, 2, ratio, 60), gozxing.BarcodeFormat_CODE_39)
		if err != nil || got != "123456" {
			t.Errorf("ratio %v: %q, %v", ratio, got, err)
		}
	}
}
