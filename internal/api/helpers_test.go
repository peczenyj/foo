package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// doRequest serves one request with handler and returns the response.
func doRequest(t *testing.T,
	handler http.Handler,
	method, target string,
) (
	statusCode int,
	body []byte,
	headers http.Header,
) {
	t.Helper()

	request := httptest.NewRequest(method, target, nil)

	w := httptest.NewRecorder()

	handler.ServeHTTP(w, request)

	response := w.Result()
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	require.NoError(t, err, "reading the response body")

	return response.StatusCode, body, response.Header
}

// decodeJSON decodes body into a T, failing the test if it is not valid JSON.
func decodeJSON[T any](t *testing.T, body []byte) T {
	t.Helper()

	var v T

	require.NoError(t, json.Unmarshal(body, &v), "decoding %q", body)

	return v
}
