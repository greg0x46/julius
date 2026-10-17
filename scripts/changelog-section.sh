#!/usr/bin/env bash
# Prints the body of one version's section of CHANGELOG.md (the lines under
# "## vX.Y.Z — date", without that heading) so the release job can use the
# committed text as the GitHub release notes. Fails if the section is missing
# or empty.
#
#   changelog-section.sh vX.Y.Z   the section body
#   changelog-section.sh --latest the version of the first "## vX.Y.Z" heading
#
# CHANGELOG_FILE overrides the file (default: CHANGELOG.md at the repo root).
set -euo pipefail

arg="${1:-}"
if (($# != 1)) || [[ "$arg" != "--latest" && ! "$arg" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
  echo "usage: $0 vX.Y.Z | --latest" >&2
  exit 2
fi

file="${CHANGELOG_FILE:-$(git rev-parse --show-toplevel)/CHANGELOG.md}"
[[ -f "$file" ]] || { echo "$0: $file not found" >&2; exit 1; }

if [[ "$arg" == "--latest" ]]; then
  version=$(awk '/^## v[0-9]/ { print $2; exit }' "$file")
  [[ -n "$version" ]] || { echo "$0: no version section in $file" >&2; exit 1; }
  echo "$version"
  exit 0
fi

body=$(awk -v want="$arg" '
  /^## / {
    if (on) exit
    on = ($2 == want)
    next
  }
  on { print }
' "$file" | sed -e :a -e '/^\n*$/{$d;N;ba' -e '}' | sed '/./,$!d')
if [[ -z "$body" ]]; then
  echo "$0: no non-empty '## $arg' section in $file" >&2
  exit 1
fi
printf '%s\n' "$body"
