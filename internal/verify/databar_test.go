package verify

import (
	"testing"

	"github.com/makiuchi-d/gozxing"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/databar"
)

// gozxing only reads GS1 DataBar Omnidirectional/Truncated (RSS-14); the
// other DataBar variants are verified with zxing-cpp (see the databar
// package's decode-verified goldens).
func TestDataBarOmniDecode(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"0001234567890", "00012345678905"},
		{"2001234567890", "20012345678909"},
		{"12345678901", "00123456789012"},
		{"(01)09501101530003", "09501101530003"},
		{"9999999999999", "99999999999997"},
		{"0", "00000000000000"},
	} {
		for _, trunc := range []bool{false, true} {
			f, h := databar.Omnidirectional, 33
			if trunc {
				f, h = databar.Truncated, 13
			}
			l, err := f(tc.in, databar.Options{})
			if err != nil {
				t.Fatal(err)
			}
			got, err := Decode(LinearImage(l, 3, 0, h*3), gozxing.BarcodeFormat_RSS_14)
			if err != nil || got != tc.want {
				t.Errorf("%q truncated=%v: got %q, %v; want %q", tc.in, trunc, got, err, tc.want)
			}
		}
	}
}
