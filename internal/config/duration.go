// Package config validates command-line values shared by both programs.
package config

import (
	"fmt"
	"math"
	"time"
)

// Seconds converts finite seconds to a duration without overflow or truncation to zero.
func Seconds(value float64, allowZero bool) (time.Duration, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value >= float64(math.MaxInt64)/1e9 {
		return 0, fmt.Errorf("seconds must be finite, nonnegative and less than %.0f", float64(math.MaxInt64)/1e9)
	}
	d := time.Duration(value * 1e9)
	if d == 0 && (!allowZero || value != 0) {
		return 0, fmt.Errorf("duration must be at least 1ns")
	}
	return d, nil
}
