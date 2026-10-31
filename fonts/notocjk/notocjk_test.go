package notocjk_test

import (
	"strings"
	"testing"

	zpl "github.com/datamagik/zpl-to-image"
	"github.com/datamagik/zpl-to-image/fonts/notocjk"
)

func ink(t *testing.T, o zpl.Options) int {
	t.Helper()
	imgs, _, err := zpl.Render(strings.NewReader("^XA^CI28^FO20,20^A0N,60^FD日本語 中文 한국어^FS^XZ"), o)
	if err != nil || len(imgs) != 1 {
		t.Fatalf("render: %v", err)
	}
	n := 0
	b := imgs[0].Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if r, _, _, _ := imgs[0].At(x, y).RGBA(); r < 0x8000 {
				n++
			}
		}
	}
	return n
}

func TestCJKFallback(t *testing.T) {
	without := ink(t, zpl.Options{LabelWidth: 4, LabelHeight: 1})
	with := ink(t, zpl.Options{LabelWidth: 4, LabelHeight: 1, FallbackFonts: [][]byte{notocjk.Regular}})
	if without != 0 {
		t.Errorf("CJK printed %d dots without a CJK font", without)
	}
	if with < 2000 {
		t.Errorf("CJK fallback printed only %d dots", with)
	}
}
