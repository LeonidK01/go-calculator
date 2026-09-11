// Package calculator applies native arithmetic without serializing expensive calls.
package calculator

import (
	"context"
	"sync"
	"time"
)

// Native must implement thread-safe, pure addition and subtraction modulo 2^64.
type Native interface {
	Add(int64, int64) int64
	Sub(int64, int64) int64
}

// Observer records the two completed native calls without retaining samples.
type Observer interface {
	Observe(time.Duration, time.Duration)
}

// Calculator owns a consistent pair of totals and bounds concurrent native work.
type Calculator struct {
	native   Native
	observer Observer
	slots    chan struct{}
	mu       sync.Mutex
	sum, sub int64
}

// New constructs a calculator. concurrency must be positive.
func New(native Native, observer Observer, concurrency int) *Calculator {
	if concurrency < 1 {
		concurrency = 1
	}
	return &Calculator{native: native, observer: observer, slots: make(chan struct{}, concurrency)}
}

// Apply completes both native calls and commits their deltas before returning.
// A cancellation after admission cannot undo an already executing native call.
func (c *Calculator) Apply(ctx context.Context, num int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case c.slots <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-c.slots }()
	start := time.Now()
	add := c.native.Add(0, num)
	cTime := time.Since(start)
	start = time.Now()
	sub := c.native.Sub(0, num)
	rustTime := time.Since(start)
	c.mu.Lock()
	c.sum += add
	c.sub += sub
	c.mu.Unlock()
	c.observer.Observe(cTime, rustTime)
	return nil
}

// Totals returns a consistent snapshot, including wrapping int64 arithmetic.
func (c *Calculator) Totals() (int64, int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sum, c.sub
}
