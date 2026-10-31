package encoding

import "testing"

func TestDecode(t *testing.T) {
	cases := []struct {
		id   int
		in   string
		want string
	}{
		{0, "A#$@[", "A#$@["},
		{6, "[\\]{|}~", "ÄÖÜäöüß"},
		{12, "\\", "¥"},
		{0, "\x82", "é"},
		{27, "\x80", "€"},
		{28, "café", "café"},
		{29, "\x00A\x00\xe9", "Aé"},
		{30, "A\x00\xe9\x00", "Aé"},
		{33, "\xc0", "А"},
		{15, "\x82\xa0", "あ"},
	}
	for _, c := range cases {
		if got := (Charset{ID: c.id}).Decode([]byte(c.in)); got != c.want {
			t.Errorf("^CI%d %q: got %q want %q", c.id, c.in, got, c.want)
		}
	}
	cs := Charset{ID: 0, Remap: map[byte]byte{'A': 'B'}}
	if got := cs.Decode([]byte("AA")); got != "BB" {
		t.Errorf("remap: %q", got)
	}
}
