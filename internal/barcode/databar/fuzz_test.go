package databar_test

import (
	"testing"

	"github.com/datamagik/zpl-to-image/internal/barcode"
	"github.com/datamagik/zpl-to-image/internal/barcode/databar"
)

func FuzzEncode(f *testing.F) {
	f.Add("0001234567890", 1, 0)
	f.Add("(01)98898765432106(3202)012345(15)991231", 6, 4)
	f.Add("(01)00012345678905(10)ABC", 6, 2)
	f.Fuzz(func(t *testing.T, data string, typ, segments int) {
		s, err := databar.Encode(databar.Type(1+abs(typ)%6), data, databar.Options{Segments: 2 * (1 + abs(segments)%11)})
		if err == nil {
			wellFormed(t, s.Matrix)
			if s.Linear != nil {
				wellFormedLinear(t, *s.Linear)
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
