#!/usr/bin/env bash
# Prints the next release tag (vMAJOR.MINOR.PATCH) derived from the
# Conventional Commits since the latest SemVer tag reachable from HEAD, or
# nothing when no commit warrants a release. With --notes, prints the release
# notes for that same range (nothing when there is no release). Never creates
# tags and never writes files. This is the only place that decides versions.
#
#   breaking change (type!: or a BREAKING CHANGE: footer) -> major (minor on 0.x)
#   feat                                                   -> minor
#   fix, perf                                              -> patch
#   anything else, merge commits, non-conventional commits -> no release
#
# Reverts: a revert commit and the commit it reverts cancel each other out when
# both are in the range, so they affect neither the version nor the notes. The
# target is found, in this order, by "This reverts commit <sha>", by
# "Reverts #N" (the squash subject "(#N)"), or by the quoted subject of a
# `Revert "<subject>"` / `revert: <subject>` commit. A revert whose target is
# outside the range (already released) is listed under "Reverts"; one whose
# target cannot be determined is listed verbatim under "Needs review". Nothing
# is ever guessed.
#
# --notes output (deterministic: oldest commit first inside each section):
#   ### Breaking changes / Features / Fixes / Performance / Reverts / Needs review
#   - **scope:** Description ([#N](https://github.com/greg0x46/julius/pull/N))
# The repository URL can be overridden with JULIUS_REPO_URL (used by the tests).
set -euo pipefail

mode="${1:-}"
if [[ -n "$mode" && "$mode" != "--notes" ]] || (($# > 1)); then
  echo "usage: $0 [--notes]" >&2
  exit 2
fi

repo_url="${JULIUS_REPO_URL:-https://github.com/greg0x46/julius}"

base=$(git tag --merged HEAD --list 'v*' \
  | grep -E '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$' \
  | sed 's/^v//' | sort -t. -k1,1n -k2,2n -k3,3n | tail -1 || true)
range=HEAD
[[ -z "$base" ]] || range="v$base..HEAD"

header='^([a-z]+)(\([^)]*\))?(!)?: (.+)$'
revert_conv='^[Rr]evert(\([^)]*\))?!?: (.*)$'
revert_quoted='^[Rr]evert "(.*)"( \(#[0-9]+\))?$'
revert_prefix='^[Rr]evert "'
us=$'\x1f'

# Oldest first, so index i+1 is newer than i.
shas=() subjects=() messages=()
declare -A by_sha=() by_pr=() by_subject=()
while IFS= read -r -d '' record; do
  sha="${record%%"$us"*}"
  message="${record#*"$us"}"
  subject="${message%%$'\n'*}"
  i=${#shas[@]}
  shas+=("$sha")
  subjects+=("$subject")
  messages+=("$message")
  by_sha[$sha]=$i
  by_subject[$subject]=$i
  if [[ "$subject" =~ \(#([0-9]+)\)$ ]]; then by_pr[${BASH_REMATCH[1]}]=$i; fi
done < <(git log -z --no-merges --reverse --format="%H${us}%B" "$range")
count=${#shas[@]}

is_revert() { # subject
  [[ "$1" =~ $revert_conv || "$1" =~ $revert_prefix ]]
}

# Entry text for a conventional subject: "**scope:** Description ([#N](url))".
entry() { # subject
  local s="$1" scope desc pr=""
  [[ "$s" =~ $header ]] || { printf '%s' "$s"; return; }
  scope="${BASH_REMATCH[2]}"
  desc="${BASH_REMATCH[4]}"
  scope="${scope#(}"
  scope="${scope%)}"
  if [[ "$desc" =~ ^(.*)\ \(#([0-9]+)\)$ ]]; then
    desc="${BASH_REMATCH[1]}"
    pr="${BASH_REMATCH[2]}"
  fi
  desc="$(tr '[:lower:]' '[:upper:]' <<<"${desc:0:1}")${desc:1}"
  [[ -z "$scope" ]] || printf '**%s:** ' "$scope"
  printf '%s' "$desc"
  [[ -z "$pr" ]] || printf ' ([#%s](%s/pull/%s))' "$pr" "$repo_url" "$pr"
}

# Pass 1, newest first: pair every effective revert with its target.
cancelled=()
outside=()  # reverts of commits that are not in the range: "revert-index<TAB>original subject"
review=()   # reverts whose target cannot be determined: the revert's subject
for ((i = count - 1; i >= 0; i--)); do
  [[ -z "${cancelled[i]:-}" ]] && is_revert "${subjects[i]}" || continue
  message="${messages[i]}"
  found=0
  # a) "This reverts commit <sha>" (one line per reverted commit)
  while IFS= read -r ref; do
    [[ -n "$ref" ]] || continue
    found=1
    full=$(git rev-parse --verify -q "${ref}^{commit}" 2>/dev/null || true)
    if [[ -n "$full" && -n "${by_sha[$full]:-}" && "${by_sha[$full]}" != "$i" ]]; then
      cancelled[${by_sha[$full]}]=1
      cancelled[i]=1
    elif [[ -n "$full" ]]; then
      outside+=("$i"$'\t'"$(git log -1 --format=%s "$full")")
    else
      review+=("${subjects[i]}")
    fi
  done < <(grep -oE '^This reverts commit [0-9a-f]{7,40}' <<<"$message" | awk '{print $4}' || true)
  if ((found)); then continue; fi
  # b) "Reverts #N" / "Reverts owner/repo#N" (what GitHub's revert button writes)
  while IFS= read -r ref; do
    [[ -n "$ref" ]] || continue
    found=1
    n="${ref##*#}"
    if [[ -n "${by_pr[$n]:-}" && "${by_pr[$n]}" != "$i" ]]; then
      cancelled[${by_pr[$n]}]=1
      cancelled[i]=1
    else
      review+=("${subjects[i]}")
    fi
  done < <(grep -oiE '^Reverts? ([A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+)?#[0-9]+' <<<"$message" || true)
  if ((found)); then continue; fi
  # c) the quoted subject of the reverted commit, with or without our own "(#N)"
  stripped="${subjects[i]}"
  [[ ! "$stripped" =~ ^(.*)\ \(#[0-9]+\)$ ]] || stripped="${BASH_REMATCH[1]}"
  target=""
  for candidate in "${subjects[i]}" "$stripped"; do
    if [[ "$candidate" =~ $revert_quoted ]]; then
      candidate="${BASH_REMATCH[1]}"
    elif [[ "$candidate" =~ $revert_conv ]]; then
      candidate="${BASH_REMATCH[2]}"
    fi
    if [[ -n "${by_subject[$candidate]:-}" && "${by_subject[$candidate]}" != "$i" ]]; then
      target="${by_subject[$candidate]}"
      break
    fi
  done
  if [[ -n "$target" ]]; then
    cancelled[target]=1
    cancelled[i]=1
  else
    review+=("${subjects[i]}")
  fi
done

# Pass 2, oldest first: version level and notes sections from the surviving commits.
level=0
breaking=() features=() fixes=() perfs=()
for ((i = 0; i < count; i++)); do
  [[ -z "${cancelled[i]:-}" ]] || continue
  subject="${subjects[i]}"
  [[ "$subject" =~ $header ]] || continue
  type="${BASH_REMATCH[1]}"
  bang="${BASH_REMATCH[3]}"
  commit_level=0
  case "$type" in
    feat) commit_level=2 ;;
    fix | perf) commit_level=1 ;;
  esac
  footer=$(grep -m1 -E '^BREAKING[ -]CHANGE: ' <<<"${messages[i]}" || true)
  if [[ -n "$bang" || -n "$footer" ]]; then
    commit_level=3
    text="- $(entry "$subject")"
    [[ -z "$footer" ]] || text+=$'\n'"  ${footer}"
    breaking+=("$text")
  else
    case "$type" in
      feat) features+=("- $(entry "$subject")") ;;
      fix) fixes+=("- $(entry "$subject")") ;;
      perf) perfs+=("- $(entry "$subject")") ;;
    esac
  fi
  ((commit_level > level)) && level=$commit_level
done

if [[ "$mode" == "--notes" ]]; then
  ((level > 0)) || exit 0
  reverts=()
  for item in ${outside[@]+"${outside[@]}"}; do
    reverts+=("- Reverted: $(entry "${item#*$'\t'}")")
  done
  reviews=()
  for item in ${review[@]+"${review[@]}"}; do
    reviews+=("- ${item} — could not tell which change this reverts; check before releasing")
  done
  first=1
  emit() { # heading line...
    local heading="$1"
    shift
    (($#)) || return 0
    ((first)) || printf '\n'
    first=0
    printf '### %s\n\n' "$heading"
    printf '%s\n' "$@"
  }
  emit "Breaking changes" ${breaking[@]+"${breaking[@]}"}
  emit "Features" ${features[@]+"${features[@]}"}
  emit "Fixes" ${fixes[@]+"${fixes[@]}"}
  emit "Performance" ${perfs[@]+"${perfs[@]}"}
  emit "Reverts" ${reverts[@]+"${reverts[@]}"}
  emit "Needs review" ${reviews[@]+"${reviews[@]}"}
  exit 0
fi

((level > 0)) || exit 0
if [[ -z "$base" ]]; then
  echo "v0.0.1"
  exit 0
fi
IFS=. read -r major minor patch <<<"$base"
((level == 3 && major == 0)) && level=2
case "$level" in
  3) major=$((major + 1)); minor=0; patch=0 ;;
  2) minor=$((minor + 1)); patch=0 ;;
  1) patch=$((patch + 1)) ;;
esac
echo "v$major.$minor.$patch"
