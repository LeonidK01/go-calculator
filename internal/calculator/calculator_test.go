package calculator

import (
	"context"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type arithmetic struct{}

func (arithmetic) Add(a, b int64) int64 { return a + b }
func (arithmetic) Sub(a, b int64) int64 { return a - b }

type observer struct{ count atomic.Int64 }

func (o *observer) Observe(_, _ time.Duration) { o.count.Add(1) }

func TestTotals(t *testing.T) {
	for _, tc := range []struct {
		name     string
		nums     []int64
		sum, sub int64
	}{
		{"mixed", []int64{10, -3, 0, 2}, 9, -9},
		{"overflow", []int64{math.MaxInt64, 1}, math.MinInt64, math.MinInt64},
		{"minimum", []int64{math.MinInt64, -1}, math.MaxInt64, -math.MaxInt64},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := &observer{}
			c := New(arithmetic{}, o, 2)
			for _, n := range tc.nums {
				if err := c.Apply(context.Background(), n); err != nil {
					t.Fatal(err)
				}
			}
			if a, b := c.Totals(); a != tc.sum || b != tc.sub {
				t.Fatalf("totals=(%d,%d)", a, b)
			}
			if o.count.Load() != int64(len(tc.nums)) {
				t.Fatal("missing observations")
			}
		})
	}
}

func TestConcurrent(t *testing.T) {
	o := &observer{}
	c := New(arithmetic{}, o, 4)
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			for range 100 {
				if err := c.Apply(context.Background(), 1); err != nil {
					t.Error(err)
				}
				a, b := c.Totals()
				if a != -b {
					t.Error("torn totals")
				}
			}
		})
	}
	wg.Wait()
	if a, b := c.Totals(); a != 3200 || b != -3200 || o.count.Load() != 3200 {
		t.Fatalf("lost updates: %d %d", a, b)
	}
}

type blocking struct {
	entered chan struct{}
	release chan struct{}
}

func (b blocking) Add(a, n int64) int64 { b.entered <- struct{}{}; <-b.release; return a + n }
func (blocking) Sub(a, n int64) int64   { return a - n }

func TestAdmissionAndCancellation(t *testing.T) {
	b := blocking{make(chan struct{}, 2), make(chan struct{})}
	c := New(b, &observer{}, 1)
	done := make(chan error, 1)
	go func() { done <- c.Apply(context.Background(), 1) }()
	<-b.entered
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := c.Apply(ctx, 9); err != context.DeadlineExceeded {
		t.Errorf("queued request: %v", err)
	}
	close(b.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if a, _ := c.Totals(); a != 1 {
		t.Fatal("canceled request changed state")
	}
	if err := c.Apply(ctx, 9); err != context.DeadlineExceeded {
		t.Fatal("pre-canceled request admitted")
	}
}

func TestNativeCallsCanOverlap(t *testing.T) {
	b := blocking{make(chan struct{}, 2), make(chan struct{})}
	c := New(b, &observer{}, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			if err := c.Apply(context.Background(), 1); err != nil {
				t.Error(err)
			}
		})
	}
	defer wg.Wait()
	defer close(b.release)
	for range 2 {
		select {
		case <-b.entered:
		case <-time.After(time.Second):
			t.Fatal("native calls serialized")
		}
	}
}
