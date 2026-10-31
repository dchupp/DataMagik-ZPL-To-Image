package verify

import (
	"image"
	"image/draw"
	"testing"

	"github.com/makiuchi-d/gozxing"

	"github.com/dchupp/DataMagik-ZPL-To-Image/barcode"
)

// withQuietZone copies img onto a white canvas with q pixels on every side.
func withQuietZone(img *image.Gray, q int) *image.Gray {
	b := img.Bounds()
	out := blank(b.Dx()+2*q, b.Dy()+2*q)
	draw.Draw(out, b.Add(image.Pt(q, q)), img, b.Min, draw.Src)
	return out
}

// TestPublicBarcodeDecode round-trips the public barcode package's output
// through gozxing. Symbologies gozxing cannot read (PDF417, MicroPDF417,
// MaxiCode, Code 11, MSI, Plessey, 2 of 5, postal, stacked DataBar,
// composites, CODABLOCK-F, Code 49, TLC39) are covered by the encoder
// packages' own vectors.
func TestPublicBarcodeDecode(t *testing.T) {
	tests := []struct {
		sym    string
		data   string
		opts   barcode.Options
		format gozxing.BarcodeFormat
		want   string
	}{
		{"code128", "Hello, World! >8 <tag>", barcode.Options{}, gozxing.BarcodeFormat_CODE_128, "Hello, World! >8 <tag>"},
		{"code128", "1234567890", barcode.Options{}, gozxing.BarcodeFormat_CODE_128, "1234567890"},
		{"code128", "12345", barcode.Options{CheckDigit: true}, gozxing.BarcodeFormat_CODE_128, "123457"},
		// gozxing drops FNC1 without the ASSUME_GS1 hint.
		{"gs1-128", "(01)09501101530003(10)ABC123", barcode.Options{}, gozxing.BarcodeFormat_CODE_128, "010950110153000310ABC123"},
		{"code39", "CODE39-1234", barcode.Options{}, gozxing.BarcodeFormat_CODE_39, "CODE39-1234"},
		{"code39", "Full ASCII: lower", barcode.Options{Ratio: 2.5}, gozxing.BarcodeFormat_CODE_39, "Full ASCII: lower"},
		{"code39", "ABC", barcode.Options{CheckDigit: true}, gozxing.BarcodeFormat_CODE_39, "ABCX"},
		{"logmars", "ABC", barcode.Options{}, gozxing.BarcodeFormat_CODE_39, "ABCX"},
		{"code93", "Code 93 test", barcode.Options{}, gozxing.BarcodeFormat_CODE_93, "Code 93 test"},
		{"itf", "1234567890", barcode.Options{}, gozxing.BarcodeFormat_ITF, "1234567890"},
		{"itf", "123456789", barcode.Options{CheckDigit: true}, gozxing.BarcodeFormat_ITF, "1234567895"},
		{"itf", "12345", barcode.Options{Ratio: 2}, gozxing.BarcodeFormat_ITF, "012345"},
		{"codabar", "A40156-$:/.+B", barcode.Options{}, gozxing.BarcodeFormat_CODABAR, "40156-$:/.+"},
		{"codabar", "31117013206375", barcode.Options{Ratio: 2}, gozxing.BarcodeFormat_CODABAR, "31117013206375"},
		{"ean13", "590123412345", barcode.Options{}, gozxing.BarcodeFormat_EAN_13, "5901234123457"},
		{"ean13", "5901234123457", barcode.Options{}, gozxing.BarcodeFormat_EAN_13, "5901234123457"},
		{"ean8", "9638507", barcode.Options{}, gozxing.BarcodeFormat_EAN_8, "96385074"},
		{"upca", "036000291452", barcode.Options{}, gozxing.BarcodeFormat_UPC_A, "036000291452"},
		{"upce", "0123453", barcode.Options{}, gozxing.BarcodeFormat_UPC_E, "01234531"},
		{"upce", "01234531", barcode.Options{}, gozxing.BarcodeFormat_UPC_E, "01234531"},
		{"upce", "1230000045", barcode.Options{}, gozxing.BarcodeFormat_UPC_E, "01234531"},
		{"upce", "1123455", barcode.Options{}, gozxing.BarcodeFormat_UPC_E, "11234555"},
		{"databar", "0950110153000", barcode.Options{}, gozxing.BarcodeFormat_RSS_14, "09501101530003"},
		{"databar", "(01)09501101530003", barcode.Options{}, gozxing.BarcodeFormat_RSS_14, "09501101530003"},
		{"databar-truncated", "0950110153000", barcode.Options{}, gozxing.BarcodeFormat_RSS_14, "09501101530003"},
		{"qr", "https://example.com/?q=1", barcode.Options{}, gozxing.BarcodeFormat_QR_CODE, "https://example.com/?q=1"},
		{"qr", "UTF-8: café 日本", barcode.Options{ECLevel: "H"}, gozxing.BarcodeFormat_QR_CODE, "UTF-8: café 日本"},
		{"qr", "0123456789ABCDEF", barcode.Options{ECLevel: "L"}, gozxing.BarcodeFormat_QR_CODE, "0123456789ABCDEF"},
		// GS1 QR: FNC1 separators decode as GS.
		{"gs1-qr", "(01)09501101530003(10)AB%1(21)X", barcode.Options{}, gozxing.BarcodeFormat_QR_CODE, "010950110153000310AB%1\x1d21X"},
		{"qr", "(01)09501101530003(10)ABC", barcode.Options{GS1: true}, gozxing.BarcodeFormat_QR_CODE, "010950110153000310ABC"},
		{"datamatrix", "Data Matrix 0123456789", barcode.Options{}, gozxing.BarcodeFormat_DATA_MATRIX, "Data Matrix 0123456789"},
		{"datamatrix", "under_score", barcode.Options{Rectangular: true}, gozxing.BarcodeFormat_DATA_MATRIX, "under_score"},
		{"datamatrix", "sized", barcode.Options{Columns: 32}, gozxing.BarcodeFormat_DATA_MATRIX, "sized"},
		// gozxing reports every Data Matrix FNC1 as GS, the leading one too.
		{"gs1-datamatrix", "(01)09501101530003(10)A_1(21)X", barcode.Options{}, gozxing.BarcodeFormat_DATA_MATRIX, "\x1d010950110153000310A_1\x1d21X"},
		{"datamatrix", "(01)09501101530003", barcode.Options{GS1: true}, gozxing.BarcodeFormat_DATA_MATRIX, "\x1d0109501101530003"},
		{"aztec", "Aztec Code", barcode.Options{}, gozxing.BarcodeFormat_AZTEC, "Aztec Code"},
		{"aztec", `UTF-8 \ caf` + "é", barcode.Options{ECPercent: 50}, gozxing.BarcodeFormat_AZTEC, `UTF-8 \ caf` + "é"},
		{"aztec", "layers", barcode.Options{Layers: 3, Compact: true}, gozxing.BarcodeFormat_AZTEC, "layers"},
	}
	for _, tc := range tests {
		o := tc.opts
		o.ModuleWidth = 3
		img, err := barcode.Encode(tc.sym, tc.data, o)
		if err != nil {
			t.Errorf("%s %q: encode: %v", tc.sym, tc.data, err)
			continue
		}
		if img.Bounds().Dy() == 3 { // a linear symbol: give it height
			o.Height = 30
			img, _ = barcode.Encode(tc.sym, tc.data, o)
		}
		got, err := Decode(withQuietZone(img, 30), tc.format)
		if err != nil {
			t.Errorf("%s %q: decode: %v", tc.sym, tc.data, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s %q: decoded %q, want %q", tc.sym, tc.data, got, tc.want)
		}
	}
}
