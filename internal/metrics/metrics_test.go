package metrics

import (
	"bytes"
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWindow(t *testing.T) {
	now := int64(1000)
	c := New()
	c.now = func() time.Time { return time.Unix(now, 0) }
	for range 3 {
		c.Request()
		c.Observe(time.Millisecond, 2*time.Millisecond)
	}
	var out bytes.Buffer
	if err := c.WritePrometheus(&out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "quantile=\"0.95\"} NaN") {
		t.Fatal("current second must be excluded")
	}
	now++
	out.Reset()
	if err := c.WritePrometheus(&out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "calculator_http_rps{age_seconds=\"1\"} 3\n") {
		t.Fatal(out.String())
	}
	if strings.Count(out.String(), "calculator_http_rps{age_seconds=") != 60 {
		t.Fatal("expected exactly 60 RPS values")
	}
	now = 1060
	out.Reset()
	if err := c.WritePrometheus(&out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "age_seconds=\"60\"} 3\n") {
		t.Fatal("oldest bucket lost early")
	}
	now = 1061
	c.Request()
	out.Reset()
	if err := c.WritePrometheus(&out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "age_seconds=\"60\"} 3\n") || !strings.Contains(out.String(), "quantile=\"0.99\"} NaN") {
		t.Fatal("expired samples retained")
	}
	if !strings.Contains(out.String(), "calculator_native_call_duration_seconds_count{library=\"c\"} 3\n") {
		t.Fatal("lifetime count expired")
	}
	now += 600
	out.Reset()
	if err := c.WritePrometheus(&out); err != nil {
		t.Fatal(err)
	}
	if strings.Count(out.String(), "} 0\n") != 60 {
		t.Fatal("idle gaps must be zero")
	}
}

func TestHistogramAccuracy(t *testing.T) {
	for d := time.Duration(1); d < time.Duration(math.MaxInt64/2); d += max(1, d/97) {
		got := upper(bucket(d)) * 1e9
		if got+1 < float64(d) || got > float64(d)*1.03125+1 {
			t.Fatalf("duration=%d upper=%g", d, got)
		}
	}
	var h [bins]uint64
	for i := 1; i <= 100; i++ {
		h[bucket(time.Duration(i)*time.Millisecond)]++
	}
	for _, q := range []float64{.95, .99} {
		if got := quantile(&h, q); got < q/10 || got > q/10*1.03125 {
			t.Fatalf("q=%g got=%g", q, got)
		}
	}
	if bucket(-1) != 0 {
		t.Fatal("negative duration")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disconnected") }
func TestWriteError(t *testing.T) {
	if New().WritePrometheus(failingWriter{}) == nil {
		t.Fatal("missing error")
	}
}

func TestConcurrentScrapes(t *testing.T) {
	c := New()
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 100 {
				c.Request()
				c.Observe(time.Microsecond, 2*time.Microsecond)
			}
		})
	}
	for range 2 {
		wg.Go(func() {
			for range 10 {
				var b bytes.Buffer
				if err := c.WritePrometheus(&b); err != nil {
					t.Error(err)
				}
			}
		})
	}
	wg.Wait()
	if c.requests != 800 || c.counts != [2]uint64{800, 800} {
		t.Fatal("lost metrics")
	}
}

func BenchmarkObserve(b *testing.B) {
	c := New()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			c.Observe(time.Microsecond, time.Microsecond)
		}
	})
}
