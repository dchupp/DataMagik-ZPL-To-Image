package zpl

import (
	"bytes"
	"os"
	"testing"
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

func BenchmarkRenderOnly8dpmm(b *testing.B) {
	src, _ := os.ReadFile("testdata/samples/shipping_label.zpl")
	b.ReportAllocs()
	for range b.N {
		if _, _, err := Render(bytes.NewReader(src), Options{}); err != nil {
			b.Fatal(err)
		}
	}
}
