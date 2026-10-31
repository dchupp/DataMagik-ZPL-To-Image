package compress

import (
	"bytes"
	"testing"
)

func TestACS(t *testing.T) {
	cases := []struct {
		in       string
		rowBytes int
		want     []byte
	}{
		{"FF00", 2, []byte{0xFF, 0x00}},
		{"JF", 2, []byte{0xFF, 0xFF}},
		{"gF", 10, bytes.Repeat([]byte{0xFF}, 10)},
		{"hGA", 21, append(bytes.Repeat([]byte{0xAA}, 20), 0xA0)},
		{"F,", 2, []byte{0xF0, 0x00}},
		{"!", 2, []byte{0xFF, 0xFF}},
		{"0F0F:", 2, []byte{0x0F, 0x0F, 0x0F, 0x0F}},
		{",:", 1, []byte{0, 0}},
	}
	for _, c := range cases {
		if got := DecodeACS(c.in, c.rowBytes); !bytes.Equal(got, c.want) {
			t.Errorf("%q: got %x want %x", c.in, got, c.want)
		}
	}
}

func TestCRC16(t *testing.T) {
	// CRC-16/XMODEM check value.
	if got := CRC16("123456789"); got != 0x31C3 {
		t.Errorf("CRC16 = %04X", got)
	}
}

func TestB64(t *testing.T) {
	got, err := DecodeGraphic(":B64:AP8=:"+"", 1, 2)
	if err != nil || !bytes.Equal(got, []byte{0x00, 0xFF}) {
		t.Errorf("got %x %v", got, err)
	}
	if _, err := DecodeGraphic(":B64:AP8=:0000", 1, 2); err != ErrCRC {
		t.Errorf("want CRC error, got %v", err)
	}
}
