package pdf417

import "sync"

// The PDF417 family (PDF417, MicroPDF417, the GS1 composite components) uses
// Reed-Solomon error correction over the prime field GF(929) with generator
// polynomial g(x) = (x-3)(x-3^2)...(x-3^k) (ISO/IEC 15438 5.10, Annex F).

const gfPrime = 929

var (
	genMu    sync.Mutex
	genCache = map[int][]int{}
)

// generator returns the k+1 coefficients of g(x), lowest order first, with
// the leading (x^k) coefficient 1.
func generator(k int) []int {
	genMu.Lock()
	defer genMu.Unlock()
	if g, ok := genCache[k]; ok {
		return g
	}
	g := []int{1} // g(x) = 1
	root := 1
	for i := 1; i <= k; i++ {
		root = root * 3 % gfPrime
		// multiply g by (x - root)
		next := make([]int, len(g)+1)
		for j, c := range g {
			next[j+1] = (next[j+1] + c) % gfPrime
			next[j] = (next[j] + gfPrime - c*root%gfPrime) % gfPrime
		}
		g = next
	}
	genCache[k] = g
	return g
}

// GeneratorCoefficients returns the coefficients a0..a(k-1) of the
// Reed-Solomon generator polynomial for k error correction codewords, in the
// order tabulated by ISO/IEC 15438 Annex F (the monic x^k term is omitted).
func GeneratorCoefficients(k int) []int {
	g := generator(k)
	return append([]int(nil), g[:k]...)
}

// ECCodewords returns the k Reed-Solomon error correction codewords for data
// (all codewords preceding the error correction, in symbol order), in the
// order they are appended to the symbol.
func ECCodewords(data []int, k int) []int {
	if k <= 0 {
		return nil
	}
	g := generator(k)
	ec := make([]int, k)
	for _, d := range data {
		t := (d + ec[k-1]) % gfPrime
		for j := k - 1; j >= 1; j-- {
			ec[j] = (ec[j-1] + gfPrime - t*g[j]%gfPrime) % gfPrime
		}
		ec[0] = (gfPrime - t*g[0]%gfPrime) % gfPrime
	}
	out := make([]int, k)
	for j := 0; j < k; j++ {
		out[k-1-j] = (gfPrime - ec[j]) % gfPrime
	}
	return out
}

// syndromesZero reports whether cw (data followed by k EC codewords) is a
// valid codeword of the RS code, i.e. c(3^i) == 0 for i = 1..k. Used by tests.
func syndromesZero(cw []int, k int) bool {
	root := 1
	for i := 1; i <= k; i++ {
		root = root * 3 % gfPrime
		s := 0
		for _, c := range cw {
			s = (s*root + c) % gfPrime
		}
		if s != 0 {
			return false
		}
	}
	return true
}
