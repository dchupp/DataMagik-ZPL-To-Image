package zpl

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/compress"
)

func TestRescale_InvalidDensity(t *testing.T) {
	for _, d := range [][2]int{{0, 8}, {8, 0}, {8, 10}, {300, 8}} {
		if _, _, err := Rescale([]byte("^XA^XZ"), d[0], d[1]); err == nil {
			t.Errorf("Rescale(%d→%d): want error", d[0], d[1])
		}
	}
}

func TestRescale_SameDensityIsIdentity(t *testing.T) {
	src := []byte("^XA^FO10,10^A0N,30^FDx^FS^GFA,1,1,1,FF^XZ")
	for _, d := range []int{6, 8, 12, 24} {
		got, warnings, err := Rescale(src, d, d)
		if err != nil || len(warnings) != 0 || &got[0] != &src[0] {
			t.Errorf("Rescale(%d→%d) = %q, %v, %v; want src itself", d, d, got, warnings, err)
		}
	}
}

// TestRescale_Commands checks each command fragment at 8→12 dots/mm (×1.5)
// unless the case names other densities. warn is a substring every warning
// must contain ("" means no warnings).
func TestRescale_Commands(t *testing.T) {
	tests := []struct {
		name     string
		from, to int
		in, want string
		warn     string
	}{
		{name: "field origin", in: "^FO10,20", want: "^FO15,30"},
		{name: "field origin justification kept", in: "^FO10,20,1", want: "^FO15,30,1"},
		{name: "spaces and CRLF kept", in: "^FT 7 ,\r\n20", want: "^FT 11 ,\r\n30"},
		{name: "trailing junk kept", in: "^FO10abc,20", want: "^FO15abc,30"},
		{name: "label home", in: "^LH5,5", want: "^LH8,8"},
		{name: "label size", in: "^PW812^LL1218", want: "^PW1218^LL1827"},
		{name: "label size clamped", in: "^PW30000", want: "^PW32000", warn: "clamped"},
		{name: "label shift negative", in: "^LS-10", want: "^LS-15"},
		{name: "label top clamped", in: "^LT200", want: "^LT240", warn: "clamped"},
		{name: "max label length", in: "^ML1000", want: "^ML1500"},
		{name: "box keeps rounding and colour", in: "^GB100,50,3,B,4", want: "^GB150,75,5,B,4"},
		{name: "box thickness stays at least 1", from: 24, to: 6, in: "^GB100,50,1", want: "^GB25,13,1"},
		{name: "circle", in: "^GC90,10,B", want: "^GC135,15,B"},
		{name: "ellipse", in: "^GE200,100,4", want: "^GE300,150,6"},
		{name: "diagonal", in: "^GD100,100,6,B,R", want: "^GD150,150,9,B,R"},
		{name: "field block", in: "^FB300,3,-4,C,10", want: "^FB450,3,-6,C,15"},
		{name: "text block", in: "^TBN,300,60", want: "^TBN,450,90"},
		{name: "field parameter gap", in: "^FPV,4", want: "^FPV,6"},
		{name: "symbol font", in: "^GSN,60,60", want: "^GSN,90,90"},
		{name: "font height and width", in: "^A0N,30,20", want: "^A0N,45,30"},
		{name: "font height only", in: "^A0N,30", want: "^A0N,45"},
		{name: "font by name", in: "^A@N,30,30,E:ARIAL.TTF", want: "^A@N,45,45,E:ARIAL.TTF"},
		{name: "font without size gets the scaled power-up default", in: "^A0N", want: "^A0N,14,8"},
		{name: "font without orientation or size", in: "^A0", want: "^A0,14,8"},
		{name: "font without size uses ^CF", in: "^CF0,40^A0N", want: "^CF0,60^A0N"},
		{name: "default font", in: "^CF0,40,30", want: "^CF0,60,45"},
		{name: "default font name only", in: "^CF0", want: "^CF0"},
		{name: "bitmap font that cannot match", in: "^ADN,18,10", want: "^ADN,27,15", warn: "bitmap font D"},
		{name: "bitmap font that matches", from: 6, to: 12, in: "^ADN,18,10", want: "^ADN,36,20"},
		{name: "barcode defaults", in: "^BY2,3,80", want: "^BY3,3,120"},
		{name: "module width rounding warned", in: "^BY1", want: "^BY2", warn: "module size 1"},
		{name: "module width clamped", from: 6, to: 24, in: "^BY3", want: "^BY10", warn: "clamped"},
		{name: "code 128 height", in: "^BY2^BCN,80,Y,N,N", want: "^BY3^BCN,120,Y,N,N"},
		{name: "code 39 height", in: "^BY2^B3N,N,80,Y,N", want: "^BY3^B3N,N,120,Y,N"},
		{name: "codabar height", in: "^BY2^BKN,N,80", want: "^BY3^BKN,N,120"},
		{name: "upc height", in: "^BY2^BUN,100,Y,N", want: "^BY3^BUN,150,Y,N"},
		{name: "postnet height", in: "^BY2^BZN,30", want: "^BY3^BZN,45"},
		{name: "barcode without height", in: "^BY2^BCN", want: "^BY3^BCN", warn: "power-up ^BY height"},
		{name: "barcode without ^BY", in: "^BCN,80", want: "^BCN,120", warn: "power-up ^BY module"},
		{name: "PDF417 row height", in: "^BY2^B7N,4,3", want: "^BY3^B7N,6,3"},
		{name: "MicroPDF417 row height", in: "^BY2^BFN,40,10", want: "^BY3^BFN,60,10"},
		{name: "CODABLOCK row height", in: "^BY2^BBN,10,Y", want: "^BY3^BBN,15,Y"},
		{name: "code 49 height multiplier unchanged", in: "^BY2^B4N,5,B,A", want: "^BY3^B4N,5,B,A"},
		{name: "QR magnification", in: "^BQN,2,4", want: "^BQN,2,6"},
		{name: "QR magnification clamped", in: "^BQN,2,8", want: "^BQN,2,10", warn: "clamped"},
		{name: "QR default magnification left to the printer", in: "^BQN,2", want: "^BQN,2"},
		{name: "QR default magnification made explicit", from: 6, to: 8, in: "^BQN,2", want: "^BQN,2,1", warn: "module size 1"},
		{name: "DataBar default magnification made explicit", in: "^BRN,1", want: "^BRN,1,2", warn: "module size 1"},
		{name: "DataBar default kept when it matches", from: 12, to: 24, in: "^BRN,1", want: "^BRN,1"},
		{name: "TLC39 defaults made explicit", from: 8, to: 24, in: "^BTN", want: "^BTN,6,,,6,12"},
		{name: "Data Matrix module", in: "^BXN,6,200", want: "^BXN,9,200"},
		{name: "Aztec magnification", in: "^BON,6,N", want: "^BON,9,N"},
		{name: "DataBar magnification", in: "^BRN,1,2,1,25", want: "^BRN,1,3,1,25"},
		{name: "TLC39", in: "^BTN,2,2,40,2,4", want: "^BTN,3,2,60,3,6"},
		{name: "MaxiCode is a fixed size", in: "^BD2,1,1", want: "^BD2,1,1"},
		{name: "field multiple origins", in: "^FM10,20,e,e", want: "^FM15,30,e,e"},
		{name: "RFID position in dots", in: "^RS8,F100^RS8,200", want: "^RS8,F150^RS8,300"},
		{name: "RFID position in millimetres", in: "^RS8,B5", want: "^RS8,B5"},
		{name: "slew and media offsets", in: "^PF10~TA-20^MNM,40", want: "^PF15~TA-30^MNM,60"},
		{name: "stored graphic magnification needs a whole ratio", in: "^XGR:LOGO.GRF,1,1", want: "^XGR:LOGO.GRF,1,1", warn: "not defined"},
		{name: "stored graphic magnified", from: 6, to: 12, in: "^XGR:LOGO.GRF,2", want: "^XGR:LOGO.GRF,4,2", warn: "not defined"},
		{name: "stored graphic recall", in: "^IMR:LOGO.GRF", want: "^IMR:LOGO.GRF", warn: "not defined"},
		{name: "stored format recall", in: "^XFR:LABEL.ZPL", want: "^XFR:LABEL.ZPL", warn: "not defined"},
		{name: "stored format in the stream is rescaled", in: "^XA^DFR:LABEL.ZPL^FS^FO10,10^FN1^FS^XZ^XA^XFR:LABEL.ZPL^XZ",
			want: "^XA^DFR:LABEL.ZPL^FS^FO15,15^FN1^FS^XZ^XA^XFR:LABEL.ZPL^XZ"},
		{name: "units in millimetres are not scaled", in: "^MUM^FO10,10^BY2", want: "^MUM^FO10,10^BY3", warn: "physical units"},
		{name: "units back to dots", in: "^MUM^FO10,10^MUD^FO10,10", want: "^MUM^FO10,10^MUD^FO15,15", warn: "physical units"},
		{name: "units dpi conversion left", in: "^MUD,200,300", want: "^MUD,200,300", warn: "conversion"},
		{name: "prefix and delimiter changes", in: "^CC+\n+FO10,20+CD;+FO10;20", want: "^CC+\n+FO15,30+CD;+FO15;30"},
		{name: "digit prefix blocks the rewrite", in: "^CT5^FO10,20", want: "^CT5^FO10,20", warn: "prefix"},
		{name: "field data untouched", in: "^A0N,30^FD10,20^FS", want: "^A0N,45^FD10,20^FS"},
		{name: "text with default font warned", in: "^FO10,10^FDx^FS", want: "^FO15,15^FDx^FS", warn: "default font size"},
		{name: "downloaded bitmap font", in: "~DBR:F.FNT,N,10,10,8,2,1,#0041.10.8.0.8.10.FF", want: "~DBR:F.FNT,N,10,10,8,2,1,#0041.10.8.0.8.10.FF", warn: "bitmap font"},
		{name: "passthrough", in: "^PR4^MD10^PQ5^SN001,1,Y^FXcomment 10,20^FS^RFW,H^HV1,8^JUS~JA^CI28^FWR",
			want: "^PR4^MD10^PQ5^SN001,1,Y^FXcomment 10,20^FS^RFW,H^HV1,8^JUS~JA^CI28^FWR"},
		{name: "downscale", from: 12, to: 8, in: "^FO15,30^A0N,45,30", want: "^FO10,20^A0N,30,20"},
		{name: "24 to 6", from: 24, to: 6, in: "^FO100,10^BY4", want: "^FO25,3^BY1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			from, to := tt.from, tt.to
			if from == 0 {
				from, to = 8, 12
			}
			got, warnings, err := Rescale([]byte(tt.in), from, to)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("Rescale(%q, %d, %d) = %q, want %q", tt.in, from, to, got, tt.want)
			}
			if tt.warn == "" && len(warnings) > 0 {
				t.Errorf("unexpected warnings %v", warnings)
			}
			if tt.warn != "" {
				found := false
				for _, w := range warnings {
					found = found || strings.Contains(w.Message, tt.warn)
				}
				if !found {
					t.Errorf("warnings %v do not mention %q", warnings, tt.warn)
				}
			}
		})
	}
}

func TestRescale_WarningOffsets(t *testing.T) {
	src := []byte("^XA^FO1,1^BY1^XZ")
	_, warnings, err := Rescale(src, 8, 12)
	if err != nil || len(warnings) != 1 {
		t.Fatalf("warnings %v, err %v", warnings, err)
	}
	if w := warnings[0]; w.Offset != 9 || w.Command != "^BY" {
		t.Errorf("warning %+v, want ^BY at byte 9", w)
	}
}

// bits decodes a rescaled graphic payload to raw rows.
func graphicBytes(t *testing.T, data string, row, total int) []byte {
	t.Helper()
	raw, err := compress.DecodeGraphic(data, row, total)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func z64(raw []byte) string {
	var z bytes.Buffer
	w := zlib.NewWriter(&z)
	w.Write(raw)
	w.Close()
	return ":Z64:" + withCRC(base64.StdEncoding.EncodeToString(z.Bytes()))
}

func TestRescale_Graphics(t *testing.T) {
	// An 8×2 graphic: a black row over a white row.
	tests := []struct {
		name     string
		from, to int
		in, want string
	}{
		{name: "hex doubled", from: 6, to: 12, in: "^GFA,2,2,1,FF00^FS", want: "^GFA,8,8,2,FFFFFFFF00000000^FS"},
		{name: "hex ×1.5", from: 8, to: 12, in: "^GFA,2,2,1,FF00\r\n^FS", want: "^GFA,6,6,2,FFF000000000\r\n^FS"},
		{name: "hex halved", from: 12, to: 6, in: "^GFA,8,8,2,FFFFFFFF00000000^FS", want: "^GFA,2,2,1,FF00^FS"},
		{name: "download", from: 6, to: 12, in: "~DGR:LOGO.GRF,2,1,FF00^XA^XGR:LOGO.GRF,1,1^FS^XZ",
			want: "~DGR:LOGO.GRF,8,2,FFFFFFFF00000000^XA^XGR:LOGO.GRF,1,1^FS^XZ"},
		{name: "changed delimiter", from: 6, to: 12, in: "^CD;^GFA;2;2;1;FF00", want: "^CD;^GFA;8;8;2;FFFFFFFF00000000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, warnings, err := Rescale([]byte(tt.in), tt.from, tt.to)
			if err != nil || len(warnings) != 0 {
				t.Fatalf("err %v, warnings %v", err, warnings)
			}
			if string(got) != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestRescale_GraphicEncodings checks compressed and binary graphics: the
// output is :Z64: data that decodes to the resampled bitmap.
func TestRescale_GraphicEncodings(t *testing.T) {
	want := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0, 0, 0, 0}
	tests := []struct {
		name, in, prefix, suffix string
		row, total               int
	}{
		{name: "Z64 ^GF", in: "^GFA,2,2,1," + z64([]byte{0xFF, 0}) + "^FS", prefix: "^GFA,8,8,2,", suffix: "^FS", row: 2, total: 8},
		{name: "ACS ^GF", in: "^GFA,2,2,1,FF,^FS", prefix: "^GFA,8,8,2,", suffix: "^FS", row: 2, total: 8},
		{name: "binary ^GF", in: "^GFB,2,2,1,\xff\x00^FS", prefix: "^GFA,8,8,2,", suffix: "^FS", row: 2, total: 8},
		{name: "binary ~DY", in: "~DYR:LOGO,B,G,2,1,\xff\x00^XA", prefix: "~DYR:LOGO,A,G,8,2,", suffix: "^XA", row: 2, total: 8},
		{name: "hex ~DY", in: "~DYR:LOGO,A,GRF,2,1,FF00^XA", prefix: "~DYR:LOGO,A,GRF,8,2,", suffix: "^XA", row: 2, total: 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, warnings, err := Rescale([]byte(tt.in), 6, 12)
			if err != nil || len(warnings) != 0 {
				t.Fatalf("err %v, warnings %v", err, warnings)
			}
			s := string(got)
			if !strings.HasPrefix(s, tt.prefix) || !strings.HasSuffix(s, tt.suffix) {
				t.Fatalf("got %q", s)
			}
			data := strings.TrimSuffix(strings.TrimPrefix(s, tt.prefix), tt.suffix)
			if raw := graphicBytes(t, data, tt.row, tt.total); !bytes.Equal(raw, want) {
				t.Errorf("decoded %x, want %x (data %q)", raw, want, data)
			}
		})
	}
}

func TestRescale_PNGDownload(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 3, 2))
	img.SetGray(0, 0, color.Gray{Y: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	src := "~DYR:IMG,P,P," + strconv.Itoa(buf.Len()) + ",0," + strings.ToUpper(hex.EncodeToString(buf.Bytes())) + "^XA^XGR:IMG.PNG^FS^XZ"
	got, warnings, err := Rescale([]byte(src), 6, 12)
	if err != nil || len(warnings) != 0 {
		t.Fatalf("err %v, warnings %v", err, warnings)
	}
	cmds, _ := Tokenize(bytes.NewReader(got))
	dy := cmds[0]
	if dy.Name != "DY" || dy.Params[1] != "P" || !strings.HasPrefix(dy.Data, ":B64:") {
		t.Fatalf("got %+v", dy)
	}
	raw := graphicBytes(t, dy.Data, 0, 0)
	if strconv.Itoa(len(raw)) != dy.Params[3] {
		t.Errorf("byte count %s, data %d bytes", dy.Params[3], len(raw))
	}
	out, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if b := out.Bounds(); b.Dx() != 6 || b.Dy() != 4 {
		t.Errorf("size %v, want 6×4", b)
	}
	if g := color.GrayModel.Convert(out.At(1, 1)).(color.Gray); g.Y != 255 {
		t.Errorf("pixel (1,1) = %v, want white", g)
	}
	if g := color.GrayModel.Convert(out.At(2, 0)).(color.Gray); g.Y != 0 {
		t.Errorf("pixel (2,0) = %v, want black", g)
	}
}

func TestRescale_UnscalableDownloads(t *testing.T) {
	for _, src := range []string{
		"~DYR:IMG,A,B,4,0,00000000",
		"^GFC,2,2,1,\xff\x00",
	} {
		got, warnings, err := Rescale([]byte(src), 8, 12)
		if err != nil || string(got) != src || len(warnings) != 1 {
			t.Errorf("Rescale(%q) = %q, %v, %v; want unchanged with one warning", src, got, warnings, err)
		}
	}
}

// TestRescale_PassthroughGolden checks that every command Rescale does not
// rewrite is copied byte for byte, by comparing the commands' kinds and the
// text between them.
func TestRescale_PassthroughGolden(t *testing.T) {
	files, _ := filepath.Glob("testdata/golden/*.zpl")
	for _, file := range files {
		src, _ := os.ReadFile(file)
		from := goldenDPMM(strings.TrimSuffix(filepath.Base(file), ".zpl"))
		to := 12
		if from == 12 {
			to = 8
		}
		got, _, err := Rescale(src, from, to)
		if err != nil {
			t.Fatal(err)
		}
		a, _ := Tokenize(bytes.NewReader(src))
		b, _ := Tokenize(bytes.NewReader(got))
		if len(a) != len(b) {
			t.Fatalf("%s: %d commands became %d", file, len(a), len(b))
		}
		for i := range a {
			ca, cb := a[i], b[i]
			if ca.Prefix != cb.Prefix || ca.Name != cb.Name || ca.Data != cb.Data {
				t.Errorf("%s: command %d %c%s %q became %c%s %q", file, i, ca.Prefix, ca.Name, ca.Data, cb.Prefix, cb.Name, cb.Data)
			}
			if !rescaled[string(ca.Prefix)+ca.Name] && string(src[ca.Offset:ca.End]) != string(got[cb.Offset:cb.End]) {
				t.Errorf("%s: %q changed to %q", file, src[ca.Offset:ca.End], got[cb.Offset:cb.End])
			}
		}
	}
}

// rescaled lists the commands with dot parameters in the golden files.
var rescaled = map[string]bool{
	"^FO": true, "^FT": true, "^PW": true, "^LL": true, "^A": true, "^CF": true, "^GB": true,
	"^GC": true, "^GE": true, "^GD": true, "^FB": true, "^TB": true, "^FP": true, "^GS": true,
	"^BY": true, "^BC": true, "^B3": true, "^BE": true, "^BU": true, "^B9": true, "^B8": true,
	"^B2": true, "^BQ": true, "^BX": true, "^B0": true, "^BO": true, "^BR": true, "^BT": true,
	"^B7": true, "^BF": true, "^BB": true, "^B4": true,
}

// TestRescale_RoundTrip checks that scaling up and back down restores every
// dot parameter exactly (the rounding error of the way up is under half a
// dot at the lower density).
func TestRescale_RoundTrip(t *testing.T) {
	src := []byte("^XA^PW812^LL1218^LH3,7^FO11,21^GB101,53,3,B,2^FS^FT7,250^A0N,31,17^FDx^FS" +
		"^FO40,360^FB301,3,-3,C,9^A0N,29^FDblock^FS^FO1,1^GE33,77,5^FS^BY3,2.5,81^FO5,5^BCN,77,Y^FD1^FS^XZ")
	for _, d := range [][2]int{{8, 12}, {6, 8}, {8, 24}, {6, 24}, {12, 24}} {
		up, _, err := Rescale(src, d[0], d[1])
		if err != nil {
			t.Fatal(err)
		}
		back, _, err := Rescale(up, d[1], d[0])
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(back, src) {
			t.Errorf("%d→%d→%d:\n got %s\nwant %s", d[0], d[1], d[0], back, src)
		}
	}
}

// FuzzRescale checks that Rescale never panics, keeps every command (so its
// output tokenizes into the same command sequence), and that the identity
// density pair returns the input.
func FuzzRescale(f *testing.F) {
	files, _ := filepath.Glob("testdata/golden/*.zpl")
	for _, file := range files {
		if b, err := os.ReadFile(file); err == nil {
			f.Add(b, uint8(1), uint8(2))
		}
	}
	f.Add([]byte("^XA^GFA,2,2,1,FF00^FS~DGR:X.GRF,2,1,FF00^XGR:X.GRF,1,1^XZ"), uint8(0), uint8(3))
	f.Add([]byte("^GFB,2,2,1,\xff\x00^FS~DYR:X,B,G,2,1,\xff\x00^A0^A^CF^MUM"), uint8(3), uint8(0))
	f.Add([]byte("^CC+\n+FO10,20+CD;+FO10;20+A0N;30"), uint8(2), uint8(1))
	densities := []int{6, 8, 12, 24}
	f.Fuzz(func(t *testing.T, src []byte, a, b uint8) {
		from, to := densities[int(a)%4], densities[int(b)%4]
		got, _, err := Rescale(src, from, to)
		if err != nil {
			t.Fatal(err)
		}
		if from == to && !bytes.Equal(got, src) {
			t.Fatal("identity changed the input")
		}
		in, _ := Tokenize(bytes.NewReader(src))
		out, err := Tokenize(bytes.NewReader(got))
		if err != nil {
			t.Fatal(err)
		}
		checkTokens(t, got, out)
		if len(in) != len(out) {
			t.Fatalf("%d commands became %d:\n in %q\nout %q", len(in), len(out), src, got)
		}
		for i := range in {
			if in[i].Prefix != out[i].Prefix || in[i].Name != out[i].Name {
				t.Fatalf("command %d %c%s became %c%s", i, in[i].Prefix, in[i].Name, out[i].Prefix, out[i].Name)
			}
		}
	})
}

// Fifty 2048x2048 graphics of compressed hex are 100 KB of ZPL but 200
// megapixels to resample at 4x: a print job must not spend seconds on that.
func TestRescale_GraphicsBudget(t *testing.T) {
	var b strings.Builder
	b.WriteString("^XA")
	row := 256 // bytes per row
	for i := 0; i < 50; i++ {
		fmt.Fprintf(&b, "^FO0,0^GFA,%d,%d,%d,F,%s^FS", row*2048, row*2048, row, strings.Repeat(":", 2047))
	}
	b.WriteString("^XZ")
	start := time.Now()
	_, _, err := Rescale([]byte(b.String()), 6, 24)
	if !errors.Is(err, ErrRescaleTooLarge) {
		t.Fatalf("err = %v, want ErrRescaleTooLarge", err)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("took %v", d)
	}
	// One ordinary graphic is well inside the budget.
	if _, _, err := Rescale([]byte("^XA^FO0,0^GFA,8,8,1,FF00FF00FF00FF00^FS^XZ"), 8, 12); err != nil {
		t.Fatal(err)
	}
}
