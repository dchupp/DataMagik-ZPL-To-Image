package verify

import (
	"testing"

	"github.com/makiuchi-d/gozxing"

	"github.com/datamagik/zpl-to-image/internal/barcode/code93"
)

func TestCode93Decode(t *testing.T) {
	tests := []struct{ data, want string }{
		{"12345ABCDE", "12345ABCDE"},
		{"TEST93", "TEST93"},
		{"CODE 93-$/+%.", "CODE 93-$/+%."},
		{"Hello world", "Hello world"},
		{")A)B&M&J", "ab\r\n"}, // Zebra substitutes for (+)A (+)B ($)M ($)J
	}
	for _, tc := range tests {
		l, err := code93.Encode(tc.data, code93.Options{})
		if err != nil {
			t.Errorf("%q: %v", tc.data, err)
			continue
		}
		got, err := Decode(LinearImage(l, 2, 0, 60), gozxing.BarcodeFormat_CODE_93)
		if err != nil {
			t.Errorf("%q: decode: %v", tc.data, err)
		} else if got != tc.want {
			t.Errorf("%q: decoded %q, want %q", tc.data, got, tc.want)
		}
	}
}
