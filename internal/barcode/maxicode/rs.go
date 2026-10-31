package maxicode

// Reed-Solomon error correction over GF(64) with field polynomial
// x^6 + x + 1 (0x43) and generator roots alpha^1..alpha^n, as specified by
// ISO/IEC 16023 section 4.10.

var gfExp [126]byte
var gfLog [64]int

func init() {
	x := 1
	for i := 0; i < 63; i++ {
		gfExp[i] = byte(x)
		gfExp[i+63] = byte(x)
		gfLog[x] = i
		x <<= 1
		if x&0x40 != 0 {
			x ^= 0x43
		}
	}
}

func gfMul(a, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}
	return gfExp[gfLog[a]+gfLog[b]]
}

// generator returns the coefficients of prod_{i=1..n}(x - alpha^i), highest
// degree first, without the leading 1.
func generator(n int) []byte {
	g := []byte{1}
	for i := 1; i <= n; i++ {
		ng := make([]byte, len(g)+1)
		for j, c := range g {
			ng[j] ^= c
			ng[j+1] ^= gfMul(c, gfExp[i])
		}
		g = ng
	}
	return g[1:]
}

// rsEncode returns the n check codewords for data (highest degree first, so
// they follow the data directly in the codeword sequence).
func rsEncode(data []byte, n int) []byte {
	g := generator(n)
	rem := make([]byte, n)
	for _, d := range data {
		f := d ^ rem[0]
		copy(rem, rem[1:])
		rem[n-1] = 0
		for j := 0; j < n; j++ {
			rem[j] ^= gfMul(f, g[j])
		}
	}
	return rem
}
