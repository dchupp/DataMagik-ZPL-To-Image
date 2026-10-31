package qr_test

import (
	"errors"
	"testing"

	"github.com/datamagik/zpl-to-image/internal/barcode"
	"github.com/datamagik/zpl-to-image/internal/barcode/qr"
)

func FuzzEncodeZPL(f *testing.F) {
	for _, s := range []string{"QA,hello", "MM,N0123,AABC,B0004\x00\x01\x02\x03", "D03048F,LA,x", "HM,K\x88\x9f", ""} {
		f.Add([]byte(s), 2)
	}
	f.Fuzz(func(t *testing.T, data []byte, model int) {
		m, err := qr.EncodeZPL(data, qr.Options{Model: 1 + model&1})
		if err == nil {
			wellFormed(t, m)
		} else if !errors.Is(err, barcode.ErrInvalidData) {
			t.Fatalf("unexpected error type: %v", err)
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
