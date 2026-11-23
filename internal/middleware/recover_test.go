package middleware_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/peczenyj/foo/internal/middleware"
	"github.com/peczenyj/foo/internal/problem"
)

// newPanicMux returns a mux whose handlers panic in the ways Recover handles.
func newPanicMux() *http.ServeMux {
	mux := http.NewServeMux()
	// Headers set for a body that never comes must not reach the 500.
	mux.HandleFunc("GET /panic", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "1000")
		w.Header().Set("Content-Type", "application/json")
		panic("boom")
	})
	mux.HandleFunc("GET /partial", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("partial"))

		panic("boom")
	})
	mux.HandleFunc("GET /abort", func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) })
	// Headers of a response that never comes: none may reach the 500.
	mux.HandleFunc("GET /half-built", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Header().Set("Set-Cookie", "session=new")
		w.Header().Set("Location", "/elsewhere")
		panic("boom")
	})
	mux.HandleFunc("GET /ok", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("ok"))
	})

	return mux
}

// recoverRequest serves one request with handler and returns the response
// recorder; a panic out of handler carries on.
func recoverRequest(t *testing.T, handler http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil))

	return recorder
}

// logLines returns the JSON log lines in logs, decoded.
func logLines(t *testing.T, logs *bytes.Buffer) []map[string]any {
	t.Helper()

	var lines []map[string]any

	for line := range strings.Lines(logs.String()) {
		var entry map[string]any

		require.NoError(t, json.Unmarshal([]byte(line), &entry), "log line %q", line)

		lines = append(lines, entry)
	}

	return lines
}

// Without a panic, Recover changes nothing and logs nothing.
func TestRecover_passes_a_response_through(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer

	handler := middleware.Chain(newPanicMux(), middleware.Recover(slog.New(slog.NewJSONHandler(&logs, nil))))

	recorder := recoverRequest(t, handler, "/ok")

	assert.Equal(t, http.StatusCreated, recorder.Code)
	assert.Equal(t, "ok", recorder.Body.String())
	assert.Empty(t, logs.String())
}

func TestRecover_answers_a_panic_with_500(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer

	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	handler := middleware.Chain(newPanicMux(), middleware.Recover(logger))

	recorder := recoverRequest(t, handler, "/panic")

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.Equal(t, problem.MediaType, recorder.Header().Get("Content-Type"))
	assert.Empty(t, recorder.Header().Get("Content-Length"), "the handler's Content-Length must not reach the 500")

	var got problem.Problem

	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &got))
	assert.Equal(t, problem.Problem{
		Type:   "about:blank",
		Title:  "Internal Server Error",
		Status: http.StatusInternalServerError,
		Detail: problem.DetailInternal,
	}, got)

	lines := logLines(t, &logs)
	require.Len(t, lines, 1)
	assert.Equal(t, "ERROR", lines[0]["level"])
	assert.Equal(t, "panic recovered", lines[0]["msg"])
	assert.Equal(t, "boom", lines[0]["panic"])
	assert.Equal(t, "GET", lines[0]["method"])
	assert.Equal(t, "GET /panic", lines[0]["route"])
	// The panicking handler's own frame, not just this test's: the stack is
	// the panic site's.
	assert.Contains(t, lines[0]["stack"], "middleware_test.newPanicMux.func")
}

// Behind Metrics, a panic after the response started is recorded with the
// status that was sent (the connection is aborted, no 500 follows).
func TestRecover_behind_Metrics_records_the_sent_status_of_an_aborted_response(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.DiscardHandler)
	recorder := &fakeRecorder{}
	handler := middleware.Chain(newPanicMux(), middleware.Metrics(logger, recorder), middleware.Recover(logger))

	assert.PanicsWithValue(t, http.ErrAbortHandler, func() { recoverRequest(t, handler, "/partial") })
	assert.Equal(t, []observation{{route: "GET /partial", status: http.StatusOK}}, recorder.observations)
}

// The 500 carries the headers set before the handler ran (an outer Headers'
// baseline), not the ones the panicking handler set for its own response: a
// gzip encoding or a cache lifetime would contradict the problem body.
func TestRecover_answers_with_the_headers_set_before_the_handler(t *testing.T) {
	t.Parallel()

	handler := middleware.Chain(newPanicMux(),
		middleware.Headers(map[string]string{"Cache-Control": "no-store", "X-Frame-Options": "DENY"}),
		middleware.Recover(slog.New(slog.DiscardHandler)),
	)

	recorder := recoverRequest(t, handler, "/half-built")

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.Equal(t, http.Header{
		"Cache-Control":          {"no-store"},
		"X-Frame-Options":        {"DENY"},
		"Content-Type":           {problem.MediaType},
		"X-Content-Type-Options": {"nosniff"},
	}, recorder.Header())
}

// Once the response has started, no 500 can be sent: the connection is
// aborted, and the panic logged once.
func TestRecover_aborts_a_started_response(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer

	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	handler := middleware.Chain(newPanicMux(), middleware.Recover(logger))
	recorder := httptest.NewRecorder()

	assert.PanicsWithValue(t, http.ErrAbortHandler, func() {
		handler.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/partial", nil))
	})
	assert.Equal(t, http.StatusOK, recorder.Code)

	lines := logLines(t, &logs)
	require.Len(t, lines, 1)
	assert.Equal(t, "panic recovered", lines[0]["msg"])
}

// http.ErrAbortHandler is how a handler asks net/http to abort quietly:
// Recover lets it through, unlogged.
func TestRecover_lets_ErrAbortHandler_through(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer

	handler := middleware.Chain(newPanicMux(), middleware.Recover(slog.New(slog.NewJSONHandler(&logs, nil))))

	assert.PanicsWithValue(t, http.ErrAbortHandler, func() { recoverRequest(t, handler, "/abort") })
	assert.Empty(t, logs.String())
}

// Behind Metrics, a recovered panic is recorded as the 500 Recover sent,
// under its route: Recover keeps the request the mux sets the pattern on.
func TestRecover_behind_Metrics_records_500_under_the_route(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.DiscardHandler)
	recorder := &fakeRecorder{}
	handler := middleware.Chain(newPanicMux(), middleware.Metrics(logger, recorder), middleware.Recover(logger))

	response := recoverRequest(t, handler, "/panic")

	assert.Equal(t, http.StatusInternalServerError, response.Code)
	assert.Equal(t, []observation{{route: "GET /panic", status: http.StatusInternalServerError}}, recorder.observations)
}

// failingWriter is a ResponseWriter whose body writes fail, as on a
// connection the client closed.
type failingWriter struct{ header http.Header }

func (w *failingWriter) Header() http.Header     { return w.header }
func (*failingWriter) Write([]byte) (int, error) { return 0, errConnectionClosed }
func (*failingWriter) WriteHeader(int)           {}

var errConnectionClosed = errors.New("connection closed")

// A 500 that cannot be sent is logged at warn level, after the panic.
func TestRecover_logs_a_500_it_could_not_send(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer

	handler := middleware.Chain(newPanicMux(), middleware.Recover(slog.New(slog.NewJSONHandler(&logs, nil))))

	handler.ServeHTTP(&failingWriter{header: make(http.Header)},
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/panic", nil))

	lines := logLines(t, &logs)
	require.Len(t, lines, 2)
	assert.Equal(t, "panic recovered", lines[0]["msg"])
	assert.Equal(t, "WARN", lines[1]["level"])
	assert.Equal(t, "unexpected error while encoding the problem response", lines[1]["msg"])
}
