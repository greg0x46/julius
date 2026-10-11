#!/usr/bin/env bash
# scripts/update-changelog.sh and scripts/changelog-section.sh on a throwaway history, plus the "one committed
# changelog, no generated dev changelog" rule (A24) and the notes round trip the release job relies on (A16, part).
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
setup
NV="$SCRIPTS/next-version.sh"
UC="$SCRIPTS/update-changelog.sh"
CS="$SCRIPTS/changelog-section.sh"
export JULIUS_REPO_URL=https://example.invalid/o/r

# The repository's real baseline CHANGELOG.md, committed on top of v0.3.0, then the history of a release candidate.
new_repo
c "feat: first"
git tag v0.3.0
cp "$SRC/CHANGELOG.md" CHANGELOG.md
git add CHANGELOG.md && git commit -q -m "docs: add changelog"
c "feat(home): show daily spending allowance (#52)"
c "fix: correct exchange-rate refresh (#79)"
c "docs: ignored (#74)"

section "update-changelog.sh"
baseline=$(cat CHANGELOG.md)
run "$UC" v0.4.0 --date 2026-10-11
eq "prepares v0.4.0" "0" "$RC"
top=$(sed -n '1,/^## Earlier releases/p' CHANGELOG.md)
has "title is kept first" "# Changelog" "$(head -1 CHANGELOG.md)"
eq "exactly one v0.4.0 section" "1" "$(grep -c '^## v0.4.0 — 2026-10-11$' CHANGELOG.md)"
eq "the section sits above the baseline note" "1" "$(awk '/^## v0.4.0/ {a=NR} /^## Earlier releases/ {b=NR} END {print (a && b && a < b) ? 1 : 0}' CHANGELOG.md)"
has "baseline note is kept" "$(sed -n '/^## Earlier releases/,$p' <<<"$baseline" | head -3 | tail -1)" "$(cat CHANGELOG.md)"
has "has the feature entry" "- **home:** Show daily spending allowance ([#52](https://example.invalid/o/r/pull/52))" "$(cat CHANGELOG.md)"
lacks "does not list docs" "ignored" "$(cat CHANGELOG.md)"
after_first=$(cat CHANGELOG.md)

run "$UC" v0.4.0 --date 2099-01-01
eq "second run succeeds" "0" "$RC"
eq "second run (even with another date) changes nothing: idempotent" "$after_first" "$(cat CHANGELOG.md)"
eq "still one v0.4.0 section" "1" "$(grep -c '^## v0.4.0' CHANGELOG.md)"

cp CHANGELOG.md "$T/before"
run "$UC" v0.5.0 --date 2026-10-11
eq "requested version differs from next-version.sh: fails" "1" "$RC"
has "...and says why" "computes v0.4.0" "$ERR"
run "$UC" v0.4.1 --date 2026-10-11
eq "a lower requested version fails too" "1" "$RC"
ok "a failed run leaves the file untouched" cmp -s "$T/before" CHANGELOG.md
run "$UC" 0.4.0
eq "version without the v is a usage error" "2" "$RC"
run "$UC" v0.4.0 --date 11/10/2026
eq "invalid date is a usage error" "2" "$RC"
run "$UC" v0.4.0 --date 2026-02-31
eq "impossible date is a usage error" "2" "$RC"

section "date handling"
git checkout -q -- . 2>/dev/null || true
cp "$SRC/CHANGELOG.md" CHANGELOG.md
run env CHANGELOG_DATE=2026-12-24 "$UC" v0.4.0
has "CHANGELOG_DATE is used" "## v0.4.0 — 2026-12-24" "$(cat CHANGELOG.md)"
cp "$SRC/CHANGELOG.md" CHANGELOG.md
run "$UC" v0.4.0
has "default date is today (UTC)" "## v0.4.0 — $(date -u +%F)" "$(cat CHANGELOG.md)"

section "nothing to release"
new_repo; c "feat: first"; git tag v0.3.0; cp "$SRC/CHANGELOG.md" CHANGELOG.md; git add -A; git commit -q -m "docs: changelog"
c "docs: only docs (#1)"
run "$UC" v0.3.1 --date 2026-10-11
eq "no releasable commit: fails" "1" "$RC"
ok "...and leaves the file untouched" cmp -s "$SRC/CHANGELOG.md" CHANGELOG.md

section "two consecutive releases keep newest first"
new_repo; c "feat: first"; git tag v0.3.0
cp "$SRC/CHANGELOG.md" CHANGELOG.md; git add -A; git commit -q -m "docs: changelog"
c "feat: one (#1)"
"$UC" v0.4.0 --date 2026-10-01 >/dev/null
git add -A; git commit -q -m "docs(changelog): v0.4.0"
git tag v0.4.0
c "fix: two (#2)"
run "$UC" v0.4.1 --date 2026-10-05
eq "second release prepared" "0" "$RC"
order=$(grep -E '^## ' CHANGELOG.md | tr '\n' '|')
eq "order: v0.4.1, v0.4.0, baseline" "## v0.4.1 — 2026-10-05|## v0.4.0 — 2026-10-01|## Earlier releases|" "$order"
has "v0.4.0 text is intact" "- One ([#1](https://example.invalid/o/r/pull/1))" "$(cat CHANGELOG.md)"

section "missing changelog file is created"
new_repo; c "feat: first"; git tag v0.3.0; c "fix: a (#3)"
"$UC" v0.3.1 --date 2026-10-11 >/dev/null
eq "header + section" $'# Changelog\n\n## v0.3.1 — 2026-10-11' "$(head -3 CHANGELOG.md)"

section "changelog-section.sh"
new_repo; c "feat: first"; git tag v0.3.0
cp "$SRC/CHANGELOG.md" CHANGELOG.md; git add -A; git commit -q -m "docs: changelog"
c "feat: one (#1)"; c "fix: two (#2)"
"$UC" v0.4.0 --date 2026-10-11 >/dev/null
notes=$("$NV" --notes)
run "$CS" v0.4.0
eq "extracts exactly what next-version.sh --notes generated (A16: notes = committed changelog)" "$notes" "$OUT"
eq "--latest is the newest version" "v0.4.0" "$("$CS" --latest)"
run "$CS" v0.4.1
eq "missing section fails" "1" "$RC"
has "...with a message" "no non-empty '## v0.4.1' section" "$ERR"
cat >"$T/cl.md" <<'MD'
# Changelog

## v0.4.10 — 2026-02-01

- ten

## v0.4.1 — 2026-01-01

- one


## Earlier releases

Before.
MD
run env CHANGELOG_FILE="$T/cl.md" "$CS" v0.4.1
eq "v0.4.1 is not confused with v0.4.10 and stops at the next heading" "- one" "$OUT"
run env CHANGELOG_FILE="$T/cl.md" "$CS" v0.4.10
eq "v0.4.10 extracted" "- ten" "$OUT"
printf '# Changelog\n\n## v0.5.0 — 2026-01-01\n\n## v0.4.0 — 2025-01-01\n\n- x\n' >"$T/empty.md"
run env CHANGELOG_FILE="$T/empty.md" "$CS" v0.5.0
eq "an empty section fails" "1" "$RC"
run env CHANGELOG_FILE="$T/nope.md" "$CS" v0.5.0
eq "missing file fails" "1" "$RC"

section "release job guard: changelog top == computed version"
# the CI step does: tag=$(next-version.sh); section for tag must exist and be the newest one
tag=$("$NV")
eq "computed tag has its section" "$tag" "$("$CS" --latest)"
new_repo; c "feat: first"; git tag v0.3.0
cp "$SRC/CHANGELOG.md" CHANGELOG.md; git add -A; git commit -q -m "docs: changelog"
c "fix: a (#8)"
"$UC" v0.3.1 --date 2026-10-11 >/dev/null
git add -A; git commit -q -m "docs(changelog): v0.3.1"
eq "fix-only candidate: computed tag matches the changelog top" "$("$NV")" "$("$CS" --latest)"
c "feat: landed after the changelog was prepared (#9)"
tag=$("$NV")
eq "a feat on top changes the computed version to v0.4.0" "v0.4.0" "$tag"
run "$CS" "$tag"
eq "...which has no section, so the release job fails instead of publishing" "1" "$RC"

section "A24: one committed changelog, no generated dev changelog"
new_repo; c "feat: first"; git tag v0.3.0
cp "$SRC/CHANGELOG.md" CHANGELOG.md; git add -A; git commit -q -m "docs: changelog"
c "feat: one (#1)"
before_files=$(git ls-files | sort)
"$NV" >/dev/null; "$NV" --notes >/dev/null
eq "next-version.sh creates nothing" "" "$(git status --porcelain)"
"$UC" v0.4.0 --date 2026-10-11 >/dev/null
eq "update-changelog.sh changes only CHANGELOG.md" " M CHANGELOG.md" "$(git status --porcelain)"
git add -A; git commit -q -m "docs(changelog): v0.4.0"
eq "exactly one tracked changelog" "1" "$(git ls-files | grep -ci 'changelog')"
eq "no new tracked files besides the changelog edit" "$before_files" "$(git ls-files | sort)"
eq "no untracked leftovers (temp files)" "" "$(git status --porcelain --ignored)"
cd "$SRC"
eq "the real repo tracks one changelog file" "CHANGELOG.md" "$(git ls-files --cached --others --exclude-standard | grep -iE '(^|/)changelog[^/]*\.(md|txt|rst)$' | tr '\n' ' ' | sed 's/ $//')"
eq "no git-cliff config in the real repo" "" "$(git ls-files --cached --others --exclude-standard | grep -iE 'cliff' || true)"
eq "the workflow does not use git-cliff" "" "$(grep -il 'git-cliff' .github/workflows/*.yml || true)"
eq "the real CHANGELOG.md starts with the title and the v0.3.0 baseline" "# Changelog" "$(head -1 CHANGELOG.md)"
has "baseline references v0.3.0" "v0.3.0" "$(cat CHANGELOG.md)"
finish
