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
background worker. Goals (`internal/goals`) read the same investments line
through `networth.InvestmentValue`; they add nothing to net worth.

The current calculation includes positions and cash of manual investments via
`investments.ManualNetWorth`, plus imported positions. Cash of an investment
account linked to an existing financial account is not added again. Manual
valuations are dated;
assets with automatic quotes are worth quantity × latest quote when it is newer
than the valuation (`market_quote`); with neither, the value is the informed
cost. The backfill still does not invent historical appreciation of
investments.

Every figure is valued in BRL (`internal/valuation`): cash, credit-card
liabilities (each eligible cycle transaction once, in its own effective
currency), synced and manual investments, and manual investment cash are
converted with `internal/exchange` for the valuation day. BRL 2,000 plus USD 100
at 5.00 is BRL 2,500. Native balances, holdings and transaction amounts are
never rewritten, and a currency without a rate fails the valuation (503
`exchange-unavailable` for the live snapshot) instead of being dropped or taken
at 1:1. Timeline starts from the same native balances at the same day's rate, so
both agree. A cash account with a balance but a missing or invalid
`currency_code` fails the computation with an error naming the account instead of
being treated as BRL. Manual bank-account balances derive from opening balance,
posted undeleted transactions and adjustment deltas. Their movements and dated
adjustments enter historical reconstruction; opening balance is the baseline.
Manual transactions on synced accounts still never enter reconstruction,
since they did not change the provider balance. See Transactions, Notes.

## HTTP routes

`GET /api/net-worth`.

## Frontend

`/patrimonio-liquido` — current composition card + `PeriodNavigator` using
`usePeriod` (shared with Home and Transactions) + `NetWorthChart` (Recharts).
The chart clamps its effective end to today; the composition remains today's
snapshot. Net worth is selected by default. Compact checkboxes select observed
cash, portfolio valuation and investment yield. No chart selection is persisted.

Stock series share one monetary axis in the response currency. Percentage mode
uses the exact first requested day's value for every selected stock; missing,
zero or negative baselines explain unavailability rather than moving the
baseline or inventing 0%. Later negative values remain valid. Both modes keep
original decimal money in tooltips and the `Ver valores` semantic table.

Cash and net worth use captured complete/stale snapshots, never the Home
scenario-inclusive/current-FX timeline. Reconstructed, backfilled, legacy and
unavailable rows are gaps; absent calendar days also break the line. One
observation is a visible dot with a short explanation. Stale FX remains qualified
in per-date tooltips/table and coverage notes. The coverage disclosure explains
why reconstructed net worth omitting investments is not comparable.

Portfolio valuation and monthly yield come from one bounded
`GET /api/investment-history` request (see Investments reference). Portfolio
points are sampled at the requested start and monthly endpoints, never joined
across missing daily coverage. Portfolio value excludes custody cash and is not
contributions or the net-worth investment component. A period with unavailable
positions, partial starts or incomplete position history is omitted altogether,
never shown as a complete subtotal or zero. Source errors/loading prevent new
investment selections; selected metrics can still be deselected, with their
unavailability explained. Cash and net worth remain usable. Period changes clear old
investment results until the new request completes. History cache keys include
report currency; a history response in a different currency is unavailable
rather than being plotted on the wealth report's monetary axis.

Yield is a separate monetary bar chart with inclusive period labels and a
Modified Dietz table, independent of the stock percentage scale. No yield is
derived from changes in portfolio valuation. Empty selections, absent coverage,
invalid baselines and failed investment requests have explicit states and retry
where applicable. Controls wrap on narrow screens; accessible values tables
provide the information without pointer hover.

## API contract verification

Wire semantics and calculation ownership are inventoried in
[`contratos-api.md`](../../contratos-api.md). Shared synthetic responses are
checked against Go DTOs and the production TypeScript parsers by the existing
backend/frontend CI tests.

## Snapshot valuation and revaluation policy

Migration `0003_snapshot_currency` tags every existing snapshot with explicit
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
its valuation currency and explains that historical FX uses each day.

## Snapshot valuation metadata

Migration `0004_snapshot_valuation` adds `valuation_version`, `valuation_status`,
`valued_at`, `rate_policy` and `valuation_rates` (JSON text) to
`net_worth_snapshots`. Rows that existed before it keep version 0 and status
`legacy`, with no valuation context: their amounts are untouched and cover only
BRL accounts and holdings, so they are never relabeled as valued. Statuses:

- `complete` / `stale`: a live read (`Snapshot`); `stale` when any rate used was stale.
- `reconstructed`: a backfilled day, valued with that day's own rates. Foreign
  cash is walked back in its native currency and converted only at the end;
  investments are still not reconstructed.
- `unavailable`: a past day that can never be valued (a movement on a foreign
  account has no amount in the account's currency, so earlier balances are
  unknown). Stored once, without figures; the API omits the breakdown and the
  chart draws a gap, never a zero.

A backfilled day whose rate could not be resolved is not stored at all, so the
next read retries it. `valuation_rates` records each rate used (value,
inversion, effective day, source, staleness); `GET /api/net-worth` returns it as
`valuation` next to `valuation_version` and `valuation_status`.
