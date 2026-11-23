package admin_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/peczenyj/foo/internal/admin"
)

func TestAdmin(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		label       string
		method      string
		target      string
		status      int
		contentType string // expected prefix of the Content-Type; empty: not checked
		body        string // expected substring of the body; empty: not checked
	}{
		{
			label:       "should serve the expvar variables as JSON",
			method:      http.MethodGet,
			target:      "/debug/vars",
			status:      http.StatusOK,
			contentType: "application/json",
			body:        `"memstats"`,
		},
		{
			label:  "should reject POST /debug/vars",
			method: http.MethodPost,
			target: "/debug/vars",
			status: http.StatusMethodNotAllowed,
		},
		{
			label:       "should list the profiles",
			method:      http.MethodGet,
			target:      "/debug/pprof/",
			status:      http.StatusOK,
			contentType: "text/html",
			body:        "goroutine",
		},
		{label: "should serve a named profile", method: http.MethodGet, target: "/debug/pprof/heap", status: http.StatusOK},
		{
			label:  "should serve the command line",
			method: http.MethodGet,
			target: "/debug/pprof/cmdline",
			status: http.StatusOK,
		},
		{label: "should look up symbols", method: http.MethodGet, target: "/debug/pprof/symbol", status: http.StatusOK},
		{label: "should not serve the public API", method: http.MethodGet, target: "/foo", status: http.StatusNotFound},
	}

	srv := httptest.NewServer(admin.New())
	t.Cleanup(srv.Close)

	for _, tc := range testcases {
		t.Run(tc.label, func(t *testing.T) {
			t.Parallel()

			status, header, body := send(t, srv, tc.method, tc.target)

			assert.Equal(t, tc.status, status)

			if tc.contentType != "" {
				assert.Contains(t, header.Get("Content-Type"), tc.contentType)
			}

			if tc.body != "" {
				assert.Contains(t, string(body), tc.body)
			}
		})
	}
}

func TestAdmin_vars_are_valid_JSON(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(admin.New())
	t.Cleanup(srv.Close)

	_, _, body := send(t, srv, http.MethodGet, "/debug/vars")

	var vars map[string]json.RawMessage

	require.NoError(t, json.Unmarshal(body, &vars))
	assert.Contains(t, vars, "cmdline")
}

// Handle is where more admin routes go (a future Prometheus /metrics).
func TestAdmin_Handle_adds_a_route(t *testing.T) {
	t.Parallel()

	a := admin.New()
	a.Handle("GET /metrics", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "up 1\n")
	}))

	srv := httptest.NewServer(a)
	t.Cleanup(srv.Close)

	status, _, body := send(t, srv, http.MethodGet, "/metrics")

	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "up 1\n", string(body))
}

func send(t *testing.T, srv *httptest.Server, method, target string) (int, http.Header, []byte) {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), method, srv.URL+target, nil)
	require.NoError(t, err)

	response, err := srv.Client().Do(request)
	require.NoError(t, err)

	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	return response.StatusCode, response.Header, body
}
