# Schema rework: legacy inventory and conversion contract

> Design input for the schema rework; no target DDL or migration tool is
> implemented by this document. Baseline: `3d78f17f0cd418b037a71770efeea57424166ee3`,
> SQLite through `00047_manual_bank_accounts`, Postgres through
> `00046_manual_bank_accounts`. Re-inventory if that baseline changes.

## Scope decisions

1. **Carry over all surviving application data.** Preserve manual, CSV and
   provider entries, balances, raw imports, sync audit history, user decisions,
   settings, payables and links, scenarios and realizations, investment ledgers,
   annotations, quotes, FX caches, net worth snapshots and idempotency records.
   A provider re-sync cannot recover historical raw payloads or reliably restore
   links to user decisions. Keep IDs, soft deletes, revisions and hashes exactly;
   do not regenerate identities or replay automation. Move encrypted settings
   byte-for-byte with the same external master key; do not decrypt or re-encrypt.
   No balances/statuses derived on read become new persisted columns.
2. **Keep the current owner/auth model.** It already uses the fixed owner
   `auth_owner.id = 1`; retain email, password hash, revision, session token
   hashes and expiry semantics. No new multi-user model or ownership columns.
   Preserve sessions without extending their expiry. Keep `app_auth` for the
   existing legacy password-to-master-key migration path; normally it is empty
   after `julius auth migrate`. Do not repopulate it or treat it as current auth.
3. **Keep every surviving application table listed below.** Each has a runtime
   reader/writer or supports the legacy auth migration. Goose's version table
   belongs to the target migration runner: initialize it for the new sequence,
   never copy source migration versions. SQLite internal tables and Postgres
   sequences are engine metadata, not application data.

These are the proposed decisions for this work, based on preserving the current
single-owner behavior. Owner review can revise them before target DDL/tool work;
this document does not authorize deleting data from a running database.

## Evidence and reproduction

The schema snapshots contain no rows, credentials or production data:

- [`legado-sqlite.sql`](legado-sqlite.sql): all tables, indexes and triggers from
  `sqlite_schema` of a fresh `internal/db.Open(":memory:")` database, ordered by
  tables, indexes, then triggers (alphabetically within each type). Only objects
  with non-NULL SQL and names not beginning `sqlite_` are included.
- [`legado-postgres.sql`](legado-postgres.sql): `pg_dump --schema-only
  --no-owner --no-privileges` after `internal/db.Open` against a fresh disposable
  Postgres 17 database. Generated dump restriction tokens are omitted.

To repeat, use a temporary Go command inside this module (so it can import
`internal/db`) to open those synthetic databases through `db.Open`, then query
`PRAGMA table_info` on SQLite and `information_schema.columns` in Postgres's
`public` schema. Do not use a running installation: opening applies migrations.
Compare `(table_name, column_name)` sets, not migration file numbers or physical
column order. Constraints, defaults, FKs and append-only protections are in the
snapshots; the inventory below records every column's physical type and meaning.

The three obsolete tables are **already absent** in both final schemas:

| Table | SQLite drop | Postgres drop | Replacement |
|---|---|---|---|
| `scenario_transaction_realizations` | 00028 | 00027 | `scenario_realizations` |
| `recurrence_reconciliations` | 00029 | 00028 | `scenario_realizations` |
| `recurring_commitments` | 00030 | 00029 | `scenarios` + `scenario_recurring_schedules` |

`recurring_commitment_conditions`, `recurring_commitment_scenario_map`, debts,
receivables and their old link tables are also absent. No new DROP migration is
needed. The future legacy tool accepts a fully migrated source at this baseline;
older sources must first run their existing migrations, which move legacy data
into the surviving tables. Do not inventory intermediate rebuild tables or
resurrect old names from migration history.

## Target column contract

The implemented baseline follows issue #31's native Postgres types, superseding
the original text-only proposal below: exact numbers use `NUMERIC`, instants
`TIMESTAMPTZ`, calendar dates `DATE`, flags `BOOLEAN` and UUID identities `UUID`.
Opaque IDs (including integrated custody IDs), keys and provider fields remain
text. SQLite keeps exact decimal/UTC text and integer flags. See
[`migrations/README.md`](../../internal/db/migrations/README.md). The snapshots
below remain the unchanged legacy source for a future conversion tool.

### Original conversion proposal (superseded by the implemented baseline)

Keep table/column names and the existing constraints, defaults, relationships,
indexes and append-only behavior unless target DDL work explicitly changes them.
Every inventory row maps to the same `table.column`. Columns absent from the
conversion map copy unchanged; a semantic classification alone is not a type
change. This minimal contract deliberately retains exact decimal text and opaque
IDs rather than imposing UUIDs, a precision limit or new dialect dependencies.

| Classification | Target representation in SQLite / Postgres |
|---|---|
| money | TEXT / text, exact decimal in its existing currency; no float, FX, scaling or rounding |
| decimal (non-money) | TEXT / text, exact quantity, percentage or exchange rate; never treat as cents |
| instant | TEXT / text, UTC `db.TimeLayout` (`2006-01-02T15:04:05.000000000Z`) |
| calendar date | TEXT / text, validated `YYYY-MM-DD`; no host timezone conversion |
| ID | Existing INTEGER or TEXT / integer or text; includes provider IDs, symbols, composite keys and hashes used as keys |
| boolean | Existing INTEGER 0/1 / boolean (or existing integer where the legacy schema uses it); preserve its CHECK |
| enum | TEXT / text; preserve CHECK values; provider vocabularies remain open where unconstrained |
| free text | TEXT / text; includes JSON strings, hashes that are not keys, and encoded encrypted values |
| integer | Existing INTEGER / integer or bigint, retaining bounds and meaning (counts, revisions, positions, day/month selectors) |
| bytes | BLOB / bytea; preserve raw payload bytes and their hashes |

The issue's seven categories need three additional categories for an honest
inventory: quantities/rates are not money, counters are not IDs, and arbitrary
payload bytes are not text. Currency codes are open enum values, never a reason
to convert amounts to BRL. JSON-bearing text (`payment_data`,
`credit_card_metadata`, `merchant`, `response_headers`) stays opaque; settings
values and rule-condition values are key/field-dependent text, not globally
money/boolean/date columns. Password hashes, salts and verifiers also stay opaque.

Calendar dates that already use `YYYY-MM-DD` copy unchanged, including
`net_worth_snapshots.captured_at` and `scenario_transactions.projected_at` despite
their `_at` names. `financial_accounts.balance_as_of` is an instant from the
latest statement movement. `financial_investments.as_of_date` is an instant:
quotes derive a Brazilian day from it, but it also carries the provider's
observation time. Transaction `occurred_at` values retain their instants even
when a manual input originally selected a calendar day.

## Changed-column mapping and conversion map

This is the complete `table.column → converter` allowlist for the future legacy
migration tool, for either dialect. There are no renames or table merges in this
contract. Target column equals source column; unchanged columns are omitted.
All converters preserve SQL NULL where allowed and reject invalid non-NULL
values with row identity and column context; never substitute zero, now, an empty
string or a guessed date. Conversion failures abort the destination import.

| Converter | Input → output | Synthetic example |
|---|---|---|
| `unix_seconds_to_instant` | Integer Unix seconds → UTC fixed-width `db.TimeLayout`; preserve the exact second, no milliseconds guess | `0` → `1970-01-01T00:00:00.000000000Z` |
| `utc_calendar_day` | Canonical legacy timestamp parsed with `db.ParseTime` → its UTC year/month/day via `dates.Day`, formatted `YYYY-MM-DD` | `2026-10-10T00:00:00.000000000Z` → `2026-10-10` |
| `provider_calendar_day` | Canonical legacy timestamp → existing `investments.ProviderDay` (fixed BRT, UTC−03:00), formatted `YYYY-MM-DD`; never use the machine's timezone | `2026-10-10T02:00:00.000000000Z` → `2026-10-09` |
| `provider_movement_day` | Canonical legacy timestamp → existing `investments.movementDay`: exact UTC midnight preserves its UTC calendar day; every other instant uses `investments.ProviderDay`, formatted `YYYY-MM-DD` | `2026-09-13T00:00:00.000000000Z` → `2026-09-13`; `2026-09-13T02:00:00.000000000Z` → `2026-09-12` |

The UTC converter matches the bill calendar convention in
`transactions/cardflow.go` and `transactions/cardtotal.go`. Investment dates use
the provider convention in `investments/quotes.go` for due, issue and purchase
dates. Trade dates use [`movementDay` in `investments/yield.go`](../../internal/investments/yield.go)
to preserve provider date-only movements stored at UTC midnight. Removing the
timestamp portion of a calendar field is intentional;
raw payloads retain provider evidence. The future readers/writers must switch
these fields together with the migration; copying date-only values into today's
schema would break its timestamp parsers. Auth readers/writers likewise need to
switch from Unix seconds while preserving lifetime/revision checks.

| Source → target (same name) | Legacy type (both dialects) | Target (both dialects) | Converter |
|---|---|---|---|
| `auth_owner.created_at` | BIGINT | TEXT instant | `unix_seconds_to_instant` |
| `auth_sessions.created_at` | BIGINT | TEXT instant | `unix_seconds_to_instant` |
| `auth_sessions.last_seen_at` | BIGINT | TEXT instant | `unix_seconds_to_instant` |
| `auth_sessions.expires_at` | BIGINT | TEXT instant | `unix_seconds_to_instant` |
| `financial_accounts.balance_close_date` | TEXT | TEXT calendar date | `utc_calendar_day` |
| `financial_accounts.balance_due_date` | TEXT | TEXT calendar date | `utc_calendar_day` |
| `financial_bills.due_date` | TEXT | TEXT calendar date | `utc_calendar_day` |
| `financial_bills.closing_date` | TEXT | TEXT calendar date | `utc_calendar_day` |
| `financial_investments.due_date` | TEXT | TEXT calendar date | `provider_calendar_day` |
| `financial_investments.issue_date` | TEXT | TEXT calendar date | `provider_calendar_day` |
| `financial_investments.purchase_date` | TEXT | TEXT calendar date | `provider_calendar_day` |
| `financial_investment_transactions.trade_date` | TEXT | TEXT calendar date | `provider_movement_day` |

The target runner creates its own `goose_db_version`; it is excluded from the
converter allowlist. Rebuild engine-managed sequences from imported rows where
needed rather than copying sequence state. Load dependencies before dependents,
preserve explicit link tables, and validate FKs, row counts, money values,
append-only events and soft deletes before switching databases. Target DDL and
the tool's transactional/cutover implementation are separate work.

## Tables still in use

All rows below are retained. Paths identify runtime SQL usage, not merely old
migrations or tests.

| Table | Runtime evidence | Purpose |
|---|---|---|
| `account_balance_adjustments` | [`transactions/account_balance.go`](../../internal/transactions/account_balance.go) | Manual account adjustment deltas |
| `app_auth` | [`settings/migrate.go`](../../internal/settings/migrate.go) | Legacy auth migration only; keep compatibility |
| `auth_owner` | [`auth/store.go`](../../internal/auth/store.go) | Fixed single owner account |
| `auth_sessions` | [`auth/store.go`](../../internal/auth/store.go) | Session hashes, revisions and expiry |
| `automation_rule_actions` | [`automation/store.go`](../../internal/automation/store.go) | Ordered rule actions |
| `automation_rule_conditions` | [`automation/store.go`](../../internal/automation/store.go) | Rule matching predicates |
| `automation_rules` | [`automation/store.go`](../../internal/automation/store.go) | User automation rules |
| `categories` | [`categories/categories.go`](../../internal/categories/categories.go) | Category definitions |
| `data_sources` | [`syncsvc/service.go`](../../internal/syncsvc/service.go) | Provider/CSV connections |
| `exchange_rates` | [`exchange/store.go`](../../internal/exchange/store.go) | Dated FX cache |
| `financial_accounts` | [`syncsvc/service.go`](../../internal/syncsvc/service.go) | Provider and manual accounts |
| `financial_bills` | [`transactions/cardflow.go`](../../internal/transactions/cardflow.go) | Synced card bills and cycle dates |
| `financial_investment_transactions` | [`syncsvc/service.go`](../../internal/syncsvc/service.go) | Provider investment movements |
| `financial_investments` | [`syncsvc/service.go`](../../internal/syncsvc/service.go) | Provider holding snapshots |
| `financial_transactions` | [`transactions/query.go`](../../internal/transactions/query.go) | Provider, CSV and manual ledger entries |
| `investment_accounts` | [`investments/storage.go`](../../internal/investments/storage.go) | Local custody accounts |
| `investment_asset_quote_coverage` | [`investments/quote_coverage.go`](../../internal/investments/quote_coverage.go) | Quote backfill coverage |
| `investment_asset_quotes` | [`investments/quotes.go`](../../internal/investments/quotes.go) | Dated unit prices |
| `investment_assets` | [`investments/assets.go`](../../internal/investments/assets.go) | Local asset identity and classification |
| `investment_operations` | [`investments/operations.go`](../../internal/investments/operations.go) | Manual investment ledger |
| `investment_portfolios` | [`investments/portfolios.go`](../../internal/investments/portfolios.go) | User goals and portfolios |
| `investment_position_portfolios` | [`investments/positions.go`](../../internal/investments/positions.go) | Portfolio annotations on holdings |
| `investment_positions` | [`investments/positions.go`](../../internal/investments/positions.go) | Local holding identities |
| `investment_reconciliations` | [`investments/reconciliations.go`](../../internal/investments/reconciliations.go) | Links to real financial movements |
| `manual_transaction_idempotency` | [`ledger/manual.go`](../../internal/ledger/manual.go) | Retry-safe manual creation |
| `net_worth_snapshots` | [`networth/store.go`](../../internal/networth/store.go) | Historical daily snapshots |
| `normalization_events` | [`syncsvc/service.go`](../../internal/syncsvc/service.go) | Append-only ingestion audit |
| `payable_transaction_links` | [`payables/store.go`](../../internal/payables/store.go) | Obligation settlement links |
| `payables` | [`payables/store.go`](../../internal/payables/store.go) | Debts and receivables |
| `raw_imports` | [`syncsvc/rawimports.go`](../../internal/syncsvc/rawimports.go) | Append-only raw provider/CSV evidence |
| `scenario_realizations` | [`scenarios/realizations.go`](../../internal/scenarios/realizations.go) | Realization/reconciliation links |
| `scenario_recurring_schedules` | [`recurrences/store.go`](../../internal/recurrences/store.go) | Recurring projection schedules |
| `scenario_transactions` | [`scenarios/store.go`](../../internal/scenarios/store.go) | Planned installments/movements |
| `scenarios` | [`scenarios/store.go`](../../internal/scenarios/store.go) | Opt-in scenarios and accounting plans |
| `settings` | [`settings/store.go`](../../internal/settings/store.go) | Preferences and encrypted credentials |
| `statement_imports` | [`httpapi/statement_imports_handlers.go`](../../internal/httpapi/statement_imports_handlers.go) | CSV import summaries |
| `sync_failures` | [`syncsvc/service.go`](../../internal/syncsvc/service.go) | Safe ingestion failure audit |
| `sync_runs` | [`syncsvc/recovery.go`](../../internal/syncsvc/recovery.go) | Sync/import status and worker leases |
| `transaction_category_decisions` | [`categories/decisions.go`](../../internal/categories/decisions.go) | Current manual/rule/learned decisions |
| `transaction_category_events` | [`categories/decisions.go`](../../internal/categories/decisions.go) | Append-only category decision history |
| `transaction_inclusion_decisions` | [`transactions/inclusion.go`](../../internal/transactions/inclusion.go) | Current totals-inclusion decisions |
| `transaction_inclusion_events` | [`transactions/inclusion.go`](../../internal/transactions/inclusion.go) | Append-only inclusion history |

`goose_db_version` is used by Goose, not a domain module. Its four columns are
classified below for completeness but its rows do not carry over.

## Engineering review record

Reviewed against the baseline above on 2026-10-12 using only synthetic databases:

- SQLite and Postgres expose the same 430 `(table, column)` pairs. Every pair
  appears exactly once below with its observed type, declared nullability and
  classification; 426 belong to the application and four to Goose.
- Both committed schema dumps restore successfully into empty databases and
  reproduce those column sets. The three requested obsolete tables are absent.
- All 42 retained application tables have runtime evidence in the linked files.
  All 12 changed-column entries refer to existing columns; money, IDs, bytes and
  unchanged calendar dates have no converters.
- Reviewed date semantics against `db/time.go`, `auth/store.go`,
  `transactions/cardflow.go`, `transactions/cardtotal.go`,
  `investments/quotes.go`, `investments/yield.go`, `networth/store.go`,
  `scenarios/store.go` and
  `httpapi/statement_imports_handlers.go`, rather than classifying by suffix alone.
- Go build, vet and tests (including disposable Postgres), frontend lint,
  typecheck, tests and build, embedded frontend build, release-script tests,
  govulncheck and the production npm audit passed.

This is an engineering consistency review. Owner acceptance of the proposed
scope and date conversion choices remains part of reviewing this design before
implementing the target schema and migration tool.

## Full column inventory

42 application tables, 426 application columns; 136 ID + 8 boolean + 1 bytes + 20 calendar date + 8 decimal (non-money) + 59 enum + 55 free text + 73 instant + 25 integer + 41 money.
Plus one Goose table with four bookkeeping columns. The physical types below
are from the migrated databases, not inferred from filenames. `NN` means a
NOT NULL declaration (primary-key constraints are also in the snapshots).
`NULL` means no explicit NOT NULL declaration; SQLite TEXT primary keys should
not be assumed to enforce NOT NULL just because Postgres does.

### `account_balance_adjustments`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `id` | TEXT / NULL | text / NN | ID |
| `account_id` | TEXT / NN | text / NN | ID |
| `amount` | TEXT / NN | text / NN | money |
| `description` | TEXT / NN | text / NN | free text |
| `created_at` | TEXT / NN | text / NN | instant |

### `app_auth`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `id` | INTEGER / NULL | integer / NN | ID |
| `salt` | TEXT / NN | text / NN | free text |
| `verifier` | TEXT / NN | text / NN | free text |
| `created_at` | TEXT / NN | text / NN | instant |
| `updated_at` | TEXT / NN | text / NN | instant |

### `auth_owner`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `id` | INTEGER / NULL | integer / NN | ID |
| `email` | TEXT / NN | text / NN | free text |
| `password_hash` | TEXT / NN | text / NN | free text |
| `revision` | INTEGER / NN | integer / NN | integer |
| `created_at` | BIGINT / NN | bigint / NN | instant |

### `auth_sessions`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `token_hash` | TEXT / NULL | text / NN | ID |
| `owner_id` | INTEGER / NN | integer / NN | ID |
| `revision` | INTEGER / NN | integer / NN | integer |
| `created_at` | BIGINT / NN | bigint / NN | instant |
| `last_seen_at` | BIGINT / NN | bigint / NN | instant |
| `expires_at` | BIGINT / NN | bigint / NN | instant |

### `automation_rule_actions`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `id` | TEXT / NULL | text / NN | ID |
| `rule_id` | TEXT / NN | text / NN | ID |
| `action_type` | TEXT / NN | text / NN | enum |
| `scenario_id` | TEXT / NULL | text / NULL | ID |
| `category_id` | TEXT / NULL | text / NULL | ID |
| `position` | INTEGER / NN | integer / NN | integer |

### `automation_rule_conditions`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `id` | TEXT / NULL | text / NN | ID |
| `rule_id` | TEXT / NN | text / NN | ID |
| `field` | TEXT / NN | text / NN | enum |
| `operator` | TEXT / NN | text / NN | enum |
| `value` | TEXT / NN | text / NN | free text |
| `position` | INTEGER / NN | integer / NN | integer |

### `automation_rules`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `id` | TEXT / NULL | text / NN | ID |
| `name` | TEXT / NN | text / NN | free text |
| `is_active` | INTEGER / NN | integer / NN | boolean |
| `logic_operator` | TEXT / NN | text / NN | enum |
| `created_at` | TEXT / NN | text / NN | instant |
| `updated_at` | TEXT / NN | text / NN | instant |

### `categories`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `id` | TEXT / NULL | text / NN | ID |
| `name` | TEXT / NN | text / NN | free text |
| `kind` | TEXT / NN | text / NN | enum |
| `is_active` | INTEGER / NN | integer / NN | boolean |
| `created_at` | TEXT / NN | text / NN | instant |
| `updated_at` | TEXT / NN | text / NN | instant |
| `icon` | TEXT / NN | text / NN | free text |
| `color` | TEXT / NN | text / NN | free text |

### `data_sources`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `id` | TEXT / NULL | text / NN | ID |
| `provider` | TEXT / NN | text / NN | enum |
| `external_item_id` | TEXT / NN | text / NN | ID |
| `display_name` | TEXT / NULL | text / NULL | free text |
| `created_at` | TEXT / NN | text / NN | instant |
| `updated_at` | TEXT / NN | text / NN | instant |
| `is_active` | INTEGER / NN | integer / NN | boolean |
| `label` | TEXT / NULL | text / NULL | free text |

### `exchange_rates`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `base_currency` | TEXT / NN | text / NN | enum |
| `quote_currency` | TEXT / NN | text / NN | enum |
| `as_of` | TEXT / NN | text / NN | calendar date |
| `effective_on` | TEXT / NN | text / NN | calendar date |
| `rate` | TEXT / NN | text / NN | decimal (non-money) |
| `source` | TEXT / NN | text / NN | enum |
| `fetched_at` | TEXT / NN | text / NN | instant |

### `financial_accounts`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `id` | TEXT / NULL | text / NN | ID |
| `source_id` | TEXT / NULL | text / NULL | ID |
| `external_id` | TEXT / NULL | text / NULL | ID |
| `institution` | TEXT / NULL | text / NULL | free text |
| `name` | TEXT / NULL | text / NULL | free text |
| `number` | TEXT / NULL | text / NULL | ID |
| `account_type` | TEXT / NULL | text / NULL | enum |
| `account_subtype` | TEXT / NULL | text / NULL | enum |
| `balance` | TEXT / NULL | text / NULL | money |
| `credit_limit` | TEXT / NULL | text / NULL | money |
| `currency_code` | TEXT / NULL | text / NULL | enum |
| `provider_updated_at` | TEXT / NULL | text / NULL | instant |
| `current_raw_import_id` | TEXT / NULL | text / NULL | ID |
| `normalized_hash` | TEXT / NULL | text / NULL | free text |
| `created_at` | TEXT / NN | text / NN | instant |
| `updated_at` | TEXT / NN | text / NN | instant |
| `available_credit_limit` | TEXT / NULL | text / NULL | money |
| `balance_close_date` | TEXT / NULL | text / NULL | calendar date |
| `balance_due_date` | TEXT / NULL | text / NULL | calendar date |
| `manual_closing_day` | INTEGER / NULL | integer / NULL | integer |
| `balance_as_of` | TEXT / NULL | text / NULL | instant |
| `origin` | TEXT / NN | text / NN | enum |
| `opening_balance` | TEXT / NULL | text / NULL | money |
| `archived_at` | TEXT / NULL | text / NULL | instant |

### `financial_bills`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `id` | TEXT / NULL | text / NN | ID |
| `source_id` | TEXT / NN | text / NN | ID |
| `account_id` | TEXT / NN | text / NN | ID |
| `external_id` | TEXT / NN | text / NN | ID |
| `due_date` | TEXT / NULL | text / NULL | calendar date |
| `closing_date` | TEXT / NULL | text / NULL | calendar date |
| `total_amount` | TEXT / NULL | text / NULL | money |
| `currency_code` | TEXT / NULL | text / NULL | enum |
| `minimum_payment_amount` | TEXT / NULL | text / NULL | money |
| `current_raw_import_id` | TEXT / NN | text / NN | ID |
| `normalized_hash` | TEXT / NN | text / NN | free text |
| `created_at` | TEXT / NN | text / NN | instant |
| `updated_at` | TEXT / NN | text / NN | instant |

### `financial_investment_transactions`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `id` | TEXT / NULL | text / NN | ID |
| `source_id` | TEXT / NN | text / NN | ID |
| `investment_id` | TEXT / NN | text / NN | ID |
| `external_id` | TEXT / NN | text / NN | ID |
| `movement_type` | TEXT / NULL | text / NULL | enum |
| `quantity` | TEXT / NULL | text / NULL | decimal (non-money) |
| `value` | TEXT / NULL | text / NULL | money |
| `amount` | TEXT / NULL | text / NULL | money |
| `occurred_at` | TEXT / NULL | text / NULL | instant |
| `trade_date` | TEXT / NULL | text / NULL | calendar date |
| `current_raw_import_id` | TEXT / NN | text / NN | ID |
| `normalized_hash` | TEXT / NN | text / NN | free text |
| `created_at` | TEXT / NN | text / NN | instant |
| `updated_at` | TEXT / NN | text / NN | instant |
| `direction` | TEXT / NULL | text / NULL | enum |

### `financial_investments`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `id` | TEXT / NULL | text / NN | ID |
| `source_id` | TEXT / NN | text / NN | ID |
| `external_id` | TEXT / NN | text / NN | ID |
| `investment_type` | TEXT / NULL | text / NULL | enum |
| `subtype` | TEXT / NULL | text / NULL | enum |
| `name` | TEXT / NULL | text / NULL | free text |
| `balance` | TEXT / NULL | text / NULL | money |
| `currency_code` | TEXT / NULL | text / NULL | enum |
| `number` | TEXT / NULL | text / NULL | ID |
| `owner` | TEXT / NULL | text / NULL | free text |
| `tax_number` | TEXT / NULL | text / NULL | ID |
| `due_date` | TEXT / NULL | text / NULL | calendar date |
| `issuer` | TEXT / NULL | text / NULL | free text |
| `issuer_code` | TEXT / NULL | text / NULL | ID |
| `rate` | TEXT / NULL | text / NULL | decimal (non-money) |
| `rate_type` | TEXT / NULL | text / NULL | enum |
| `fixed_annual_rate` | TEXT / NULL | text / NULL | decimal (non-money) |
| `annual_rate` | TEXT / NULL | text / NULL | decimal (non-money) |
| `last_twelve_months_rate` | TEXT / NULL | text / NULL | decimal (non-money) |
| `quantity` | TEXT / NULL | text / NULL | decimal (non-money) |
| `value` | TEXT / NULL | text / NULL | money |
| `amount` | TEXT / NULL | text / NULL | money |
| `amount_profit` | TEXT / NULL | text / NULL | money |
| `amount_withdrawal` | TEXT / NULL | text / NULL | money |
| `as_of_date` | TEXT / NULL | text / NULL | instant |
| `provider_updated_at` | TEXT / NULL | text / NULL | instant |
| `isin` | TEXT / NULL | text / NULL | ID |
| `code` | TEXT / NULL | text / NULL | ID |
| `provider_status` | TEXT / NULL | text / NULL | enum |
| `current_raw_import_id` | TEXT / NN | text / NN | ID |
| `normalized_hash` | TEXT / NN | text / NN | free text |
| `created_at` | TEXT / NN | text / NN | instant |
| `updated_at` | TEXT / NN | text / NN | instant |
| `amount_original` | TEXT / NULL | text / NULL | money |
| `taxes` | TEXT / NULL | text / NULL | money |
| `taxes2` | TEXT / NULL | text / NULL | money |
| `issue_date` | TEXT / NULL | text / NULL | calendar date |
| `purchase_date` | TEXT / NULL | text / NULL | calendar date |
| `issuer_cnpj` | TEXT / NULL | text / NULL | ID |

### `financial_transactions`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `id` | TEXT / NULL | text / NN | ID |
| `source_id` | TEXT / NULL | text / NULL | ID |
| `account_id` | TEXT / NN | text / NN | ID |
| `external_id` | TEXT / NULL | text / NULL | ID |
| `description` | TEXT / NULL | text / NULL | free text |
| `description_raw` | TEXT / NULL | text / NULL | free text |
| `amount` | TEXT / NULL | text / NULL | money |
| `amount_in_account_currency` | TEXT / NULL | text / NULL | money |
| `balance_after` | TEXT / NULL | text / NULL | money |
| `currency_code` | TEXT / NULL | text / NULL | enum |
| `occurred_at` | TEXT / NULL | text / NULL | instant |
| `provider_status` | TEXT / NULL | text / NULL | enum |
| `movement_type` | TEXT / NULL | text / NULL | enum |
| `source_category` | TEXT / NULL | text / NULL | free text |
| `source_category_id` | TEXT / NULL | text / NULL | ID |
| `provider_code` | TEXT / NULL | text / NULL | ID |
| `payment_data` | TEXT / NULL | text / NULL | free text |
| `credit_card_metadata` | TEXT / NULL | text / NULL | free text |
| `merchant` | TEXT / NULL | text / NULL | free text |
| `operation_type` | TEXT / NULL | text / NULL | enum |
| `operation_type_additional_info` | TEXT / NULL | text / NULL | free text |
| `operation_category` | TEXT / NULL | text / NULL | enum |
| `provider_id` | TEXT / NULL | text / NULL | ID |
| `provider_order` | INTEGER / NULL | integer / NULL | integer |
| `provider_created_at` | TEXT / NULL | text / NULL | instant |
| `provider_updated_at` | TEXT / NULL | text / NULL | instant |
| `current_raw_import_id` | TEXT / NULL | text / NULL | ID |
| `normalized_hash` | TEXT / NULL | text / NULL | free text |
| `created_at` | TEXT / NN | text / NN | instant |
| `updated_at` | TEXT / NN | text / NN | instant |
| `origin` | TEXT / NN | text / NN | enum |
| `deleted_at` | TEXT / NULL | text / NULL | instant |

### `goose_db_version`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `id` | INTEGER / NULL | integer / NN | ID |
| `version_id` | INTEGER / NN | bigint / NN | integer |
| `is_applied` | INTEGER / NN | boolean / NN | boolean |
| `tstamp` | TIMESTAMP / NULL | timestamp without time zone / NN | instant |

### `investment_accounts`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `id` | TEXT / NULL | text / NN | ID |
| `name` | TEXT / NN | text / NN | free text |
| `kind` | TEXT / NN | text / NN | enum |
| `currency_code` | TEXT / NULL | text / NULL | enum |
| `source_id` | TEXT / NULL | text / NULL | ID |
| `financial_account_id` | TEXT / NULL | text / NULL | ID |
| `is_active` | INTEGER / NN | boolean / NN | boolean |
| `created_at` | TEXT / NN | text / NN | instant |
| `updated_at` | TEXT / NN | text / NN | instant |

### `investment_asset_quote_coverage`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `asset_id` | TEXT / NULL | text / NN | ID |
| `source` | TEXT / NN | text / NN | enum |
| `symbol` | TEXT / NN | text / NN | ID |
| `covered_from` | TEXT / NN | text / NN | calendar date |
| `covered_to` | TEXT / NN | text / NN | calendar date |
| `updated_at` | TEXT / NN | text / NN | instant |

### `investment_asset_quotes`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `asset_id` | TEXT / NN | text / NN | ID |
| `quoted_on` | TEXT / NN | text / NN | calendar date |
| `price` | TEXT / NN | text / NN | money |
| `source` | TEXT / NN | text / NN | enum |
| `raw_import_id` | TEXT / NULL | text / NULL | ID |
| `created_at` | TEXT / NN | text / NN | instant |
| `updated_at` | TEXT / NN | text / NN | instant |
| `origin` | TEXT / NN | text / NN | enum |

### `investment_assets`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `id` | TEXT / NULL | text / NN | ID |
| `canonical_key` | TEXT / NN | text / NN | ID |
| `name` | TEXT / NN | text / NN | free text |
| `ticker` | TEXT / NULL | text / NULL | ID |
| `asset_type` | TEXT / NN | text / NN | enum |
| `currency_code` | TEXT / NN | text / NN | enum |
| `created_at` | TEXT / NN | text / NN | instant |
| `updated_at` | TEXT / NN | text / NN | instant |
| `quote_source` | TEXT / NULL | text / NULL | enum |
| `quote_symbol` | TEXT / NULL | text / NULL | ID |
| `asset_class` | TEXT / NN | text / NN | enum |

### `investment_operations`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `id` | TEXT / NULL | text / NN | ID |
| `account_id` | TEXT / NN | text / NN | ID |
| `position_id` | TEXT / NULL | text / NULL | ID |
| `kind` | TEXT / NN | text / NN | enum |
| `occurred_on` | TEXT / NN | text / NN | calendar date |
| `amount` | TEXT / NN | text / NN | money |
| `quantity` | TEXT / NULL | text / NULL | decimal (non-money) |
| `unit_price` | TEXT / NULL | text / NULL | money |
| `fees` | TEXT / NN | text / NN | money |
| `taxes` | TEXT / NN | text / NN | money |
| `notes` | TEXT / NULL | text / NULL | free text |
| `source` | TEXT / NN | text / NN | enum |
| `transfer_id` | TEXT / NULL | text / NULL | ID |
| `created_at` | TEXT / NN | text / NN | instant |
| `updated_at` | TEXT / NN | text / NN | instant |

### `investment_portfolios`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `id` | TEXT / NULL | text / NN | ID |
| `name` | TEXT / NN | text / NN | free text |
| `target_amount` | TEXT / NULL | text / NULL | money |
| `target_date` | TEXT / NULL | text / NULL | calendar date |
| `notes` | TEXT / NULL | text / NULL | free text |
| `created_at` | TEXT / NN | text / NN | instant |
| `updated_at` | TEXT / NN | text / NN | instant |

### `investment_position_portfolios`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `manual_position_id` | TEXT / NULL | text / NULL | ID |
| `financial_investment_id` | TEXT / NULL | text / NULL | ID |
| `portfolio_id` | TEXT / NN | text / NN | ID |
| `created_at` | TEXT / NN | text / NN | instant |
| `updated_at` | TEXT / NN | text / NN | instant |

### `investment_positions`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `id` | TEXT / NULL | text / NN | ID |
| `account_id` | TEXT / NN | text / NN | ID |
| `asset_id` | TEXT / NN | text / NN | ID |
| `notes` | TEXT / NULL | text / NULL | free text |
| `created_at` | TEXT / NN | text / NN | instant |
| `updated_at` | TEXT / NN | text / NN | instant |

### `investment_reconciliations`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `id` | TEXT / NULL | text / NN | ID |
| `operation_id` | TEXT / NN | text / NN | ID |
| `financial_transaction_id` | TEXT / NULL | text / NULL | ID |
| `financial_investment_transaction_id` | TEXT / NULL | text / NULL | ID |
| `amount` | TEXT / NN | text / NN | money |
| `created_at` | TEXT / NN | text / NN | instant |

### `manual_transaction_idempotency`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `owner_id` | INTEGER / NN | integer / NN | ID |
| `idempotency_key` | TEXT / NN | text / NN | ID |
| `request_hash` | TEXT / NN | text / NN | free text |
| `transaction_id` | TEXT / NULL | text / NULL | ID |
| `created_at` | TEXT / NN | text / NN | instant |

### `net_worth_snapshots`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `id` | TEXT / NULL | text / NN | ID |
| `captured_at` | TEXT / NN | text / NN | calendar date |
| `total_assets` | TEXT / NN | text / NN | money |
| `total_liabilities` | TEXT / NN | text / NN | money |
| `net_worth` | TEXT / NN | text / NN | money |
| `cash_balance` | TEXT / NN | text / NN | money |
| `investment_balance` | TEXT / NN | text / NN | money |
| `credit_card_balance` | TEXT / NN | text / NN | money |
| `payables_debt` | TEXT / NN | text / NN | money |
| `created_at` | TEXT / NN | text / NN | instant |
| `updated_at` | TEXT / NN | text / NN | instant |
| `is_backfilled` | INTEGER / NN | integer / NN | boolean |

### `normalization_events`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `id` | TEXT / NULL | text / NN | ID |
| `sync_run_id` | TEXT / NN | text / NN | ID |
| `raw_import_id` | TEXT / NN | text / NN | ID |
| `entity_type` | TEXT / NN | text / NN | enum |
| `account_id` | TEXT / NULL | text / NULL | ID |
| `transaction_id` | TEXT / NULL | text / NULL | ID |
| `investment_id` | TEXT / NULL | text / NULL | ID |
| `investment_transaction_id` | TEXT / NULL | text / NULL | ID |
| `bill_id` | TEXT / NULL | text / NULL | ID |
| `external_id` | TEXT / NULL | text / NULL | ID |
| `outcome` | TEXT / NN | text / NN | enum |
| `normalized_hash` | TEXT / NULL | text / NULL | free text |
| `created_at` | TEXT / NN | text / NN | instant |

### `payable_transaction_links`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `id` | TEXT / NULL | text / NN | ID |
| `payable_id` | TEXT / NN | text / NN | ID |
| `transaction_id` | TEXT / NN | text / NN | ID |
| `linked_amount` | TEXT / NN | text / NN | money |
| `linked_at` | TEXT / NN | text / NN | instant |

### `payables`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `id` | TEXT / NULL | text / NN | ID |
| `kind` | TEXT / NN | text / NN | enum |
| `name` | TEXT / NN | text / NN | free text |
| `total_amount` | TEXT / NN | text / NN | money |
| `starting_settled_amount` | TEXT / NN | text / NN | money |
| `created_at` | TEXT / NN | text / NN | instant |
| `updated_at` | TEXT / NN | text / NN | instant |

### `raw_imports`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `id` | TEXT / NULL | text / NN | ID |
| `sync_run_id` | TEXT / NN | text / NN | ID |
| `source_id` | TEXT / NN | text / NN | ID |
| `scope` | TEXT / NN | text / NN | enum |
| `external_account_id` | TEXT / NULL | text / NULL | ID |
| `page_sequence` | INTEGER / NN | integer / NN | integer |
| `request_attempt` | INTEGER / NN | integer / NN | integer |
| `request_method` | TEXT / NULL | text / NULL | enum |
| `request_path` | TEXT / NULL | text / NULL | free text |
| `http_status` | INTEGER / NULL | integer / NULL | integer |
| `response_headers` | TEXT / NULL | text / NULL | free text |
| `payload` | BLOB / NN | bytea / NN | bytes |
| `payload_sha256` | TEXT / NN | text / NN | free text |
| `received_at` | TEXT / NN | text / NN | instant |

### `scenario_realizations`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `id` | TEXT / NULL | text / NN | ID |
| `scenario_id` | TEXT / NN | text / NN | ID |
| `scenario_transaction_id` | TEXT / NULL | text / NULL | ID |
| `occurrence_date` | TEXT / NULL | text / NULL | calendar date |
| `transaction_id` | TEXT / NULL | text / NULL | ID |
| `relation_type` | TEXT / NN | text / NN | free text |
| `state` | TEXT / NN | text / NN | enum |
| `origin` | TEXT / NN | text / NN | enum |
| `allocated_amount` | TEXT / NULL | text / NULL | money |
| `linked_amount` | TEXT / NULL | text / NULL | money |
| `created_at` | TEXT / NN | text / NN | instant |

### `scenario_recurring_schedules`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `scenario_id` | TEXT / NULL | text / NN | ID |
| `cashflow_kind` | TEXT / NN | text / NN | enum |
| `amount` | TEXT / NN | text / NN | money |
| `category_id` | TEXT / NULL | text / NULL | ID |
| `account_id` | TEXT / NULL | text / NULL | ID |
| `cadence` | TEXT / NN | text / NN | enum |
| `day_of_month` | INTEGER / NN | integer / NN | integer |
| `month_of_year` | INTEGER / NULL | integer / NULL | integer |
| `start_date` | TEXT / NN | text / NN | calendar date |
| `end_date` | TEXT / NULL | text / NULL | calendar date |

### `scenario_transactions`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `id` | TEXT / NULL | text / NN | ID |
| `scenario_id` | TEXT / NN | text / NN | ID |
| `description` | TEXT / NN | text / NN | free text |
| `amount` | TEXT / NN | text / NN | money |
| `projected_at` | TEXT / NN | text / NN | calendar date |
| `category` | TEXT / NULL | text / NULL | free text |
| `created_at` | TEXT / NN | text / NN | instant |
| `updated_at` | TEXT / NN | text / NN | instant |

### `scenarios`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `id` | TEXT / NULL | text / NN | ID |
| `kind` | TEXT / NN | text / NN | enum |
| `name` | TEXT / NN | text / NN | free text |
| `payable_id` | TEXT / NULL | text / NULL | ID |
| `is_active` | INTEGER / NN | integer / NN | boolean |
| `is_accounting_source` | INTEGER / NN | integer / NN | boolean |
| `created_at` | TEXT / NN | text / NN | instant |
| `updated_at` | TEXT / NN | text / NN | instant |

### `settings`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `key` | TEXT / NULL | text / NN | ID |
| `value` | TEXT / NULL | text / NULL | free text |
| `is_encrypted` | INTEGER / NN | integer / NN | boolean |
| `updated_at` | TEXT / NN | text / NN | instant |

### `statement_imports`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `id` | TEXT / NULL | text / NN | ID |
| `sync_run_id` | TEXT / NN | text / NN | ID |
| `source_id` | TEXT / NN | text / NN | ID |
| `account_id` | TEXT / NN | text / NN | ID |
| `format` | TEXT / NN | text / NN | enum |
| `adapter_version` | TEXT / NN | text / NN | free text |
| `filename` | TEXT / NN | text / NN | free text |
| `file_sha256` | TEXT / NN | text / NN | free text |
| `rows_total` | INTEGER / NN | integer / NN | integer |
| `rows_invalid` | INTEGER / NN | integer / NN | integer |
| `rows_duplicate` | INTEGER / NN | integer / NN | integer |
| `created_at` | TEXT / NN | text / NN | instant |

### `sync_failures`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `id` | TEXT / NULL | text / NN | ID |
| `sync_run_id` | TEXT / NN | text / NN | ID |
| `raw_import_id` | TEXT / NULL | text / NULL | ID |
| `account_id` | TEXT / NULL | text / NULL | ID |
| `external_account_id` | TEXT / NULL | text / NULL | ID |
| `external_transaction_id` | TEXT / NULL | text / NULL | ID |
| `investment_id` | TEXT / NULL | text / NULL | ID |
| `external_investment_id` | TEXT / NULL | text / NULL | ID |
| `bill_id` | TEXT / NULL | text / NULL | ID |
| `external_bill_id` | TEXT / NULL | text / NULL | ID |
| `stage` | TEXT / NN | text / NN | enum |
| `error_code` | TEXT / NN | text / NN | free text |
| `safe_message` | TEXT / NN | text / NN | free text |
| `created_at` | TEXT / NN | text / NN | instant |

### `sync_runs`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `id` | TEXT / NULL | text / NN | ID |
| `source_id` | TEXT / NN | text / NN | ID |
| `status` | TEXT / NN | text / NN | enum |
| `started_at` | TEXT / NN | text / NN | instant |
| `finished_at` | TEXT / NULL | text / NULL | instant |
| `heartbeat_at` | TEXT / NULL | text / NULL | instant |
| `worker_id` | TEXT / NULL | text / NULL | ID |
| `accounts_processed` | INTEGER / NN | integer / NN | integer |
| `transactions_inserted` | INTEGER / NN | integer / NN | integer |
| `transactions_updated` | INTEGER / NN | integer / NN | integer |
| `general_error_code` | TEXT / NULL | text / NULL | free text |
| `general_error_message` | TEXT / NULL | text / NULL | free text |
| `investments_processed` | INTEGER / NN | integer / NN | integer |
| `investment_transactions_inserted` | INTEGER / NN | integer / NN | integer |
| `investment_transactions_updated` | INTEGER / NN | integer / NN | integer |
| `bills_processed` | INTEGER / NN | integer / NN | integer |
| `run_type` | TEXT / NN | text / NN | enum |

### `transaction_category_decisions`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `transaction_id` | TEXT / NULL | text / NN | ID |
| `category_id` | TEXT / NN | text / NN | ID |
| `revision` | INTEGER / NN | integer / NN | integer |
| `changed_at` | TEXT / NN | text / NN | instant |
| `origin` | TEXT / NN | text / NN | enum |

### `transaction_category_events`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `id` | TEXT / NULL | text / NN | ID |
| `transaction_id` | TEXT / NN | text / NN | ID |
| `revision` | INTEGER / NN | integer / NN | integer |
| `previous_category_id` | TEXT / NULL | text / NULL | ID |
| `resulting_category_id` | TEXT / NN | text / NN | ID |
| `changed_at` | TEXT / NN | text / NN | instant |
| `origin` | TEXT / NN | text / NN | enum |

### `transaction_inclusion_decisions`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `transaction_id` | TEXT / NULL | text / NN | ID |
| `state` | TEXT / NN | text / NN | enum |
| `revision` | INTEGER / NN | integer / NN | integer |
| `changed_at` | TEXT / NN | text / NN | instant |
| `origin` | TEXT / NN | text / NN | enum |
| `rule_id` | TEXT / NULL | text / NULL | ID |
| `rule_name` | TEXT / NULL | text / NULL | free text |

### `transaction_inclusion_events`

| Column | SQLite type / nullability | Postgres type / nullability | Classification |
|---|---|---|---|
| `id` | TEXT / NULL | text / NN | ID |
| `transaction_id` | TEXT / NN | text / NN | ID |
| `revision` | INTEGER / NN | integer / NN | integer |
| `previous_state` | TEXT / NN | text / NN | enum |
| `resulting_state` | TEXT / NN | text / NN | enum |
| `changed_at` | TEXT / NN | text / NN | instant |
| `origin` | TEXT / NN | text / NN | enum |
| `rule_id` | TEXT / NULL | text / NULL | ID |
| `rule_name` | TEXT / NULL | text / NULL | free text |

