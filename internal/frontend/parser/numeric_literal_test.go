package parser

import "testing"

func TestParseNumericLiteralRadix(t *testing.T) {
	for text, want := range map[string]float64{
		"0xc0":   192,
		"0Xff":   255,
		"0b1010": 10,
		"0B11":   3,
		"0o17":   15,
		"0O10":   8,
		"42":     42,
	} {
		if got := parseNumericLiteral(text); got != want {
			t.Fatalf("parseNumericLiteral(%q) = %v, want %v", text, got, want)
		}
	}
}
