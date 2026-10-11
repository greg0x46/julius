#!/usr/bin/env bash
# scripts/next-version.sh: SemVer level, deterministic --notes, reverts, no side effects.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
setup
NV="$SCRIPTS/next-version.sh"
export JULIUS_REPO_URL=https://example.invalid/o/r

# A repository whose last release is v0.3.0.
released_repo() { new_repo; c "feat: first"; git tag v0.3.0; }

section "version level"
new_repo; c "feat: x"
eq "no tag yet: first release is v0.0.1" "v0.0.1" "$("$NV")"
released_repo; c "docs: only docs (#1)"; c "chore: deps"; c "Update stuff"
eq "docs/chore/non-conventional: no release" "" "$("$NV")"
eq "no release: no notes" "" "$("$NV" --notes)"
c "fix(api): patch me (#2)"
eq "fix -> patch" "v0.3.1" "$("$NV")"
c "perf: faster (#3)"
eq "perf -> patch" "v0.3.1" "$("$NV")"
c "feat: new thing (#4)"
eq "feat -> minor" "v0.4.0" "$("$NV")"
c "refactor!: drop legacy (#5)"
eq "breaking on 0.x -> minor" "v0.4.0" "$("$NV")"
released_repo; c "feat: y (#6)" "BREAKING CHANGE: the API changed"
eq "BREAKING CHANGE footer on 0.x -> minor" "v0.4.0" "$("$NV")"
released_repo; git tag v1.2.0; c "fix: z (#7)"
eq "fix on 1.x -> patch" "v1.2.1" "$("$NV")"
c "feat!: big (#8)"
eq "breaking on 1.x -> major" "v2.0.0" "$("$NV")"
released_repo; git tag v0.3.1-rc1; git tag v0.10.0; c "fix: q (#9)"
eq "highest SemVer tag wins, pre-release tags ignored (numeric sort)" "v0.10.1" "$("$NV")"

section "--notes: deterministic, readable, linked"
released_repo
c "docs: ignored (#10)"
c "feat(home): show daily spending allowance (#52)"
c "fix: correct historical exchange-rate refresh (#79)"
c "perf(frontend): run Vitest in parallel (#70)"
c "feat(holdings): allow manual positions"
c "fix(syncsvc): refuse cross-account merges (#71)"
c "refactor!: drop the legacy export (#80)" "BREAKING CHANGE: /api/export is gone"
c "chore: noise (#81)"
expected='### Breaking changes

- Drop the legacy export ([#80](https://example.invalid/o/r/pull/80))
  BREAKING CHANGE: /api/export is gone

### Features

- **home:** Show daily spending allowance ([#52](https://example.invalid/o/r/pull/52))
- **holdings:** Allow manual positions

### Fixes

- Correct historical exchange-rate refresh ([#79](https://example.invalid/o/r/pull/79))
- **syncsvc:** Refuse cross-account merges ([#71](https://example.invalid/o/r/pull/71))

### Performance

- **frontend:** Run Vitest in parallel ([#70](https://example.invalid/o/r/pull/70))'
run "$NV" --notes
eq "notes are grouped by type, oldest first, with PR links" "$expected" "$OUT"
eq "notes: same output on a second run" "$OUT" "$("$NV" --notes)"
eq "version for the same range" "v0.4.0" "$("$NV")"
eq "running the script writes nothing" "" "$(git status --porcelain)"
eq "running the script creates no tag" "v0.3.0" "$(git tag --list | tr '\n' ' ' | sed 's/ $//')"
unset JULIUS_REPO_URL
run "$NV" --notes
has "default PR links point at greg0x46/julius" "https://github.com/greg0x46/julius/pull/52" "$OUT"
export JULIUS_REPO_URL=https://example.invalid/o/r

section "merge commits do not hide or add changes (promotion PR = merge commit)"
released_repo
c "fix: before the branch (#20)"
git checkout -q -b release/v0.3.1
c "fix: on the release branch (#21)"
c "docs(changelog): v0.3.1"
git checkout -q main
git merge -q --no-ff release/v0.3.1 -m "Merge pull request #22 from o/release/v0.3.1"
notes=$("$NV" --notes)
has "commits reached through a merge are listed" "On the release branch" "$notes"
has "commits before the branch are listed" "Before the branch" "$notes"
lacks "the merge commit itself is not listed" "Merge pull request" "$notes"
lacks "docs(changelog) is not listed" "changelog" "$notes"
eq "version through the merge" "v0.3.1" "$("$NV")"

section "reverts"
released_repo
c "feat: add widget (#30)"
git revert --no-edit HEAD -q >/dev/null 2>&1 || git revert --no-edit HEAD >/dev/null
eq "git revert of an in-range feat: no release" "" "$("$NV")"
eq "git revert of an in-range feat: no notes" "" "$("$NV" --notes)"
c "fix: real fix (#31)"
run "$NV" --notes
eq "reverted feat is not listed; the fix is" $'### Fixes\n\n- Real fix ([#31](https://example.invalid/o/r/pull/31))' "$OUT"
eq "reverted feat does not raise the level" "v0.3.1" "$("$NV")"

released_repo
c "feat: add gadget (#40)"
c 'Revert "feat: add gadget (#40)" (#41)' "Reverts o/r#40"
c "fix: other (#42)"
eq "GitHub-style 'Reverts o/r#N' pairs by PR number" $'### Fixes\n\n- Other ([#42](https://example.invalid/o/r/pull/42))' "$("$NV" --notes)"
eq "...and the level" "v0.3.1" "$("$NV")"

released_repo
c "feat: add thing (#50)"
c 'Revert "feat: add thing (#50)"'
c "fix: other (#51)"
eq "quoted-subject revert pairs by subject" "v0.3.1" "$("$NV")"
lacks "...and is not listed" "thing" "$("$NV" --notes)"

released_repo
c "feat(ui): add panel (#60)"
c "revert(ui): feat(ui): add panel (#60) (#61)"
c "fix: other (#62)"
eq "conventional 'revert:' pairs by subject too" "v0.3.1" "$("$NV")"

released_repo
c "feat: add item (#70)"
c 'Revert "feat: add item (#70)"'
c 'Revert "Revert "feat: add item (#70)""' "This reverts commit $(head_sha)."
eq "revert of a revert restores the feature" "v0.4.0" "$("$NV")"
has "revert of a revert: the feature is listed again" "Add item" "$("$NV" --notes)"

new_repo; c "feat: shipped earlier (#80)"
shipped=$(head_sha)
git tag v0.3.0
c "fix: later (#81)"
c 'Revert "feat: shipped earlier (#80)"' "This reverts commit $shipped."
notes=$("$NV" --notes)
has "revert of an already released change is listed, not hidden" "- Reverted: Shipped earlier ([#80]" "$notes"
has "...next to the other changes" "Later" "$notes"
lacks "...and is not mistaken for a new feature" "### Features" "$notes"

released_repo
c "fix: something (#90)"
c 'Revert "feat: something nobody can find (#91)"'
notes=$("$NV" --notes)
has "ambiguous revert is flagged, never guessed" "### Needs review" "$notes"
has "...quoting the revert verbatim" 'Revert "feat: something nobody can find (#91)" — could not tell which change this reverts' "$notes"
lacks "...without inventing an entry" "Something nobody" "$(sed '/Needs review/,$d' <<<"$notes")"

released_repo
c "fix: something (#92)"
c "revert: undo it" "This reverts commit 0123456789abcdef0123456789abcdef01234567."
has "revert pointing at an unknown sha is flagged" "### Needs review" "$("$NV" --notes)"

section "usage"
run "$NV" --bogus
eq "unknown option is a usage error" "2" "$RC"
finish
