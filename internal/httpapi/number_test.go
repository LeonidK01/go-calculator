package httpapi

import (
	"math"
	"testing"
)

func TestPythonIntegerCompatibility(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want int64
	}{
		{"9223372036854775808", math.MinInt64}, {"18446744073709551617", 1},
		{"-18446744073709551617", -1}, {"1_000", 1000}, {"+000_010", 10},
		{"١٢٣", 123}, {"１２３", 123}, {"𝟙𝟚𝟛", 123},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			n, err := parseNumber(tc.raw)
			if err != nil || n != tc.want {
				t.Fatal(n, err)
			}
		})
	}
	for _, raw := range []string{"", "+", "-", "_1", "1_", "1__2", "0x12", "1.0", "+_1", "²", "a", "1 2"} {
		if _, err := parseNumber(raw); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
}
