# Wealth (Patrimônio)

> Living reference — what exists and works today. Keep it updated when the
> feature changes. The folder, package (`internal/networth`) and route
> (`/api/net-worth`) keep their historical "net worth" names.

## What it is

Patrimônio is what the user has **already realized**: cash plus investments,
valued in BRL. It is not net worth: no credit card, debt, receivable or future
bill enters it, because credit is not wealth and future obligations have not
happened. Payables and cards keep their own screens and totals.

## State vs. ideal

Not evaluated in depth as a domain-engine candidate — see "Not engines" in
`.specs/motores-de-dominio.md`: it does not (for now) meet criterion 3 (reuse
across multiple contexts).

## Backend

`internal/networth`. **Nothing is stored**: today's figure and every past day are
derived on each read, so a corrected transaction, an edited operation or a late
quote reaches the whole history at once. (The old `net_worth_snapshots` table is
no longer written or read; it was left in place, unmigrated, until someone
decides what to do with the observations it holds.)

- `Compute` / `ComputeValued`: today. Cash is `transactions.CashOnHandValued`
  (non-`CREDIT` accounts), investments are `investmentBalance` (provider
  holdings at their balance, plus `investments.ManualNetWorthValued`: manual
  positions at the latest quote or typed valuation, else cost, plus custody
  cash not already a linked bank account). `Total = Cash + Investments`.
- `Series` / `SeriesWith`: the wealth at the end of every day from the first
  dated movement (cash transaction or investment operation) through today,
  oldest first, today being `Compute`.
  - Cash walks back from today's native balances, reversing each real movement
    (`cashDeltasDescending`): every eligible synced entry on a synced account
    (ignored and transfer-categorized ones too — the money moved), posted
    movements and dated adjustments of manual accounts. Manual entries on synced
    accounts never changed the provider balance and are left out. Foreign
    balances are walked back in their own currency and converted at the end.
  - Investments come from `investments.ValuesOn`: each account's operations are
    replayed once through the days, a manual position takes the latest quote on
    or before the day (or the typed valuation, or its cost) by the same rule as
    today's `CurrentValue`, custody cash counts unless linked to a bank account,
    and a provider holding is today's quantity rebuilt backwards times the last
    price known that day (carried until the next one).
- Valuation (`internal/valuation`): each day converts with its own rate. A past
  day that cannot be valued is `unavailable` and carries no figures, never a
  zero: a missing rate or provider outage (the provider is asked once, not once
  per day), a foreign movement with no amount in its account's currency lying
  after it, or a provider holding with no price at all. Today failing to value
  is an error (503 `exchange-unavailable`).
- Goals (`internal/goals`) read the same investments line through
  `networth.InvestmentValue`; they add nothing to wealth.

## HTTP routes

`GET /api/net-worth` → `{ series, latest }`, each point
`{ date, currency_code, fx, valuation_status, valuation, cash_balance,
investment_balance, total }`. `valuation_status` is `complete`, `stale` (a stale
rate was used) or `unavailable` (figures absent). The points are stored in BRL
terms and shown in the reference currency at each day's own FX, with that day's
`fx` provenance; any missing rate makes the report unavailable (503).

## Frontend

`/patrimonio` (the old `/patrimonio-liquido` redirects) — a hero strip with
today's total, cash and investments, plus `NetWorthChart` (Recharts) with the
evolution. Unavailable days are gaps in the line, not zeros.

## API contract verification

Wire semantics and calculation ownership are inventoried in
[`contratos-api.md`](../../contratos-api.md). Shared synthetic responses are
checked against Go DTOs and the production TypeScript parsers by the existing
backend/frontend CI tests.
