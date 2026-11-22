#!/usr/bin/env bash
# Checks that a developer-installed tool is on the PATH, at the version CI uses.
#
# Usage:  scripts/check-tool.sh <make-var> <binary> <module> <version> <install-hint>
#
# Missing binary: error with the install hint (exit 1). Other version: a
# warning only, since results may differ but usually don't. The version is
# read from the binary's Go build info (go version -m), which every tool here
# has, whatever way it was installed.
set -euo pipefail

var=$1 bin=$2 mod=$3 want=$4 hint=$5

path=$(command -v "$bin" 2>/dev/null) || {
  echo "error: $bin not found." >&2
  echo "Install $mod $want, the version CI uses:" >&2
  echo "  $hint" >&2
  echo "or point to another binary: make <target> $var=/path/to/$bin" >&2
  exit 1
}

have=$("${GO:-go}" version -m "$path" 2>/dev/null | awk -v m="$mod" '$1 == "mod" && $2 == m { print $3 }')

if [[ -z "$have" ]]; then
  echo "warning: cannot read the version of $path; CI uses $want ($hint)" >&2
elif [[ "$have" != "$want" ]]; then
  echo "warning: $bin $have is installed, CI uses $want: results may differ ($hint)" >&2
fi
