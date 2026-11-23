package middleware

import (
	"log/slog"
	"net/http"
)

// NewStatusWriter returns the writer Metrics and Recover wrap w in, logging
// to logger, and a func that returns the status it recorded.
func NewStatusWriter(w http.ResponseWriter, logger *slog.Logger) (http.ResponseWriter, func() int) {
	sw := newStatusWriter(w, logger)

	return sw, func() int { return sw.status }
}

// Wrap is wrap, for the tests.
func Wrap(w http.ResponseWriter, logger *slog.Logger) http.ResponseWriter { return wrap(w, logger) }
