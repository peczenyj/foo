package middleware

import "net/http"

// Headers sets each of headers on every response, before the next handler
// runs: a handler can still override one. headers must not change once the
// middleware is built.
func Headers(headers map[string]string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for name, value := range headers {
				w.Header().Set(name, value)
			}

			next.ServeHTTP(w, r)
		})
	}
}
