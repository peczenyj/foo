#!/usr/bin/env bash
# Exports the lite kit (README "Lite kit"): a smaller copy of the template
# for short, time-boxed work, renamed to the given module and name.
#
# Usage:  scripts/export-lite.sh <dest> <module> <name>
#         (make export-lite DEST=<dir> MODULE=<module> NAME=<name> [FORCE=1])
#
# The kit is the template's files listed in SHARED below, then the kit's own
# files from _kit/ on top (they replace or add: the lite Makefile, AGENTS.md,
# CI files, lean .gitignore and .dockerignore, tests with a hand-written fake
# instead of mocks), and
# .golangci.lite.yml as .golangci.yml. It is assembled and renamed
# (scripts/rename.sh) in a temporary directory, its go.mod tidied, and only
# then copied into <dest> (created if missing): a failure leaves <dest>
# untouched. Files that already exist in <dest> stop the export, unless
# FORCE=1 overwrites them. It lists what it copied, for one commit.
set -euo pipefail

cd "$(dirname "$0")/.."

[[ $# -eq 3 ]] || { echo "usage: $0 <dest> <module> <name>" >&2; exit 2; }
dest=$1 module=$2 name=$3

# The template's files the kit uses as they are.
SHARED=(
  CLAUDE.md
  Dockerfile
  go.mod
  go.sum
  cmd/server/main.go
  cmd/server/main_test.go
  cmd/server/admin_test.go
  internal/config/config.go
  internal/config/config_test.go
  internal/config/export_test.go
  internal/config/secret.go
  internal/config/secret_test.go
  internal/foo/foo.go
  internal/foo/foo_test.go
  internal/admin/admin.go
  internal/admin/admin_test.go
  internal/metrics/metrics.go
  internal/metrics/metrics_test.go
  internal/metrics/export_test.go
  internal/middleware/middleware.go
  internal/middleware/headers.go
  internal/middleware/status.go
  internal/middleware/metrics.go
  internal/middleware/recover.go
  internal/middleware/export_test.go
  internal/middleware/chain_test.go
  internal/middleware/headers_test.go
  internal/middleware/status_test.go
  internal/middleware/metrics_test.go
  internal/middleware/recover_test.go
  internal/problem/problem.go
  internal/problem/problem_test.go
  internal/api/handler.go
  internal/api/problem.go
  internal/api/helpers_test.go
  internal/api/export_test.go
  scripts/check-tool.sh
)

staging=$(mktemp -d)
trap 'rm -rf "$staging"' EXIT

for f in "${SHARED[@]}"; do
  mkdir -p "$staging/$(dirname "$f")"
  cp "$f" "$staging/$f"
done
cp -R _kit/. "$staging/"

# .golangci.lite.yml as .golangci.yml, its settings unchanged; its header
# (the comments before "version:") speaks of the two profiles, so the kit gets
# its own.
{
  cat <<'HEADER'
# Lint configuration: the template's lite profile, which keeps what finds bugs
# and drops what enforces style (wsl_v5, lll, mnd, funlen, revive…), the
# error-wrapping policy and the test-structure rules. make lint runs it;
# make fmt applies the formatters.
HEADER
  awk 'started || /^version:/ { started = 1; print }' .golangci.lite.yml
} >"$staging/.golangci.yml"

# The rename, run on the staged copy: rename.sh works on the directory it
# lives in. Its summary is about the template; its warnings are kept.
mkdir -p "$staging/scripts"
cp scripts/rename.sh "$staging/scripts/rename.sh"
bash "$staging/scripts/rename.sh" "$module" "$name" >/dev/null
rm "$staging/scripts/rename.sh"

# The kit leaves out the template's OpenAPI contract test and mocks: tidy
# drops what only they needed (kin-openapi).
(cd "$staging" && go mod tidy)

# Portable to macOS's bash 3.2: no mapfile, no empty "${array[@]}" under set -u.
files=()
while IFS= read -r f; do files+=("$f"); done < <(cd "$staging" && find . -type f | sed 's|^\./||' | LC_ALL=C sort)

existing=()
for f in "${files[@]}"; do
  if [[ -e "$dest/$f" ]]; then existing+=("$f"); fi
done
if [[ ${#existing[@]} -gt 0 && "${FORCE:-}" != 1 ]]; then
  echo "error: these files already exist in $dest; nothing was copied (FORCE=1 overwrites them):" >&2
  printf '  %s\n' "${existing[@]}" >&2
  exit 1
fi

mkdir -p "$dest"
cp -R "$staging/." "$dest/"

echo "exported the lite kit as $module ($name) into $dest:"
for f in "${files[@]}"; do
  overwritten=""
  for e in ${existing[@]+"${existing[@]}"}; do
    if [[ "$e" == "$f" ]]; then overwritten=" (overwritten)"; fi
  done
  echo "  $f$overwritten"
done
cat <<MSG
Next: cd $dest && make ci, then commit these files ("project skeleton").
MSG
