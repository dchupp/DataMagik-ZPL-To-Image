package raster

import (
	"testing"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
)

// TestWideWidth_ZD420 pins the wide element widths a ZD420 (300 dpi) draws
// for Code 39 at ^BY module widths 1–4 and ratios 2.0–3.0, read back with
// ^HG: floor(module × ratio), with the ratio taken to tenths first (a
// trailing 5 rounds down: 2.25 is 2.2, 2.29 is 2.3).
func TestWideWidth_ZD420(t *testing.T) {
	measured := map[int][11]int{ // ratio 2.0, 2.1, ..., 3.0
		1: {2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 3},
		2: {4, 4, 4, 4, 4, 5, 5, 5, 5, 5, 6},
		3: {6, 6, 6, 6, 7, 7, 7, 8, 8, 8, 9},
		4: {8, 8, 8, 9, 9, 10, 10, 10, 11, 11, 12},
	}
	for m, row := range measured {
		for i, want := range row {
			r := 2 + float64(i)/10
			if got := WideWidth(m, r); got != want {
				t.Errorf("WideWidth(%d, %.1f) = %d, want %d", m, r, got, want)
			}
		}
	}
	for _, tc := range []struct {
		m    int
		r    float64
		want int
	}{
		{7, 2.29, 16}, {9, 2.15, 18}, {10, 2.25, 22}, {10, 2.29, 23},
	} {
		if got := WideWidth(tc.m, tc.r); got != tc.want {
			t.Errorf("WideWidth(%d, %v) = %d, want %d", tc.m, tc.r, got, tc.want)
		}
	}
}

// TestElementWidth_ZD420 pins the two odd elements measured on a ZD420: the
// Plessey termination bar (wide + narrow) and the Code 11 '-' centre bar
// (floor(5/3 × ratio × module)).
func TestElementWidth_ZD420(t *testing.T) {
	dash := []struct {
		m    int
		r    float64
		want int
	}{
		{1, 2.0, 3}, {1, 2.5, 4}, {2, 2.0, 6}, {2, 2.5, 8}, {2, 3.0, 10},
		{3, 2.0, 10}, {3, 2.2, 11}, {3, 2.6, 13}, {4, 2.1, 14}, {4, 2.3, 15},
		{4, 2.7, 18}, {5, 2.4, 20}, {6, 2.7, 27}, {7, 2.3, 26},
	}
	for _, tc := range dash {
		if got := ElementWidth(barcode.ElemDash, tc.m, tc.r); got != tc.want {
			t.Errorf("dash at %d, %.1f = %d, want %d", tc.m, tc.r, got, tc.want)
		}
	}
	plessey := []struct {
		m    int
		r    float64
		want int
	}{
		{1, 2.0, 3}, {1, 2.5, 3}, {2, 3.0, 8}, {3, 2.0, 9}, {3, 2.2, 9},
		{3, 2.6, 10}, {4, 2.7, 14}, {5, 2.4, 17}, {2, 2.5, 7},
	}
	for _, tc := range plessey {
		if got := ElementWidth(barcode.ElemWideNarrow, tc.m, tc.r); got != tc.want {
			t.Errorf("wide+narrow at %d, %.1f = %d, want %d", tc.m, tc.r, got, tc.want)
		}
	}
	if ElementWidth(barcode.ElemNarrow, 3, 2.6) != 3 || ElementWidth(barcode.ElemWide, 3, 2.6) != 7 {
		t.Error("narrow/wide")
	}
}

// TestPostalPitch_ZD420: POSTNET, PLANET and IMb bars are floor(2.5 ×
// module) apart on a ZD420 whatever the ^BY ratio.
func TestPostalPitch_ZD420(t *testing.T) {
	for m, want := range map[int]int{1: 2, 2: 5, 3: 7, 4: 10, 5: 12, 6: 15, 7: 17} {
		if got := PostalPitch(m); got != want {
			t.Errorf("PostalPitch(%d) = %d, want %d", m, got, want)
		}
	}
}
