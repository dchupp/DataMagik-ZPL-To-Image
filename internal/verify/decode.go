package verify

import (
	"image"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/aztec"
	"github.com/makiuchi-d/gozxing/datamatrix"
	"github.com/makiuchi-d/gozxing/oned"
	"github.com/makiuchi-d/gozxing/oned/rss"
	"github.com/makiuchi-d/gozxing/qrcode"
)

// Decode reads one barcode of the given format from img and returns its text.
func Decode(img image.Image, format gozxing.BarcodeFormat) (string, error) {
	bmp, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		return "", err
	}
	hints := map[gozxing.DecodeHintType]interface{}{
		gozxing.DecodeHintType_TRY_HARDER:       true,
		gozxing.DecodeHintType_PURE_BARCODE:     format != gozxing.BarcodeFormat_AZTEC,
		gozxing.DecodeHintType_POSSIBLE_FORMATS: []gozxing.BarcodeFormat{format},
	}
	var r gozxing.Reader
	switch format {
	case gozxing.BarcodeFormat_QR_CODE:
		r = qrcode.NewQRCodeReader()
	case gozxing.BarcodeFormat_DATA_MATRIX:
		r = datamatrix.NewDataMatrixReader()
	case gozxing.BarcodeFormat_AZTEC:
		r = aztec.NewAztecReader()
	case gozxing.BarcodeFormat_RSS_14:
		r = rss.NewRSS14Reader()
	default:
		r = oned.NewMultiFormatUPCEANReader(hints)
		switch format {
		case gozxing.BarcodeFormat_CODE_128:
			r = oned.NewCode128Reader()
		case gozxing.BarcodeFormat_CODE_39:
			r = oned.NewCode39ReaderWithFlags(false, true)
		case gozxing.BarcodeFormat_CODE_93:
			r = oned.NewCode93Reader()
		case gozxing.BarcodeFormat_ITF:
			r = oned.NewITFReader()
		case gozxing.BarcodeFormat_CODABAR:
			r = oned.NewCodaBarReader()
		}
	}
	res, err := r.Decode(bmp, hints)
	if err != nil {
		return "", err
	}
	return res.GetText(), nil
}
