package config

import (
	"math"
	"testing"
	"time"
)

func TestSeconds(t *testing.T) {
	for _, tc := range []struct {
		name        string
		n           float64
		zero, valid bool
	}{
		{"fraction", .1, false, true}, {"zero", 0, true, true}, {"forbiddenzero", 0, false, false},
		{"negative", -1, true, false}, {"nan", math.NaN(), true, false}, {"infinite", math.Inf(1), true, false},
		{"overflow", 1e20, true, false}, {"too small", 1e-12, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, err := Seconds(tc.n, tc.zero)
			if (err == nil) != tc.valid {
				t.Fatal(d, err)
			}
			if tc.name == "fraction" && d != 100*time.Millisecond {
				t.Fatal(d)
			}
		})
	}
}
