package code128

import "github.com/datamagik/zpl-to-image/internal/barcode"

// EncodeComposite encodes GS1-128 data (mode D syntax) as the linear part of
// a GS1 Composite symbol. Per ISO/IEC 24723 7.4.3 an extra code set
// character is placed between the last data character and the check
// character to signal the 2D component: CC-A/B use Code B (final set A),
// Code C (final set B) or Code A (final set C); CC-C uses Code C (final set
// A), Code A (final set B) or Code B (final set C).
func EncodeComposite(data string, ccc bool) (barcode.Linear, error) {
	vals, text, err := Codewords(data, Options{Mode: ModeGS1})
	if err != nil {
		return barcode.Linear{}, err
	}
	set := map[int]int{103: 'A', 104: 'B', 105: 'C'}[vals[0]]
	for i := 1; i < len(vals); i++ {
		v := vals[i]
		switch {
		case v == 98 && set != 'C':
			i++ // shift affects only the next character
		case v == 99 && set != 'C':
			set = 'C'
		case v == 100 && set != 'B':
			set = 'B'
		case v == 101 && set == 'B', v == 101 && set == 'C':
			set = 'A'
		}
	}
	link := map[[2]int]int{
		{'A', 0}: 100, {'B', 0}: 99, {'C', 0}: 101,
		{'A', 1}: 99, {'B', 1}: 101, {'C', 1}: 100,
	}
	k := 0
	if ccc {
		k = 1
	}
	vals = append(vals, link[[2]int{set, k}])
	return linear(vals, text), nil
}
