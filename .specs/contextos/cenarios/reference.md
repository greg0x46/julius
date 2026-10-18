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
