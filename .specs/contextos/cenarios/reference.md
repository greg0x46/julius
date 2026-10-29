# Scenarios

> Living reference — what exists and works today. Keep it updated when the
> feature changes. Engine behind it: Scenarios, the hypothetical layer (section
> 5 of `.specs/motores-de-dominio.md`).

## What it is

User-authored projections, never synced facts. One entity
(`Scenario`/`ScenarioTransaction`), several kinds: attached to a `Payable` (the
installment plan of a specific debt/receivable), free (`standalone`, a generic
"what if" with no `Payable` — e.g. trip, new job), or `recurring` (see
Recurrences). A scenario never contaminates real totals by default — it enters a
report only when the caller passes `scenario_id` explicitly.

## State vs. ideal

Fully implemented as described: `Kind` covers `debt_plan`, `receivable_plan`,
`standalone` and `recurring`; `PayableID` is null for `standalone`/`recurring`
and required for the plan kinds (enforced by a database constraint and in the
HTTP layer).

## Backend

`internal/scenarios`. Besides CRUD: `SignedAmount` (the cash direction of an
installment, derived from the scenario's own `Kind` — what lets the Timeline
project a plan without knowing `payables`), `ListPlanInstallments` (unrealized
installments of every plan in a range) and `Summarize`/`SummarizeTransaction`
(per-installment status and accumulated deviation, previously computed in the
HTTP handler). `internal/projections` is the read boundary consumers such as the
Timeline use; it returns one event shape for plans, standalone and recurring
scenarios.

## HTTP routes

`GET/POST /api/scenarios`, `GET/DELETE /api/scenarios/{id}`,
`POST .../generate-installments`, `POST .../readjust`,
`POST/PUT/DELETE .../transactions[/{transactionId}]`,
`POST/DELETE .../transactions/{transactionId}/realizations[/{realizationId}]`.

Nested installment updates/deletions and realization creation/deletion validate
that the installment belongs to the scenario named in the URL. A missing scenario
or a mismatched/missing installment returns 404 before changing any data; deleting
a realization also checks that it belongs to the named installment.

Readjusting with `abater_do_final` keeps the plan's cadence, inferred from the
most common gap between its installments (weekly, biweekly, otherwise monthly).

Linking a real transaction to a standalone event (`PUT /api/scenarios/{id}/planned-transactions/{eventKey}/realization`)
requires a considered BRL transaction flowing in the direction of the event's
amount sign, like a recurring occurrence. Installment status and accumulated
deviation compare allocations (positive magnitudes) with the amount's
magnitude, so a fully realized standalone expense is `paga`.

## Frontend

`/cenarios` — "Simule decisões hipotéticas" (standalone scenarios). The payment
plan of a specific debt/receivable is embedded in `/pendencias/:id` (see
Payables).

## Monetary precision

Equal installment generation floors the exact quotient to BRL cents; the last
installment keeps the full remainder, including fractional cents. It does not
round division before flooring ([policy](../../precisao-monetaria.md)).

Fixed-amount installment generation and reduce-term readjustment derive the
count from exact integer quotient and remainder. Every installment stays at or
below the chosen amount, including remainders beyond 16 decimal places.
Generation and readjustment reject more than 10,000 installments per operation
before allocating a plan or replacing existing installments; HTTP returns 422.

## MCP reads

`list_scenarios` discovers IDs/kinds/activation flags without applying any
scenario. `list_projection_events` returns the unified projection read model
(events, dates, direction, tiers, realization state), and `get_timeline` uses
`BuildParams.ScenarioSelection = explicit`. Both select active non-standalone
commitments plus explicitly requested IDs. Omitted/empty IDs add no hypothetical
data, including when a standalone scenario is marked active. Unknown IDs are
rejected; selections allow at most 20 additional IDs. Realized wealth/goals/
payable balances never consume this selection.

Tool inputs, output bounds and dev validation: [README MCP](../../../README.md#mcp).

## MCP writes

`create_scenario`, `update_scenario`, `delete_scenario` manage standalone
scenarios only. Creation is inactive; update renames via
`scenarios.RenameScenario`, preserving activation. Planned children use
`create_planned_transaction`, `update_planned_transaction` and
`delete_planned_transaction`, with top-level `scenario_id` and the child's `id`
for existing entries. Requests reuse REST description/amount/projected_at/category
validation; signed amounts are exact BRL strings. Parent kind and child ownership
are checked before approval and again at execution. Payable plans and recurring
scenarios cannot be modified through these tools.

All operations require `mcp:write` and the existing browser owner approval.
Previews state hypothetical selection and deletion effects. Deletion snapshots
include planned children and realization links; before-state is rechecked in the
same serializable transaction as deletion. Cascades remove planning data/links,
never real transactions. No write changes MCP's explicit `scenario_ids` selection
or realized totals/balances. Synthetic lifecycle, timeline isolation, stale cascade,
invalid amount/date/ownership and authorization tests live in
`internal/httpapi/mcp_planning_writes_test.go` and
`internal/mcp/planning_writes_test.go` (SQLite and Postgres).
