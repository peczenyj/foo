#!/usr/bin/env bash
# Adapts the template to another module and project name: rewrites the module
# path and the few places that hold the name (README "Adapting the template").
#
# Usage:  scripts/rename.sh <module> <name>
#         (make rename MODULE=<module> NAME=<name> also regenerates the mocks
#         and tidies go.mod afterwards)
#
# - <module>: the new Go module path, e.g. example.com/acme/widget. go.mod is
#   left alone when its module line already says so (an existing repository,
#   or a copy made with gonew), and rewritten otherwise.
# - <name>: the new project name, e.g. widget: binaries, image, programName;
#   the environment variable prefix is the name upper-cased, "-" → "_".
#
# Running it twice with the same arguments changes nothing the second time.
# Portable: plain awk, no GNU-only sed -i.
set -euo pipefail

cd "$(dirname "$0")/.."

[[ $# -eq 2 ]] || { echo "usage: $0 <module> <name>" >&2; exit 2; }
new_module=$1 new_name=$2

[[ "$new_module" =~ ^[a-z0-9][A-Za-z0-9._~-]*(/[A-Za-z0-9._~-]+)+$ ]] || {
  echo "error: \"$new_module\" is not a module path like example.com/acme/widget" >&2; exit 2; }
[[ "$new_name" =~ ^[a-z][a-z0-9-]*$ ]] || {
  echo "error: \"$new_name\" must be lower case letters, digits and \"-\", starting with a letter" >&2; exit 2; }

old_module=$(awk '/^module / { print $2; exit }' go.mod)
old_name=$(awk '/^NAME[ \t]*\?=/ { print $3; exit }' Makefile)
[[ -n "$old_module" && -n "$old_name" ]] || { echo "error: cannot read the current module (go.mod) or NAME (Makefile)" >&2; exit 1; }

# The module path without its host: the owner/repo form in badges and URLs.
old_repo=${old_module#*/}
new_repo=${new_module#*/}
new_prefix=$(tr 'a-z-' 'A-Z_' <<<"$new_name")_

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

# literal <file> <old> <new>...: replace every literal occurrence, pair by pair.
literal() {
  local file=$1 tmp="$1.rename.tmp"
  shift
  cp "$file" "$tmp"
  while [[ $# -ge 2 ]]; do
    awk -v old="$1" -v new="$2" '{
      out = ""
      while ((i = index($0, old)) > 0) { out = out substr($0, 1, i - 1) new; $0 = substr($0, i + length(old)) }
      print out $0
    }' "$tmp" >"$tmp.2" && mv "$tmp.2" "$tmp"
    shift 2
  done
  write "$file" "$tmp"
}

# setline <file> <line-regex> <replacement-line>: replace the whole matching lines.
setline() {
  awk -v re="$2" -v line="$3" '$0 ~ re { print line; next } { print }' "$1" >"$1.rename.tmp"
  write "$1" "$1.rename.tmp"
}

# files <name-pattern>: the project's files with that name, tracked or not
# (gonew's copy is not a git repository), outside .git, vendor and bin.
files() {
  find . \( -name .git -o -name vendor -o -name bin -o -name node_modules \) -prune -o -type f -name "$1" -print | sort
}

# 1. The module path: Go sources, the mockery config and the docs (not the
#    CHANGELOG, which is history). The docs also get the owner/repo form.
while IFS= read -r f; do
  literal "$f" "$old_module" "$new_module"
done < <(files '*.go'; echo ./.mockery.yaml)

while IFS= read -r f; do
  [[ "$f" == ./CHANGELOG.md ]] && continue
  literal "$f" "$old_module" "$new_module" "$old_repo" "$new_repo"
done < <(files '*.md')

# 2. go.mod, unless it already names the new module.
if [[ "$old_module" != "$new_module" ]]; then
  go mod edit -module "$new_module"
  changed+=(go.mod)
fi

# 3. The name: Makefile, programName, envPrefix, Dockerfile defaults.
setline Makefile '^NAME[ \t]*\\?=' "NAME          ?= $new_name"
setline cmd/server/main.go '^\tprogramName = "' "\\tprogramName = \"$new_name\""
setline internal/config/config.go '^const envPrefix = "' "const envPrefix = \"$new_prefix\""
setline Dockerfile '^ARG NAME=' "ARG NAME=$new_name"
setline Dockerfile '^ARG SOURCE=' "ARG SOURCE=https://$new_module"

# 4. The old mocks: make rename regenerates them under the new module path.
if [[ "$old_module" != "$new_module" && -d "mocks/$old_module" ]]; then
  rm -rf "mocks/$old_module"
  find mocks -depth -type d -empty -exec rmdir {} \;
  changed+=("mocks/$old_module (removed)")
fi

echo "renamed $old_module ($old_name) → $new_module ($new_name), env prefix $new_prefix"
if [[ ${#changed[@]} -eq 0 ]]; then
  echo "nothing to change"
else
  printf '  %s\n' "${changed[@]#./}" | awk '!seen[$0]++'
fi
cat <<MSG
Not changed, set them per repository: GitHub secrets (CODECOV_TOKEN), branch
protection, the Codecov project, and the prose that still says "$old_name" in
README.md and AGENTS.md.
MSG
