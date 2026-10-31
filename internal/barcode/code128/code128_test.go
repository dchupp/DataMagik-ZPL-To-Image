package code128

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
)

// runs converts "B8 s4 ..." pixel runs measured from a Zebra-rendered figure
// to module widths.
func runs(s string, module int) []int {
	var out []int
	for _, f := range strings.Fields(s) {
		n, _ := strconv.Atoi(f[1:])
		out = append(out, (n+module/2)/module)
	}
	return out
}

func TestZebraUCCFigure(t *testing.T) {
	// ZPL II guide ^BC figure "U for the m parameter":
	// ^BY4^BC,256,Y,N,,U^FD0012345123451234512^FS (4-dot module).
	want := runs("B8 s4 B4 s8 B12 s8 B16 s4 B4 s4 B12 s4 B8 s4 B8 s8 B8 s8 B4 s4 B8 s8 B12 s8 B4 s12 "+
		"B4 s4 B8 s12 B8 s4 B12 s4 B4 s12 B12 s4 B8 s4 B12 s4 B4 s4 B12 s4 B8 s12 B4 s4 B8 s8 B12 s8 "+
		"B4 s12 B4 s4 B8 s12 B8 s4 B12 s4 B4 s12 B8 s8 B4 s8 B12 s4 B12 s12 B4 s4 B8 s4 B8 s12 B12 s4 B4 s4 B8", 4)
	l, err := Encode("0012345123451234512", Options{Mode: ModeUCC})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(l.Elements, want) {
		t.Errorf("elements\n got %v\nwant %v", l.Elements, want)
	}
	if l.Text != "00123451234512345120" {
		t.Errorf("text %q", l.Text)
	}
	// The same symbol via mode N with the e (UCC check digit) parameter, as
	// in the guide's "N for the m parameter" figure.
	n, err := Encode(">;>80012345123451234512", Options{UCCCheckDigit: true})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(n.Elements, want) || n.Text != l.Text {
		t.Errorf("mode N + e: %v %q", n.Elements, n.Text)
	}
}

func TestCodewords(t *testing.T) {
	tests := []struct {
		name string
		data string
		opts Options
		want []int
		text string
	}{
		{"default B", "CODE128", Options{}, []int{104, 35, 47, 36, 37, 17, 18, 24}, "CODE128"},
		{"explicit B", ">:CODE128", Options{}, []int{104, 35, 47, 36, 37, 17, 18, 24}, "CODE128"},
		{"literal codes", "a>0b><>=", Options{}, []int{104, 65, 30, 66, 62, 94}, "a>b^~"},
		{"subset C", ">;123456", Options{}, []int{105, 12, 34, 56}, "123456"},
		// Guide: non-integer first in a pair is ignored; second invalidates the pair.
		{"C ignore", ">;12D24", Options{}, []int{105, 12, 24}, "1224"},
		{"C invalidate", ">;1D24", Options{}, []int{105, 24}, "24"},
		{"C odd before shift", ">;123>6AB", Options{}, []int{105, 12, 100, 33, 34}, "12AB"},
		{"C to B to A", ">;382436>6CODE>7\x01", Options{}, []int{105, 38, 24, 36, 100, 35, 47, 36, 37, 101, 65}, "382436CODE"},
		{"A subset", ">9AB\x09", Options{}, []int{103, 33, 34, 73}, "AB"},
		{"shift", ">:a>4\x0db", Options{}, []int{104, 65, 98, 77, 66}, "ab"},
		{"FNC1 B", ">:>8AB", Options{}, []int{104, 102, 33, 34}, "AB"},
		{"auto digits", "1234", Options{Mode: ModeAuto}, []int{105, 12, 34}, "1234"},
		{"auto mixed", "AB12345678", Options{Mode: ModeAuto}, []int{104, 33, 34, 99, 12, 34, 56, 78}, "AB12345678"},
		{"auto odd", "12345X", Options{Mode: ModeAuto}, []int{105, 12, 34, 100, 21, 56}, "12345X"},
		{"auto short digits", "A12B", Options{Mode: ModeAuto}, []int{104, 33, 17, 18, 34}, "A12B"},
		{"auto control", "A\x01B", Options{Mode: ModeAuto}, []int{103, 33, 65, 34}, "AB"},
		{"auto shift", "a\x01b", Options{Mode: ModeAuto}, []int{104, 65, 98, 65, 66}, "ab"},
		// m=A takes the field data literally: ">8" is '>' and '8', not FNC1
		// (a ZD420 encodes it so).
		{"auto literal invocations", "A1>82>0", Options{Mode: ModeAuto},
			[]int{104, 33, 17, 30, 24, 18, 30, 16}, "A1>82>0"},
		{"gs1 sscc", "(00)10084423 7449200940", Options{Mode: ModeGS1},
			[]int{105, 102, 0, 10, 8, 44, 23, 74, 49, 20, 9, 41}, "(00)10084423 7449200941"},
		{"gs1 gtin append", "(01)0061414199999", Options{Mode: ModeGS1},
			[]int{105, 102, 1, 0, 61, 41, 41, 99, 99, 96}, "(01)00614141999996"},
		{"gs1 gtin replace", "(01)00614141999990(10)AB1", Options{Mode: ModeGS1},
			[]int{105, 102, 1, 0, 61, 41, 41, 99, 99, 96, 10, 100, 33, 34, 17}, "(01)00614141999996(10)AB1"},
		{"gs1 variable", "(10)12(21)34", Options{Mode: ModeGS1},
			[]int{105, 102, 10, 12, 102, 21, 34}, "(10)12(21)34"},
		{"gs1 explicit", "(91)0005886>8(10)0000410549>8(99)05", Options{Mode: ModeGS1},
			[]int{104, 102, 25, 99, 10, 0, 58, 86, 102, 10, 0, 0, 41, 5, 49, 102, 99, 5}, "(91)0005886(10)0000410549(99)05"},
		// Short data is padded with zeros on the left (a ZD420 encodes "12"
		// as 0000000000000000012 and check digit 3, and prints "123").
		{"ucc pad", "12", Options{Mode: ModeUCC}, []int{105, 102, 0, 0, 0, 0, 0, 0, 0, 0, 1, 23}, "123"},
	}
	for _, tc := range tests {
		got, text, err := Codewords(tc.data, tc.opts)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if !reflect.DeepEqual(got, tc.want) || text != tc.text {
			t.Errorf("%s: got %v %q, want %v %q", tc.name, got, text, tc.want, tc.text)
		}
	}
}

func TestCheckAndStop(t *testing.T) {
	// "PJJ123C" in subset B: 104+48+2*42+3*42+4*17+5*18+6*19+7*35 = 879,
	// 879 mod 103 = 55.
	l, err := Encode("PJJ123C", Options{})
	if err != nil {
		t.Fatal(err)
	}
	el := l.Elements
	check := el[len(el)-13 : len(el)-7]
	if !reflect.DeepEqual(check, []int{3, 1, 1, 3, 2, 1}) { // value 55
		t.Errorf("check pattern %v", check)
	}
	if !reflect.DeepEqual(el[len(el)-7:], []int{2, 3, 3, 1, 1, 1, 2}) {
		t.Errorf("stop pattern %v", el[len(el)-7:])
	}
	if l.Ratio || len(el)%2 != 1 {
		t.Errorf("ratio %v, %d elements", l.Ratio, len(el))
	}
}

func TestEmpty(t *testing.T) {
	if _, err := Encode("", Options{}); !errors.Is(err, barcode.ErrInvalidData) {
		t.Errorf("err = %v", err)
	}
}

// Regression tests from the encoder review.

func TestAutoModeExtendedNotShifted(t *testing.T) {
	// "aÄb" in UTF-8: the Ä bytes are extended characters. A Shift before
	// FNC4 would apply to the FNC4 only and corrupt the character.
	vals, _, err := Codewords("a\xc3\x84b", Options{Mode: ModeAuto})
	if err != nil {
		t.Fatal(err)
	}
	if got := decodeVals(t, vals); got != "a\xc3\x84b" {
		t.Errorf("decodes as %q", got)
	}
}

func TestGS1DataParentheses(t *testing.T) {
	vals, text, err := Codewords("(21)AB(C)D", Options{Mode: ModeGS1})
	if err != nil {
		t.Fatal(err)
	}
	if got := decodeVals(t, vals); got != "\x1d21AB(C)D" {
		t.Errorf("decodes as %q (text %q)", got, text)
	}
}

func TestModeNDroppedShift(t *testing.T) {
	// In subset B, >4 shifts 'a' into subset A, which has no 'a': the
	// character is dropped and no Shift may remain to corrupt 'b'.
	vals, _, err := Codewords(">:>4ab", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := decodeVals(t, vals); got != "b" {
		t.Errorf("decodes as %q", got)
	}
}

// decodeVals is a reference Code 128 decoder for symbol values (start
// through data, no check/stop). FNC1 decodes as GS (0x1D).
func decodeVals(t *testing.T, vals []int) string {
	t.Helper()
	set := vals[0] - 103 // 0 A, 1 B, 2 C
	var out []byte
	fnc4 := false
	shift := false
	for i := 1; i < len(vals); i++ {
		v := vals[i]
		cur := set
		if shift {
			cur = 1 - set
			shift = false
		}
		if cur == 2 {
			switch {
			case v < 100:
				out = append(out, byte('0'+v/10), byte('0'+v%10))
			case v == 100:
				set = 1
			case v == 101:
				set = 0
			case v == 102:
				out = append(out, 0x1d)
			}
			continue
		}
		switch {
		case v == 98:
			shift = true
		case v == 99:
			set = 2
		case v == 100 && cur == 0, v == 101 && cur == 1:
			if cur == 0 {
				set = 1
			} else {
				set = 0
			}
		case v == 101 && cur == 0, v == 100 && cur == 1:
			fnc4 = true
		case v == 102:
			out = append(out, 0x1d)
		case v == 96 || v == 97:
		default:
			var c int
			if cur == 0 {
				if v < 64 {
					c = v + 32
				} else {
					c = v - 64
				}
			} else {
				c = v + 32
			}
			if fnc4 {
				c += 128
				fnc4 = false
			}
			out = append(out, byte(c))
		}
	}
	return string(out)
}

// TestAutoOddDigitsLast pins a ZD420's encoding of ^BCN,,,,,A^FD12345678901:
// Start C, the five pairs, Code B, then the odd digit.
func TestAutoOddDigitsLast(t *testing.T) {
	vals, _, err := Codewords("12345678901", Options{Mode: ModeAuto})
	if err != nil {
		t.Fatal(err)
	}
	if want := []int{105, 12, 34, 56, 78, 90, 100, 17}; !reflect.DeepEqual(vals, want) {
		t.Errorf("got %v, want %v", vals, want)
	}
}

// TestAutoModeZD420 pins m=A subset choices measured on a ZD420 (bars read
// back with ^HG): a run of four or more digits uses subset C; at the start
// an odd run is encoded as pairs followed by the last digit in subset B,
// elsewhere the first digit is encoded before Code C; two digits alone stay
// in subset B; invocation codes are literal characters.
func TestAutoModeZD420(t *testing.T) {
	tests := []struct {
		data string
		want []int
	}{
		{"AB1234567", []int{104, 33, 34, 17, 99, 23, 45, 67}},
		{"1234567", []int{105, 12, 34, 56, 100, 23}},
		{"12", []int{104, 17, 18}},
		{"123", []int{104, 17, 18, 19}},
		{"1234", []int{105, 12, 34}},
		{"12345", []int{105, 12, 34, 100, 21}},
		{"AB1234", []int{104, 33, 34, 99, 12, 34}},
		{"AB123", []int{104, 33, 34, 17, 18, 19}},
		{"AB12345CD", []int{104, 33, 34, 17, 99, 23, 45, 100, 35, 36}},
		{"AB123456CD", []int{104, 33, 34, 99, 12, 34, 56, 100, 35, 36}},
		{"abc12345", []int{104, 65, 66, 67, 17, 99, 23, 45}},
		{"A123456789", []int{104, 33, 17, 99, 23, 45, 67, 89}},
		{"12345AB", []int{105, 12, 34, 100, 21, 33, 34}},
		{"A12345>8678", []int{104, 33, 17, 99, 23, 45, 100, 30, 99, 86, 78}},
		{">;1234", []int{104, 30, 27, 99, 12, 34}},
		{"ab\x09cd", []int{104, 65, 66, 98, 73, 67, 68}},
		{"AB\x09ab", []int{103, 33, 34, 73, 100, 65, 66}},
	}
	for _, tc := range tests {
		got, _, err := Codewords(tc.data, Options{Mode: ModeAuto})
		if err != nil {
			t.Errorf("%q: %v", tc.data, err)
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q: got %v, want %v", tc.data, got, tc.want)
		}
	}
	// m=D shares the subset rules after its leading FNC1.
	got, _, err := Codewords("(21)A12345", Options{Mode: ModeGS1})
	if err != nil {
		t.Fatal(err)
	}
	if want := []int{104, 102, 18, 17, 33, 17, 99, 23, 45}; !reflect.DeepEqual(got, want) {
		t.Errorf("(21)A12345: got %v, want %v", got, want)
	}
}
