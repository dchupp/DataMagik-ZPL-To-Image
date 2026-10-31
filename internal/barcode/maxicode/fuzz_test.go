package maxicode_test

import (
	"testing"

	"github.com/datamagik/zpl-to-image/internal/barcode"
	"github.com/datamagik/zpl-to-image/internal/barcode/maxicode"
)

func FuzzEncode(f *testing.F) {
	f.Add([]byte("002840152382802[)>\x1e01\x1d96"), 2)
	f.Add([]byte("MaxiCode mode 4"), 4)
	f.Add([]byte("ABC 123"), 3)
	f.Fuzz(func(t *testing.T, data []byte, mode int) {
		m, err := maxicode.Encode(data, maxicode.Options{Mode: 2 + abs(mode)%5, SymbolNumber: 1, SymbolCount: 1})
		if err == nil {
			wellFormed(t, m)
			if m.Height() != maxicode.Rows || m.Width() != maxicode.Cols {
				t.Fatalf("size %dx%d", m.Width(), m.Height())
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
