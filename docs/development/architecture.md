# Architecture

Julius serves one owner from one Go binary: `net/http` routes in
`internal/httpapi`, domain packages under `internal/`, and an embedded
React/TypeScript frontend. SQLite supports one process; Postgres supports
multiple instances. Pluggy, CSV and manual input feed the same ledger.

This reference describes implemented boundaries. The
[financial invariant matrix](financial-invariants.md) owns financial rules and
pinning tests; [context decisions](context-decisions.md) records policies that
must remain distinct. Feature inventories remain linked from that guide.

## Domain engines

An engine provides reusable behavior across contexts, independent of a provider.
A catalog alone is not an engine. Reuse existing owners before extracting a
shared abstraction; similar arithmetic can represent different decisions.

| Engine | Packages | Responsibility and boundary |
| --- | --- | --- |
| Ledger | `money`, `transactions`, `ledger` | Classify movements, choose effective money, determine eligibility, query real entries and orchestrate writes. Ingestion supplies facts; local decisions are separate annotations. |
| Rules | `rules` | Pure, stateless AND/OR matching of candidates against field/operator/value conditions. No persistence or effects. |
| Automation | `automation` | Apply `ignore`, `set_category` and `reconcile` actions to new/changed entries or retroactively, preserving manual decisions. Recurrence resolution consumes reconcile rules. |
| Payables | `payables` | Real debt/receivable targets paid down by linked real entries. `Summarize` owns derived settled/remaining/status. Card debt belongs to transactions. |
| Scenarios | `scenarios`, read boundary `projections` | User-authored plans: `debt_plan`, `receivable_plan`, `standalone` and `recurring`. `projections.List` provides one event shape for consumers. |
| Timeline | `timeline` | Merge Ledger and scenario projections into one series. Its payable knowledge is encapsulated by Scenarios. Cash amounts and reportable amounts remain separate. |

Recurrences compose schedules, Rules and Automation. A commitment is a
`Scenario{Kind: recurring}` plus `scenario_recurring_schedules`, sharing one
scenario ID across projections, automation targets and occurrence decisions.
`internal/recurrences` owns occurrence generation and reconciliation; a manually
detached or linked occurrence takes precedence over rules.

Categories own the catalog and decision history. Only category kind `transfer`
affects financial eligibility; `income` and `expense` are labels. Investments
own custody accounts, positions, operation replay, valuation, cost and
reconciliation. Quotes/market data supply prices without writing valuation
operations. Wealth (`networth`, a historical package/route name) composes cash
plus investments and derives daily history. Goals measure a global metric from
its owner; they are separate from payables because they measure stock rather
than settling linked flows, and they add to no total.

## Ledger, reporting and cash

`money.Classify` and `SelectEffectiveMoney` are shared rules. Effective money
prefers a complete account-currency pair over the transaction-currency pair.
`money.Amount` carries value and ISO currency; `money.Balances` keeps currencies
separate. Adding different currencies fails, and missing/invalid codes are
errors rather than an implicit BRL conversion.

`money.Eligibility` reports the first exclusion in this order:
`ignored`, `unclassified`, `ineligible_status`, `missing_money_pair`,
`zero_value`, `transfer_category`. Transfer comes last because `MovedCash`
retains otherwise eligible transfer principal as cash. Linked own-account
principal and reconciled investment principal also remain cash while leaving
income/expense totals. Separate fees use ordinary expense rules.

Both `PENDING` and `POSTED` pass status eligibility. A pending card purchase can
already be an expense. Consumers separately determine the report period, bill
cycle and cash date; eligibility is not proof of settlement.

Synced cash uses provider balances. Manual bank cash uses opening balance plus
posted undeleted movements and dated adjustment deltas. Category/inclusion
annotations do not change either balance. Manual transactions on synced
accounts do not rewrite provider balances, although card bills count them.
`CREDIT` accounts are not cash. Card debt uses eligible cycle entries and
bypasses transfer-category exclusion so a bill payment still reduces debt.

`reporting` converts eligible flows at historical reporting-date rates and
exposes native totals and FX coverage. `valuation`/`exchange` value cash and
holdings in BRL; wealth reconstructs native history before each day's
conversion. `httpapi.reportValuation` converts consolidated outputs to the
configured reference currency (BRL by default), retaining provenance and stale
flags without rewriting native records. Missing FX must not look like a
complete total. Payables, scenarios, goals and manual investments retain their
BRL contracts; reference currency is a valuation preference.

## Cross-cutting principles

1. **Recompute derived figures on read.** Payable summaries, scenario event
   status, resolved recurrence reconciliation, investment quote values, wealth
   history and goal progress come from their owning sources. Persist facts and
   decisions, not a second copy of their result.
2. **Separate external facts from local annotations.** Planning rows have no
   hard FK to real entries. Explicit link tables (`payable_transaction_links`,
   `scenario_realizations`) hold relationships. A reconciliation row stores a
   decision, including detach, rather than a cached resolved state. Ingestion
   preserves user categories, inclusion and planning/investment links.
3. **Make hypothetical selection explicit.** Standalone scenarios start
   inactive. There is no global simulation mode. Current Timeline defaults to
   active scenarios and adds requested IDs for simulations; `is_active` is
   persisted today. Explicit projection selection can select none. MCP selects
   active non-standalone commitments plus requested IDs, excluding even an
   active standalone scenario unless requested. Realized wealth, balances and
   goals never consume that selection.
4. **Carry certainty with projected entries.** Timeline tiers are Realized,
   Confirmed, Projected and Hypothetical (`realizado`, `confirmado`, `projetado`,
   `hipotetico`). Card entries move to the bill due date as Confirmed; payable
   plans are Confirmed, unreconciled recurrences Projected and standalone
   events Hypothetical. Realized events suppress their projected counterpart.
   The chart groups realized/projected readings; it does not expose every tier
   as a separate screen control.
5. **Share calendar-day rules.** `dates.Day` uses UTC midnight of the caller's
   wall-clock date. `DaysInMonth` supplies timezone-independent calendar month
   lengths and `AddMonths` steps months, clamping to month end. Reporting
   buckets first choose the request timezone; bill-cycle calculations
   deliberately use `time.Local`. Generated plan installments and recurring
   schedules both clamp to month end.
6. **Keep cross-engine writes atomic.** Pass the caller's `Querier` through
   `ledger.Service` and `ApplyNewTransactionDecisions`; do not open nested
   transactions for categorization, rules or link cleanup. Sync commits each
   record and its decisions/events together; statement confirmation commits
   the whole import together.
7. **Preserve exact monetary boundaries.** Use decimal strings, shared database
   scan adapters and operation-specific rounding. See
   [monetary precision](monetary-precision.md) and [API contracts](api-contracts.md).

## Persistence and delivery boundaries

`internal/db/types` normalizes SQLite/Postgres timestamps, days, booleans, IDs
and exact decimals before domain logic. Decimal reads accept text/integers and
reject floats. Postgres stores native numeric/temporal/UUID/boolean values;
SQLite stores exact text and integer flags. Migrations are forward-only with
matching numbers/names per dialect. The
[migration reference](../../internal/db/migrations/README.md) owns storage,
legacy conversion, backup and validation details.

HTTP handlers use Go ServeMux patterns and Problem responses, with private API
reads, no-store responses, origin/CSRF checks on writes and safe database
errors. `httplog` is the single request middleware; error/event values are
controlled literals, never payloads or secrets. MCP reuses the domain services;
financial writes have separate scopes and browser owner approval. See
[security](../self-hosting/security.md) and [MCP integration](../mcp.md).

Frontend consumers use the shared API transport and runtime parsers. Backend
read models own financial totals; intentional client metrics are inventoried
in [API contracts](api-contracts.md#calculation-ownership-and-frontend-inventory).
Local UI controls and SVG charts use shared theme tokens; numeric coordinates
are for geometry only. See [frontend testing](../../frontend/TESTING.md).
