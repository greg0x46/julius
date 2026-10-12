<!--
PRs go to `stage` (the integration branch), not `main`. The PR title becomes the squash commit message and
drives the release version, so it must be a Conventional Commit: `feat(scope): ...`, `fix: ...`, `perf: ...`,
`docs: ...`. Keep the headings and bold labels below as they are; replace the guidance with real content.
-->

## Summary

<!-- What changes and why, in a few lines. User-visible behavior first. -->

## How to validate in dev

<!-- Written for someone who did not write the code and will check it in the dev instance without reading the diff. -->

**Where:** <!-- the page / route / command in the dev instance, e.g. Accounts > Investments -->

**Prerequisites / data:** <!-- accounts, rows, settings or flags that must exist (synthetic data is fine); "none" if there are none -->

**Steps:**

1. <!-- first action -->
2. <!-- next action -->

**Expected result:** <!-- what the person should see or get when it works -->

**Limitations:** <!-- what this does not cover, known gaps, anything that cannot be checked in dev; "none" if there are none -->

## Tests

<!-- Commands run and their result (e.g. `go test -p 1 -count=1 ./...`, `npm test -- --run`), plus the tests added or changed. -->

## Related issue

Refs #N
<!-- Replace N with the issue number. Use "Refs", never "Closes/Fixes": the issue is closed by the release PR that ships this change. -->
