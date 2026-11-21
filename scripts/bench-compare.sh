#!/usr/bin/env bash
# Compares the benchmarks of a git ref (before) with the working tree (after),
# uncommitted changes included, and prints benchstat's comparison.
#
# Usage:  scripts/bench-compare.sh [base-ref]      (default: devel)
# Env:    BENCH (benchmark filter, default .), BENCH_COUNT (runs, default 6;
#         benchstat needs at least 6 for a confidence interval)
#
# The base ref is checked out in a temporary git worktree, so the working
# tree is never touched. Results stay in bench-base.out and bench.out.
set -euo pipefail

cd "$(dirname "$0")/.."

base="${1:-devel}"
bench="${BENCH:-.}"
count="${BENCH_COUNT:-6}"

git rev-parse --verify --quiet "${base}^{commit}" >/dev/null || {
  echo "unknown git ref: $base" >&2
  exit 1
}

tmp=$(mktemp -d)
cleanup() {
  git worktree remove --force "$tmp/base" >/dev/null 2>&1 || true
  rm -rf "$tmp"
}
trap cleanup EXIT

git worktree add --detach --quiet "$tmp/base" "$base"

# run <dir> <output>: benchmarks of the module in <dir>, raw output to <output>.
run() {
  if ! (cd "$1" && go test -run='^$' -bench="$bench" -benchmem -count="$count" ./...) >"$2" 2>&1; then
    cat "$2" >&2
    echo "benchmarks failed in $1" >&2
    exit 1
  fi
}

echo "benchmarking $base ($count runs)..." >&2
run "$tmp/base" bench-base.out

echo "benchmarking the working tree ($count runs)..." >&2
run . bench.out

grep -q '^Benchmark' bench-base.out ||
  echo "warning: $base has no benchmarks matching '$bench': nothing to compare against" >&2

# benchstat is pinned in the tools module (tools/go.mod).
scripts/tool.sh benchstat "$base=bench-base.out" "working-tree=bench.out"
