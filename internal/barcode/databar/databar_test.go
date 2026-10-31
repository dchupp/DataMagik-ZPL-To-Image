package databar

import (
	"fmt"
	"strings"
	"testing"
)

func rowString(r []bool) string {
	var b strings.Builder
	for _, d := range r {
		if d {
			b.WriteByte('1')
		} else {
			b.WriteByte('0')
		}
	}
	return b.String()
}

func TestZintExamples(t *testing.T) {
	for i, ex := range zintExamples {
		s, err := Encode(ex.t, ex.data, Options{Segments: ex.segments})
		if err != nil {
			t.Errorf("#%d %q: %v", i, ex.data, err)
			continue
		}
		if len(s.Matrix.Rows) != len(ex.rows) {
			t.Errorf("#%d %q: %d rows, want %d", i, ex.data, len(s.Matrix.Rows), len(ex.rows))
			continue
		}
		for r, want := range ex.rows {
			if got := rowString(s.Matrix.Rows[r]); got != want {
				t.Errorf("#%d %q row %d:\n got %s\nwant %s", i, ex.data, r, got, want)
			}
		}
	}
}

func TestZintCompositeExamples(t *testing.T) {
	for i, ex := range zintCompositeExamples {
		s, err := Encode(ex.t, ex.data, Options{Segments: ex.segments, Linkage: true})
		if err != nil {
			t.Errorf("#%d %q: %v", i, ex.data, err)
			continue
		}
		got := []string{rowString(s.CompositeSeparator)}
		for _, r := range s.Matrix.Rows {
			got = append(got, rowString(r))
		}
		want := ex.rows[len(ex.rows)-len(got):]
		w := len(got[0])
		ok := false
		for off := 0; off+w <= len(want[0]) && !ok; off++ {
			ok = true
			for r := range got {
				if want[r][off:off+w] != got[r] || strings.Contains(want[r][:off]+want[r][off+w:], "1") {
					ok = false
					break
				}
			}
		}
		if !ok {
			t.Errorf("#%d %q (type %d) mismatch:\n got %s\nwant %s", i, ex.data, ex.t, strings.Join(got, "\n     "), strings.Join(want, "\n     "))
		}
	}
}

func TestDecodeVerified(t *testing.T) {
	for i, ex := range decodeVerified {
		s, err := Encode(ex.t, ex.data, Options{Segments: ex.segments})
		if err != nil {
			t.Fatalf("#%d %q: %v", i, ex.data, err)
		}
		if s.Text != ex.text {
			t.Errorf("#%d text %q, want %q", i, s.Text, ex.text)
		}
		if len(s.Matrix.Rows) != len(ex.rows) {
			t.Errorf("#%d %q: %d rows, want %d", i, ex.data, len(s.Matrix.Rows), len(ex.rows))
			continue
		}
		for r, want := range ex.rows {
			if got := rowString(s.Matrix.Rows[r]); got != want {
				t.Errorf("#%d %q row %d:\n got %s\nwant %s", i, ex.data, r, got, want)
			}
		}
	}
}

func TestGetWidths(t *testing.T) {
	// Every value of a character subset (sizes from ISO/IEC 24724 Tables 1,
	// 2, 6 and 8) maps to a distinct pattern with the right module count and
	// maximum element width.
	// check enumerates every valid composition independently, then checks
	// that getWidths maps the set's values 0..size-1 to distinct valid
	// patterns.
	check := func(n, elements, maxW int, requireNarrow bool, size int) bool {
		all := map[string]bool{}
		var rec func(prefix []int, left int)
		rec = func(prefix []int, left int) {
			if len(prefix) == elements {
				if left != 0 {
					return
				}
				narrow := false
				for _, e := range prefix {
					narrow = narrow || e == 1
				}
				if !requireNarrow || narrow {
					all[fmt.Sprint(prefix)] = true
				}
				return
			}
			for e := 1; e <= maxW && e <= left; e++ {
				rec(append(append([]int(nil), prefix...), e), left-e)
			}
		}
		rec(nil, n)
		seen := map[string]bool{}
		for v := range size {
			k := fmt.Sprint(getWidths(v, n, elements, maxW, requireNarrow))
			if !all[k] || seen[k] {
				t.Errorf("getWidths(%d, n=%d, e=%d, max=%d) = %s: invalid or repeated", v, n, elements, maxW, k)
				return false
			}
			seen[k] = true
		}
		return true
	}
	tests := []struct {
		n, elements, maxW int
		requireNarrow     bool
		want              int
	}{
		{12, 4, 8, false, 161},  // Table 1 group 1: Nodd 12, Wmax 8 -> Godd 161
		{4, 4, 1, true, 1},      // Teven 1
		{10, 4, 6, false, 80},   // group 2 odd: 800/10
		{6, 4, 3, true, 10},     // group 2 even: Teven 10
		{5, 4, 2, true, 4},      // Table 2 group 1 odd (inside): Todd 4
		{10, 4, 7, false, 84},   // inside group 1 even: 336/4
		{17, 7, 6, false, 6538}, // Table 6 group 1 odd: 183064/28
		{9, 7, 3, true, 28},     // Table 6 group 1 even: Teven 28
		{12, 4, 7, true, 87},    // Table 8 group 1 odd: 348/4
		{5, 4, 2, false, 4},     // Table 8 group 1 even: Teven 4
	}
	for _, tc := range tests {
		check(tc.n, tc.elements, tc.maxW, tc.requireNarrow, tc.want)
	}
}
