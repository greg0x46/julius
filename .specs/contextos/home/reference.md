# Home

> Living reference — what exists and works today. Keep it updated when the
> feature changes. Engine behind it: Timeline, the merge engine (section 6 of
> `.specs/motores-de-dominio.md`).

## What it is

The landing dashboard: balance/inflows/outflows of the selected period, the
balance projection (today, end of horizon, lowest balance), the daily spending
allowance until the end of the month, and spending by category. The first three
read the backend `Series`; they differ only in which field each consumes. The
daily allowance is the only computation done on the client (floor, truncation
and division over the daily balances the server returns): a deliberate
exception to "Home never recomputes", kept client-side to avoid a new API field
for one division; moving it to the server is the natural step if another screen
needs the same number.

A monthly retrospective screen once existed (`/relatorio-financeiro`, with
month-by-month breakdown, category drill-down and comparisons); it was removed
together with the `/api/timeline` aggregations only it consumed
(`monthly_breakdown`, `category_breakdown`, `month_over_month`,
`year_over_year`, `category_evolution`). Recover them from git history if the
retrospective comes back.

## Backend

`internal/timeline` — `Series{Points, Entries, StartingBalance, LowestBalance,
FirstNegative}`, `Entry` with `CertaintyTier`
(`realizado`/`confirmado`/`projetado`/`hipotetico`). `BuildSeries` merges **2
sources**: Ledger entries and the unified scenario projection flow
(`projections.List`) — recurrence is no longer a parallel source but
`Scenario{Kind: recurring}`. All 4 tiers are still produced:

- **Realized** — a real transaction; a credit-card entry becomes **Confirmed**,
  re-dated to the bill's due date.
- **Confirmed** — installment of a payable plan (real debt with a planned date).
- **Projected** — a **non-reconciled** recurrence occurrence. An occurrence
  reconciled against a real transaction produces no entry: the real
  transaction already carries the money (it used to, and the amount was counted
  twice). The candidates an occurrence tries to reconcile are scoped to its
  month. `recurrences.Reconciler` decides whether an occurrence is reconciled,
  the same resolver the Recurrences screen reads — so detaching by hand makes
  the occurrence project here again, and reconciling by hand suppresses it. A
  manual link may point to a transaction in the neighboring month; the
  occurrence is suppressed all the same (reconciliation is a fact about the
  occurrence), and the narrow candidate window keeps the effect limited to a
  month boundary.
- **Hypothetical** — a transaction of an explicitly selected standalone
  scenario.

The package **does not know Payables**: a plan's link to a `Payable` is
encapsulated in `internal/scenarios` (`Scenario.PayableID`/`Kind`, via
`ListPlanInstallments`/`SignedAmount`). The anchor reads native balances through `transactions.AccountBalance`, preserving
provider ownership and derived manual balances, then converts them to BRL at the
reference date. Real flows use the shared `internal/reporting` boundary and
historical rates for their reporting dates; `Series.Reporting`,
`BalanceReporting` and native `Totals` disclose coverage separately. Missing
flow or anchor rates return `reporting.IncompleteError`, never a complete
partial balance or period total. Legacy NULL account currency remains BRL.
`Entry.Amount` and reportable values are BRL; original effective currency and
reportable amounts remain on the entry for native drill-down. Balance adjustments enter the current
anchor and the dated balance walk, never transaction income/expense totals or
reporting entries. The walk also reverses adjustments after a historical
window before reconstructing its days. Posted manual movements are already in
the anchor even when future-dated; the walk reverses them before applying them
on their dates, so they affect the curve once. These cash-only inputs respect
account selection and currently handle BRL manual accounts only.

The reference-day point is the balance at the **end** of that day, not the
anchor balance: a real transaction from today is already in cash and enters the
anchor; what is only planned for today (unreconciled recurrence, plan
installment, scenario) and a card bill due today have not moved money yet, and
are walked as future entries (`awaitingCash` in `build.go`). A bill due today
that was already paid cancels out (real bank leg in cash + card leg with the
same due date cancelling the purchases); bills due before the reference date
are still treated as paid (`cashEntries`). `StartingBalance` remains the cash
as of now.

Each `Entry` carries two readings of the same money. The balance curve
(`Points`) uses `Amount` and includes every movement that moved cash — the
`BuildSeries` filter is `MovesCash`, not `Included`. Period inflows and outflows
(`TotalsForPeriod`) use `ReportableAmount`, the reportable value that
`transactions.toItem` zeroes for what totals eligibility excludes. So a
transfer between own accounts (transfer category, or the installment reconciled
as an investment contribution/withdrawal) reduces the balance but stays out of
inflows and outflows.

## HTTP routes

`GET /api/timeline` — `reference_date`, `from`, `to`, filters (`account_ids`,
`category_ids`, `card_numbers`) and `scenario_ids`. Returns `base`,
`period_totals`, `simulation` and `scenario_impacts`.

`GET /api/timeline/range` — the widest window the curve covers (from the oldest
transaction to the last planned installment), used by the "whole period" option.

## Frontend

Home (`/`):
- `PeriodBalanceCard` — balance, inflows and outflows of the period
  (`period_totals`), with historical BRL consolidation, explicit missing-rate
  states and expandable native currency totals for mixed-currency periods.
- `ProjectionSummaryCard` — balance today, balance at end of horizon, lowest
  balance, plus `ProjectionTimeline`. Selectable horizon (end of month, 3, 6,
  12 months; 3 by default).
- `DailyAllowanceCard` — daily spending allowance ("R$ X / dia"), below the
  period result, as a compact figure (`Money` size `row`; the page hero is only
  the period result). It runs its own query with `reference_date = from = today`
  and `to = last day of the month`, independent of the selected period:
  `base.points` has one point per day, today's first, already net of real
  entries, plan installments, unreconciled recurrences and bills due today.
  Value = the minimum, over days k = 1..N (today = 1), of `max(0, balance_k) / k`,
  truncated to the cent (`presentation/dailyAllowance.ts`): spending d per day
  leaves `balance_k - k·d` on day k, so that minimum is the largest d that never
  zeroes the balance — an early dip weighs more than the same dip at month end.
  Assumes spending at the end of each day. The cent math (`clampBRLAtZero`,
  `minBRL`, `divideBRLFloor`, which rejects negative values) lives in
  `presentation/money.ts`. "Today" rolls over at midnight (`useToday`, shared
  with `ProjectionSummaryCard`, each card with its own timer). The React Query
  key is by value: the query is shared with another card only if the window is
  identical, which the default period (start of month) is not. If a background
  refetch fails it keeps the last value; the error state shows only with no data
  at all. Nothing is persisted: the value follows any `timeline` invalidation.
  It is not a reserve nor a budget.
- `SpendingByCategoryCard` — spending by category, read from
  `GET /api/transactions/category-breakdown` (Transactions context).

## Notes

**Scenario simulation has no UI entry point today.** `GET /api/timeline` still
accepts `scenario_ids` and returns `simulation` and `scenario_impacts` — the
backend is complete — but no screen consumes them: Home is deliberately the base
only ("no hypothetical scenarios"). The components that did this
(`ScenarioMultiSelect`, `BaseVsSimulationCompare`, `ProjectionComposition`) were
removed as dead; recover them from git history if simulation returns to the
screen.

For the same reason `CertaintyTier` appears nowhere in the UI — the certainty
vocabulary (principle 4 of `.specs/motores-de-dominio.md`) lives only in the
backend today. The badge that showed it (`EntryOriginBadge`) and its label map
were removed with it.
