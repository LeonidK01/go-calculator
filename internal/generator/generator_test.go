package generator

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestRequestsAndStats(t *testing.T) {
	for _, status := range []int{200, 500, 302} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			var calls atomic.Int64
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n, err := strconv.Atoi(r.URL.Query().Get("num"))
				if err != nil || n < -100 || n > 100 || r.Method != "POST" || r.URL.Query().Get("keep") != "yes" {
					t.Error("invalid generated request")
				}
				calls.Add(1)
				w.WriteHeader(status)
			}))
			defer s.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
			defer cancel()
			stats, err := Run(ctx, Config{URL: s.URL + "/calc?keep=yes&num=999", Workers: 3, Timeout: time.Second, Interval: time.Millisecond})
			if err != nil || calls.Load() == 0 {
				t.Fatal(stats, err)
			}
			if status == 200 {
				if stats.OK == 0 || stats.Errors != 0 {
					t.Fatal(stats)
				}
			} else if stats.Errors == 0 || stats.OK != 0 {
				t.Fatal(stats)
			}
		})
	}
}

func TestValidation(t *testing.T) {
	for _, u := range []string{"", "://", "ftp://host", "http:///calc", "http://user:pass@host", "http://host/#x", "http://host/?x=%GG"} {
		if _, err := Run(context.Background(), Config{URL: u, Workers: 1, Timeout: time.Second}); err == nil {
			t.Fatalf("accepted %q", u)
		}
	}
	if _, err := Run(context.Background(), Config{URL: "http://host", Workers: 0, Timeout: time.Second}); err == nil {
		t.Fatal("zero workers")
	}
}

func TestCancellationInterruptsRequest(t *testing.T) {
	entered := make(chan struct{}, 1)
	s := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { entered <- struct{}{}; <-r.Context().Done() }))
	defer s.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan Stats, 1)
	go func() {
		stats, err := Run(ctx, Config{URL: s.URL, Workers: 1, Timeout: time.Minute})
		if err != nil {
			t.Error(err)
		}
		done <- stats
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("no request")
	}
	cancel()
	select {
	case stats := <-done:
		if stats != (Stats{}) {
			t.Fatal(stats)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not stop worker")
	}
}

func TestTimeout(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	stats, err := Run(ctx, Config{URL: s.URL, Workers: 1, Timeout: 10 * time.Millisecond})
	if err != nil || stats.Errors == 0 || stats.OK != 0 {
		t.Fatal(stats, err)
	}
}
