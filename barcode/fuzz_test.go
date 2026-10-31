package barcode

import (
	"errors"
	"testing"
)

// FuzzEncode checks that any data and options either encode to a
// well-formed black-on-white image or fail with one of the package errors,
// and never panic.
//
//	go test -run XXX -fuzz FuzzEncode -fuzztime 60s ./barcode
func FuzzEncode(f *testing.F) {
	for i, s := range Symbologies() {
		f.Add(uint8(i), samples[s.Name], uint8(0), uint8(0), uint8(0))
	}
	f.Add(uint8(0), "", uint8(0), uint8(0), uint8(0))
	f.Add(uint8(19), "\xff\xfe invalid UTF-8", uint8(3), uint8(7), uint8(9))
	syms := Symbologies()
	f.Fuzz(func(t *testing.T, sym uint8, data string, level, columns, flags uint8) {
		if len(data) > 2000 {
			return // keep each run fast; length limits are the encoders' own tests
		}
		o := Options{
			ECLevel:     []string{"", "L", "M", "Q", "H"}[int(level)%5],
			Security:    int(level) % 9,
			ECPercent:   int(level) % 100,
			Layers:      int(columns) % 33,
			Columns:     int(columns) % 40,
			Rows:        int(flags) % 50,
			RowHeight:   int(flags) % 10,
			Mode:        []int{0, 2, 3, 4, 5, 6}[int(flags)%6],
			Ratio:       []float64{0, 2, 2.5, 3}[int(flags)%4],
			CheckDigit:  flags&1 != 0,
			GS1:         flags&2 != 0,
			Compact:     flags&4 != 0,
			Rectangular: flags&8 != 0,
		}
		name := syms[int(sym)%len(syms)].Name
		img, err := Encode(name, data, o)
		if err != nil {
			if !errors.Is(err, ErrInvalidData) && !errors.Is(err, ErrInvalidOption) {
				t.Fatalf("%s %q %+v: unexpected error %v", name, data, o, err)
			}
			return
		}
		if img.Bounds().Empty() {
			t.Fatalf("%s %q: empty image", name, data)
		}
		for _, p := range img.Pix {
			if p != 0 && p != 255 {
				t.Fatalf("%s %q: pixel value %d", name, data, p)
			}
		}
	})
}
