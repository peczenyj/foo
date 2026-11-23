package api_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/peczenyj/foo/internal/api"
	"github.com/peczenyj/foo/internal/foo"
	"github.com/peczenyj/foo/internal/metrics"
)

// discardWriter is a reusable http.ResponseWriter that drops the response.
// A fresh httptest.ResponseRecorder per iteration would allocate on its own
// and blur what the handler itself costs.
type discardWriter struct{ header http.Header }

func newDiscardWriter() *discardWriter { return &discardWriter{header: make(http.Header)} }

func (w *discardWriter) Header() http.Header         { return w.header }
func (w *discardWriter) Write(p []byte) (int, error) { return len(p), nil }
func (w *discardWriter) WriteHeader(int)             {}

// reset clears the headers set by the previous request.
func (w *discardWriter) reset() { clear(w.header) }

// newGetFoo returns the handler and request of the GET /foo hot path as the
// binary serves it: routing, the real Fooer, JSON encoding and the expvar
// recorder, with logs discarded.
func newGetFoo(b *testing.B) (http.Handler, *http.Request) {
	b.Helper()

	handler := api.New(foo.New(), slog.New(slog.DiscardHandler), api.WithRecorder(metrics.Expvar()))

	return handler, httptest.NewRequestWithContext(b.Context(), http.MethodGet, "/foo", nil)
}

// BenchmarkAPI_GetFoo measures GET /foo. Run it with make bench, which
// repeats it and summarises time, bytes and allocations with benchstat.
func BenchmarkAPI_GetFoo(b *testing.B) {
	handler, request := newGetFoo(b)
	w := newDiscardWriter()

	b.ReportAllocs()

	for b.Loop() {
		w.reset()
		handler.ServeHTTP(w, request)
	}
}
