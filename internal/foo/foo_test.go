package foo_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/peczenyj/foo/internal/foo"
)

func TestBar_Foo(t *testing.T) {
	t.Parallel()

	got, err := foo.New().Foo(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "bar", got)
}

func TestInputError(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("checking account 42: %w", &foo.InputError{Detail: "empty name"})

	require.EqualError(t, err, "checking account 42: invalid input: empty name")

	var inputErr *foo.InputError

	require.ErrorAs(t, err, &inputErr)
	assert.Equal(t, "empty name", inputErr.Detail, "Detail stays free of the wrappers' context")
}
