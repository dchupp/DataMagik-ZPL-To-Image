// Package gs1 holds the GS1 rules shared by the GS1-aware symbologies
// (GS1-128, GS1 DataBar, composite components, EAN/UPC, ITF).
package gs1

// predefinedLength gives the total length (AI + data) of element strings
// whose AI starts with these two digits; they need no FNC1 separator (GS1
// General Specifications, Figure 7.8.5-2).
var predefinedLength = map[string]int{
	"00": 20, "01": 16, "02": 16, "03": 16, "04": 18,
	"11": 8, "12": 8, "13": 8, "14": 8, "15": 8, "16": 8, "17": 8, "18": 8, "19": 8,
	"20": 4,
	"31": 10, "32": 10, "33": 10, "34": 10, "35": 10, "36": 10,
	"41": 16,
}

// PredefinedLength returns the total element string length (AI + data)
// for AIs of predefined length, which are never followed by FNC1.
func PredefinedLength(ai string) (int, bool) {
	if len(ai) < 2 {
		return 0, false
	}
	n, ok := predefinedLength[ai[:2]]
	return n, ok
}

// AI parses a bracketed application identifier at the start of s:
// "(" or "[", 2–4 digits (spaces ignored), then the matching ")" or "]".
// It returns the AI digits and the number of bytes consumed, or ("", 0)
// when s does not start with an AI, so that other brackets stay data.
func AI(s string) (string, int) {
	if len(s) == 0 || (s[0] != '(' && s[0] != '[') {
		return "", 0
	}
	closer := byte(')')
	if s[0] == '[' {
		closer = ']'
	}
	var ai []byte
	for i := 1; i < len(s) && i <= 9; i++ {
		switch c := s[i]; {
		case c == closer:
			if len(ai) < 2 || len(ai) > 4 {
				return "", 0
			}
			return string(ai), i + 1
		case c == ' ':
		case c >= '0' && c <= '9':
			ai = append(ai, c)
		default:
			return "", 0
		}
	}
	return "", 0
}

// CheckDigit returns the GS1 Mod 10 check digit of the decimal digits in s
// (non-digits are skipped), weighting the rightmost digit 3.
func CheckDigit(s string) byte {
	sum, w := 0, 3
	for i := len(s) - 1; i >= 0; i-- {
		if c := s[i]; c >= '0' && c <= '9' {
			sum += int(c-'0') * w
			w = 4 - w
		}
	}
	return byte('0' + (10-sum%10)%10)
}
