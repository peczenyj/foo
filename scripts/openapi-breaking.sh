#!/usr/bin/env bash
# Fails when api/openapi.yaml breaks clients generated from the last release:
# oasdiff compares it with the same file at the baseline and fails on any
# ERR-level change (a removed operation or field, a changed type, a new
# required parameter...). A deliberate break is listed in
# api/oasdiff-ignore.txt, in the same pull request, so a reviewer sees it.
#
# Usage:  bash scripts/openapi-breaking.sh [ref]       compare with ref
#         bash scripts/openapi-breaking.sh --print-base print the baseline
#
# The baseline is the highest final release tag, vX.Y.Z (never a prerelease,
# never git describe: release tags sit on main's merge commits, which devel
# does not contain). No baseline, or a baseline without the spec (v0.3.0 and
# older), is not an error: there is nothing to break yet.
# Env: OASDIFF (the oasdiff binary, default oasdiff).
set -euo pipefail

cd "$(dirname "$0")/.."

spec=api/openapi.yaml
ignore=api/oasdiff-ignore.txt
oasdiff=${OASDIFF:-oasdiff}

# baseline: the highest vX.Y.Z tag, or nothing. awk reads every line, so git
# never gets a SIGPIPE.
baseline() {
  git tag -l 'v*' --sort=-v:refname | awk '/^v[0-9]+\.[0-9]+\.[0-9]+$/ && !found { print; found = 1 }'
}

# A copy made with gonew is not a git repository yet: no tags to compare with.
if ! git rev-parse --git-dir >/dev/null 2>&1; then
  echo "not a git repository: no breaking-change check"
  exit 0
fi

if [[ "${1:-}" == --print-base ]]; then
  baseline
  exit 0
fi

base=${1:-$(baseline)}

if [[ -z "$base" ]]; then
  echo "no release tag (vX.Y.Z) yet: no breaking-change check"
  exit 0
fi

git rev-parse --verify --quiet "$base^{commit}" >/dev/null || { echo "error: unknown git ref: $base" >&2; exit 2; }

if ! git cat-file -e "$base:$spec" 2>/dev/null; then
  echo "$base has no $spec: no breaking-change check"
  exit 0
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
old="$tmp/${base//\//_}-openapi.yaml"
git show "$base:$spec" >"$old"

echo "checking $spec against $base for breaking changes"
"$oasdiff" breaking "$old" "$spec" --fail-on ERR --err-ignore "$ignore"
