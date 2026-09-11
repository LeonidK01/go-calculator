// Package httpapi defines the calculator HTTP contract.
package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"calculator/internal/metrics"
)

// Calculator applies one number before the HTTP response is sent.
type Calculator interface {
	Apply(context.Context, int64) error
}

// New builds an HTTP handler with exact routing and request accounting.
func New(calc Calculator, stats *metrics.Collector) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stats.Request()
		switch r.URL.Path {
		case "/calc":
			if r.Method != http.MethodPost {
				w.Header().Set("Allow", "POST")
				respond(w, 405, "method not allowed")
				return
			}
			query, err := url.ParseQuery(r.URL.RawQuery)
			if err != nil {
				respond(w, 400, "invalid query string")
				return
			}
			raw := query.Get("num")
			if raw == "" {
				respond(w, 400, "missing 'num' query parameter")
				return
			}
			num, err := parseNumber(strings.TrimSpace(raw))
			if err != nil {
				respond(w, 400, "'num' must be an integer")
				return
			}
			if err := calc.Apply(r.Context(), num); err != nil {
				respond(w, 503, "request canceled")
				return
			}
			respond(w, 200, "ok")
		case "/metrics":
			if r.Method != http.MethodGet {
				w.Header().Set("Allow", "GET")
				respond(w, 405, "method not allowed")
				return
			}
			var body bytes.Buffer
			if err := stats.WritePrometheus(&body); err != nil {
				respond(w, 500, "metrics encoding failed")
				return
			}
			w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
			w.Header().Set("Content-Length", strconv.Itoa(body.Len()))
			// A disconnected client has no further response channel.
			_, _ = w.Write(body.Bytes())
		default:
			respond(w, 404, "not found")
		}
	})
}

func respond(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	// A disconnected client has no further response channel.
	_, _ = w.Write([]byte(body))
}
