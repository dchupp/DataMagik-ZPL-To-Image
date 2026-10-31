package upcean

import (
	"errors"
	"reflect"
	"testing"

	"github.com/datamagik/zpl-to-image/internal/barcode"
)

// Element widths measured from the ZPL II guide figures (^BY3).
var (
	figEAN8  = []int{1, 1, 1, 2, 2, 2, 1, 2, 1, 2, 2, 1, 4, 1, 1, 1, 1, 3, 2, 1, 1, 1, 1, 1, 1, 2, 3, 1, 1, 1, 1, 4, 1, 3, 1, 2, 3, 2, 1, 1, 1, 1, 1}
	figEAN13 = []int{1, 1, 1, 3, 2, 1, 1, 3, 2, 1, 1, 3, 2, 1, 1, 2, 2, 2, 1, 2, 1, 2, 2, 1, 4, 1, 1, 1, 1, 1, 1, 1, 1, 1, 3, 2, 1, 2, 3, 1, 1, 1, 1, 4, 1, 3, 1, 2, 1, 2, 1, 3, 1, 1, 3, 2, 1, 1, 1}
	figUPCE  = []int{1, 1, 1, 1, 2, 2, 2, 2, 2, 1, 2, 1, 4, 1, 1, 2, 3, 1, 1, 1, 2, 3, 1, 1, 4, 1, 1, 1, 1, 1, 1, 1, 1}
	figEAN2  = []int{1, 1, 2, 2, 2, 2, 1, 1, 1, 2, 1, 2, 2}
)

func TestZebraFigures(t *testing.T) {
	l, err := EncodeEAN8("1234567") // ^B8N,100,Y,N^FD1234567^FS
	if err != nil || !reflect.DeepEqual(l.Elements, figEAN8) || l.Text != "12345670" {
		t.Errorf("EAN-8: %v %q %v", l.Elements, l.Text, err)
	}
	l, err = EncodeEAN13("12345678") // ^BEN,100,Y,N^FD12345678^FS
	if err != nil || !reflect.DeepEqual(l.Elements, figEAN13) || l.Text != "0000123456784" {
		t.Errorf("EAN-13: %v %q %v", l.Elements, l.Text, err)
	}
	l, err = EncodeUPCE("1230000045", UPCOptions{PrintCheckDigit: true}) // ^B9N,100,Y,N,Y
	if err != nil || !reflect.DeepEqual(l.Elements, figUPCE) || l.Text != "01234531" {
		t.Errorf("UPC-E: %v %q %v", l.Elements, l.Text, err)
	}
	l, err = EncodeAddOn("12") // ^BSN,100,Y,N^FD12^FS
	if err != nil || !reflect.DeepEqual(l.Elements, figEAN2) || l.Text != "12" {
		t.Errorf("EAN-2: %v %q %v", l.Elements, l.Text, err)
	}
}

func TestUPCA(t *testing.T) {
	// 03600029145 -> check 2 (well-known UPC-A example 036000291452).
	l, err := EncodeUPCA("03600029145", UPCOptions{PrintCheckDigit: true})
	if err != nil {
		t.Fatal(err)
	}
	if l.Text != "036000291452" || l.Width(0) != 95 || len(l.Elements) != 59 {
		t.Errorf("%q width %d", l.Text, l.Width(0))
	}
	wantGroups := []string{"0", "36000", "29145", "2"}
	for i, g := range l.TextGroups {
		if g.Text != wantGroups[i] {
			t.Errorf("group %d = %q", i, g.Text)
		}
	}
	// Guards: start/centre/end bars plus the bars of the first and last digit.
	n := 0
	for i, g := range l.Guards {
		if g {
			if i%2 == 1 {
				t.Errorf("space %d marked as guard", i)
			}
			n++
		}
	}
	if n != 2+2+2+2+2 {
		t.Errorf("%d guard bars", n)
	}
	l, _ = EncodeUPCA("03600029145", UPCOptions{})
	if l.Text != "03600029145" || len(l.TextGroups) != 3 {
		t.Errorf("no check: %q", l.Text)
	}
	// Twelve digits: the supplied check digit is replaced.
	l, _ = EncodeUPCA("036000291459", UPCOptions{PrintCheckDigit: true})
	if l.Text != "036000291452" {
		t.Errorf("12 digits: %q", l.Text)
	}
}

func TestEAN13(t *testing.T) {
	l, err := EncodeEAN13("400638133393")
	if err != nil || l.Text != "4006381333931" || l.Width(0) != 95 {
		t.Errorf("%q %v", l.Text, err)
	}
	if l.TextGroups[0].Text != "4" || l.TextGroups[0].End >= 0 {
		t.Errorf("leading digit group %+v", l.TextGroups[0])
	}
	if _, err := EncodeEAN13("12345X"); !errors.Is(err, barcode.ErrInvalidData) {
		t.Errorf("err = %v", err)
	}
}

func TestUPCESuppression(t *testing.T) {
	tests := []struct{ in, want string }{
		{"1200000789", "01278907"},   // manufacturer ends 000 -> M1 M2 P3 P4 P5 M3
		{"1230000045", "01234531"},   // ends 300
		{"1234000006", "01234640"},   // ends 0 -> ... 4
		{"1234500007", "01234572"},   // no trailing zero, product 5..9
		{"112345000058", "11234555"}, // number system 1, supplied check replaced
	}
	for _, tc := range tests {
		l, err := EncodeUPCE(tc.in, UPCOptions{PrintCheckDigit: true})
		if err != nil || l.Text != tc.want || l.Width(0) != 51 {
			t.Errorf("%s: %q %v", tc.in, l.Text, err)
		}
	}
	if _, err := EncodeUPCE("1234512345", UPCOptions{}); !errors.Is(err, barcode.ErrInvalidData) {
		t.Errorf("err = %v", err)
	}
}

func TestEAN5(t *testing.T) {
	// "52495" (price $24.95): 3*(5+4+5)+9*(2+9) = 141 -> parity 1 = GLGLL.
	l, err := EncodeAddOn("52495")
	if err != nil || l.Width(0) != 47 || l.Text != "52495" {
		t.Fatalf("%v %v", l, err)
	}
	if !reflect.DeepEqual(l.Elements[3:7], []int{1, 3, 2, 1}) { // G5 = reversed L5 (1231)
		t.Errorf("first char %v", l.Elements[3:7])
	}
}
