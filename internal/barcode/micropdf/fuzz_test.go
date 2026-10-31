package micropdf_test

import (
	"testing"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/micropdf"
)

func FuzzEncode(f *testing.F) {
	f.Add([]byte("MicroPDF417"), 10)
	f.Add([]byte("123456789012"), 33)
	f.Fuzz(func(t *testing.T, data []byte, mode int) {
		m, err := micropdf.Encode(data, micropdf.Options{Mode: abs(mode) % 34})
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
