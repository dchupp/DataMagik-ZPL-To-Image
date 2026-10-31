// Package qr encodes QR Code symbols (ISO/IEC 18004) for the ZPL ^BQ command:
// Model 2 versions 1-40 and the legacy Model 1 versions 1-14, error
// correction levels L/M/Q/H, numeric/alphanumeric/byte/Kanji modes with
// optimal automatic segmentation, structured append, ECI and FNC1 (GS1).
//
// ParseZPL interprets Zebra's ^BQ field data switches ("QA,data",
// "MM,N123,AABC", "D03048F,LM,..."); Encode builds the module matrix.
package qr

import (
	"fmt"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
)

// ECLevel is a QR error correction level.
type ECLevel int

// Error correction levels, in increasing strength.
const (
	ECLevelL ECLevel = iota // ~7% recovery ("high density")
	ECLevelM                // ~15% recovery ("standard")
	ECLevelQ                // ~25% recovery ("high reliability")
	ECLevelH                // ~30% recovery ("ultra-high reliability")
)

// String returns the level letter.
func (l ECLevel) String() string {
	if l >= ECLevelL && l <= ECLevelH {
		return string("LMQH"[l])
	}
	return fmt.Sprintf("ECLevel(%d)", int(l))
}

// Mode is a QR data encoding mode for a segment.
type Mode int

// Segment modes. ModeAuto lets the encoder split the data into the shortest
// sequence of numeric/alphanumeric/byte/Kanji segments.
const (
	ModeAuto Mode = iota
	ModeNumeric
	ModeAlphanumeric
	ModeByte
	ModeKanji
)

// Segment is a run of data in one mode. For ModeKanji, Data holds Shift-JIS
// double-byte characters (2 bytes each).
type Segment struct {
	Mode Mode
	Data []byte
}

// StructuredAppend identifies one symbol of a structured append series.
type StructuredAppend struct {
	Position int  // 1-based position of this symbol (1..Total)
	Total    int  // number of symbols in the series (2..16)
	Parity   byte // XOR of all bytes of the complete original message
}

// Parity computes the structured append parity byte of a full message.
func Parity(message []byte) byte {
	var p byte
	for _, b := range message {
		p ^= b
	}
	return p
}

// Options controls Encode. The zero value encodes a Model 2, level L symbol
// of the smallest version with automatic mask selection.
type Options struct {
	// Model is 1 (original, versions 1-14) or 2 (versions 1-40). 0 means 2.
	Model int
	// ECLevel is the error correction level.
	ECLevel ECLevel
	// Version forces a symbol version; 0 selects the smallest that fits.
	Version int
	// ForceMask uses Mask (0-7) instead of choosing the mask pattern with
	// the lowest ISO penalty score.
	ForceMask bool
	Mask      int
	// StructuredAppend, when non-nil, prefixes the structured append header.
	StructuredAppend *StructuredAppend
	// DisableAutoKanji stops ModeAuto from encoding Shift-JIS double-byte
	// pairs in Kanji mode (set it when the field data is UTF-8, e.g. ^CI28,
	// where such byte pairs are not Shift-JIS).
	DisableAutoKanji bool
	// ECI, when > 0, emits an ECI designator first (Model 2 only).
	ECI int
	// GS1 emits the FNC1-in-first-position mode indicator.
	GS1 bool
}

// Encode builds a QR symbol from data segments. The result has no quiet zone.
func Encode(segs []Segment, opts Options) (barcode.Matrix, error) {
	model, maxVer, err := validate(segs, opts)
	if err != nil {
		return barcode.Matrix{}, err
	}
	v, bits, err := fit(segs, opts, model, maxVer)
	if err != nil {
		return barcode.Matrix{}, err
	}
	capacity := dataCodewords(model, v, opts.ECLevel) * 8
	data := finishBits(bits, capacity)
	cw := addECC(data, model, v, opts.ECLevel)
	return buildMatrix(cw, model, v, opts), nil
}

// Fit returns the version Encode would use for segs and opts, without
// building the symbol (which scores all eight masks).
func Fit(segs []Segment, opts Options) (int, error) {
	model, maxVer, err := validate(segs, opts)
	if err != nil {
		return 0, err
	}
	v, _, err := fit(segs, opts, model, maxVer)
	return v, err
}

// validate checks the options and segments, returning the model and its
// highest version.
func validate(segs []Segment, opts Options) (model, maxVer int, err error) {
	model = opts.Model
	if model == 0 {
		model = 2
	}
	if model != 1 && model != 2 {
		return 0, 0, invalid("model %d", opts.Model)
	}
	if opts.ECLevel < ECLevelL || opts.ECLevel > ECLevelH {
		return 0, 0, invalid("error correction level %d", int(opts.ECLevel))
	}
	if opts.ForceMask && (opts.Mask < 0 || opts.Mask > 7) {
		return 0, 0, invalid("mask %d", opts.Mask)
	}
	maxVer = 40
	if model == 1 {
		maxVer = 14
		if opts.ECI > 0 {
			return 0, 0, invalid("ECI is not supported by Model 1")
		}
	}
	if opts.Version < 0 || opts.Version > maxVer {
		return 0, 0, invalid("version %d", opts.Version)
	}
	if opts.ECI > 999999 {
		return 0, 0, invalid("ECI %d", opts.ECI)
	}
	if sa := opts.StructuredAppend; sa != nil {
		if sa.Total < 2 || sa.Total > 16 || sa.Position < 1 || sa.Position > sa.Total {
			return 0, 0, invalid("structured append %d of %d", sa.Position, sa.Total)
		}
	}
	for _, s := range segs {
		if err := validateSegment(s); err != nil {
			return 0, 0, err
		}
	}
	return model, maxVer, nil
}

// fit finds the smallest version (or the forced one) that holds the data.
func fit(segs []Segment, opts Options, model, maxVer int) (int, *bitBuf, error) {
	lo, hi := 1, maxVer
	if opts.Version > 0 {
		lo, hi = opts.Version, opts.Version
	}
	var lastErr error
	for v := lo; v <= hi; v++ {
		bits, err := buildBits(segs, opts, model, v)
		if err != nil {
			return 0, nil, err
		}
		capacity := dataCodewords(model, v, opts.ECLevel) * 8
		if bits.len() > capacity {
			lastErr = invalid("data too long for QR Model %d version %d-%s (%d bits, capacity %d)", model, v, opts.ECLevel, bits.len(), capacity)
			continue
		}
		return v, bits, nil
	}
	return 0, nil, lastErr
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: qr: %s", barcode.ErrInvalidData, fmt.Sprintf(format, args...))
}
