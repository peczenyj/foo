package metrics_test

import (
	"expvar"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/peczenyj/foo/internal/metrics"
)

// The build fails if Nop stops being a Recorder.
var _ metrics.Recorder = metrics.Nop{}

func TestNop_records_nothing(t *testing.T) {
	t.Parallel()

	assert.NotPanics(t, func() { metrics.Nop{}.ObserveRequest("GET /foo", 200, time.Second) })
}

func TestExpvar_counts_and_sums_by_route_and_status(t *testing.T) {
	t.Parallel()

	recorder := metrics.NewUnpublishedExpvar()

	recorder.ObserveRequest("GET /foo", 200, 100*time.Millisecond)
	recorder.ObserveRequest("GET /foo", 200, 300*time.Millisecond)
	recorder.ObserveRequest("GET /foo", 500, time.Second)
	recorder.ObserveRequest(metrics.Unmatched, 404, time.Millisecond)

	requests, seconds := metrics.ExpvarMaps(recorder)

	assert.Equal(t, int64(2), intVar(t, requests, "GET /foo 200"))
	assert.Equal(t, int64(1), intVar(t, requests, "GET /foo 500"))
	assert.Equal(t, int64(1), intVar(t, requests, "unmatched 404"))
	assert.InDelta(t, 0.4, floatVar(t, seconds, "GET /foo 200"), 1e-9)
	assert.InDelta(t, 1.0, floatVar(t, seconds, "GET /foo 500"), 1e-9)
}

// Expvar is process-wide: every call returns the same recorder, published
// once under RequestsVar and RequestSecondsVar (a second expvar.Publish of a
// name would panic).
func TestExpvar_is_published_once(t *testing.T) {
	t.Parallel()

	first := metrics.Expvar()
	require.Same(t, first, metrics.Expvar())

	requests, seconds := metrics.ExpvarMaps(first)

	assert.Same(t, requests, expvar.Get(metrics.RequestsVar))
	assert.Same(t, seconds, expvar.Get(metrics.RequestSecondsVar))
}

func intVar(t *testing.T, m *expvar.Map, key string) int64 {
	t.Helper()

	v, ok := m.Get(key).(*expvar.Int)
	require.True(t, ok, "%s is not an *expvar.Int: %v", key, m.Get(key))

	return v.Value()
}

func floatVar(t *testing.T, m *expvar.Map, key string) float64 {
	t.Helper()

	v, ok := m.Get(key).(*expvar.Float)
	require.True(t, ok, "%s is not an *expvar.Float: %v", key, m.Get(key))

	return v.Value()
}
