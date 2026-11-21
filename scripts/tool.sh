#!/usr/bin/env sh
# Runs a developer tool pinned in tools/go.mod (mockery, gotestsum,
# benchstat) from the current directory, with its arguments.
#
# Usage:  scripts/tool.sh <tool> [args...]
#         e.g. scripts/tool.sh gotestsum -- -race -run TestAPI ./internal/api/
#
# The tools live in their own module so their dependencies stay out of the
# service's go.mod. `go tool -modfile=tools/go.mod` would run them from here,
# but it checks the service's vendor/ against tools/go.mod and fails once
# make vendor has run. Instead, go builds the tool inside tools/ (cached
# after the first run) and prints its path, and the binary runs here.
set -eu

[ $# -ge 1 ] || {
  echo "usage: $0 <tool> [args...]" >&2
  exit 2
}

go=${GO:-go}
name=$1
shift

bin=$("$go" -C "$(dirname "$0")/../tools" tool -n "$name")

# As `go tool` does: the tool runs with the same go first in PATH.
PATH="$("$go" env GOROOT)/bin:$PATH"
export PATH

exec "$bin" "$@"
