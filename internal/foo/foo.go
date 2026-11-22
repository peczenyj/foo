// Package foo defines the Fooer, the domain behind GET /foo, and its real
// implementation.
package foo

import "context"

var _ Fooer = (*Bar)(nil)

// InputError is returned (wrapped or not) by a Fooer when its input is
// invalid: a client mistake. The API answers 400 with Detail as the problem
// detail, and only Detail: text added by wrappers up the chain reaches the
// logs, never the client. Detail must be safe to show to the client:
//
//	return "", &foo.InputError{Detail: "name is too long"}
type InputError struct {
	Detail string // what the client got wrong, safe to send
}

// Error implements error.
func (e *InputError) Error() string { return "invalid input: " + e.Detail }

// Fooer produces the message served by GET /foo. The error lets callers
// handle a failing implementation (*InputError for a client mistake,
// anything else for a failure); Bar never fails.
type Fooer interface {
	Foo(ctx context.Context) (string, error)
}

// Bar is the real Fooer: it always answers "bar". Create it with New.
type Bar struct{}

// New returns the real Fooer. Dependencies a real implementation needs
// (clients, settings) would be its parameters, so callers build a Bar in
// one place only.
func New() *Bar {
	return &Bar{}
}

// Foo implements Fooer.
func (*Bar) Foo(context.Context) (string, error) {
	return "bar", nil
}
