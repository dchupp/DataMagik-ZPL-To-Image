package qr

import (
	"math/rand"
	"testing"
)

// penaltyRef is the original, straightforward ISO 18004 penalty
// implementation, kept as the reference for the optimised penalty.
func penaltyRef(g [][]bool) int {
	n := len(g)
	score := 0
	at := func(x, y int, vertical bool) bool {
		if vertical {
			return g[x][y]
		}
		return g[y][x]
	}
	for _, vertical := range []bool{false, true} {
		for y := 0; y < n; y++ {
			// N1: runs of five or more same-colour modules.
			run := 1
			for x := 1; x <= n; x++ {
				if x < n && at(x, y, vertical) == at(x-1, y, vertical) {
					run++
					continue
				}
				if run >= 5 {
					score += 3 + run - 5
				}
				run = 1
			}
			// N3: 1:1:3:1:1 finder-like pattern with four light modules on
			// either side (outside the symbol counts as light).
			light := func(x int) bool { return x < 0 || x >= n || !at(x, y, vertical) }
			for x := 0; x+7 <= n; x++ {
				if !(at(x, y, vertical) && !at(x+1, y, vertical) && at(x+2, y, vertical) &&
					at(x+3, y, vertical) && at(x+4, y, vertical) && !at(x+5, y, vertical) && at(x+6, y, vertical)) {
					continue
				}
				before := light(x-1) && light(x-2) && light(x-3) && light(x-4)
				after := light(x+7) && light(x+8) && light(x+9) && light(x+10)
				if before || after {
					score += 40
				}
			}
		}
	}
	// N2: 2x2 blocks of one colour.
	dark := 0
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if g[y][x] {
				dark++
			}
			if x+1 < n && y+1 < n {
				c := g[y][x]
				if g[y][x+1] == c && g[y+1][x] == c && g[y+1][x+1] == c {
					score += 3
				}
			}
		}
	}
	// N4: dark module proportion.
	total := n * n
	k := (abs(dark*20-total*10)+total-1)/total - 1
	score += k * 10
	return score
}

func TestPenaltyMatchesReference(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 500; i++ {
		n := 21 + 4*r.Intn(40)
		g := make([][]bool, n)
		density := r.Float64()
		for y := range g {
			g[y] = make([]bool, n)
			for x := range g[y] {
				g[y][x] = r.Float64() < density
			}
		}
		if got, want := penalty(g), penaltyRef(g); got != want {
			t.Fatalf("n=%d: penalty %d, reference %d", n, got, want)
		}
	}
}

func BenchmarkPenalty(b *testing.B) {
	r := rand.New(rand.NewSource(1))
	g := make([][]bool, 57)
	for y := range g {
		g[y] = make([]bool, 57)
		for x := range g[y] {
			g[y][x] = r.Intn(2) == 1
		}
	}
	b.Run("optimised", func(b *testing.B) {
		for range b.N {
			penalty(g)
		}
	})
	b.Run("reference", func(b *testing.B) {
		for range b.N {
			penaltyRef(g)
		}
	})
}
