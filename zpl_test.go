package zpl

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/compress"
)

var fixedNow = func() time.Time { return time.Date(2026, 3, 4, 13, 5, 6, 0, time.UTC) }

func render(t *testing.T, src string, opts ...Options) []image.Image {
	t.Helper()
	o := Options{LabelWidth: 2, LabelHeight: 1, Now: fixedNow}
	if len(opts) > 0 {
		o = opts[0]
		if o.Now == nil {
			o.Now = fixedNow
		}
	}
	imgs, _, err := Render(strings.NewReader(src), o)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return imgs
}

func render1(t *testing.T, src string) image.Image {
	t.Helper()
	imgs := render(t, src)
	if len(imgs) != 1 {
		t.Fatalf("got %d images, want 1", len(imgs))
	}
	return imgs[0]
}

func black(img image.Image, x, y int) bool {
	r, _, _, _ := img.At(x, y).RGBA()
	return r < 0x8000
}

func inkCount(img image.Image) int {
	n := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if black(img, x, y) {
				n++
			}
		}
	}
	return n
}

func same(a, b image.Image) bool {
	if a.Bounds() != b.Bounds() {
		return false
	}
	r := a.Bounds()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if black(a, x, y) != black(b, x, y) {
				return false
			}
		}
	}
	return true
}

func assertSame(t *testing.T, a, b string) {
	t.Helper()
	if !same(render1(t, a), render1(t, b)) {
		t.Errorf("renders differ:\n%s\n%s", a, b)
	}
}

func TestSizeFromPWAndLL(t *testing.T) {
	imgs, _, err := Render(strings.NewReader("^XA^PW300^LL200^XZ"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if b := imgs[0].Bounds(); b.Dx() != 300 || b.Dy() != 200 {
		t.Errorf("size %v, want 300x200", b)
	}
	imgs, _, _ = Render(strings.NewReader("^XA^XZ"), Options{DPMM: 12})
	if b := imgs[0].Bounds(); b.Dx() != 1219 || b.Dy() != 1829 {
		t.Errorf("default 4x6 at 12 dpmm = %v", b)
	}
}

func TestGraphicBox(t *testing.T) {
	img := render1(t, "^XA^FO10,10^GB100,50,5^FS^XZ")
	for _, p := range []image.Point{{10, 10}, {109, 59}, {14, 30}, {60, 12}} {
		if !black(img, p.X, p.Y) {
			t.Errorf("%v should be black", p)
		}
	}
	for _, p := range []image.Point{{9, 10}, {15, 15}, {60, 30}, {110, 30}} {
		if black(img, p.X, p.Y) {
			t.Errorf("%v should be white", p)
		}
	}
}

func TestFieldReverseAndLabelReverse(t *testing.T) {
	img := render1(t, "^XA^FO0,0^GB100,100,100^FS^FO20,20^FR^GB10,10,10^FS^XZ")
	if black(img, 25, 25) || !black(img, 15, 15) {
		t.Error("^FR should clear pixels inside the filled box")
	}
	assertSame(t,
		"^XA^LRY^FO0,0^GB100,100,100^FS^FO20,20^GB10,10,10^FS^LRN^XZ",
		"^XA^FO0,0^GB100,100,100^FS^FO20,20^FR^GB10,10,10^FS^XZ")
}

func TestWhiteBox(t *testing.T) {
	img := render1(t, "^XA^FO0,0^GB100,100,100^FS^FO20,20^GB10,10,10,W^FS^XZ")
	if black(img, 25, 25) {
		t.Error("white box should clear")
	}
}

func TestFieldHexEqualsLiteral(t *testing.T) {
	assertSame(t, "^XA^FO10,10^A0N,30^FH^FD_41_42C^FS^XZ", "^XA^FO10,10^A0N,30^FDABC^FS^XZ")
	assertSame(t, "^XA^FO10,10^A0N,30^FH#^FD#41#42C^FS^XZ", "^XA^FO10,10^A0N,30^FDABC^FS^XZ")
}

func TestCharsetUTF8MatchesCP850(t *testing.T) {
	// é is 0x82 in code page 850 (the ^CI0 upper half) and C3 A9 in UTF-8.
	assertSame(t, "^XA^CI28^FO10,10^A0N,30^FDcafé^FS^XZ", "^XA^CI0^FO10,10^A0N,30^FH^FDcaf_82^FS^XZ")
}

func TestChangedPrefixAndDelimiter(t *testing.T) {
	assertSame(t, "^XA^CD;^CC+\n+FO10;10+A0N;30+FDHi+FS+XZ", "^XA^FO10,10^A0N,30^FDHi^FS^XZ")
}

func TestDownloadGraphicAndRecall(t *testing.T) {
	// 16x2 graphic: first row black, second row left half black, via ACS.
	dg := "~DGR:BAR.GRF,4,2,!FF,\n"
	img := render1(t, dg+"^XA^FO10,10^XGR:BAR.GRF,2,2^FS^XZ")
	if !black(img, 10, 10) || !black(img, 41, 11) || !black(img, 10, 13) || black(img, 41, 13) {
		t.Error("recalled graphic pixels wrong")
	}
	assertSame(t, dg+"^XA^FO10,10^XGBAR^FS^XZ", "^XA^FO10,10^GFA,4,4,2,FFFFFF00^FS^XZ")
}

func TestGraphicFieldZ64(t *testing.T) {
	raw := bytes.Repeat([]byte{0xF0, 0x0F}, 8) // 16 rows of 2 bytes
	var z bytes.Buffer
	w := zlib.NewWriter(&z)
	w.Write(raw)
	w.Close()
	b64 := base64.StdEncoding.EncodeToString(z.Bytes())
	data := fmt.Sprintf(":Z64:%s:%04X", b64, compress.CRC16(b64))
	hex := strings.Repeat("F00F", 8)
	assertSame(t,
		"^XA^FO5,5^GFA,16,16,2,"+data+"^FS^XZ",
		"^XA^FO5,5^GFA,16,16,2,"+hex+"^FS^XZ")
	assertSame(t,
		"^XA^FO5,5^GFB,16,16,2,"+string(raw)+"^FS^XZ",
		"^XA^FO5,5^GFA,16,16,2,"+hex+"^FS^XZ")
}

func TestStoredFormatRecall(t *testing.T) {
	stored := "^XA^DFR:LBL.ZPL^FS^FO10,10^A0N,30^FN1^FS^FO10,60^A0N,30^FN2^FDdefault^FS^XZ"
	p, err := NewPrinter(Options{LabelWidth: 2, LabelHeight: 1})
	if err != nil {
		t.Fatal(err)
	}
	imgs, _, err := p.Render(strings.NewReader(stored))
	if err != nil || len(imgs) != 0 {
		t.Fatalf("^DF should not print: %d images, %v", len(imgs), err)
	}
	imgs, _, _ = p.Render(strings.NewReader("^XA^XFR:LBL.ZPL^FN1^FDHello^FS^XZ"))
	want := render1(t, "^XA^FO10,10^A0N,30^FDHello^FS^FO10,60^A0N,30^FDdefault^FS^XZ")
	if len(imgs) != 1 || !same(imgs[0], want) {
		t.Error("recalled format differs from inline format")
	}
}

func TestSerialNumbers(t *testing.T) {
	imgs := render(t, "^XA^FO10,10^A0N,30^SN0098,1,Y^FS^PQ3^XZ",
		Options{LabelWidth: 2, LabelHeight: 1, ExpandCopies: true})
	if len(imgs) != 3 {
		t.Fatalf("got %d labels, want 3", len(imgs))
	}
	for i, v := range []string{"0098", "0099", "0100"} {
		want := render1(t, "^XA^FO10,10^A0N,30^FD"+v+"^FS^XZ")
		if !same(imgs[i], want) {
			t.Errorf("copy %d should read %s", i, v)
		}
	}
	imgs = render(t, "^XA^FO10,10^A0N,30^SF%ddA,B^FDAB12Z^FS^PQ2^XZ",
		Options{LabelWidth: 2, LabelHeight: 1, ExpandCopies: true})
	if !same(imgs[1], render1(t, "^XA^FO10,10^A0N,30^FDAB13A^FS^XZ")) {
		t.Error("^SF increment wrong")
	}
}

func TestFieldTypesetChaining(t *testing.T) {
	assertSame(t,
		"^XA^FT10,50^A0N,30^FDAB^FS^FT^A0N,30^FDCD^FS^XZ",
		"^XA^FT10,50^A0N,30^FDABCD^FS^XZ")
}

func TestLabelHome(t *testing.T) {
	assertSame(t, "^XA^LH20,30^FO10,10^GB5,5,5^FS^LH0,0^XZ", "^XA^FO30,40^GB5,5,5^FS^XZ")
}

func TestPrintOrientationAndMirror(t *testing.T) {
	img := render1(t, "^XA^POI^FO0,0^GB10,10,10^FS^XZ")
	b := img.Bounds()
	if !black(img, b.Max.X-1, b.Max.Y-1) || black(img, 0, 0) {
		t.Error("^POI should rotate the label 180°")
	}
	img = render1(t, "^XA^PMY^FO0,0^GB10,10,10^FS^XZ")
	if !black(img, b.Max.X-1, 0) || black(img, 0, 0) {
		t.Error("^PMY should mirror the label")
	}
}

func TestMapClearKeepsBackground(t *testing.T) {
	p, _ := NewPrinter(Options{LabelWidth: 2, LabelHeight: 1})
	imgs, _, _ := p.Render(strings.NewReader("^XA^MCN^FO0,0^GB10,10,10^FS^FO50,0^FV^GB10,10,10^FS^XZ^XA^FO20,0^GB10,10,10^FS^MCY^XZ"))
	second := imgs[1]
	if !black(second, 5, 5) || !black(second, 25, 5) || black(second, 55, 5) {
		t.Error("^MCN should keep ^FD fields but not ^FV fields")
	}
}

func TestImageSaveAndLoad(t *testing.T) {
	p, _ := NewPrinter(Options{LabelWidth: 2, LabelHeight: 1})
	imgs, _, _ := p.Render(strings.NewReader("^XA^FO0,0^GB10,10,10^FS^ISR:BG.GRF,N^XZ^XA^ILR:BG.GRF^FO20,0^GB10,10,10^FS^XZ"))
	if len(imgs) != 1 || !black(imgs[0], 5, 5) || !black(imgs[0], 25, 5) {
		t.Error("^IS/^IL should store and reload the label")
	}
}

func TestHalfDensity(t *testing.T) {
	img := render1(t, "^XA^JMB^FO10,10^GB10,10,10^FS^XZ")
	if !black(img, 20, 20) || !black(img, 39, 39) || black(img, 41, 41) {
		t.Error("^JMB should double every dot")
	}
}

func TestFieldEncodingInsert(t *testing.T) {
	assertSame(t,
		"^XA^FN2^FDField FN 2 Data^FS^FE#^FO10,10^A0N,30^FD#2,b,1,4#^FS^XZ",
		"^XA^FO10,10^A0N,30^FDData^FS^XZ")
}

func TestFieldBlockLineBreak(t *testing.T) {
	assertSame(t,
		"^XA^FO10,10^A0N,30^FB300,2^FDONE\\&TWO^FS^XZ",
		"^XA^FO10,10^A0N,30^FB300,2^FDONE^FS^FO10,40^A0N,30^FB300,1^FDTWO^FS^XZ")
}

func TestClock(t *testing.T) {
	assertSame(t, "^XA^FO10,10^A0N,30^FC%^FD%Y-%m-%d %H:%M^FS^XZ", "^XA^FO10,10^A0N,30^FD2026-03-04 13:05^FS^XZ")
}

func TestRotationKeepsInk(t *testing.T) {
	base := inkCount(render1(t, "^XA^FO50,50^A0N,30^FDRotate^FS^XZ"))
	for _, o := range "RIB" {
		got := inkCount(render1(t, fmt.Sprintf("^XA^FO50,50^A0%c,30^FDRotate^FS^XZ", o)))
		if got != base {
			t.Errorf("orientation %c ink %d, want %d", o, got, base)
		}
	}
}

func TestBitmapFontMagnification(t *testing.T) {
	a := inkCount(render1(t, "^XA^FO10,10^ADN,18,10^FDX^FS^XZ"))
	b := inkCount(render1(t, "^XA^FO10,10^ADN,36,20^FDX^FS^XZ"))
	if b < 5*a/2 || b > 5*a {
		t.Errorf("2x magnification ink %d vs %d", b, a)
	}
}

func TestSymbolFontAndDownloadedFont(t *testing.T) {
	if inkCount(render1(t, "^XA^FO10,10^GSN,40,40^FDABCDE^FS^XZ")) == 0 {
		t.Error("^GS printed nothing")
	}
	db := "~DBR:TINY.FNT,N,8,8,7,8,1,TEST,#0041.8.8.0.7.9.FF818181818181FF\n"
	img := render1(t, db+"^XA^FO10,10^A@N,8,8,R:TINY.FNT^FDA^FS^XZ")
	if !black(img, 10, 10) || !black(img, 17, 17) || black(img, 12, 12) {
		t.Error("~DB glyph not drawn as a hollow square")
	}
}

func TestDownloadPNG(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 4, 4))
	for i := range src.Pix {
		src.Pix[i] = 0xff
	}
	src.SetGray(1, 1, color.Gray{})
	var buf bytes.Buffer
	png.Encode(&buf, src)
	b64 := base64.StdEncoding.EncodeToString(buf.Bytes())
	zpl := fmt.Sprintf("~DYR:DOT,P,P,%d,0,:B64:%s:%04X\n^XA^FO10,10^IMR:DOT.PNG^FS^XZ", buf.Len(), b64, compress.CRC16(b64))
	img := render1(t, zpl)
	if !black(img, 11, 11) || black(img, 10, 10) {
		t.Error("~DY PNG not recalled")
	}
}

func TestUnknownCommand(t *testing.T) {
	_, warnings, err := Render(strings.NewReader("^XA^QQ1^XZ"), Options{})
	if err != nil || len(warnings) != 1 {
		t.Errorf("want one warning, got %v %v", warnings, err)
	}
	if _, _, err := Render(strings.NewReader("^XA^QQ1^XZ"), Options{Strict: true}); err == nil {
		t.Error("strict mode should fail")
	}
}

func TestRenderPNG(t *testing.T) {
	pngs, _, err := RenderPNG(strings.NewReader("^XA^FO0,0^GB10,10,10^FS^XZ^XA^XZ"), Options{LabelWidth: 1, LabelHeight: 1})
	if err != nil || len(pngs) != 2 {
		t.Fatalf("got %d PNGs, %v", len(pngs), err)
	}
	if _, err := png.Decode(bytes.NewReader(pngs[0])); err != nil {
		t.Error(err)
	}
}

// TestCopiesMatchStandaloneRenders checks that replaying unchanged fields
// across ^PQ copies gives exactly the image of rendering each copy alone.
func TestCopiesMatchStandaloneRenders(t *testing.T) {
	format := func(serial string) string {
		return "^XA^FO20,20^A0N,40^FDStatic^FS" +
			"^FO20,70^BQN,2,3^FDQA,static qr^FS" +
			"^FT200,120^A0N,40^FD" + serial + "^FS^FT^A0N,40^FD-tail^FS" +
			"^FO200,140^FR^GB150,60,60^FS^FO210,150^A0N,40^FR^FDREV^FS" +
			"^FO20,220^BY2^BCN,50,Y,N,N^FD" + serial + "^FS"
	}
	o := Options{LabelWidth: 3, LabelHeight: 1.5}
	copies := o
	copies.ExpandCopies = true
	imgs := render(t, format("^SN0098,1,Y")+"^PQ4^XZ", copies)
	if len(imgs) != 4 {
		t.Fatalf("%d copies", len(imgs))
	}
	for i, v := range []string{"0098", "0099", "0100", "0101"} {
		want := render(t, format("^FD"+v)+"^XZ", o)[0]
		if !same(imgs[i], want) {
			t.Errorf("copy %d differs from a standalone render of %s", i, v)
		}
	}
}
