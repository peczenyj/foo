// Package api is the HTTP interface of the service.
package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/peczenyj/foo/internal/foo"
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
	mux    *http.ServeMux
	routes []string // the patterns registered on mux, in order (see handle)
	fooer  foo.Fooer
	logger *slog.Logger
}

// New returns an API serving /healthz and /foo with f, logging to logger.
// It panics if f or logger is nil: that is a wiring bug, not a runtime condition.
func New(f foo.Fooer, logger *slog.Logger) *API {
	if f == nil {
		panic("api.New: nil Fooer")
	}

	if logger == nil {
		panic("api.New: nil Logger")
	}

	a := &API{mux: http.NewServeMux(), fooer: f, logger: logger}

	a.handle("GET /healthz", a.handleHealthz)
	a.handle("GET /foo", a.handleFoo)

	return a
}

// ServeHTTP implements http.Handler.
func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) { a.mux.ServeHTTP(w, r) }

// handle registers h for pattern and records the pattern: the contract test
// requires the recorded routes and the operations in api/openapi.yaml to be
// the same set. Register every route through it.
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
