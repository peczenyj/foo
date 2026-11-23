// Package metrics records the service's own metrics behind a small
// interface, Recorder: expvar now (ExpvarRecorder, served by the admin
// server on /debug/vars), another backend later (Prometheus, for example)
// without touching the code that records them.
package metrics

import (
	"expvar"
	"strconv"
	"sync"
	"time"
)

// Recorder records metrics. Implementations must be safe for concurrent use.
type Recorder interface {
	// ObserveRequest records one HTTP request: its route (the ServeMux
	// pattern, such as "GET /foo", or Unmatched), its status and duration.
	ObserveRequest(route string, status int, duration time.Duration)
}

// Unmatched is the route of a request that no pattern matched (the router's
// 404 and 405): never the raw path, which a client could vary without end.
const Unmatched = "unmatched"

// Nop records nothing.
type Nop struct{}

// ObserveRequest implements Recorder.
func (Nop) ObserveRequest(string, int, time.Duration) {}

// The expvar variables Expvar publishes, both keyed "<route> <status>"
// (e.g. "GET /foo 200"): the request count, and the total of their
// durations in seconds (total / count is the mean).
const (
	RequestsVar       = "http_requests_total"
	RequestSecondsVar = "http_request_duration_seconds_total"
)

// ExpvarRecorder is a Recorder on two expvar maps (RequestsVar and
// RequestSecondsVar).
type ExpvarRecorder struct {
	requests *expvar.Map
	seconds  *expvar.Map
}

// ObserveRequest implements Recorder.
func (r *ExpvarRecorder) ObserveRequest(route string, status int, duration time.Duration) {
	key := route + " " + strconv.Itoa(status)

	r.requests.Add(key, 1)
	r.seconds.AddFloat(key, duration.Seconds())
}

// newExpvarRecorder returns an ExpvarRecorder on maps that are not published.
func newExpvarRecorder() *ExpvarRecorder {
	return &ExpvarRecorder{requests: new(expvar.Map).Init(), seconds: new(expvar.Map).Init()}
}

// published is the process-wide ExpvarRecorder: expvar names are global, and
// expvar.Publish panics on a name published twice.
//
//nolint:gochecknoglobals // expvar names are process-wide: published once.
var published = sync.OnceValue(func() *ExpvarRecorder {
	r := newExpvarRecorder()

	expvar.Publish(RequestsVar, r.requests)
	expvar.Publish(RequestSecondsVar, r.seconds)

	return r
})

// Expvar returns the process-wide ExpvarRecorder, published as RequestsVar
// and RequestSecondsVar on its first call; every call returns the same one.
func Expvar() *ExpvarRecorder { return published() }
