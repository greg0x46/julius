# Payables (debts and receivables)

> Living reference — what exists and works today. Keep it updated when the
> feature changes. Engine behind it: Payables (section 4 of
> `.specs/motores-de-dominio.md`). Folder and route name: `pendencias`.

## What it is

Tracks a real target: a total (debt or receivable) paid down by linked real
transactions, with settled/remaining/status always recomputed on read, never
persisted.

## State vs. ideal

`internal/payables` fully replaced the old `internal/debts` (gone from the
code). `Kind` covers only `debt`/`receivable` and will stay that way: goals
("save R$10,000 by December") are a separate package, `internal/goals` (see
[`metas`](../metas/reference.md)), not a Payables `Kind`. A payable is paid down
by linked real transactions; a goal measures a figure owned by another engine.
The name "Payables" therefore remains accurate.

## Backend

`internal/payables` — total/settled/remaining always recomputed on read from
`payable_transaction_links`, never persisted. `summary.go` is the only place
that recomputation lives: `Summarize` (one payable),
`SummarizeAll` (a set, in two queries — what the HTTP list uses) and
`RemainingTotal` (aggregate). The
same sequence used to be reimplemented in four places, two of them in the HTTP
layer. The pure math (settled/remaining/status) is internal to the package:
`Summarize` is the only entry point, so nobody reassembles the pieces
externally.

`Summary` carries its own links (`Links []LinkSummary`, with effective amount
and the transaction's display fields) instead of just a count — the detail
screen formats what that pass already loaded, without reloading every link and
the transaction behind it.

Also includes link eligibility (`eligibility.go`, which today derives the
accepted flow direction from the debt/receivable `Kind`).
The ignored/direction/effective-BRL checks are owned by
`money.BRLLinkIneligibility`, shared with recurring reconciliation. The
already-linked check remains here, after those checks.

The card debt / bill due-date helpers **moved out of here** to
`internal/transactions` (Ledger engine) — see Transactions. The "payables debt
+ card bill" sum of the "Dívida total" widget did not come back here: it joins
two engines and is composed in the handler (`handlePayableTotalOwed`), because
neither owns the number — and that is also where the BRL currency is pinned.

## HTTP routes

`GET/POST /api/payables`, `GET/PUT/DELETE /api/payables/{id}`,
`GET /api/payables/total-owed`, `/total-to-receive`, `/eligible-transactions`,
`POST /api/payables/{id}/links`, `DELETE .../links/{linkId}`, plus
payable-scoped scenario routes — `GET/POST /api/payables/{id}/scenarios` (see
Scenarios).

## Frontend

`/pendencias` — combined list of debts/receivables.
`/pendencias/:id` — detail, including the timeline and the associated payment
plan (scenario).

## Monetary precision

Money forms and plan allocations submit decimal strings; remaining amounts,
minimums and comparisons preserve fractional cents. Read-only BRL figures round
half away from zero ([policy](../../precisao-monetaria.md)).
