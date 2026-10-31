package zpl

import (
	"errors"
	"image"
	"strings"
	"testing"
	"time"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/engine"
)

// Regression tests for the code review findings.

func TestRecursiveRecallTerminates(t *testing.T) {
	cases := map[string]string{
		"self": "^XA^DFR:A.ZPL^FS^FO10,10^GB5,5,5^FS^XFR:A.ZPL^XZ" +
			"^XA^XFR:A.ZPL^XZ",
		"mutual fan-out": "^XA^DFR:A.ZPL^FS^XFR:B.ZPL^XFR:B.ZPL^FO10,10^GB5,5,5^FS^XZ" +
			"^XA^DFR:B.ZPL^FS^XFR:A.ZPL^XFR:A.ZPL^XZ" +
			"^XA^XFR:A.ZPL^XZ",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			start := time.Now()
			imgs, warnings, err := Render(strings.NewReader(src), Options{LabelWidth: 1, LabelHeight: 1})
			if err != nil || len(imgs) != 1 {
				t.Fatalf("got %d images, %v", len(imgs), err)
			}
			if !black(imgs[0], 12, 12) {
				t.Error("recalled field missing")
			}
			if len(warnings) == 0 {
				t.Error("want a warning about the recursion limit")
			}
			if d := time.Since(start); d > 5*time.Second {
				t.Errorf("took %v", d)
			}
		})
	}
}

func TestHugeLabelRejected(t *testing.T) {
	_, _, err := Render(strings.NewReader("^XA^PW100000^LL100000^XZ"), Options{})
	if !errors.Is(err, ErrLabelTooLarge) {
		t.Errorf("got %v, want ErrLabelTooLarge", err)
	}
	// Fields larger than any label are dropped rather than allocated.
	if _, _, err := Render(strings.NewReader("^XA^FO0,0^GB32000,32000,32000^FS^A0N,32000^FDXXXXXXXX^FS^XZ"), Options{}); err != nil {
		t.Errorf("oversized fields: %v", err)
	}
}

func TestCopiesStreamAndBudget(t *testing.T) {
	src := "^XA^FO10,10^SN1,1^FS^PQ300^XZ"
	o := Options{LabelWidth: 1, LabelHeight: 1, ExpandCopies: true, MaxPixels: 1 << 20}
	if _, _, err := Render(strings.NewReader(src), o); !errors.Is(err, ErrTooMuchOutput) {
		t.Errorf("Render: got %v, want ErrTooMuchOutput", err)
	}
	p, _ := NewPrinter(o)
	n := 0
	if _, err := p.RenderEach(strings.NewReader(src), func(image.Image) error { n++; return nil }); err != nil || n != 300 {
		t.Errorf("RenderEach: %d labels, %v", n, err)
	}
	stop := errors.New("stop")
	n = 0
	_, err := p.RenderEach(strings.NewReader(src), func(image.Image) error {
		if n++; n == 3 {
			return stop
		}
		return nil
	})
	if !errors.Is(err, stop) || n != 3 {
		t.Errorf("callback error not propagated: %d labels, %v", n, err)
	}
}

func TestTemplateGraphicDrawnOnce(t *testing.T) {
	// A reversed box drawn twice would cancel itself out.
	p, _ := NewPrinter(Options{LabelWidth: 1, LabelHeight: 1})
	imgs, _, err := p.Render(strings.NewReader(
		"^XA^DFR:T.ZPL^FS^FO10,10^FR^GB40,40,40^FN1^FS^XZ" +
			"^XA^XFR:T.ZPL^FN1^FDx^FS^XZ"))
	if err != nil || len(imgs) != 1 {
		t.Fatalf("%d images, %v", len(imgs), err)
	}
	if !black(imgs[0], 30, 30) {
		t.Error("reversed template box disappeared")
	}
}

func TestStrictAppliesToRecalledFormats(t *testing.T) {
	p, _ := NewPrinter(Options{Strict: true})
	if _, _, err := p.Render(strings.NewReader("^XA^DFR:S.ZPL^FS^QQ1^XZ")); err != nil {
		t.Fatalf("storing: %v", err)
	}
	_, _, err := p.Render(strings.NewReader("^XA^XFR:S.ZPL^XZ"))
	var unknown *engine.UnknownCommandError
	if !errors.As(err, &unknown) {
		t.Errorf("got %v, want UnknownCommandError", err)
	}
}

func TestPQInRecalledFormat(t *testing.T) {
	p, _ := NewPrinter(Options{LabelWidth: 1, LabelHeight: 1, ExpandCopies: true})
	imgs, _, err := p.Render(strings.NewReader(
		"^XA^DFR:Q.ZPL^FS^FO10,10^A0N,30^SN1,1^FS^PQ3^XZ^XA^XFR:Q.ZPL^XZ"))
	if err != nil || len(imgs) != 3 {
		t.Errorf("got %d labels, %v; want 3", len(imgs), err)
	}
}

func TestDeleteAndTransferDefaults(t *testing.T) {
	dg := "~DGR:LOGO.GRF,2,1,FFFF\n"
	p, _ := NewPrinter(Options{LabelWidth: 1, LabelHeight: 1})
	imgs, warnings, _ := p.Render(strings.NewReader(dg + "^XA^IDR:LOGO^FS^XZ^XA^FO10,10^XGR:LOGO^FS^XZ"))
	if black(imgs[1], 10, 10) || len(warnings) == 0 {
		t.Error("^IDR:LOGO should delete R:LOGO.GRF")
	}
	p, _ = NewPrinter(Options{LabelWidth: 1, LabelHeight: 1})
	imgs, _, _ = p.Render(strings.NewReader(dg + "^XA^TOR:LOGO,E:COPY^FS^FO10,10^XGE:COPY.GRF^FS^XZ"))
	if !black(imgs[0], 10, 10) {
		t.Error("^TOR:LOGO,E:COPY should copy the graphic to E:COPY.GRF")
	}
	p, _ = NewPrinter(Options{LabelWidth: 1, LabelHeight: 1})
	imgs, _, _ = p.Render(strings.NewReader(dg + "^XA^TOR:*.GRF,B:^FS^IDR:LOGO^FO10,10^XGB:LOGO^FS^XZ"))
	if !black(imgs[0], 10, 10) {
		t.Error("^TOR:*.GRF,B: should copy keeping the name")
	}
}

func TestQRErrorCorrectionParameter(t *testing.T) {
	// The field's level switch wins; the ^BQ parameter applies only when the
	// switch is not a valid level.
	assertSame(t, "^XA^FO10,10^BQN,2,4,L^FDHA,HELLO^FS^XZ", "^XA^FO10,10^BQN,2,4^FDHA,HELLO^FS^XZ")
	assertSame(t, "^XA^FO10,10^BQN,2,4,H^FDXA,HELLO^FS^XZ", "^XA^FO10,10^BQN,2,4^FDHA,HELLO^FS^XZ")
	assertSame(t, "^XA^FO10,10^BQN,2,4^FDXA,HELLO^FS^XZ", "^XA^FO10,10^BQN,2,4^FDQA,HELLO^FS^XZ")
}

func TestDataMatrixQualityWarning(t *testing.T) {
	_, warnings, _ := Render(strings.NewReader("^XA^FO10,10^BXN,5^FDabc^FS^XZ"), Options{})
	if len(warnings) != 0 {
		t.Errorf("omitted quality warned: %v", warnings)
	}
	_, warnings, _ = Render(strings.NewReader("^XA^FO10,10^BXN,5,80^FDabc^FS^XZ"), Options{})
	if len(warnings) != 1 || warnings[0].Offset != 11 {
		t.Errorf("explicit ECC 080: want one warning at offset 11, got %v", warnings)
	}
}

func TestGoldenPathNaming(t *testing.T) {
	for i, want := range map[int]string{0: "x.png", 1: "x-2.png", 9: "x-10.png"} {
		if got := goldenPath("x", i); got != want {
			t.Errorf("goldenPath(%d) = %s, want %s", i, got, want)
		}
	}
}

// Regression tests for the security review findings.

func TestSerialNumberLongDigitRun(t *testing.T) {
	src := "^XA^FO10,10^A0N,30^SN" + strings.Repeat("9", 70) + ",1,Y^FS^PQ2^XZ"
	imgs, _, err := Render(strings.NewReader(src), Options{LabelWidth: 2, LabelHeight: 1, ExpandCopies: true})
	if err != nil || len(imgs) != 2 {
		t.Errorf("got %d labels, %v", len(imgs), err)
	}
}

func TestOversizedFieldsWithHugeLabel(t *testing.T) {
	for _, src := range []string{
		"^XA^PW100000^LL100000^FO0,0^GFA,8800000,8800000,8000,!^FS^XZ",
		"^XA^PW100000^LL100000^FO0,0^GB60000,60000,10,B,8^FS^XZ",
		"^XA^PW100000^LL100000^FO0,0^GE60000,60000,10^FS^GC60000,10^FS^GD60000,60000,10^FS^XZ",
	} {
		start := time.Now()
		_, _, err := Render(strings.NewReader(src), Options{})
		if errors.Is(err, engine.ErrInternal) {
			t.Errorf("%.40s: %v", src, err)
		}
		if d := time.Since(start); d > 5*time.Second {
			t.Errorf("%.40s: took %v", src, d)
		}
	}
}
