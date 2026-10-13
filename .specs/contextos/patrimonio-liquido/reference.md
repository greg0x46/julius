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
in the consolidated BRL base. Manual bank-account balances derive from opening balance,
posted undeleted transactions and adjustment deltas. Their movements and dated
adjustments enter historical reconstruction; opening balance is the baseline.
Manual transactions on synced accounts still never enter reconstruction,
since they did not change the provider balance. See Transactions, Notes.

## HTTP routes

`GET /api/net-worth`.

## Frontend

`/patrimonio-liquido` — composition card + `NetWorthChart` (Recharts) with the
evolution over time.

## Snapshot valuation and revaluation policy

Migration `0002_snapshot_currency` tags every existing snapshot with explicit
`valuation_currency = BRL` in both dialects, without changing its amounts.
Snapshots and backfill continue to write BRL valuations, enforced by a database
constraint. Changing the reference preference never rebuilds or rewrites a
historical snapshot. Today's upsert retains its existing refresh behavior.

On read, `/api/net-worth` converts each stored BRL breakdown using
`internal/exchange.On` for `captured_at`, returning the chosen `currency_code`,
`stored_currency = BRL` and that day's full `fx` provenance alongside the figures.
The chosen reference currency is read once for the whole response. Today's
snapshot uses today's FX. Historical revaluation can therefore change when
cached FX is refreshed, but the stored BRL observation is preserved. There is
no automatic history rebuild: old observations retain their original coverage,
and reconstructed days still omit historical investment balances.

Any missing historical rate makes the report unavailable (503); there is no
mixed-currency curve, silently omitted day, or 1:1 fallback. The chart labels
its valuation currency and explains that historical FX uses each day. Native
foreign balances/holdings remain outside the existing BRL base.
