package aztec

// gf is a Galois field GF(2^m) used by the Aztec Reed-Solomon codes.
type gf struct {
	size int
	exp  []int
	log  []int
}

// Field polynomials from ISO/IEC 24778 section 7.3.1.2.
const (
	poly4  = 0x13   // GF(16):   x^4 + x + 1 (mode message)
	poly6  = 0x43   // GF(64):   x^6 + x + 1
	poly8  = 0x12D  // GF(256):  x^8 + x^5 + x^3 + x^2 + 1
	poly10 = 0x409  // GF(1024): x^10 + x^3 + 1
	poly12 = 0x1069 // GF(4096): x^12 + x^6 + x^5 + x^3 + 1
)

// fields holds the precomputed fields, built at init so concurrent use is
// safe.
var fields = func() map[int]*gf {
	m := map[int]*gf{}
	for _, b := range []int{4, 6, 8, 10, 12} {
		m[b] = newField(b)
	}
	return m
}()

func field(bits int) *gf { return fields[bits] }

func newField(bits int) *gf {
	var p int
	switch bits {
	case 4:
		p = poly4
	case 6:
		p = poly6
	case 8:
		p = poly8
	case 10:
		p = poly10
	case 12:
		p = poly12
	default:
		panic("aztec: bad field size")
	}
	size := 1 << bits
	f := &gf{size: size, exp: make([]int, 2*size), log: make([]int, size)}
	x := 1
	for i := 0; i < size-1; i++ {
		f.exp[i] = x
		f.log[x] = i
		x <<= 1
		if x&size != 0 {
			x ^= p
		}
	}
	for i := size - 1; i < 2*size; i++ {
		f.exp[i] = f.exp[i-(size-1)]
	}
	return f
}

func (f *gf) mul(a, b int) int {
	if a == 0 || b == 0 {
		return 0
	}
	return f.exp[f.log[a]+f.log[b]]
}

// rsEncode returns nCheck Reed-Solomon check words for data using the
// generator polynomial prod_{i=1..nCheck}(x - a^i) (Aztec uses first root
// a^1).
func rsEncode(bits int, data []int, nCheck int) []int {
	f := field(bits)
	// generator coefficients, highest degree first, g[0] = 1
	g := []int{1}
	for i := 1; i <= nCheck; i++ {
		ng := make([]int, len(g)+1)
		r := f.exp[i%(f.size-1)]
		for j, c := range g {
			ng[j] ^= c
			ng[j+1] ^= f.mul(c, r)
		}
		g = ng
	}
	rem := make([]int, nCheck)
	for _, d := range data {
		fb := d ^ rem[0]
		copy(rem, rem[1:])
		rem[nCheck-1] = 0
		if fb != 0 {
			for j := 0; j < nCheck; j++ {
				rem[j] ^= f.mul(g[j+1], fb)
			}
		}
	}
	return rem
}
