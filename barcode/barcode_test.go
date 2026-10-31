package barcode

import (
	"bytes"
	"errors"
	"image"
	"testing"
)

// samples holds valid data for every symbology.
var samples = map[string]string{
	"code128":                  "Hello, World! >8 <tag>",
	"gs1-128":                  "(01)09501101530003(10)ABC123",
	"code39":                   "CODE39-1234",
	"logmars":                  "LOGMARS123",
	"code93":                   "Code 93 test",
	"code11":                   "123-45",
	"codabar":                  "A40156B",
	"itf":                      "1234567890",
	"industrial2of5":           "123456",
	"standard2of5":             "123456",
	"msi":                      "1234567",
	"plessey":                  "01234ABCD",
	"ean13":                    "590123412345",
	"ean8":                     "9638507",
	"upca":                     "03600029145",
	"upce":                     "0123456",
	"postnet":                  "12345",
	"planet":                   "12345678901",
	"imb":                      "01234567094987654321",
	"qr":                       "https://example.com/café",
	"gs1-qr":                   "(01)09501101530003(10)ABC%123",
	"datamatrix":               "Data Matrix 0123456789",
	"gs1-datamatrix":           "(01)09501101530003(10)ABC_123",
	"aztec":                    "Aztec Code é",
	"pdf417":                   "PDF417 sample text 0123456789",
	"micropdf417":              "MicroPDF417",
	"maxicode":                 "MaxiCode sample text",
	"codablock-f":              "CODABLOCK F sample text, a bit longer",
	"code49":                   "CODE 49 TEST",
	"tlc39":                    "123456,ABCd12345678901234",
	"databar":                  "0950110153000",
	"databar-truncated":        "0950110153000",
	"databar-stacked":          "0950110153000",
	"databar-stacked-omni":     "0950110153000",
	"databar-limited":          "1501234567890",
	"databar-expanded":         "(01)98898765432106(3202)012345(15)991231",
	"databar-expanded-stacked": "(01)98898765432106(3202)012345(15)991231",
	"upca-cc":                  "03600029145|(10)ABC123",
	"upce-cc":                  "0123456|(10)ABC123",
	"ean13-cc":                 "331234567890|(10)ABC123",
	"ean8-cc":                  "9638507|(21)A12345678",
	"gs1-128-cc":               "(01)09501101530003|(10)ABC123",
	"gs1-128-ccc":              "(01)09501101530003|(10)ABC123(21)XYZ",
}

func checkImage(t *testing.T, name string, img *image.Gray) {
	t.Helper()
	b := img.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 {
		t.Fatalf("%s: empty image", name)
	}
	var black, white int
	for _, p := range img.Pix {
		switch p {
		case 0:
			black++
		case 255:
			white++
		default:
			t.Fatalf("%s: pixel value %d, want 0 or 255", name, p)
		}
	}
	if black == 0 || white == 0 {
		t.Errorf("%s: %d black and %d white pixels", name, black, white)
	}
	// No quiet zone: ink reaches the left and right edges.
	for _, x := range []int{0, b.Dx() - 1} {
		ink := false
		for y := 0; y < b.Dy(); y++ {
			ink = ink || img.GrayAt(x, y).Y == 0
		}
		if !ink {
			t.Errorf("%s: column %d is blank (quiet zone?)", name, x)
		}
	}
}

func TestSymbologies_AllHaveSamples(t *testing.T) {
	for _, s := range Symbologies() {
		if _, ok := samples[s.Name]; !ok {
			t.Errorf("no sample for %s", s.Name)
		}
		if s.Description == "" {
			t.Errorf("%s: no description", s.Name)
		}
	}
	if len(samples) != len(Symbologies()) {
		t.Errorf("%d samples for %d symbologies", len(samples), len(Symbologies()))
	}
}

func TestEncode_EverySymbology(t *testing.T) {
	for _, s := range Symbologies() {
		t.Run(s.Name, func(t *testing.T) {
			img, err := Encode(s.Name, samples[s.Name], Options{})
			if err != nil {
				t.Fatal(err)
			}
			checkImage(t, s.Name, img)
			for _, a := range s.Aliases {
				alt, err := Encode(a, samples[s.Name], Options{})
				if err != nil {
					t.Fatalf("alias %s: %v", a, err)
				}
				if !bytes.Equal(alt.Pix, img.Pix) {
					t.Errorf("alias %s: different image", a)
				}
			}
		})
	}
}

func TestEncode_LinearIsOneModuleHigh(t *testing.T) {
	for _, s := range Symbologies() {
		if s.TwoD || len(s.Options) == 0 || s.Options[0] != "Height" {
			continue
		}
		switch s.Name {
		case "postnet", "planet", "imb":
			continue
		}
		img, err := Encode(s.Name, samples[s.Name], Options{})
		if err != nil {
			t.Fatal(err)
		}
		if h := img.Bounds().Dy(); h != 1 {
			t.Errorf("%s: height %d, want 1", s.Name, h)
		}
	}
}

func TestEncode_UnknownSymbology(t *testing.T) {
	for _, name := range []string{"", "code-0", "qrcodes", "ean14"} {
		if _, err := Encode(name, "123", Options{}); !errors.Is(err, ErrUnknownSymbology) {
			t.Errorf("%q: err = %v, want ErrUnknownSymbology", name, err)
		}
	}
}

func TestEncode_NameNormalization(t *testing.T) {
	want, err := Encode("gs1-128", samples["gs1-128"], Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"GS1-128", "gs1_128", "GS1 128", "gs1128", "EAN-128", "code128", "Code 128"} {
		o := Options{}
		if name == "code128" || name == "Code 128" {
			o.GS1 = true
		}
		got, err := Encode(name, samples["gs1-128"], o)
		if err != nil {
			t.Fatalf("%q: %v", name, err)
		}
		if !bytes.Equal(got.Pix, want.Pix) {
			t.Errorf("%q: differs from gs1-128", name)
		}
	}
}

func TestEncode_InvalidData(t *testing.T) {
	tests := []struct{ sym, data string }{
		{"ean13", "12345abc"},
		{"ean13", "12345678901234"},
		{"itf", "12AB"},
		{"code128", "café"},
		{"gs1-128", "(01)123"},
		{"gs1-128-cc", "(01)123|(10)A"},
		{"ean13-cc", "12345678901234|(10)A"},
		{"postnet", "12a"},
		{"qr", ""},
		{"upce", "123456789"},
		{"upce", "1234567890"}, // UPC-A form that cannot be zero-suppressed
		{"upce", "2123456"},    // number system 2
	}
	for _, tc := range tests {
		if _, err := Encode(tc.sym, tc.data, Options{}); !errors.Is(err, ErrInvalidData) {
			t.Errorf("%s %q: err = %v, want ErrInvalidData", tc.sym, tc.data, err)
		}
	}
}

func TestEncode_CheckDigit(t *testing.T) {
	tests := []struct {
		sym, without, with, wrong string
	}{
		{"ean13", "590123412345", "5901234123457", "5901234123450"},
		{"ean8", "9638507", "96385074", "96385075"},
		{"upca", "03600029145", "036000291452", "036000291453"},
		{"upce", "0123456", "01234565", "01234566"},
	}
	for _, tc := range tests {
		a, err := Encode(tc.sym, tc.without, Options{})
		if err != nil {
			t.Fatalf("%s %q: %v", tc.sym, tc.without, err)
		}
		b, err := Encode(tc.sym, tc.with, Options{})
		if err != nil {
			t.Fatalf("%s %q: %v", tc.sym, tc.with, err)
		}
		if !bytes.Equal(a.Pix, b.Pix) {
			t.Errorf("%s: %q and %q differ", tc.sym, tc.without, tc.with)
		}
		_, err = Encode(tc.sym, tc.wrong, Options{})
		if !errors.Is(err, ErrCheckDigit) || !errors.Is(err, ErrInvalidData) {
			t.Errorf("%s %q: err = %v, want ErrCheckDigit and ErrInvalidData", tc.sym, tc.wrong, err)
		}
	}
}

func TestEncode_UPCEForms(t *testing.T) {
	// UPC-E 0 123455 is UPC-A 0 12345 00005 (last digit 5..9: product
	// code 0000d).
	tests := []struct{ upce, upca string }{
		{"123455", "1234500005"},
		{"0123455", "01234500005"},
		{"123450", "1200000345"},
		{"123453", "1230000045"},
		{"123454", "1234000005"},
	}
	for _, tc := range tests {
		a, err := Encode("upce", tc.upce, Options{})
		if err != nil {
			t.Fatalf("%q: %v", tc.upce, err)
		}
		b, err := Encode("upce", tc.upca, Options{})
		if err != nil {
			t.Fatalf("%q: %v", tc.upca, err)
		}
		if !bytes.Equal(a.Pix, b.Pix) {
			t.Errorf("UPC-E %q and UPC-A form %q differ", tc.upce, tc.upca)
		}
	}
}

func size(t *testing.T, sym, data string, o Options) (int, int) {
	t.Helper()
	img, err := Encode(sym, data, o)
	if err != nil {
		t.Fatalf("%s %+v: %v", sym, o, err)
	}
	return img.Bounds().Dx(), img.Bounds().Dy()
}

func TestEncode_QRECLevelChangesSize(t *testing.T) {
	data := "The quick brown fox jumps over the lazy dog"
	prev := 0
	for _, l := range []string{"L", "M", "Q", "H"} {
		w, h := size(t, "qr", data, Options{ECLevel: l})
		if w != h {
			t.Errorf("level %s: %dx%d is not square", l, w, h)
		}
		if w < prev {
			t.Errorf("level %s: width %d, want at least %d", l, w, prev)
		}
		prev = w
	}
	wl, _ := size(t, "qr", data, Options{ECLevel: "L"})
	if wl >= prev {
		t.Errorf("level L width %d, want less than level H %d", wl, prev)
	}
	if w, _ := size(t, "qr", data, Options{}); w != 33 {
		t.Errorf("default (M) width %d, want 33 (version 4)", w)
	}
}

func TestEncode_PDF417Security(t *testing.T) {
	data := "PDF417 security level test"
	o := Options{Columns: 4}
	_, h1 := size(t, "pdf417", data, Options{Columns: 4, Security: 1})
	_, h5 := size(t, "pdf417", data, Options{Columns: 4, Security: 5})
	if h5 <= h1 {
		t.Errorf("security 5 height %d, want more than security 1 (%d)", h5, h1)
	}
	// Automatic: level 2 for this data length.
	_, ha := size(t, "pdf417", data, o)
	_, h2 := size(t, "pdf417", data, Options{Columns: 4, Security: 2})
	if ha != h2 {
		t.Errorf("automatic height %d, want level 2's %d", ha, h2)
	}
}

func TestEncode_PDF417RowHeight(t *testing.T) {
	_, h3 := size(t, "pdf417", "rows", Options{})
	_, h6 := size(t, "pdf417", "rows", Options{RowHeight: 6})
	if h6 != 2*h3 {
		t.Errorf("RowHeight 6 height %d, want twice the default %d", h6, h3)
	}
}

func TestEncode_AztecOptions(t *testing.T) {
	w1, _ := size(t, "aztec", "Aztec", Options{})
	w2, _ := size(t, "aztec", "Aztec", Options{ECPercent: 90})
	if w2 <= w1 {
		t.Errorf("ECPercent 90 width %d, want more than default %d", w2, w1)
	}
	if w, _ := size(t, "aztec", "Aztec", Options{Layers: 4, Compact: true}); w != 27 {
		t.Errorf("compact 4 layers: width %d, want 27", w)
	}
	if w, _ := size(t, "aztec", "Aztec", Options{Layers: 5}); w != 37 {
		t.Errorf("full 5 layers: width %d, want 37", w)
	}
}

func TestEncode_DataMatrixSize(t *testing.T) {
	if w, h := size(t, "datamatrix", "123", Options{Columns: 20}); w != 20 || h != 20 {
		t.Errorf("Columns 20: %dx%d", w, h)
	}
	if w, h := size(t, "datamatrix", "123", Options{Rectangular: true}); w == h {
		t.Errorf("Rectangular: %dx%d is square", w, h)
	}
}

func TestEncode_ModuleWidthAndHeight(t *testing.T) {
	for _, sym := range []string{"code128", "qr", "datamatrix", "pdf417", "ean13", "code39"} {
		w1, h1 := size(t, sym, samples[sym], Options{})
		w3, h3 := size(t, sym, samples[sym], Options{ModuleWidth: 3})
		if w3 != 3*w1 || h3 != 3*h1 {
			t.Errorf("%s: ModuleWidth 3 gives %dx%d, want %dx%d", sym, w3, h3, 3*w1, 3*h1)
		}
	}
	if _, h := size(t, "code128", "x", Options{Height: 50, ModuleWidth: 2}); h != 100 {
		t.Errorf("Height 50 at ModuleWidth 2: height %d, want 100", h)
	}
	if _, h := size(t, "postnet", "12345", Options{}); h != 10 {
		t.Errorf("postnet default height %d, want 10", h)
	}
}

func TestEncode_Ratio(t *testing.T) {
	w2, _ := size(t, "code39", "ABC", Options{Ratio: 2})
	w3, _ := size(t, "code39", "ABC", Options{})
	// Code 39: 5 characters × 3 wide elements each.
	if w3-w2 != 15 {
		t.Errorf("ratio 3 width %d, ratio 2 width %d: want 15 more", w3, w2)
	}
	w25, _ := size(t, "code39", "ABC", Options{Ratio: 2.5, ModuleWidth: 2})
	if w25 != 2*w2+15 {
		t.Errorf("ratio 2.5 at ModuleWidth 2: width %d, want %d", w25, 2*w2+15)
	}
	// Wide is floor(module × ratio) as on a Zebra printer: 2 pixels at
	// ModuleWidth 1 for every ratio below 3.
	if w, _ := size(t, "code39", "ABC", Options{Ratio: 2.9}); w != w2 {
		t.Errorf("ratio 2.9 at ModuleWidth 1: width %d, want %d", w, w2)
	}
}

func TestEncode_CheckDigitOption(t *testing.T) {
	for _, sym := range []string{"code39", "itf", "code128"} {
		data := map[string]string{"code39": "ABC", "itf": "1234", "code128": "1234"}[sym]
		w0, _ := size(t, sym, data, Options{})
		w1, _ := size(t, sym, data, Options{CheckDigit: true})
		if w1 <= w0 {
			t.Errorf("%s: CheckDigit width %d, want more than %d", sym, w1, w0)
		}
	}
}

func TestEncode_CodabarStartStop(t *testing.T) {
	a, err := Encode("codabar", "40156", Options{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := Encode("codabar", "A40156A", Options{})
	if err != nil {
		t.Fatal(err)
	}
	c, err := Encode("codabar", "B40156D", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.Pix, b.Pix) {
		t.Error("default start/stop is not A")
	}
	if bytes.Equal(a.Pix, c.Pix) {
		t.Error("start/stop B..D ignored")
	}
}

func TestEncode_CompositeAddsComponent(t *testing.T) {
	for _, sym := range []string{"ean13-cc", "gs1-128-cc", "databar"} {
		data := samples[sym]
		if sym == "databar" {
			data += "|(10)ABC123"
		}
		_, hc := size(t, sym, data, Options{})
		linear := data[:bytes.IndexByte([]byte(data), '|')]
		_, hl := size(t, sym, linear, Options{})
		if hc <= hl {
			t.Errorf("%s: composite height %d, linear only %d", sym, hc, hl)
		}
	}
}

func TestEncode_InvalidOptions(t *testing.T) {
	tests := []struct {
		sym string
		o   Options
	}{
		{"code128", Options{ModuleWidth: -1}},
		{"code128", Options{ModuleWidth: 101}},
		{"code128", Options{Height: -1}},
		{"code39", Options{Ratio: 1.5}},
		{"code39", Options{Ratio: 3.5}},
		{"qr", Options{ECLevel: "X"}},
		{"aztec", Options{ECPercent: 100}},
		{"aztec", Options{Layers: 5, Compact: true}},
		{"pdf417", Options{Security: 9}},
		{"pdf417", Options{Columns: 31}},
		{"pdf417", Options{Rows: 2}},
		{"micropdf417", Options{Columns: 5}},
		{"datamatrix", Options{Columns: 145}},
		{"maxicode", Options{Mode: 7}},
		{"databar-expanded-stacked", Options{Columns: 3}},
		{"pdf417", Options{RowHeight: 101}},
	}
	for _, tc := range tests {
		if _, err := Encode(tc.sym, samples[tc.sym], tc.o); !errors.Is(err, ErrInvalidOption) {
			t.Errorf("%s %+v: err = %v, want ErrInvalidOption", tc.sym, tc.o, err)
		}
	}
}

func TestEncode_TooLarge(t *testing.T) {
	if _, err := Encode("code128", "x", Options{ModuleWidth: 100, Height: 10000}); !errors.Is(err, ErrInvalidOption) {
		t.Errorf("err = %v, want ErrInvalidOption", err)
	}
}

func TestSymbologies_ReturnsCopy(t *testing.T) {
	a := Symbologies()
	a[0].Name = "changed"
	a[0].Aliases[0] = "changed"
	b := Symbologies()
	if b[0].Name == "changed" || b[0].Aliases[0] == "changed" {
		t.Error("Symbologies shares its slices")
	}
}

func BenchmarkEncode_QR(b *testing.B) {
	for b.Loop() {
		Encode("qr", "https://example.com/some/path?id=1234567890", Options{})
	}
}

func BenchmarkEncode_Code128(b *testing.B) {
	for b.Loop() {
		Encode("code128", "ABC-1234567890", Options{Height: 50})
	}
}
