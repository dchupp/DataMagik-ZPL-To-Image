package verify

import (
	"testing"

	"github.com/makiuchi-d/gozxing"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/codabar"
)

// gozxing strips the start/stop characters (no RETURN_CODABAR_START_END
// hint); they are checked by the unit tests against the Zebra figure.
func TestCodabarDecode(t *testing.T) {
	tests := []struct {
		data        string
		start, stop byte
		want        string
	}{
		{"123456", 0, 0, "123456"},
		{"40156-$:/.+", 'B', 'D', "40156-$:/.+"},
		{"31117013206375", 'C', 'A', "31117013206375"},
	}
	for _, tc := range tests {
		l, err := codabar.Encode(tc.data, codabar.Options{Start: tc.start, Stop: tc.stop})
		if err != nil {
			t.Errorf("%q: %v", tc.data, err)
			continue
		}
		for _, ratio := range []float64{2, 3} {
			got, err := Decode(LinearImage(l, 2, ratio, 60), gozxing.BarcodeFormat_CODABAR)
			if err != nil {
				t.Errorf("%q ratio %v: decode: %v", tc.data, ratio, err)
			} else if got != tc.want {
				t.Errorf("%q ratio %v: decoded %q, want %q", tc.data, ratio, got, tc.want)
			}
		}
	}
}
