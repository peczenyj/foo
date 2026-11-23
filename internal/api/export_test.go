package api

import (
	"net/http"
	"slices"
)

// Routes returns the patterns registered by New, for the contract test.
func (a *API) Routes() []string { return slices.Clone(a.routes) }

// NewStatusWriter returns the writer ServeHTTP wraps w in, and a func that
// returns the status it recorded.
func NewStatusWriter(w http.ResponseWriter) (http.ResponseWriter, func() int) {
	sw := newStatusWriter(w)

	return sw, func() int { return sw.status }
}
