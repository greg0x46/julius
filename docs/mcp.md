# Self-hosted MCP setup and interoperability

Julius exposes generic MCP in its existing binary. No AI provider, model API
key, Pluggy account or external identity provider is needed. An MCP client may
send data it receives to its own AI provider; review that client's data policy.

**Current boundary:** this checkout implements OAuth and MCP transport only.
`tools/list` returns `[]`; no financial read or write tool exists. The first
financial milestone is intended to be read-only, but authenticated financial
reads and their contracts cannot be validated until the dependent tools land.
The smoke tests below establish transport/auth interoperability, not completion
of that financial milestone. Do not use a successful connection as evidence
that financial reads are available.

## Deployment

1. Follow [installation and authentication](../README.md#authentication) to
   build/install Julius, configure its database and master key, and initialize
   the owner. Enable authentication in settings. Use a separate disposable
   database and synthetic owner when testing; do not use demo mode, which
   deliberately disables authentication and refuses MCP enablement.
2. Set `JULIUS_PUBLIC_URL` to the externally reachable HTTPS origin, without a
   path or trailing slash, and `JULIUS_MCP_ENABLED=true`; restart Julius. Keep
   the master key configured as for an ordinary deployment. MCP defaults to
   disabled: `/mcp`, OAuth and discovery return 404 until enabled.
3. Terminate TLS with a trusted certificate at your reverse proxy. Forward
   `/mcp`, `/oauth/*`, `/.well-known/oauth-authorization-server` and
   `/.well-known/oauth-protected-resource[/mcp]` to the same Julius instance.
   Preserve Authorization, Content-Type, Accept and MCP protocol headers and
   response status/headers. Do not cache responses or log credentials, token
   request bodies or callback query strings. Allow at least the server's
   30-second request timeout. Do not put an interactive proxy login in front
   of the MCP/discovery/token routes; Julius performs its own authorization.
4. From the client machine, verify discovery reports the configured public
   origin as issuer and that origin plus `/mcp` as resource. Forwarded headers
   never change those URLs. HTTP is supported only on loopback for development.
5. Configure the client's **Streamable HTTP** endpoint as that exact `/mcp`
   URL. Clients must support Dynamic Client Registration, public-client
   authentication (`none`), authorization code, S256 PKCE and the OAuth
   `resource` parameter at authorization and token exchange.

The owner enters Julius credentials only on the consent page at the configured
Julius origin. Check the displayed client name (self-reported), client ID,
redirect URI, resource and scope. Choose **Negar acesso** to deny, or enter fresh
owner credentials and choose **Autorizar acesso** to approve. Browser login does
not imply consent. The only scope is `mcp:access`; it grants transport access,
with no financial tools in this version. There are no settings secrets or bank
credentials in the token.

Access tokens last one hour, with no refresh tokens. Reconnect and authorize
again after expiry. To revoke one token, the client sends a form-encoded
`POST /oauth/revoke` with `client_id` and `token`, without an Authorization header
(public-client authentication is `none`). Repeating revocation is harmless.
Password change/reset revokes all OAuth grants; browser logout does not. Until
there is an owner grant-management UI, use the client revocation function or
password reset. Do not assume removing a client locally revokes its token.

## Repeatable automated smoke test

Requires Go from `go.mod`, Python 3.10+ with venv/pip and Node 24/npm. From the
repository root, install test-only clients outside the checkout:

```sh
python3 -m venv /tmp/julius-mcp-client
/tmp/julius-mcp-client/bin/pip install -r internal/mcp/testdata/requirements.txt
npm install --prefix /tmp/julius-mcp-inspector --ignore-scripts --no-audit --no-fund @modelcontextprotocol/inspector@0.21.1
export JULIUS_MCP_TEST_PYTHON=/tmp/julius-mcp-client/bin/python
export JULIUS_MCP_TEST_INSPECTOR=/tmp/julius-mcp-inspector/node_modules/.bin/mcp-inspector
go test -p 1 -count=1 -v ./internal/mcp ./cmd/julius
```

SQLite runs without additional configuration. To include PostgreSQL, set
`JULIUS_TEST_POSTGRES_DSN` to a **disposable** PostgreSQL database first, as for
all repository database tests. Never use a development/production database.
CI provisions PostgreSQL and both external clients before its ordinary
`go test -p 1 -count=1 ./...`, so external-client tests do not skip there.
Locally, an unset client executable variable explicitly skips its test;
a missing executable or dependency when configured fails the test.

Every test creates synthetic credentials, temporary migrated storage and an
HTTP server on an ephemeral loopback port. It closes the server and removes
test data afterward. No running Julius instance or bank connection is needed.

| Client/check | Expected result |
| --- | --- |
| Go SDK (`go.mod`), `TestProtocolLifecycle` | Initialize, empty discovery, unknown tool rejected, revocation denies further discovery on an existing connection; both dialects |
| Independent [Python SDK](https://github.com/modelcontextprotocol/python-sdk/tree/v1.26.0), `TestPythonClientInterop` | HTTP discovery, registration, denial, unknown-scope rejection, approval, S256 exchange, code replay denied, SDK initialize/discovery, unknown call rejected, revoked token denied; both dialects |
| [MCP Inspector CLI](https://github.com/modelcontextprotocol/inspector/tree/0.21.1), `TestInspectorClientInterop` | Authenticated Streamable HTTP initialize and empty tool discovery; both dialects |
| OAuth/access boundary tests | Missing/expired/wrong-resource/revoked tokens, insufficient scope, CSRF, owner reset, open mode and browser/REST separation rejected; both dialects |

Validation record (2026-10-28): Go SDK 1.8.0, Python SDK 1.26.0 and Inspector
CLI 0.21.1 passed the checks above on SQLite and PostgreSQL 17. The full Go
suite, frontend lint/typecheck and 907 unit tests, six Playwright journeys,
embedded-frontend build, release-script tests, production npm audit and
reachable-code vulnerability scan passed. Interactive Inspector OAuth and
remote AI applications remain manual checks, unverified in this run.

Inspector uses the TypeScript SDK; the Go and Python SDKs are independent
implementations. The Inspector CLI test supplies a synthetic token issued by
the real Julius consent/code flow; the Python test also exercises that flow
over HTTP. None invokes a financial tool because none is registered. When the
read tools land, extend these checks with synthetic financial fixtures and an
actual read/result assertion in both independent clients, including denied
scope and revoked access to that read.

Python and Inspector are development/CI dependencies only. The release remains
a single Go binary with embedded frontend; neither client is installed or
executed by Julius.

## Manual consent and remote-client checks

Use a disposable deployment following the steps above and a synthetic owner.
These checks cover browser consent and your actual proxy/TLS/network path,
which loopback automated tests cannot validate.

1. Start `npx @modelcontextprotocol/inspector@0.21.1` on your workstation. Keep
   the Inspector proxy local and its own authentication enabled. Select
   Streamable HTTP and your public `/mcp` URL. Use its OAuth authorization
   controls with the discovered metadata; register the exact redirect URI it
   displays, including the port. Julius accepts HTTPS or HTTP loopback
   redirects, with exact matching and no wildcard ports.
2. Authorize once with **Negar acesso**. Expect `access_denied` and no token.
   Repeat, review details and approve with the synthetic owner's credentials.
   Expect initialize and **List Tools** to succeed with an empty list. If a
   particular Inspector version cannot send the required resource or DCR
   fields, record that as a client compatibility failure; do not weaken Julius
   checks. The CLI test above isolates transport from interactive OAuth.
3. Revoke through the client, or submit the form-encoded revocation request
   described above. Retry using the old token: expect 401. Reauthorize, reset
   the synthetic owner's password and retry: expect 401 again.
4. Repeat with another remote MCP application that supports the listed OAuth
   requirements. Record client/version, Julius commit, database dialect,
   protocol negotiated, allow/deny/revoke results and tool discovery result.
   Keep tokens, passwords, callback codes and financial content out of reports.

ChatGPT is an example remote MCP client, not a required runtime or identity
provider. Client account/plan/admin policies, network access to private hosts,
and support for DCR/resource parameters may limit any remote application's
ability to connect. Consult that client's current documentation separately;
Julius does not require a ChatGPT account or OpenAI API key. An empty tool list
may prevent some applications from exposing a connected server to their UI.
Do not report those clients as verified without completing their manual check.

## Errors and troubleshooting

| Symptom | Check/action |
| --- | --- |
| 404 on MCP/discovery | Enable MCP and restart; ensure proxy routes reach Julius rather than the SPA. `/mcp/` is intentionally different from `/mcp`. |
| 401 with Bearer challenge | Discover metadata and authorize; check expiry, revocation, owner password reset and exact resource. Cookies cannot authorize MCP. |
| 403 / `insufficient_scope` | Token must contain `mcp:access`; unknown requested scopes are rejected with OAuth `invalid_scope`. Obtain a new grant. |
| 403 / `access_denied` | Owner denied consent, authentication is off, or browser Origin/CSRF binding failed. Keep auth enabled; use the exact configured origin, allow the consent cookie and restart authorization in the same browser. |
| `invalid_client` / redirect rejected | Use DCR with `none`, no Authorization header/client secret at token/revoke endpoints, and the exact registered callback including loopback port. Client ID Metadata Documents are unsupported. |
| `invalid_grant` | Start fresh authorization; codes are single-use, last five minutes and bind client, redirect, resource, PKCE and owner revision. |
| Connection fails only remotely | Check DNS, trusted TLS, firewall reachability from the client and proxy forwarding; private loopback URLs are not remotely reachable. |
| Browser CORS failure / unexpected Origin | Direct cross-origin browser API access is unsupported. Use a native/server-side MCP client or local Inspector proxy that does not forward its browser Origin. Do not disable Origin checks. |
| GET/DELETE 405 | Select stateless Streamable HTTP with POST and JSON responses; legacy SSE transport, persistent sessions and resumption are unsupported. |
| 429 | Respect Retry-After; there are 16 concurrent MCP slots, OAuth/credential rate limits and a registration cap. |
| 413 / timeout | MCP bodies are limited to 1 MiB and requests to 30 seconds; OAuth bodies to 16 KiB. |
| 503 | Check local server/database health and server logs without publishing secrets; errors intentionally omit storage internals. |
| Connect succeeds, no tools | Expected in the transport/OAuth milestone. No financial reads, writes, imports, sync, prompts or resources are exposed. |

Other unsupported flows: refresh tokens, implicit/password/device grants, OIDC,
confidential client secrets, registration management and browser CORS. Turning
authentication off rejects existing MCP tokens with 403. MCP cannot be enabled
in demo mode. Security defaults and precise limits also appear in
[README MCP](../README.md#mcp).
