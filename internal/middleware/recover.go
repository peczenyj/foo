package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/peczenyj/foo/internal/problem"
)

// Recover answers a panicking handler with a 500 problem response
// (problem.DetailInternal), and logs the panic at error level with its stack,
// the method and the route.
//
// If the response has already started, no status can follow: the panic is
// logged and the connection aborted (it re-panics http.ErrAbortHandler,
// which net/http does not log again). A handler's own
// panic(http.ErrAbortHandler) goes through unlogged.
//
// Put it innermost, next to the router: an outer Metrics then records the
// 500 it sends like any other response.
func Recover(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sw := wrap(w, logger)

			defer func() {
				v := recover()
				if v == nil {
					return
				}

				if v == http.ErrAbortHandler { //nolint:errorlint,err113 // net/http compares the value itself.
					panic(v)
				}

				logger.Error("panic recovered",
					slog.Any("panic", v),
					slog.String("method", r.Method),
					slog.String("route", r.Pattern),
					slog.String("stack", string(debug.Stack())),
				)

				if sw.wroteHeader {
					panic(http.ErrAbortHandler)
				}

				// Set for a body that never comes: the problem's own length
				// differs.
				sw.Header().Del("Content-Length")

				if err := problem.Write(sw, http.StatusInternalServerError, problem.DetailInternal); err != nil {
					logger.Warn("unexpected error while encoding the problem response", slog.Any("error", err))
				}
			}()

			next.ServeHTTP(sw, r)
		})
	}
}
