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

// TestAPI_with_real_Fooer runs the API with the real Fooer behind a real
// HTTP server: routing, handler, encoding and the network stack together.
func TestAPI_with_real_Fooer(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(api.New(foo.Bar{}, slog.New(slog.DiscardHandler)))
	t.Cleanup(srv.Close)

	t.Run("GET /foo replies bar", func(t *testing.T) {
		t.Parallel()

		resp, err := srv.Client().Get(srv.URL + "/foo")
		require.NoError(t, err)

		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, mediaTypeJSON, resp.Header.Get("Content-Type"))
		assert.Equal(t, api.MessageBody{Message: "bar"}, decodeJSON[api.MessageBody](t, body))
	})

	t.Run("GET /bar is not found", func(t *testing.T) {
		t.Parallel()

		resp, err := srv.Client().Get(srv.URL + "/bar")
		require.NoError(t, err)

		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}
