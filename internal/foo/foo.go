// Package foo defines the Fooer, the domain behind GET /foo, and its real
// implementation.
package foo

import "context"

var _ Fooer = (*Bar)(nil)

// Fooer produces the message served by GET /foo. The error lets callers
// handle a failing implementation; Bar never fails.
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
