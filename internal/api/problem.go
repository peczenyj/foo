package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/peczenyj/foo/internal/foo"
	"github.com/peczenyj/foo/internal/problem"
)

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
		return http.StatusInternalServerError, problem.DetailInternal
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

// writeProblem writes a problem response with status and detail
// (problem.Write), logging an encoding error as writeBody does.
func (a *API) writeProblem(w http.ResponseWriter, status int, detail string) {
	if err := problem.Write(w, status, detail); err != nil {
		a.logger.Warn("unexpected error while encoding the problem response", slog.Any("error", err))
	}
}
