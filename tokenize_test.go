package zpl

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestTokenize_Commands(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []Command
	}{
		{
			name: "field with font and data",
			src:  "^XA^FO10,20^A0N,30,20^FDHello, world^FS^XZ",
			want: []Command{
				{Prefix: '^', Name: "XA", Offset: 0, End: 3},
				{Prefix: '^', Name: "FO", Params: []string{"10", "20"}, Offset: 3, End: 11},
				{Prefix: '^', Name: "A", Params: []string{"0N", "30", "20"}, Offset: 11, End: 21},
				{Prefix: '^', Name: "FD", Data: "Hello, world", Offset: 21, End: 36},
				{Prefix: '^', Name: "FS", Offset: 36, End: 39},
				{Prefix: '^', Name: "XZ", Offset: 39, End: 42},
			},
		},
		{
			name: "scalable font by name",
			src:  "^A@N,30,30,E:ARIAL.TTF",
			want: []Command{{Prefix: '^', Name: "A", Params: []string{"@N", "30", "30", "E:ARIAL.TTF"}, Offset: 0, End: 22}},
		},
		{
			name: "control command and comment",
			src:  "~HS^FXa, b^FS",
			want: []Command{
				{Prefix: '~', Name: "HS", Offset: 0, End: 3},
				{Prefix: '^', Name: "FX", Data: "a, b", Offset: 3, End: 10},
				{Prefix: '^', Name: "FS", Offset: 10, End: 13},
			},
		},
		{
			name: "hex escapes stay raw",
			src:  "^FH^FDA_1DB^FS",
			want: []Command{
				{Prefix: '^', Name: "FH", Offset: 0, End: 3},
				{Prefix: '^', Name: "FD", Data: "A_1DB", Offset: 3, End: 11},
				{Prefix: '^', Name: "FS", Offset: 11, End: 14},
			},
		},
		{
			name: "changed prefixes and delimiter are normalised",
			src:  "^CC+\n+CD;+FO1;2+CT#+FDa^b#HS",
			want: []Command{
				{Prefix: '^', Name: "CC", Params: []string{"+"}, Offset: 0, End: 5},
				{Prefix: '^', Name: "CD", Params: []string{";"}, Offset: 5, End: 9},
				{Prefix: '^', Name: "FO", Params: []string{"1", "2"}, Offset: 9, End: 15},
				{Prefix: '^', Name: "CT", Params: []string{"#"}, Offset: 15, End: 19},
				{Prefix: '^', Name: "FD", Data: "a^b", Offset: 19, End: 25},
				{Prefix: '~', Name: "HS", Offset: 25, End: 28},
			},
		},
		{
			name: "graphic payload",
			src:  "^GFA,2,2,1,FF00^FS",
			want: []Command{
				{Prefix: '^', Name: "GF", Params: []string{"A", "2", "2", "1"}, Data: "FF00", Offset: 0, End: 15},
				{Prefix: '^', Name: "FS", Offset: 15, End: 18},
			},
		},
		{
			name: "binary download",
			src:  "~DYR:X,B,T,2,0,^~^XA",
			want: []Command{
				{Prefix: '~', Name: "DY", Params: []string{"R:X", "B", "T", "2", "0"}, Data: "^~", Offset: 0, End: 17},
				{Prefix: '^', Name: "XA", Offset: 17, End: 20},
			},
		},
		{
			name: "serial number start value",
			src:  "^SN001,1,Y",
			want: []Command{{Prefix: '^', Name: "SN", Params: []string{"001", "1", "Y"}, Data: "001", Offset: 0, End: 10}},
		},
		{
			name: "junk before the first command",
			src:  "hello ^XA",
			want: []Command{{Prefix: '^', Name: "XA", Offset: 6, End: 9}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Tokenize(strings.NewReader(tt.src))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

func TestTokenize_ReadError(t *testing.T) {
	if _, err := Tokenize(errReader{}); err == nil {
		t.Fatal("want read error")
	}
}

// checkTokens verifies the Offset/End invariants and that the commands
// plus the bytes between them reproduce src.
func checkTokens(t *testing.T, src []byte, cmds []Command) {
	t.Helper()
	var rebuilt bytes.Buffer
	prev := 0
	for i, c := range cmds {
		if c.Offset < prev || c.End <= c.Offset || c.End > len(src) {
			t.Fatalf("command %d (%c%s) has bad span [%d,%d) after %d, len %d", i, c.Prefix, c.Name, c.Offset, c.End, prev, len(src))
		}
		if c.Prefix != '^' && c.Prefix != '~' {
			t.Fatalf("command %d prefix %q", i, c.Prefix)
		}
		rebuilt.Write(src[prev:c.Offset])
		rebuilt.Write(src[c.Offset:c.End])
		prev = c.End
	}
	rebuilt.Write(src[prev:])
	if !bytes.Equal(rebuilt.Bytes(), src) {
		t.Fatal("commands do not reproduce the source")
	}
}

func TestTokenize_GoldenSpans(t *testing.T) {
	files, _ := filepath.Glob("testdata/golden/*.zpl")
	files = append(files, "testdata/samples/shipping_label.zpl")
	for _, file := range files {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		cmds, err := Tokenize(bytes.NewReader(src))
		if err != nil {
			t.Fatal(err)
		}
		checkTokens(t, src, cmds)
		// In well-formed files only whitespace lies between commands.
		prev := 0
		for _, c := range cmds {
			if gap := strings.TrimSpace(string(src[prev:c.Offset])); gap != "" {
				t.Errorf("%s: non-space gap %q before byte %d", file, gap, c.Offset)
			}
			prev = c.End
		}
	}
}

// FuzzTokenize checks that Tokenize never panics and that command spans are
// ordered, in bounds and reproduce the input.
func FuzzTokenize(f *testing.F) {
	files, _ := filepath.Glob("testdata/golden/*.zpl")
	for _, file := range files {
		if b, err := os.ReadFile(file); err == nil {
			f.Add(b)
		}
	}
	f.Add([]byte("^XA^CC+\n+CD;+FO1;2+CT#+FDa^b#HS+XZ"))
	f.Add([]byte("^GFB,4,4,2,\x00^~\xff^FS~DYR:X,B,T,9,0,ab"))
	f.Add([]byte("^A"))
	f.Add([]byte("^CC"))
	f.Fuzz(func(t *testing.T, src []byte) {
		cmds, err := Tokenize(bytes.NewReader(src))
		if err != nil {
			t.Fatal(err)
		}
		checkTokens(t, src, cmds)
	})
}
