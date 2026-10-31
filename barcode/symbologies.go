package barcode

import (
	"fmt"
	"strings"
	"unicode/utf8"

	ibarcode "github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/aztec"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/codabar"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/codablock"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/code11"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/code128"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/code39"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/code49"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/code93"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/databar"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/datamatrix"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/i2of5"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/maxicode"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/micropdf"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/msi"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/pdf417"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/plessey"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/postal"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/qr"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/raster"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/tlc39"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/twoof5"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/barcode/upcean"
	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/bitmap"
)

type symbology struct {
	Info
	encode func(data string, o opts) (*bitmap.Bitmap, error)
}

// Option names for Info.Options.
var (
	linearOpts   = []string{"Height"}
	twoWidthOpts = []string{"Height", "Ratio"}
	compOpts     = []string{"Height"}
)

var symbologies = []symbology{
	{Info{"code128", []string{"c128"}, false, "Code 128 (subsets chosen automatically)", []string{"Height", "CheckDigit", "GS1"}}, encodeCode128(false)},
	{Info{"gs1-128", []string{"ean128", "ucc128"}, false, "GS1-128: Code 128 with FNC1, data as (AI) element string", linearOpts}, encodeCode128(true)},
	{Info{"code39", []string{"c39", "3of9", "code3of9"}, false, "Code 39; characters outside its set use Full ASCII pairs", []string{"Height", "Ratio", "CheckDigit"}}, encodeCode39},
	{Info{"logmars", nil, false, "LOGMARS: Code 39 with a mandatory Mod 43 check character", twoWidthOpts}, linear(func(d string, o opts) (ibarcode.Linear, error) { return code39.EncodeLOGMARS(d) })},
	{Info{"code93", []string{"c93"}, false, "Code 93 (full ASCII, two check characters)", linearOpts}, linear(func(d string, o opts) (ibarcode.Linear, error) { return code93.Encode(d, code93.Options{}) })},
	{Info{"code11", nil, false, "Code 11 (USD-8) with C and K check digits", twoWidthOpts}, linear(func(d string, o opts) (ibarcode.Linear, error) { return code11.Encode(d, code11.Options{}) })},
	{Info{"codabar", []string{"nw7"}, false, "Codabar; data may start and end with start/stop characters A–D (default A)", twoWidthOpts}, encodeCodabar},
	{Info{"itf", []string{"i2of5", "interleaved2of5", "i25"}, false, "Interleaved 2 of 5 (odd lengths get a leading zero)", []string{"Height", "Ratio", "CheckDigit"}}, linear(func(d string, o opts) (ibarcode.Linear, error) {
		return i2of5.Encode(d, i2of5.Options{CheckDigit: o.CheckDigit})
	})},
	{Info{"industrial2of5", []string{"code25", "2of5", "industrial25"}, false, "Industrial 2 of 5", twoWidthOpts}, linear(func(d string, o opts) (ibarcode.Linear, error) { return twoof5.EncodeIndustrial(d) })},
	{Info{"standard2of5", []string{"standard25"}, false, "Standard 2 of 5", twoWidthOpts}, linear(func(d string, o opts) (ibarcode.Linear, error) { return twoof5.EncodeStandard(d) })},
	{Info{"msi", []string{"msiplessey"}, false, "MSI Plessey with a Mod 10 check digit", twoWidthOpts}, linear(func(d string, o opts) (ibarcode.Linear, error) { return msi.Encode(d, msi.Options{}) })},
	{Info{"plessey", []string{"ukplessey"}, false, "Plessey (hex digits) with CRC", twoWidthOpts}, linear(func(d string, o opts) (ibarcode.Linear, error) { return plessey.Encode(d, plessey.Options{}) })},
	{Info{"ean13", []string{"jan13", "ean"}, false, "EAN-13: 12 digits (check digit computed) or 13 (check digit verified)", linearOpts}, encodeUPCEAN("ean13", 12, upcean.EncodeEAN13)},
	{Info{"ean8", []string{"jan8"}, false, "EAN-8: 7 digits (check digit computed) or 8 (check digit verified)", linearOpts}, encodeUPCEAN("ean8", 7, upcean.EncodeEAN8)},
	{Info{"upca", []string{"upc"}, false, "UPC-A: 11 digits (check digit computed) or 12 (check digit verified)", linearOpts}, encodeUPCEAN("upca", 11, upcA)},
	{Info{"upce", nil, false, "UPC-E: 6 digits, number system + 6, or + check digit (8); or the 10–12 digit UPC-A form", linearOpts}, encodeUPCEAN("upce", 11, upcE)},
	{Info{"postnet", nil, false, "USPS POSTNET (5, 9 or 11 digits; check digit added)", linearOpts}, encodePostal(postal.EncodePOSTNET, false)},
	{Info{"planet", nil, false, "USPS PLANET (11 or 13 digits; check digit added)", linearOpts}, encodePostal(postal.EncodePLANET, false)},
	{Info{"imb", []string{"intelligentmail", "uspsimb", "onecode"}, false, "USPS Intelligent Mail barcode (20-digit tracking code + 0, 5, 9 or 11 digit routing code)", linearOpts}, encodePostal(postal.EncodeIMb, true)},
	{Info{"qr", []string{"qrcode"}, true, "QR Code (Model 2)", []string{"ECLevel", "GS1"}}, encodeQR(false)},
	{Info{"gs1-qr", []string{"gs1qrcode"}, true, "GS1 QR Code: data as (AI) element string", []string{"ECLevel"}}, encodeQR(true)},
	{Info{"datamatrix", []string{"dm", "datamatrixecc200"}, true, "Data Matrix ECC 200", []string{"GS1", "Columns", "Rows", "Rectangular"}}, encodeDataMatrix(false)},
	{Info{"gs1-datamatrix", nil, true, "GS1 DataMatrix: data as (AI) element string", []string{"Columns", "Rows", "Rectangular"}}, encodeDataMatrix(true)},
	{Info{"aztec", []string{"azteccode"}, true, "Aztec Code", []string{"ECPercent", "Layers", "Compact"}}, encodeAztec},
	{Info{"pdf417", nil, true, "PDF417", []string{"Security", "Columns", "Rows", "RowHeight", "Compact"}}, encodePDF417},
	{Info{"micropdf417", []string{"micropdf"}, true, "MicroPDF417", []string{"Columns", "RowHeight"}}, encodeMicroPDF},
	{Info{"maxicode", nil, true, "MaxiCode", []string{"Mode"}}, encodeMaxiCode},
	{Info{"codablock-f", []string{"codablock"}, true, "CODABLOCK-F (stacked Code 128)", []string{"Columns", "Rows", "RowHeight", "Ratio", "GS1"}}, encodeCodablock},
	{Info{"code49", nil, true, "Code 49 (stacked)", []string{"RowHeight"}}, encodeCode49},
	{Info{"tlc39", nil, true, "TLC39: Code 39 ECI number with a MicroPDF417, data \"ECI[,serial[,fields...]]\"", []string{"Height", "Ratio", "RowHeight"}}, encodeTLC39},
	{Info{"databar", []string{"databaromni", "databaromnidirectional", "gs1databar", "rss14"}, false, "GS1 DataBar Omnidirectional (GTIN; '|' adds a composite component)", nil}, encodeDataBar(databar.TypeOmnidirectional, false)},
	{Info{"databar-truncated", []string{"rss14truncated"}, false, "GS1 DataBar Truncated", nil}, encodeDataBar(databar.TypeTruncated, false)},
	{Info{"databar-stacked", []string{"rss14stacked"}, true, "GS1 DataBar Stacked", nil}, encodeDataBar(databar.TypeStacked, false)},
	{Info{"databar-stacked-omni", []string{"databarstackedomnidirectional", "rss14stackedomni"}, true, "GS1 DataBar Stacked Omnidirectional", nil}, encodeDataBar(databar.TypeStackedOmnidirectional, false)},
	{Info{"databar-limited", []string{"rsslimited"}, false, "GS1 DataBar Limited (GTIN starting 0 or 1)", nil}, encodeDataBar(databar.TypeLimited, false)},
	{Info{"databar-expanded", []string{"rssexpanded"}, false, "GS1 DataBar Expanded: (AI) element string", nil}, encodeDataBar(databar.TypeExpanded, false)},
	{Info{"databar-expanded-stacked", []string{"rssexpandedstacked"}, true, "GS1 DataBar Expanded Stacked: (AI) element string", []string{"Columns"}}, encodeDataBar(databar.TypeExpanded, true)},
	{Info{"upca-cc", nil, true, "UPC-A composite: \"digits|(AI)2D data\" (CC-A or CC-B)", compOpts}, encodeComposite(raster.CompositeUPCA)},
	{Info{"upce-cc", nil, true, "UPC-E composite (CC-A or CC-B)", compOpts}, encodeComposite(raster.CompositeUPCE)},
	{Info{"ean13-cc", nil, true, "EAN-13 composite (CC-A or CC-B)", compOpts}, encodeComposite(raster.CompositeEAN13)},
	{Info{"ean8-cc", nil, true, "EAN-8 composite (CC-A or CC-B)", compOpts}, encodeComposite(raster.CompositeEAN8)},
	{Info{"gs1-128-cc", nil, true, "GS1-128 composite with CC-A or CC-B", compOpts}, encodeComposite(raster.CompositeGS1128)},
	{Info{"gs1-128-ccc", nil, true, "GS1-128 composite with CC-C", compOpts}, encodeComposite(raster.CompositeGS1128CCC)},
}

var lookup = func() map[string]*symbology {
	m := map[string]*symbology{}
	add := func(name string, s *symbology) {
		k := normalize(name)
		if _, dup := m[k]; dup {
			panic("barcode: duplicate symbology name " + name)
		}
		m[k] = s
	}
	for i := range symbologies {
		s := &symbologies[i]
		add(s.Name, s)
		for _, a := range s.Aliases {
			add(a, s)
		}
	}
	return m
}()

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrInvalidData}, a...)...)
}

// linear wraps a linear encoder: bars Height modules high.
func linear(enc func(string, opts) (ibarcode.Linear, error)) func(string, opts) (*bitmap.Bitmap, error) {
	return func(data string, o opts) (*bitmap.Bitmap, error) {
		l, err := enc(data, o)
		if err != nil {
			return nil, err
		}
		return raster.Bars(l, o.mw, o.Ratio, o.height(1)), nil
	}
}

func encodeCode128(gs1 bool) func(string, opts) (*bitmap.Bitmap, error) {
	return linear(func(data string, o opts) (ibarcode.Linear, error) {
		if !isASCII(data) {
			return ibarcode.Linear{}, invalid("code128: data %q is not ASCII", data)
		}
		if gs1 || o.GS1 {
			if _, err := gs1Reduced(data); err != nil {
				return ibarcode.Linear{}, err
			}
			return code128.Encode(data, code128.Options{Mode: code128.ModeGS1})
		}
		// Automatic mode reads ">x" as a Zebra function code; ">0" is a
		// literal '>'.
		return code128.Encode(strings.ReplaceAll(data, ">", ">0"), code128.Options{Mode: code128.ModeAuto, UCCCheckDigit: o.CheckDigit})
	})
}

var encodeCode39 = linear(func(data string, o opts) (ibarcode.Linear, error) {
	return code39.Encode(data, code39.Options{CheckDigit: o.CheckDigit})
})

func isStartStop(c byte) bool { return strings.IndexByte("ABCDabcd", c) >= 0 }

var encodeCodabar = linear(func(data string, o opts) (ibarcode.Linear, error) {
	co := codabar.Options{}
	if len(data) >= 2 && isStartStop(data[0]) && isStartStop(data[len(data)-1]) {
		co.Start, co.Stop = data[0], data[len(data)-1]
		data = data[1 : len(data)-1]
	}
	return codabar.Encode(data, co)
})

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// upcData validates EAN/UPC data and returns the digits to pass to the
// encoder, which computes the check digit. Up to n digits pass as they are
// (the encoder zero pads them on the left, as ZPL does); n+1 digits must end
// in the correct check digit, which is dropped. UPC-E (upce true) also
// takes its own 6, 7 or 8 digit forms, expanded to the UPC-A message.
func upcData(name, data string, n int, upce bool) (string, error) {
	if data == "" || !allDigits(data) {
		return "", invalid("%s: data %q must be digits", name, data)
	}
	if upce {
		switch len(data) {
		case 6:
			return expandUPCE('0', data), nil
		case 7:
			return expandUPCE(data[0], data[1:]), nil
		case 8:
			a := expandUPCE(data[0], data[1:7])
			return a, verifyCheck(name, data, a, data[7])
		case 10, 11, 12:
		default:
			return "", invalid("%s: data %q must have 6, 7, 8, 10, 11 or 12 digits", name, data)
		}
	}
	switch {
	case len(data) <= n:
		return data, nil
	case len(data) == n+1:
		return data[:n], verifyCheck(name, data, data[:n], data[n])
	}
	return "", invalid("%s: data %q has more than %d digits", name, data, n+1)
}

func verifyCheck(name, data, digits string, got byte) error {
	if want := upcean.CheckDigit(digits); got != want {
		return fmt.Errorf("%w: %w: %s: %q ends in %c, want %c", ErrInvalidData, ErrCheckDigit, name, data, got, want)
	}
	return nil
}

func encodeUPCEAN(name string, n int, enc func(string) (ibarcode.Linear, error)) func(string, opts) (*bitmap.Bitmap, error) {
	return linear(func(data string, o opts) (ibarcode.Linear, error) {
		d, err := upcData(name, data, n, name == "upce")
		if err != nil {
			return ibarcode.Linear{}, err
		}
		return enc(d)
	})
}

func upcA(d string) (ibarcode.Linear, error) { return upcean.EncodeUPCA(d, upcean.UPCOptions{}) }
func upcE(d string) (ibarcode.Linear, error) { return upcean.EncodeUPCE(d, upcean.UPCOptions{}) }

// expandUPCE converts a number system digit and six UPC-E digits to the
// 11-digit UPC-A message.
func expandUPCE(ns byte, e string) string {
	var m, p string // manufacturer and product codes, 5 digits each
	switch e[5] {
	case '0', '1', '2':
		m, p = e[:2]+e[5:6]+"00", "00"+e[2:5]
	case '3':
		m, p = e[:3]+"00", "000"+e[3:5]
	case '4':
		m, p = e[:4]+"0", "0000"+e[4:5]
	default:
		m, p = e[:5], "0000"+e[5:6]
	}
	return string(ns) + m + p
}

func encodePostal(enc func(string) (ibarcode.Postal, error), imb bool) func(string, opts) (*bitmap.Bitmap, error) {
	return func(data string, o opts) (*bitmap.Bitmap, error) {
		pc, err := enc(data)
		if err != nil {
			return nil, err
		}
		def := 10
		if imb {
			def = 9
		}
		// Bars and spaces are one module wide, as the USPS specifies
		// (about 22 bars per inch).
		return raster.Postal(pc, o.mw, 2*o.mw, o.height(def), imb), nil
	}
}

func matrix(m ibarcode.Matrix, o opts) *bitmap.Bitmap { return raster.Matrix(m, o.mw, o.mw) }

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 128 {
			return false
		}
	}
	return true
}

// gs1Reduced parses a GS1 element string into its encoded form: AIs and
// data with GS (FNC1) after variable-length fields.
func gs1Reduced(data string) (string, error) {
	r, _, err := databar.ParseElementString(data)
	return r, err
}

func encodeQR(gs1 bool) func(string, opts) (*bitmap.Bitmap, error) {
	return func(data string, o opts) (*bitmap.Bitmap, error) {
		qo := qr.Options{Model: 2, DisableAutoKanji: true, GS1: gs1 || o.GS1}
		switch strings.ToUpper(o.ECLevel) {
		case "L":
			qo.ECLevel = qr.ECLevelL
		case "Q":
			qo.ECLevel = qr.ECLevelQ
		case "H":
			qo.ECLevel = qr.ECLevelH
		default:
			qo.ECLevel = qr.ECLevelM
		}
		seg := qr.Segment{Mode: qr.ModeAuto, Data: []byte(data)}
		if qo.GS1 {
			r, err := gs1Reduced(data)
			if err != nil {
				return nil, err
			}
			seg.Data = []byte(r)
			// In GS1 mode a reader turns an alphanumeric '%' into FNC1, so
			// data with '%' goes in byte mode, where GS is FNC1.
			if strings.Contains(r, "%") {
				seg.Mode = qr.ModeByte
			}
		} else if !isASCII(data) && utf8.ValidString(data) {
			qo.ECI = 26 // UTF-8
		}
		m, err := qr.Encode([]qr.Segment{seg}, qo)
		if err != nil {
			return nil, err
		}
		return matrix(m, o), nil
	}
}

func encodeDataMatrix(gs1 bool) func(string, opts) (*bitmap.Bitmap, error) {
	return func(data string, o opts) (*bitmap.Bitmap, error) {
		if o.Columns > 144 || o.Rows > 144 {
			return nil, fmt.Errorf("%w: Data Matrix Columns/Rows %d/%d above 144", ErrInvalidOption, o.Columns, o.Rows)
		}
		do := datamatrix.Options{Quality: 200, Columns: o.Columns, Rows: o.Rows, Rectangular: o.Rectangular}
		b := []byte(data)
		if gs1 || o.GS1 {
			r, err := gs1Reduced(data)
			if err != nil {
				return nil, err
			}
			// FNC1 first, and as the separator: "_1" with '_' as escape.
			do.Escape = '_'
			r = strings.ReplaceAll(r, "_", "__")
			b = []byte("_1" + strings.ReplaceAll(r, "\x1d", "_1"))
		}
		m, err := datamatrix.Encode(b, do)
		if err != nil {
			return nil, err
		}
		return matrix(m, o), nil
	}
}

func encodeAztec(data string, o opts) (*bitmap.Bitmap, error) {
	ao := aztec.Options{ECPercent: o.ECPercent, Layers: o.Layers, Compact: o.Compact && o.Layers > 0}
	if o.Compact && o.Layers > 4 {
		return nil, fmt.Errorf("%w: compact Aztec Layers %d above 4", ErrInvalidOption, o.Layers)
	}
	b := []byte(data)
	if !isASCII(data) && utf8.ValidString(data) {
		// ECI 26 (UTF-8); a literal backslash is escaped as "\\".
		ao.ECI = true
		b = []byte(`\000026` + strings.ReplaceAll(data, `\`, `\\`))
	}
	m, err := aztec.Encode(b, ao)
	if err != nil {
		return nil, err
	}
	return matrix(m, o), nil
}

func encodePDF417(data string, o opts) (*bitmap.Bitmap, error) {
	if o.Columns > 30 || (o.Rows != 0 && (o.Rows < 3 || o.Rows > 90)) {
		return nil, fmt.Errorf("%w: PDF417 Columns %d / Rows %d out of range", ErrInvalidOption, o.Columns, o.Rows)
	}
	po := pdf417.Options{SecurityLevel: o.Security, Columns: o.Columns, Rows: o.Rows, Truncated: o.Compact}
	if o.Security == 0 {
		po.SecurityLevel = pdf417.SecurityAuto
	}
	m, err := pdf417.Encode([]byte(data), po)
	if err != nil {
		return nil, err
	}
	return raster.Matrix(m, o.mw, o.rowHeight(3)*o.mw), nil
}

func encodeMicroPDF(data string, o opts) (*bitmap.Bitmap, error) {
	if o.Columns > 4 {
		return nil, fmt.Errorf("%w: MicroPDF417 Columns %d above 4", ErrInvalidOption, o.Columns)
	}
	cws := pdf417.CompactMicro([]byte(data))
	v, ok := micropdf.Smallest(o.Columns, len(cws))
	if !ok {
		return nil, invalid("micropdf417: data too long (%d codewords)", len(cws))
	}
	m, err := micropdf.EncodeCodewords(cws, v)
	if err != nil {
		return nil, err
	}
	return raster.Matrix(m, o.mw, o.rowHeight(2)*o.mw), nil
}

func encodeMaxiCode(data string, o opts) (*bitmap.Bitmap, error) {
	mode := o.Mode
	if mode == 0 {
		mode = 4
	}
	m, err := maxicode.Encode([]byte(data), maxicode.Options{Mode: mode})
	if err != nil {
		return nil, err
	}
	return raster.MaxiCode(m, float64(10*o.mw)), nil
}

func encodeCodablock(data string, o opts) (*bitmap.Bitmap, error) {
	if o.Columns > 62 || o.Rows > 44 {
		return nil, fmt.Errorf("%w: CODABLOCK-F Columns %d / Rows %d out of range", ErrInvalidOption, o.Columns, o.Rows)
	}
	mode := codablock.ModeF
	if o.GS1 {
		mode = codablock.ModeE
	}
	m, err := codablock.Encode([]byte(data), codablock.Options{
		Mode:      mode,
		Columns:   o.Columns,
		Rows:      o.Rows,
		RowHeight: o.rowHeight(8),
		SepHeight: 1,
		Narrow:    o.mw,
		Ratio:     o.Ratio,
	})
	if err != nil {
		return nil, err
	}
	return raster.Matrix(m, 1, 1), nil
}

func encodeCode49(data string, o opts) (*bitmap.Bitmap, error) {
	sym, err := code49.Encode(data, code49.Options{Mode: 'A', RowHeight: o.rowHeight(8)})
	if err != nil {
		return nil, err
	}
	return matrix(sym.Matrix, o), nil
}

func encodeTLC39(data string, o opts) (*bitmap.Bitmap, error) {
	sym, err := tlc39.Encode(data, tlc39.Options{})
	if err != nil {
		return nil, err
	}
	bm, _ := raster.TLC39(sym, o.mw, o.Ratio, o.height(20), o.mw, o.rowHeight(2)*o.mw)
	return bm, nil
}

// encodeDataBar draws a GS1 DataBar type. For Expanded Stacked
// (expandedStacked) Columns is the segments per row, 4 by default.
func encodeDataBar(t databar.Type, expandedStacked bool) func(string, opts) (*bitmap.Bitmap, error) {
	return func(data string, o opts) (*bitmap.Bitmap, error) {
		segments := 22
		if expandedStacked {
			segments = o.Columns
			if segments == 0 {
				segments = 4
			}
			if segments < 2 || segments > 22 || segments%2 != 0 {
				return nil, fmt.Errorf("%w: DataBar Expanded Stacked Columns %d must be even, 2-22", ErrInvalidOption, segments)
			}
		}
		bm, err := raster.DataBar(t, data, o.mw, 1, segments, false)
		if err != nil {
			return nil, err
		}
		return trimX(bm), nil
	}
}

func encodeComposite(kind raster.CompositeLinear) func(string, opts) (*bitmap.Bitmap, error) {
	return func(data string, o opts) (*bitmap.Bitmap, error) {
		lin, cc, hasCC := strings.Cut(data, "|")
		var err error
		switch kind {
		case raster.CompositeUPCA:
			lin, err = upcData("upca-cc", lin, 11, false)
		case raster.CompositeUPCE:
			lin, err = upcData("upce-cc", lin, 11, true)
		case raster.CompositeEAN13:
			lin, err = upcData("ean13-cc", lin, 12, false)
		case raster.CompositeEAN8:
			lin, err = upcData("ean8-cc", lin, 7, false)
		default:
			_, err = gs1Reduced(lin)
		}
		if err != nil {
			return nil, err
		}
		if hasCC {
			lin += "|" + cc
		}
		bm, err := raster.Composite(kind, lin, raster.CompositeOptions{
			Mag:        o.mw,
			SepHeight:  1,
			Height:     o.height(25),
			UPCHeight:  o.height(74),
			EAN8Height: o.height(60),
			Lead:       true,
		})
		if err != nil {
			return nil, err
		}
		return trimX(bm), nil
	}
}

// trimX removes blank columns at the left and right.
func trimX(bm *bitmap.Bitmap) *bitmap.Bitmap {
	lo, hi := bm.W, -1
	for y := 0; y < bm.H; y++ {
		for x := 0; x < bm.W; x++ {
			if bm.Get(x, y) {
				lo, hi = min(lo, x), max(hi, x)
			}
		}
	}
	if hi < lo || (lo == 0 && hi == bm.W-1) {
		return bm
	}
	out := bitmap.New(hi-lo+1, bm.H)
	out.Draw(bm, -lo, 0, bitmap.Black)
	return out
}
