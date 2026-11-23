package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/peczenyj/foo/internal/config"
)

func env(vars map[string]string) func(string) string {
	return func(name string) string { return vars[name] }
}

func TestRun(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		label    string
		args     []string
		env      map[string]string
		exitCode int
		stdout   string // expected exact output
		stderr   string // expected substring
	}{
		{
			label:    "should print the version and exit",
			args:     []string{"-version"},
			exitCode: exitOK,
			stdout:   programName + " dev (revision unknown)\n",
		},
		{
			label:    "should print the usage on -h",
			args:     []string{"-h"},
			exitCode: exitOK,
			stderr:   "Usage of " + programName,
		},
		{
			label:    "should reject an unknown flag",
			args:     []string{"-nope"},
			exitCode: exitUsage,
			stderr:   "flag provided but not defined",
		},
		{
			label:    "should reject an invalid port flag",
			args:     []string{"-port=70000"},
			exitCode: exitUsage,
			stderr:   "invalid configuration",
		},
		{
			label:    "should reject an invalid port env var",
			env:      map[string]string{config.EnvPort: "abc"},
			exitCode: exitUsage,
			stderr:   config.EnvPort,
		},
		{
			label:    "should not print the version with an invalid env var",
			args:     []string{"-version"},
			env:      map[string]string{config.EnvPort: "abc"},
			exitCode: exitUsage,
			stderr:   config.EnvPort,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.label, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer

			got := run(context.Background(), tc.args, env(tc.env), &stdout, &stderr)

			assert.Equal(t, tc.exitCode, got, "stderr: %s", stderr.String())
			assert.Equal(t, tc.stdout, stdout.String())
			assert.Contains(t, stderr.String(), tc.stderr)
		})
	}
}

// TestRun_serves_until_cancelled checks the whole wiring: configuration,
// logger, server start on the real bound address, graceful shutdown.
func TestRun_serves_until_cancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // the server shuts down as soon as it starts

	var stdout, stderr bytes.Buffer

	// port 0: any free port, so parallel test runs don't collide
	got := run(ctx, []string{"-host=127.0.0.1", "-port=0", "-log-format=json"}, env(nil), &stdout, &stderr)

	require.Equal(t, exitOK, got, "stderr: %s", stderr.String())
	assert.Contains(t, stderr.String(), `"msg":"application start"`)
	assert.Contains(t, stderr.String(), `"msg":"listening"`)
	assert.Contains(t, stderr.String(), `"addr":"127.0.0.1:`)
	assert.NotContains(t, stderr.String(), `"addr":"127.0.0.1:0"`, "must log the bound port, not the requested one")
	assert.NotContains(t, stderr.String(), "admin listening", "the admin server is off by default")
}

func TestRun_fails_if_the_address_is_in_use(t *testing.T) {
	t.Parallel()

	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	_, port, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	got := run(context.Background(), []string{"-host=127.0.0.1", "-port=" + port}, env(nil), &stdout, &stderr)

	assert.Equal(t, exitError, got)
	assert.Contains(t, stderr.String(), "listen")

	// The cause, checked on every OS: the error text differs (Linux "address
	// already in use", Windows "Only one usage of each socket address…"), and
	// so does the errno (syscall.EADDRINUSE on Unix, WSAEADDRINUSE on Windows).
	_, err = listen(t.Context(), listener.Addr().String())

	var opErr *net.OpError
	require.ErrorAs(t, err, &opErr)
	assert.Equal(t, "listen", opErr.Op)
}

// When one server fails, runAll stops the others, so the service never runs
// half-up, and returns the failure, named after its server if it has a name.
func TestRunAll_stops_the_others_when_one_fails(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")

	testcases := []struct {
		label   string
		failing string // the failing server's name
		wantErr string
	}{
		{
			label:   "should stop the admin server when the public one fails",
			failing: "",
			wantErr: "boom",
		},
		{
			label:   "should stop the public server when the admin one fails",
			failing: "admin server",
			wantErr: "admin server: boom",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.label, func(t *testing.T) {
			t.Parallel()

			causes := make(chan error, 1)

			waiting := namedServer{name: "other", run: func(ctx context.Context) error {
				<-ctx.Done()

				causes <- context.Cause(ctx)

				return nil
			}}
			failing := namedServer{name: tc.failing, run: func(context.Context) error { return boom }}

			err := runAll(t.Context(), waiting, failing)

			require.ErrorIs(t, err, boom)
			require.EqualError(t, err, tc.wantErr)
			assert.ErrorIs(t, <-causes, errOtherFailed)
		})
	}
}

// net/http reports a handler panic through the server's ErrorLog: it must
// reach the logs at error level, even when info logs are disabled.
func TestNewHTTPServer_logs_handler_panics_at_error_level(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer

	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn}))
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") })

	srv := httptest.NewUnstartedServer(handler)
	srv.Config = newHTTPServer(handler, logger)
	srv.Start()
	t.Cleanup(srv.Close)

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
	require.NoError(t, err)

	response, err := srv.Client().Do(request)
	if err == nil {
		_ = response.Body.Close()
	}

	require.Error(t, err, "the server drops the connection of a panicking handler")

	srv.Close() // waits for the connection to close, after the panic is logged

	assert.Contains(t, logs.String(), `"level":"ERROR"`)
	assert.Contains(t, logs.String(), "panic serving")
	assert.Contains(t, logs.String(), "boom")
}

// The public handler serves the API through its middlewares.
func TestNewHandler_serves_the_api(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()

	newHandler(slog.New(slog.DiscardHandler)).
		ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/foo", nil))

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `{"message":"bar"}`, recorder.Body.String())
}

// Every public response carries the security baseline, the router's own 404
// and 405 included.
func TestNewHandler_sets_the_security_headers(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"Cache-Control":           "no-store",
		"Content-Security-Policy": "default-src 'none'; frame-ancestors 'none'",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "no-referrer",
	}

	handler := newHandler(slog.New(slog.DiscardHandler))

	requests := []struct {
		method, target string
		status         int
	}{
		{method: http.MethodGet, target: "/foo", status: http.StatusOK},
		{method: http.MethodGet, target: "/missing", status: http.StatusNotFound},
		{method: http.MethodPost, target: "/foo", status: http.StatusMethodNotAllowed},
	}

	for _, req := range requests {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), req.method, req.target, nil))

		// The headers as sent: recorder.Header() would also show those set
		// after the response was written.
		response := recorder.Result()
		require.NoError(t, response.Body.Close())
		require.Equal(t, req.status, response.StatusCode, "%s %s", req.method, req.target)

		for name, value := range want {
			assert.Equal(t, value, response.Header.Get(name), "%s %s: %s", req.method, req.target, name)
		}
	}
}
