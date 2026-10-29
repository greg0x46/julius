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
summaries are converted using current BRL/reference FX; the historical
wealth series uses each day's FX. API responses carry `currency_code` and `fx`
(pair, exact rate, inversion, requested/effective day, source, fetched time,
stale flag) from `internal/exchange`. Missing FX returns 503
`exchange-unavailable` with the pair, never an identity estimate; stale stored
rates remain usable with a visible warning. The ISO catalog includes currencies
without provider coverage: selecting one is valid, but reports require a stored
or provider-supplied rate.

Saving clears the timeline, category/transaction, patrimônio and investment-summary
caches. Report labels come from their own response currency, so a pending
request never relabels an old BRL number as USD. The API validates a consistent
currency across the wealth series. Native account, transaction, position and
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

## MCP transport

`internal/mcp` uses the official Go MCP SDK and stateless Streamable HTTP,
mounted at `/mcp` by `cmd/julius`, outside the REST cookie/open-mode gate.
`JULIUS_MCP_ENABLED` defaults to false (404); invalid boolean values and enabling
it with demo mode refuse startup. `/mcp/` and descendants are reserved (404),
including in the SPA fallback. Eight read-only planning tools are registered by
`internal/httpapi.RegisterMCPPlanningTools`, sharing domain engines and REST DTO
formatters without HTTP dispatch. Separately scoped writes use a fixed operation allowlist
and the existing REST validators/handlers or domain category transaction. Protocol versions,
initialize/discovery, transport
errors and JSON-RPC errors are handled by the SDK.

Every MCP request requires a scoped OAuth Bearer token from `internal/mcp`;
owner sessions and cookies never grant access. Authentication must remain
enabled (403 when off); storage failures return 503. Invalid, expired, revoked
or wrong-resource tokens return 401 with a protected-resource metadata challenge;
missing `mcp:access` returns 403/`insufficient_scope`. Origin is optional for
non-browser clients, but when present must equal the configured public origin.
There is no CORS. `/api` retains its existing cookie/CSRF gate and ignores
OAuth tokens.

### OAuth authorization

The OAuth handler is mounted outside `/api` by `cmd/julius/server.go`, alongside
`/mcp`. All routes disappear when MCP is disabled; demo mode refuses startup,
and the authentication preference is checked before every OAuth request.

| Method / route | Behavior |
|---|---|
| GET /.well-known/oauth-protected-resource[/mcp] | RFC 9728 metadata, exact public `/mcp` resource, authorization server and supported scope |
| GET /.well-known/oauth-authorization-server | RFC 8414 issuer/endpoints, code grant, S256 PKCE, public-client authentication and scopes |
| POST /oauth/register | RFC 7591 public-client registration; random opaque ID, name and exact HTTPS/loopback redirect URIs |
| GET /oauth/authorize | Validates client, exact redirect, resource, scope and S256 challenge; renders owner consent |
| POST /oauth/authorize | Checks bound cookie and Origin; explicit denial or fresh owner credentials plus explicit approval; redirects code/error with state and issuer |
| POST /oauth/token | Form-encoded code exchange; consumes code atomically and validates client, redirect, resource, PKCE and owner revision |
| GET/POST /oauth/write | Immutable write preview and one-time owner approval/denial; fresh credentials, Origin and browser CSRF binding |
| POST /oauth/revoke | Form-encoded client ID/token; idempotent revocation of the connection belonging to that client/token |

Migration `0007_mcp_oauth` adds `oauth_clients` and `oauth_grants` on both
dialects. These tables start empty when converting legacy databases. Client IDs
are opaque text. Grant hashes identify consent requests, authorization codes
and access tokens; grant rows contain client, redirect, resource, scope, PKCE
challenge, state, CSRF hash, owner revision and expiry. SQLite stores UTC text
expiry; Postgres uses `TIMESTAMPTZ`. Expired grants are pruned on authorization.
`auth.Store.VerifyOwner` checks fresh credentials without issuing a browser
session. Code/token insertion compares the verified owner revision to prevent
password-reset races; reads also require the current revision. Browser logout
has no effect on OAuth tokens; password changes/resets revoke all of them.

`mcp:access` covers the transport and read-only tools. Optional `mcp:write`
requires `mcp:access` too; scope order is accepted and normalized. Optional
`mcp:operations` additionally requires `mcp:write`, authorizing guarded network
requests explicitly disclosed on consent, without a new financial approval. Read-only
grants cannot prepare or inspect writes. Each write needs its own owner approval. Existing grants also permit these reads; consent now explicitly
discloses financial access and explicit hypothetical selection. Unknown requested
scopes are rejected. Consent offers the requested tier and lower ones, defaulting
to read-only (the owner can grant less than requested; the selection persists
through credential errors). A receiving middleware rechecks, on every tool call,
the MCP and browser-auth flags, token expiry/revision, connection revocation and
the current scope narrowed to the token scope; tools enforce their own scope from
that fresh identity. Consent requests and codes last
five minutes; access tokens last one hour without idle refresh. Only hashes of
random 32-byte credentials reach storage. Code and consent consumption are
single-use under concurrent requests. Tokens are bound to the exact configured
public origin plus `/mcp`; both authorization and token requests require this
resource. No secret from settings is involved in issuance.

The server-rendered Portuguese consent page (`consent.go`) shows escaped,
unverified client name, client ID, redirect, resource and scope. It requires
fresh owner credentials and a separate explicit approval even with a browser
session. Denial needs no password. An HttpOnly/SameSite=Strict host cookie binds
the pending request to the browser; consent POST requires exact Origin. CSP
blocks framing/scripts and restricts the credential form to the same origin;
responses are no-store and no-referrer. After approval or denial, a fresh HTML
document navigates to the validated callback with a meta refresh and an escaped
fallback link, avoiding CSP restrictions on cross-origin form redirects. Failed
credentials retain the details/email and clear the password.
OAuth protocol errors use OAuth JSON or a validated redirect, not REST Problems.

Supported registration is RFC 7591 DCR only. Client ID Metadata Documents are
not supported and their discovery flag is false. Only public clients using
`none` authentication are supported; no refresh token, implicit/password grant,
OIDC, device flow, registration-management API or browser CORS. Redirect URIs
must match exactly, including loopback ports; HTTPS is required except HTTP
loopback redirects. Deployment requires a valid HTTPS public origin outside
loopback; no forwarded header determines the issuer/resource. See README MCP
for client setup and synthetic dev validation.

OAuth routes have a per-instance 120/minute global limit and 16 KiB bodies;
credential verification is limited to five/minute and one concurrent KDF.
Registration is capped at 1,000 clients per database. These local limits do not
replace edge abuse controls for publicly exposed deployments.

Tests in `internal/mcp/oauth_test.go` run on SQLite and Postgres: successful
registration/consent/exchange/discovery, restart, denial, replay and concurrent
exchange, malformed registrations, PKCE/client/redirect/resource mismatch,
expiry, revocation, password reset, insufficient scope, XSS/CSRF, rate limits,
open mode, anonymous tools and separation from browser sessions and REST.

JSON responses, no persistent MCP sessions, 1 MiB bodies, a 30-second timeout
(including body reads), and 16 in-flight requests bound resource use. Saturation
returns 429 with Retry-After; GET/DELETE return 405. Responses are no-store.
The binary handles SIGINT/SIGTERM and drains HTTP for 35 seconds before closing
connections and then the database. Tests: `internal/mcp/server_test.go` covers
configuration, both-dialect access checks, generic SDK discovery, legacy
initialize, malformed/unsupported requests, unknown tools, limits and
cancellation; `cmd/julius/server_test.go` pins route isolation and shutdown.


### MCP authorization management

Paired migration `0008_mcp_authorizations` adds `mcp_authorizations`, with UUID
connection ID, opaque OAuth client ID, current scope, approved scope ceiling,
owner revision, creation/last-use/expiry/revocation timestamps. `oauth_grants`
links codes/tokens to this connection; pending consent has no connection. Every
fresh consent creates an independent connection, including for the same client.
Existing pre-upgrade tokens and codes without a connection fail closed and need
fresh authorization. Legacy conversion leaves these tables empty.

`internal/mcpaccess` owns lifecycle persistence and `mcp.enabled` (unencrypted,
default true under the server's disabled-by-default environment flag). A false
runtime flag denies further MCP/OAuth requests with 403; re-enabling resumes
unexpired, non-revoked connections. Environment disable removes routes until
restart and cannot be overridden through settings. Both checks are independent
of browser login. Management requires an authenticated owner even in open mode;
demo blocks management. Clients never gain REST access through an OAuth token.

| Method / route | Behavior |
|---|---|
| GET /api/settings/mcp | Returns server availability, runtime enabled flag and all authorized connections, including expired/revoked; no credentials/hashes |
| PUT /api/settings/mcp | Saves the runtime enabled flag; requires normal CSRF headers |
| PATCH /api/settings/mcp/connections/{id} | Sets a scope tier (read, read/write, read/write/operate) within the original consent ceiling; expired/revoked/escalating changes return 409 |
| DELETE /api/settings/mcp/connections/{id} | Idempotently records revocation for that connection; unknown IDs return 409 |

Security settings reuses Section, Switch, Select and Popconfirm, with loading,
retry, empty, unavailable-server and per-action error states. Names are explicitly
self-reported in consent; IDs distinguish identical names. Read-only consent has
no tier selector. Active broader consent can be reduced and restored within its
ceiling; broader access requires new consent. Revoked/expired rows retain dates
and permission summaries without editing controls. Disabling MCP leaves the list
and revocation controls usable; demo/open mode shows explanatory copy.

A connection expires 65 minutes after consent (five-minute exchange window plus
one-hour token lifetime); the token itself can expire earlier. Successful MCP
credential checks update last use, including discovery; neither lifetime extends.
Revocation blocks outstanding codes and later calls. Password change/reset marks
all connections revoked in the same transaction that increments owner revision
and clears browser sessions. Revision checks prevent racing issuance from gaining
access. Logout affects only the browser. In-flight executions may finish.

Tests: `internal/mcpaccess/store_test.go` covers persistence/lifecycle on both
dialects. `internal/mcp/authorizations_test.go` tests management isolation, CSRF,
revoked-code exchange, read/write execution, runtime disable, consent ceiling,
demo/open access and safe errors on SQLite/Postgres. Frontend
`MCPSettings.test.tsx` covers controls, confirmation, errors, states and independent
connections; `api/mcpSettings.test.ts` validates response data and transport.

### Financial read inventory

See [README MCP](../../../docs/legacy-readme.md#mcp) for the tool/input inventory and synthetic
validation steps. `get_investment_portfolio`, `get_net_worth`, `list_goals`,
`list_payables`, `list_recurring_commitments`, `list_scenarios`,
`list_projection_events` and `get_timeline` advertise typed input/output schemas
and read-only annotations. Decimal types in FX metadata are explicitly described
as strings. Lists use limit/offset (50 default, 100 maximum); timeline/events/
wealth ranges are inclusive and at most 366 days; each result is limited to
256 KiB. Oversized results fail with narrowing instructions rather than dropping
financial records or altering totals. At most 20 explicit scenario IDs are
accepted and unknown IDs fail validation.

MCP timeline/event reads explicitly select active non-standalone commitments
plus requested IDs, excluding even active standalone hypotheticals unless named.
Actual wealth, goal progress and payable settlements never read scenarios.
Portfolio and wealth use reference-currency FX; goals/payables/commitments/events
remain BRL. Assumptions, partial history, unavailable figures and FX provenance
survive presentation. Domain failures become sanitized tool errors; incomplete
reporting carries structured coverage JSON in the error text. No financial
payload accompanies an error.

`internal/httpapi/mcp_planning_test.go` verifies generic SDK calls and output
schema validation, application parity, exact decimals, mixed/reference currencies,
unavailable historical FX and portfolio figures, date validation/clamping,
explicit/empty scenario selection (including an active hypothetical), recurring
reconciliation, pagination, output bounds and sanitized storage failures.
Selection and missing-FX tests run on both database dialects.


### Financial write approval

`internal/mcp/writes.go` registers thirty-one preparation tools plus `get_write_result`;
[README financial writes](../../../docs/legacy-readme.md#financial-writes) lists exact tool
names, body fields, bounds and synthetic dev validation. `internal/httpapi/mcp_writes.go`
validates allowlisted operations, captures before-state, installment identifiers
and financial links/amounts, and calls existing ledger/automation/category paths.
`internal/httpapi/mcp_planning_writes.go` adds goal, standalone scenario/planned
transaction and recurring commitment validation/snapshots, reusing their existing
request conversions and domain flows. Scenario updates rename only. Planned
transactions carry an explicit parent `scenario_id`; parent kind and child
ownership are checked on preview/application. Goal progress and realized totals
remain derived; standalone selection stays explicit.
Ignore/delete previews include all own-account transfer participants and their
native amounts and before-state: removing a transfer relationship preserves the
counterpart entries but changes which reporting decisions apply.
Tool input schemas reuse the corresponding REST DTO fields; decimal amounts
are strings. Raw JSON result fields have explicit unconstrained JSON schemas,
not byte-array schemas. Unknown operations/fields are rejected.

Every mutation returns an immutable preview and an unguessable approval URL,
bound to the exact originating write-scoped token. Only the browser's same-origin,
CSRF-bound POST with fresh owner credentials can execute it. Client-supplied
approval flags or prompts cannot grant approval. Original grant scope, resource,
expiry and owner revision are rechecked before application. Revocation and
open mode block pending actions. The Portuguese server-rendered page reuses
consent styles/accessibility, escapes client content, displays requested/current
values and affected IDs/count, and discloses cleanup and reporting effects.

Approval executes once while holding the per-instance action mutex; duplicate
POSTs/polls return the retained result. Terminal states are applied/denied/failed;
503 results can be indeterminate after a committed write/read-back failure, so
inspect data and reuse the persistent manual-creation key. At most 100 actions
are retained. Pending actions expire in five minutes; execution outcomes in ten.
Restart loses pending approvals/results, so request another preview; persistent
creation idempotency survives. Multiple replicas need request affinity.

Before-state changes invalidate approval. Installment categorization rechecks
membership and every before-state inside the same serializable transaction as
all decisions/events (100 identifiers maximum). No arbitrary batch or
retroactive rule application is exposed. Scenario/planned-transaction deletion
also rechecks its planning cascade and
realization snapshots inside the deletion transaction, preserving real entries.
Recurrence preview includes schedules, manual decisions and targeting rules; edits
retain orphan cleanup and deletion refuses targeting reconcile actions.
One-record manual/category/rule/goal/recurrence paths revalidate the preview before invoking their existing domain service boundaries.
Read clients can discover schemas but cannot prepare, commit or read write results.

Tests `internal/mcp/writes_test.go` and `internal/httpapi/mcp_writes_test.go`
exercise real SDK/OAuth HTTP flows, both dialects, read-only/anonymous rejection,
CSRF/Origin/credentials, denial/replay/concurrency, token binding/revocation,
expiry, exact money, persistent creation retries/conflicts, soft deletion,
manual/provider restrictions, categorization cascades, stale previews, rule
and catalog management, and injected partial failures with full rollback.

Planning write contracts/authorization are covered by
`internal/mcp/planning_writes_test.go` and
`internal/httpapi/mcp_planning_writes_test.go`: all twelve tools, exact BRL money,
goal/recurrence parity, explicit hypothetical timeline effects, invalid dates/IDs,
stale cascades, parent ownership, anonymous/read-only rejection and destructive
denial/replay. The OAuth consent disclosure names planning writes; each operation
still requires a separate owner approval before application.


### Guarded operations

`internal/mcp/operations.go` registers six tools: `get_operations_status`,
`list_sync_runs`, `get_sync_run`, `request_sync`, `request_quote_refresh` and
`get_operation_result`. The first three require access scope; request and receipt
polling require access, write and operations scopes. All stay behind the same
authentication, demo/open-mode restrictions, 16-request concurrency gate and
30-second transport timeout. Unknown inputs, malformed UUIDs, invalid pagination
and client-supplied overrides are rejected. Reads are bounded to 100 items and
256 KiB; failure details are paginated and use `syncsvc.SafeMessage`, never raw
stored messages, external account/transaction IDs or payloads. Source names are
blanked because REST names can fall back to private provider item IDs.

`httpapi.MCPRequestSync` wraps only the existing enqueue handler. `worker.Ready`
checks the same decrypted credentials as the worker and returns only a boolean.
`cmd/julius` injects the same secrets, market service, sync schedule and shutdown
context used by the existing server. No settings write or arbitrary route is
exposed. Automatic quote disabling preserves manual refresh eligibility.

Request receipts use validated idempotency keys and exact token ownership;
matching retries return the original result even after completion, while changed
arguments fail. A mutex serializes receipt creation, with at most 100 retained
for one hour, excluding running work from expiry. Like write approvals, receipts
are process-local; restart/another instance requires inspecting durable history
before retrying. Sync execution remains in the existing worker and returns
queued IDs. Quote execution calls `quotes.RefreshNow` asynchronously with a
five-minute deadline and server cancellation, retaining shared scheduler/REST
guards, provider cache/retry policy and persistent last-refresh outcomes.
Receipt `finished` is distinct from quote `success`; failures/deferrals retain
counts and do not expose upstream errors. History backfill remains separate.

Setup, troubleshooting and synthetic dev steps: [README operational tools](../../../docs/legacy-readme.md#operational-tools).
`internal/mcp/operations_test.go` covers real OAuth grants, consent disclosure,
read/write/operations scope separation, strict inputs, sanitized bounded history,
ineligible/unavailable services, concurrent queue requests, token isolation,
terminal retries, shared quote guards, upstream failure/deferral and shutdown.
Sync integration coverage runs on SQLite and disposable Postgres.
