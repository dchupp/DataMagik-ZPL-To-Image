package verify

import (
	"image"
	"strings"
	"testing"

	"github.com/makiuchi-d/gozxing"

	zpl "github.com/dchupp/DataMagik-ZPL-To-Image"
)

// TestRenderedLabelsDecode renders single-symbol labels through the public
// API and decodes them, checking the whole pipeline (parameters, scaling,
// placement, rotation) rather than the encoders alone.
func TestRenderedLabelsDecode(t *testing.T) {
	cases := []struct {
		zpl    string
		format gozxing.BarcodeFormat
		want   string
	}{
		{"^BY2^BCN,80,Y,N,N^FDHello 128^FS", gozxing.BarcodeFormat_CODE_128, "Hello 128"},
		{"^BY2^BCN,80,Y,N,N,A^FD1234567890ab^FS", gozxing.BarcodeFormat_CODE_128, "1234567890ab"},
		{"^BY2^BCR,80,Y,N,N^FDROTATED^FS", gozxing.BarcodeFormat_CODE_128, "ROTATED"},
		{"^BY2^BCB,80,Y,N,N^FDBOTTOMUP^FS", gozxing.BarcodeFormat_CODE_128, "BOTTOMUP"},
		{"^BY2^BCI,80,Y,N,N^FDINVERTED^FS", gozxing.BarcodeFormat_CODE_128, "INVERTED"},
		{"^BY2,3^B3N,N,80,Y,N^FDCODE39^FS", gozxing.BarcodeFormat_CODE_39, "CODE39"},
		{"^BY2^BAN,80,Y,N^FDCODE93^FS", gozxing.BarcodeFormat_CODE_93, "CODE93"},
		{"^BY2,2.5^B2N,80,Y,N^FD12345678^FS", gozxing.BarcodeFormat_ITF, "12345678"},
		{"^BY2^BEN,80,Y,N^FD400638133393^FS", gozxing.BarcodeFormat_EAN_13, "4006381333931"},
		{"^BY2^B8N,80,Y,N^FD1234567^FS", gozxing.BarcodeFormat_EAN_8, "12345670"},
		{"^BY2^BUN,80,Y,N^FD07000002198^FS", gozxing.BarcodeFormat_UPC_A, "070000021985"},
		{"^BY2^B9N,80,Y,N^FD1230000045^FS", gozxing.BarcodeFormat_UPC_E, "01234531"},
		{"^BY2^BKN,N,80,Y,N,A,B^FD123456^FS", gozxing.BarcodeFormat_CODABAR, "123456"},
		{"^BQN,2,5^FDMA,QR via ZPL^FS", gozxing.BarcodeFormat_QR_CODE, "QR via ZPL"},
		{"^BXN,6,200^FDData Matrix via ZPL^FS", gozxing.BarcodeFormat_DATA_MATRIX, "Data Matrix via ZPL"},
		{"^B0N,6,N,0^FDAztec via ZPL^FS", gozxing.BarcodeFormat_AZTEC, "Aztec via ZPL"},
		{"^BRN,1,3,1,25^FD0001234567890^FS", gozxing.BarcodeFormat_RSS_14, "00012345678905"},
	}
	for _, c := range cases {
		src := "^XA^FO60,60" + c.zpl + "^XZ"
		imgs, warnings, err := zpl.Render(strings.NewReader(src), zpl.Options{LabelWidth: 4, LabelHeight: 4})
		if err != nil || len(imgs) != 1 || len(warnings) > 0 {
			t.Errorf("%s: %v %v", c.zpl, err, warnings)
			continue
		}
		got, err := Decode(rgb(imgs[0]), c.format)
		if err != nil || got != c.want {
			t.Errorf("%s: decoded %q, %v; want %q", c.zpl, got, err, c.want)
		}
	}
}

// rgb crops img to its ink plus a 40-dot quiet zone and converts it to
// grayscale for gozxing.
func rgb(img image.Image) image.Image {
	b := img.Bounds()
	ink := image.Rectangle{}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if r, _, _, _ := img.At(x, y).RGBA(); r < 0x8000 {
				ink = ink.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	ink = ink.Inset(-40).Intersect(b)
	out := image.NewGray(image.Rect(0, 0, ink.Dx(), ink.Dy()))
	for y := 0; y < ink.Dy(); y++ {
		for x := 0; x < ink.Dx(); x++ {
			out.Set(x, y, img.At(ink.Min.X+x, ink.Min.Y+y))
		}
	}
	return out
}
