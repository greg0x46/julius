# Settings / Authentication

> Living reference of the implemented behavior.

## Model

A single owner account, with no public sign-up or provider dependency.
`internal/auth` persists the account and sessions in SQLite/Postgres; Argon2id
(19 MiB, two iterations, parallelism 1, versioned parameters) protects the
password. The email is normalized; the password has no length or composition
rules beyond being non-empty.

Sessions use random 32-byte tokens, only the SHA-256 is stored, and the cookie
is HttpOnly/SameSite=Lax/Path=/ with no Domain. HTTPS uses Secure and the
`__Host-` prefix; local HTTP uses the unprefixed name. Absolute lifetime of
seven days and 24 hours of inactivity, refreshed on every authenticated
request (the UI's periodic polling counts as activity). Expiry is validated by
the server. Logout revokes one session; password change/reset revoke all. The
account revision also invalidates sessions created concurrently with a
change/reset. Login clears expired sessions. Sessions survive restarts.

`internal/settings.Secrets` holds the immutable key used for AES-256-GCM; it is
not a user session. The external key is validated against the master verifier
and every encrypted value before HTTP and the worker start. The worker does not
depend on login; it waits while Pluggy is not configured. The old
password-derivation functions in settings exist only for migration and tests.

## Configuration and commands

- `JULIUS_PUBLIC_URL`: exact origin without a path, HTTPS in production; HTTP
  only for localhost/127.0.0.1/::1. Security is not derived from proxy headers.
- Exactly one of `JULIUS_MASTER_KEY` and `JULIUS_MASTER_KEY_FILE`: a
  random 32-byte key in Base64. No cloud integration.
- `julius auth init [-db ...]`: creates the account in a database without
  legacy configuration.
- `julius auth migrate [-db ...]`: with the process stopped, asks for the
  old password, email and new password; re-encrypts every secret, creates the
  account and removes the legacy verifier in one transaction. Requires a prior
  backup; refuses to run twice.
- `julius auth reset-password [-db ...]`: resets the password and revokes
  sessions; does not need or change the key. Passwords are read without echo
  from a terminal.
- `JULIUS_DEMO=true` (empty or false: off; any other value refuses to start):
  public demo mode, see "Demo mode" below.
- `julius demo seed [-db ...]`: fills a database that has no owner with
  synthetic data (`internal/demo`) and refuses one that has an owner. Needs the
  same `JULIUS_PUBLIC_URL` and key variables as the server; reads nothing
  interactively.

Owner/session timestamps use UTC text in SQLite and native `TIMESTAMPTZ` in
Postgres, retaining second-level lifetime and idle-timeout semantics. Database
opening automatically converts final legacy Postgres in one transaction while
retaining the original tables in a backup schema, including hashes, revisions,
encrypted settings and session deadlines. Concurrent startups serialize and
invalid data rolls back the conversion. Legacy SQLite requires a
`cmd/migrate-legacy` converts a final legacy SQLite backup into a separate fresh
SQLite/Postgres target. Unix-second owner/session timestamps become UTC instants
without extending sessions; encrypted settings, hashes and revisions copy
unchanged with the same external master key. Dry-run rolls back imported data
and reports row errors; count and native account balance checks gate completion.
See README "Database schema generations" for cutover and retry steps.

The server refuses pending configuration or an invalid key. A lost key cannot
be recovered with the password; key rotation is not implemented. Instructions
are in the README.

## HTTP

| Method / route | Behavior |
|---|---|
| POST /api/auth/login | Takes email/password, sets the cookie, returns authenticated/email |
| GET /api/auth/session | Returns authenticated and email when authenticated; false without a session |
| POST /api/auth/logout | Revokes the current cookie, returns authenticated=false |
| PUT /api/auth/password | Takes current_password/password, revokes all sessions, returns authenticated=false |
| PUT /api/auth/config | Enables/disables authentication (see README "open mode") |
| PUT /api/settings/pluggy | Stores pluggy_client_id/pluggy_client_secret encrypted; optional pluggy_item_id (trimmed; whitespace-only is 422) creates a connection in the same transaction unless it already exists, so repeating the call is idempotent; returns saved=true |
| GET /api/settings/quotes | Returns enabled, time, timezone and last_refresh (or null); never returns the token |
| PUT /api/settings/quotes | Partially updates enabled/time/timezone and optional brapi_token atomically; returns saved=true |
| POST /api/settings/quotes/refresh | Runs a manual full refresh and returns its recorded outcome; 409 if another refresh is running, 503 if market data is unavailable |
| GET/PUT /api/preferences | Existing preferences, now authenticated |

All `/api` routes are private by default, except the login and status
endpoints. No session: 401; database failure: 503. Static files and `/health`
are public. The legacy setup/unlock routes are not registered. Writes,
including login, require the configured Origin and `X-Julius-Request: 1`
(403 otherwise), with no CORS. `/api` responses get `Cache-Control: no-store`.
Login and password change share limits of five attempts/minute per email and
thirty global, in bounded memory, plus one concurrent KDF operation. Excess or
contention: 429 with `Retry-After`. Credential messages are generic; the KDF
also runs for a wrong email. Authentication bodies are limited to 4 KiB.

## Frontend

`AuthGate` checks the session, including every minute and on focus, and shows
`LoginPage` in place when needed. The centralized HTTP transport injects the
CSRF header, uses same-origin cookies and combines call cancellation with the
session's. A private 401 clears session/cache and unmounts content; pending
responses/bodies are discarded after logout. Wrong login or password shows an
error in the form. Settings contains password change, Sair (log out) and the
write-only Pluggy form.

## Validation and limits

Tests cover migration/rollback on both dialects, expiry, revocation, restart,
cookies, CSRF, rate limiting, database failures and cancellation/cache.
Postgres uses `JULIUS_TEST_POSTGRES_DSN` with a schema isolated per auth
test. Claiming of sync runs by the worker is coordinated across instances via
Postgres (see Open Banking sync); the rate limit stays per instance. No
multiple users, MFA or email recovery. Hosting and provisioning are external.

## Quote schedule settings

`enabled` is a boolean, `time` requires canonical `HH:MM` (00:00–23:59), and
`timezone` must be a loadable zone such as `America/Sao_Paulo`. Omitted fields
keep their current values; `enabled: false` disables refresh, and
`brapi_token: ""` clears the token. The token stays encrypted and write-only.
A time-only update does not require or alter the key/token. Invalid values
return 422; database or key failures return 503 without a partial write. GET
and PUT require a session, and PUT keeps the usual CSRF protection. The
default is enabled at 19:00 America/Sao_Paulo. The running scheduler applies
changes without restart, including suspending history backfill. Legacy
environment initialization and precedence are described under Investments.
The investment settings screen shows the persisted schedule, last refresh trigger
and result, and provides “Atualizar agora”; the brapi token form is in a collapsed
advanced provider section. Manual refresh works with the schedule disabled. Only
full refreshes are recorded; spot refreshes triggered by saved positions are not.
A shared lock prevents manual and scheduled spot refreshes from overlapping.
History backfill follows the spot refresh asynchronously and is not included in
the recorded outcome. Provider failures leave previously stored quotations
intact.

Schedule initialization stores the unencrypted flag using SQL `FALSE`, which
works with SQLite integer flags and Postgres native booleans.
`TestInitializeScheduleBothDialects` covers startup initialization;
`TestQuoteSettingsPostgresRoundTrip` covers HTTP schedule writes.

## Financial reference currency

`finance.reference_currency` is an unencrypted persisted preference, defaulting
to `BRL` when absent. GET/PUT `/api/preferences` expose `reference_currency`
alongside `transactions_period_basis`. The currency must be a canonical
uppercase ISO 4217 code from the same catalog as `/api/accounts/currencies`,
excluding `XXX`. Invalid input returns 422 before either preference is written;
the two writes share a transaction. Older clients may omit the currency to
preserve the saved preference. The General settings screen offers one searchable
currency selector and one Save action for both preferences.

This is a reporting valuation, never a change to the original currency, amount,
balance or quantity. The existing consolidated BRL base and its eligibility
exclusions are preserved. Home/timeline, category reports and investment
summaries are converted using current BRL/reference FX; historical net-worth
snapshots use captured-day FX. API responses carry `currency_code` and `fx`
(pair, exact rate, inversion, requested/effective day, source, fetched time,
stale flag) from `internal/exchange`. Missing FX returns 503
`exchange-unavailable` with the pair, never an identity estimate; stale stored
rates remain usable with a visible warning. The ISO catalog includes currencies
without provider coverage: selecting one is valid, but reports require a stored
or provider-supplied rate.

Saving clears the timeline, category/transaction, net-worth and investment-summary
caches. Report labels come from their own response currency, so a pending
request never relabels an old BRL number as USD. The API validates a consistent
currency across a net-worth series. Native account, transaction, position and
operation details retain their original currency. Investment secondary totals
computed in the browser use the summary's exact FX rate; percentages are unchanged.

Non-multipart request bodies are capped at 1 MiB (413 `body-too-large`); the statement upload keeps its own cap.

## Demo mode

`auth.Config.Demo` (from `JULIUS_DEMO`) makes the gate treat authentication as
off without reading the stored preference, so nothing can close the instance,
and adds `"demo": true` to the open-mode `GET /api/auth/session` (the UI shows
a notice and replaces the access controls with an explanation). Writes whose
path, cleaned of dot segments, is under `/api/auth/` or `/api/settings/`, or is
`/api/sync-runs` or `/api/data-sources[/...]`, answer 403 `demo-mode` after the
Origin check; everything else is unchanged. `cmd/julius` starts neither the sync
worker nor the schedules in demo mode.

`internal/demo.Seed` fills a fresh, migrated database through the real HTTP
handler in process, so every row follows the normal validation and
transactions. It creates the owner with a random unrecoverable password,
disables authentication and the quote refresh, seeds the asset catalog and
then builds a synthetic household relative to the day it runs: three manual
accounts, six months of income, bills and spending, transfers, recurring
commitments, a debt and a receivable, goals, an automation rule and a portfolio
whose listed assets carry a stored price series with its coverage (the demo
never calls a quote provider). Tests pin that every account balance is
non-negative and every position is valued.
