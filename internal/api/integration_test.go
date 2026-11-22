//go:build integration

package api_test

import "testing"

// Integration tests exercise the service against real external dependencies
// (a database, a cache, another HTTP service), usually started in Docker by
// testcontainers-go or dockertest. They only build with the integration tag
// and run with make test-integration; name them TestIntegration_*.
//
// The service has no external dependency yet: this example only shows the
// convention, and skips.
func TestIntegration_example(t *testing.T) {
	t.Parallel()

	t.Skip("example only: the service has no external dependency to integrate with yet")
}
