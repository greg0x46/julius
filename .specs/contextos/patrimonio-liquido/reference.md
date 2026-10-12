# Net Worth

> Living reference — what exists and works today. Keep it updated when the
> feature changes. No dedicated feature spec so far.

## What it is

Snapshots and history of net worth (assets − liabilities) over time.

## State vs. ideal

Not evaluated in depth as a domain-engine candidate — see "Not engines" in
`.specs/motores-de-dominio.md`: it does not (for now) meet criterion 3 (reuse
across multiple contexts).

## Backend

`internal/networth` — snapshot calculation/backfill (`Breakdown`,
`SnapshotRow`). A snapshot for "today" is written on every read; there is no
background worker.

The current calculation includes positions and cash of manual investments via
`investments.ManualNetWorth`, plus imported positions. Cash of an investment
account linked to an existing financial account is not added again. Goal
portfolios are groupings and add no net worth. Manual valuations are dated;
assets with automatic quotes are worth quantity × latest quote when it is newer
than the valuation (`market_quote`); with neither, the value is the informed
cost. The backfill still does not invent historical appreciation of
investments.

Cash is a BRL aggregate: foreign account balances and movements are excluded
without FX rates. Manual bank-account balances derive from opening balance,
posted undeleted transactions and adjustment deltas. Their movements and dated
adjustments enter historical reconstruction; opening balance is the baseline.
Manual transactions on synced accounts still never enter reconstruction,
since they did not change the provider balance. See Transactions, Notes.

## HTTP routes

`GET /api/net-worth`.

## Frontend

`/patrimonio-liquido` — composition card + `NetWorthChart` (Recharts) with the
evolution over time.
