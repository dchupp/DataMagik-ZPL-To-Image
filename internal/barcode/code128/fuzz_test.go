package code128_test

import (
	"testing"

	"github.com/datamagik/zpl-to-image/internal/barcode"
	"github.com/datamagik/zpl-to-image/internal/barcode/code128"
)

func FuzzEncode(f *testing.F) {
	for _, s := range []string{"Hello 128", ">;1234>6AB", "(00)100844237449200941", "a\xc3\x84b", ">:>4ab"} {
		f.Add(s, 0)
	}
	f.Fuzz(func(t *testing.T, data string, mode int) {
		l, err := code128.Encode(data, code128.Options{Mode: code128.Mode(abs(mode) % 4)})
		if err == nil {
			wellFormedLinear(t, l)
		}
	})
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// wellFormedLinear fails unless l alternates bars and spaces of sane width.
func wellFormedLinear(t *testing.T, l barcode.Linear) {
	t.Helper()
	if len(l.Elements) == 0 || len(l.Elements)%2 == 0 {
		t.Fatalf("%d elements; a symbol starts and ends with a bar", len(l.Elements))
	}
	for i, e := range l.Elements {
		if e < 1 || e > 9 {
			t.Fatalf("element %d has width %d", i, e)
		}
	}
}
