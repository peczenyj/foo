package middleware

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"runtime"
	"strings"
)

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

// wrap returns w as a statusWriter: w itself when it already is one (an
// outer middleware wrapped it), so the middlewares of a request share one
// writer and see the same status.
func wrap(w http.ResponseWriter, logger *slog.Logger) *statusWriter {
	if sw, ok := w.(*statusWriter); ok {
		return sw
	}

	return newStatusWriter(w, logger)
}
