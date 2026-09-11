// Package generator produces bounded concurrent HTTP load with connection reuse.
package generator

import (
	"context"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Config defines the worker count, endpoint and per-worker timing.
type Config struct {
	URL               string
	Workers           int
	Interval, Timeout time.Duration
}

// Stats counts completed requests; cancellation of an in-flight request is excluded.
type Stats struct{ OK, Errors uint64 }

// Run starts workers, waits for cancellation, and joins every worker before returning.
func Run(ctx context.Context, cfg Config) (Stats, error) {
	if cfg.Workers < 1 || cfg.Interval < 0 || cfg.Timeout <= 0 {
		return Stats{}, fmt.Errorf("workers and timeout must be positive; interval must be nonnegative")
	}
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return Stats{}, fmt.Errorf("parse endpoint: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.Fragment != "" || u.User != nil {
		return Stats{}, fmt.Errorf("endpoint must be an HTTP(S) URL without credentials or fragment")
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return Stats{}, fmt.Errorf("parse endpoint query: %w", err)
	}
	var endpoints [201]string
	for i := range endpoints {
		query.Set("num", strconv.Itoa(i-100))
		u.RawQuery = query.Encode()
		endpoints[i] = u.String()
	}
	transport := &http.Transport{
		Proxy:        http.ProxyFromEnvironment,
		DialContext:  (&net.Dialer{Timeout: cfg.Timeout, KeepAlive: 30 * time.Second}).DialContext,
		MaxIdleConns: cfg.Workers, MaxIdleConnsPerHost: cfg.Workers, MaxConnsPerHost: cfg.Workers,
		IdleConnTimeout: 90 * time.Second, TLSHandshakeTimeout: cfg.Timeout,
		ResponseHeaderTimeout: cfg.Timeout, ForceAttemptHTTP2: true,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: cfg.Timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	var ok, failures atomic.Uint64
	var wg sync.WaitGroup
	for range cfg.Workers {
		wg.Go(func() {
			timer := time.NewTimer(time.Hour)
			if !timer.Stop() {
				<-timer.C
			}
			defer timer.Stop()
			for ctx.Err() == nil {
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoints[rand.IntN(len(endpoints))], nil)
				if err != nil {
					failures.Add(1)
					return
				}
				resp, err := client.Do(req)
				success := false
				if err == nil {
					// Bound work for an unexpected endpoint; oversized bodies are errors.
					n, readErr := io.Copy(io.Discard, io.LimitReader(resp.Body, (1<<20)+1))
					closeErr := resp.Body.Close()
					success = resp.StatusCode >= 200 && resp.StatusCode < 300 && readErr == nil && closeErr == nil && n <= 1<<20
				}
				if success {
					ok.Add(1)
				} else if ctx.Err() == nil {
					failures.Add(1)
				}
				if cfg.Interval > 0 {
					timer.Reset(cfg.Interval)
					select {
					case <-ctx.Done():
						return
					case <-timer.C:
					}
				}
			}
		})
	}
	wg.Wait()
	return Stats{OK: ok.Load(), Errors: failures.Load()}, nil
}
