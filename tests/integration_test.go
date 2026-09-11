//go:build integration && linux && cgo

package tests

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"calculator/internal/calculator"
	"calculator/internal/generator"
	"calculator/internal/httpapi"
	"calculator/internal/metrics"
	"calculator/internal/native"
)

func libraries(t testing.TB) native.Library {
	t.Helper()
	dir := os.Getenv("CALCULATOR_NATIVE_DIR")
	if dir == "" {
		t.Fatal("integration tests require CALCULATOR_NATIVE_DIR")
	}
	l, err := native.Open(filepath.Join(dir, "libcalculator.so"), filepath.Join(dir, "libcalculator_rust.so"))
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

func TestRealHTTPAndGenerator(t *testing.T) {
	stats := metrics.New()
	calc := calculator.New(libraries(t), stats, 4)
	s := httptest.NewServer(httpapi.New(calc, stats))
	defer s.Close()
	client := &http.Client{Timeout: time.Second}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 50 {
				resp, err := client.Post(s.URL+"/calc?num=3", "", nil)
				if err != nil {
					t.Error(err)
					return
				}
				body, err := io.ReadAll(resp.Body)
				closeErr := resp.Body.Close()
				if err != nil || closeErr != nil || resp.StatusCode != 200 || string(body) != "ok" {
					t.Error("bad response")
				}
			}
		})
	}
	wg.Wait()
	if sum, sub := calc.Totals(); sum != 1200 || sub != -1200 {
		t.Fatalf("totals %d %d", sum, sub)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	result, err := generator.Run(ctx, generator.Config{URL: s.URL + "/calc", Workers: 8, Timeout: time.Second})
	if err != nil || result.OK == 0 || result.Errors != 0 {
		t.Fatal(result, err)
	}
	s.Close() // Wait for requests whose response was canceled before inspecting totals.
	if sum, sub := calc.Totals(); sum != -sub {
		t.Fatalf("totals %d %d", sum, sub)
	}
}

func TestProcessGracefulShutdown(t *testing.T) {
	dir := os.Getenv("CALCULATOR_NATIVE_DIR")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(dir, "calculator_server"), "--host", "127.0.0.1", "--port", fmt.Sprint(port), "--interval", "0.1")
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		if !waited {
			cancel()
			_ = cmd.Wait()
		}
	}()
	client := &http.Client{Timeout: time.Second}
	endpoint := fmt.Sprintf("http://127.0.0.1:%d", port)
	ready := false
	for range 100 {
		resp, err := client.Get(endpoint + "/metrics")
		if err == nil {
			if err := resp.Body.Close(); err != nil {
				t.Fatal(err)
			}
			ready = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ready {
		t.Fatal("server did not start")
	}
	for _, n := range []int{1, 2, 3} {
		resp, err := client.Post(endpoint+"/calc?num="+fmt.Sprint(n), "", nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := resp.Body.Close(); err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 200 {
			t.Fatal(resp.StatusCode)
		}
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	err = cmd.Wait()
	waited = true
	if err != nil {
		t.Fatalf("%v: %s", err, output.String())
	}
	if !strings.Contains(output.String(), "[final] sum=6 sub=-6") {
		t.Fatal(output.String())
	}
}

type noMetrics struct{}

func (noMetrics) Observe(_, _ time.Duration) {}

// BenchmarkCalculation compares lock placement with identical real native work.
// It is a service throughput microbenchmark, not a Python-vs-Go HTTP comparison.
func BenchmarkCalculation(b *testing.B) {
	l := libraries(b)
	b.Run("Serialized", func(b *testing.B) {
		var mu sync.Mutex
		var sum, sub int64
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				mu.Lock()
				sum = l.Add(sum, 1)
				sub = l.Sub(sub, 1)
				mu.Unlock()
			}
		})
	})
	b.Run("Concurrent", func(b *testing.B) {
		c := calculator.New(l, noMetrics{}, runtime.GOMAXPROCS(0))
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				if err := c.Apply(context.Background(), 1); err != nil {
					b.Error(err)
				}
			}
		})
	})
	b.Run("ConcurrentWithMetrics", func(b *testing.B) {
		m := metrics.New()
		c := calculator.New(l, m, runtime.GOMAXPROCS(0))
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				m.Request()
				if err := c.Apply(context.Background(), 1); err != nil {
					b.Error(err)
				}
			}
		})
	})
}
