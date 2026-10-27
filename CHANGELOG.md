# Changelog

Notable changes per release. A section is added by `scripts/update-changelog.sh`
only when a release is prepared, and the release job publishes that same text as
the GitHub release notes. Newest release first.

## v0.14.1 — 2026-10-27

### Fixes

- **deps:** Bump decimal to 1.6.0 and sqlite to 1.61.0 ([#159](https://github.com/greg0x46/julius/pull/159))

## v0.14.0 — 2026-10-25

### Features

- **frontend:** Add generated design tokens and a semantic dark theme ([#156](https://github.com/greg0x46/julius/pull/156))

## v0.13.1 — 2026-10-24

### Performance

- **frontend:** Replace UI libraries with local components ([#153](https://github.com/greg0x46/julius/pull/153))

## v0.13.0 — 2026-10-24

### Breaking changes

- **networth:** Derive wealth as cash plus investments on every read ([#150](https://github.com/greg0x46/julius/pull/150))

### Features

- **demo:** Open public demo mode with seeded synthetic data ([#152](https://github.com/greg0x46/julius/pull/152))

### Fixes

- **deps:** Bump Go to 1.26.10 (GO-2026-6809) ([#151](https://github.com/greg0x46/julius/pull/151))
- **deps:** Bump github.com/pressly/goose/v3 ([#143](https://github.com/greg0x46/julius/pull/143))
- **deps:** Bump the npm-production-minor-patch group ([#145](https://github.com/greg0x46/julius/pull/145))
- **deps:** Bump @tanstack/react-query ([#154](https://github.com/greg0x46/julius/pull/154))

## v0.12.0 — 2026-10-19

### Features

- **goals:** Project dated goals and redesign the goals cards

### Fixes

- Correct monetary precision, projections and reconciliation
- Reject excessive installment generation before allocation
- **categories:** Keep manual sibling installments when a rule applies
- **timeline:** Keep active scenarios in a simulation
- **investments:** Drop the typed valuation when a holding is sold out
- **httpapi:** Make PUT /api/settings/pluggy idempotent and trim the Item ID
- **transactions:** Treat paid_at bill due date as a calendar day and clamp the next-month estimate
- **scenarios:** Create a plan and backfill its settlements atomically
- **sync:** Drop recurrence and investment links when a sync update makes a rule ignore a transaction
- **payables:** Drop standalone event allocations when a transaction is ignored
- **httpapi:** Cap JSON request bodies at 1 MiB
- **httpapi:** Reject huge page and page_size in transaction queries
- **investments:** Never share a catalog asset across currencies for synced quotes
- **httpapi:** Fail sync-run and investment-transaction reads on row errors
- **payables:** Map only unique violations to link conflicts and refuse soft-deleted transactions
- **frontend:** Label net worth chart days in UTC
- **httpapi:** Match the realization unique violation by constraint, not error text
- **investments:** List synced positions without writing and create assets race-safely
- **sync:** Fail unclaimed runs with worker_unavailable and log credential errors once
- **frontend:** Keep the app mounted when a background session refetch fails
- **pluggy:** Escape item, account and cursor values in request URLs
- **automation:** Validate amount and day_of_month condition values on write
- **recurrences:** Drop overrides of dates a schedule update removes
- **sync:** Tie provider waits and raw-import writes to the run context and cap response bodies
- **exchange:** Cool down a failing provider per pair, not per day and request
- **marketdata:** Truncate the response body in HTTP status errors
- **goals:** Value only investments so unrelated foreign cash cannot fail goals
- **investments:** Treat a provider investment without currency as reais in reconciliations
- **reporting:** Report an invalid currency as unsupported, not as a provider failure
- **frontend:** Use MoneyInput for manual transfer amounts
- **scenarios:** Compare installment status and deviation by magnitude
- **scenarios:** Validate transactions linked to standalone events
- **frontend:** Clamp pagination to the last page when the list is shorter
- **scenarios:** Keep the plan cadence when readjusting by term
- **frontend:** Reuse the created commitment when an automation save is retried
- **frontend:** Compute payable settled percent with exact decimals
- **frontend:** Use the loaded payable's kind on the detail page
- **frontend:** Restrict integer fields to whole numbers
- **frontend:** Invalidate commitments, reconciliation and projection views after related writes
- **frontend:** Require a non-zero expected amount in reconcile automations
- **investments:** Don't compare an empty asset id on Postgres when creating an asset

### Performance

- **networth:** Skip loading the ledger when backfill has no gap

## v0.11.0 — 2026-10-18

### Features

- **goals:** Replace investment objetivos with metric-based goals
- **investments:** Period-based portfolio view
- **investments:** Group positions by class or custody, drop manual/synced split
- **exchange:** Add Frankfurter v2 coverage and conversion diagnostics ([#128](https://github.com/greg0x46/julius/pull/128))

### Fixes

- **goals:** Roll back writes when progress cannot be computed
- **goals:** Require exact target progress before marking goals reached

## v0.10.0 — 2026-10-18

### Features

- **settings:** Make financial reference currency configurable ([#120](https://github.com/greg0x46/julius/pull/120))
- **networth:** Value multi-currency cash, debt, and investments consistently ([#81](https://github.com/greg0x46/julius/pull/81))

### Fixes

- **investments:** Use backend exact acquisition cost for yields
- **investments:** Round exact acquisition costs before displaying returns
- **api:** Align contract fixtures and yield math with exact decimals and valuation fields
- **deps:** Bump the go-minor-patch group with 3 updates ([#125](https://github.com/greg0x46/julius/pull/125))

## v0.9.0 — 2026-10-17

### Features

- **investments:** Expose gross monthly capital movements over HTTP
- **investments:** Pick or create the catalog asset inside the position form
- **investments:** Automatically seed the starter asset catalog ([#118](https://github.com/greg0x46/julius/pull/118))
- **transfers:** Link cross-currency account movements and explicit FX fees ([#119](https://github.com/greg0x46/julius/pull/119))
- **investments:** Report realized gain on sales in the monthly movements
- **investments:** Refuse manual holdings in a currency other than reais

### Fixes

- **money:** Keep cash balances separated by currency
- **money:** Restrict BRL cash walks and validate ISO 4217 codes
- **money:** Exclude foreign-currency accounts from BRL cash walks
- **investments:** Exclude reconciliation pivots from monthly flows
- **investments:** Preserve position draft across account refreshes
- **investments:** Match backend asset identity when detecting duplicates
- **money:** Preserve decimal precision at application boundaries ([#113](https://github.com/greg0x46/julius/pull/113))
- **investments:** Read monthly movements through the typed-column decoders
- **statement-imports:** Deduplicate files and show balance conflicts ([#122](https://github.com/greg0x46/julius/pull/122))

### Performance

- **frontend:** Profile and reduce expensive frontend test overhead ([#121](https://github.com/greg0x46/julius/pull/121))

## v0.8.0 — 2026-10-13

### Features

- **settings:** Manage automatic quotations from the UI ([#77](https://github.com/greg0x46/julius/pull/77))
- **reporting:** Consolidate multi-currency cash flows in reference currency ([#80](https://github.com/greg0x46/julius/pull/80))
- **ui:** Show build version in the bottom-right footer ([#117](https://github.com/greg0x46/julius/pull/117))

### Fixes

- **deps:** Bump the go-minor-patch group with 3 updates

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
