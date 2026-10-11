# Transactions

> Living reference — what exists and works today. Keep it updated when the
> feature changes. Engine behind it: the Ledger engine (section 1 of
> `.specs/motores-de-dominio.md`).

## What it is

A searchable/filterable list of real transactions, with manual inclusion/exclusion
(e.g. ignoring a chargeback or duplicate) and categorization. It also exposes
accounts, cards and investments — adjacent data served by the same presentation
handlers.

Besides Pluggy-synced transactions, an entry can be created by hand
(`origin='manual'`) in an account that already exists — same entity, same
categorization/inclusion/automation rules, just another way in. Editing and
deleting apply only to manual entries; a synced one never changes here. The
account balance still comes only from the provider — a manual entry never
changes it (see Notes).

Categorizing as a transfer between own accounts (`kind='transfer'`) also takes
the entry out of totals, with its own reason (`transfer_category`) and without
marking it ignored — see the note on the two paths in
`.specs/motores-de-dominio.md` section 1.

## Backend

- `internal/transactions` — `QueryRequest`/`Filters`/`Item`/`Page`/`Group`,
  inclusion/category mutation, and `manual.go`
  (`CreateManual`/`UpdateManual`/`DeleteManual`) for the manual entry path.
- `internal/money` — the rules that decide what counts toward totals:
  `Classify`, `SelectEffectiveMoney`, `Considered`/`Ignored`, and the `transfer`
  `CategoryKind`. Origin-agnostic — Pluggy and manual entries populate the same
  table, and the engine does not presume which. `Eligibility` takes the kind of
  the assigned category; callers computing *balance* instead of flow pass `""`
  so the transfer rule does not leak (`networth/backfill.go`,
  `transactions/cardtotal.go`).
- `financial_transactions.origin` (`synced`/`manual`) distinguishes the two
  paths; sync-specific columns (`source_id`, `external_id`,
  `current_raw_import_id`, `normalized_hash`) are `NULL` on a manual row — there
  is no fake `data_sources`/`raw_import` behind it. `deleted_at` is the
  soft-delete of a manual entry: `transaction_category_events`/
  `transaction_inclusion_events` are append-only with `ON DELETE RESTRICT` to
  `financial_transactions`, so an already categorized or ignored manual row
  cannot be truly deleted — `DeleteManual` sets `deleted_at` instead, and
  `viewSelect` (the base of every SELECT in this package, including `GetItem`)
  filters on `deleted_at IS NULL`. A manual entry never enters
  `networth.cashDeltasDescending`/`earliestCashTransactionDay` (filter
  `origin = 'synced'`) — see Notes.
- Credit card (`cardflow.go`/`cardtotal.go`, moved from `internal/payables`):
  `CardDueDates`/`ProjectedEntryDate` (the date a card purchase becomes a cash
  outflow — the bill's due date, not `occurred_at`), `CreditAccountIDs`,
  `CardMetadataByTransaction` and `CreditCardTransactionTotal(At)` (how much is
  owed in the open cycle, computed from eligible entries, never from
  `financial_accounts.balance`).
- `balance.go` — `CashOnHand`: money in accounts (sum of reported balances of
  non-credit accounts), the single implementation, used by the Timeline and Net
  Worth. See the balance note in `.specs/motores-de-dominio.md` section 1 for
  why an ignored transaction stays inside the balance.

## HTTP routes

- `POST /api/transactions/query`, `GET /api/transactions/spending-by-category`,
  `GET /api/transactions/category-breakdown`,
  `PUT /api/transactions/{id}/inclusion`, `PUT /api/transactions/{id}/category`.
- In `POST /api/transactions/query`, `filters.classification` accepts `inflow`
  (positive amount), `outflow` (negative amount) and `unclassified`; the UI
  offers only inflows and outflows. Any other value is 400
  (`invalid-classification`). `filters.origin` accepts `manual`, `synced`.
- Manual entry: `POST /api/transactions` (create), `PUT /api/transactions/{id}`
  (edit), `DELETE /api/transactions/{id}` (delete) — all three accept only a
  transaction with `origin='manual'` (409 otherwise). The flow lives in
  `ledger.Service` (`internal/ledger`); handlers only parse the request and map
  domain errors to Problem. Creation applies the learned category and runs
  automation (`automation.ApplyToNewTransactionWithQuerier`) as a sync would,
  then applies an explicit `category_id` on top if present — the user's choice
  always beats what automation decided. All in the same database transaction.
  `ledger.ApplyNewTransactionDecisions` is the same sequence, reused by
  statement-import confirmation.
- Clients that need to retry `POST /api/transactions` safely send an
  `Idempotency-Key` header (8 to 128 ASCII characters, no spaces). The same key
  and the same entry return the original ID; the same key with different data
  returns 409. The key is reserved together with the entry in the database
  transaction and belongs to the installation's owner. Without the header the
  legacy behavior remains: each POST creates a new entry.
- Accounts/cards: `GET /api/accounts[/{id}][/cards|/bills]`,
  `PUT /api/accounts/{id}/closing-day`.
- Investments: `GET /api/investments[/{id}][/transactions]`.

## Frontend

- `/transacoes` — main ledger: filters, grouping, totals, category/inclusion
  edit drawer, "Novo lançamento manual" button and — for a manual entry — a
  "Manual" tag in the list and drawer, with Editar/Excluir actions.
  `ManualTransactionForm` serves create and edit.
- `/contas-e-cartoes` + `/contas-e-cartoes/:id`.
- `/investimentos` + `/investimentos/:id`.

## Notes

Reconciled investments add `investment_transfer_amount` and `reportable_amount`
to the entry. The contribution/withdrawal portion does not count in
income/expense totals, but the full amount remains a cash movement. The reason
for full exclusion is `investment_transfer`. See
[`investimentos/reference.md`](../investimentos/reference.md).

Ingestion: Pluggy, manual entry and statement import are the entry paths today,
all data providers, not part of this engine — see Open Banking sync and
`.specs/motores-de-dominio.md` section 1. Creating a 100% manual account (with
no Pluggy link) is out of scope for now — every manual entry points to an
account that already exists (statement import creates its own file accounts).

User decisions live in their own tables, which ingestion never rewrites: manual
category and inclusion (`transaction_category_*`, `transaction_inclusion_*`),
`payable_transaction_links` and `scenario_realizations`. A provider update
(PENDING → POSTED, corrections) keeps the entry's `id`. Full policy in
[`sincronizacao-open-banking/reference.md`](../sincronizacao-open-banking/reference.md).

The account balance (`CashOnHand`, Net Worth) never reflects a manual entry —
only what the provider reports in `financial_accounts.balance`. A manual entry
affects the statement and income/expense totals, but never the displayed
account balance or the historical net-worth reconstruction (`networth.Backfill`
filters `origin = 'synced'` explicitly for that). Credit card is the deliberate
exception: `CreditCardTransactionTotal` already sums eligible entries (never the
reported balance), so a manual entry on a credit account counts normally toward
the open bill.
