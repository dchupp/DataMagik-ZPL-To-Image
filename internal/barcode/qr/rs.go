package qr

// Reed-Solomon over GF(256) with the QR field polynomial
// x^8 + x^4 + x^3 + x^2 + 1 (0x11D) and generator roots alpha^0..alpha^(n-1).

var gfExp [512]byte
var gfLog [256]int

func init() {
	x := 1
	for i := 0; i < 255; i++ {
		gfExp[i] = byte(x)
		gfLog[x] = i
		x <<= 1
		if x&0x100 != 0 {
			x ^= 0x11D
		}
	}
	for i := 255; i < 512; i++ {
		gfExp[i] = gfExp[i-255]
	}
}

func gfMul(a, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}
	return gfExp[gfLog[a]+gfLog[b]]
}

// rsGenerator returns the generator polynomial coefficients (highest degree
// first, leading 1 omitted) of degree n.
func rsGenerator(n int) []byte {
	g := []byte{1}
	for i := 0; i < n; i++ {
		// multiply g by (x - alpha^i)
		ng := make([]byte, len(g)+1)
		for j, c := range g {
			ng[j] ^= c
			ng[j+1] ^= gfMul(c, gfExp[i])
		}
		g = ng
	}
	return g[1:]
}

// rsEncode returns n error correction codewords for data.
func rsEncode(data []byte, n int) []byte {
	gen := rsGenerator(n)
	rem := make([]byte, n)
	for _, d := range data {
		f := d ^ rem[0]
		copy(rem, rem[1:])
		rem[n-1] = 0
		for j := 0; j < n; j++ {
			rem[j] ^= gfMul(gen[j], f)
		}
	}
	return rem
}
