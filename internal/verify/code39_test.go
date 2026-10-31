package verify

import (
	"testing"

	"github.com/makiuchi-d/gozxing"

	"github.com/datamagik/zpl-to-image/internal/barcode/code39"
)

// The decoder runs in Full ASCII mode, so lowercase and punctuation must
// round-trip; the check character is reported as data (no check verification).
func TestCode39Decode(t *testing.T) {
	tests := []struct {
		data  string
		check bool
		want  string
	}{
		{"123ABC", false, "123ABC"},
		{"CODE 39-.", false, "CODE 39-."},
		{"Hello, world! {x}", false, "Hello, world! {x}"},
		{"CODE39", true, "CODE39W"}, // 12+24+13+14+3+9 = 75 -> 75 mod 43 = 32 'W'
	}
	for _, tc := range tests {
		l, err := code39.Encode(tc.data, code39.Options{CheckDigit: tc.check})
		if err != nil {
			t.Errorf("%q: %v", tc.data, err)
			continue
		}
		for _, ratio := range []float64{2, 3} {
			got, err := Decode(LinearImage(l, 2, ratio, 60), gozxing.BarcodeFormat_CODE_39)
			if err != nil {
				t.Errorf("%q ratio %v: decode: %v", tc.data, ratio, err)
			} else if got != tc.want {
				t.Errorf("%q ratio %v: decoded %q, want %q", tc.data, ratio, got, tc.want)
			}
		}
	}
	l, err := code39.EncodeLOGMARS("12ab")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := Decode(LinearImage(l, 2, 3, 60), gozxing.BarcodeFormat_CODE_39); err != nil || got != "12ABO" {
		t.Errorf("LOGMARS decoded %q, %v", got, err)
	}
}
