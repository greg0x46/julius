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
deleting apply only to manual entries; a synced one never changes here.
Synced account balances still come only from the provider. Manual bank accounts derive their native balance from opening balance, posted movements
and explicit adjustments (see Notes).

Categorizing as a transfer between own accounts (`kind='transfer'`) also takes
the entry out of totals, with its own reason (`transfer_category`) and without
marking it ignored — see the note on reporting exclusions in
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
  filters on `deleted_at IS NULL`. Manual entries on synced accounts never
  enter net-worth balance reconstruction; posted movements on manual accounts do — see Notes.
- Credit card (`cardflow.go`/`cardtotal.go`, moved from `internal/payables`):
  `CardDueDates`/`ProjectedEntryDate` (the date a card purchase becomes a cash
  outflow — the bill's due date, not `occurred_at`), `CreditAccountIDs`,
  `CardMetadataByTransaction` and `CreditCardTransactionTotal(At)` (how much is
  owed in the open cycle, computed from eligible entries, never from
  `financial_accounts.balance`).
- `balance.go` — `CashOnHandByCurrency`: money in non-credit accounts
  (provider-reported synced balances plus derived manual balances), one total
  per currency in `money.Balances`; `CashOnHandIn` returns a single currency's
  total and is what Net Worth reads (BRL). A cash account with a balance and a
  missing or non-ISO-4217 `currency_code` is an explicit error naming the
  account (`money.ErrMissingCurrency`/`ErrInvalidCurrency`), never BRL.
  Timeline uses the same native `AccountBalance` ownership before applying
  reporting conversion. See the balance note in
  `.specs/motores-de-dominio.md` section 1 for
  why an ignored transaction stays inside the balance.

Database reads (including accounts, cards and investment presentation) use
`internal/db/types` scan adapters for dates, timestamps, IDs, booleans and
decimals. The shared boundary is documented in [the doc map](../../README.md);
ledger rules and persisted formats are unchanged. Account cycle and bill dates use
calendar-date adapters, including SQLite date-only text and legacy timestamps.

## Reporting conversion

`internal/reporting` consolidates eligible effective account-currency money to
BRL using `internal/exchange` historical rates for the reporting calendar date.
Queries, groups and category reports return conversion coverage and native
per-currency totals alongside consolidated totals. Missing rates expose
incomplete/unavailable states with no deceptively complete subtotal. Transfers
and existing inclusion/status rules retain their precedence; original and
effective transaction money remain available.

Transaction summaries and Home period/category cards show BRL reporting values,
with expandable native currency totals for mixed periods. BRL-only periods retain
the existing controls. Shared `Money` and decimal-string formatting accept an
explicit ISO currency code. Transaction rows/details use effective currency
with the same flow sign/tone across BRL/USD/EUR.

## Status, periods and settlement

`money.Eligibility` accepts `PENDING` and `POSTED` identically. Valid Pluggy/
Nubank purchases in an open card bill can be pending and already count as
expenses. Missing/other statuses are excluded; so are ignored, unclassified,
missing-money and zero entries. A transfer category excludes income/expense
only after those checks. Status alone never implies inclusion or settlement.

| Consumer | Current treatment |
| --- | --- |
| `transactions.Query`, `SpendingByCategory`, `CategoryBreakdown` | Eligible pending and posted purchases contribute once per stored row; category spending is BRL-only. Explicit status filters can narrow the report. |
| Report period (`effectiveDate`) | Default `occurred_at`: each installment uses its own provider date. `paid_at`: cards use a matching bill's due date, otherwise `occurred_at + 1 month`. Bank entries keep their transaction date. Grouping and ordering still use `occurred_at`. |
| `CardDueDates.ProjectedEntryDate` (Timeline/projections) | Card cash is projected on the associated bill due date, then a forecast month or inferred cycle. `billForecastDate` informs this projection, not report period filtering. Payment legs use the bill they settle. |
| `CreditCardTransactionTotal(At)` | Pending/posted entries count only in the current open cycle; ignored entries do not. Category transfer exclusion is bypassed so a payment assigned to that cycle still reduces debt. Forecast month alone does not establish a debt cycle. |
| `CashOnHandByCurrency` / net-worth history | Synced bank cash uses provider balances; card balances are not cash. Backfill uses eligible synced non-credit entries, including pending, with inclusion/category exclusions bypassed. Manual bank balance/history uses posted movements only. |

Installments are existing provider rows, not copies generated by the money
engine. Tests use three synthetic pending installments with distinct provider
dates, known bill associations and one future forecast bill. They verify each
period's transaction and category totals and the separate projected due dates
(`TestPendingCardInstallmentsCountOnceInTheirPeriods`). If the provider dates
do not distinguish installments and no bill is known, report filtering retains
the fallback above; it does not infer report months from installment numbers
or `billForecastDate`.

Paying a bill is separate from recognizing its purchases. The bank debit and
card credit, categorized as `Pagamento de Fatura de Cartão` (transfer), add no
income/expense; the bank balance changes when the provider reports it, and an
early payment associated with the open bill reduces its calculated debt
(`TestPendingCardPurchaseAndBillSettlementHaveSeparateEffects`).
`TotalsEligibility.MovesCash` is a cash-flow selection signal, not proof of
settlement or an instruction to subtract every card purchase from today's
bank balance.

A resync from `PENDING` to `POSTED` retains the row's identity and user-owned
decisions/links. The same amount still contributes once; a correction replaces
the previous amount, and replay adds no expense
(`TestSyncPendingToPostedKeepsIdentityAndUserDecisions`).

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
- Manual bank accounts: `POST /api/accounts`, `PUT /api/accounts/{id}`.
  Body: `name`, `currency_code` (default BRL), `opening_balance` (decimal
  string, default zero), `archived` (default false). PUT replaces these fields
  and only accepts manual accounts (409 for synced). GET list/detail includes
  `origin`, nullable `external_id`, `opening_balance`, `archived_at`, and
  `has_movements`. `GET /api/accounts/currencies` lists ISO currency codes
  from the existing `golang.org/x/text/currency` catalog.
- `POST /api/accounts/{id}/adjustments`: signed decimal-string `amount` plus
  `description`. Only active manual accounts accept adjustments; they are
  balance deltas, separate from income/expense transactions. The adjustment
  and account timestamp are written atomically. Its response is the account.
  Currency and opening balance are locked once transactions (even deleted)
  or adjustments exist (409); account writes and transaction creation share
  a row lock to serialize this check. Archive/reactivate via PUT `archived`.
  Archived accounts retain balances/history and reject creation/update of
  transactions targeting them (409), and new adjustments (409).
- Investments: `GET /api/investments[/{id}][/transactions]`.

## Frontend

- `/transacoes` — main ledger: filters, grouping, totals, category/inclusion
  edit drawer, "Novo lançamento manual" button and — for a manual entry — a
  "Manual" tag in the list and drawer, with Editar/Excluir actions.
  `ManualTransactionForm` serves create and edit.
- `/contas-e-cartoes` + `/contas-e-cartoes/:id` — native balance rows,
  "Nova conta" drawer (name, currency, opening balance), and manual-only edit,
  signed balance-adjustment, archive/reactivate actions on detail. The detail
  displays opening balance and locks its currency/initial fields after activity.
  Drawers fill compact screens. Manual transaction amount labels follow the
  selected account currency; archived accounts leave the account selector.
  Optional converted equivalents are omitted because there is no rate source.
  BRL summaries explicitly exclude foreign accounts.
- `/investimentos` + `/investimentos/:id`.

## Notes

Reconciled investments add `investment_transfer_amount` and `reportable_amount`
to the entry. The contribution/withdrawal portion does not count in
income/expense totals, but the full amount remains a cash movement. The reason
for full exclusion is `investment_transfer`. See
[`investimentos/reference.md`](../investimentos/reference.md).

Ingestion: Pluggy, manual entry and statement import are the entry paths today,
all data providers, not part of this engine — see Open Banking sync and
`.specs/motores-de-dominio.md` section 1. Standalone manual bank accounts use the same `financial_accounts` table,
with `origin=manual` and NULL source/external/raw/hash identity. Synced and
file-imported accounts keep `origin=synced` and their existing identity
constraints. Manual investment custody accounts remain a separate model.

User decisions live in their own tables, which ingestion never rewrites: manual
category and inclusion (`transaction_category_*`, `transaction_inclusion_*`),
`payable_transaction_links` and `scenario_realizations`. A provider update
(PENDING → POSTED, corrections) keeps the entry's `id`. Full policy in
[`sincronizacao-open-banking/reference.md`](../sincronizacao-open-banking/reference.md).

For a manual bank account, native balance is **opening balance + sum of
signed account-currency amounts of POSTED, undeleted transactions + sum of
explicit adjustment deltas**. Inclusion/category changes do not change this
balance; editing, moving or soft-deleting a transaction recomputes it on read.
There is no persisted second balance (`balance` must be NULL on manual
accounts). Adjustments do not enter income/expense totals. Archived accounts
keep their money in the balance. Creating USD 10 on a USD account stores USD
10 in both transaction amount fields; no FX conversion occurs.

For synced accounts, manual entries still never change provider balances or
historical reconstruction. Credit-card totals remain the deliberate exception:
they sum eligible entries, so manual entries count toward the open bill.
`CashOnHandIn(..., BRL)` (Net Worth) remains a BRL aggregate and excludes foreign
accounts; Timeline converts native balances separately for its reference-date anchor. Net-worth history reverses posted manual-account movements and
adjustment deltas, using opening balance as its historical baseline; existing
snapshots are never rewritten.

## Database representation

Money uses exact Postgres `NUMERIC` or SQLite decimal text; transaction times
remain instants. Account cycle dates and bill due/closing dates are UTC calendar
days, stored as Postgres `DATE` or SQLite date text. UUID identities use native
Postgres UUIDs. Both dialects start at the same `0001_initial.sql` baseline;
legacy databases are rejected before migrations and require a separate converter.

Transaction queries scan nullable money and instants directly into decimal and
time values through `internal/db/types`, preserving SQL NULL without intermediate
text conversions. Shared test fixtures bind binary payloads and use portable
boolean literals so query coverage runs against both typed baselines.
`TestQueryTypedValuesBothDialects` pins exact large amounts, UTC instants,
decision timestamps and absent values on SQLite and Postgres.

## Monetary precision

HTTP amounts retain fractional minor units and large values as decimal strings.
Manual edits preserve their precision; display rounding follows the shared
[monetary policy](../../precisao-monetaria.md).

## Transfers between owner accounts

`transactions/transfers.go` owns lightweight annotations in `transfer_links`
and `transfer_transaction_links` (native migration `0002` in both dialects).
Each relationship contains one source and one destination, plus an optional
separate fee movement. A transaction belongs to at most one relationship in
any role. All financial accounts belong to the installation's single owner;
there is no external-account or multi-owner pairing path.

`ledger.Service.LinkTransfer` pairs existing synced or manual movements without
changing provider rows or category/inclusion decisions. `CreateManualTransfer`
creates both movements, optionally a fee, applies normal learned categories and
automation, and links everything in one database transaction. Any validation or
write failure rolls back all new rows. New pairs require active manual accounts;
existing pairs may use synced accounts. Credit-card accounts are rejected.

Both legs require a known native account-currency amount, a known account
currency, opposite signed DEBIT/CREDIT directions, distinct accounts, considered
inclusion and POSTED/PENDING status. Same-currency principal must have equal
absolute amounts. Unknown/deleted movements, duplicate links, and existing
investment reconciliations are rejected. The reverse order is rejected too:
investment reconciliation cannot reuse any transfer participant, including its
fee. Pairing locks participant bank rows in identifier order before validation
to serialize concurrent investment reconciliation and participant edits,
deletions or inclusion changes. A fee must be a separate included outflow on either participating account; it cannot be an ignored or
transfer-categorized movement. Pairing never infers a fee from market rates.

Source/destination amounts stay in `financial_transactions`, in their account
currencies. Detail reads these current amounts and computes **absolute source
amount / absolute destination amount**, rounded to 16 decimal places: BRL 540
sent and USD 100 received display `1 USD = 5.4 BRL`. This execution rate is not
a market quote and is not stored as a derived value. Provider corrections are
reflected on read; an unavailable principal amount or zero destination amount
makes the rate unavailable. Missing native amounts are returned as JSON null
and displayed as **Valor indisponível**, retaining detail and unlink access.
The annotation remains until explicitly unlinked or removed by ignoring/deleting
a participating movement.

Linked principal receives `linked_transfer` after ordinary eligibility checks,
leaves income/expense/category totals and still satisfies `MovesCash()`.
Native account balance and cash reconstruction retain the original movements.
Fee reporting uses its ordinary transaction exactly once; later user decisions
continue to govern that fee. No second cost entry or inferred spread exists.

Manual edits of a participating movement return 409 until unlinking. Soft
deletion or ignoring any participant removes the whole relationship in the
same transaction; other movements survive and resume their current category
and inclusion decisions. Explicit unlink does the same without deleting any
movement or changing a decision. An existing transfer category therefore
continues to exclude that movement after unlinking. Relinking requires an
explicit new pairing and the same validation.

### HTTP and UI

- `POST /api/transfers`: existing `source_transaction_id`,
  `destination_transaction_id`, optional `fee_transaction_id`; or `source`,
  `destination`, optional `fee` manual-entry bodies (signed decimal-string
  amounts and calendar dates; categories omitted/null). Do not combine modes.
  Returns the transfer detail with 201. Writes use ordinary auth/CSRF protection.
- `GET /api/transactions/{id}/transfer`: detail for any participant, or JSON
  null when it has no relationship. Source/destination include transaction ID,
  account ID/name, nullable signed native amount and currency; `effective_rate`
  includes the source units per destination unit; `fee` is nullable.
- `DELETE /api/transfers/{id}`: unlink, 204; missing link 404. Duplicate or
  investment-linked participants are 409; invalid pairing is 422; database
  failures are 503 without internals.
- `/transacoes`: **Nova transferência** opens a manual-account form with native
  amount inputs, exact decimal-string arithmetic, execution-rate preview and
  optional separate fee in the source currency. The existing transaction panel
  adds **Transferência entre contas** for pairing, reading both native amounts,
  rate and fee, and confirming unlink. Existing relationships remain accessible
  even when a provider correction leaves a movement unclassified. Counterpart
  and fee pickers support search/pagination and native currency labels. Principal inclusion is disabled
  while linked, with an explanation; errors retain selections and offer retries.
  Inclusion changes and retroactive automation refresh transfer details for all
  participants, so ignoring a fee also clears the counterpart relationship in
  open panels. Inactive detail queries become stale for the next visit.

Tests: `TestLinkedTransfersPreserveNativeCashAndReporting`,
`TestSyncedTransferLinkingAndUnlinkDecisions`,
`TestTransferProviderCorrectionsKeepDetailAndUnlinkAvailable`,
`TestTransferValidationAndAtomicManualPairs`,
`TestTransferRejectsInvalidFeesAndKeepsPreviousTransferCategory`,
`TestTransferHTTPCreateReadUnlinkAndValidation`,
`TestTransferMigrationPreservesExistingMovements`; frontend
`TransactionTransferScreen.test.tsx`, `ManualTransferForm.test.tsx`,
`transferRate.test.ts` and `transfers.test.ts`.
