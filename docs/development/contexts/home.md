# Home

> Living reference — what exists and works today. Keep it updated when the
> feature changes. Engine behind it: Timeline, the merge engine ("Domain engines" in
> `docs/development/architecture.md`).

## What it is

The landing dashboard. One summary card: the daily spending allowance (the page
hero), today's cash and the projected end-of-month balance beside/under it, then
the selected period's result, entradas and saídas on a thin line. Each figure
appears once above the fold. Below: a warning band when the forecast goes negative,
the balance projection, spending by category, and three cards (wealth, main
goal, total debt) linking to their screens. The header carries the period
control. The first three figures read the backend
`Series`. The client selects historical minima with exact decimal comparisons
and computes the daily allowance (floor, truncation and division over the daily
balances the server returns). These are deliberate exceptions to "Home never
recomputes"; moving them to the server is the natural step if another screen
needs the same figures. Other client-side reading is limited to: the exact
decimal difference between two valued days of the wealth series, and the exact sum of
same-day card purchases for a chart label. Everything else (negative-balance
dates, lowest balance, goal progress/projection, totals owed) is the server's
figure shown as it comes.

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
- **Hypothetical** — a transaction of a standalone scenario, included through
  the active selection or explicit scenario IDs. Standalone scenarios start inactive.

The package **does not know Payables**: a plan's link to a `Payable` is
encapsulated in `internal/scenarios` (`Scenario.PayableID`/`Kind`, via
`ListPlanInstallments`/`SignedAmount`). The anchor reads native balances through `transactions.AccountBalance`, preserving
provider ownership and derived manual balances, then converts them to BRL at the
reference date. Real flows use the shared `internal/reporting` boundary and
historical rates for their reporting dates; `Series.Reporting`,
`BalanceReporting` and native `Totals` disclose coverage separately. Missing
flow or anchor rates return `reporting.IncompleteError`, never a complete
partial balance or period total. Coverage issues identify the currency, account
and reporting date, distinguishing unsupported coverage, missing history and
provider failure. Unsupported feedback does not recommend synchronization;
native account balances remain accessible in Contas e cartões. Frankfurter v2
blend/cache semantics are documented in the
[Transactions reference](transactions.md#fx-provider-and-cache).
Legacy NULL account currency remains BRL.
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
`period_totals`, `simulation` and `scenario_impacts`. `simulation` is `base` plus
the listed scenarios (active scenarios stay in it); each impact is only what
that scenario adds. A window longer than 100 years (`dates.MaxSpanDays`) is a
422, because the series is built day by day.

`GET /api/timeline/range` — the widest window the curve covers (from the oldest
transaction to the last planned installment), used by the "whole period" option.
Its start is clamped so the window never exceeds 100 years: a stray old date
(a mistyped year) cannot stretch it, and stored rows are left untouched. The
other range reads whose cost grows with the span (recurrence occurrences,
planned projections, investment portfolio and yield, the net-worth series) apply
the same 100-year cap.

## Frontend

The authenticated shell (`frontend/src/app/App.tsx`) renders `BuildFooter` once
through the local `App` shell on every page. Its right-aligned, muted caption
shows `Julius <build label> · <7-character SHA>` with a link to the full commit
in GitHub. Missing/invalid SHA omits the separator and link; missing label falls
back to `dev`. Vite supplies Git/default metadata at build time; CI supplies
`stage` or the selected release tag and exact SHA. There is no metadata API or
GitHub lookup. The footer stays in document flow, includes bottom safe-area
padding, and reserves room on mobile pages with a fixed bottom action bar.

Home (`/`). Layout by named areas (`styles/home.css`): wide screens stack top
summary card, alert, chart (full width), then categories on the left and the
three cards on the right; a phone is one column in that order, with the three
cards as one list of rows (figure and detail on one line, a chevron, empty
states as a single quiet line; sparkline and secondary notes dropped).
- `DailyAllowanceCard` — the hero, "Pode gastar por dia até DD/MM" ("R$ X / dia";
  not a reserve nor a budget). Runs its own query through `useMonthTimeline`
  with `reference_date = from = today` and `to = last day of the month`,
  independent of the selected period: `base.points` has one point per day,
  today's first, already net of real entries, plan installments, unreconciled
  recurrences and bills due today.
  Value = the minimum, over days k = 1..N (today = 1), of
  `max(0, min(balance_k, starting_balance)) / k`, truncated to the cent
  (`presentation/dailyAllowance.ts`): spending d per day leaves
  `balance_k - k·d` on day k, so that minimum is the largest d that never
  zeroes the balance — an early dip weighs more than the same dip at month end.
  The `starting_balance` cap (cash now) keeps money that has not arrived out of
  the allowance: a future inflow only offsets the outflows already due, and any
  surplus never becomes spendable.
  Assumes spending at the end of each day. The cent math (`clampBRLAtZero`,
  `minBRL`, `divideBRLFloor`, which rejects negative values) lives in
  `presentation/money.ts`. "Today" rolls over at midnight (`useToday`). The
  React Query key is by value, so cards with an identical window share one
  request. If a background refetch fails it keeps the last value; the error
  state shows only with no data at all. Nothing is persisted.
- `MonthBalancesCard` — "Saldo hoje" (`starting_balance`, the cash now) and
  "Em DD/MM" tagged "projetado" (the last point of the same month series:
  end-of-day balance on the last day, still including what has not happened).
  Same request as the allowance; the stale-rate notice is the allowance card's.
- `PeriodBalanceCard` — balance, inflows and outflows of the selected period
  (`period_totals`), with historical BRL consolidation, explicit missing-rate
  states and expandable native currency totals for mixed-currency periods. No
  longer the page hero: one compact line (result, entradas, saídas); the
  "incluindo cenários ativos" note is hidden on a phone unless the period is
  empty.
- `NegativeBalanceAlert` — warning band (warning tokens) "O saldo fica negativo
  em DD/MM se nada mudar. Ponto mais baixo: … em DD/MM." with an inline link to
  the period's transactions. The lowest point is left out when it is the last
  day of the chart's window (that figure is already the end balance). Reads `first_negative` and `lowest_balance` of the
  chart's series (from today onward; null `first_negative` or a loading/error
  state renders nothing). The year is added when it differs from today's. It
  replaces the alert that used to sit inside the chart.
- `ProjectionSummaryCard` — `ProjectionTimeline` with a figure strip only for
  what nothing else above the fold says: the lowest balance (`lowest_balance`,
  or the historical minimum for a window that ends today) unless the forecast
  is negative (the alert says it) or it equals the month-end balance already in
  the summary card; and the balance at the end of a future window unless that
  is the month end. No strip when both are repeats. Uses the shared selected period, including the whole-period option,
  through `useHomeProjection` (shared with the alert, one request). Historical
  minimum selection and the empty-balance check use exact decimal comparisons.
  Equal minima retain the first date; neither converts monetary values to
  JavaScript numbers. The chart preserves the server's end-of-day balance for
  today, including commitments and card bills due today.
  `ProjectionTimeline`: realized solid in ink, projected dashed in brand green,
  short legend (Realizado / Projetado / Evento / Negativo; "Menor saldo" only
  when there is no negative marker), "Hoje" line, zero line, lowest-balance dot, a labelled dot at the first
  negative day, and up to 3 labelled hollow dots (1 on a phone) for the biggest
  upcoming events (`presentation/chartEvents.ts`): Confirmado/Projetado entries
  after today — card purchases due the same day summed as one "Fatura",
  plan installments as "Parcela", recurrences by name — ranked by exact amount,
  one per day, never on the first negative day. Labels have a halo in the surface color and
  sit to the dot's left in the last 40% of the window so they stay inside the
  plot. The chart includes scenarios explicitly activated through Settings →
  Scenarios; there is no separate simulation comparison in this view.
- `SpendingByCategoryCard` — spending by category, read from
  `GET /api/transactions/category-breakdown` (Transactions context).
- `NetWorthSummaryCard` — link to `/patrimonio`: latest wealth (cash plus
  investments) in the response's currency and a sparkline plus the exact change
  since the oldest valued day in the last 30 days
  (`presentation/netWorthTrend.ts`). Every day is derived on read, so the
  window is a plain date range; days that cannot be valued (`unavailable`) are
  skipped, never read as zero, and without an earlier valued day it says there
  is not enough history. An unvaluable latest day says "Indisponível", never
  zero.
- `PrimaryGoalCard` — link to `/metas`: the first goal in the Metas page order
  (open goals first, smallest target first), with progress meter, current/target
  in BRL (goals are never converted) and the server's projection ("No ritmo
  para …" / "Atrás do ritmo … aporte de …"); "mais N metas" when there are
  others. No goals: one quiet line with a "Criar meta" link.
- `TotalOwedCard` — link to `/pendencias`: `GET /api/payables/total-owed`, open
  debts ("Pendências") plus card bills still accruing ("faturas abertas"), in
  the currency the response declares (BRL by construction).

## Notes

**Scenario simulation has no UI entry point today.** `GET /api/timeline` still
accepts `scenario_ids` and returns `simulation` and `scenario_impacts` — the
backend is complete — but no screen consumes them: Home requests the base
series with its active-scenario selection, without an explicit simulation.
The components that did this
(`ScenarioMultiSelect`, `BaseVsSimulationCompare`, `ProjectionComposition`) were
removed as dead; recover them from git history if simulation returns to the
screen.

`CertaintyTier` only selects which entries get a chart marker (Confirmado and
Projetado); it is not shown as a badge — the one that did (`EntryOriginBadge`)
was removed.

## API contract verification

Wire semantics and calculation ownership are inventoried in
[`api-contracts.md`](../api-contracts.md). Shared synthetic responses are
checked against Go DTOs and the production TypeScript parsers by the existing
backend/frontend CI tests.

## Reference valuation

`/api/timeline` values its consolidated BRL series in the persisted financial
reference currency (BRL by default). One current FX rate applies to the base,
simulation, entries, daily balances, period totals and scenario deltas; past
and projected points are current-rate equivalents, not an FX forecast or a
historical-rate cash reconstruction. This keeps the balance walk and all Home
figures on the same basis. The original BRL eligibility and foreign-currency
exclusions are unchanged. Responses include `currency_code` and full `fx`
provenance; missing FX makes the report unavailable and stale FX is disclosed.
Home money figures, daily allowance, accessible links and chart tooltips read
that response currency. Category reports use the same current-rate policy.

## Local chart rendering

The projection uses shared SVG `LinePlot`: realized balances remain solid,
projected and simulated balances dashed, with today/zero references and the
lowest-balance marker. Pointer/touch inspection and a keyboard-accessible date
selector expose the original decimal values and entries; realized days still
open transactions. Category breakdown uses local SVG rings and its linked list.
Inspection follows the selected calendar day across period updates and hides
when that day is outside the displayed period.
No chart library or external UI runtime is required.

## MCP reads

`get_timeline` reads the same `timeline.BuildSeries`, `TotalsForPeriod` and REST
formatters, with required reference/from/to calendar days and optional timezone.
It returns one selected series, period totals, selected IDs, reporting coverage
and reference-currency FX for the current UTC day, matching REST even when the
server uses another timezone. Unlike the application's default active selection,
MCP explicitly selects active non-standalone commitments plus requested IDs;
active standalone hypotheticals require explicit selection. `ScenarioSelection`
is optional on `BuildParams`; omitting it preserves existing application behavior.
Reporting summaries in both MCP and REST share the converted base summary, so
reference-currency conversion occurs once (including the period's coverage).

Tool inputs, output bounds and dev validation: [MCP tool reference](../mcp-tools.md).
