// Package admin is the operators' HTTP interface: profiling (net/http/pprof)
// and metrics (expvar), on a mux of its own. cmd/server serves it on a port
// of its own, never on the public one, and only when a port is configured.
//
// Importing net/http/pprof and expvar also registers their handlers on
// http.DefaultServeMux, which this service never serves.
package admin

import (
	"expvar"
	"net/http"
	"net/http/pprof"
)

// Admin is the admin http.Handler.
type Admin struct {
	mux *http.ServeMux
}

// The compile fails if Admin stops being an http.Handler.
var _ http.Handler = (*Admin)(nil)

// New returns an Admin serving:
//   - GET /debug/vars: every expvar variable as JSON (the Go runtime's
//     memstats and cmdline, and the service's metrics, package metrics);
//   - /debug/pprof/: the profiles of net/http/pprof (index, cmdline,
//     profile, symbol, trace, and the named ones such as heap).
func New() *Admin {
	a := &Admin{mux: http.NewServeMux()}

	a.Handle("GET /debug/vars", expvar.Handler())
	a.Handle("/debug/pprof/", http.HandlerFunc(pprof.Index))
	a.Handle("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
	a.Handle("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
	a.Handle("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
	a.Handle("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))

	return a
}

// Handle registers h for pattern: the place for more admin routes, such as a
// Prometheus /metrics next to /debug/vars.
func (a *Admin) Handle(pattern string, h http.Handler) { a.mux.Handle(pattern, h) }

// ServeHTTP implements http.Handler.
func (a *Admin) ServeHTTP(w http.ResponseWriter, r *http.Request) { a.mux.ServeHTTP(w, r) }
