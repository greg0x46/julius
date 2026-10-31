# Categories

> Living reference — what exists and works today. Keep it updated when the
> feature changes.

## What it is

A user-defined category catalog with categorization history. Not a domain
engine — a passive catalog with no matching/computation/merging logic of its own
(see criterion 2 and "Not engines" in `docs/development/architecture.md`).

One exception to "passive": the `transfer` kind carries a rule. A transaction
with a category of this kind is **left out of income/expense totals** (reason
`transfer_category`), because counting both legs of a transfer would count the
same money twice. It does not affect balance, historical wealth or card
debt. `expense`/`income` remain plain labels. The seed ships one category of
this kind (`Transferência entre Contas Próprias`, `00004_categories.sql`); users
can create more.

Category controls and the transaction category picker follow the browser language
selector; server-provided errors may remain in Portuguese. See the [frontend reference](../frontend.md) for overall coverage.

## Backend

`internal/categories` — simple CRUD (`Create/Update/Get/List`) over
`money.CategoryKind`, plus per-transaction category decisions
(`transaction_category_decisions`, append-only history in
`transaction_category_events`).

### Decision origin and precedence

Each decision carries an `origin`; precedence is
`manual > rule > learned > automatic`:

- `manual` — the user chose (`AssignManual`). Never overwritten.
- `rule` — `set_category` action of an automation rule (`ApplyRule`). Overwrites
  `learned` and `automatic`; on an installment purchase it skips siblings the user
  categorized manually.
- `learned` — copied from the **most recent manual** decision made on a similar
  transaction (`ApplyLearned`, `learned.go`). Similar = same normalized
  description (lowercase, collapsed whitespace, ` N/M` installment suffix
  removed) and same `movement_type`. Overwrites `automatic`; propagates to all
  installments of an installment purchase. Applies only to transactions that
  arrive **after** the manual decision — a manual decision never reclassifies
  the past.
- `automatic` — Pluggy `source_category` mapping (`ApplyAutomatic`) or the
  card-bill-payment heuristic (`ApplyAutomaticCardPayment`). Writes only when
  there is no decision.

Order during sync (`syncsvc.upsertTransaction`): card bill → Pluggy mapping →
learned (these three on insert only) → rules (hook, also on update). All share
the upsert's transaction; a failure rolls back data, decisions and events
together, so the next sync can retry. A manual entry without `category_id`
follows the same order (learned → rules).

## HTTP routes

`GET/POST /api/categories`, `PATCH /api/categories/{id}`.

## Frontend

`/categorias` — catalog CRUD.

Category report values use the financial reference currency (default BRL),
with current FX and its provenance. Catalog and transaction category decisions
are unchanged; see [Transactions — Consolidated report valuation](transactions.md#consolidated-report-valuation).

MCP `list_categories` exposes the same catalog, including inactive categories,
with bounded result pages. `search_transactions` retains category decision
origin, and `spending_by_category` reuses the real category-breakdown contract.
See the [tool inventory](settings-authentication.md#mcp-financial-tool-inventory).
`TestMCPFinancialReadsMatchHTTPFixtures` compares these with the REST fixtures.

## MCP writes

MCP can prepare category creation and updates (including deactivation), and
manual transaction categorization. Owner approval in Julius is required for each
mutation. Assignment previews enumerate the domain installment cascade; every
decision/event commits together after a serializable preview recheck. Catalog
updates preserve existing decisions; category deletion is not exposed.

Tool inputs, approval contract, limits and synthetic dev validation:
[MCP financial writes](../mcp-tools.md#financial-writes).
