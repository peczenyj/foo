package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/peczenyj/foo/internal/foo"
)

const (
	mediaTypeProblem = `application/problem+json`
	// problemTypeBlank: the problem is fully described by its status code.
	problemTypeBlank = "about:blank"
	// detailInternal is the only detail a 5xx response carries: the real
	// error is logged, never sent.
	detailInternal = "the server could not complete the request"
)

// Problem is the body of an error response: RFC 9457 problem details, served
// as application/problem+json.
type Problem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// problemFor maps an error to the status and detail of its problem response.
// It is the only place that does: to support a new kind of error, add a typed
// error (like foo.InputError) or a sentinel, and a case here. The detail sent
// comes from the typed error's own field, never from err.Error(): wrappers up
// the chain may add internal context, which belongs in the logs only.
func problemFor(err error) (status int, detail string) {
	var inputErr *foo.InputError

	switch {
	case errors.As(err, &inputErr):
		return http.StatusBadRequest, inputErr.Detail
	default:
		return http.StatusInternalServerError, detailInternal
	}
}

// writeError answers err as a problem response. A server error is logged at
// error level, with the error the client never sees; a client error only at
// debug level.
func (a *API) writeError(w http.ResponseWriter, err error) {
	status, detail := problemFor(err)

	if status >= http.StatusInternalServerError {
		a.logger.Error("request failed", slog.Int("status", status), slog.Any("error", err))
	} else {
		a.logger.Debug("request rejected", slog.Int("status", status), slog.Any("error", err))
	}

	a.writeProblem(w, status, detail)
}

// writeProblem writes a problem response with status and detail.
func (a *API) writeProblem(w http.ResponseWriter, status int, detail string) {
	// Like http.Error: browsers must not guess another content type.
	w.Header().Set("X-Content-Type-Options", "nosniff")

	_ = a.writeBody(w, status, mediaTypeProblem, &Problem{
		Type:   problemTypeBlank,
		Title:  http.StatusText(status),
		Status: status,
		Detail: detail,
	})
}
