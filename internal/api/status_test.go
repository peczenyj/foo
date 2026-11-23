package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/peczenyj/foo/internal/api"
)

// The status recorded for a request is the one the client gets.
func TestStatusWriter_records_the_status_sent(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		label string
		write func(w http.ResponseWriter)
		want  int
	}{
		{
			label: "should record 200 when nothing is written",
			write: func(http.ResponseWriter) {},
			want:  http.StatusOK,
		},
		{
			label: "should record the status of WriteHeader",
			write: func(w http.ResponseWriter) { w.WriteHeader(http.StatusNotFound) },
			want:  http.StatusNotFound,
		},
		{
			label: "should record 200 for a body written without WriteHeader",
			write: func(w http.ResponseWriter) { _, _ = w.Write([]byte("body")) },
			want:  http.StatusOK,
		},
		{
			label: "should keep the first status when WriteHeader is called twice",
			write: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusCreated)
				w.WriteHeader(http.StatusInternalServerError)
			},
			want: http.StatusCreated,
		},
		{
			label: "should keep 200 when WriteHeader follows a Write",
			write: func(w http.ResponseWriter) {
				_, _ = w.Write([]byte("body"))
				w.WriteHeader(http.StatusInternalServerError)
			},
			want: http.StatusOK,
		},
		{
			label: "should skip an informational status for the final one",
			write: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusEarlyHints)
				w.WriteHeader(http.StatusNoContent)
			},
			want: http.StatusNoContent,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.label, func(t *testing.T) {
			t.Parallel()

			w, status := api.NewStatusWriter(httptest.NewRecorder())

			tc.write(w)

			assert.Equal(t, tc.want, status())
		})
	}
}

// http.ResponseController reaches the underlying writer through Unwrap.
func TestStatusWriter_unwraps_for_ResponseController(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	w, _ := api.NewStatusWriter(recorder)

	require.NoError(t, http.NewResponseController(w).Flush())
	assert.True(t, recorder.Flushed)
}
