package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/peczenyj/foo/internal/metrics"
)

// Metrics records every request with each of recorders: its route, status
// and duration. logger gets statusWriter's warnings (a superfluous
// WriteHeader). It panics without a recorder or with a nil one: that is a
// wiring bug.
//
// The route is the pattern http.ServeMux matched (r.Pattern, which the mux
// sets on the request it is given), read once the next handler returns, or
// metrics.Unmatched for the mux's 404 and 405: never the raw path. So nothing
// between Metrics and the mux may replace the request (r.WithContext,
// r.Clone), or every request is recorded as unmatched.
//
// A request whose handler panics is recorded too, as a 500 unless a status
// was already sent (net/http drops the connection); the panic carries on.
func Metrics(logger *slog.Logger, recorders ...metrics.Recorder) Middleware {
	if len(recorders) == 0 {
		panic("middleware.Metrics: no Recorder")
	}

	for _, recorder := range recorders {
		if recorder == nil {
			panic("middleware.Metrics: nil Recorder")
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := wrap(w, logger)
			returned := false

			defer func() {
				status := sw.status
				if !returned && !sw.wroteHeader {
					status = http.StatusInternalServerError
				}

				route := r.Pattern
				if route == "" {
					route = metrics.Unmatched
				}

				duration := time.Since(start)

				for _, recorder := range recorders {
					recorder.ObserveRequest(route, status, duration)
				}
			}()

			next.ServeHTTP(sw, r)

			returned = true
		})
	}
}
