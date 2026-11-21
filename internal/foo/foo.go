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

// Bar is the real Fooer: it always answers "bar". Use it as a pointer,
// &Bar{}: only *Bar implements Fooer.
type Bar struct{}

// Foo implements Fooer.
func (*Bar) Foo(context.Context) (string, error) {
	return "bar", nil
}
