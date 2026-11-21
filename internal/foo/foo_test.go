package foo_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/peczenyj/foo/internal/foo"
)

func TestBar_Foo(t *testing.T) {
	t.Parallel()

	got, err := foo.Bar{}.Foo(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "bar", got)
}
