package httpapi

import (
	"fmt"
	"math/big"
	"strconv"
	"unicode"
)

// parseNumber keeps a cheap int64 path and preserves Python int -> ctypes.c_int64
// semantics for larger decimal integers, digit separators and Unicode Nd digits.
func parseNumber(raw string) (int64, error) {
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return n, nil
	}
	if raw == "" {
		return 0, fmt.Errorf("empty integer")
	}
	normalized := make([]byte, 0, len(raw))
	if raw[0] == '+' || raw[0] == '-' {
		normalized = append(normalized, raw[0])
		raw = raw[1:]
	}
	previousDigit := false
	for _, r := range raw {
		if r == '_' && previousDigit {
			previousDigit = false
			continue
		}
		d, ok := decimalDigit(r)
		if !ok {
			return 0, fmt.Errorf("invalid decimal integer")
		}
		normalized = append(normalized, '0'+d)
		previousDigit = true
	}
	if !previousDigit {
		return 0, fmt.Errorf("missing trailing digit")
	}
	n, ok := new(big.Int).SetString(string(normalized), 10)
	if !ok {
		return 0, fmt.Errorf("invalid decimal integer")
	}
	low := n.Uint64() // Uint64 returns the low bits of the absolute value.
	if n.Sign() < 0 {
		low = 0 - low
	}
	return int64(low), nil
}

func decimalDigit(r rune) (byte, bool) {
	if r >= '0' && r <= '9' {
		return byte(r - '0'), true
	}
	if r < 128 {
		return 0, false
	}
	for _, rr := range unicode.Digit.R16 {
		if uint32(r) >= uint32(rr.Lo) && uint32(r) <= uint32(rr.Hi) && (uint32(r)-uint32(rr.Lo))%uint32(rr.Stride) == 0 {
			return byte((uint32(r) - uint32(rr.Lo)) / uint32(rr.Stride) % 10), true
		}
	}
	for _, rr := range unicode.Digit.R32 {
		if uint32(r) >= rr.Lo && uint32(r) <= rr.Hi && (uint32(r)-rr.Lo)%rr.Stride == 0 {
			return byte((uint32(r) - rr.Lo) / rr.Stride % 10), true
		}
	}
	return 0, false
}
