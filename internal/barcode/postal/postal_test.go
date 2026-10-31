package postal

import (
	"errors"
	"strings"
	"testing"

	"github.com/datamagik/zpl-to-image/internal/barcode"
)

func atdf(p barcode.Postal) string {
	var b strings.Builder
	for _, s := range p.Bars {
		b.WriteByte("FADT"[s])
	}
	return b.String()
}

func swapAD(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case 'A':
			return 'D'
		case 'D':
			return 'A'
		}
		return r
	}, s)
}

func TestIMb(t *testing.T) {
	tests := []struct{ in, want string }{
		// USPS-B-3200 appendix examples.
		{"01234567094987654321", "ATTFATTDTTADTAATTDTDTATTDAFDDFADFDFTFFFFFTATFAAAATDFFTDAADFTFDTDT"},
		{"0123456709498765432101234", "DTTAFADDTTFTDTFTFDTDDADADAFADFATDDFTAAAFDTTADFAAATDFDTDFADDDTDFFT"},
		{"01234567094987654321012345678", "ADFTTAFDTTTTFATTADTAAATFTFTATDAAAFDDADATATDTDTTDFDTDATADADTDFFTFA"},
		{"01234567094987654321-01234567891", "AADTFFDFTDADTAADAATFDTDDAAADDTDTTDAFADADDDTFFFDDTTTADFAAADFTDAADA"},
		// ZPL II guide ^BZ type 3 figure (^FD00123123456123456789^FS). The
		// figure bitmap has ascenders and descenders swapped (it is drawn
		// upside down relative to USPS-B-3200, which the vectors above
		// follow); the bar sequence is otherwise identical.
		{"00123123456123456789", swapAD("AAAAFDDTFTTTTDDTTDFTTTTTTADATFDDTAFFFDAFTADDFDDDFDDFADADFFFTFFDFD")},
	}
	for _, tc := range tests {
		p, err := EncodeIMb(tc.in)
		if err != nil {
			t.Errorf("%s: %v", tc.in, err)
			continue
		}
		if got := atdf(p); got != tc.want {
			t.Errorf("%s:\n got %s\nwant %s", tc.in, got, tc.want)
		}
	}
	for _, bad := range []string{"0123456709498765432", "05234567094987654321", "012345670949876543211234", "0123456709498765432A"} {
		if _, err := EncodeIMb(bad); !errors.Is(err, barcode.ErrInvalidData) {
			t.Errorf("%s: err = %v", bad, err)
		}
	}
}

func TestIMbTables(t *testing.T) {
	// First and last entries of USPS-B-3200 Appendix D tables I and II.
	if table5of13[0] != 0x001F || table5of13[1] != 0x1F00 || table5of13[1286] != 0x01F0 {
		t.Errorf("table I: %#x %#x %#x", table5of13[0], table5of13[1], table5of13[1286])
	}
	if table2of13[0] != 0x0003 || table2of13[1] != 0x1800 || table2of13[77] != 0x00A0 {
		t.Errorf("table II: %#x %#x %#x", table2of13[0], table2of13[1], table2of13[77])
	}
}

func TestPOSTNET(t *testing.T) {
	p, err := EncodePOSTNET("12345")
	if err != nil {
		t.Fatal(err)
	}
	// frame, 1=00011 2=00101 3=00110 4=01001 5=01010, check 5=01010, frame.
	want := "F" + "TTTFF" + "TTFTF" + "TTFFT" + "TFTTF" + "TFTFT" + "TFTFT" + "F"
	if got := atdf(p); got != want || p.Text != "12345" {
		t.Errorf("got %s %q", got, p.Text)
	}
	p, _ = EncodePLANET("12345")
	want = "F" + "FFFTT" + "FFTFT" + "FFTTF" + "FTFFT" + "FTFTF" + "FTFTF" + "F"
	if got := atdf(p); got != want {
		t.Errorf("planet %s", got)
	}
	if _, err := EncodePOSTNET("12-34"); !errors.Is(err, barcode.ErrInvalidData) {
		t.Errorf("err = %v", err)
	}
}
