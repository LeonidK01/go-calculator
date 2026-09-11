// Package metrics exposes bounded-memory Prometheus metrics for completed seconds.
package metrics

import (
	"fmt"
	"io"
	"math"
	"math/bits"
	"sync"
	"time"
)

const bins = 2048

type second struct {
	epoch int64
	rps   uint64
	hist  [2][bins]uint64
}

// Collector retains 60 completed seconds plus the current, incomplete second.
// Zero value is not ready for use; construct with New.
type Collector struct {
	mu       sync.Mutex
	now      func() time.Time
	window   [61]second
	requests uint64
	counts   [2]uint64
	sums     [2]float64
}

// New creates a collector using the wall clock for one-second windows.
func New() *Collector { return &Collector{now: time.Now} }

func (c *Collector) current() *second {
	epoch := c.now().Unix()
	s := &c.window[(epoch%61+61)%61]
	if s.epoch != epoch {
		*s = second{epoch: epoch}
	}
	return s
}

// Request counts every request reaching the application, including /metrics and errors.
func (c *Collector) Request() {
	c.mu.Lock()
	c.current().rps++
	c.requests++
	c.mu.Unlock()
}

// bucket uses 32 subdivisions per power of two; the upper-bound error is <3.125%.
// Durations below 64ns have exact integer nanosecond resolution.
func bucket(d time.Duration) int {
	n := uint64(max(d, 0))
	if n < 64 {
		return int(n)
	}
	e := bits.Len64(n) - 1
	return e*32 + int((n-uint64(1)<<e)>>(e-5))
}

func upper(i int) float64 {
	if i < 64 {
		return float64(i) / 1e9
	}
	e := i / 32
	return float64((uint64(1)<<e)+uint64(i%32+1)*(uint64(1)<<(e-5))-1) / 1e9
}

// Observe records elapsed time around each real native call, including cgo overhead.
func (c *Collector) Observe(cTime, rustTime time.Duration) {
	c.mu.Lock()
	s := c.current()
	for lib, d := range [2]time.Duration{cTime, rustTime} {
		s.hist[lib][bucket(d)]++
		c.counts[lib]++
		c.sums[lib] += max(d.Seconds(), 0)
	}
	c.mu.Unlock()
}

func quantile(h *[bins]uint64, q float64) float64 {
	var count uint64
	for _, n := range h {
		count += n
	}
	if count == 0 {
		return math.NaN()
	}
	rank := uint64(math.Ceil(float64(count) * q))
	var cumulative uint64
	for i, n := range h {
		cumulative += n
		if cumulative >= rank {
			return upper(i)
		}
	}
	return math.NaN()
}

// WritePrometheus writes a consistent snapshot in Prometheus text format 0.0.4.
// age_seconds=1 is the newest completed second. Count and sum are lifetime counters.
func (c *Collector) WritePrometheus(w io.Writer) error {
	var rps [60]uint64
	var h [2][bins]uint64
	c.mu.Lock()
	now := c.now().Unix()
	for age := int64(1); age <= 60; age++ {
		s := &c.window[((now-age)%61+61)%61]
		if s.epoch != now-age {
			continue
		}
		rps[age-1] = s.rps
		for lib := range h {
			for i, n := range s.hist[lib] {
				h[lib][i] += n
			}
		}
	}
	requests, counts, sums := c.requests, c.counts, c.sums
	c.mu.Unlock()
	// Encode after releasing the lock; a slow scrape cannot block observations.
	if _, err := fmt.Fprintf(w, "# HELP calculator_http_requests_total Requests reaching the application.\n# TYPE calculator_http_requests_total counter\ncalculator_http_requests_total %d\n# HELP calculator_http_rps Requests in each completed second before the snapshot.\n# TYPE calculator_http_rps gauge\n", requests); err != nil {
		return err
	}
	for i, n := range rps {
		if _, err := fmt.Fprintf(w, "calculator_http_rps{age_seconds=\"%d\"} %d\n", i+1, n); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "# HELP calculator_rps_window_end_timestamp_seconds Exclusive end of the completed-second window.\n# TYPE calculator_rps_window_end_timestamp_seconds gauge\ncalculator_rps_window_end_timestamp_seconds %d\n# HELP calculator_native_call_duration_seconds Native call latency; quantiles over 60 completed seconds, upper-bound error less than 3.125 percent.\n# TYPE calculator_native_call_duration_seconds summary\n", now); err != nil {
		return err
	}
	for lib, name := range [2]string{"c", "rust"} {
		for _, q := range [2]float64{0.95, 0.99} {
			if _, err := fmt.Fprintf(w, "calculator_native_call_duration_seconds{library=\"%s\",quantile=\"%g\"} %g\n", name, q, quantile(&h[lib], q)); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "calculator_native_call_duration_seconds_sum{library=\"%s\"} %g\ncalculator_native_call_duration_seconds_count{library=\"%s\"} %d\n", name, sums[lib], name, counts[lib]); err != nil {
			return err
		}
	}
	return nil
}
