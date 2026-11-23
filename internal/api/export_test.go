package api

import (
	"log/slog"
	"net/http"
	"slices"
)

// Routes returns the patterns registered by New, for the contract test.
func (a *API) Routes() []string { return slices.Clone(a.routes) }

// NewStatusWriter returns the writer ServeHTTP wraps w in, logging to logger,
// and a func that returns the status it recorded.
func NewStatusWriter(w http.ResponseWriter, logger *slog.Logger) (http.ResponseWriter, func() int) {
	sw := newStatusWriter(w, logger)

	return sw, func() int { return sw.status }
}

// HandleTestRoute registers h on the API's mux without listing it in Routes,
// so a test can exercise ServeHTTP (recording, statusWriter, body limit)
// with a handler of its own; the contract test never sees it.
func (a *API) HandleTestRoute(pattern string, h http.HandlerFunc) { a.mux.HandleFunc(pattern, h) }
