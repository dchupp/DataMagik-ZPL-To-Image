package notocjk

import (
	"bytes"
	"testing"
)

// The module has no dependencies, so this only checks the embedded data;
// rendering with it is tested in the internal/verify module.
func TestEmbeddedFont(t *testing.T) {
	if len(Regular) < 10<<20 {
		t.Fatalf("font is %d bytes; embed failed?", len(Regular))
	}
	if !bytes.HasPrefix(Regular, []byte("OTTO")) {
		t.Errorf("not an OpenType CFF font: % x", Regular[:4])
	}
}
