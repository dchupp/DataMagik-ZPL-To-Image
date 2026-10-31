package codablock_test

import (
	"testing"

	"github.com/datamagik/zpl-to-image/internal/barcode"
	"github.com/datamagik/zpl-to-image/internal/barcode/codablock"
)

func FuzzEncode(f *testing.F) {
	f.Add([]byte("CODABLOCK F TEST"), byte('F'), 10, 0)
	f.Add([]byte("CODABLOCK A 123"), byte('A'), 0, 3)
	f.Add([]byte("\x80GS1"), byte('E'), 0, 0)
	f.Fuzz(func(t *testing.T, data []byte, mode byte, cols, rows int) {
		modes := []codablock.Mode{codablock.ModeA, codablock.ModeE, codablock.ModeF}
		m, err := codablock.Encode(data, codablock.Options{Mode: modes[int(mode)%3], Columns: abs(cols) % 70, Rows: abs(rows) % 30, RowHeight: 8, SepHeight: 1, Narrow: 1, Ratio: 3})
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
