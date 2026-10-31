package tlc39

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/micropdf"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/pdf417"
)

func elemString(l barcode.Linear) string {
	var b strings.Builder
	for _, e := range l.Elements {
		b.WriteByte(byte('0' + e))
	}
	return b.String()
}

// TestLinear: the Code 39 symbols were decoded by zxing-cpp as "123456"
// and "239316".
func TestLinear(t *testing.T) {
	s, err := Encode("123456", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if s.MicroPDF != nil || s.Linear.Text != "123456" || !s.Linear.Ratio {
		t.Errorf("unexpected symbol %+v", s)
	}
	// * 1 2 3 4 5 6 *: 8 characters of 9 elements + 7 gaps.
	if len(s.Linear.Elements) != 79 {
		t.Errorf("%d elements", len(s.Linear.Elements))
	}
	want := "121121211" + "1" + "211211112" // "*" then "1"
	if got := elemString(s.Linear); !strings.HasPrefix(got, want) {
		t.Errorf("elements %s", got)
	}
	if got := elemString(s.Flag); got != "111121221" { // 'T': 5 bars, 4 spaces
		t.Errorf("flag %s", got)
	}
	for _, bad := range []string{"12345", "12345A", "", "123456,", "123456,AB-C", "123456,A," + strings.Repeat("B", 26)} {
		if _, err := Encode(bad, Options{}); !errors.Is(err, barcode.ErrInvalidData) {
			t.Errorf("%q accepted", bad)
		}
	}
}

// TestMicroPDF: the 2D part is a 4-column MicroPDF417 whose first codeword
// is 918 (TCIF linkage), followed by an explicit Text latch. zxing-cpp
// decoded the same symbol without the 918 prefix (variant 4x10) as
// "ABCd12345678901234,5551212,88899"; it rejects the 918 linkage codeword.
func TestMicroPDF(t *testing.T) {
	s, err := Encode("123456,ABCd12345678901234,5551212,88899", Options{})
	if err != nil {
		t.Fatal(err)
	}
	m := s.MicroPDF
	// 32 characters with separators reserve 2 + 25 + 2 codewords: 4x12.
	if m == nil || m.Width() != 99 || m.Height() != 12 {
		t.Fatalf("MicroPDF %v", m)
	}
	var row strings.Builder
	for _, d := range m.Rows[0] {
		if d {
			row.WriteByte('1')
		} else {
			row.WriteByte('0')
		}
	}
	first, _ := strconv.ParseUint(row.String()[10:27], 2, 32)
	v, _ := micropdf.Smallest(4, 29)
	if uint32(first) != pdf417.CodewordPattern(v.Cluster, pdf417.CWLinkageOther) {
		t.Errorf("first codeword pattern %017b", first)
	}
	h := sha256.New()
	for _, r := range m.Rows {
		fmt.Fprint(h, r)
	}
	if got := fmt.Sprintf("%x", h.Sum(nil))[:16]; got != tlcHash {
		t.Errorf("matrix hash %s", got)
	}
	s2, _ := Encode("123456,ABC,X", Options{FieldSeparator: 0x1D})
	if s2.MicroPDF == nil || s2.MicroPDF.Height() != 6 { // 4x6 is the smallest a ZD420 prints
		t.Errorf("small symbol: %v", s2.MicroPDF)
	}
}

// tlcHash pins the MicroPDF417 module rows of TestMicroPDF. The commas are
// shifted to Punctuation, as a ZD420 encodes them (printer fixture q26).
const tlcHash = "f0ae6c806c2465a8"

func TestRejectsTrailingDataWithoutComma(t *testing.T) {
	for _, d := range []string{"1234567;SERIAL", "1234567", "123456X"} {
		if _, err := Encode(d, Options{}); !errors.Is(err, barcode.ErrInvalidData) {
			t.Errorf("Encode(%q) = %v, want ErrInvalidData", d, err)
		}
	}
	if _, err := Encode("123456", Options{}); err != nil {
		t.Errorf("ECI only: %v", err)
	}
}
