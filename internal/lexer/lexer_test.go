package lexer

import (
	"reflect"
	"testing"
)

func keys(cmds []Command) []string {
	var out []string
	for _, c := range cmds {
		out = append(out, c.Key()+"|"+string(c.Params))
	}
	return out
}

func TestBasic(t *testing.T) {
	got := keys(All([]byte("junk^XA\r\n^FO10,20^A0N,30,30^FDHello, world~ ^FS~HS^XZ")))
	want := []string{"^XA|", "^FO|10,20", "^A|0N,30,30", "^FD|Hello, world", "~ |", "^FS|", "~HS|", "^XZ|"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestPrefixChanges(t *testing.T) {
	cmds := All([]byte("^XA^CC++CD;+FO1;2+CT#+FDa^b#HS+XZ"))
	got := keys(cmds)
	want := []string{"^XA|", "^CC|+", "^CD|;", "^FO|1;2", "^CT|#", "^FD|a^b", "~HS|", "^XZ|"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
	if cmds[2].Delim != ',' || cmds[3].Delim != ';' {
		t.Error("delimiter not tracked")
	}
}

func TestBinaryGF(t *testing.T) {
	data := []byte{'^', '~', 0x00, 0xFF}
	src := append([]byte("^XA^GFB,4,4,2,"), data...)
	src = append(src, "^FS^XZ"...)
	cmds := All(src)
	if string(cmds[1].Params) != "B,4,4,2,"+string(data) {
		t.Errorf("binary payload %q", cmds[1].Params)
	}
	if cmds[2].Key() != "^FS" {
		t.Errorf("next command %s", cmds[2].Key())
	}
}

func TestBinaryDY(t *testing.T) {
	cmds := All([]byte("~DYR:X,B,T,3,0,^~\n^XA^XZ"))
	if string(cmds[0].Params) != "R:X,B,T,3,0,^~\n" || cmds[1].Key() != "^XA" {
		t.Errorf("got %q then %s", cmds[0].Params, cmds[1].Key())
	}
}

func TestOffsets(t *testing.T) {
	src := []byte("junk^XA\r\n^FO10,\r\n20^A0N,30^GFB,2,2,1,^~^FS  ~DYR:X,B,T,1,0,^\n^XZ")
	cmds := All(src)
	want := []string{"^XA\r\n", "^FO10,\r\n20", "^A0N,30", "^GFB,2,2,1,^~", "^FS  ", "~DYR:X,B,T,1,0,^", "^XZ"}
	if len(cmds) != len(want) {
		t.Fatalf("got %d commands", len(cmds))
	}
	for i, c := range cmds {
		if got := string(src[c.Offset:c.End]); got != want[i] {
			t.Errorf("command %d = %q, want %q", i, got, want[i])
		}
	}
	if a := cmds[2]; string(src[a.ParamStart:a.End]) != "0N,30" {
		t.Errorf("^A params start at %q", src[a.ParamStart:a.End])
	}
	if fo := cmds[1]; string(src[fo.ParamStart:fo.End]) != "10,\r\n20" {
		t.Errorf("^FO params start at %q", src[fo.ParamStart:fo.End])
	}
}
