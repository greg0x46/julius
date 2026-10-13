# Changelog

Notable changes per release. A section is added by `scripts/update-changelog.sh`
only when a release is prepared, and the release job publishes that same text as
the GitHub release notes. Newest release first.

## v0.7.0 — 2026-10-12

### Breaking changes

- **db:** Replace legacy migrations with typed baselines and a schema guard ([#106](https://github.com/greg0x46/julius/pull/106))

### Features

- **quotes:** Persist and dynamically apply refresh settings ([#58](https://github.com/greg0x46/julius/pull/58))
- **automation:** Apply a category from the same reconcile automation ([#93](https://github.com/greg0x46/julius/pull/93))
- **db:** Add one-shot legacy data migration tool ([#108](https://github.com/greg0x46/julius/pull/108))

### Fixes

- **db:** Automatically convert legacy Postgres at startup ([#112](https://github.com/greg0x46/julius/pull/112))

## v0.6.0 — 2026-10-12

### Features

- **investments:** Allow manual holdings in synchronized accounts ([#51](https://github.com/greg0x46/julius/pull/51))

## v0.5.0 — 2026-10-12

### Features

- **accounts:** Create and manage native-currency manual bank accounts ([#90](https://github.com/greg0x46/julius/pull/90))

### Fixes

- **recurrences:** Don't require the commitment's category for automatic reconciliation ([#92](https://github.com/greg0x46/julius/pull/92))

## v0.4.0 — 2026-10-12

### Features

- **exchange:** Persist and resolve current and historical FX rates ([#79](https://github.com/greg0x46/julius/pull/79))

## Earlier releases

This changelog starts after [v0.3.0](https://github.com/greg0x46/julius/releases/tag/v0.3.0).
Earlier releases are documented only in their
[GitHub release notes](https://github.com/greg0x46/julius/releases).
