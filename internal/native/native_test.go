//go:build linux && cgo

package native

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func testLibrary(t testing.TB) Library {
	t.Helper()
	dir := os.Getenv("CALCULATOR_NATIVE_DIR")
	if dir == "" {
		t.Skip("set CALCULATOR_NATIVE_DIR to built bin directory")
	}
	l, err := Open(filepath.Join(dir, "libcalculator.so"), filepath.Join(dir, "libcalculator_rust.so"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := l.Close(); err != nil {
			t.Error(err)
		}
	})
	return l
}

func TestNativeArithmetic(t *testing.T) {
	l := testLibrary(t)
	for _, tc := range []struct{ a, b int64 }{{1, 2}, {-10, 3}, {math.MaxInt64, 1}, {math.MinInt64, 1}, {0, math.MinInt64}} {
		if got := l.Add(tc.a, tc.b); got != tc.a+tc.b {
			t.Fatalf("add: %d", got)
		}
		if got := l.Sub(tc.a, tc.b); got != tc.a-tc.b {
			t.Fatalf("sub: %d", got)
		}
	}
}

func TestLoadFailure(t *testing.T) {
	if _, err := Open("/missing/calculator.so", "/missing/rust.so"); err == nil {
		t.Fatal("missing libraries accepted")
	}
	dir := os.Getenv("CALCULATOR_NATIVE_DIR")
	if dir == "" {
		return
	}
	c := filepath.Join(dir, "libcalculator.so")
	r := filepath.Join(dir, "libcalculator_rust.so")
	for _, paths := range [][2]string{{c, "/missing/rust.so"}, {r, c}, {c, c}} {
		if _, err := Open(paths[0], paths[1]); err == nil {
			t.Fatal("missing symbol/library accepted")
		}
	}
}

func BenchmarkNative(b *testing.B) {
	l := testLibrary(b)
	b.Run("C", func(b *testing.B) {
		for b.Loop() {
			l.Add(0, 42)
		}
	})
	b.Run("Rust", func(b *testing.B) {
		for b.Loop() {
			l.Sub(0, 42)
		}
	})
}
