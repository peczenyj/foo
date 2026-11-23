package middleware_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/peczenyj/foo/internal/metrics"
	"github.com/peczenyj/foo/internal/middleware"
)

// observation is one ObserveRequest call.
type observation struct {
	route  string
	status int
}

// fakeRecorder is a metrics.Recorder that keeps what it is given.
type fakeRecorder struct {
	mu           sync.Mutex
	observations []observation
	durations    []time.Duration
}

func (r *fakeRecorder) ObserveRequest(route string, status int, duration time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.observations = append(r.observations, observation{route: route, status: status})
	r.durations = append(r.durations, duration)
}

// newTestMux returns a mux with the routes the middleware tests need.
func newTestMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /foo", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("bar")) })
	mux.HandleFunc("GET /fail", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	mux.HandleFunc("GET /panic", func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) })

	return mux
}

// serve serves one request with handler, its response discarded.
func serve(t *testing.T, handler http.Handler, method, target string) {
	t.Helper()

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), method, target, nil))
}

func TestMetrics_records_every_request(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		label  string
		method string
		target string
		want   observation
	}{
		{
			label:  "should record the route pattern and status",
			method: http.MethodGet,
			target: "/foo",
			want:   observation{route: "GET /foo", status: http.StatusOK},
		},
		{
			label:  "should record a handler error's status",
			method: http.MethodGet,
			target: "/fail",
			want:   observation{route: "GET /fail", status: http.StatusInternalServerError},
		},
		{
			label:  "should record HEAD under its GET route",
			method: http.MethodHead,
			target: "/foo",
			want:   observation{route: "GET /foo", status: http.StatusOK},
		},
		{
			label:  "should record an unknown path as unmatched, never the path itself",
			method: http.MethodGet,
			target: "/secret-token-in-the-path",
			want:   observation{route: metrics.Unmatched, status: http.StatusNotFound},
		},
		{
			label:  "should record a wrong method as unmatched",
			method: http.MethodPost,
			target: "/foo",
			want:   observation{route: metrics.Unmatched, status: http.StatusMethodNotAllowed},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.label, func(t *testing.T) {
			t.Parallel()

			recorder := &fakeRecorder{}
			handler := middleware.Chain(newTestMux(), middleware.Metrics(slog.New(slog.DiscardHandler), recorder))

			serve(t, handler, tc.method, tc.target)

			assert.Equal(t, []observation{tc.want}, recorder.observations)
			assert.Len(t, recorder.durations, 1)
		})
	}
}

func TestMetrics_panics_without_a_recorder(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.DiscardHandler)

	assert.PanicsWithValue(t, "middleware.Metrics: no Recorder", func() { middleware.Metrics(logger) })
	assert.PanicsWithValue(t, "middleware.Metrics: nil Recorder", func() { middleware.Metrics(logger, nil) })
}

// A panicking handler is recorded too, as a 500 (net/http drops the
// connection), and the panic carries on.
func TestMetrics_records_a_panicking_request(t *testing.T) {
	t.Parallel()

	recorder := &fakeRecorder{}
	handler := middleware.Chain(newTestMux(), middleware.Metrics(slog.New(slog.DiscardHandler), recorder))

	assert.PanicsWithValue(t, http.ErrAbortHandler, func() { serve(t, handler, http.MethodGet, "/panic") })
	assert.Equal(t, []observation{{route: "GET /panic", status: http.StatusInternalServerError}}, recorder.observations)
}

// Every recorder records: a second backend never replaces the first.
func TestMetrics_records_with_every_recorder(t *testing.T) {
	t.Parallel()

	first, second := &fakeRecorder{}, &fakeRecorder{}
	handler := middleware.Chain(newTestMux(), middleware.Metrics(slog.New(slog.DiscardHandler), first, second))

	serve(t, handler, http.MethodGet, "/foo")

	want := []observation{{route: "GET /foo", status: http.StatusOK}}
	assert.Equal(t, want, first.observations)
	assert.Equal(t, want, second.observations)
}
