package zpl

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

// Run with -race: one shared Printer used from many goroutines, and many
// Printers rendering in parallel (sharing font caches and encoder state).
func TestConcurrentUse(t *testing.T) {
	label := func(i int) string {
		return fmt.Sprintf("^XA^CI28^FO20,20^A0N,40^FDOrder %d^FS^FO20,80^ADN,36,20^FDText^FS"+
			"^FO20,140^BY2^BCN,60,Y,N,N^FD%06d^FS^FO300,20^BQN,2,3^FDQA,%d^FS"+
			"^FO300,140^BXN,4,200^FD%d^FS^FO500,20^BD4^FDmaxi %d^FS^XZ", i, i, i, i, i)
	}
	shared, _ := NewPrinter(Options{LabelWidth: 4, LabelHeight: 2})
	if _, _, err := shared.Render(strings.NewReader("^XA^DFR:T.ZPL^FS^FO10,10^A0N,30^FN1^FS^XZ")); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for g := range 16 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for i := range 5 {
				imgs, _, err := shared.Render(strings.NewReader(fmt.Sprintf("^XA^XFR:T.ZPL^FN1^FD%d-%d^FS^XZ", g, i)))
				if err != nil || len(imgs) != 1 {
					errs <- fmt.Errorf("shared: %d images, %v", len(imgs), err)
				}
			}
		}()
		go func() {
			defer wg.Done()
			p, _ := NewPrinter(Options{LabelWidth: 4, LabelHeight: 2})
			for i := range 3 {
				want := render1With(t, label(g*10+i), Options{LabelWidth: 4, LabelHeight: 2})
				imgs, _, err := p.Render(strings.NewReader(label(g*10 + i)))
				if err != nil || len(imgs) != 1 || !same(imgs[0], want) {
					errs <- fmt.Errorf("parallel printer %d: %v", g, err)
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
