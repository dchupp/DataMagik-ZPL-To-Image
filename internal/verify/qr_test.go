package verify

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/qr"
	"github.com/makiuchi-d/gozxing"
)

func qrDecode(t *testing.T, m barcode.Matrix) string {
	t.Helper()
	got, err := Decode(MatrixImage(m, 3, 0, 4), gozxing.BarcodeFormat_QR_CODE)
	if err != nil {
		t.Fatalf("decode %dx%d: %v", m.Width(), m.Height(), err)
	}
	return got
}

func TestQRRoundTrip(t *testing.T) {
	cases := []string{
		"0", "01234567", "HELLO WORLD", "Hello, World!", "https://www.zebra.com/?a=1&b=2",
		"ABC123456789012345xyz", "1234567890123456789012345678901234567890",
		"MIXED 123 lower case + UPPER 0987654321 $%*+-./:",
		strings.Repeat("The quick brown fox jumps over the lazy dog. ", 20),
	}
	for _, s := range cases {
		for l := qr.ECLevelL; l <= qr.ECLevelH; l++ {
			m, err := qr.Encode([]qr.Segment{{Mode: qr.ModeAuto, Data: []byte(s)}}, qr.Options{ECLevel: l})
			if err != nil {
				t.Fatalf("%q %s: %v", s, l, err)
			}
			if got := qrDecode(t, m); got != s {
				t.Errorf("%s: got %q want %q", l, got, s)
			}
		}
	}
}

// Fill every version/level exactly with alphanumeric data so each of the
// 160 symbol sizes is exercised.
func TestQRAllVersions(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	const alnum = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ $%*+-./:"
	for v := 1; v <= 40; v++ {
		for l := qr.ECLevelL; l <= qr.ECLevelH; l++ {
			// Grow the payload until it no longer fits in version v.
			n := 1
			var last barcode.Matrix
			var lastData string
			for {
				var b strings.Builder
				for range n {
					b.WriteByte(alnum[rng.Intn(len(alnum))])
				}
				m, err := qr.Encode([]qr.Segment{{Mode: qr.ModeAlphanumeric, Data: []byte(b.String())}}, qr.Options{ECLevel: l, Version: v})
				if err != nil {
					break
				}
				last, lastData = m, b.String()
				n += max(1, n/8)
			}
			if lastData == "" {
				t.Fatalf("v%d-%s: nothing fits", v, l)
			}
			if last.Width() != 17+4*v {
				t.Fatalf("v%d: width %d", v, last.Width())
			}
			if got := qrDecode(t, last); got != lastData {
				t.Errorf("v%d-%s: mismatch", v, l)
			}
		}
	}
}

func TestQRMasksAndModes(t *testing.T) {
	segs := []qr.Segment{
		{Mode: qr.ModeNumeric, Data: []byte("0123456789")},
		{Mode: qr.ModeAlphanumeric, Data: []byte("AC-42")},
		{Mode: qr.ModeByte, Data: []byte("bytes,\x01ok")},
		{Mode: qr.ModeKanji, Data: []byte("\x93\xfa\x96\x7b")}, // 日本
	}
	for mask := 0; mask < 8; mask++ {
		m, err := qr.Encode(segs, qr.Options{ECLevel: qr.ECLevelQ, ForceMask: true, Mask: mask})
		if err != nil {
			t.Fatal(err)
		}
		if got := qrDecode(t, m); got != "0123456789AC-42bytes,\x01ok日本" {
			t.Errorf("mask %d: %q", mask, got)
		}
	}
}

func TestQRZPLFieldData(t *testing.T) {
	cases := []struct{ fd, want string }{
		{"QA,0123456789ABCD 2D code", "0123456789ABCD 2D code"},
		{"LA,a,b,c", "a,b,c"},
		{"HM,N123456789012345", "123456789012345"},
		{"MM,AAC-42", "AC-42"},
		{"HM,B0011hello,world", "hello,world"},
		{"QM,N0123,AABC,B0003xyz,K\x93\xfa\x96\x7b", "0123ABCxyz日本"},
		{"QA,\x93\xfa\x96\x7b\x8c\xea", "日本語"}, // automatic Kanji
		{"D03048F,LM,N0123456789,A12AABB,B0006qrcode", "012345678912AABBqrcode"},
		{"D03040C,LA,012345678912AABBqrcode", "012345678912AABBqrcode"},
	}
	for _, c := range cases {
		m, err := qr.EncodeZPL([]byte(c.fd), qr.Options{Model: 2})
		if err != nil {
			t.Fatalf("%q: %v", c.fd, err)
		}
		if got := qrDecode(t, m); got != c.want {
			t.Errorf("%q: got %q want %q", c.fd, got, c.want)
		}
	}
}

func TestQRStructuredAppendECIAndLong(t *testing.T) {
	msg := "PART ONE|part two|PART 3"
	parts := strings.Split(msg, "|")
	for i, p := range parts {
		m, err := qr.Encode([]qr.Segment{{Mode: qr.ModeAuto, Data: []byte(p)}}, qr.Options{
			ECLevel:          qr.ECLevelM,
			StructuredAppend: &qr.StructuredAppend{Position: i + 1, Total: len(parts), Parity: qr.Parity([]byte(strings.Join(parts, "")))},
		})
		if err != nil {
			t.Fatal(err)
		}
		if got := qrDecode(t, m); got != p {
			t.Errorf("part %d: %q", i+1, got)
		}
	}
	m, err := qr.Encode([]qr.Segment{{Mode: qr.ModeByte, Data: []byte("Grüße – ÄÖÜ €")}}, qr.Options{ECI: 26, DisableAutoKanji: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := qrDecode(t, m); got != "Grüße – ÄÖÜ €" {
		t.Errorf("ECI 26: %q", got)
	}
	// Large binary payload near the 40-L byte capacity (2953).
	data := make([]byte, 2900)
	rng := rand.New(rand.NewSource(2))
	for i := range data {
		data[i] = byte('a' + rng.Intn(26))
	}
	m, err = qr.Encode([]qr.Segment{{Mode: qr.ModeByte, Data: data}}, qr.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := qrDecode(t, m); got != string(data) {
		t.Errorf("2900 bytes: mismatch (len %d)", len(got))
	}
}
