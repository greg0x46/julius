# MCP setup and authorization

Julius serves a provider-neutral Model Context Protocol (MCP) endpoint in its
existing binary. No AI provider, model API key or Pluggy account is required.
Julius's MCP server exposes your Julius data; Pluggy's separate MCP capability
is not this endpoint and does not configure or authorize it. Review your MCP
client's data policy before granting it access to financial information.

## Enable and discover the endpoint

1. Follow the [getting started](../getting-started.md) and
   [authentication setup](../self-hosting/security.md#authentication-and-recovery).
   Initialize the owner account, retain the encryption master key, and keep
   authentication enabled in settings. Open mode rejects MCP access; demo mode
   refuses MCP enablement. Use synthetic data in a disposable deployment for
   connection validation.
2. Configure the server environment, keeping the existing database and master
   key settings, then restart Julius:

   ```sh
   export JULIUS_PUBLIC_URL="https://julius.example.com"
   export JULIUS_MCP_ENABLED=true
   ```

   Replace the example with the externally reachable origin, without a path or
   trailing slash. The Streamable HTTP URL is exactly
   `https://julius.example.com/mcp`, without a trailing slash. MCP and OAuth
   routes return 404 when the flag is unset or false. Invalid flag values
   prevent startup.
3. Use HTTPS with a trusted certificate for remote access. HTTP is allowed only
   on loopback for development. Configure DNS, firewall and reverse proxy so
   both the client and the owner's browser can reach the public origin. Forward
   `/mcp`, `/oauth/*`, `/.well-known/oauth-authorization-server`,
   `/.well-known/oauth-protected-resource` and
   `/.well-known/oauth-protected-resource/mcp` to Julius, along with its web UI.
   Preserve Authorization, Content-Type, Accept, MCP protocol headers and
   response headers/status. Disable caching, allow the 30-second request
   timeout, and avoid an extra interactive proxy login on these endpoints.
   Do not log tokens, passwords, token request bodies or callback query strings.
4. In **Configurações → Segurança → Clientes MCP**, check **Permitir acesso MCP**.
   This owner control defaults to enabled but cannot override the environment
   flag. Disabling it blocks subsequent access immediately without a restart.
5. From the client's network, inspect the public discovery endpoints without
   credentials:

   ```sh
   curl -i https://julius.example.com/.well-known/oauth-protected-resource/mcp
   curl -i https://julius.example.com/.well-known/oauth-authorization-server
   curl -i -X POST https://julius.example.com/mcp
   ```

   The metadata requests should return 200 with JSON. The resource must equal
   `https://julius.example.com/mcp`; the issuer and authorization server must
   equal `https://julius.example.com`. The unauthenticated POST should return
   401 with a Bearer `WWW-Authenticate` challenge pointing to resource metadata.
   Forwarded headers do not override the configured origin.

## Connect a compatible client

Select **Streamable HTTP** in the client and enter the exact public `/mcp` URL.
Use the client's OAuth connection flow with discovery, Dynamic Client
Registration (DCR), public-client authentication (`none`), authorization code,
S256 PKCE and the OAuth `resource` parameter at authorization and token exchange.
Julius uses stateless POST requests and JSON responses; legacy SSE transport,
GET/DELETE streams, persistent sessions and resumption are unsupported.

ChatGPT is an example of a remote MCP client, not a required runtime or identity
provider. This guide does **not** establish verified ChatGPT compatibility.
Client account/admin policies, network reachability and support for the required
registration and resource parameters can prevent a connection. Use the same
requirements for any client; do not weaken Julius authorization to accommodate it.

For existing automated coverage, validation evidence and a repeatable manual
client check, see [MCP interoperability](../mcp.md#repeatable-automated-smoke-test).
That document distinguishes protocol tests from unverified remote applications;
an automated SDK check does not establish compatibility with a particular hosted
client. Validate the actual client/version and deployment before calling it
verified. Keep credentials and financial content out of validation reports.

## OAuth, PKCE and owner consent

The client handles the following flow; the owner only enters credentials on
Julius pages at the configured public origin.

1. Discover resource metadata from the Bearer challenge, then authorization
   server metadata at `/.well-known/oauth-authorization-server`.
2. Register with JSON at `POST /oauth/register`. A minimal explicit metadata
   shape is shown below; replace the callback with the client's exact URI:

   ```json
   {
     "client_name": "My MCP client",
     "redirect_uris": ["https://client.example.com/oauth/callback"],
     "token_endpoint_auth_method": "none",
     "grant_types": ["authorization_code"],
     "response_types": ["code"]
   }
   ```

   Julius returns a generated `client_id`; do not supply one during registration.
   Redirects must use HTTPS, or HTTP on `localhost`, `127.0.0.1` or `::1`.
   They must have no userinfo or fragment and match registration exactly,
   including the loopback port. There are no wildcard ports. A client may also
   list `refresh_token` in `grant_types`; Julius accepts it but registers only
   `authorization_code` and issues no refresh tokens. Registration
   identifies an application, not a new Julius owner, and grants no data access.
3. Open `/oauth/authorize` with `response_type=code`, the registered `client_id`
   and `redirect_uri`, `state`, `scope=mcp:access`,
   `resource=https://julius.example.com/mcp`, `code_challenge` and
   `code_challenge_method=S256`. The challenge is the unpadded base64url-encoded
   SHA-256 hash of the original random PKCE verifier.
4. Review the displayed client name (self-reported, not a verified identity),
   client ID, redirect, resource and permissions. Consent defaults to read-only;
   the owner can grant less than requested. Enter fresh Julius owner credentials
   and choose **Autorizar acesso**, or choose **Negar acesso** to deny. An existing
   browser session never skips explicit consent. The page needs its bound
   anti-CSRF cookie and matching Origin; complete it in the same browser.
5. Exchange the single-use code at `POST /oauth/token`, form-encoded, with
   `grant_type=authorization_code`, `code`, `client_id`, `redirect_uri`,
   `resource` and the original `code_verifier`. Both resource parameters must
   equal the exact public `/mcp` URL. Send no client secret or Authorization
   header. Consent requests and codes expire after five minutes.
6. Send the returned token as `Authorization: Bearer <access_token>` on `/mcp`.
   Access tokens expire after one hour without refresh tokens; authorize again
   after expiry. Browser cookies cannot authorize MCP, and MCP tokens cannot
   authorize the ordinary `/api` routes.

Client ID Metadata Documents are **unsupported**: a client that requires them
instead of DCR cannot register. Julius also has no confidential-client secrets,
implicit/password/device grants, OIDC identity tokens, registration-management
API or browser CORS support. Use a native/server-side client or a local client
proxy; do not disable Origin checks for direct cross-origin browser requests.

## Scopes and financial change approval

| Requested scope | Permission |
| --- | --- |
| `mcp:access` | Transport access and read-only financial, planning and operational tools. |
| `mcp:access mcp:write` | Also prepare financial changes for owner review; no automatic application. |

Existing read grants gain no write permission automatically. For a financial
change, the write tool returns a preview, `request_id` and `approval_url`:

1. Open the approval URL in the owner's browser. Review the client and exact
   effects on the Julius page.
2. Enter fresh owner credentials and choose **Aprovar e aplicar alteração** to
   apply that change, or deny it. Every financial change needs this separate
   approval; a prompt instruction or client boolean cannot approve it. Never
   give the owner password to the MCP client.
3. The client polls `get_write_result` with the request ID and exact original
   access token. `pending` changes nothing; `applied`, `denied` and `failed` are
   terminal. Changed preview data requires a new preview and approval. Pending
   requests expire after five minutes and are lost on restart.

Optional maintenance requests use the additional `mcp:operations` scope, which
requires both access and write scopes. They have separate authorization
semantics; see the [operations reference](../legacy-readme.md#operational-tools).

## Reduce access or revoke a connection

In **Configurações → Segurança → Clientes MCP**, inspect the client ID,
authorization time, permissions, expiry and last use. Each consent creates an
independent connection, even for the same registered client. Use **Permissões**
and **Salvar permissões** to reduce permissions or restore them within the
original consent ceiling. Broader permissions require fresh authorization.

Use **Revogar acesso** and confirm to invalidate that connection and outstanding
codes. Clients can also send form-encoded `POST /oauth/revoke` with `client_id`
and `token`, without a client secret or Authorization header. Repeating revocation
is harmless; a client cannot revoke another client's token this way. Subsequent
calls with the revoked token return 401; work already executing may finish.

Turning **Permitir acesso MCP** off pauses access rather than revoking grants;
unexpired connections resume when enabled again. Revoked or expired connections
cannot be restored. Password change/reset revokes all OAuth connections and
codes. Browser logout, or removing a connection only in the client, does not
revoke Julius access.

## Troubleshooting

| Symptom | Check/action |
| --- | --- |
| 404 on discovery or MCP | Enable the environment flag and restart. Route the exact `/mcp` path and discovery endpoints to Julius, not the SPA. |
| `invalid_client_metadata` | Send one JSON object, no supplied `client_id`, 1–10 redirects, a name of at most 200 characters, and the public-client metadata above. Client ID Metadata Documents and secret-based authentication are unsupported. |
| `invalid_redirect_uri` / `invalid_client` | Check HTTPS or HTTP loopback callback rules and exact registered URI/port. Token/revocation requests must have no Authorization header or client secret. |
| `invalid_scope` | Request `mcp:access`, optionally adding `mcp:write`. Unknown or duplicate scopes and operations without write scope are rejected. Reauthorize for more access. |
| `invalid_target` | Send the exact configured public origin plus `/mcp` as resource in both authorization and exchange; do not use an internal proxy address. |
| `invalid_grant` | Restart authorization: the code is single-use and bound to its client, redirect, resource, verifier and owner revision. Check five-minute expiry and S256 PKCE. |
| 401 Bearer challenge | Discover and authorize again; check token expiry, revocation and password reset. Browser sessions cannot substitute for MCP tokens. |
| 403 / `access_denied` / scope error | Check authentication and the owner MCP switch, granted permissions, and any supplied Origin against the public origin. For consent, allow the CSRF cookie and restart in the same browser. Denied consent grants no access. |
| Remote connection, TLS or tunnel failure | Check DNS, trusted certificate, firewall and reachability from the client. A remote client cannot reach your loopback address. If a tunnel changes the origin, update `JULIUS_PUBLIC_URL`, restart and reconnect; avoid tunnel login/interstitial pages on protocol routes. |
| Browser CORS failure / 405 | Use a native/server-side client or local proxy and stateless Streamable HTTP POST, not direct cross-origin browser calls or legacy SSE. |
| 429 / 413 / timeout | Respect Retry-After. Julius limits concurrent MCP requests to 16, MCP bodies to 1 MiB, OAuth bodies to 16 KiB and requests to 30 seconds; registration and credential checks are also rate-limited. |
| Approval unavailable / 503 | Prepare a new preview after expiry/restart. For 503, inspect local server/database health and sanitized logs; never publish secrets or callback codes. |
