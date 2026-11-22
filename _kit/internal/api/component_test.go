package api_test

import (
	"errors"
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
// process is involved.
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

		status, _, headers := get(t, srv, "/bar")

		assert.Equal(t, http.StatusNotFound, status)
		assert.Equal(t, plainTextMediaType, headers.Get("Content-Type"))
	})
}

// TestAPI_component_problem: a failing Fooer behind the real HTTP stack
// answers a problem response, and its error never crosses the network.
func TestAPI_component_problem(t *testing.T) {
	t.Parallel()

	fooer := &fakeFooer{err: errors.New("secret db failure")}

	srv := httptest.NewServer(api.New(fooer, slog.New(slog.DiscardHandler)))
	t.Cleanup(srv.Close)

	status, body, headers := get(t, srv, "/foo")

	assert.Equal(t, http.StatusInternalServerError, status)
	assert.Equal(t, problemMediaType, headers.Get("Content-Type"))
	assert.Equal(t, api.Problem{
		Type:   "about:blank",
		Title:  "Internal Server Error",
		Status: http.StatusInternalServerError,
		Detail: "the server could not complete the request",
	}, decodeJSON[api.Problem](t, body))
	assert.NotContains(t, string(body), "secret")
	assert.Equal(t, int32(1), fooer.calls.Load(), "calls to Foo")
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
