package api_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/peczenyj/foo/internal/api"
	"github.com/peczenyj/foo/internal/foo"
)

// TestAPI_component tests the API as a component: the real Fooer behind a
// real HTTP server, in-process, through its public HTTP interface (routing,
// handler, encoding and the network stack together). Nothing outside the
// process is involved: tests that need external services are integration
// tests (//go:build integration, make test-integration).
func TestAPI_component(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(api.New(foo.New(), slog.New(slog.DiscardHandler)))
	t.Cleanup(srv.Close)

	t.Run("GET /foo replies bar", func(t *testing.T) {
		t.Parallel()

		status, body, headers := get(t, srv, "/foo")

		assert.Equal(t, http.StatusOK, status)
		assert.Equal(t, wantContentType, headers.Get("Content-Type"))
		assert.Equal(t, api.MessageBody{Message: "bar"}, decodeJSON[api.MessageBody](t, body))
	})

	t.Run("GET /bar is not found", func(t *testing.T) {
		t.Parallel()

		status, _, _ := get(t, srv, "/bar")

		assert.Equal(t, http.StatusNotFound, status)
	})
}

// get sends GET path to srv over the network and returns the response.
func get(t *testing.T,
	srv *httptest.Server,
	path string,
) (
	statusCode int,
	body []byte,
	headers http.Header,
) {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+path, nil)
	require.NoError(t, err)

	response, err := srv.Client().Do(request)
	require.NoError(t, err)

	defer response.Body.Close()

	body, err = io.ReadAll(response.Body)
	require.NoError(t, err, "reading the response body")

	return response.StatusCode, body, response.Header
}
