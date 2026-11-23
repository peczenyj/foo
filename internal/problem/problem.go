// Package problem writes error responses as RFC 9457 problem details
// (application/problem+json), for the API and the middlewares alike.
package problem

import (
	"encoding/json"
	"fmt"
	"net/http"
)

const (
	// MediaType is the Content-Type of a problem response.
	MediaType = `application/problem+json`
	// DetailInternal is the only detail a 5xx response carries: the real
	// error is logged, never sent.
	DetailInternal = "the server could not complete the request"

	// typeBlank: the problem is fully described by its status code.
	typeBlank = "about:blank"
)

// Problem is the body of an error response: RFC 9457 problem details, served
// as MediaType.
type Problem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// Write answers status as a problem response with detail. It returns the
// error of encoding the body, for the caller to log: the status is already
// sent by then.
func Write(w http.ResponseWriter, status int, detail string) error {
	// Like http.Error: browsers must not guess another content type.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", MediaType)
	w.WriteHeader(status)

	err := json.NewEncoder(w).Encode(&Problem{
		Type:   typeBlank,
		Title:  http.StatusText(status),
		Status: status,
		Detail: detail,
	})
	if err != nil {
		return fmt.Errorf("encode the problem response: %w", err)
	}

	return nil
}
