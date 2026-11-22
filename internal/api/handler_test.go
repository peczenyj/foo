package api_test

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/peczenyj/foo/internal/api"
	"github.com/peczenyj/foo/internal/foo"
	foomock "github.com/peczenyj/foo/mocks/github.com/peczenyj/foo/foo"
)

const (
	wantContentType  = "application/json"
	problemMediaType = "application/problem+json"
	// The router's own 404 and 405 keep net/http's plain-text body.
	plainTextMediaType = "text/plain; charset=utf-8"
)

func TestAPI(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		label   string
		setup   func(m *foomock.Fooer) // nil: Foo must not be called
		method  string
		target  string
		status  int
		message string // expected MessageBody.Message, for 200 on /foo only
		detail  string // expected problem detail; empty: not a problem response
		allow   string // expected Allow header, for 405 only
		logged  string // expected substring of the logs; empty: nothing logged
	}{
		{
			label:   "should reply the Fooer message",
			setup:   func(m *foomock.Fooer) { m.EXPECT().Foo(mock.Anything).Return("bar", nil).Once() },
			method:  http.MethodGet,
			target:  "/foo",
			status:  http.StatusOK,
			message: "bar",
		},
		{
			label:   "should reply whatever the Fooer returns",
			setup:   func(m *foomock.Fooer) { m.EXPECT().Foo(mock.Anything).Return("baz", nil).Once() },
			method:  http.MethodGet,
			target:  "/foo",
			status:  http.StatusOK,
			message: "baz",
		},
		{
			label: "should hide the Fooer error",
			setup: func(m *foomock.Fooer) {
				m.EXPECT().Foo(mock.Anything).Return("", errors.New("secret db failure")).Once()
			},
			method: http.MethodGet,
			target: "/foo",
			status: http.StatusInternalServerError,
			detail: "the server could not complete the request",
			logged: "secret db failure",
		},
		{
			label: "should reply 400 with the detail of an input error",
			setup: func(m *foomock.Fooer) {
				m.EXPECT().Foo(mock.Anything).Return("", &foo.InputError{Detail: "name is too long"}).Once()
			},
			method: http.MethodGet,
			target: "/foo",
			status: http.StatusBadRequest,
			detail: "name is too long",
			logged: "level=DEBUG msg=\"request rejected\" status=400 error=\"invalid input: name is too long\"",
		},
		{
			label: "should send only the input error's detail, never its wrappers",
			setup: func(m *foomock.Fooer) {
				// Internal context added up the chain reaches the logs, not the client.
				err := fmt.Errorf("account 42 in secret shard eu-3: %w", &foo.InputError{Detail: "empty name"})
				m.EXPECT().Foo(mock.Anything).Return("", err).Once()
			},
			method: http.MethodGet,
			target: "/foo",
			status: http.StatusBadRequest,
			detail: "empty name",
			logged: "account 42 in secret shard eu-3: invalid input: empty name",
		},
		{
			label:  "should return 404 for an unknown path",
			method: http.MethodGet,
			target: "/bar",
			status: http.StatusNotFound,
		},
		{
			label:  "should return 404 for a trailing slash",
			method: http.MethodGet,
			target: "/foo/",
			status: http.StatusNotFound,
		},
		{
			label:  "should be case sensitive",
			method: http.MethodGet,
			target: "/FOO",
			status: http.StatusNotFound,
		},
		{
			label:  "should reject POST /foo",
			method: http.MethodPost,
			target: "/foo",
			status: http.StatusMethodNotAllowed,
			allow:  "GET, HEAD",
		},
		{
			label:  "should reject DELETE /healthz",
			method: http.MethodDelete,
			target: "/healthz",
			status: http.StatusMethodNotAllowed,
			allow:  "GET, HEAD",
		},
		{
			label:  "should report healthy",
			method: http.MethodGet,
			target: "/healthz",
			status: http.StatusOK,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.label, func(t *testing.T) {
			t.Parallel()

			// NewFooer asserts the expectations on cleanup: a case
			// without setup fails if Foo is called.
			fooer := foomock.NewFooer(t)
			if tc.setup != nil {
				tc.setup(fooer)
			}

			var logs bytes.Buffer

			// Debug level: client errors are logged at debug, with the full chain.
			logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

			status, body, headers := doRequest(t, api.New(fooer, logger), tc.method, tc.target)

			assert.Equal(t, tc.status, status)

			switch {
			case tc.message != "":
				assert.Equal(t, wantContentType, headers.Get("Content-Type"))
				assert.Equal(t, api.MessageBody{Message: tc.message}, decodeJSON[api.MessageBody](t, body))
			case tc.detail != "":
				// Handler errors are RFC 9457 problem details.
				assert.Equal(t, problemMediaType, headers.Get("Content-Type"))
				assert.Equal(t, "nosniff", headers.Get("X-Content-Type-Options"))
				assert.Equal(t, api.Problem{
					Type:   "about:blank",
					Title:  http.StatusText(tc.status),
					Status: tc.status,
					Detail: tc.detail,
				}, decodeJSON[api.Problem](t, body))
			case status >= http.StatusBadRequest:
				// The router's 404 and 405 keep net/http's plain text.
				assert.Equal(t, plainTextMediaType, headers.Get("Content-Type"))
			default:
				assert.Empty(t, body)
			}

			// An internal error never reaches the client.
			assert.NotContains(t, string(body), "secret")

			// The error the client never sees must reach the logs.
			if tc.logged != "" {
				assert.Contains(t, logs.String(), tc.logged)
			} else {
				assert.Empty(t, logs.String())
			}

			if tc.allow != "" {
				assert.Equal(t, tc.allow, headers.Get("Allow"))
			}
		})
	}
}

func TestNew_panics_on_nil_Fooer(t *testing.T) {
	t.Parallel()

	assert.PanicsWithValue(t, "api.New: nil Fooer", func() { api.New(nil, slog.Default()) })
}

func TestNew_panics_on_nil_Logger(t *testing.T) {
	t.Parallel()

	assert.PanicsWithValue(t, "api.New: nil Logger", func() { api.New(foomock.NewFooer(t), nil) })
}
