package aztec_test

import (
	"testing"

	"github.com/datamagik/zpl-to-image/internal/barcode"
	"github.com/datamagik/zpl-to-image/internal/barcode/aztec"
)

func FuzzEncodeZPL(f *testing.F) {
	f.Add([]byte("Aztec 0123"), 0, false, 1)
	f.Add([]byte("42"), 300, false, 1)
	f.Add([]byte("abc"), 104, true, 3)
	f.Fuzz(func(t *testing.T, data []byte, size int, menu bool, symbols int) {
		m, err := aztec.EncodeZPL(data, aztec.ZPLParams{Size: size % 400, Menu: menu, Symbols: 1 + abs(symbols)%26})
		if err == nil {
			wellFormed(t, m)
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
