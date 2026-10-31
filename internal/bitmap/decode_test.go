package bitmap

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/png"
	"testing"
)

// hugePNG is a valid 1×1 PNG whose header then claims w×h.
func hugePNG(t *testing.T, w, h uint32) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	// Signature (8), IHDR length (4), "IHDR" (4), width, height ...
	binary.BigEndian.PutUint32(b[16:], w)
	binary.BigEndian.PutUint32(b[20:], h)
	binary.BigEndian.PutUint32(b[29:], crc32.ChecksumIEEE(b[12:29]))
	return b
}

func TestDecodePNG_RefusesHugeDeclaredSize(t *testing.T) {
	_, err := DecodePNG(hugePNG(t, 100000, 100000))
	if !errors.Is(err, ErrImageTooLarge) {
		t.Fatalf("err = %v, want ErrImageTooLarge before decoding", err)
	}
}

func TestDecodePNG_DecodesAnOrdinaryImage(t *testing.T) {
	var buf bytes.Buffer
	_ = png.Encode(&buf, image.NewGray(image.Rect(0, 0, 8, 4)))
	img, err := DecodePNG(buf.Bytes())
	if err != nil || img.Bounds().Dx() != 8 || img.Bounds().Dy() != 4 {
		t.Fatalf("got %v %v", img, err)
	}
}
