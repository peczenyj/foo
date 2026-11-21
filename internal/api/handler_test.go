package api_test

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/peczenyj/foo/internal/api"
	foomock "github.com/peczenyj/foo/mocks/github.com/peczenyj/foo/foo"
)

const wantContentType = "application/json"

func TestAPI(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		label   string
		setup   func(m *foomock.Fooer) // nil: Foo must not be called
		method  string
		target  string
		status  int
		message string // expected MessageBody.Message, for 200 on /foo only
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
			logged: "secret db failure",
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

			logger := slog.New(slog.NewTextHandler(&logs, nil))

			status, body, headers := doRequest(t, api.New(fooer, logger), tc.method, tc.target)

			assert.Equal(t, tc.status, status)

			switch {
			case tc.message != "":
				assert.Equal(t, wantContentType, headers.Get("Content-Type"))
				assert.Equal(t, api.MessageBody{Message: tc.message}, decodeJSON[api.MessageBody](t, body))
			case status >= http.StatusBadRequest:
				// JSON only on success: errors keep the default response.
				assert.NotEqual(t, wantContentType, headers.Get("Content-Type"))
				assert.NotContains(t, string(body), "secret")
			default:
				assert.Empty(t, body)
			}

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
