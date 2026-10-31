package zpl

import (
	"image"
	"os"
	"strings"
	"testing"
)

func loadFont(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/fonts/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestArabicShaping(t *testing.T) {
	o := Options{LabelWidth: 4, LabelHeight: 1, FallbackFonts: [][]byte{loadFont(t, "NotoSansArabic.ttf")}}
	const word = "سلام" // joined letters are narrower than isolated forms
	plain := render1With(t, "^XA^CI28^FO20,20^A0N,60^FD"+word+"^FS^XZ", o)
	shaped := render1With(t, "^XA^CI28^PA0,1,1,1^FO20,20^A0N,60^FD"+word+"^FS^XZ", o)
	if same(plain, shaped) {
		t.Fatal("^PA shaping had no effect")
	}
	if inkCount(shaped) == 0 {
		t.Fatal("no Arabic glyphs drawn (fallback font not used)")
	}
}

func TestBidiReordering(t *testing.T) {
	o := Options{LabelWidth: 4, LabelHeight: 1, FallbackFonts: [][]byte{loadFont(t, "NotoSansHebrew.ttf")}}
	// With bidi on, "ABC" followed by Hebrew renders the Hebrew reversed;
	// the Latin part stays first, so the left edge is unchanged while the
	// Hebrew part differs.
	logical := render1With(t, "^XA^CI28^FO20,20^A0N,60^FDABC אבג^FS^XZ", o)
	visual := render1With(t, "^XA^CI28^PA0,1,0,0^FO20,20^A0N,60^FDABC אבג^FS^XZ", o)
	reversedInput := render1With(t, "^XA^CI28^FO20,20^A0N,60^FDABC גבא^FS^XZ", o)
	if same(logical, visual) {
		t.Error("bidi reordering had no effect")
	}
	if !same(visual, reversedInput) {
		t.Error("bidi output should equal the visually ordered input")
	}
}

func TestDevanagariShapingInTextBlock(t *testing.T) {
	o := Options{LabelWidth: 4, LabelHeight: 1, FallbackFonts: [][]byte{loadFont(t, "NotoSansDevanagari.ttf")}}
	// क्षि: a conjunct plus a pre-base vowel sign that shaping reorders.
	plain := render1With(t, "^XA^CI28^FO20,20^A0N,60^TBN,700,100^FDक्षि^FS^XZ", o)
	shaped := render1With(t, "^XA^CI28^PA0,0,1,1^FO20,20^A0N,60^TBN,700,100^FDक्षि^FS^XZ", o)
	if same(plain, shaped) {
		t.Error("Devanagari shaping in ^TB had no effect")
	}
}

func TestFallbackForBitmapFont(t *testing.T) {
	// The bitmap fonts' character set has no Arabic: without a fallback
	// the characters print as spaces.
	without := render1With(t, "^XA^CI28^FO20,20^ADN,36,20^FDسلام^FS^XZ", Options{LabelWidth: 4, LabelHeight: 1})
	with := render1With(t, "^XA^CI28^FO20,20^ADN,36,20^FDسلام^FS^XZ",
		Options{LabelWidth: 4, LabelHeight: 1, FallbackFonts: [][]byte{loadFont(t, "NotoSansArabic.ttf")}})
	if inkCount(without) != 0 || inkCount(with) == 0 {
		t.Errorf("ink without fallback %d, with %d", inkCount(without), inkCount(with))
	}
}

func render1With(t *testing.T, src string, o Options) image.Image {
	t.Helper()
	imgs, _, err := Render(strings.NewReader(src), o)
	if err != nil || len(imgs) != 1 {
		t.Fatalf("render: %d images, %v", len(imgs), err)
	}
	return imgs[0]
}
