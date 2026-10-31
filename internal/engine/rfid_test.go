package engine_test

import (
	"image"
	"strings"
	"testing"

	zpl "github.com/dchupp/DataMagik-ZPL-To-Image"
)

func ink(img image.Image) int {
	n := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if r, _, _, _ := img.At(x, y).RGBA(); r < 0x8000 {
				n++
			}
		}
	}
	return n
}

// A Zebra RFID printer writes ^RF data to the tag and prints nothing for the
// field; ^HV returns a read to the host. A printer with no encoder (the ZD420,
// fixture q27) prints the data as text.
func TestRFIDFields_PrintOnlyWithoutAnEncoder(t *testing.T) {
	render := func(src string, encoder bool) int {
		imgs, _, err := zpl.Render(strings.NewReader(src), zpl.Options{LabelWidth: 2, LabelHeight: 1, RFIDEncoder: encoder})
		if err != nil || len(imgs) != 1 {
			t.Fatalf("render: %v", err)
		}
		return ink(imgs[0])
	}
	const label = "^XA^RS8^RFW,H,,,E^FD3000ABCDEF0123456789ABCD^FS^FN2^RFR,H,0,8,2^FS^HV2,,TID:^FS^XZ"
	if n := render(label, true); n != 0 {
		t.Fatalf("with an encoder, RFID fields drew %d dots", n)
	}
	if n := render(label, false); n == 0 {
		t.Fatal("without an encoder the tag data prints, as on a ZD420")
	}
}
