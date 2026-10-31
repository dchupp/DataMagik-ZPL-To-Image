package pdf417

import (
	"reflect"
	"strings"
	"testing"
)

// textTokens decodes Text Compaction codewords (starting in Alpha) into
// readable tokens: characters ("_" for a space), and the latch/shift names
// ll, ml, al, pl, as, ps. Codewords >= 900 appear as <n>.
func textTokens(cws []int) string {
	toks, _ := decodeText(cws)
	return toks
}

// decodeText returns textTokens' tokens and the decoded text.
func decodeText(cws []int) (string, string) {
	var text strings.Builder
	const mixed = "0123456789&\r\t,:#-.$/+%*=^"
	const punct = ";<>@[\\]_`~!\r\t,:\n-.$/\"|*()?{}'"
	var vals []int
	var toks []string
	for _, c := range cws {
		if c >= 900 {
			toks = append(toks, "<"+string(rune('0'+c/100%10))+">")
			continue
		}
		vals = append(vals, c/30, c%30)
	}
	sub, shift := 'A', rune(0)
	for _, v := range vals {
		cur := sub
		if shift != 0 {
			cur, shift = shift, 0
		}
		switch {
		case v == 26 && cur != 'P':
			toks = append(toks, "_")
			text.WriteByte(' ')
		case cur == 'A' && v < 26:
			toks = append(toks, string(rune('A'+v)))
			text.WriteByte(byte('A' + v))
		case cur == 'L' && v < 26:
			toks = append(toks, string(rune('a'+v)))
			text.WriteByte(byte('a' + v))
		case cur == 'M' && v < 25:
			toks = append(toks, string(mixed[v]))
			text.WriteByte(mixed[v])
		case cur == 'P' && v < 29:
			toks = append(toks, string(punct[v]))
			text.WriteByte(punct[v])
		case cur == 'P':
			toks, sub = append(toks, "al"), 'A'
		case v == 27 && cur == 'L':
			toks, shift = append(toks, "as"), 'A'
		case v == 27:
			toks, sub = append(toks, "ll"), 'L'
		case v == 28 && cur == 'M':
			toks, sub = append(toks, "al"), 'A'
		case v == 28:
			toks, sub = append(toks, "ml"), 'M'
		case v == 25:
			toks, sub = append(toks, "pl"), 'P'
		default: // 29
			toks, shift = append(toks, "ps"), 'P'
		}
	}
	return strings.Join(toks, " "), text.String()
}

// TestCompactZD420Text pins Text Compaction sub-mode choices read back from
// a ZD420 (^B7 symbols decoded from ^HG images). In Alpha or Lower a
// punctuation character is shifted (ps) even when it is also in Mixed,
// unless four or more punctuation characters follow in a row (then ml pl);
// upper case inside lower case is shifted (as) unless five or more follow
// (then ml al). A trailing pad value is dropped before comparing.
func TestCompactZD420Text(t *testing.T) {
	tests := []struct{ data, want string }{
		{"ab:/cd", "ll a b ps : ps / c d"},
		{"ab12cd", "ll a b ml 1 2 ll c d"},
		{"ab.12", "ll a b ps . ml 1 2"},
		{"AB::::CD", "A B ml pl : : : : al C D"},
		{"ab;<>@cd", "ll a b ml pl ; < > @ al ll c d"},
		{"ab;<cd", "ll a b ps ; ps < c d"},
		{"abCd", "ll a b as C d"},
		{"abCDEFgh", "ll a b as C as D as E as F g h"},
		{"abCDE", "ll a b as C as D as E"},
		{"abCDEFG", "ll a b ml al C D E F G"},
		{"abCDEFGh", "ll a b ml al C D E F G ll h"},
		{"AB=CD", "A B ml = al C D"},
		{"ab=cd", "ll a b ml = ll c d"},
		{"1.5ab", "ml 1 . 5 ll a b"},
		{"12:30 x", "ml 1 2 : 3 0 _ ll x"},
		{"ab-cd-12", "ll a b ps - c d ps - ml 1 2"},
		{"x?y=1&z=2", "ll x ps ? y ml = 1 & ll z ml = 2"},
		{"A-B", "A ps - B"},
		{"ab:", "ll a b ps :"},
		{"AB.", "A B ps ."},
		{"AB/12", "A B ps / ml 1 2"},
		{"a@b", "ll a ps @ b"},
		{"AB--CD", "A B ps - ps - C D"},
		{"AB,CD,EF", "A B ps , C D ps , E F"},
		{"ab:::", "ll a b ps : ps : ps :"},
		{"ab;;;", "ll a b ps ; ps ; ps ;"},
		{"ab;;;;", "ll a b ml pl ; ; ; ;"},
		{"1;2", "ml 1 ps ; 2"},
		{"1;;;2", "ml 1 ps ; ps ; ps ; 2"},
		{"1;;;;2", "ml 1 pl ; ; ; ; al ml 2"},
		{"ab:/:/12", "ll a b ml pl : / : / al ml 1 2"},
		{"abC1", "ll a b as C ml 1"},
		{"https://data-magik.com/label-designer?serial=S2026101301",
			"ll h t t p s ps : ps / ps / d a t a ps - m a g i k ps . c o m ps / l a b e l ps - d e s i g n e r ps ? s e r i a l ml = al S ml 2 0 2 6 1 0 1 3 0 1"},
	}
	for _, tc := range tests {
		cws := Compact([]byte(tc.data))
		got, text := decodeText(cws)
		// A trailing pad value may decode as one more shift or latch only.
		if text != tc.data {
			t.Errorf("%q decodes as %q", tc.data, text)
		}
		for _, pad := range []string{" ps", " al"} {
			got = strings.TrimSuffix(got, pad)
		}
		if got != tc.want {
			t.Errorf("%q:\n got %s\nwant %s", tc.data, got, tc.want)
		}
	}
}

// TestCompactZD420URL pins the data codewords of the printer comparison
// fixture q19 as a ZD420 encodes them.
func TestCompactZD420URL(t *testing.T) {
	want := []int{817, 589, 468, 884, 889, 889, 90, 570, 886, 360, 188, 329, 512, 432, 889, 330, 34,
		359, 483, 138, 246, 394, 539, 768, 137, 240, 358, 718, 568, 60, 66, 30, 33, 1}
	if got := Compact([]byte("https://data-magik.com/label-designer?serial=S2026101301")); !reflect.DeepEqual(got, want) {
		t.Errorf("got  %v\nwant %v", got, want)
	}
}

// TestCompactRoundTrip checks that text with every sub-mode transition still
// decodes to its input.
func TestCompactRoundTrip(t *testing.T) {
	for _, s := range []string{
		"Hello, World! abc DEF ;;;; xyz\r\n\t~{}",
		"aBCDEFGhIJKLm;;;;;;n::1",
		"x@y.z/w-v:u?t", "A;B;;C;;;D;;;;E", "1;2;;3;;;4;;;;5", "ab CD EFGHI jk",
	} {
		if _, text := decodeText(Compact([]byte(s))); text != s {
			t.Errorf("%q decodes as %q", s, text)
		}
	}
}
