# MCP tool reference

Follow [MCP setup and authorization](../guides/mcp.md) before using these tools.
For transport and client checks, see [MCP interoperability](../mcp.md).

The `mcp:access` scope permits the read-only financial, planning and operational tools below.
Financial read pages contain at most 200 items (default 50); results carry the HTTP status and the API payload in `data`.
Request `scope=mcp:access mcp:write` for financial writes; the owner may grant
less than requested on the consent page, and existing read-only grants gain no
write permission. Write consent permits preparing changes,
while each change still requires fresh owner approval in Julius.
Consent explicitly discloses financial reads. Existing grants with this scope
also permit these reads; revoke them if that access is no longer wanted.
Unknown scopes are rejected; missing required token scope returns 403 with an
`insufficient_scope` challenge. The SDK handles protocol negotiation, typed
tool discovery, JSON-RPC errors and the legacy initialize handshake.

| Tool | Inputs and result |
|---|---|
| `list_accounts` | Optional `page`, `page_size`; accounts and cards with native balances, credit limits and closing-day metadata |
| `list_card_bills` | Required `account_id`, optional `page`, `page_size`; recorded card bills (the open bill is the account balance, never fabricated) |
| `search_transactions` | Required `timezone`; REST query filters, inclusive `YYYY-MM-DD` ranges, `group_by` (`none`, `day`, `week`, `month`, `year`); page totals cover all matches, with explicit eligibility, native totals and reporting coverage/FX |
| `list_categories` | Optional `page`, `page_size`; catalog including inactive categories |
| `spending_by_category` | Required `timezone`; the REST category-breakdown contract (period, classification) with reporting coverage and FX |
| `get_investment_portfolio` | Required `to`, optional `from`, `group_by` (`none`, `account`, `class`), `source` (`manual`, `synced`), `account_id`; positions, period returns, portfolio/group totals, native figures and FX |
| `get_net_worth` | Required `from`, `to`; realized cash plus investments, daily history within the range and today's `latest` |
| `list_goals` | Optional `limit`, `offset`; current progress, target-date projection and required monthly contribution in BRL |
| `list_payables` | Optional `kind` (`debt`, `receivable`), `limit`, `offset`; actual settled/remaining amounts in BRL, excluding card bills |
| `list_recurring_commitments` | Optional `limit`, `offset`; active and inactive BRL schedules |
| `list_scenarios` | Optional `limit`, `offset`; scenario IDs, kinds and activation flags; discovery applies nothing |
| `list_projection_events` | Required `from`, `to`, optional `scenario_ids`, `limit`, `offset`; dated BRL installments/occurrences with realization state |
| `get_timeline` | Required `from`, `to`, `reference_date`, optional IANA `timezone`, `scenario_ids`; daily cash, income/expense totals, certainty tiers and reporting coverage |

All dates are inclusive calendar days (`YYYY-MM-DD`). Net-worth, event and
timeline windows allow at most 366 days. Net worth cannot extend into the
future; portfolio `to` is clamped to today and echoes `to_clamped`. Portfolio
`from` may be omitted for inception; `account_id` limits displayed positions,
not portfolio totals. List pages default to 50 items (maximum 100) and return
`next_offset`; offset pages should be read while the underlying data is stable.
Results are limited to 256 KiB; oversized results return a tool error asking
for a smaller page, date range or account scope, never silently truncated totals.

Hypothetical standalone scenarios require explicit `scenario_ids` (maximum 20),
even when marked active. Omitting IDs or passing `[]` includes only active
non-standalone commitments. Selected IDs add to those commitments; unknown IDs
are rejected. This explicit MCP selection differs from the application's default
timeline, which includes all active scenarios. Investments, wealth, goals and
payable balances never include hypotheticals.

Tools reuse Julius's domain engines and application formatters. Monetary values,
quantities, rates and ratios are decimal strings. Portfolio and wealth reports
use the configured reference currency and carry FX provenance; goals, payables,
schedules and projected events remain BRL. Unavailable portfolio figures are
null with reasons and partial totals are flagged. Unavailable historical wealth
days have a null `breakdown`; an unvaluable current day fails the tool. Timeline
FX errors carry native totals and coverage issues (currency, account, date and
reason). Errors disclose no database details. Tool descriptions/results state
calculation assumptions, including Modified Dietz, historical FX, zero assumed
goal appreciation when history is too short, and current FX across projections.
Financial writes require the separate approval flow below. No chats or inference calls are exposed.

In **Configurações → Segurança → Clientes MCP**, inspect each connection's
self-reported name, client ID, granted capabilities, authorization time, last
use (if any), expiry and revocation. Multiple connections are independent,
including separate authorizations of the same registered client. Consent
offers only the tiers the client requested (read-only, plus writing, plus
operations) and defaults to read-only. Reduce a connection to a narrower tier
or restore it within the original consent ceiling; widening beyond what was
consented requires a new authorization. A downgrade also applies to tokens
already issued. **Revogar acesso** invalidates only that connection, including
outstanding codes. Neither tokens nor their hashes are shown in settings.

The **Permitir acesso MCP** switch blocks further MCP calls and OAuth requests
without restarting or disabling browser login. Disabling the server environment
flag still removes all MCP/OAuth routes (404) and requires a restart; settings
cannot override it. Switching MCP off pauses access rather than revoking grants:
unexpired authorizations resume if enabled again. Every tool call rechecks the
persisted authorization, so revocation, disabling and downgrades affect further
calls; calls already executing may finish. Open-browser mode and demo mode
cannot manage authorizations or bypass access.

Consent requests and authorization codes expire after five minutes; access
tokens expire after one hour, without idle extension. A connection's maximum
lifetime is 65 minutes from consent, bounding code exchange plus token lifetime;
its token may expire earlier. Last use records successful credential validation
on MCP requests, including discovery, and never extends expiry. Only SHA-256
hashes of random credentials are stored. Grants and lifecycle history survive
restarts; existing pre-upgrade tokens/codes without a connection must reauthorize.
Clients revoke their connection with form-encoded `POST /oauth/revoke`
(`client_id` and `token`); repeating this is harmless and a different client's
token cannot be revoked through that request.

Password change or `julius auth reset-password` revokes every MCP connection and
invalidates all OAuth codes/tokens atomically with the password change. Revision
checks also reject issuance racing a reset. Browser logout only revokes that
browser session; it does not revoke MCP access. Expired, revoked or wrong-audience
tokens return 401; storage failures return 503 without database details. Back up
the database and encryption master key using the same guidance as other
credentials, and authorize only clients whose identity and requested
capabilities you recognize.

Supported methods are public-client registration and authorization code with
S256 PKCE. Client ID Metadata Documents are **not supported or advertised**;
clients must support Dynamic Client Registration. There are no client secrets,
refresh tokens, implicit/password grants, device flow, OIDC identity tokens,
registration-management API or browser CORS support. The consent page works
without JavaScript, requires a bound anti-CSRF cookie and matching Origin, and
cannot be framed. OAuth endpoints allow 120 requests/minute per instance and
registration its own 20/minute; consent and write approval allow five failed
credential checks per minute (a success never counts) and one concurrent
password hash. At most 1,000 clients may be registered per database; a full
registry makes room by removing the oldest client that never completed an
authorization, and clients with any authorization are never removed. These
limits protect a small self-hosted instance; an untrusted public deployment
also needs edge abuse controls. Application names are self-reported, not verified identities.

Self-hosting requires a reachable HTTPS `JULIUS_PUBLIC_URL` with a valid
certificate; HTTP is allowed only on loopback for development. Keep discovery,
`/oauth/*` and `/mcp` reachable at that origin through the TLS reverse proxy.
Issuer and resource URLs use this configured origin, never forwarded headers.
An Origin header on MCP, when supplied, must match it. Keep tokens, callback
codes and passwords out of logs and URLs you share. The encryption master key,
Pluggy credentials and browser cookie are never issued to MCP clients.


## Financial writes

Write tools prepare an immutable preview; they never commit on an MCP call.
All require both `mcp:access` and `mcp:write`. Inputs are an optional `id` UUID
(required for changes to an existing record; integrated investment accounts use
`integrated:<source UUID>`), a typed `data` object with the
body fields below, and `idempotency_key` for manual transaction creation.
Discovery publishes the body fields and exact string amount schema.

| Tool | `data` fields |
|---|---|
| `assign_transaction_category` | `category_id` (active UUID); includes every sibling installment in the preview |
| `set_transaction_inclusion` | `state`: `considered` or `ignored` |
| `create_manual_transaction` | `account_id`, `description`, signed decimal-string `amount`, calendar `occurred_at` (`YYYY-MM-DD`), optional `category_id`; also requires top-level `idempotency_key` |
| `update_manual_transaction` | Same body as creation, without an idempotency key; manual entries only |
| `delete_manual_transaction` | `{}`; manual entries only, soft delete |
| `create_category` | `name`, `kind` (`expense`, `income`, `transfer`), `icon`, `color` from the Julius catalog palette |
| `update_category` | At least one of `name`, `is_active`, `icon`, `color`; deactivate instead of delete |
| `create_automation_rule`, `update_automation_rule` | `name`, `is_active`, `logic_operator`, `conditions`, `actions` as in `/api/automation-rules`; `apply_retroactively` must be omitted or false |
| `delete_automation_rule` | `{}`; previous category/inclusion decisions remain |
| `create_goal`, `update_goal` | `name`, `metric` (`invested_value`), positive BRL `target_amount`; optional `target_date` (`YYYY-MM-DD` or null), `notes`; updates replace the goal fields |
| `delete_goal` | `{}`; removes the target without changing investments |
| `create_scenario`, `update_scenario` | `name`; standalone hypotheticals only; updates rename without changing activation |
| `delete_scenario` | `{}`; removes the standalone scenario, its planned transactions and realization links, preserving real entries |
| `create_planned_transaction`, `update_planned_transaction` | `description`, signed BRL `amount`, `projected_at` (`YYYY-MM-DD`), optional `category` UUID; requires top-level `scenario_id`, plus `id` for an existing planned transaction |
| `delete_planned_transaction` | `{}`; requires top-level `scenario_id` and planned transaction `id`; preserves real entries |
| `create_recurring_commitment`, `update_recurring_commitment` | `name`, `kind` (`income`/`expense`), positive BRL `amount`, `category_id`, `cadence` (`monthly`/`annual`), `day_of_month` (1–31), `start_date`, `is_active`; optional `account_id`, `end_date`, `month_of_year` (required for annual); updates replace schedule fields |
| `delete_recurring_commitment` | `{}`; removes schedule/reconciliation decisions, preserving real entries; remove any targeting automation reconcile action first |
| `create_investment_account` | `name`; optional `kind` (`manual`), `currency_code` (`BRL`), `financial_account_id` UUID; provider accounts come from sync |
| `update_investment_account` | Optional `name`, `active`, `financial_account_id` (null unlinks); integrated custody accepts only the bank link |
| `delete_investment_account` | `{}`; manual custody without positions or operations only |
| `create_investment_position` | `account_id` (UUID or `integrated:<source UUID>`), `occurred_on` (`YYYY-MM-DD`), `asset_id` UUID or `name`; optional `ticker`, `asset_type`, `initial_quantity`, `initial_unit_cost`, `initial_value`, `notes` |
| `update_investment_position` | Optional `name`, `ticker`, `asset_type`, `notes`; manual holdings only; instrument descriptions are shared across holdings of that asset |
| `delete_investment_position` | `{}`; manual position without operations only |
| `create_investment_operation`, `update_investment_operation` | `account_id`, `kind` (`initial_balance`, `deposit`, `withdrawal`, `buy`, `sell`, `income`, `fee`, `tax`, `valuation`), `occurred_on` (`YYYY-MM-DD`); optional `position_id`, `amount`, `quantity`, `unit_price`, `fees`, `taxes`, `notes`, as allowed by the investment ledger. Updates replace operation fields. Without `amount`, quantity × unit price supplies the gross amount. |
| `delete_investment_operation` | `{}`; manual operation without reconciliations or a custody-transfer pair only |
| `create_manual_account` | `name`; optional ISO `currency_code` (default `BRL`), decimal-string `opening_balance` (default `0`), `archived` (default false); manual bank accounts only |
| `update_manual_account` | All of `name`, `currency_code`, `opening_balance`, `archived`; replaces account fields. Currency/opening balance cannot change after any movement or adjustment |
| `adjust_manual_account` | Signed nonzero native `amount` and `description` (maximum 200 characters); top-level `id` is an active manual account; adjusts balance without income/expense |
| `create_transfer` | Either `source_transaction_id`, `destination_transaction_id`, optional `fee_transaction_id` for existing real movements, or `source`, `destination`, optional `fee` containing manual transaction fields without categories. New movements require active manual accounts; amounts are in each account's currency |
| `delete_transfer` | `{}`; top-level `id` is the transfer ID; unlinks without deleting movements |
| `create_payable` | `kind` (`debt`/`receivable`), `name`, positive BRL `total_amount`; optional `initial_remaining_amount` between zero and total |
| `update_payable` | `name`, positive BRL `total_amount`; kind and initial settled amount remain unchanged |
| `delete_payable` | `{}`; deletes payable, plans and links, preserving real transactions |
| `create_payable_link` | `payable_id`, `transaction_id`; one payable per transaction, eligible BRL outflow for debt or inflow for receivable |
| `delete_payable_link` | `payable_id`; top-level `id` is the link ID; removes dependent settlements/allocations, preserving real transactions |
| `get_write_result` | Top-level `request_id`; returns approval state and the final REST status/body |

Investment monetary fields, quantities and prices are plain decimal strings (at
most 64 characters, no exponent or JSON numbers). Manual records remain BRL,
including local positions within integrated custody; a foreign-currency asset
cannot receive a manual holding. Synced holdings and operations retain their
native data and currencies. Integrated custody permits local buys/sells only
for its manual holdings, and cash-only contribution/withdrawal/income/fee/tax
records without changing provider cash. Transfers, reconciliation writes and
asset-catalog management are not exposed by these tools.

Investment approval captures the relevant custody ledgers, positions and links;
up to 500 positions plus operations per affected custody, also subject to the
256 KiB preview limit. The approval recheck and mutation share a serializable
transaction. Invalid history (including negative cash or quantity) rolls back
entirely. Replaying the same approval applies it once; a new tool call prepares
a new owner approval, not a persistent investment creation idempotency key.

For example, `create_manual_transaction` accepts:

```json
{"idempotency_key":"synthetic-entry-001","data":{"account_id":"<account UUID>","description":"Synthetic groceries","amount":"-42.501","occurred_at":"2026-10-01"}}
```

1. Review the returned `preview`: affected identifiers/count, current objects,
   requested fields and effects on reporting, balances and financial links.
   Transfer-unlink previews include the other participants and exact native amounts;
   those entries survive and resume their category/inclusion reporting decisions.
   New records have no identifier until commit; account/obligation previews also
   list existing accounts, linked transactions and dependent plans that are affected.
2. Open `approval_url` in the owner's browser. The Julius page identifies the
   requesting client, shows the exact request and effects, and requires fresh
   owner credentials plus **Aprovar e aplicar alteração**. Never give the owner
   password to the MCP client. A prompt instruction or a client boolean cannot
   approve a write. Denial needs no password.
3. Call `get_write_result` with `request_id`, using the exact original access
   token. `pending` does not change data; `applied`, `denied` and `failed` are
   terminal. A successful approval POST executes once, including concurrent
   retries. Changed preview data requires a new preview and approval.

Manual transaction creation keys use the existing persistent owner idempotency
contract: 8–128 printable ASCII characters without whitespace/commas; retry the same normalized
payload with the same key, including after losing a response. A different
payload with that key returns a 409 Problem. Domain failures retain sanitized
REST Problems. A 503 can mean the write committed but its result could not be
read back: inspect Julius before retrying; always reuse the creation key.

Manual operations refuse provider entries, archived accounts and prohibited
investment/transfer links just like the UI. Category propagation uses the domain
installment grouping, capped at 100 identifiers, and rechecks the preview in the
same serializable transaction as all decisions/events. Scenario deletion also
captures planned children and realization links and rechecks its preview inside the deletion transaction. Arbitrary batches and retroactive
rule application are unavailable.
Rules affect future arrivals/changed sync entries, respecting manual decisions.
Ignoring cleans up payable, recurring and investment links without changing bank
cash. Manual creation shares learned categorization, automation and exact money
with the UI; updates/deletes retain the same service restrictions.

Bodies are limited to 16 KiB; previews to 256 KiB; names to 200 characters,
descriptions/condition values to 1,000, rule conditions to 20 and actions to 10.
Amounts are plain decimal strings of at most 64 characters (no exponent).
Transaction and adjustment amounts must be nonzero; opening balances and initial
remaining amounts may be zero.
Goal notes are limited to 4,000 characters. Approvals are instance-local, capped at 100, expire after five minutes and are
lost on restart; final execution results remain for ten minutes. Use the same
instance for tool calls and browser approval. Token revocation, expiry, password
reset or disabling authentication prevents pending approval. Browser requests
require same-origin POST and an HttpOnly/SameSite CSRF cookie; ordinary MCP calls
cannot supply approval. There is no HTTP proxy or SQL tool.

To validate in dev with synthetic data, create a manual BRL account in Julius,
authorize a client with both scopes, prepare the example with that account UUID,
and check that the account is unchanged until browser approval. Poll the result,
repeat creation with the same key (one entry), categorize it, then update and
delete it with separate approvals. A read-only client must fail every write;
editing/deleting a synced entry must fail. Deny a request and verify it remains
unchanged. These steps need no live provider credentials; synced restrictions
and rollback faults are also covered by the integration tests.

Account, transfer and payable writes recheck the entire preview inside the same
serializable transaction as every mutation. Linking existing movements preserves
provider balances and user decisions. Transfers exclude native principal from
income/expense, retain both cash effects and keep explicit fees separate; no FX
spread is inferred. Unlinking restores ordinary category/inclusion reporting.
Payable totals remain derived and BRL-only. No provider settings or secrets are
writable through these tools. Approval POST retries execute once. A new preparation
and approval is a new operation: after a lost result or restart, inspect Julius
before preparing another account, adjustment, manual transfer or payable creation.

To validate account and obligation writes in dev, use a synthetic owner and a
client with both scopes; no provider connection is required:

1. Prepare/approve `create_manual_account` twice with distinct synthetic names,
   `currency_code: "BRL"` and `opening_balance: "1000"`; retain the returned IDs.
   Rename one through `update_manual_account`, explicitly supplying all four fields.
2. Prepare/approve `adjust_manual_account` with `amount: "0.001"` and a reason.
   Check the native balance increases exactly once, with no income/expense entry.
3. Prepare `create_transfer` with `source` amount `"-50.001"`, `destination`
   amount `"50.001"`, and `fee` amount `"-1"` on the source account; each includes
   account ID, description and `occurred_at: "2026-10-01"`. Before approval nothing
   changes. After approval, both balances move, principal is excluded from reports
   and only the fee is an expense. Unlink using `delete_transfer`; all entries remain.
4. Create a `debt` payable for `"100"`, link the BRL debit using
   `create_payable_link`, and compare remaining amounts with Pendências. Unlink it
   with its returned link ID and `payable_id`; remaining returns to `"100"`.
   Edit and delete the payable through separate approvals; real entries remain.
5. A read-only client must fail these writes. Deny an adjustment and verify its
   balance is unchanged. Wrong directions, unknown accounts, unequal same-currency
   transfer amounts and foreign-currency payable links must fail without mutation.
   Test foreign-account cash separately: USD movements retain USD; no inferred FX.

Planning writes use the same goal and recurrence validation/derived states as the
application. Active recurring commitments enter base forecasts and reconciled
occurrences are suppressed. Schedule edits release manual links/detachments whose
dates cease to be occurrences. Standalone scenarios remain hypothetical: even an
active one enters MCP timeline/events only through explicit `scenario_ids`.
Standalone planning writes never change goal progress, realized totals or account balances.

To validate planning writes in dev, use a synthetic owner, a client authorized for
both scopes and an expense category from Julius; no bank connection is required:

1. Prepare `create_scenario` with `{"data":{"name":"Synthetic vehicle"}}`.
   Approve in Julius, poll `get_write_result`, and keep the returned scenario ID.
2. Prepare `create_planned_transaction` with that top-level `scenario_id` and
   `data` containing `description`, `amount: "-20000.001"` and a future
   `projected_at`. Approve and poll. Call `get_timeline` for that date range
   with `reference_date` today: compare omitted IDs with `scenario_ids: [id]`.
   Only the selected simulation should include the planned expense.
3. Rename and then delete the scenario with separate approvals. Denying deletion
   must retain it; approving must remove its planned events and preserve all real
   transactions. A changed preview requires a new request.
4. Create/edit a goal (`invested_value`, positive `target_amount`, optional future
   `target_date`) and compare `list_goals` with Metas. Create/edit an active monthly
   recurring expense with the category ID and compare its schedule/occurrences with
   Recorrências and `list_projection_events`. Derived progress, calendar clamping
   and reconciliation must match. Delete each with separate approvals.
5. A read-only client must fail these writes. Invalid decimal strings/dates,
   stale IDs and planned transaction IDs belonging to another scenario must fail
   without mutation. Recurrence deletion must fail while targeted by automation.

The official [Go MCP SDK](https://github.com/modelcontextprotocol/go-sdk) serves
stateless Streamable HTTP with JSON responses: 1 MiB bodies, 30 seconds and
16 concurrent requests (429 when busy). OAuth request bodies are limited to
16 KiB. GET/DELETE MCP streams are unsupported (405); responses use `no-store`.
SIGINT/SIGTERM drains HTTP for up to 35 seconds before forced closure. There
are no stored chats, inference calls or provider-specific integrations.

To validate in dev with a synthetic owner and no bank connection:

1. Leave MCP disabled and verify `/mcp` and discovery return 404. Enable it and
   restart; verify unauthenticated `/mcp` returns 401 with resource metadata.
2. Connect a client supporting Dynamic Client Registration, follow the consent
   page, and choose **Negar acesso**: no access token should be issued.
3. Connect again, review the details, enter the synthetic owner's credentials
   and choose **Autorizar acesso** (read-only is preselected). Discovery should list the eight financial read tools, operational tools and guarded financial write tools.
   Call `list_goals` with `{}` and `get_timeline` with today's `from`, `to` and
   `reference_date`; empty synthetic data should return empty lists/zero BRL
   figures. With a synthetic investment and dated goal, compare portfolio and
   goal figures with Investimentos and Metas. Create a hypothetical scenario:
   it must affect only calls selecting its ID, including if marked active.
4. With `mcp:access mcp:write`, prepare and approve `create_investment_account`
   with a synthetic name, then `create_investment_position` in that custody
   with a synthetic asset name and today's `occurred_on`. Approve a `deposit`
   of `100`, then a `buy` of quantity `3` at unit price `10`, fees `0.1` and
   taxes `0.2`, then a `sell` of quantity `1` at `12`, fees `0.2` and taxes
   `0.3`. Inspect Investimentos: quantity is `2`, cost is `20.2` and cash is
   `81.2`. Record a `valuation` of `24` for the position and compare
   `get_investment_portfolio` with the page: yield is `5.2`, costs `0.8`.
   Before valuation, unavailable period values follow the existing quote
   coverage rules. Reject a prepared deletion: the operation remains. Approve
   deletion of the sale and replay that approval: it applies once. An attempted
   sale of more units than held must fail without changing any record.
   Provider ownership can additionally be checked with synthetic synced fixtures
   in the automated tests; no live provider credentials are needed.
   Pace approvals within the existing limit of five owner login attempts per
   minute; wait for the next minute after a 429 response.
5. Revoke the access token through `/oauth/revoke`, then verify the same token
   gets 401. Repeat authorization and reset the synthetic owner's password;
   that token should also get 401. Open mode must reject authorization.

## Operational tools

Authorize with `scope=mcp:access mcp:write mcp:operations` to request network
operations. Existing read-only and financial-write tokens gain no operations
permission. OAuth consent explicitly authorizes maintenance requests without a
new approval per operation; financial changes still require their own approval.
No tool reads credentials, raw provider payloads, environment variables or keys,
or accepts arbitrary commands or configuration changes.

| Tool | Inputs and result |
|---|---|
| `get_operations_status` | `{}`; worker credential readiness, sync/quote daily schedules, quote service availability and last full spot refresh |
| `list_sync_runs` | Optional `limit` (1–100, default 20), `offset`; newest provider sync runs, IDs, status and counters |
| `get_sync_run` | Required `id` UUID; optional `limit`, `offset` page the safe failures, alongside run status; excludes file imports |
| `request_sync` | Required `idempotency_key`; optional active Pluggy `source_id` UUID, omitted for all active connections; returns a receipt and queued run IDs |
| `request_quote_refresh` | Required `idempotency_key`; returns a running receipt immediately; no `source_id` |
| `get_operation_result` | Required `request_id` UUID; poll using the same operations-scoped access token |

Read tools require only `mcp:access`; requests and receipt polling require all
three scopes. Results use `status` and JSON `data`; request receipts add
`request_id`, `state` and an optional `result` or safe `error`. Sync `accepted`
means queued (HTTP result 202), not completed: poll each run with `get_sync_run`.
Busy/inactive/file connections are rejected with the existing 409 policy; busy
connections do not stop other eligible connections. Missing worker credentials
reject the request without queueing. The existing worker owns claims, leases,
recovery and provider retries; no second sync execution path is added.

A quote receipt starts `running`, then becomes `finished` or `rejected`.
`finished` carries the actual spot outcome (`success`, `partial`, `deferred` or
`failed`) and counts; it does not claim upstream success. The shared REST and
scheduler guard prevents overlapping spot refreshes. Provider timeouts, retries,
rate limits and caching remain in force. Accepted quote work survives a client
disconnect but observes server shutdown and a five-minute deadline. History
backfill follows separately. Explicit manual refresh remains available when the
automatic daily quote schedule is disabled; requests never change schedules.

Use an 8–128 character printable ASCII `idempotency_key`, without spaces or
commas. Retrying the same key and arguments returns the same receipt, including
a terminal failure; changed arguments are rejected. A deliberate new attempt
needs a new key. At most 100 receipts are retained per server process for one
hour (running work is retained until it finishes), bound to the original token.
Receipts and their retry protection do not survive restart or move between
instances. If a receipt is unavailable, inspect durable `list_sync_runs` and
`get_operations_status` before requesting again; keep retries on the same
instance. Sync queue uniqueness and quote spot guards retain their existing
scope (database-wide per connection for sync; process-local for quotes).

To validate in dev with synthetic data and no live credentials:

1. Enable MCP/authentication with a synthetic owner and authorize one read token
   and one token with all three scopes. Read operational status/history with
   either. A read token and a financial-write-only token must fail both requests.
2. Without configured Pluggy credentials, `request_sync` must fail without a new
   run. Inactive/file sources and overlapping runs are covered by synthetic
   integration tests; no real bank connection is needed.
3. With no quoted holdings, call `request_quote_refresh` with
   `{"idempotency_key":"synthetic-refresh-1"}` and poll `get_operation_result`.
   It should finish with zero assets without provider traffic. Retry the same
   key: the request ID must remain unchanged. Supplying `force`, credentials,
   an invalid UUID or a quote `source_id` must fail.
4. With synthetic quoted holdings, automated fake-provider tests exercise
   concurrency, upstream outage, rate limits and shutdown. Demo mode must refuse
   MCP startup. No credential is returned by any read or error.

Access management, with a synthetic owner:

1. Open **Configurações → Segurança → Clientes MCP**. Confirm the client ID,
   permissions and dates; authorize a second client and verify both appear.
   Request writing on a new consent and select it explicitly; reduce that
   connection to read-only in settings: its write tools must now fail.
2. Revoke one connection: its token must get 401 while the other still works.
   Turn **Permitir acesso MCP** off: further calls and OAuth requests must get
   403 while browser settings remain usable.
3. Enable MCP again and reset the synthetic owner's password: all connections
   must show as revoked and their tokens must get 401. Open mode must reject
   authorization and connection management.

For troubleshooting, reauthorize when operations scopes are missing; configure
Pluggy only in Julius when `sync_ready` is false. For busy requests, inspect the
active run or wait for spot work to finish. Inspect quote outcome counts rather
than treating `finished` as success. After failure or deferral, use a new retry
key only when a new attempt is intended. MCP transport limits (30 seconds,
16 concurrent requests) also apply to these tools; the async quote deadline is
separate.
