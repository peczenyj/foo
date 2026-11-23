package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/peczenyj/foo/internal/middleware"
)

func TestHeaders_sets_each_header_before_the_handler(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /plain", func(http.ResponseWriter, *http.Request) {})
	mux.HandleFunc("GET /override", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "max-age=60")
	})

	handler := middleware.Chain(mux, middleware.Headers(map[string]string{"X-A": "1", "Cache-Control": "no-store"}))

	testcases := []struct {
		label  string
		target string
		want   map[string]string
	}{
		{
			label:  "should set every header",
			target: "/plain",
			want:   map[string]string{"X-A": "1", "Cache-Control": "no-store"},
		},
		{
			label:  "should let the handler override one",
			target: "/override",
			want:   map[string]string{"X-A": "1", "Cache-Control": "max-age=60"},
		},
		{
			label:  "should set them on the router's own 404",
			target: "/missing",
			want:   map[string]string{"X-A": "1", "Cache-Control": "no-store"},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.label, func(t *testing.T) {
			t.Parallel()

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, tc.target, nil))

			// The headers as sent: recorder.Header() would also show those
			// set after the response was written.
			response := recorder.Result()
			require.NoError(t, response.Body.Close())

			for name, value := range tc.want {
				assert.Equal(t, value, response.Header.Get(name), name)
			}
		})
	}
}
