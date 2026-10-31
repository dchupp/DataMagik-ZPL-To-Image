package font

import (
	"reflect"
	"testing"
)

func TestBidiLevels(t *testing.T) {
	// UAX #9: digits after Arabic resolve to level 2, trailing Latin to 0.
	text := []rune("ab الطلب 45 cd")
	want := []int{0, 0, 0, 1, 1, 1, 1, 1, 1, 2, 2, 0, 0, 0}
	if got := bidiLevels(text); !reflect.DeepEqual(got, want) {
		t.Errorf("levels %v\nwant   %v", got, want)
	}
}

func TestVisualOrder(t *testing.T) {
	builtins()
	face := font0.Face(40, 40).(*scalableFace)
	text := []rune("ab אבג 45 cd")
	var got []rune
	for _, seg := range visualSegments([]*scalableFace{face}, text, true) {
		part := text[seg.start:seg.end]
		if seg.rtl {
			for i := len(part) - 1; i >= 0; i-- {
				got = append(got, part[i])
			}
		} else {
			got = append(got, part...)
		}
	}
	if want := "ab 45 גבא cd"; string(got) != want {
		t.Errorf("visual order %q, want %q", string(got), want)
	}
}
