//go:build labelary

package verify

// The Labelary comparison sends the golden test fixtures (and nothing else)
// to the public Labelary API and reports how far our renders differ.
//
//	ZPL_LABELARY=1 go test -tags labelary -run Labelary -v ./...
//
// Labelary is an independent emulator, not a printer, so differences are
// reported rather than treated as failures, except for gross mismatches.

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	zpl "github.com/dchupp/DataMagik-ZPL-To-Image"
)

// notComparable lists fixtures using symbologies Labelary does not render.
var notComparable = map[string]bool{"codablock_tlc39": true}

func TestLabelaryDiff(t *testing.T) {
	if os.Getenv("ZPL_LABELARY") != "1" {
		t.Skip("set ZPL_LABELARY=1 to send the golden fixtures to api.labelary.com")
	}
	files, _ := filepath.Glob("../../testdata/golden/*.zpl")
	out := filepath.Join(os.TempDir(), "zpl-labelary-diff")
	os.MkdirAll(out, 0o755)
	for _, file := range files {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		name := strings.TrimSuffix(filepath.Base(file), ".zpl")
		if notComparable[name] {
			continue
		}
		dpmm := 8
		if _, d, ok := strings.Cut(name, "@"); ok {
			fmt.Sscan(d, &dpmm)
		}
		ours, _, err := zpl.Render(bytes.NewReader(src), zpl.Options{DPMM: dpmm})
		if err != nil || len(ours) == 0 {
			t.Errorf("%s: render: %v", name, err)
			continue
		}
		theirs, err := labelary(src, dpmm)
		if err != nil {
			t.Errorf("%s: labelary: %v", name, err)
			continue
		}
		pct, diff := compare(ours[0], theirs)
		path := filepath.Join(out, name+"-diff.png")
		f, _ := os.Create(path)
		png.Encode(f, diff)
		f.Close()
		t.Logf("%-18s %6.2f%% pixels differ (diff image: %s)", name, pct, path)
		if pct > 10 {
			t.Errorf("%s differs by %.2f%%", name, pct)
		}
		time.Sleep(500 * time.Millisecond) // stay well inside Labelary's rate limit
	}
}

func labelary(src []byte, dpmm int) (image.Image, error) {
	url := fmt.Sprintf("http://api.labelary.com/v1/printers/%ddpmm/labels/4x6/0/", dpmm)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(src))
	req.Header.Set("Accept", "image/png")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %s", resp.Status)
	}
	return png.Decode(resp.Body)
}

// compare returns the percentage of differing pixels and a diff image:
// black where both are black, red only ours, blue only Labelary's.
// Labelary renders the whole 4-inch media and centres a narrower ^PW print
// width on it, so our image is aligned to its centre.
func compare(a, b image.Image) (float64, image.Image) {
	dx := max((b.Bounds().Dx()-a.Bounds().Dx())/2, 0) // wider ^PW is clipped, not centred
	r := a.Bounds().Intersect(b.Bounds().Sub(image.Pt(dx, 0)))
	diff := image.NewRGBA(r)
	n := 0
	dark := func(img image.Image, x, y int) bool { v, _, _, _ := img.At(x, y).RGBA(); return v < 0x8000 }
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			da, db := dark(a, x, y), dark(b, x+dx, y)
			c := color.RGBA{255, 255, 255, 255}
			switch {
			case da && db:
				c = color.RGBA{0, 0, 0, 255}
			case da:
				c, n = color.RGBA{220, 0, 0, 255}, n+1
			case db:
				c, n = color.RGBA{0, 0, 220, 255}, n+1
			}
			diff.Set(x, y, c)
		}
	}
	return 100 * float64(n) / float64(r.Dx()*r.Dy()), diff
}
