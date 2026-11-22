#!/usr/bin/env bash
# Adapts the template to another module and project name: rewrites the module
# path and the few places that hold the name (README "Adapting the template").
#
# Usage:  scripts/rename.sh <module> <name>
#         (make rename MODULE=<module> NAME=<name> also regenerates the mocks
#         and tidies go.mod afterwards)
#
# - <module>: the new Go module path, e.g. example.com/acme/widget or just
#   widget. go.mod is left alone when its module line already says so (an
#   existing repository, or a copy made with gonew), and rewritten otherwise.
#   The old module path is read from the template's own imports, not from
#   go.mod, so an existing repository's go.mod doesn't hide it.
# - <name>: the new project name, e.g. widget: binaries, image, programName;
#   the environment variable prefix is the name upper-cased, "-" → "_".
#
# Running it twice with the same arguments changes nothing the second time.
# Portable: plain awk, no GNU-only sed -i.
set -euo pipefail

cd "$(dirname "$0")/.."

[[ $# -eq 2 ]] || { echo "usage: $0 <module> <name>" >&2; exit 2; }
new_module=$1 new_name=$2

[[ "$new_module" =~ ^[a-z0-9][A-Za-z0-9._~-]*(/[A-Za-z0-9._~-]+)*$ ]] || {
  echo "error: \"$new_module\" is not a module path like example.com/acme/widget" >&2; exit 2; }
[[ "$new_name" =~ ^[a-z][a-z0-9-]*$ ]] || {
  echo "error: \"$new_name\" must be lower case letters, digits and \"-\", starting with a letter" >&2; exit 2; }

# The template's module path. Not from go.mod: an existing repository's go.mod
# already holds the new module. First from .mockery.yaml's internal/foo key,
# which gonew leaves alone (it rewrites go.mod and the Go imports, not YAML
# or docs); then from the import of internal/config in cmd/server/main.go.
old_module=$(awk '/^  [^ #][^:]*\/internal\/foo:[ \t]*$/ { sub(/^  /, ""); sub(/\/internal\/foo:.*$/, ""); print; exit }' .mockery.yaml 2>/dev/null || true)
[[ -n "$old_module" ]] ||
  old_module=$(awk -F'"' '$2 ~ /\/internal\/config$/ { sub(/\/internal\/config$/, "", $2); print $2; exit }' cmd/server/main.go)
gomod_module=$(awk '/^module / { print $2; exit }' go.mod)
old_name=$(awk '/^NAME[ \t]*[?]=/ { print $3; exit }' Makefile)
[[ -n "$old_module" && -n "$gomod_module" && -n "$old_name" ]] || {
  echo "error: cannot read the template's module (cmd/server/main.go imports), go.mod's module or NAME (Makefile)" >&2; exit 1; }

# github_repo <module>: owner/repo for a github.com module (a /vN suffix or a
# subdirectory dropped), empty for any other host.
github_repo() {
  [[ "$1" =~ ^github\.com/([^/]+)/([^/]+) ]] && echo "${BASH_REMATCH[1]}/${BASH_REMATCH[2]}"
  return 0
}
old_repo=$(github_repo "$old_module")
new_repo=$(github_repo "$new_module")
# source_url <module> <owner/repo>: the project's source URL, for the image
# label and the spec's contact: the repository when it is on GitHub, else the
# module path without a /vN suffix.
source_url() {
  if [[ -n "$2" ]]; then
    echo "https://github.com/$2"
  elif [[ "$1" =~ ^(.+)/v[0-9]+$ ]]; then
    echo "https://${BASH_REMATCH[1]}"
  else
    echo "https://$1"
  fi
}
old_source=$(source_url "$old_module" "$old_repo")
new_source=$(source_url "$new_module" "$new_repo")
new_prefix=$(tr 'a-z-' 'A-Z_' <<<"$new_name")_
warnings=()

changed=()

# write <file> <tmp>: replace <file> by <tmp> if they differ, keeping its mode.
write() {
  if cmp -s "$1" "$2"; then
    rm -f "$2"
  else
    cat "$2" >"$1" && rm -f "$2"
    changed+=("$1")
  fi
}

# literal <file> <old> <new>...: replace every literal occurrence of each old
# value by its new value, in a single pass. At each position the earliest
# match wins (the longest on a tie), and replaced text is never scanned
# again, so a new value that contains an old one (foo → foo-api, foo →
# foo/v2) is not replaced twice. Values go through the environment: awk -v
# would interpret backslashes.
literal() {
  local file=$1 n=0
  shift
  while [[ $# -ge 2 ]]; do
    n=$((n + 1))
    export "RENAME_OLD_$n=$1" "RENAME_NEW_$n=$2"
    shift 2
  done
  RENAME_PAIRS=$n awk '
    BEGIN { n = ENVIRON["RENAME_PAIRS"]; for (k = 1; k <= n; k++) { old[k] = ENVIRON["RENAME_OLD_" k]; new[k] = ENVIRON["RENAME_NEW_" k] } }
    {
      rest = $0; out = ""
      while (1) {
        best = 0; at = 0
        for (k = 1; k <= n; k++) {
          i = index(rest, old[k])
          if (i > 0 && (at == 0 || i < at || (i == at && length(old[k]) > length(old[best])))) { at = i; best = k }
        }
        if (best == 0) break
        out = out substr(rest, 1, at - 1) new[best]
        rest = substr(rest, at + length(old[best]))
      }
      print out rest
    }' "$file" >"$file.rename.tmp"
  write "$file" "$file.rename.tmp"
}

# setvalue <file> <line-regex> <value-regex> <value>: on the lines matching
# <line-regex>, replace what <value-regex> matches by <value>, keeping the
# rest (alignment included). No matching line is an error: the template
# changed and this script must follow.
setvalue() {
  local n
  n=$(awk -v re="$2" '$0 ~ re { n++ } END { print n + 0 }' "$1")
  [[ "$n" -gt 0 ]] || { echo "error: no line matches /$2/ in $1: update scripts/rename.sh" >&2; exit 1; }
  awk -v re="$2" -v vre="$3" -v val="$4" '$0 ~ re { sub(vre, val) } { print }' "$1" >"$1.rename.tmp"
  write "$1" "$1.rename.tmp"
}

# files <name-pattern>: the project's files with that name, tracked or not
# (gonew's copy is not a git repository), outside .git, vendor and bin.
files() {
  find . \( -name .git -o -name vendor -o -name bin -o -name node_modules \) -prune -o -type f -name "$1" -print | sort
}

# 1. The module path: Go sources, the mockery config and the docs (not the
#    CHANGELOG, which is history). The docs also get the owner/repo form
#    (badges, image names) when both modules are on GitHub.
while IFS= read -r f; do
  literal "$f" "$old_module" "$new_module"
done < <(files '*.go'; files .mockery.yaml)

# Repository URLs first (https://github.com/owner/repo, never with a /vN or a
# subdirectory), then the module path, then the bare owner/repo (badges,
# image names).
doc_pairs=()
if [[ -n "$old_repo" && -n "$new_repo" ]]; then
  doc_pairs+=("https://github.com/$old_repo" "https://github.com/$new_repo")
fi
doc_pairs+=("$old_module" "$new_module")
if [[ -n "$old_repo" && -n "$new_repo" ]]; then
  doc_pairs+=("$old_repo" "$new_repo")
fi
while IFS= read -r f; do
  [[ "$f" == ./CHANGELOG.md ]] && continue
  literal "$f" "${doc_pairs[@]}"
  # Not on GitHub: badges and links in the form owner/repo stay as they are.
  if [[ -n "$old_repo" && -z "$new_repo" ]] && grep -qF "$old_repo" "$f"; then
    warnings+=("$new_module is not on github.com: badges and links in ${f#./} still point to $old_repo")
  fi
done < <(files '*.md')

# 2. go.mod, unless it already names the new module.
if [[ "$gomod_module" != "$new_module" ]]; then
  go mod edit -module "$new_module"
  changed+=(go.mod)
fi

# 3. The name: Makefile, programName, envPrefix, Dockerfile defaults; and
#    gofumpt's module path in the lint configs.
setvalue Makefile '^NAME[ \t]*[?]=' '[?]=.*' "?= $new_name"
setvalue cmd/server/main.go '^[ \t]*programName[ \t]*= "' '"[^"]*"' "\"$new_name\""
setvalue internal/config/config.go 'envPrefix[ \t]*= "' '"[^"]*"' "\"$new_prefix\""
setvalue Dockerfile '^ARG NAME=' '=.*' "=$new_name"
setvalue Dockerfile '^ARG SOURCE=' '=.*' "=$new_source"
# gofumpt needs the module path only when its first element has no dot.
gofumpt_module=""
[[ "${new_module%%/*}" == *.* ]] || gofumpt_module=$new_module
for f in .golangci.yml .golangci.lite.yml; do
  [[ -f "$f" ]] && setvalue "$f" '^[ \t]*module-path:' ':.*' ": \"$gofumpt_module\""
done
[[ "$new_module" == */* ]] || warnings+=("$new_module has no host: the image source label ($new_source) is a placeholder")

# 4. The OpenAPI spec: info.title (two-space indent: never the title property
#    of a schema) and the contact URL.
if [[ -f api/openapi.yaml ]]; then
  setvalue api/openapi.yaml '^  title:' ':.*' ": $new_name"
  literal api/openapi.yaml "$old_source" "$new_source"
fi

# 5. The old mocks: make rename regenerates them under the new module path.
if [[ "$old_module" != "$new_module" && -d "mocks/$old_module" ]]; then
  rm -rf "mocks/$old_module"
  find mocks -depth -type d -empty -exec rmdir {} \;
  changed+=("mocks/$old_module (removed)")
fi

# 6. Leftovers: the old module path anywhere else (a file type this script
#    doesn't cover yet), outside the CHANGELOG. The new module path and the
#    new repository URL are removed from each line first: when they contain
#    the old path (foo → foo-api, foo → foo/v2), they are not leftovers.
if [[ "$old_module" != "$new_module" ]]; then
  while IFS= read -r f; do
    warnings+=("still mentions $old_module: ${f#./}")
  done < <(grep -rIlF --exclude-dir=.git --exclude-dir=vendor --exclude-dir=bin --exclude-dir=node_modules \
    --exclude=CHANGELOG.md --exclude='*.rename.tmp' -e "$old_module" . | sort |
    RENAME_OLD="$old_module" RENAME_SKIP1="$new_module" RENAME_SKIP2="${new_repo:+https://github.com/$new_repo}" awk '
      # strip <line> <text>: <line> without any literal occurrence of <text>.
      function strip(line, text,   out, i) {
        if (text == "") return line
        out = ""
        while ((i = index(line, text)) > 0) { out = out substr(line, 1, i - 1); line = substr(line, i + length(text)) }
        return out line
      }
      {
        file = $0
        while ((getline line < file) > 0) {
          if (index(strip(strip(line, ENVIRON["RENAME_SKIP1"]), ENVIRON["RENAME_SKIP2"]), ENVIRON["RENAME_OLD"]) > 0) { print file; break }
        }
        close(file)
      }')
fi

echo "renamed $old_module ($old_name) → $new_module ($new_name), env prefix $new_prefix"
if [[ ${#changed[@]} -eq 0 ]]; then
  echo "nothing to change"
else
  printf '  %s\n' "${changed[@]#./}" | awk '!seen[$0]++'
fi
if [[ ${#warnings[@]} -gt 0 ]]; then
  printf 'warning: %s\n' "${warnings[@]}" >&2
fi
cat <<MSG
Not changed, set them per repository: GitHub secrets (CODECOV_TOKEN), branch
protection, the Codecov project, and the prose that still says "$old_name" in
README.md and AGENTS.md.
MSG
