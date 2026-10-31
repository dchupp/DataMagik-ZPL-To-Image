package datamatrix

// Reed-Solomon over GF(256) with the ECC 200 field polynomial
// x^8+x^5+x^3+x^2+1 (301) and generator roots α^1..α^n (ISO/IEC 16022 5.7).

var gfExp [512]int
var gfLog [256]int

func init() {
	x := 1
	for i := 0; i < 255; i++ {
		gfExp[i] = x
		gfLog[x] = i
		x <<= 1
		if x&0x100 != 0 {
			x ^= 301
		}
	}
	for i := 255; i < 512; i++ {
		gfExp[i] = gfExp[i-255]
	}
}

func gfMul(a, b int) int {
	if a == 0 || b == 0 {
		return 0
	}
	return gfExp[gfLog[a]+gfLog[b]]
}

// rsGenerator returns the coefficients of g(x) = Π(x - α^i), i=1..n, highest
// degree first excluding the leading 1 (length n).
func rsGenerator(n int) []int {
	g := []int{1}
	for i := 1; i <= n; i++ {
		ng := make([]int, len(g)+1)
		for j, c := range g {
			ng[j] ^= c
			ng[j+1] ^= gfMul(c, gfExp[i])
		}
		g = ng
	}
	return g[1:]
}

// rsECC computes n error correction codewords for data.
func rsECC(data []byte, n int) []byte {
	g := rsGenerator(n)
	ecc := make([]int, n)
	for _, d := range data {
		f := int(d) ^ ecc[0]
		copy(ecc, ecc[1:])
		ecc[n-1] = 0
		if f != 0 {
			for j := 0; j < n; j++ {
				ecc[j] ^= gfMul(f, g[j])
			}
		}
	}
	out := make([]byte, n)
	for i, v := range ecc {
		out[i] = byte(v)
	}
	return out
}
