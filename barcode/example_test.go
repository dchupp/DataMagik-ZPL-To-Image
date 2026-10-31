package barcode_test

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"

	"github.com/dchupp/DataMagik-ZPL-To-Image/barcode"
)

// Encode a QR Code with one pixel per module, then scale and recolour it.
func Example() {
	img, err := barcode.Encode("qr", "https://example.com", barcode.Options{ECLevel: "H"})
	if err != nil {
		panic(err)
	}
	fmt.Println("modules:", img.Bounds().Dx(), "x", img.Bounds().Dy())

	// Scale by 8 with nearest neighbour and draw dark blue on transparent.
	const scale = 8
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx()*scale, b.Dy()*scale))
	ink := image.NewUniform(color.NRGBA{0x10, 0x20, 0x60, 0xff})
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			if img.GrayAt(x, y).Y == 0 {
				draw.Draw(out, image.Rect(x*scale, y*scale, (x+1)*scale, (y+1)*scale), ink, image.Point{}, draw.Src)
			}
		}
	}
	fmt.Println("image:", out.Bounds().Dx(), "x", out.Bounds().Dy())
	// Output:
	// modules: 29 x 29
	// image: 232 x 232
}

// Linear symbols are one module high by default; Height sets the bar
// height in modules.
func ExampleEncode_linear() {
	img, err := barcode.Encode("code128", "ABC-123", barcode.Options{ModuleWidth: 2, Height: 40})
	if err != nil {
		panic(err)
	}
	fmt.Println(img.Bounds().Dx(), "x", img.Bounds().Dy())
	// Output: 224 x 80
}

// EAN-13 computes the check digit, and checks one that is given.
func ExampleEncode_checkDigit() {
	_, err := barcode.Encode("ean13", "5901234123450", barcode.Options{})
	fmt.Println(err)
	// Output: barcode: invalid data: barcode: check digit mismatch: ean13: "5901234123450" ends in 0, want 7
}

func ExampleSymbologies() {
	for _, s := range barcode.Symbologies()[:3] {
		fmt.Println(s.Name, s.Aliases, s.TwoD)
	}
	// Output:
	// code128 [c128] false
	// gs1-128 [ean128 ucc128] false
	// code39 [c39 3of9 code3of9] false
}
