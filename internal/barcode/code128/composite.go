package code128

import (
	"fmt"
	"strings"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
)

// Link selects the 2D component that a composite linear symbol signals.
type Link int

const (
	LinkNone Link = iota // no 2D component: no linkage character
	LinkAB               // CC-A or CC-B
	LinkC                // CC-C
)

// EncodeComposite encodes GS1-128 data (mode D syntax) as the linear part of
// a GS1 Composite symbol. Per ISO/IEC 24723 7.4.3 an extra code set
// character is placed between the last data character and the check
// character to signal the 2D component: CC-A/B use Code B (final set A),
// Code C (final set B) or Code A (final set C); CC-C uses Code C (final set
// A), Code A (final set B) or Code B (final set C).
//
// Subsets follow a ZD420: subset A is preferred over B when either would
// do, and bracketed data is encoded as its text, parentheses included,
// after FNC1.
func EncodeComposite(data string, link Link) (barcode.Linear, error) {
	vals, text, err := compositeCodewords(data)
	if err != nil {
		return barcode.Linear{}, err
	}
	if link == LinkNone {
		return linear(vals, text), nil
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
	links := map[[2]int]int{
		{'A', 0}: 100, {'B', 0}: 99, {'C', 0}: 101,
		{'A', 1}: 99, {'B', 1}: 101, {'C', 1}: 100,
	}
	k := 0
	if link == LinkC {
		k = 1
	}
	vals = append(vals, links[[2]int{set, k}])
	return linear(vals, text), nil
}

func compositeCodewords(data string) ([]int, string, error) {
	var toks []int
	text := data
	if strings.HasPrefix(data, "(") {
		toks = []int{tokFNC1}
		for i := 0; i < len(data); i++ {
			if data[i] >= 128 {
				return nil, "", fmt.Errorf("%w: code128: GS1 data must be ASCII", barcode.ErrInvalidData)
			}
			toks = append(toks, int(data[i]))
		}
	} else {
		toks, text = parseGS1(data)
	}
	vals := autoEncodeTie(toks, setA)
	if len(vals) <= 2 {
		return nil, "", fmt.Errorf("%w: code128: no encodable data in %q", barcode.ErrInvalidData, data)
	}
	return vals, text, nil
}
