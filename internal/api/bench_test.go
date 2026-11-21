package api_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/peczenyj/foo/internal/api"
	"github.com/peczenyj/foo/internal/foo"
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

// newGetFoo returns the handler and request of the GET /foo hot path:
// routing, the real Fooer and JSON encoding, with logs discarded.
func newGetFoo() (http.Handler, *http.Request) {
	handler := api.New(foo.Bar{}, slog.New(slog.DiscardHandler))

	return handler, httptest.NewRequest(http.MethodGet, "/foo", nil)
}

// BenchmarkAPI_GetFoo measures GET /foo. Run it with make bench, which
// repeats it and summarises time, bytes and allocations with benchstat.
func BenchmarkAPI_GetFoo(b *testing.B) {
	handler, request := newGetFoo()
	w := newDiscardWriter()

	b.ReportAllocs()

	for b.Loop() {
		w.reset()
		handler.ServeHTTP(w, request)
	}
}

// getFooAllocBudget is the most allocations GET /foo may make. Unlike
// timings, allocation counts are deterministic, so CI can enforce them.
// When a change or a Go upgrade moves the count, re-measure and update it
// on purpose.
const getFooAllocBudget = 2

// TestAPI_GetFoo_allocation_budget must not run in parallel:
// testing.AllocsPerRun counts every allocation in the process.
func TestAPI_GetFoo_allocation_budget(t *testing.T) {
	handler, request := newGetFoo()
	w := newDiscardWriter()

	budget := getFooAllocBudget
	if raceEnabled {
		budget++ // the race detector allocates once more per request
	}

	allocs := testing.AllocsPerRun(1000, func() {
		w.reset()
		handler.ServeHTTP(w, request)
	})

	assert.LessOrEqual(t, allocs, float64(budget), "GET /foo allocations per request")
}
