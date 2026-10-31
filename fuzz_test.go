package zpl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// FuzzRender checks that arbitrary input never panics or hangs.
//
//	go test -fuzz FuzzRender -fuzztime 60s
func FuzzRender(f *testing.F) {
	files, _ := filepath.Glob("testdata/golden/*.zpl")
	for _, file := range files {
		if b, err := os.ReadFile(file); err == nil {
			f.Add(string(b))
		}
	}
	f.Add("^XA^FO10,10^GFA,8,8,1,:Z64:eJxjYGAAAAAEAAE=:0000^FS^XZ")
	f.Add("~DGR:X.GRF,4,2,!,:^XA^XGX^FS^XZ")
	f.Add("^XA^PW100000^LL100000^XZ")
	f.Add("^XA^DFR:A.ZPL^FS^XFR:A.ZPL^XZ^XA^XFR:A.ZPL^XZ")
	f.Add("^XA^PQ99999^XZ")
	f.Fuzz(func(t *testing.T, src string) {
		done := make(chan struct{})
		go func() {
			defer close(done)
			// No label size override, so ^PW/^LL from the input apply.
			Render(strings.NewReader(src), Options{ExpandCopies: true, MaxPixels: 64 << 20})
		}()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatalf("render hung on %q", src)
		}
	})
}
