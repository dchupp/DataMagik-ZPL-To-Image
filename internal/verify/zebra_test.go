//go:build zebra

package verify

// The printer comparison renders fixtures on a real Zebra printer and
// compares them with this library, without printing anything:
//
//	ZPL_PRINTER=10.0.20.200:9100 go test -tags zebra -run Zebra -v ./internal/verify
//
// ZPL_PRINTER_ONLY=q07 limits the run to fixtures whose name contains q07.
//
// Each fixture is sent with ^IS (store the label image in printer RAM, do
// not print), the image is read back with ^HG, then deleted with ^ID. At the
// end the printer's saved configuration is reloaded with ^JUR, undoing any
// runtime settings (^PW, ^LH, ...) the fixtures changed. Fixtures come from
// testdata/printer and testdata/golden (single-format files only).

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	zpl "github.com/dchupp/DataMagik-ZPL-To-Image"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/compress"
)

// zebraSend sends ZPL and returns whatever the printer answers within wait.
func zebraSend(addr string, zplData []byte, wait time.Duration) ([]byte, error) {
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if _, err := conn.Write(zplData); err != nil {
		return nil, err
	}
	var out []byte
	buf := make([]byte, 64<<10)
	deadline := time.Now().Add(wait)
	for {
		conn.SetReadDeadline(deadline)
		n, err := conn.Read(buf)
		out = append(out, buf[:n]...)
		if err != nil {
			return out, nil // timeout or close ends the reply
		}
		deadline = time.Now().Add(1500 * time.Millisecond) // keep reading while data flows
	}
}

// zebraReset restores the printer's saved configuration and the power-up
// values of the field defaults that persist across labels.
const zebraReset = "^XA^JUR^XZ^XA^BY2,3.0,10^CFA,9,5^FWN,0^XZ"

var dgHeader = regexp.MustCompile(`~DG[^,]*,(\d+),(\d+),`)

// zebraRender renders src on the printer and returns its stored image.
func zebraRender(addr string, src []byte) (image.Image, error) {
	i := bytes.LastIndex(src, []byte("^XZ"))
	if i < 0 {
		return nil, fmt.Errorf("no ^XZ")
	}
	job := append(append(append([]byte{}, src[:i]...), "^ISR:ZPLCMP.GRF,N"...), src[i:]...)
	if _, err := zebraSend(addr, job, 500*time.Millisecond); err != nil {
		return nil, err
	}
	reply, err := zebraSend(addr, []byte("^XA^HGR:ZPLCMP.GRF^XZ"), 10*time.Second)
	zebraSend(addr, []byte("^XA^IDR:ZPLCMP.GRF^FS^XZ"), 200*time.Millisecond)
	if err != nil {
		return nil, err
	}
	m := dgHeader.FindSubmatchIndex(reply)
	if m == nil {
		return nil, fmt.Errorf("no ~DG in reply (%d bytes)", len(reply))
	}
	total, _ := strconv.Atoi(string(reply[m[2]:m[3]]))
	row, _ := strconv.Atoi(string(reply[m[4]:m[5]]))
	data := strings.NewReplacer("\r", "", "\n", "", "\x02", "", "\x03", "").Replace(string(reply[m[1]:]))
	raw, err := compress.DecodeGraphic(data, row, total)
	if err != nil {
		return nil, err
	}
	h := total / row
	img := image.NewGray(image.Rect(0, 0, row*8, h))
	for y := range h {
		for x := range row * 8 {
			c := uint8(0xff)
			if raw[y*row+x/8]&(0x80>>(x%8)) != 0 {
				c = 0
			}
			img.Pix[y*img.Stride+x] = c
		}
	}
	return img, nil
}

// zebraDPMM reads the printer's density from its ~HI reply.
func zebraDPMM(t *testing.T, addr string) (string, int) {
	reply, err := zebraSend(addr, []byte("~HI"), 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	id := strings.Trim(string(reply), "\x02\x03\r\n")
	dpi := regexp.MustCompile(`(\d+)\s*dpi`).FindStringSubmatch(id)
	if dpi == nil {
		t.Fatalf("cannot read density from ~HI reply %q", id)
	}
	d, _ := strconv.Atoi(dpi[1])
	switch {
	case d >= 550:
		return id, 24
	case d >= 280:
		return id, 12
	case d >= 190:
		return id, 8
	}
	return id, 6
}

func dark(img image.Image, x, y int) bool {
	if !(image.Point{x, y}.In(img.Bounds())) {
		return false
	}
	v, _, _, _ := img.At(x, y).RGBA()
	return v < 0x8000
}

func inkBox(img image.Image) image.Rectangle {
	var box image.Rectangle
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if dark(img, x, y) {
				box = box.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return box
}

// align finds the offset of ours relative to theirs that best matches the
// ink, starting from the ink bounding boxes and refining by a few dots.
func align(ours, theirs image.Image) image.Point {
	a, b := inkBox(ours), inkBox(theirs)
	base := b.Min.Sub(a.Min)
	// Candidates: around the ink-box alignment, and around the shared
	// origin (printers shift the stored image by a fixed margin, e.g. 8
	// dots on a ZD420), so that ink present in only one image (such as an
	// interpretation digit drawn further left) cannot skew the alignment.
	var cands []image.Point
	for dy := -4; dy <= 4; dy++ {
		for dx := -4; dx <= 4; dx++ {
			cands = append(cands, base.Add(image.Pt(dx, dy)))
		}
	}
	for dy := -6; dy <= 6; dy++ {
		for dx := -4; dx <= 16; dx++ {
			cands = append(cands, image.Pt(dx, dy))
		}
	}
	best, bestDiff := base, -1
	for _, off := range cands {
		{
			n := 0
			for y := a.Min.Y; y < a.Max.Y; y += 2 {
				for x := a.Min.X; x < a.Max.X; x += 2 {
					if dark(ours, x, y) != dark(theirs, x+off.X, y+off.Y) {
						n++
					}
				}
			}
			if bestDiff < 0 || n < bestDiff {
				best, bestDiff = off, n
			}
		}
	}
	return best
}

func TestZebraComparison(t *testing.T) {
	addr := os.Getenv("ZPL_PRINTER")
	if addr == "" {
		t.Skip("set ZPL_PRINTER=host:9100 to compare against a real Zebra printer")
	}
	if !strings.Contains(addr, ":") {
		addr += ":9100"
	}
	id, dpmm := zebraDPMM(t, addr)
	t.Logf("printer: %s (%d dots/mm)", id, dpmm)
	defer zebraSend(addr, []byte(zebraReset), 500*time.Millisecond)

	out := filepath.Join(os.TempDir(), "zpl-zebra-diff")
	os.MkdirAll(out, 0o755)
	files, _ := filepath.Glob("../../testdata/printer/*.zpl")
	if os.Getenv("ZPL_PRINTER_GOLDEN") != "" {
		golden, _ := filepath.Glob("../../testdata/golden/*.zpl")
		files = append(files, golden...)
	}
	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".zpl")
		if only := os.Getenv("ZPL_PRINTER_ONLY"); only != "" && !strings.Contains(name, only) {
			continue
		}
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Count(src, []byte("^XA")) != 1 {
			continue // multi-format fixtures do not map to one stored image
		}
		// ^JUR reloads saved settings, but ^BY, ^CF and ^FW persist even
		// through it, so reset those to their power-up values too: every
		// fixture then starts from the state of a fresh Printer.
		zebraSend(addr, []byte(zebraReset), 300*time.Millisecond)
		theirs, err := zebraRender(addr, src)
		if err != nil {
			t.Errorf("%s: printer: %v", name, err)
			continue
		}
		imgs, _, err := zpl.Render(bytes.NewReader(src), zpl.Options{DPMM: dpmm})
		if err != nil || len(imgs) != 1 {
			t.Errorf("%s: render: %v", name, err)
			continue
		}
		ours := imgs[0]
		off := align(ours, theirs)
		pct, diff, side := zebraCompare(ours, theirs, off)
		save(t, filepath.Join(out, name+"-diff.png"), diff)
		save(t, filepath.Join(out, name+"-side.png"), side)
		save(t, filepath.Join(out, name+"-printer.png"), theirs)
		t.Logf("%-28s %6.2f%% of ink area differs (offset %v)", name, pct, off)
	}
	t.Logf("images: %s", out)
}

// zebraCompare reports the share of differing pixels within the union of
// both ink boxes, a diff image (black both, red only ours, blue only the
// printer's) and a side-by-side image (ours left, printer right).
func zebraCompare(ours, theirs image.Image, off image.Point) (float64, image.Image, image.Image) {
	area := inkBox(ours).Union(inkBox(theirs).Sub(off))
	diff := image.NewRGBA(area)
	n := 0
	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
			a, b := dark(ours, x, y), dark(theirs, x+off.X, y+off.Y)
			c := color.RGBA{255, 255, 255, 255}
			switch {
			case a && b:
				c = color.RGBA{0, 0, 0, 255}
			case a:
				c, n = color.RGBA{220, 0, 0, 255}, n+1
			case b:
				c, n = color.RGBA{0, 0, 220, 255}, n+1
			}
			diff.Set(x, y, c)
		}
	}
	side := image.NewGray(image.Rect(0, 0, 2*area.Dx()+20, area.Dy()))
	for i := range side.Pix {
		side.Pix[i] = 0xff
	}
	for y := 0; y < area.Dy(); y++ {
		for x := 0; x < area.Dx(); x++ {
			if dark(ours, area.Min.X+x, area.Min.Y+y) {
				side.SetGray(x, y, color.Gray{})
			}
			if dark(theirs, area.Min.X+x+off.X, area.Min.Y+y+off.Y) {
				side.SetGray(area.Dx()+20+x, y, color.Gray{})
			}
		}
	}
	return 100 * float64(n) / float64(max(area.Dx()*area.Dy(), 1)), diff, side
}

func save(t *testing.T, path string, img image.Image) {
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	png.Encode(f, img)
}
