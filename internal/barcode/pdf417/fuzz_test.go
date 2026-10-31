package pdf417_test

import (
	"testing"

	"github.com/datamagik/zpl-to-image/internal/barcode"
	"github.com/datamagik/zpl-to-image/internal/barcode/pdf417"
)

func FuzzEncode(f *testing.F) {
	f.Add([]byte("PDF417 sample 1234567890"), 2, 4, 0, false)
	f.Add([]byte("\x00\xff binary"), 0, 0, 0, true)
	f.Fuzz(func(t *testing.T, data []byte, level, cols, rows int, trunc bool) {
		m, err := pdf417.Encode(data, pdf417.Options{SecurityLevel: abs(level) % 9, Columns: abs(cols) % 31, Rows: abs(rows) % 91, Truncated: trunc})
		if err == nil {
			wellFormed(t, m)
		}
		ms, err := pdf417.EncodeStructured(data, pdf417.Options{SecurityLevel: abs(level) % 9}, 3)
		if err == nil {
			for _, m := range ms {
				wellFormed(t, m)
			}
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
