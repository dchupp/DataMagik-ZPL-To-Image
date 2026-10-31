package bitmap

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"

	"golang.org/x/image/bmp"
)

// ErrImageTooLarge is returned by DecodePNG and DecodeBMP for an image whose
// header declares more pixels than a label bitmap may hold.
var ErrImageTooLarge = errors.New("image too large")

// DecodePNG decodes a PNG, refusing one whose declared size Fits does not
// allow before any pixel memory is allocated: a few bytes of PNG can declare
// gigapixels.
func DecodePNG(raw []byte) (image.Image, error) {
	return decodeBounded(raw, png.DecodeConfig, png.Decode)
}

// DecodeBMP is DecodePNG for BMP.
func DecodeBMP(raw []byte) (image.Image, error) {
	return decodeBounded(raw, bmp.DecodeConfig, bmp.Decode)
}

func decodeBounded(raw []byte, config func(r io.Reader) (image.Config, error), decode func(r io.Reader) (image.Image, error)) (image.Image, error) {
	cfg, err := config(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	if !Fits(cfg.Width, cfg.Height) {
		return nil, fmt.Errorf("%w: %dx%d", ErrImageTooLarge, cfg.Width, cfg.Height)
	}
	return decode(bytes.NewReader(raw))
}
