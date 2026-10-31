package verify

import (
	"testing"

	"github.com/makiuchi-d/gozxing"

	"github.com/datamagik/zpl-to-image/internal/barcode"
	"github.com/datamagik/zpl-to-image/internal/barcode/codablock"
)

// Every Codablock row is a complete Code 39 (A) or Code 128 (E/F) symbol:
// decode the data rows one at a time.
func TestCodablockRowsDecode(t *testing.T) {
	m, err := codablock.Encode([]byte("THE QUICK BROWN FOX JUMPS OVER THE LAZY DOG"), codablock.Options{Mode: codablock.ModeA, Columns: 10})
	if err != nil {
		t.Fatal(err)
	}
	wantA := []string{"PTHE QUICK P", "0BROWN FOX 0", "1JUMPS OVER1", "2 THE LAZY 2", "3DOG     FN3"}
	for i, y := 0, 1; y < len(m.Rows); i, y = i+1, y+2 {
		row := barcode.Matrix{Rows: [][]bool{m.Rows[y]}}
		got, err := Decode(MatrixImage(row, 2, 60, 12), gozxing.BarcodeFormat_CODE_39)
		if err != nil || got != wantA[i] {
			t.Errorf("A row %d: %q, %v; want %q", i, got, err, wantA[i])
		}
	}

	m, err = codablock.Encode([]byte("CODABLOCK F 34567890123456789010040digit"), codablock.Options{Columns: 8})
	if err != nil {
		t.Fatal(err)
	}
	rows := 0
	for y := 1; y < len(m.Rows); y += 2 {
		row := barcode.Matrix{Rows: [][]bool{m.Rows[y]}}
		if _, err := Decode(MatrixImage(row, 2, 60, 12), gozxing.BarcodeFormat_CODE_128); err != nil {
			t.Errorf("F row %d: %v", y/2, err)
		}
		rows++
	}
	if rows != 4 {
		t.Errorf("%d rows", rows)
	}
}
