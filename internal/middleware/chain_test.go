package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/peczenyj/foo/internal/middleware"
)

func TestChain_runs_the_first_middleware_outermost(t *testing.T) {
	t.Parallel()

	var calls []string

	trace := func(name string) middleware.Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls = append(calls, name)

				next.ServeHTTP(w, r)

				calls = append(calls, name+"'")
			})
		}
	}

	handler := middleware.Chain(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls = append(calls, "handler") }),
		trace("a"), trace("b"), trace("c"),
	)

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))

	assert.Equal(t, []string{"a", "b", "c", "handler", "c'", "b'", "a'"}, calls)
}
