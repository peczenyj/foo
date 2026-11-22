package api_test

import (
	"context"
	"sync/atomic"

	"github.com/peczenyj/foo/internal/foo"
)

// The build fails if fakeFooer stops implementing foo.Fooer.
var _ foo.Fooer = (*fakeFooer)(nil)

// fakeFooer is a hand-written foo.Fooer: Foo returns message and err, and
// counts its calls, so a test can check how often the handler asked. The
// counter is atomic: a server under test calls Foo on its own goroutine.
type fakeFooer struct {
	message string
	err     error
	calls   atomic.Int32
}

func (f *fakeFooer) Foo(context.Context) (string, error) {
	f.calls.Add(1)

	return f.message, f.err
}
