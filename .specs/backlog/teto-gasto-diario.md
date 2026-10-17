# Idea (do not implement yet): daily spending cap ("how much can I spend today")

> Discussion saved to resume later. Not implemented as designed here (Home has
> a simpler `DailyAllowanceCard`, see `contextos/home/reference.md`). Some code
> references below predate refactors (e.g. `internal/debts` is now
> `internal/payables`); re-verify them before relying on them.

## Original motivation

Show the user how much they can spend **today** without compromising the rest of
the month — not a fixed average (budget ÷ days in the month), but a cap that
rises when previous days saved and falls when they overspent, with a brake
against sharp drops (a big purchase must not zero the cap from one day to the
next).

## Version 1: manual budget + damping (designed, two bugs found)

### Mechanism
```
raw_cap(day N)        = (effective_budget_of_month − spent_until_yesterday) / remaining_days
damped_cap(N)         = raw_cap(N), if it is a rise (no limit)
                       = max(raw_cap(N), damped_cap(N-1) * (1 - damping%)), if it is a fall
                       = 0 immediately (no damping), if raw_cap(N) <= 0
                         + exposes "overspent by R$ X" = -raw_cap(N) * remaining_days
```
"Eligible spending" = sum of `money.SelectEffectiveMoney` (BRL) of `Outflow` +
`Considered` transactions with category `kind='expense'`.

No per-day state persisted — recomputed from scratch on every read, in the same
spirit as `internal/payables` (settled/remaining/status never stored, always
recomputed from current links — see `internal/payables/eligibility.go` and
`summary.go`).

### Bug 1 — naive rollover cancels itself out / inverts
Always comparing the previous month's real spending against the **current raw
value** of the budget makes a deficit disappear (or become a phantom credit) in
about 2 months: the next month already spends within a reduced cap, but would be
compared again against the unreduced value.
Example: budget R$3000, January spends R$4000 (overspends R$1000) → February's
effective cap is R$2000 → if the user spends exactly R$2000 in February, March
recomputes rollover = 3000 − 2000 = **+1000** (phantom credit, when in fact they
only broke even).

### Bug 2 — "pure" cumulative rollover (no history) is unstable under edits
Naive fix for bug 1: a running balance since an anchor (`Σ budget − real spending`
for each month). But if each term of the sum uses the **current** budget value
(since the decision was "budget = a single value, no history"), editing the
budget today retroactively recomputes the **entire accumulated past**, with no
time limit — unlike the daily cap, which only reopens days of the current month.

### Decided fix (but superseded by Version 2 below)
Monthly snapshot inside the `settings` table itself (no new migration):
- `budget.monthly_amount` — current/editable value.
- `budget.monthly_amount.<YYYY-MM>` — frozen snapshot of each month, created on
  that month's first read; editing the budget in the current month updates both
  keys together, past months are never touched.
- `budget.first_configured_month` — anchor written once, so rollover is not
  applied retroactively to months before a budget was configured.
- Cumulative rollover = `Σ (snapshot_of_month(m) − real_spending(m))` from
  `first_configured_month` to the previous month — stable, does not cancel itself
  out and is not distorted by future edits.

## Version 2 (proposed during discussion, not designed in detail): budget = real balance

Instead of the user configuring a manual number ("monthly budget"), use the
**real account balances** (`financial_accounts.balance`, already synced via
Pluggy) as the cap's basis:
```
raw_cap(day N) ≈ available_balance_now / remaining_days
```

### Why it is better
- **Kills both rollover bugs outright** — the real balance carries the history
  by itself (no monthly snapshot, anchor or cumulative sum needed).
- **Removes the dependency on correct categorization** — transfers between own
  accounts cancel by summing the balances of all accounts (outflow of one,
  inflow of another, net zero), with no need to filter on
  `kind='expense'`/`kind='transfer'`.
- Reflects real cash timing (tight before payday, looser after) instead of an
  artificial average.

### What stays open / is more complex
- **`financial_accounts.balance` is just a snapshot** (updated on every sync,
  `internal/syncsvc/service.go`), not a historical series — to run the damping
  loop day by day within the current month, the balance trajectory must be
  rebuilt from transactions (today's balance minus/plus each day's movements);
  it is not a direct value.
- **`account_type`/`account_subtype` are raw text from Pluggy**
  (`internal/pluggy/mapping.go`, `payload["type"]`/`payload["subtype"]`), **with
  no validated enum in this repo** — no `CHECK` constraint (unlike
  `categories.kind`), and nowhere in the code assumes specific values today.
- **Outdated since the idea was written:** it claimed no API or screen exposed
  account balances. `GET /api/accounts` and `/contas-e-cartoes` exist now; check
  what they expose before designing.

### Open question: which accounts go into the "available balance" sum?
Discussed, **not decided** — three options, with trade-offs:

1. **Sum all accounts** (checking + savings + investment + credit card as
   negative debt). Mathematically cleanest — transfers and bill payments cancel
   themselves. But money kept in savings/investment would lower today's cap as
   much as money in checking, which may not match what the user feels as
   "available".
2. **Checking accounts only** (exclude savings/investment/card). More faithful to
   "money really available to spend now", but requires distinguishing account
   types from Pluggy's raw `account_type`/`account_subtype` — with no validated
   enum, it would need real sync data to find the exact values used, or else a
   new screen for the user to mark them manually.
3. **User chooses manually which accounts count**. Most correct and flexible, but
   requires building an accounts screen from scratch just for that toggle —
   a much bigger scope than the other two.

## Reusable technical pieces (valid for either version)

- `internal/money/money.go`: `Classify`, `SelectEffectiveMoney`, `Eligibility`,
  `CanonicalDecimal`.
- `internal/money/period.go`: `Date`, `PeriodFor` (`GroupDay`/`GroupMonth`) —
  day/month bucketing in local time given a time zone.
- `internal/payables/eligibility.go`, `summary.go`: the "recompute everything on
  read, never persist derived values" pattern to mirror.
- `internal/settings/store.go`: plain-text key-value `Get`/`Set`
  (`encrypted=false, unlockKey=nil`), reusable for any new config without a
  migration.
- `internal/httpapi/transactions_handlers.go` `handleSpendingByCategory`:
  per-request `?timezone=` + `time.LoadLocation`, with no time zone persisted on
  the server — reuse the same for "today".
- **Confirmed performance caution**: `internal/transactions/query.go`'s
  `fetchAllViews` does `SELECT *` with no `WHERE` and filters everything in Go —
  acceptable for current use, but this feature must not reuse that pattern in a
  loop (one query per day of the month); it needs bounded queries with
  `WHERE occurred_at >= ? AND occurred_at < ?`, using the existing index
  `ix_financial_transactions_occurred_at_id_desc`.

## Next steps when resuming
1. Decide Version 1 (manual budget, with the monthly snapshot fix) vs. Version 2
   (real balance) — Version 2 is more elegant but has a larger scope (exposing
   account balance is new territory for the app).
2. If Version 2: decide the account scope (one of the 3 options above) and look at
   real Pluggy sync data to learn the available `account_type`/`account_subtype`
   values before designing any automatic filter.
