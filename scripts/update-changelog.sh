#!/usr/bin/env bash
# Prepares the changelog for a release: prepends ONE section
#
#   ## vX.Y.Z — YYYY-MM-DD
#
#   <scripts/next-version.sh --notes>
#
# to CHANGELOG.md, right above the newest existing version section (or above the
# baseline note when there is none). Run it once, on the release branch, when
# the release is prepared; nothing else writes the changelog.
#
#   update-changelog.sh vX.Y.Z [--date YYYY-MM-DD]
#
# * The requested version must be exactly what scripts/next-version.sh computes
#   for HEAD; otherwise it fails and touches nothing.
# * Idempotent: if the "## vX.Y.Z" section already exists the file is left
#   untouched (a hand-edited section is never overwritten).
# * Deterministic: the date comes from --date, or CHANGELOG_DATE, or today (UTC).
# CHANGELOG_FILE overrides the file (default: CHANGELOG.md at the repo root).
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
usage() { echo "usage: $0 vX.Y.Z [--date YYYY-MM-DD]" >&2; exit 2; }

version="" date="${CHANGELOG_DATE:-}"
while (($#)); do
  case "$1" in
    --date) (($# >= 2)) || usage; date="$2"; shift 2 ;;
    -*) usage ;;
    *) [[ -z "$version" ]] || usage; version="$1"; shift ;;
  esac
done
[[ "$version" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || usage
[[ -n "$date" ]] || date=$(date -u +%F)
[[ "$date" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]] && date -d "$date" +%F >/dev/null 2>&1 \
  || { echo "$0: invalid date '$date' (want YYYY-MM-DD)" >&2; exit 2; }

expected=$("$here/next-version.sh")
if [[ -z "$expected" ]]; then
  echo "$0: next-version.sh finds nothing to release; refusing to write $version" >&2
  exit 1
fi
if [[ "$expected" != "$version" ]]; then
  echo "$0: requested $version but next-version.sh computes $expected" >&2
  exit 1
fi

file="${CHANGELOG_FILE:-$(git rev-parse --show-toplevel)/CHANGELOG.md}"
if [[ -f "$file" ]] && awk -v want="$version" '/^## / && $2 == want { found = 1 } END { exit !found }' "$file"; then
  echo "$0: $file already has the $version section; nothing to do"
  exit 0
fi

notes=$("$here/next-version.sh" --notes)
if [[ -z "$notes" ]]; then
  echo "$0: no release notes for $version" >&2
  exit 1
fi

tmp=$(mktemp "${file}.XXXXXX")
trap 'rm -f "$tmp"' EXIT
section="## $version — $date"$'\n\n'"$notes"$'\n'
if [[ -s "$file" ]]; then
  # Above the first version section, else above the first "## " heading (the
  # baseline note), else at the end.
  awk -v section="$section" '
    NR == FNR { if (!v && /^## v[0-9]/) v = FNR; if (!h && /^## /) h = FNR; last = FNR; next }
    FNR == 1 { ins = v ? v : h }
    FNR == ins { printf "%s\n", section }
    { print }
    FNR == last && !ins { printf "\n%s", section }
  ' "$file" "$file" >"$tmp"
else
  printf '# Changelog\n\n%s' "$section" >"$tmp"
fi
chmod 644 "$tmp"
mv "$tmp" "$file"
trap - EXIT
echo "$0: added $version to $file"
