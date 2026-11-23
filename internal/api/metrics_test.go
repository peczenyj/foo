package api_test

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/peczenyj/foo/internal/api"
	"github.com/peczenyj/foo/internal/foo"
	"github.com/peczenyj/foo/internal/metrics"
	foomock "github.com/peczenyj/foo/mocks/github.com/peczenyj/foo/foo"
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

func TestAPI_records_every_request(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		label  string
		setup  func(m *foomock.Fooer)
		method string
		target string
		want   observation
	}{
		{
			label:  "should record the route pattern and status",
			setup:  func(m *foomock.Fooer) { m.EXPECT().Foo(mock.Anything).Return("bar", nil).Once() },
			method: http.MethodGet,
			target: "/foo",
			want:   observation{route: "GET /foo", status: http.StatusOK},
		},
		{
			label: "should record a handler error's status",
			setup: func(m *foomock.Fooer) {
				m.EXPECT().Foo(mock.Anything).Return("", errors.New("db down")).Once()
			},
			method: http.MethodGet,
			target: "/foo",
			want:   observation{route: "GET /foo", status: http.StatusInternalServerError},
		},
		{
			label:  "should record HEAD under its GET route",
			setup:  func(m *foomock.Fooer) { m.EXPECT().Foo(mock.Anything).Return("bar", nil).Once() },
			method: http.MethodHead,
			target: "/foo",
			want:   observation{route: "GET /foo", status: http.StatusOK},
		},
		{
			label:  "should record the health check",
			method: http.MethodGet,
			target: "/healthz",
			want:   observation{route: "GET /healthz", status: http.StatusOK},
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

			fooer := foomock.NewFooer(t)
			if tc.setup != nil {
				tc.setup(fooer)
			}

			recorder := &fakeRecorder{}
			handler := api.New(fooer, slog.New(slog.DiscardHandler), api.WithRecorder(recorder))

			doRequest(t, handler, tc.method, tc.target)

			assert.Equal(t, []observation{tc.want}, recorder.observations)
			assert.Len(t, recorder.durations, 1)
		})
	}
}

func TestWithRecorder_panics_on_nil(t *testing.T) {
	t.Parallel()

	assert.PanicsWithValue(t, "api.WithRecorder: nil Recorder", func() { api.WithRecorder(nil) })
}

// A panicking handler is recorded too, as a 500 (net/http drops the
// connection), and the panic carries on to net/http.
func TestAPI_records_a_panicking_request(t *testing.T) {
	t.Parallel()

	recorder := &fakeRecorder{}
	handler := api.New(foo.New(), slog.New(slog.DiscardHandler), api.WithRecorder(recorder))
	handler.HandleTestRoute("GET /panic", func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) })

	assert.PanicsWithValue(t, http.ErrAbortHandler, func() { doRequest(t, handler, http.MethodGet, "/panic") })
	assert.Equal(t, []observation{{route: "GET /panic", status: http.StatusInternalServerError}}, recorder.observations)
}

// Each WithRecorder adds a recorder: a second backend never replaces the
// first.
func TestAPI_records_with_every_recorder(t *testing.T) {
	t.Parallel()

	first, second := &fakeRecorder{}, &fakeRecorder{}
	handler := api.New(foo.New(), slog.New(slog.DiscardHandler), api.WithRecorder(first), api.WithRecorder(second))

	doRequest(t, handler, http.MethodGet, "/healthz")

	want := []observation{{route: "GET /healthz", status: http.StatusOK}}
	assert.Equal(t, want, first.observations)
	assert.Equal(t, want, second.observations)
}

// Request bodies are capped with net/http's own writer, so a body over the
// cap also closes the connection after the response.
func TestAPI_caps_the_request_body(t *testing.T) {
	t.Parallel()

	handler := api.New(foo.New(), slog.New(slog.DiscardHandler))
	handler.HandleTestRoute("POST /body", func(w http.ResponseWriter, r *http.Request) {
		var tooLarge *http.MaxBytesError
		if _, err := io.ReadAll(r.Body); errors.As(err, &tooLarge) {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
		}
	})

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	body := strings.NewReader(strings.Repeat("x", 2<<20)) // 2 MiB, over the 1 MiB cap
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/body", body)
	require.NoError(t, err)

	response, err := server.Client().Do(request)
	require.NoError(t, err)

	defer response.Body.Close()

	assert.Equal(t, http.StatusRequestEntityTooLarge, response.StatusCode)
	assert.True(t, response.Close, "the connection must close after a body over the cap")
}
