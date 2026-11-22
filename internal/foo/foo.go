// Package foo defines the Fooer, the domain behind GET /foo, and its real
// implementation.
package foo

import (
	"context"
	"errors"
)

var _ Fooer = (*Bar)(nil)

// ErrInvalidInput is wrapped by a Fooer when its input is invalid. The API
// answers 400 with the error's text as the problem detail, so wrap it only
// with text that is safe to show to the client:
//
//	fmt.Errorf("%w: name is too long", foo.ErrInvalidInput)
var ErrInvalidInput = errors.New("invalid input")

// Fooer produces the message served by GET /foo. The error lets callers
// handle a failing implementation (ErrInvalidInput for a client mistake,
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
