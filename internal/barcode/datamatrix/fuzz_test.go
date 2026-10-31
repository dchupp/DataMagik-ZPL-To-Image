package datamatrix_test

import (
	"testing"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/datamatrix"
)

func FuzzEncode(f *testing.F) {
	f.Add([]byte("Hello 123"), 0, 0, false)
	f.Add([]byte("_1011234567890123110ABC"), 0, 0, false)
	f.Add([]byte("\xff\x00ABC"), 36, 16, true)
	f.Fuzz(func(t *testing.T, data []byte, cols, rows int, rect bool) {
		m, err := datamatrix.Encode(data, datamatrix.Options{Quality: 200, Columns: cols % 150, Rows: rows % 150, Escape: '_', Rectangular: rect})
		if err == nil {
			wellFormed(t, m)
		}
	})
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
