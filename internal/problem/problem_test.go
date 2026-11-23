package problem_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/peczenyj/foo/internal/problem"
)

func TestWrite_answers_problem_details(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()

	err := problem.Write(recorder, http.StatusBadRequest, "bad id")
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Equal(t, "application/problem+json", recorder.Header().Get("Content-Type"))
	assert.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))

	var got problem.Problem

	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &got))
	assert.Equal(t, problem.Problem{
		Type:   "about:blank",
		Title:  "Bad Request",
		Status: http.StatusBadRequest,
		Detail: "bad id",
	}, got)
}

var errBoom = errors.New("boom")

// failingWriter is a ResponseWriter whose body writes fail.
type failingWriter struct{ *httptest.ResponseRecorder }

func (*failingWriter) Write([]byte) (int, error) { return 0, errBoom }

func TestWrite_returns_the_encode_error(t *testing.T) {
	t.Parallel()

	err := problem.Write(&failingWriter{ResponseRecorder: httptest.NewRecorder()}, http.StatusInternalServerError, "")

	require.ErrorIs(t, err, errBoom)
}
