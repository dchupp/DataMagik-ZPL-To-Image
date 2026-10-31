package zpl

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"fmt"
	"image"
	"os"
	"strings"
	"testing"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/compress"
)

func benchmarkLabel(b *testing.B, file string, o Options) {
	src, err := os.ReadFile(file)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for range b.N {
		if _, _, err := RenderPNG(bytes.NewReader(src), o); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkShippingLabel8dpmm(b *testing.B) {
	benchmarkLabel(b, "testdata/samples/shipping_label.zpl", Options{})
}

func BenchmarkShippingLabel24dpmm(b *testing.B) {
	benchmarkLabel(b, "testdata/samples/shipping_label.zpl", Options{DPMM: 24})
}

// BenchmarkParallelShippingLabel renders with one Printer per call on all
// GOMAXPROCS goroutines, as a server would (run with -cpu 1,8,24).
func BenchmarkParallelShippingLabel(b *testing.B) {
	src, err := os.ReadFile("testdata/samples/shipping_label.zpl")
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, _, err := RenderPNG(bytes.NewReader(src), Options{}); err != nil {
				b.Error(err)
				return
			}
		}
	})
}

func BenchmarkRenderOnly8dpmm(b *testing.B) {
	src, _ := os.ReadFile("testdata/samples/shipping_label.zpl")
	b.ReportAllocs()
	for range b.N {
		if _, _, err := Render(bytes.NewReader(src), Options{}); err != nil {
			b.Fatal(err)
		}
	}
}

// benchmarkGolden renders a golden fixture to PNG at its own density.
func benchmarkGolden(b *testing.B, name string) {
	src, err := os.ReadFile("testdata/golden/" + name + ".zpl")
	if err != nil {
		b.Fatal(err)
	}
	o := Options{DPMM: goldenDPMM(name)}
	b.ReportAllocs()
	for range b.N {
		if _, _, err := RenderPNG(bytes.NewReader(src), o); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGoldenText(b *testing.B)      { benchmarkGolden(b, "text") }
func BenchmarkGoldenLinear(b *testing.B)    { benchmarkGolden(b, "linear") }
func BenchmarkGolden2D(b *testing.B)        { benchmarkGolden(b, "2d") }
func BenchmarkGoldenStacked(b *testing.B)   { benchmarkGolden(b, "stacked") }
func BenchmarkGoldenDensity24(b *testing.B) { benchmarkGolden(b, "density@24") }

// BenchmarkLargeGraphic renders a full-label ^GF logo (Z64-compressed).
func BenchmarkLargeGraphic(b *testing.B) {
	const w, h = 100, 1200 // 800×1200 dots
	raw := make([]byte, w*h)
	for i := range raw {
		raw[i] = byte(i*7) ^ byte(i>>9)
	}
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	zw.Write(raw)
	zw.Close()
	b64 := base64.StdEncoding.EncodeToString(z.Bytes())
	src := fmt.Sprintf("^XA^FO0,0^GFA,%d,%d,%d,:Z64:%s:%04X^FS^XZ", len(raw), len(raw), w, b64, compress.CRC16(b64))
	b.ReportAllocs()
	for range b.N {
		if _, _, err := RenderPNG(strings.NewReader(src), Options{}); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSerialCopies streams 100 serialised copies through one Printer.
func BenchmarkSerialCopies(b *testing.B) {
	src := "^XA^FO50,50^A0N,60^FDLot^FS^FO50,130^BY3^BCN,120,Y,N,N^SN000001,1,Y^FS^FO500,50^BQN,2,5^FDQA,serial^FS^PQ100^XZ"
	b.ReportAllocs()
	for range b.N {
		p, _ := NewPrinter(Options{ExpandCopies: true})
		if _, err := p.RenderEach(strings.NewReader(src), func(img image.Image) error {
			_, err := EncodePNG(img)
			return err
		}); err != nil {
			b.Fatal(err)
		}
	}
}
