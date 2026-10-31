package verify

import (
	"testing"

	"github.com/makiuchi-d/gozxing"

	"github.com/datamagik/zpl-to-image/internal/barcode"
	"github.com/datamagik/zpl-to-image/internal/barcode/upcean"
)

func TestUPCEANDecode(t *testing.T) {
	type enc func() (barcode.Linear, error)
	tests := []struct {
		name   string
		f      enc
		format gozxing.BarcodeFormat
		want   string
	}{
		{"EAN-13", func() (barcode.Linear, error) { return upcean.EncodeEAN13("400638133393") }, gozxing.BarcodeFormat_EAN_13, "4006381333931"},
		{"EAN-13 padded", func() (barcode.Linear, error) { return upcean.EncodeEAN13("12345678") }, gozxing.BarcodeFormat_EAN_13, "0000123456784"},
		{"EAN-8", func() (barcode.Linear, error) { return upcean.EncodeEAN8("1234567") }, gozxing.BarcodeFormat_EAN_8, "12345670"},
		{"UPC-A", func() (barcode.Linear, error) {
			return upcean.EncodeUPCA("03600029145", upcean.UPCOptions{PrintCheckDigit: true})
		}, gozxing.BarcodeFormat_UPC_A, "036000291452"},
		{"UPC-E", func() (barcode.Linear, error) {
			return upcean.EncodeUPCE("1230000045", upcean.UPCOptions{PrintCheckDigit: true})
		}, gozxing.BarcodeFormat_UPC_E, "01234531"},
		{"UPC-E 000", func() (barcode.Linear, error) {
			return upcean.EncodeUPCE("1200000789", upcean.UPCOptions{})
		}, gozxing.BarcodeFormat_UPC_E, "01278907"},
		{"UPC-E 4", func() (barcode.Linear, error) {
			return upcean.EncodeUPCE("1234000006", upcean.UPCOptions{})
		}, gozxing.BarcodeFormat_UPC_E, "01234640"},
		{"UPC-E NS1", func() (barcode.Linear, error) {
			return upcean.EncodeUPCE("11234500005", upcean.UPCOptions{})
		}, gozxing.BarcodeFormat_UPC_E, "11234555"},
	}
	for _, tc := range tests {
		l, err := tc.f()
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		got, err := Decode(LinearImage(l, 2, 0, 60), tc.format)
		if err != nil {
			t.Errorf("%s: decode: %v", tc.name, err)
		} else if got != tc.want {
			t.Errorf("%s: decoded %q, want %q", tc.name, got, tc.want)
		}
	}
}
