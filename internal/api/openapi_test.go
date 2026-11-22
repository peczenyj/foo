package api_test

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/peczenyj/foo/internal/api"
	"github.com/peczenyj/foo/internal/foo"
	foomock "github.com/peczenyj/foo/mocks/github.com/peczenyj/foo/foo"
)

// specPath is the OpenAPI spec, the contract clients generate code from.
const specPath = "../../api/openapi.yaml"

// contract is the loaded spec and a router that finds its operations.
type contract struct {
	doc    *openapi3.T
	router routers.Router
}

// loadContract loads api/openapi.yaml and fails the test if it is invalid.
func loadContract(t *testing.T) contract {
	t.Helper()

	doc, err := openapi3.NewLoader().LoadFromFile(specPath)
	require.NoError(t, err, "loading %s", specPath)
	require.NoError(t, doc.Validate(t.Context()), "validating %s", specPath)

	// The spec's server (http://localhost:8080) documents where clients
	// connect; tests send requests to any host, so match on paths only.
	doc.Servers = nil

	router, err := legacy.NewRouter(doc)
	require.NoError(t, err)

	return contract{doc: doc, router: router}
}

// operations returns every operation in the spec as "METHOD /path" (the
// http.ServeMux pattern syntax) mapped to its operationId.
func (c contract) operations() map[string]string {
	ops := make(map[string]string)

	for path, item := range c.doc.Paths.Map() {
		for method, op := range item.Operations() {
			ops[method+" "+path] = op.OperationID
		}
	}

	return ops
}

// check validates one response against the spec and returns the key of the
// documented response it matched ("getFoo 200"). A request that matches no
// operation must be the router's own 404 or 405, which the spec leaves out
// on purpose: it returns "" without validating the body.
func (c contract) check(t *testing.T, method, target string, status int, header http.Header, body []byte) string {
	t.Helper()

	request := httptest.NewRequestWithContext(t.Context(), method, target, nil)

	route, pathParams, err := c.router.FindRoute(request)
	if err == nil && strings.HasSuffix(request.URL.Path, "/") != strings.HasSuffix(route.Path, "/") {
		// The legacy router matches /foo/ to /foo; OpenAPI and http.ServeMux
		// treat them as different paths, so neither does this check.
		err = routers.ErrPathNotFound
	}

	if err != nil {
		require.Contains(t, []int{http.StatusNotFound, http.StatusMethodNotAllowed}, status,
			"%s %s matches no operation in %s (%v): only the router's 404 and 405 may answer it",
			method, target, specPath, err)

		return ""
	}

	err = openapi3filter.ValidateResponse(t.Context(), &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{
			Request:    request,
			PathParams: pathParams,
			Route:      route,
		},
		Status: status,
		Header: header,
		Body:   io.NopCloser(bytes.NewReader(body)),
		// Without it, a status the spec does not list passes silently.
		Options: &openapi3filter.Options{IncludeResponseStatus: true},
	})
	require.NoError(t, err, "%s %s answered %d, which %s does not describe", method, target, status, specPath)

	return route.Operation.OperationID + " " + strconv.Itoa(status)
}

// documented returns every documented response as "operationId status".
func (c contract) documented() []string {
	var keys []string

	for _, item := range c.doc.Paths.Map() {
		for _, op := range item.Operations() {
			for status := range op.Responses.Map() {
				keys = append(keys, op.OperationID+" "+status)
			}
		}
	}

	return keys
}

// TestOpenAPI_routes_match_spec: the routes api.New registers and the
// operations the spec describes are the same set, both ways.
func TestOpenAPI_routes_match_spec(t *testing.T) {
	t.Parallel()

	spec := loadContract(t)

	described := make([]string, 0, len(spec.operations()))
	for pattern := range spec.operations() {
		described = append(described, pattern)
	}

	routes := api.New(foo.New(), slog.New(slog.DiscardHandler)).Routes()

	assert.ElementsMatch(t, described, routes,
		"the routes registered in internal/api and the operations in %s differ", specPath)
}

// TestOpenAPI_responses_match_spec: every response the spec documents is
// produced by the real handler and matches the spec, so the spec describes
// no response the service never sends.
func TestOpenAPI_responses_match_spec(t *testing.T) {
	t.Parallel()

	spec := loadContract(t)

	failing := func(err error) foo.Fooer {
		m := foomock.NewFooer(t)
		m.EXPECT().Foo(mock.Anything).Return("", err).Once()

		return m
	}

	testcases := []struct {
		label  string
		fooer  foo.Fooer
		target string
	}{
		{label: "getFoo 200", fooer: foo.New(), target: "/foo"},
		{label: "getFoo 400", fooer: failing(&foo.InputError{Detail: "name is too long"}), target: "/foo"},
		{label: "getFoo 500", fooer: failing(errors.New("db down")), target: "/foo"},
		{label: "getHealthz 200", fooer: foo.New(), target: "/healthz"},
	}

	produced := make([]string, 0, len(testcases))

	for _, tc := range testcases {
		handler := api.New(tc.fooer, slog.New(slog.DiscardHandler))
		status, body, headers := doRequest(t, handler, http.MethodGet, tc.target)

		key := spec.check(t, http.MethodGet, tc.target, status, headers, body)
		assert.Equal(t, tc.label, key)

		produced = append(produced, key)
	}

	assert.ElementsMatch(t, spec.documented(), produced,
		"every response in %s must be produced by a case here, and every case must be documented", specPath)
}

// TestOpenAPI_media_types: how kin-openapi matches Content-Type. It ignores
// parameters (application/json; charset=utf-8 matches application/json), so
// the exact media types are asserted by the handler tests; a different media
// type fails validation.
func TestOpenAPI_media_types(t *testing.T) {
	t.Parallel()

	spec := loadContract(t)
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/foo", nil)

	route, pathParams, err := spec.router.FindRoute(request)
	require.NoError(t, err)

	validate := func(contentType string) error {
		return openapi3filter.ValidateResponse(t.Context(), &openapi3filter.ResponseValidationInput{
			RequestValidationInput: &openapi3filter.RequestValidationInput{
				Request:    request,
				PathParams: pathParams,
				Route:      route,
			},
			Status:  http.StatusOK,
			Header:  http.Header{"Content-Type": {contentType}},
			Body:    io.NopCloser(bytes.NewReader([]byte(`{"message":"bar"}`))),
			Options: &openapi3filter.Options{IncludeResponseStatus: true},
		})
	}

	require.NoError(t, validate("application/json"))
	require.NoError(t, validate("application/json; charset=utf-8"), "parameters are ignored when matching")
	require.Error(t, validate("text/plain; charset=utf-8"), "another media type must fail")
	require.Error(t, validate("application/problem+json"), "a problem type on a 200 must fail")
}
