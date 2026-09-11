package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"calculator/internal/metrics"
)

type fakeCalc struct {
	nums []int64
	err  error
}

func (c *fakeCalc) Apply(_ context.Context, n int64) error { c.nums = append(c.nums, n); return c.err }

func TestContract(t *testing.T) {
	for _, tc := range []struct {
		name, method, target string
		status               int
		body                 string
	}{
		{"positive", "POST", "/calc?num=7", 200, "ok"},
		{"negative", "POST", "/calc?num=-5", 200, "ok"},
		{"minimum", "POST", "/calc?num=-9223372036854775808", 200, "ok"},
		{"maximum", "POST", "/calc?num=9223372036854775807", 200, "ok"},
		{"encoded", "POST", "/calc?num=%20%2B42%20", 200, "ok"},
		{"duplicate", "POST", "/calc?num=1&num=2", 200, "ok"},
		{"missing", "POST", "/calc", 400, "missing"},
		{"empty", "POST", "/calc?num=", 400, "missing"},
		{"overflow", "POST", "/calc?num=9223372036854775808", 200, "ok"},
		{"decimal", "POST", "/calc?num=1.5", 400, "integer"},
		{"invalid", "POST", "/calc?num=%GG", 400, "invalid"},
		{"wrongmethod", "GET", "/calc?num=1", 405, "method"},
		{"unknown", "POST", "/other?num=1", 404, "not found"},
		{"slash", "POST", "/calc/?num=1", 404, "not found"},
		{"metrics", "GET", "/metrics", 200, "# TYPE calculator_http_rps gauge"},
		{"metricsmethod", "POST", "/metrics", 405, "method"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &fakeCalc{}
			h := New(c, metrics.New())
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(tc.method, tc.target, nil))
			if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.body) {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			if tc.status != 200 && len(c.nums) != 0 {
				t.Fatal("invalid request performed calculation")
			}
			if tc.name == "duplicate" && c.nums[0] != 1 {
				t.Fatal("must use first num")
			}
			if tc.status == 405 && w.Header().Get("Allow") == "" {
				t.Fatal("missing Allow")
			}
		})
	}
}

func TestCanceled(t *testing.T) {
	h := New(&fakeCalc{err: errors.New("canceled")}, metrics.New())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/calc?num=1", nil))
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
}

func FuzzQuery(f *testing.F) {
	for _, s := range []string{"num=1", "num=%GG", "num=-9223372036854775808", "num=1&num=2"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, query string) {
		r := httptest.NewRequest("POST", "/calc", nil)
		r.URL.RawQuery = query
		c := &fakeCalc{}
		w := httptest.NewRecorder()
		New(c, metrics.New()).ServeHTTP(w, r)
		if w.Code == 200 && len(c.nums) != 1 {
			t.Fatal("success without one calculation")
		}
		if w.Code != 200 && len(c.nums) != 0 {
			t.Fatal("failure changed state")
		}
	})
}
