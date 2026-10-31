package micropdf

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
)

func matrixStrings(m barcode.Matrix) []string {
	out := make([]string, len(m.Rows))
	for i, r := range m.Rows {
		var b strings.Builder
		for _, d := range r {
			if d {
				b.WriteByte('1')
			} else {
				b.WriteByte('0')
			}
		}
		out[i] = b.String()
	}
	return out
}

// TestISOVectors reproduces the four symbols of ISO/IEC 24728 Figure 1.
func TestISOVectors(t *testing.T) {
	for _, v := range isoVectors {
		m, err := Encode([]byte(v.data), Options{Mode: v.mode})
		if err != nil {
			t.Fatalf("%s: %v", v.name, err)
		}
		if got := matrixStrings(m); !reflect.DeepEqual(got, v.rows) {
			t.Errorf("%s:\n got %q\nwant %q", v.name, got, v.rows)
		}
	}
}

// zebraTable is the ^BF mode table: columns, rows, % of codewords for EC.
var zebraTable = [34][3]int{
	{1, 11, 64}, {1, 14, 50}, {1, 17, 41}, {1, 20, 40}, {1, 24, 33}, {1, 28, 29},
	{2, 8, 50}, {2, 11, 41}, {2, 14, 32}, {2, 17, 29}, {2, 20, 28}, {2, 23, 28}, {2, 26, 29},
	{3, 6, 67}, {3, 8, 58}, {3, 10, 53}, {3, 12, 50}, {3, 15, 47}, {3, 20, 43}, {3, 26, 41},
	{3, 32, 40}, {3, 38, 39}, {3, 44, 38},
	{4, 6, 50}, {4, 8, 44}, {4, 10, 40}, {4, 12, 38}, {4, 15, 35}, {4, 20, 33}, {4, 26, 31},
	{4, 32, 30}, {4, 38, 29}, {4, 44, 28}, {4, 4, 50},
}

func TestZebraModes(t *testing.T) {
	for m, z := range zebraTable {
		v, err := ZebraMode(m)
		if err != nil {
			t.Fatal(err)
		}
		pct := (100*v.ECCodewords + v.Columns*v.Rows/2) / (v.Columns * v.Rows)
		if v.Columns != z[0] || v.Rows != z[1] || pct < z[2]-1 || pct > z[2]+1 {
			t.Errorf("mode %d: %+v (EC %d%%), want %v", m, v, pct, z)
		}
	}
	if _, err := ZebraMode(34); !errors.Is(err, barcode.ErrInvalidData) {
		t.Error("mode 34 accepted")
	}
}

// TestModesGolden pins all 34 variants; each of these symbols was decoded by
// zxing-cpp back to its input during development ("AB12" for mode 0,
// "MICRO<m>" otherwise).
func TestModesGolden(t *testing.T) {
	want := map[int]string{}
	for _, l := range strings.Split(strings.TrimSpace(modeHashes), "\n") {
		var m int
		var h string
		fmt.Sscanf(l, "%d %s", &m, &h)
		want[m] = h
	}
	for m := 0; m < 34; m++ {
		data := fmt.Sprintf("MICRO%d", m)
		if m == 0 {
			data = "AB12"
		}
		mx, err := Encode([]byte(data), Options{Mode: m})
		if err != nil {
			t.Fatal(err)
		}
		v, _ := ZebraMode(m)
		if mx.Height() != v.Rows || mx.Width() != v.Modules() {
			t.Errorf("mode %d: %dx%d", m, mx.Width(), mx.Height())
		}
		h := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(matrixStrings(mx), "\n"))))[:16]
		if want[m] != h {
			t.Errorf("mode %d: hash %s want %s", m, h, want[m])
		}
	}
}

func TestErrors(t *testing.T) {
	if _, err := Encode([]byte(strings.Repeat("A", 40)), Options{Mode: 0}); !errors.Is(err, barcode.ErrInvalidData) {
		t.Error("overflow accepted")
	}
	if _, err := Encode([]byte(strings.Repeat("\x80", 300)), Options{Mode: ModeAuto}); !errors.Is(err, barcode.ErrInvalidData) {
		t.Error("auto overflow accepted")
	}
	if v, ok := Smallest(0, 10); !ok || v.Columns*v.Rows > 24 {
		t.Errorf("Smallest(0,10) = %+v", v)
	}
}

// modeHashes: mode, first 16 hex digits of SHA-256 of the module rows.
const modeHashes = `
0 7cc2f297aecfb55f
1 e3e0b789d8afe6a4
2 4ecd00f0a875260d
3 02a62eb0124d68c0
4 23e767f29592dde6
5 b6fb4f1711280c75
6 5cce6de39db2384b
7 98a247f54375110c
8 0b33588ea06d5904
9 0eef2e8ec26280c3
10 91dcd6b3ed8b4487
11 7241518f0c4f4844
12 b0098f80b66cc421
13 6fc02feb36ae4ef6
14 dd80369bcf945f65
15 9b51068a80560eb1
16 baa2ad558b2b0100
17 c5ae282528b60d5e
18 4704454ad97d1e16
19 e5e243b019153bf6
20 650f4a1a114e58bd
21 223b70b0485179a5
22 31795a6778b4c444
23 c56f9da84185adbc
24 b46358307f43fba2
25 279c130e077740e4
26 d99acf783b8d4d40
27 6d631e5cb89c5f32
28 846e3d5c4e5b60d8
29 7b38a96dbf95ed7c
30 90562b2998438dca
31 267becc0fb51103c
32 dc49907264f0aa19
33 a02c1166f95af29a
`
