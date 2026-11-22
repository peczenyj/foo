#!/usr/bin/env bash
# Checks that the lite lint profile is a subset of the full one: every linter
# and formatter that .golangci.lite.yml enables, .golangci.yml enables too.
# Code that passes full then passes lite (their shared settings are kept
# identical by hand; make lint-profiles also runs lite to prove it).
#
# Usage:  scripts/lint-profiles.sh   (GOLANGCI_LINT: the binary, default golangci-lint)
set -euo pipefail

cd "$(dirname "$0")/.."

lint="${GOLANGCI_LINT:-golangci-lint}"
full=.golangci.yml
lite=.golangci.lite.yml

# enabled <linters|formatters> <config>: the names golangci-lint enables, one per line.
enabled() {
  "$lint" "$1" --config "$2" | awk '/^Enabled by your configuration/ { on = 1; next } /^$|^Disabled/ { on = 0 } on { sub(/:.*/, ""); print }' | sort
}

status=0
for kind in linters formatters; do
  extra=$(comm -13 <(enabled "$kind" "$full") <(enabled "$kind" "$lite"))
  if [[ -n "$extra" ]]; then
    echo "error: $lite enables $kind that $full does not:" >&2
    sed 's/^/  /' <<<"$extra" >&2
    status=1
  fi
done

[[ $status -eq 0 ]] && echo "$lite is a subset of $full ($(enabled linters "$lite" | wc -l | tr -d ' ') of $(enabled linters "$full" | wc -l | tr -d ' ') linters)"
exit $status
