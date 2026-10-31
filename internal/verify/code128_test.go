package verify

import (
	"testing"

	"github.com/makiuchi-d/gozxing"

	"github.com/datamagik/zpl-to-image/internal/barcode/code128"
)

// gozxing drops FNC1 separators unless ASSUME_GS1 is set, so GS1 cases
// compare without them (FNC1 placement is checked with zxing-cpp and the
// unit tests).
func TestCode128Decode(t *testing.T) {
	tests := []struct {
		data string
		opts code128.Options
		want string
	}{
		{"CODE128", code128.Options{}, "CODE128"},
		{">:Hello, World!", code128.Options{}, "Hello, World!"},
		{">;1234567890", code128.Options{}, "1234567890"},
		{">;382436>6CODE>7\x01X", code128.Options{}, "382436CODE\x01X"},
		{">9ABC\x09DEF", code128.Options{}, "ABC\tDEF"},
		{">:a>4\x0db", code128.Options{}, "a\rb"},
		{"a>0b><c>=", code128.Options{}, "a>b^c~"},
		{"ABC123456789xyz", code128.Options{Mode: code128.ModeAuto}, "ABC123456789xyz"},
		{"12345X\x01a\x02bcd99887766", code128.Options{Mode: code128.ModeAuto}, "12345X\x01a\x02bcd99887766"},
		{"0012345123451234512", code128.Options{Mode: code128.ModeUCC}, "00123451234512345120"},
		{">;>80012345123451234512", code128.Options{UCCCheckDigit: true}, "00123451234512345120"},
		{"(00)10084423 7449200940", code128.Options{Mode: code128.ModeGS1}, "00100844237449200941"},
		{"(91)0005886>8(10)0000410549>8(99)05", code128.Options{Mode: code128.ModeGS1}, "9100058861000004105499905"},
		{"(01)00614141999990(10)ABC123(21)XYZ", code128.Options{Mode: code128.ModeGS1}, "010061414199999610ABC12321XYZ"},
	}
	for _, tc := range tests {
		l, err := code128.Encode(tc.data, tc.opts)
		if err != nil {
			t.Errorf("%q: %v", tc.data, err)
			continue
		}
		got, err := Decode(LinearImage(l, 2, 0, 60), gozxing.BarcodeFormat_CODE_128)
		if err != nil {
			t.Errorf("%q: decode: %v", tc.data, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%q: decoded %q, want %q", tc.data, got, tc.want)
		}
	}
}
