// Package api is the HTTP interface of the service.
package api

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/peczenyj/foo/internal/foo"
	"github.com/peczenyj/foo/internal/metrics"
)

// API is an http.Handler: the build fails if it stops being one.
var _ http.Handler = (*API)(nil)

const (
	contentTypeHeader = `Content-Type`
	mediaTypeJSON     = `application/json`
)

// API is the HTTP interface of the service. It routes requests itself, so
// tests exercise the same routing as production.
//
// GET /foo answers with JSON. Its errors are RFC 9457 problem details
// (problemFor maps errors to statuses). The router's own errors keep
// net/http's plain text: http.ServeMux answers 404 for unknown paths and 405
// (with an Allow header) for a known path with another method.
type API struct {
	mux       *http.ServeMux
	routes    []string // the patterns registered on mux, in order (see handle)
	fooer     foo.Fooer
	logger    *slog.Logger
	recorders []metrics.Recorder // see WithRecorder
}

// Option configures an API in New.
type Option func(*API)

// WithRecorder adds r to the recorders of every request (its route pattern,
// status and duration): each WithRecorder adds one, so expvar and another
// backend can record side by side. Without it, New records nothing. It
// panics if r is nil.
func WithRecorder(r metrics.Recorder) Option {
	if r == nil {
		panic("api.WithRecorder: nil Recorder")
	}

	return func(a *API) { a.recorders = append(a.recorders, r) }
}

// New returns an API serving /healthz and /foo with f, logging to logger.
// It panics if f or logger is nil: that is a wiring bug, not a runtime condition.
func New(f foo.Fooer, logger *slog.Logger, opts ...Option) *API {
	if f == nil {
		panic("api.New: nil Fooer")
	}

	if logger == nil {
		panic("api.New: nil Logger")
	}

	a := &API{mux: http.NewServeMux(), fooer: f, logger: logger}

	for _, opt := range opts {
		opt(a)
	}

	a.handle("GET /healthz", a.handleHealthz)
	a.handle("GET /foo", a.handleFoo)

	return a
}

// maxRequestBodyBytes caps every request body. ServeHTTP applies it with
// net/http's own writer, so a body over the cap also closes the connection
// after the response (http.MaxBytesReader does that only with that writer,
// never through statusWriter).
const maxRequestBodyBytes = 1 << 20 // 1 MiB

// ServeHTTP implements http.Handler: it routes the request, then records it
// with every recorder. The route is the pattern ServeMux matched (r.Pattern,
// set by the mux), or metrics.Unmatched for its 404 and 405: never the raw
// path.
//
// A request whose handler panics is recorded too, as a 500 unless a status
// was already sent (net/http drops the connection); the panic carries on to
// net/http.
func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	if r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	}

	sw := newStatusWriter(w, a.logger)
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

		for _, recorder := range a.recorders {
			recorder.ObserveRequest(route, status, duration)
		}
	}()

	a.mux.ServeHTTP(sw, r)

	returned = true
}

// statusWriter keeps the status the client gets: the first final status
// written (not an informational 1xx, except 101 Switching Protocols), or 200
// once a body is written or flushed without one.
//
// It implements http.Flusher, http.Hijacker and io.ReaderFrom by delegating
// to the writer it wraps (through http.ResponseController), so handlers can
// stream, upgrade the connection, and keep net/http's sendfile path; Unwrap
// gives http.ResponseController the rest (deadlines, full duplex). Hijack
// answers http.ErrNotSupported where the wrapped writer can't (HTTP/2).
type statusWriter struct {
	http.ResponseWriter

	logger      *slog.Logger
	status      int
	wroteHeader bool
}

// Compile-time checks: handlers rely on these.
var (
	_ http.Flusher  = (*statusWriter)(nil)
	_ http.Hijacker = (*statusWriter)(nil)
	_ io.ReaderFrom = (*statusWriter)(nil)
)

func newStatusWriter(w http.ResponseWriter, logger *slog.Logger) *statusWriter {
	return &statusWriter{ResponseWriter: w, logger: logger, status: http.StatusOK}
}

// WriteHeader sends status, unless the response already has a final one:
// net/http would drop that call and log a "superfluous" warning naming
// statusWriter instead of the handler at fault, so it is dropped and logged
// here, with the real caller.
func (w *statusWriter) WriteHeader(status int) {
	if w.wroteHeader {
		w.logger.Error("superfluous WriteHeader call",
			slog.Int("status", status),
			slog.Int("sent", w.status),
			slog.String("caller", callerOf(1)),
		)

		return
	}

	// An informational status (103 Early Hints) precedes the final one; 101
	// Switching Protocols is final.
	if status >= http.StatusOK || status == http.StatusSwitchingProtocols {
		w.status, w.wroteHeader = status, true
	}

	w.ResponseWriter.WriteHeader(status)
}

// Write sends the header first, as 200, if no final status was written.
func (w *statusWriter) Write(b []byte) (int, error) {
	w.wroteHeader = true

	return w.ResponseWriter.Write(b) //nolint:wrapcheck // a pass-through: the caller sees the writer's own error.
}

// ReadFrom writes src as the body, as Write does, through the wrapped
// writer's ReadFrom when it has one (net/http's uses sendfile). Like
// net/http's, it sends the header only with the first byte: after an empty
// src, a status can still be written.
func (w *statusWriter) ReadFrom(src io.Reader) (n int64, err error) {
	if rf, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		n, err = rf.ReadFrom(src)
	} else {
		n, err = io.Copy(w.ResponseWriter, src)
	}

	if n > 0 {
		w.wroteHeader = true
	}

	return n, err //nolint:wrapcheck // a pass-through: the caller sees the writer's own error.
}

// FlushError sends the header (as 200 if no status was written) and the
// buffered body. http.ResponseController.Flush calls it.
func (w *statusWriter) FlushError() error {
	err := http.NewResponseController(w.ResponseWriter).Flush()
	if !errors.Is(err, http.ErrNotSupported) {
		w.wroteHeader = true
	}

	return err //nolint:wrapcheck // a pass-through: the caller sees the writer's own error.
}

// Flush implements http.Flusher: FlushError without the error.
func (w *statusWriter) Flush() { _ = w.FlushError() }

// Hijack hands the connection to the handler, which then answers on it
// itself: the request is recorded as 101 Switching Protocols (a WebSocket
// upgrade) unless a status was already written.
func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, rw, err := http.NewResponseController(w.ResponseWriter).Hijack()
	if err == nil && !w.wroteHeader {
		w.status, w.wroteHeader = http.StatusSwitchingProtocols, true
	}

	return conn, rw, err //nolint:wrapcheck // a pass-through: the caller sees the writer's own error.
}

// Unwrap lets http.ResponseController reach the underlying writer
// (deadlines, full duplex).
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

const (
	// maxCallerDepth bounds the frames callerOf looks through for its caller.
	maxCallerDepth = 16
	// callerOfFrames are the frames runtime.Callers counts before callerOf's
	// caller: runtime.Callers itself and callerOf.
	callerOfFrames = 2
)

// callerOf returns "function (file:line)" of the caller skip frames above its
// own caller, past net/http's own functions (http.Error, ServeContent…), as
// net/http names the caller in its warnings.
func callerOf(skip int) string {
	pcs := make([]uintptr, maxCallerDepth)
	frames := runtime.CallersFrames(pcs[:runtime.Callers(skip+callerOfFrames, pcs)])

	for {
		frame, more := frames.Next()
		if !more || !strings.HasPrefix(frame.Function, "net/http.") {
			return fmt.Sprintf("%s (%s:%d)", frame.Function, frame.File, frame.Line)
		}
	}
}

// handle registers h for pattern and records the pattern, so tests can list
// the registered routes. Register every route through it.
func (a *API) handle(pattern string, h http.HandlerFunc) {
	a.mux.HandleFunc(pattern, h)
	a.routes = append(a.routes, pattern)
}

func (a *API) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// MessageBody is the JSON body of a successful GET /foo.
type MessageBody struct {
	Message string `json:"message"`
}

func (a *API) handleFoo(w http.ResponseWriter, r *http.Request) {
	message, err := a.fooer.Foo(r.Context())
	if err != nil {
		a.writeError(w, err)

		return
	}

	_ = a.writeJSON(w, http.StatusOK, &MessageBody{Message: message})
}

// writeJSON writes result as the JSON body with statusCode. The returned
// error is already logged; callers only need it to know if the body was sent.
func (a *API) writeJSON(w http.ResponseWriter, statusCode int, result any) error {
	return a.writeBody(w, statusCode, mediaTypeJSON, result)
}

// writeBody writes result as JSON with statusCode and the mediaType
// Content-Type. The returned error is already logged.
func (a *API) writeBody(w http.ResponseWriter, statusCode int, mediaType string, result any) error {
	w.Header().Set(contentTypeHeader, mediaType)

	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(result); err != nil {
		a.logger.Warn("unexpected error while encoding the JSON response", slog.Any("error", err))

		return fmt.Errorf("encode the JSON response: %w", err)
	}

	return nil
}
