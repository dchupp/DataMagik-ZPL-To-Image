package gs1

import "testing"

func TestCheckDigit(t *testing.T) {
	for in, want := range map[string]byte{"400638133393": '1', "0001234567890": '5', "629104150021": '3', "1234567": '0'} {
		if got := CheckDigit(in); got != want {
			t.Errorf("CheckDigit(%s) = %c, want %c", in, got, want)
		}
	}
}

func TestAI(t *testing.T) {
	cases := map[string][2]any{
		"(01)123": {"01", 4}, "[3103]x": {"3103", 6}, "( 21 )A": {"21", 6},
		"(C)D": {"", 0}, "(1)": {"", 0}, "(12345)": {"", 0}, "(21": {"", 0}, "x(21)": {"", 0},
	}
	for in, want := range cases {
		ai, n := AI(in)
		if ai != want[0] || n != want[1] {
			t.Errorf("AI(%q) = %q,%d want %v", in, ai, n, want)
		}
	}
}

func TestPredefinedLength(t *testing.T) {
	if n, ok := PredefinedLength("3103"); !ok || n != 10 {
		t.Errorf("3103: %d %v", n, ok)
	}
	if _, ok := PredefinedLength("10"); ok {
		t.Error("AI 10 is variable length")
	}
}
