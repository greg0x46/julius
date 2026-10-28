# Open Banking sync

> Living reference — what exists and works today. Keep it updated when the
> feature changes. No dedicated feature spec (the Pluggy integration is the
> project's historical base). This context is not a domain engine — it is an
> ingestion provider for the Ledger engine; see "Not engines" in
> `.specs/motores-de-dominio.md`.

## What it is

A background worker polls Pluggy periodically for new transactions and account
data, recording each execution as an auditable "sync run" (history, metrics,
failures) — not a black-box import.

Several connections may exist (one `data_sources` row per Pluggy item, i.e. one
per bank). All use the same client ID/secret — they are items of the same Pluggy
application — and each sync run belongs to exactly one connection.

## Backend

- `internal/datasources` — connection registry: list/create/rename and
  activate/deactivate `data_sources`. There is no deletion: deactivating
  preserves accounts, transactions and history and only removes the connection
  from future syncs.
- `internal/pluggy` — Pluggy API adapter (`Adapter`, `Config`, auth/retry), maps
  raw payloads to `SourceSnapshot`/accounts/investments/bills. No routes of its
  own. Ids and cursors are
  escaped in request URLs; every wait (retry, rate limit) and raw-import write
  is tied to the run's context, so cancelling the run stops it promptly; response
  bodies are capped at 32 MiB.
- `internal/syncsvc` — `Service.Execute` orchestrates a sync run against a
  `Provider` (the Pluggy adapter): processes accounts, investments, bills,
  records raw imports and failures/rejections. Each entry uses a database
  transaction covering the upsert, hash, categories, automation hook, audit
  events and counters. If any step fails, the whole record is rolled back and
  the rejection is recorded separately; the other records on the page continue.
  A new sync with the same payload retries the insert/update, since the failure
  did not commit the new hash. Completed records with an equal hash stay
  `unchanged`, without re-applying categories or automation.
- `internal/worker` — background polling loop (`ClaimNextRun`, `ProcessClaim`,
  `Run`). The item queried comes from the run's `source_id`, not from global
  configuration. Runs one run at a time per instance, so N connections sync
  serially.
  - Atomic claim: a single `UPDATE ... WHERE worker_id IS NULL ... RETURNING`
    sets `worker_id`/`heartbeat_at`; with several instances on the same
    Postgres, exactly one wins each run.
  - Lease: the owner updates `heartbeat_at` every 15 s; if the heartbeat no
    longer finds the run as its own, execution is cancelled
    (`sync_run_claim_lost`).
  - Owner-only writes: raw imports and each persistence transaction verify run
    ownership in the same transaction that writes the data. `finalize`,
    `failGeneral` and `FailRun` also require the owner; otherwise they return
    `syncsvc.ErrClaimLost`. A run already terminal is a no-op (no double
    completion).
  - Recovery: at start and every ~45 s, independently of another sync's
    execution, claimed runs with no heartbeat for 90 s become `failed` with
    `general_error_code = 'interrupted'`, via compare-and-set (a single failure
    recorded even with concurrent recoveries). There is no automatic retry; the
    daily schedule or "Sincronizar agora" creates a new run. Unclaimed runs
    survive a restart and run later, but one unclaimed for 15 min (e.g. no
    Pluggy credentials configured) becomes `failed` with
    `general_error_code = 'worker_unavailable'` (`syncsvc.FailUnclaimedRuns`,
    same compare-and-set), so it no longer blocks the connection's next sync.
    Missing credentials are logged once per distinct reason, not every poll. With one instance, a run interrupted by a
    crash is only recovered when the lease expires (up to ~90 s after restart),
    and until then the connection shows as syncing.
  - Limits: SQLite serves a single process (one connection; sharing the file
    across processes is unsupported). Multiple instances require Postgres.

## Identity and preservation of user decisions

- A synced entry's identity is `(source_id, external_id)` (unique in SQLite and
  Postgres). The same external account/transaction in two connections yields two
  independent rows.
- A new sync of the same record compares `normalized_hash`: equal is `unchanged`
  (nothing is written besides the normalization event); different is `updated`
  and rewrites only the provider's fields. PENDING → POSTED and provider
  corrections keep the same `id`.
- Provider fields (overwritten on update): description, amounts, status,
  `occurred_at`, merchant, source category, provider ids and dates,
  `current_raw_import_id` and `normalized_hash`.
- User data (never written by a provider update): manual category and its
  events, manual inclusion decision, `payable_transaction_links`,
  `scenario_realizations` (settlement, allocation and reconciliation) and
  `financial_accounts.manual_closing_day`. Entries have no user note field, so
  there is no note to preserve.
- Derived decisions (automatic, learned, card-payment) are written only on
  insert; a correction does not recompute them. Automation runs again on
  `updated`, so a rule may start to apply, but it never overrides a manual
  decision.
- If the same connection returns an `external_id` already stored under another
  account, the record is rejected (`unsafe_account_association`) and the
  existing row, its hash and the user's decisions stay intact; the other records
  on the page continue.
- Investment movements get the same protection: an `external_id` already stored
  under another investment is rejected (`unsafe_investment_association`),
  without changing the existing movement; the other movements continue.
- Matches between Pluggy, manual entry and file are never merged.
- Coverage: `internal/syncsvc/ingestion_test.go` and `transaction_retry_test.go`
  run on SQLite and, with `JULIUS_TEST_POSTGRES_DSN`, on Postgres
  (`internal/db/dbtest`).

## HTTP routes

`GET/POST /api/data-sources`, `GET/PATCH /api/data-sources/{id}`.

`POST/GET /api/sync-runs`, `GET /api/sync-runs/{id}`. POST accepts an optional
`source_id`: without it, it queues one run per active connection (skipping those
already syncing) and responds 202 with the list of what started; it responds 409
only when nothing could start.

## Frontend

- `/open-banking` — registered connections (add, rename, activate/deactivate,
  sync a single one) and the run list/history, with a sync-all action.
- `/open-banking/sync-runs/:id` — run detail, polling while it is in progress.

## Notes

- The item is created in the Pluggy dashboard and pasted here — the adapter has
  no support for the Pluggy Connect connect token / widget.
- The account balance reported by Pluggy is the reference for cash on hand, but
  not for income/expense totals or card debt — it ignores transaction
  inclusion/exclusion rules. See Transactions and section 1 of
  `.specs/motores-de-dominio.md`.


## MCP operations

`list_sync_runs` and `get_sync_run` expose bounded sync history, counters and
canonical safe failure messages with `mcp:access`; file imports stay excluded.
`request_sync` requires `mcp:access mcp:write mcp:operations`, validates the optional
connection UUID and checks worker credential readiness before calling the same
HTTP enqueue operation. It returns a receipt with 202 and durable queued run IDs,
never synchronous completion. Active-connection filtering, per-source database
uniqueness, partial enqueue outcomes, worker claims/leases and scheduling stay
unchanged. Repeated keys return the original receipt within the process/token
retention period; a new intentional request needs a new key. MCP exposes neither
provider item IDs nor credentials. Setup, retry limits and validation:
[README operational tools](../../../README.md#operational-tools).
