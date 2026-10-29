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
See the [legacy conversion guide](../../self-hosting/legacy-conversion.md) for cutover and retry steps.

The server refuses pending configuration or an invalid key. A lost key cannot
be recovered with the password; key rotation is not implemented. Instructions
are in the [security guide](../../self-hosting/security.md).

## HTTP

| Method / route | Behavior |
|---|---|
| POST /api/auth/login | Takes email/password, sets the cookie, returns authenticated/email |
| GET /api/auth/session | Returns authenticated and email when authenticated; false without a session |
| POST /api/auth/logout | Revokes the current cookie, returns authenticated=false |
| PUT /api/auth/password | Takes current_password/password, revokes all sessions, returns authenticated=false |
| PUT /api/auth/config | Enables/disables authentication (see [authentication controls](../../self-hosting/security.md#authentication-and-recovery)) |
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
Every route, REST or OAuth/MCP, is covered by the access log and security
events described in "HTTP access log and security events" below.

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
including in the SPA fallback. Five financial read tools reuse the allowlisted
`internal/httpapi.FinancialReadHandlers` (no client-supplied route or URL is dispatched). Eight read-only planning tools are registered by
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
responses are no-store and `Referrer-Policy: same-origin`. The policy must not
be `no-referrer`: browsers then submit the form with `Origin: null`, which the
exact check rejects. `same-origin` still sends no `Referer` to the client's
`redirect_uri` or any other site. After approval or denial, a fresh HTML
document navigates to the validated callback with a meta refresh and an escaped
fallback link, avoiding CSP restrictions on cross-origin form redirects. Failed
credentials retain the details/email and clear the password.
OAuth protocol errors use OAuth JSON or a validated redirect, not REST Problems.

Supported registration is RFC 7591 DCR only. Client ID Metadata Documents are
not supported and their discovery flag is false. Only public clients using
`none` authentication are supported; no refresh token is issued (registration
accepts `refresh_token` in `grant_types` but registers only `authorization_code`),
no implicit/password grant,
OIDC, device flow, registration-management API or browser CORS. Redirect URIs
must match exactly, including loopback ports; HTTPS is required except HTTP
loopback redirects. Deployment requires a valid HTTPS public origin outside
loopback; no forwarded header determines the issuer/resource. See MCP tool reference
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


### Cross-client validation

`internal/mcp/interop_test.go` runs an independent Python SDK client through
HTTP discovery, DCR, scope rejection, denial/approval, S256 exchange, replay
rejection, MCP initialize/discovery, unknown-tool rejection and revocation.
It also runs the pinned Inspector CLI (TypeScript SDK) against authenticated
tool discovery. Both use `dbtest.Each`; CI installs the test-only clients and
sets their executable paths before the ordinary Go test suite. Locally the
external tests skip unless `JULIUS_MCP_TEST_PYTHON` and
`JULIUS_MCP_TEST_INSPECTOR` are set, respectively. The Go SDK protocol lifecycle
also runs on both dialects and rejects discovery on an already connected client
after revocation. No new binary/runtime dependency is introduced.

[Self-hosted setup and validation](../../mcp.md) documents HTTPS,
consent, revocation, repeatable tests and manual browser/remote-client checks.
The clients validate transport, OAuth and tool discovery. Both the Go SDK
lifecycle test and Python client test seed a synthetic goal, invoke `list_goals`
and assert the returned goal data.

### MCP financial tool inventory

All five tools have `readOnlyHint=true` and `idempotentHint=true` and are
available under the `mcp:access` scope. They expose no credential, provider
payload or configuration data.

| Tool | Inputs | Existing REST read contract |
| --- | --- | --- |
| `list_accounts` | Optional `page`, `page_size` | `GET /api/accounts`: native bank/card balances, currency, limits and closing metadata, including nullable/unavailable fields. |
| `list_card_bills` | Required `account_id`; optional pagination | `GET /api/accounts/{id}/bills`: recorded bills in descending due-date order. The open cycle remains in account balance/cycle metadata. |
| `search_transactions` | Required IANA `timezone`; optional `group_by`, pagination and `filters` | `POST /api/transactions/query`: shared filter DTO and validation; items, category/inclusion decisions, eligibility, all matching native totals and reporting/FX metadata. |
| `list_categories` | Optional pagination | `GET /api/categories`: includes active and inactive categories with kind, icon and color. |
| `spending_by_category` | Required IANA `timezone` and `classification` (`outflow`/`inflow`); exactly one of `month=YYYY-MM`, inclusive `date_from`/`date_to`, or `period=all`; optional pagination | `GET /api/transactions/category-breakdown`: real category totals and native/reporting coverage; no scenarios. |

Pagination defaults to page 1 and 50 items; page is 1–1,000,000 and page size
1–200. Zero selects the default; negatives and larger values fail explicitly.
Search defaults `group_by` to `none`; valid groups are `none`, `day`, `week`,
`month`, `year`. Filters preserve the REST singular/plural account/category,
institution/status, origin/source, card, description, amount and date semantics.
Date range endpoints are YYYY-MM-DD and must be supplied together. Unknown
arguments are rejected by the SDK schema. Empty collections remain `[]`.

Structured results (also serialized in text content) are `{status, data,
pagination?}`. `data` retains the REST JSON payload, exact decimal strings,
explicit nulls, date-only group/range values, transaction instants and legacy
midnight timestamp representation of account/bill calendar dates. List/report
pagination has `{page, page_size, total_items, total_pages}` outside `data`;
transaction pagination remains the API's `data.page`. Category items are paged,
but financial totals always cover the entire requested period. Pages beyond the
end are empty with unchanged counts. No transaction is hidden on eligibility
grounds; pending/ignored/unclassified rows remain queryable. Financial rules
and reference-currency valuation are unchanged. Missing FX retains native totals,
issues and incomplete/unavailable status with null consolidated totals.

REST Problems retain their status and payload and set MCP `isError=true`;
invalid tool arguments produce SDK tool errors. The adapter rejects source JSON
above 8 MiB before pagination, rather than returning a silently truncated read;
clients must narrow transaction/report filters when necessary. Queries reuse the
existing in-memory ledger aggregation and remain subject to the transport's
30-second deadline and concurrency limit; this does not add SQL pagination.

Tests: `internal/httpapi/mcp_reads_test.go` compares all five tools through a
generic SDK client against existing HTTP fixtures, including precision, pending
cards, ignored/transfer exclusions, native and missing/partial FX reports,
filters, empty collections, missing accounts and invalid input.
`internal/mcp/reads_test.go` pins explicit rejection of oversized responses.

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

**Configurações → Segurança → Clientes MCP** reuses Section, Switch, Select
and Popconfirm, with loading, retry, empty, unavailable-server and per-action
error states. Names are explicitly
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

See [MCP tool reference](../mcp-tools.md) for the tool/input inventory and synthetic
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

`internal/mcp/writes.go` registers forty-one preparation tools plus `get_write_result`;
[MCP financial writes](../mcp-tools.md#financial-writes) lists exact tool
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
approval flags or prompts cannot grant approval. Original token and current
connection scope, resource, expiry, revocation and owner revision are rechecked when viewing the preview and before application.
Runtime disable and open mode also block pending actions, including after the
owner credential check. The Portuguese server-rendered page reuses
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

Account and obligation preparation tools extend the same scoped approval lifecycle:
manual accounts/full updates/adjustments, transfers/link removal, payable CRUD and
payable transaction links. Consent and approval pages identify these operations,
exact native/BRL values and cleanup/reporting consequences; provider settings and
secrets remain unavailable. `httpapi/mcp_accounts_writes.go` reuses transaction-aware
ledger/transaction/payable operations and rechecks the captured accounts, movements,
plans and links in one serializable transaction. Updates require explicit account
fields to prevent omitted values from silently resetting balance/currency/archive.

`internal/mcp/accounts_writes_test.go` covers all ten tools, SDK schemas, anonymous
and read-only rejection, owner credentials/Origin/CSRF, denial and approval replay.
`internal/httpapi/mcp_accounts_writes_test.go` covers native precision, ownership,
reporting exclusion/fees, payable direction/BRL eligibility, stale approvals,
scoped unlinks, cleanup and injected failure rollback on both database dialects.
Account/adjustment/transfer/payable creation has no persistent creation key: a fresh
preparation plus approval means a new operation. Inspect Julius before preparing
another after a lost result; same approval POST retries still execute once.

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

Setup, troubleshooting and synthetic dev steps: [MCP operational tools](../mcp-tools.md#operational-tools).
`internal/mcp/operations_test.go` covers real OAuth grants, consent disclosure,
read/write/operations scope separation, strict inputs, sanitized bounded history,
ineligible/unavailable services, concurrent queue requests, token isolation,
terminal retries, shared quote guards, upstream failure/deferral and shutdown.
Sync integration coverage runs on SQLite and disposable Postgres.

## HTTP access log and security events

`internal/httplog` writes the HTTP access log and a closed set of security
events with `log/slog` JSON on stderr. `cmd/julius/server.go:loggedHandler`
wraps the whole server, so a new route is logged without further work.

### Model

- One middleware creates a per-request record in the request context. It
  generates the `request_id` (128 random bits, 32 hex characters), never reads
  it from a header and never returns it to the client.
- Handlers only report controlled codes and reasons (`httplog.SetError`,
  `SetErrorValues`, `SetAuth`, `SetOAuthClient`) and emit events
  (`httplog.Event`); they never format a log line. `writeProblem` feeds its
  `kind` to `error_code`; `httplog.WithRoute` names the route even for requests
  rejected before the router.
- When the handler returns (also on a panic, which is re-raised) the middleware
  writes one `http_request` line, unless the request is routine noise. Event
  lines are written when the action completes and carry the same `request_id`.
- Nothing is stored in the database. Every line also has the `slog` fields
  `time`, `level` and `msg` (equal to `event`).

### `http_request` fields

| Field | Type | Appears | Meaning |
|---|---|---|---|
| `event` | string | always | `http_request` |
| `request_id` | string | always | 32 lowercase hex characters, unique per request, shared with the request's events |
| `method` | string | always | A standard HTTP method, otherwise `OTHER` |
| `route` | string | always | The `ServeMux` pattern that matched (`GET /api/accounts/{id}`, `/mcp`, `/oauth/`, `/`), at most 128 runes; `unmatched` if none. Never a path value or query |
| `status` | int | always | Final status. 1xx are not final except 101 (hijack); a handler that returns without writing is `200` |
| `duration_ms` | float | always | Wall time, microsecond precision |
| `response_bytes` | int | always | Bytes the handler wrote to the body (not headers) |
| `peer_ip` | string | always | IP of the TCP peer (IPv4-mapped IPv6 unmapped, zone dropped); `unknown` when not an IP |
| `forwarded_ip` | string | a valid header is present | **Unverified**, diagnostics only; never used for authorization, rate limits or URLs. `CF-Connecting-IP` wins whenever present (if repeated or invalid the field is omitted, with no fallback); otherwise the first element of the first `X-Forwarded-For` line. Zones, unspecified and malformed values are omitted |
| `user_agent` | string | non-empty | Sanitized, at most 200 runes |
| `auth_type` | string | always | `none`, `session` or `oauth` |
| `auth_result` | string | always | `not_required`, `ok` or `rejected` |
| `client_name` | string | OAuth client known | Registered name, sanitized, at most 100 runes |
| `client_id_prefix` | string | OAuth client known | First 8 characters of the client id |
| `error_code` | string | handler reported | Controlled code, see the catalog |
| `error_reason` | string | handler reported | Controlled sub-reason |
| `invalid_field` | string | handler reported | Name of the input that failed, from a fixed list |
| `invalid_values` | string array | handler reported | At most 8 categorical values, each from a fixed list or `other` |

`error_code`, `error_reason`, `invalid_field` and `invalid_values` entries
match `^[a-z0-9_.:-]{1,64}$`; anything else is logged as `invalid_value`.

**Levels:** status 5xx `error`, 4xx `warn`, everything else `info`; a panic is
always `error`.

**`auth_type` / `auth_result`.** Every request starts as `none` /
`not_required`; only code that really validates credentials changes it.

| Situation | `auth_type` | `auth_result` |
|---|---|---|
| Public route, static file, health, OAuth metadata, REST with authentication off or in demo mode | `none` | `not_required` |
| Private `/api` route without a session cookie (401 `missing_session`) | `none` | `rejected` |
| Private `/api` route, cookie unknown, expired, revoked or malformed (401 `invalid_session`), or session store failure (503) | `session` | `rejected` |
| Private `/api` route with a valid session | `session` | `ok` |
| `GET /api/auth/session` with a valid session | `session` | `ok` |
| `GET /api/auth/session` without or with an invalid session | `none` | `not_required` (the 200 `authenticated:false` is the expected answer, never a rejection); a store failure is a 503 |
| `POST /api/auth/login`, wrong credentials | `none` | `rejected` |
| `POST /api/auth/login`, success, validation error or 429 | `none` | `not_required` (the request creates a session, it does not use one) |
| Failure of the Origin / `X-Julius-Request` check or of the demo guard | `none` | `not_required` (they run before authentication) |
| `/mcp` without an `Authorization` header or with a non-Bearer scheme | `none` | `rejected` |
| `/mcp`, Bearer token unknown, expired, revoked or for another resource | `oauth` | `rejected` |
| `/mcp`, storage failure while checking settings or the token (503) | `oauth` | `rejected` |
| `/mcp`, valid token (also when the scope is insufficient: 403) | `oauth` | `ok` |
| `/mcp` 429 (concurrency), 403 (origin) or 403 (authentication disabled) | `none` | `not_required` |
| `POST /oauth/authorize` or `POST /oauth/write`, wrong owner credentials | `none` | `rejected` |
| Every other OAuth route, including consent with correct credentials | `none` | `not_required` |

**Routine noise.** A request is not logged when its status is below 400 and it
was not `rejected`, and it is one of: a static frontend asset (`GET`/`HEAD` on
the SPA route `/` whose path is under `/assets/` or has an asset extension);
a trivial CORS preflight (`OPTIONS` with `Access-Control-Request-Method`);
`GET /health`; the polls `GET /api/auth/session` and `GET /api/sync-runs/{id}`.
Every 4xx/5xx, every `rejected` request and every panic is logged, and SPA
navigations (`/`, `/transactions`) are logged.

### Security events

Events are emitted when the action has completed, never when it is only
requested. The attribute allowlist is `reason`, `limiter`, `scope`,
`client_name`, `client_id_prefix`; client address and user agent stay on the
request line. A name outside this set is dropped (`httplog.eventLevels`).

| Event | Level | Emitted when | Attributes |
|---|---|---|---|
| `auth.login.success` | info | A session was created and its cookie set | none |
| `auth.login.failure` | warn | `POST /api/auth/login` credentials were rejected (not for validation, 429 or storage failures) | `reason=invalid_credentials` |
| `auth.logout` | info | The session was revoked in the store | none |
| `oauth.client.registered` | info | The client row was inserted (201) | `client_name`, `client_id_prefix` |
| `oauth.consent.granted` | info | Owner credentials checked and the authorization code stored | `scope`, `client_name`, `client_id_prefix` |
| `oauth.consent.denied` | info | The owner explicitly denied the pending request and it was consumed | `client_name`, `client_id_prefix` |
| `oauth.credentials.rejected` | warn | Wrong owner credentials on the consent page or the write approval page | `reason=invalid_credentials`, `client_name`, `client_id_prefix` |
| `oauth.token.revoked` | info | An access token row was actually deleted (an idempotent no-op is silent) | `client_name`, `client_id_prefix` |
| `security.rate_limited` | warn | A limiter refused the request (429) | `limiter`, `reason` |
| `security.invalid_origin` | warn | A state-changing request has a wrong `Origin` or missing `X-Julius-Request` | `reason` |

`limiter` is `auth_attempts` or `auth_hashing_busy` (REST login and password
change), `oauth_requests`, `oauth_consent_attempts`, `oauth_hashing_busy` or
`mcp_concurrency`; `reason` is `window_exceeded` or `busy`. For
`security.invalid_origin`, `reason` is `origin_mismatch`, `origin_missing`,
`origin_null` (the header is exactly `null`; MCP and OAuth only) or, for REST
only, `request_header_missing`. A wrong current password on
`PUT /api/auth/password` is a 401 on the request line (session `ok`) without an
event. The denial of a write approval (`/oauth/write`) and the approval itself
have no event.

### Error catalog

`error_code` is the public error where there is one; `error_reason` and
`invalid_field` add what the response hides. A dash means the field is not set.
Sections list the first check that fails, in code order.

#### POST /oauth/register

The public body is always `{"error":"invalid_client_metadata"}` (or
`invalid_redirect_uri`). Checks run in this order and only the first failure is
logged, so the log identifies the exact validation:

| Status | `error_code` | `error_reason` | `invalid_field` | `invalid_values` | Cause |
|---|---|---|---|---|---|
| 400 | `invalid_client_metadata` | `empty_body` | - | - | Empty or blank body |
| 400 | `invalid_client_metadata` | `body_too_large` | - | - | Body over 16 KiB |
| 400 | `invalid_client_metadata` | `invalid_json` | - | - | Malformed or truncated JSON (also a client that disconnects mid-body) |
| 400 | `invalid_client_metadata` | `invalid_field_type` | `client_name`, `redirect_uris`, `token_endpoint_auth_method`, `grant_types`, `response_types`, `client_id`, `other` (any other member) or `body` (top-level value is not an object) | - | A member has the wrong JSON type |
| 400 | `invalid_client_metadata` | `body_read_error` | - | - | Any other read failure |
| 400 | `invalid_client_metadata` | `multiple_json_values` | - | - | A second JSON value follows the first (garbage after it is `invalid_json`) |
| 400 | `invalid_client_metadata` | `client_id_provided` | `client_id` | - | The client sent `client_id` |
| 400 | `invalid_client_metadata` | `name_too_long` | `client_name` | - | `client_name` over 200 bytes |
| 400 | `invalid_client_metadata` | `redirect_uris_missing` | `redirect_uris` | - | Missing or empty |
| 400 | `invalid_client_metadata` | `too_many_redirect_uris` | `redirect_uris` | - | More than 10 |
| 400 | `invalid_client_metadata` | `unsupported_auth_method` | `token_endpoint_auth_method` | the method: `client_secret_basic`, `client_secret_post`, `client_secret_jwt`, `private_key_jwt`, `tls_client_auth`, `self_signed_tls_client_auth` or `other` | Anything but `none` |
| 400 | `invalid_client_metadata` | `unsupported_grant_types` | `grant_types` | each of the first 8: `authorization_code`, `refresh_token`, `implicit`, `client_credentials`, `password`, `urn:ietf:params:oauth:grant-type:device_code`, `urn:ietf:params:oauth:grant-type:jwt-bearer`, `urn:ietf:params:oauth:grant-type:token-exchange` or `other` | Not `["authorization_code"]` alone or with `refresh_token` (each at most once) |
| 400 | `invalid_client_metadata` | `unsupported_response_types` | `response_types` | each of the first 8: `code`, `token`, `id_token`, `none`, `code.id_token`, `code.token`, `id_token.token`, `code.id_token.token` (space-separated combinations, dotted) or `other` | Not exactly `["code"]` |
| 400 | `invalid_redirect_uri` | `unparseable` | `redirect_uris` | - | First invalid URI does not parse |
| 400 | `invalid_redirect_uri` | `no_host` | `redirect_uris` | - | No host (relative, `mailto:`, opaque) |
| 400 | `invalid_redirect_uri` | `userinfo` | `redirect_uris` | - | Contains `user@` or `user:pass@` |
| 400 | `invalid_redirect_uri` | `fragment` | `redirect_uris` | - | Has a non-empty fragment |
| 400 | `invalid_redirect_uri` | `too_long` | `redirect_uris` | - | Over 2048 bytes |
| 400 | `invalid_redirect_uri` | `scheme_not_allowed` | `redirect_uris` | - | Neither `https` nor `http` |
| 400 | `invalid_redirect_uri` | `http_non_loopback` | `redirect_uris` | - | `http` to a host other than `127.0.0.1`, `::1` or `localhost` |
| 429 | `temporarily_unavailable` | `client_registry_full` | - | - | 1,000 clients already registered |
| 503 | `temporarily_unavailable` | `db_error` | - | - | Insert failed |

Success is 201 with `oauth.client.registered`, which exists only for that
request and has its `request_id`. Reading the log for a failed registration:
`error_reason` names the check, `invalid_field` the member and
`invalid_values` the categories received; for example `unsupported_grant_types`
with `["implicit"]` means the client asked for a grant Julius does not support.

#### All `/oauth/*` and `/.well-known/*` routes (before the handler)

| Status | `error_code` | `error_reason` | Cause |
|---|---|---|---|
| 429 | `temporarily_unavailable` | `rate_limited` | 120 requests per minute per process; also `security.rate_limited` (`oauth_requests`, `window_exceeded`) |
| 503 | `temporarily_unavailable` | `settings_unavailable` | Authentication setting unreadable |
| 403 | `access_denied` | `authentication_disabled` | Open mode |

#### Form parsing (POST `/oauth/authorize`, `/oauth/token`, `/oauth/revoke`, `/oauth/write`)

| Status | `error_code` | `error_reason` | `invalid_field` | `invalid_values` | Cause |
|---|---|---|---|---|---|
| 400 | `invalid_request` | `invalid_content_type` | `content_type` | - | Not `application/x-www-form-urlencoded` |
| 400 | `invalid_request` | `form_unparseable` | - | - | Malformed form |
| 400 | `invalid_request` | `body_too_large` | - | - | Over 16 KiB |
| 400 | `invalid_request` | `duplicate_parameter` | the parameter, or `other` | - | A name sent more than once |

Known parameter names (others are `other`): `client_id`, `redirect_uri`,
`resource`, `scope`, `code_challenge`, `code_challenge_method`, `response_type`,
`state`, `grant_type`, `code`, `code_verifier`, `client_secret`, `token`,
`request`, `decision`, `email`, `password`.

#### GET /oauth/authorize

| Status | `error_code` | `error_reason` | `invalid_field` | `invalid_values` | Cause |
|---|---|---|---|---|---|
| 400 | `invalid_request` | `query_too_long` | - | - | Query over 8192 bytes |
| 400 | `invalid_request` | `query_unparseable` | - | - | Malformed query |
| 400 | `invalid_request` | `duplicate_parameter` | the parameter, or `other` | - | A name sent more than once |
| 400 | `invalid_client` | `unknown_client` | `client_id` | - | Unregistered client |
| 400 | `invalid_client` | `redirect_uri_mismatch` | `redirect_uri` | - | Not one of the client's URIs |
| 503 | `temporarily_unavailable` | `db_error` | - | - | Storage failure |

From here the client is validated (`client_name` / `client_id_prefix` are
logged) and errors return to the client as a 303 redirect:

| Status | `error_code` | `error_reason` | `invalid_field` | `invalid_values` | Cause |
|---|---|---|---|---|---|
| 303 | `unsupported_response_type` | `response_type_not_code` | `response_type` | - | Not `code` |
| 303 | `invalid_target` | `resource_mismatch` | `resource` | - | Not the public `/mcp` URL |
| 303 | `invalid_scope` | `scope_not_supported` | `scope` | requested scopes (first 8): `mcp:access`, `mcp:write`, `mcp:operations`, `openid`, `profile`, `email`, `offline_access`, `other` | A scope Julius does not offer |
| 303 | `invalid_scope` | `scope_duplicate` | `scope` | same | A scope repeated |
| 303 | `invalid_scope` | `scope_invalid_combination` | `scope` | same | `mcp:access` missing, or `mcp:operations` without `mcp:write` |
| 303 | `invalid_request` | `code_challenge_method_not_s256` | `code_challenge_method` | - | Not `S256` |
| 303 | `invalid_request` | `code_challenge_invalid` | `code_challenge` | - | Not 32 bytes of unpadded base64url |
| 503 | `temporarily_unavailable` | `db_error` | - | - | Storage failure |

Success is 200 (consent page) with no event.

#### POST /oauth/authorize (consent)

| Status | `error_code` | `error_reason` | `invalid_field` | Cause / event |
|---|---|---|---|---|
| 403 | `access_denied` | `origin_mismatch`, `origin_missing` or `origin_null` | `origin` | `Origin` is not the public origin; `security.invalid_origin` |
| 400 | `invalid_request` | form parsing rows | | See above |
| 400 | `invalid_request` | `consent_request_not_found` | `request` | Unknown, expired or already used (also when consumed concurrently) |
| 403 | `access_denied` | `csrf_cookie_missing` | - | Browser binding cookie absent |
| 403 | `access_denied` | `csrf_mismatch` | - | Cookie does not match the request |
| 400 | `invalid_request` | `invalid_decision` | `decision` | Neither `allow` nor `deny` |
| 429 | `temporarily_unavailable` | `rate_limited` | - | 5 verifications per minute (`oauth_consent_attempts`, `window_exceeded`) or one in progress (`oauth_hashing_busy`, `busy`); `security.rate_limited` |
| 401 | `invalid_credentials` | `invalid_credentials` | - | Wrong owner credentials (HTML page); auth `none`/`rejected`; `oauth.credentials.rejected` |
| 200 | `access_denied` | `user_denied` | - | Explicit denial; callback page; `oauth.consent.denied` |
| 200 | - | - | - | Granted; callback page; `oauth.consent.granted` |
| 503 | `temporarily_unavailable` | `db_error` | - | Storage failure |

`client_name` and `client_id_prefix` are logged from the `csrf_*` rows onwards,
once the pending request is loaded.

#### POST /oauth/token

| Status | `error_code` | `error_reason` | `invalid_field` | `invalid_values` | Cause |
|---|---|---|---|---|---|
| 400 | `invalid_request` | form parsing rows | | | See above |
| 400 | `unsupported_grant_type` | `unsupported_grant_type` | `grant_type` | the type: the grant types listed for registration, or `other` | Not `authorization_code` |
| 401 | `invalid_client` | `client_authentication_not_supported` | `authorization` | - | `Authorization` header present |
| 401 | `invalid_client` | `client_authentication_not_supported` | `client_secret` | - | `client_secret` sent |
| 400 | `invalid_grant` | `code_not_found_or_expired` | `code` | - | Unknown, expired or used code |
| 400 | `invalid_grant` | `verifier_invalid_format` | `code_verifier` | - | Not 43-128 unreserved characters |
| 400 | `invalid_grant` | `pkce_mismatch` | `code_verifier` | - | Does not match the challenge |
| 400 | `invalid_grant` | `client_id_mismatch` | `client_id` | - | Different from the code's client |
| 400 | `invalid_grant` | `redirect_uri_mismatch` | `redirect_uri` | - | Different from the authorization request |
| 400 | `invalid_target` | `resource_mismatch` | `resource` | - | Not the public `/mcp` URL |
| 400 | `invalid_grant` | `owner_revision_changed` | - | - | Password changed or reset since consent |
| 503 | `temporarily_unavailable` | `db_error` | - | - | Storage failure |

The code is consumed before the verifier, client, redirect and resource
checks, so those failures also spend it. After the `code_verifier`,
`client_id` and `redirect_uri` checks pass, `client_id_prefix` (not the name)
is logged. Success is 200, no event.

#### POST /oauth/revoke

| Status | `error_code` | `error_reason` | `invalid_field` | Cause |
|---|---|---|---|---|
| 400 | `invalid_request` | form parsing rows | | See above |
| 401 | `invalid_client` | `client_authentication_not_supported` | `authorization` or `client_secret` | Client authentication attempted |
| 401 | `invalid_client` | `unknown_client` | `client_id` | Unregistered client |
| 503 | `temporarily_unavailable` | `db_error` | - | Storage failure |

Success is 200 even if nothing was revoked (idempotent); only an actual
deletion emits `oauth.token.revoked`.

#### GET and POST /oauth/write

| Status | `error_code` | `error_reason` | `invalid_field` | Cause / event |
|---|---|---|---|---|
| 403 | `access_denied` | `origin_mismatch`, `origin_missing` or `origin_null` | `origin` | POST only; `security.invalid_origin` |
| 400 | `invalid_request` | form parsing rows | | POST only |
| 410 | `invalid_request` | `write_request_not_found` | `request` | Unknown or expired approval (HTML page) |
| 403 | `access_denied` | `grant_inactive` | - | The token that prepared the write was revoked, expired or lost its write scope |
| 403 | `access_denied` | `csrf_cookie_missing`, `csrf_mismatch` | - | POST only |
| 400 | `invalid_request` | `invalid_decision` | `decision` | POST only |
| 200 | `access_denied` | `user_denied` | - | Denied; no event |
| 429 | `temporarily_unavailable` | `rate_limited` | - | As in consent; `security.rate_limited` |
| 401 | `invalid_credentials` | `invalid_credentials` | - | Wrong owner credentials; `oauth.credentials.rejected` |
| 503 | `temporarily_unavailable` | `db_error` | - | Storage failure |

#### /mcp

| Status | `error_code` | `error_reason` | `invalid_field` | Cause / event |
|---|---|---|---|---|
| 429 | `too_many_requests` | `busy` | - | 16 requests in flight; `security.rate_limited` (`mcp_concurrency`, `busy`) |
| 403 | `access_denied` | `origin_mismatch` or `origin_null` | `origin` | An `Origin` header that is not the public origin (an absent header is allowed for non-browser clients); `security.invalid_origin` |
| 401 | `unauthorized` | `missing_bearer` | - | No `Authorization` header |
| 401 | `unauthorized` | `malformed_authorization` | - | Header is not `Bearer <token>` |
| 401 | `unauthorized` | `invalid_token` | - | Unknown, expired, revoked or wrong-resource token |
| 503 | `temporarily_unavailable` | `auth_unavailable` | - | Settings or token lookup failed |
| 403 | `access_denied` | `authentication_disabled` | - | Open mode |
| 403 | `insufficient_scope` | `scope_invalid` | - | The stored scope lacks `mcp:access` |

Protocol and JSON-RPC errors come from the MCP SDK and carry no `error_code`.

#### REST API (`/api/*`)

| Status | `error_code` | `error_reason` | Cause / event |
|---|---|---|---|
| 413 | `body-too-large` | - | Declared body over 1 MiB (before the gate) |
| 503 | `auth-config-unavailable` | `auth_config_unavailable` | Authentication setting unreadable |
| 403 | `invalid-origin` | `origin_mismatch` | Write whose `Origin` differs from the public origin; `security.invalid_origin` |
| 403 | `invalid-origin` | `origin_missing` | Write without `Origin`; same event |
| 403 | `invalid-origin` | `request_header_missing` | Right `Origin`, `X-Julius-Request` is not `1`; same event |
| 403 | `demo-mode` | `demo_blocked` | Write blocked by demo mode |
| 401 | `unauthorized` | `missing_session` | No session cookie |
| 401 | `unauthorized` | `invalid_session` | Unknown, expired, revoked or malformed cookie |
| 503 | `auth-unavailable` | `auth_unavailable` | Session store failure (also login, logout, password change) |
| 401 | `unauthorized` | `invalid_credentials` | Wrong password on login (`auth.login.failure`) or wrong current password on `PUT /api/auth/password` (no event) |
| 429 | `too-many-attempts` | `window_exceeded` | 5 attempts per minute per email or 30 per minute per process on login and password change; `security.rate_limited` (`auth_attempts`) |
| 429 | `too-many-attempts` | `busy` | A password hash is already running; `security.rate_limited` (`auth_hashing_busy`) |

Every other `writeProblem(w, status, kind, ...)` in `internal/httpapi` (over
140 kinds, including `auth-disabled`, `invalid-login`, `invalid-password`,
`invalid-auth-config`, `account-not-found` and `reporting-fx-unavailable`)
sets only `error_code` to `kind`, the `type` slug of the response body; there
is no reason. Unknown `/api` paths answer 404 JSON without an `error_code`.

### Privacy rules

- **Allowlist.** The line holds only the fields above. Never logged: bodies,
  query strings, raw paths, cookies, any header except `User-Agent` and the
  forwarding headers behind `forwarded_ip`, credentials, emails, tokens,
  authorization codes, `state`, PKCE values, CSRF values, redirect URIs, the
  `Origin` value, `err.Error()` text or request ids supplied by clients.
- **Controlled values.** Codes, reasons, fields and event attributes are
  literals in the code; anything that is not a literal token becomes
  `invalid_value`. Handlers must never pass client text to `SetError`.
- **Categories.** Client-controlled choices (auth method, grant and response
  types, scopes, parameter names) pass through `httplog.Known`, which maps
  anything outside a fixed list to `other`.
- **Sanitization and limits.** Free text (user agent, client name, event
  attributes) has invalid UTF-8, control characters (CR, LF, tab, DEL, C1),
  format characters (bidi, zero-width), line separators and other whitespace
  replaced by a space, runs collapsed and ends trimmed, then is cut to: user
  agent 200 runes, client name 100, `client_id_prefix` 8, route 128, event
  attributes 64. A hostile value therefore cannot add a line or a field.
- **Addresses.** `peer_ip` is the TCP peer. Proxy headers feed only the
  unverified `forwarded_ip` and have no effect on rate limits or responses.
- **Request ids.** Generated by the server, absent from every request and
  response header, so a client cannot choose or read one.

### Known limitations

- The `503 MCP request timed out` written by `http.TimeoutHandler` (30 s) has
  no `error_code`: look for `route` `/mcp` with `duration_ms` close to 30000.
  A client that disconnects before the answer on `/mcp` or `/oauth/*` is also
  logged as 503, because `TimeoutHandler` answers 503 for any cancellation. The
  handler it abandoned may set `error_code` after the line is written, so that
  code can be missing.
- Elsewhere a handler that returns without writing (for example after noticing
  that its client is gone) is logged as `200` with `response_bytes` 0.
- For `HEAD`, `response_bytes` counts what the handler wrote, although none of
  it is sent.
- Requests that `net/http` rejects while parsing (for example a header value with
  a control character, or a malformed request line) are answered with 400 before
  any handler runs, so they produce no `http_request` line.
- A hijacked connection is logged as `101`; bytes sent over it are not counted.
- A panic is logged as `500` with `error_code` `handler_panic` (or
  `handler_aborted` for `http.ErrAbortHandler`) even on routes that are normally
  skipped, then re-raised.
- Behind a proxy or tunnel `peer_ip` is the proxy; `forwarded_ip` is what the
  proxy claims and is not verified.
- The standard `log` package writes plain-text startup and background lines to
  the same stream, so consumers must tolerate non-JSON lines.
- The concurrency tests have not been run under `-race` (it needs cgo); run
  `go test -race ./internal/httplog/ ./cmd/julius/` where cgo is available.

Tests: `internal/httplog` (fields, sanitization, noise, writer, concurrency),
`internal/httpapi/logging_*_test.go` and `internal/mcp/logging_*_test.go`
(per-endpoint catalog, events, secrets) and `cmd/julius/logging_test.go`
(the assembled server over real connections: route coverage, registration
diagnosis, request-id correlation, origin handling, secret sweep, unchanged
responses, cancellation and graceful shutdown).
