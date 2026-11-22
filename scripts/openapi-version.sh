#!/usr/bin/env bash
# Checks that the OpenAPI spec's info.version matches a release tag: the tag
# without its leading "v" and without any prerelease suffix (v0.4.0 and
# v0.4.0-rc.1 both expect 0.4.0). CI runs it on tag pushes, and the release
# workflow before attaching the spec, so a release never ships a spec that
# names another version.
#
# Usage:  bash scripts/openapi-version.sh <tag> [spec]   (spec: api/openapi.yaml)
set -euo pipefail

cd "$(dirname "$0")/.."

[[ $# -ge 1 && $# -le 2 ]] || { echo "usage: $0 <tag> [spec]" >&2; exit 2; }
tag=$1 spec=${2:-api/openapi.yaml}

[[ "$tag" =~ ^v([0-9]+\.[0-9]+\.[0-9]+)(-[0-9A-Za-z.-]+)?$ ]] || {
  echo "error: $tag is not a release tag (vX.Y.Z or vX.Y.Z-prerelease)" >&2; exit 2; }
want=${BASH_REMATCH[1]}

[[ -f "$spec" ]] || { echo "error: $spec not found" >&2; exit 1; }

# info.version: the version key indented under the top-level info block.
have=$(awk '
  /^[^[:space:]#]/ { in_info = ($0 ~ /^info:/) }
  in_info && /^[[:space:]]+version:/ {
    sub(/^[[:space:]]+version:[[:space:]]*/, ""); sub(/[[:space:]]+#.*$/, ""); gsub(/["\047]/, "")
    print; exit
  }' "$spec")

[[ -n "$have" ]] || { echo "error: no info.version in $spec" >&2; exit 1; }

if [[ "$have" != "$want" ]]; then
  echo "error: $spec has info.version $have, but the tag $tag needs $want: bump it in the release pull request" >&2
  exit 1
fi

echo "$spec: info.version $have matches $tag"
