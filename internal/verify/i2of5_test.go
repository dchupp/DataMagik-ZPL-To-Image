package verify

import (
	"testing"

	"github.com/makiuchi-d/gozxing"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/i2of5"
)

func TestI2of5Decode(t *testing.T) {
	tests := []struct {
		data  string
		check bool
		want  string
	}{
		{"123456", false, "123456"},
		{"1234567", false, "01234567"},
		{"1234567890123", true, "12345678901231"},
		{"0123456789012345", false, "0123456789012345"},
	}
	for _, tc := range tests {
		l, err := i2of5.Encode(tc.data, i2of5.Options{CheckDigit: tc.check})
		if err != nil {
			t.Errorf("%q: %v", tc.data, err)
			continue
		}
		for _, ratio := range []float64{2, 3} {
			got, err := Decode(LinearImage(l, 2, ratio, 60), gozxing.BarcodeFormat_ITF)
			if err != nil {
				t.Errorf("%q ratio %v: decode: %v", tc.data, ratio, err)
			} else if got != tc.want {
				t.Errorf("%q ratio %v: decoded %q, want %q", tc.data, ratio, got, tc.want)
			}
		}
	}
}
