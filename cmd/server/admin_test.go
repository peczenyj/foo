package main

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// syncBuffer is a bytes.Buffer safe for the concurrent writes of a running
// server's logger and the reads of the test.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

// waitForAddr returns the address logged with msg (JSON logs), failing the
// test if it does not appear within a few seconds.
func waitForAddr(t *testing.T, logs *syncBuffer, msg string) string {
	t.Helper()

	re := regexp.MustCompile(`"msg":"` + msg + `","addr":"([^"]+)"`)

	var addr string

	require.Eventually(t, func() bool {
		if m := re.FindStringSubmatch(logs.String()); m != nil {
			addr = m[1]

			return true
		}

		return false
	}, 5*time.Second, 10*time.Millisecond, "no %q log line: %s", msg, logs)

	return addr
}

func get(t *testing.T, url string) (int, string) {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err)

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)

	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	return response.StatusCode, string(body)
}

// With an admin port, run serves the admin server next to the public one:
// the public server's requests show up in the admin server's /debug/vars,
// and both shut down when ctx is cancelled.
func TestRun_serves_the_admin_server(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	var stdout bytes.Buffer

	stderr := &syncBuffer{}
	args := []string{"-host=127.0.0.1", "-port=0", "-admin-host=127.0.0.1", "-admin-port=0", "-log-format=json"}

	exit := make(chan int, 1)

	go func() { exit <- run(ctx, args, env(nil), &stdout, stderr) }()

	public := waitForAddr(t, stderr, "listening")
	adminAddr := waitForAddr(t, stderr, "admin listening")
	assert.NotEqual(t, public, adminAddr)

	status, _ := get(t, "http://"+public+"/foo")
	require.Equal(t, http.StatusOK, status)

	status, vars := get(t, "http://"+adminAddr+"/debug/vars")
	require.Equal(t, http.StatusOK, status)
	assert.Contains(t, vars, `"GET /foo 200": `)

	status, _ = get(t, "http://"+adminAddr+"/debug/pprof/")
	assert.Equal(t, http.StatusOK, status)

	// The admin server never serves the public API, and the reverse.
	status, _ = get(t, "http://"+adminAddr+"/foo")
	assert.Equal(t, http.StatusNotFound, status)

	status, _ = get(t, "http://"+public+"/debug/vars")
	assert.Equal(t, http.StatusNotFound, status)

	cancel()

	select {
	case code := <-exit:
		assert.Equal(t, exitOK, code, "stderr: %s", stderr)
	case <-time.After(5 * time.Second):
		t.Fatalf("run did not return after cancel: %s", stderr)
	}
}

// When the admin server cannot listen, run fails, and the public server it
// had started shuts down instead of serving alone.
func TestRun_fails_if_the_admin_address_is_in_use(t *testing.T) {
	t.Parallel()

	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	_, port, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)

	var stdout bytes.Buffer

	stderr := &syncBuffer{}
	args := []string{"-host=127.0.0.1", "-port=0", "-admin-host=127.0.0.1", "-admin-port=" + port, "-log-format=json"}

	exit := make(chan int, 1)

	go func() { exit <- run(context.Background(), args, env(nil), &stdout, stderr) }()

	select {
	case code := <-exit:
		assert.Equal(t, exitError, code)
		assert.Contains(t, stderr.String(), "admin")
		assert.Contains(t, stderr.String(), "listen")
	case <-time.After(5 * time.Second):
		t.Fatalf("run did not return: %s", stderr)
	}
}
