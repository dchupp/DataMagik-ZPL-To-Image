// Package code128 encodes Code 128 (ZPL ^BC) including Zebra's subset
// invocation codes and its UCC Case, Automatic and UCC/EAN (GS1-128) modes.
package code128

import (
	"fmt"
	"strings"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/gs1"
)

// Mode is the ^BC m parameter.
type Mode int

const (
	// ModeNone (m=N) honours Zebra subset invocation codes (>9 >: >; >5 >6
	// >7 >8 ...) in the field data; with no start code subset B is used.
	ModeNone Mode = iota
	// ModeUCC (m=U) is UCC Case Mode: exactly 19 digits (extra digits are
	// dropped, missing ones padded with zeros on the right), encoded as
	// Start C, FNC1, the digits and a Mod 10 check digit.
	ModeUCC
	// ModeAuto (m=A) chooses subsets automatically; >8 inserts FNC1.
	ModeAuto
	// ModeGS1 (m=D) is UCC/EAN mode: parenthesised application identifiers
	// and spaces are stripped for encoding (but printed), FNC1 is inserted at
	// the start and after variable-length fields, and check digits of AIs
	// that carry one are computed.
	ModeGS1
)

// Options carries the parsed ^BC parameters relevant to encoding.
type Options struct {
	Mode Mode
	// UCCCheckDigit is the ^BC e parameter: append a Mod 10 (GS1 style) check
	// digit computed over the digits of the data. Ignored in ModeUCC (which
	// always adds one) and ModeGS1 (which computes AI check digits itself).
	UCCCheckDigit bool
}

// Code 128 symbol values with special meaning.
const (
	valFNC3   = 96
	valFNC2   = 97
	valShift  = 98
	valCodeC  = 99
	valCodeB  = 100 // also FNC4 in subset B
	valCodeA  = 101 // also FNC4 in subset A
	valFNC1   = 102
	valStartA = 103
	valStartB = 104
	valStartC = 105
	valStop   = 106
)

// patterns holds bar/space module widths for values 0..106 (106 = stop,
// which includes the final 2-module termination bar).
var patterns = [107]string{
	"212222", "222122", "222221", "121223", "121322", "131222", "122213", "122312", "132212", "221213",
	"221312", "231212", "112232", "122132", "122231", "113222", "123122", "123221", "223211", "221132",
	"221231", "213212", "223112", "312131", "311222", "321122", "321221", "312212", "322112", "322211",
	"212123", "212321", "232121", "111323", "131123", "131321", "112313", "132113", "132311", "211313",
	"231113", "231311", "112133", "112331", "132131", "113123", "113321", "133121", "313121", "211331",
	"231131", "213113", "213311", "213131", "311123", "311321", "331121", "312113", "312311", "332111",
	"314111", "221411", "431111", "111224", "111422", "121124", "121421", "141122", "141221", "112214",
	"112412", "122114", "122411", "142112", "142211", "241211", "221114", "413111", "241112", "134111",
	"111242", "121142", "121241", "114212", "124112", "124211", "411212", "421112", "421211", "212141",
	"214121", "412121", "111143", "111341", "131141", "114113", "114311", "411113", "411311", "113141",
	"114131", "311141", "411131", "211412", "211214", "211232", "2331112",
}

type subset int

const (
	setA subset = iota
	setB
	setC
)

// Encode encodes data per the ^BC rules for opts.Mode. The returned Text is
// the interpretation line as Zebra prints it.
func Encode(data string, opts Options) (barcode.Linear, error) {
	vals, text, err := Codewords(data, opts)
	if err != nil {
		return barcode.Linear{}, err
	}
	return linear(vals, text), nil
}

// Codewords returns the symbol values (start character through data, without
// the check character and stop) and the interpretation line.
func Codewords(data string, opts Options) ([]int, string, error) {
	var vals []int
	var text string
	switch opts.Mode {
	case ModeNone:
		if opts.UCCCheckDigit {
			data += string(gs1.CheckDigit(stripInvocations(data)))
		}
		vals, text = encodeN(data)
	case ModeUCC:
		vals, text = encodeUCC(data)
	case ModeAuto:
		if opts.UCCCheckDigit {
			data += string(gs1.CheckDigit(stripInvocations(data)))
		}
		toks := tokensAuto(data)
		vals = autoEncode(toks)
		text = tokenText(toks)
	case ModeGS1:
		var toks []int
		toks, text = parseGS1(data)
		vals = autoEncode(toks)
	default:
		return nil, "", fmt.Errorf("%w: code128: unknown mode %d", barcode.ErrInvalidData, opts.Mode)
	}
	if len(vals) <= 1 {
		return nil, "", fmt.Errorf("%w: code128: no encodable data in %q", barcode.ErrInvalidData, data)
	}
	return vals, text, nil
}

func linear(vals []int, text string) barcode.Linear {
	sum := vals[0]
	for i, v := range vals[1:] {
		sum += (i + 1) * v
	}
	all := append(append([]int(nil), vals...), sum%103, valStop)
	var el []int
	for _, v := range all {
		for _, c := range patterns[v] {
			el = append(el, int(c-'0'))
		}
	}
	return barcode.Linear{Elements: el, Text: text}
}

// stripInvocations removes Zebra ">x" invocation codes so that a check
// digit is computed over data characters only.
func stripInvocations(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '>' {
			i++
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func valueA(c byte) (int, bool) {
	switch {
	case c < 32:
		return int(c) + 64, true
	case c < 96:
		return int(c) - 32, true
	}
	return 0, false
}

func valueB(c byte) (int, bool) {
	if c >= 32 && c < 128 {
		return int(c) - 32, true
	}
	return 0, false
}

func printable(c byte) bool { return c >= 32 && c != 127 }

// encodeN implements m=N: the data is interpreted with Zebra's invocation
// codes. Characters a subset cannot represent are silently dropped (Zebra
// prints nothing for them). In subset C a non-digit in the first position of
// a pair is ignored, a non-digit in the second position discards the pair,
// and an unpaired digit before a subset change (or at the end) is ignored,
// as described in the ZPL II guide.
func encodeN(data string) ([]int, string) {
	set := setB
	i := 0
	if len(data) >= 2 && data[0] == '>' {
		switch data[1] {
		case '9':
			set, i = setA, 2
		case ':':
			set, i = setB, 2
		case ';':
			set, i = setC, 2
		}
	}
	vals := []int{valStartA + int(set)}
	var text strings.Builder
	pend := -1     // pending first digit of a subset C pair
	shift := false // next character is encoded in the other of A/B
	emitChar := func(c byte) {
		cur := set
		if shift {
			cur = setA + setB - set
			shift = false
		}
		var v int
		var ok bool
		if cur == setA {
			v, ok = valueA(c)
		} else {
			v, ok = valueB(c)
		}
		if ok {
			if cur != set {
				vals = append(vals, valShift)
			}
			vals = append(vals, v)
			if printable(c) {
				text.WriteByte(c)
			}
		}
	}
	switchTo := func(s subset) {
		pend = -1
		if s == set {
			return
		}
		vals = append(vals, [...]int{valCodeA, valCodeB, valCodeC}[s])
		set = s
	}
	for i < len(data) {
		c := data[i]
		if c == '>' {
			if i+1 >= len(data) {
				break // a lone trailing '>' is not a valid invocation
			}
			code := data[i+1]
			i += 2
			// A pending >4 Shift applies only to an immediately following
			// character; any other invocation code cancels it.
			shift = false
			if set == setC {
				switch code {
				case '6', ':':
					switchTo(setB)
				case '7', '9':
					switchTo(setA)
				case '8':
					pend = -1
					vals = append(vals, valFNC1)
				default:
					pend = -1
				}
				continue
			}
			// The value-producing codes map straight to a symbol value
			// in the current subset (the text shows the ASCII equivalent).
			if v, ok := literalCodes[code]; ok {
				shift = false
				vals = append(vals, v)
				switch code {
				case '<':
					text.WriteByte('^')
				case '0':
					text.WriteByte('>')
				case '=':
					if set == setB {
						text.WriteByte('~')
					}
				}
				continue
			}
			switch code {
			case '4':
				shift = set != setC // emitted with the next character
			case '5', ';':
				switchTo(setC)
			case '6':
				if set == setA {
					switchTo(setB)
				} else {
					vals = append(vals, valCodeB) // FNC4 in subset B
				}
			case '7':
				if set == setB {
					switchTo(setA)
				} else {
					vals = append(vals, valCodeA) // FNC4 in subset A
				}
			case '8':
				vals = append(vals, valFNC1)
			case '9':
				switchTo(setA)
			case ':':
				switchTo(setB)
			}
			continue
		}
		i++
		if set == setC {
			if c < '0' || c > '9' {
				pend = -1
				continue
			}
			if pend < 0 {
				pend = int(c - '0')
				continue
			}
			vals = append(vals, pend*10+int(c-'0'))
			fmt.Fprintf(&text, "%02d", pend*10+int(c-'0'))
			pend = -1
			continue
		}
		emitChar(c)
	}
	return vals, text.String()
}

// literalCodes are the >x invocation codes that map straight to a symbol
// value in the current subset.
var literalCodes = map[byte]int{'<': 62, '0': 30, '=': 94, '1': 95, '2': valFNC3, '3': valFNC2}

// Token values used by the automatic encoder; non-negative tokens are bytes.
const (
	tokFNC1 = -1
	tokFNC2 = -2
	tokFNC3 = -3
)

// tokensAuto converts m=A field data to tokens. >8 is FNC1, >2/>3 are
// FNC3/FNC2, and the literal codes >0 (>), >< (^), >= (~) and >1 (DEL) are
// honoured; subset selection codes are ignored because the printer picks
// subsets itself.
func tokensAuto(data string) []int {
	var toks []int
	for i := 0; i < len(data); i++ {
		c := data[i]
		if c != '>' {
			toks = append(toks, int(c))
			continue
		}
		if i+1 >= len(data) {
			break
		}
		i++
		switch data[i] {
		case '8':
			toks = append(toks, tokFNC1)
		case '2':
			toks = append(toks, tokFNC3)
		case '3':
			toks = append(toks, tokFNC2)
		case '0':
			toks = append(toks, '>')
		case '<':
			toks = append(toks, '^')
		case '=':
			toks = append(toks, '~')
		case '1':
			toks = append(toks, 127)
		}
	}
	return toks
}

func tokenText(toks []int) string {
	var b strings.Builder
	for _, t := range toks {
		if t >= 0 && printable(byte(t)) {
			b.WriteByte(byte(t))
		}
	}
	return b.String()
}

func isDigitTok(t int) bool { return t >= '0' && t <= '9' }

func digitRun(toks []int, i int) int {
	n := 0
	for i+n < len(toks) && isDigitTok(toks[i+n]) {
		n++
	}
	return n
}

// fits reports whether token t can be encoded in subset s (A or B).
func fits(t int, s subset) bool {
	if t < 0 {
		return true // FNC1..FNC3 exist in both A and B
	}
	t &= 0x7f // high bytes are encoded with FNC4
	if s == setA {
		return t < 96
	}
	return t >= 32
}

// chooseAB picks subset A if a control character appears before any
// lowercase character from position i on, else subset B (ISO/IEC 15417
// Annex E).
func chooseAB(toks []int, i int) subset {
	for ; i < len(toks); i++ {
		t := toks[i]
		if t < 0 {
			continue
		}
		t &= 0x7f
		if t < 32 {
			return setA
		}
		if t >= 96 {
			return setB
		}
	}
	return setB
}

// autoEncode selects subsets following the ISO/IEC 15417 Annex E rules,
// which match Zebra's documented behaviour (a run of four or more digits
// switches to subset C).
func autoEncode(toks []int) []int {
	j := 0
	for j < len(toks) && toks[j] == tokFNC1 {
		j++
	}
	var set subset
	nd := digitRun(toks, j)
	switch {
	case nd >= 4 && nd%2 == 0, nd == 2 && j+2 == len(toks):
		set = setC
	default:
		set = chooseAB(toks, j)
	}
	vals := []int{valStartA + int(set)}
	enc := func(t int, s subset) {
		switch t {
		case tokFNC1:
			vals = append(vals, valFNC1)
			return
		case tokFNC2:
			vals = append(vals, valFNC2)
			return
		case tokFNC3:
			vals = append(vals, valFNC3)
			return
		}
		if t >= 128 {
			vals = append(vals, [...]int{valCodeA, valCodeB}[s]) // FNC4
			t -= 128
		}
		var v int
		if s == setA {
			v, _ = valueA(byte(t))
		} else {
			v, _ = valueB(byte(t))
		}
		vals = append(vals, v)
	}
	for i := 0; i < len(toks); {
		t := toks[i]
		if set == setC {
			if t == tokFNC1 {
				vals = append(vals, valFNC1)
				i++
				continue
			}
			if digitRun(toks, i) >= 2 {
				vals = append(vals, (t-'0')*10+toks[i+1]-'0')
				i += 2
				continue
			}
			set = chooseAB(toks, i)
			vals = append(vals, [...]int{valCodeA, valCodeB}[set])
			continue
		}
		if nd := digitRun(toks, i); nd >= 4 {
			if nd%2 == 1 {
				enc(t, set)
				i++
			}
			vals = append(vals, valCodeC)
			set = setC
			continue
		}
		if fits(t, set) {
			enc(t, set)
			i++
			continue
		}
		other := setA + setB - set
		// Shift covers one symbol character only, so it cannot carry an
		// extended character (FNC4 + character); switch subsets instead.
		if t < 128 && i+1 < len(toks) && fits(toks[i+1], set) && !fits(toks[i+1], other) {
			vals = append(vals, valShift)
			enc(t, other)
			i++
			continue
		}
		vals = append(vals, [...]int{valCodeA, valCodeB}[other])
		set = other
	}
	return vals
}

// encodeUCC implements m=U (UCC Case Mode).
func encodeUCC(data string) ([]int, string) {
	var d []byte
	for i := 0; i < len(data); i++ {
		if c := data[i]; c >= '0' && c <= '9' {
			d = append(d, c)
		}
	}
	if len(d) > 19 {
		d = d[:19]
	}
	for len(d) < 19 {
		d = append(d, '0')
	}
	s := string(d) + string(gs1.CheckDigit(string(d)))
	toks := []int{tokFNC1}
	for i := 0; i < len(s); i++ {
		toks = append(toks, int(s[i]))
	}
	return autoEncode(toks), s
}

// gs1CheckLen returns the length of the data field of AIs that end in a
// Mod 10 check digit, or 0.
func gs1CheckLen(ai string) int {
	switch {
	case ai == "00", ai == "8017", ai == "8018":
		return 18
	case ai == "01", ai == "02", ai == "03":
		return 14
	case len(ai) == 3 && ai[:2] == "41" && ai[2] >= '0' && ai[2] <= '7':
		return 13
	}
	return 0
}

// parseGS1 implements m=D. It returns the tokens to encode (starting with
// FNC1) and the interpretation line (data with parentheses and spaces kept,
// >8 removed and check digits filled in).
func parseGS1(data string) ([]int, string) {
	toks := []int{tokFNC1}
	var text strings.Builder
	type field struct {
		ai       string
		raw      []byte // field text as printed (spaces kept)
		explicit bool   // ended by an explicit >8
	}
	fields := []field{{}} // data before the first AI
	cur := &fields[0]
	for i := 0; i < len(data); i++ {
		c := data[i]
		switch {
		case c == '(':
			ai, n := gs1.AI(data[i:])
			if n == 0 {
				cur.raw = append(cur.raw, c) // a data parenthesis
				continue
			}
			fields = append(fields, field{ai: ai})
			cur = &fields[len(fields)-1]
			i += n - 1
		case c == '>' && i+1 < len(data):
			if data[i+1] == '8' {
				cur.explicit = true
			}
			i++
		default:
			cur.raw = append(cur.raw, c)
		}
	}
	for k := range fields {
		f := &fields[k]
		if n := gs1CheckLen(f.ai); n > 0 {
			var digits []int // indexes of digits in raw
			for i, c := range f.raw {
				if c >= '0' && c <= '9' {
					digits = append(digits, i)
				}
			}
			switch len(digits) {
			case n:
				last := digits[n-1]
				body := make([]byte, 0, n-1)
				for _, idx := range digits[:n-1] {
					body = append(body, f.raw[idx])
				}
				f.raw[last] = gs1.CheckDigit(string(body))
			case n - 1:
				f.raw = append(f.raw, gs1.CheckDigit(string(f.raw)))
			}
		}
	}
	for k, f := range fields {
		if f.ai != "" {
			text.WriteString("(" + f.ai + ")")
			for i := 0; i < len(f.ai); i++ {
				toks = append(toks, int(f.ai[i]))
			}
		}
		if raw := strings.TrimLeft(string(f.raw), " "); f.ai == "00" && raw != "" {
			// Zebra prints an SSCC as "(00) 1 0084423 7449200941": a space
			// after the AI and after the extension digit (ZPL II guide ^BC
			// figure "D for the m parameter").
			text.WriteString(" " + raw[:1] + " " + strings.TrimLeft(raw[1:], " "))
		} else {
			text.Write(f.raw)
		}
		for _, c := range f.raw {
			if c != ' ' {
				toks = append(toks, int(c))
			}
		}
		last := k == len(fields)-1
		_, fixed := gs1.PredefinedLength(f.ai)
		sep := f.explicit || (f.ai != "" && !fixed)
		if !last && sep && toks[len(toks)-1] != tokFNC1 {
			toks = append(toks, tokFNC1)
		}
	}
	return toks, text.String()
}

// Pattern returns the bar/space module widths of symbol value v (0–106;
// 106 is the stop character including its termination bar).
func Pattern(v int) string { return patterns[v] }
