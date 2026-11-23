// Package middleware holds what applies to every request of the public
// server, around the API: request metrics (Metrics), panic recovery (Recover)
// and fixed response headers (Headers), composed with Chain.
package middleware

import (
	"net/http"
	"slices"
)

// Middleware wraps a handler: it returns a handler that does its own work
// around a call to next.
type Middleware = func(next http.Handler) http.Handler

// Chain returns h wrapped in mws, the first one outermost: the order a
// request goes through them. Chain(h, a, b) is a(b(h)).
func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for _, mw := range slices.Backward(mws) {
		h = mw(h)
	}

	return h
}
