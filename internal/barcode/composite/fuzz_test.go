package composite_test

import (
	"testing"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/composite"
)

func FuzzEncode(f *testing.F) {
	f.Add("(10)ABCD123456(410)3898765432108", 0, 1, 99)
	f.Add("(17)050101(10)ABC123", 8, 3, 70)
	f.Fuzz(func(t *testing.T, data string, linear, typ, width int) {
		c, err := composite.Encode(data, composite.Options{
			Linear: composite.Linear(abs(linear) % 9), Type: composite.Type(abs(typ) % 4), LinearWidth: abs(width) % 400})
		if err == nil {
			wellFormed(t, c.Matrix)
		}
	})
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// wellFormed fails unless m is a non-empty rectangle with valid row heights.
func wellFormed(t *testing.T, m barcode.Matrix) {
	t.Helper()
	if m.Height() == 0 || m.Width() == 0 {
		t.Fatalf("empty matrix")
	}
	for y, r := range m.Rows {
		if len(r) != m.Width() {
			t.Fatalf("row %d has %d modules, want %d", y, len(r), m.Width())
		}
	}
	if m.RowHeights != nil && len(m.RowHeights) != m.Height() {
		t.Fatalf("%d row heights for %d rows", len(m.RowHeights), m.Height())
	}
}
