#!/usr/bin/env bash
# Runs the hurl suite (test/hurl/*.hurl).
#
# Usage:  scripts/hurl.sh             build bin/foo, start it on a free port, test it, stop it
#         scripts/hurl.sh <base-url>  test an already running server, e.g. http://localhost:8080
set -euo pipefail

cd "$(dirname "$0")/.."

command -v hurl >/dev/null || { echo "hurl not found: https://hurl.dev/docs/installation.html" >&2; exit 1; }

run_hurl() {
  hurl --test --variable "base_url=$1" test/hurl/*.hurl
}

if [[ $# -gt 0 ]]; then
  run_hurl "$1"
  exit
fi

# make test-functional builds bin/foo as a prerequisite (no recursive make);
# run on its own, the script builds it.
[[ -n "${MAKELEVEL:-}" ]] || make --no-print-directory build

log=$(mktemp)
pid=""

cleanup() {
  if [[ -n "$pid" ]]; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
  rm -f "$log"
}
trap cleanup EXIT

# Port 0: the kernel picks a free port; the server logs the one it bound.
# -log-level=info: the "listening" line is an info log, whatever FOO_LOG_LEVEL says.
bin/foo -host=127.0.0.1 -port=0 -log-format=json -log-level=info 2>"$log" &
pid=$!

addr=""
for _ in $(seq 1 100); do
  addr=$(grep -o '"msg":"listening","addr":"[^"]*"' "$log" | sed 's/.*"addr":"\([^"]*\)"/\1/' || true)
  [[ -n "$addr" ]] && break
  kill -0 "$pid" 2>/dev/null || { echo "server exited:" >&2; cat "$log" >&2; exit 1; }
  sleep 0.1
done

[[ -n "$addr" ]] || { echo "server did not start listening within 10s:" >&2; cat "$log" >&2; exit 1; }

run_hurl "http://$addr"
