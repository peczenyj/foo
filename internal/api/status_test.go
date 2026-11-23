package api_test

import (
	"bytes"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/peczenyj/foo/internal/api"
)

// onlyReader hides every method of its Reader but Read: io.Copy calls a
// source's WriteTo before the destination's ReadFrom, and strings.Reader has
// one.
type onlyReader struct{ io.Reader }

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
			label: "should keep 200 when WriteHeader follows a Flush",
			write: func(w http.ResponseWriter) {
				_ = http.NewResponseController(w).Flush()
				w.WriteHeader(http.StatusInternalServerError)
			},
			want: http.StatusOK,
		},
		{
			label: "should keep 200 when WriteHeader follows a body copied with ReadFrom",
			write: func(w http.ResponseWriter) {
				_, _ = io.Copy(w, onlyReader{strings.NewReader("body")})
				w.WriteHeader(http.StatusInternalServerError)
			},
			want: http.StatusOK,
		},
		{
			label: "should record the status written after an empty ReadFrom",
			write: func(w http.ResponseWriter) {
				_, _ = io.Copy(w, onlyReader{strings.NewReader("")})
				w.WriteHeader(http.StatusNotFound)
			},
			want: http.StatusNotFound,
		},
		{
			label: "should record 101 Switching Protocols as final",
			write: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusSwitchingProtocols)
				w.WriteHeader(http.StatusInternalServerError)
			},
			want: http.StatusSwitchingProtocols,
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

			w, status := api.NewStatusWriter(httptest.NewRecorder(), slog.New(slog.DiscardHandler))

			tc.write(w)

			assert.Equal(t, tc.want, status())
		})
	}
}

// http.ResponseController reaches the underlying writer through Unwrap for
// what statusWriter doesn't implement itself (deadlines).
func TestStatusWriter_unwraps_for_ResponseController(t *testing.T) {
	t.Parallel()

	errs := make(chan error, 1)

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		w, _ := api.NewStatusWriter(rw, slog.New(slog.DiscardHandler))

		errs <- http.NewResponseController(w).SetWriteDeadline(time.Now().Add(time.Minute))
	}))
	t.Cleanup(server.Close)

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
	require.NoError(t, err)

	response, err := server.Client().Do(request)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())

	assert.NoError(t, <-errs)
}

// Handlers that check the writer's interfaces directly (server-sent events,
// WebSocket libraries) still find them.
func TestStatusWriter_keeps_the_writer_interfaces(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	w, status := api.NewStatusWriter(recorder, slog.New(slog.DiscardHandler))

	flusher, ok := w.(http.Flusher)
	require.True(t, ok, "not an http.Flusher")

	flusher.Flush()
	assert.True(t, recorder.Flushed)
	assert.Equal(t, http.StatusOK, status())

	_, ok = w.(http.Hijacker)
	assert.True(t, ok, "not an http.Hijacker")

	_, ok = w.(io.ReaderFrom)
	assert.True(t, ok, "not an io.ReaderFrom")
}

// A hijacked connection (a WebSocket upgrade) is recorded as 101.
func TestStatusWriter_records_a_hijacked_connection_as_101(t *testing.T) {
	t.Parallel()

	statuses := make(chan int, 1)

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		w, status := api.NewStatusWriter(rw, slog.New(slog.DiscardHandler))

		hijacker, ok := w.(http.Hijacker)
		if !ok {
			statuses <- 0

			return
		}

		conn, buf, err := hijacker.Hijack()
		if err != nil {
			statuses <- 0

			return
		}

		defer conn.Close()

		_, _ = buf.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: test\r\n\r\n")
		_ = buf.Flush()

		statuses <- status()
	}))
	t.Cleanup(server.Close)

	conn, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", server.Listener.Addr().String())
	require.NoError(t, err)

	defer conn.Close()

	_, err = io.WriteString(conn, "GET / HTTP/1.1\r\nHost: test\r\nConnection: Upgrade\r\nUpgrade: test\r\n\r\n")
	require.NoError(t, err)

	line, err := readLine(conn)
	require.NoError(t, err)
	assert.Equal(t, "HTTP/1.1 101 Switching Protocols", line)
	assert.Equal(t, http.StatusSwitchingProtocols, <-statuses)
}

// readLine reads up to the first CRLF from r, one byte at a time.
func readLine(r io.Reader) (string, error) {
	var line []byte

	b := make([]byte, 1)

	for {
		if _, err := r.Read(b); err != nil {
			return string(line), err
		}

		if b[0] == '\n' {
			return strings.TrimSuffix(string(line), "\r"), nil
		}

		line = append(line, b[0])
	}
}

// A second WriteHeader is dropped, and logged with the handler that made it
// (net/http would name statusWriter instead), past net/http's own helpers
// such as http.Error, as net/http does.
func TestStatusWriter_logs_a_superfluous_WriteHeader_with_its_caller(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		label  string
		second func(w http.ResponseWriter)
	}{
		{
			label:  "should name the handler that called WriteHeader",
			second: func(w http.ResponseWriter) { w.WriteHeader(http.StatusInternalServerError) },
		},
		{
			label:  "should name the handler, not http.Error",
			second: func(w http.ResponseWriter) { http.Error(w, "boom", http.StatusInternalServerError) },
		},
	}

	for _, tc := range testcases {
		t.Run(tc.label, func(t *testing.T) {
			t.Parallel()

			var logs bytes.Buffer

			recorder := httptest.NewRecorder()
			w, status := api.NewStatusWriter(recorder, slog.New(slog.NewTextHandler(&logs, nil)))

			w.WriteHeader(http.StatusCreated)
			tc.second(w)

			assert.Equal(t, http.StatusCreated, status())
			assert.Equal(t, http.StatusCreated, recorder.Code)
			assert.Contains(t, logs.String(), `msg="superfluous WriteHeader call" status=500 sent=201`)
			assert.Contains(t, logs.String(), "TestStatusWriter_logs_a_superfluous_WriteHeader_with_its_caller")
			assert.Contains(t, logs.String(), "status_test.go:")
			assert.NotContains(t, logs.String(), "net/http.")
		})
	}
}

// Through net/http's own writer, ReadFrom sends what Write would: the body
// with an implicit 200, and nothing for an empty source, so a status written
// next still reaches the client.
func TestStatusWriter_ReadFrom_sends_what_Write_would(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		label      string
		body       string
		then       int // status written after the copy
		wantStatus int
	}{
		{
			label:      "should send the body with a 200",
			body:       "body",
			then:       http.StatusInternalServerError,
			wantStatus: http.StatusOK,
		},
		{
			label:      "should send the status written after an empty body",
			body:       "",
			then:       http.StatusNotFound,
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.label, func(t *testing.T) {
			t.Parallel()

			statuses := make(chan int, 1)

			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
				w, status := api.NewStatusWriter(rw, slog.New(slog.DiscardHandler))

				_, _ = io.Copy(w, onlyReader{strings.NewReader(tc.body)})
				w.WriteHeader(tc.then)

				statuses <- status()
			}))
			t.Cleanup(server.Close)

			request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
			require.NoError(t, err)

			response, err := server.Client().Do(request)
			require.NoError(t, err)

			body, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())

			assert.Equal(t, tc.wantStatus, response.StatusCode)
			assert.Equal(t, tc.wantStatus, <-statuses, "recorded status")
			assert.Equal(t, tc.body, string(body))
		})
	}
}
