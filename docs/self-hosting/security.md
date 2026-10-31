# Security

Julius is designed for one owner. Keep authentication enabled, protect the
host and database, and use HTTPS for remote access.

## HTTPS and public access

Terminate HTTPS at your reverse proxy or hosting platform; Julius serves HTTP
on its backend listener. Set `JULIUS_PUBLIC_URL` to the exact HTTPS origin the
browser uses, such as `https://finance.example.com`. Serve at the origin root;
subpath hosting is not supported by this setting.

Forward requests to the backend and restrict that listener to the proxy or a
private network. The proxy must preserve the browser's `Origin` header.
Julius chooses its cookie policy and allowed origin from `JULIUS_PUBLIC_URL`,
not proxy headers. An HTTPS origin enables Secure session cookies even when
the proxy-to-backend connection uses HTTP.

Use the configured hostname and port consistently. Every HTTP write requires
an exact matching `Origin` and `X-Julius-Request: 1`; the built-in UI supplies
these. A hostname mismatch causes `invalid-origin`. HTTP public origins are
accepted only on loopback hosts for local development.

## Browser security headers

Julius sets these headers on every response (the interface, `/api`, `/health`,
`/mcp` and `/oauth/*`), so you do not need to add them in the proxy:

| Header | Value |
| --- | --- |
| `X-Content-Type-Options` | `nosniff` |
| `X-Frame-Options` | `DENY` |
| `Referrer-Policy` | `same-origin` |
| `Content-Security-Policy` | Same-origin only, no inline scripts, no framing (`frame-ancestors 'none'`), `base-uri 'none'`, `object-src 'none'`, `form-action 'self'`. The interface also loads its brand font from `fonts.googleapis.com` and `fonts.gstatic.com`. |
| `Strict-Transport-Security` | `max-age=31536000`, only when `JULIUS_PUBLIC_URL` is HTTPS |

The OAuth consent pages and `/mcp` send an even stricter policy
(`default-src 'none'`) that replaces the default. `Referrer-Policy` must stay
`same-origin`: with `no-referrer` browsers send `Origin: null` on the consent
forms and Julius rejects them.

Keep the proxy from removing or weakening these headers. It may add more, such
as `Strict-Transport-Security` with `includeSubDomains`, once every subdomain
of the host serves HTTPS. Julius leaves `includeSubDomains` and `preload` to
you because they bind the whole domain. A proxy that sets a header Julius also
sets can replace it; check the final response with `curl -I`.

## Master key and stored secrets

Use the [one-time key generation](../getting-started.md#2-configure-the-key-and-local-origin)
instructions. Keep the file outside the repository, readable only by the
service account (for example, mode `0600`). A mounted secret file or your
host's secret manager can supply it; set exactly one of
`JULIUS_MASTER_KEY_FILE` and `JULIUS_MASTER_KEY`.

Never put the key in command arguments, logs, source control, or `VITE_*`
variables. Protect environment files and PostgreSQL credentials similarly.
Stored integration secrets are encrypted; the financial database itself is
not fully encrypted by Julius, so protect disks and backups too.

Back up the key separately from the database, with access limited to the
owner. A password reset cannot recover a lost key. Replacing the key is not
rotation: startup rejects a different key. Key rotation is not supported.
See [backup and restore](upgrading.md#back-up) for recovery steps.

## Authentication and recovery

Create the owner interactively with `auth init`; there is no public sign-up.
Sessions survive restarts, last at most seven days, and expire after 24 hours
of inactivity. Cookies are HttpOnly and SameSite=Lax. Logging out with
**Sair** ends that browser's session; changing the password under
**Configurações** requires the current password and revokes every session.

If you forget the password, use a trusted terminal on the host and the same
database path or configured PostgreSQL DSN:

```sh
./julius auth reset-password -db ./julius.db
```

The command prompts for a new password and confirmation, requires no master
key, and revokes all sessions. The server still needs its original key to run.
There is no email-based password recovery.

**Configurações > Acesso** can disable authentication. This lets anyone who
can reach the instance read and change all data. Keep it enabled on a hosted
installation; origin checks alone do not restrict who can read your data.

## Legacy authentication migration

When upgrading from Contadinho, replace its `CONTADINHO_*` environment
variables with the corresponding `JULIUS_*` names. Julius does not read the old
configuration names. If `julius.db` does not exist but `contadinho.db` does,
Julius keeps using the existing database automatically; if both files exist,
it uses `julius.db`. Existing browser sessions need a new login.

To migrate your existing database:

1. Stop the old version and take a consistent backup of the database.
2. Configure and keep the new key.
3. Run `./julius auth migrate`.
4. Enter the old unlock password, your email and the new login password.
5. Start the application with the same key configured.

The migration re-encrypts every secret and creates the account in a single
transaction; failures never leave a partial conversion. The old verifier is
removed and a second migration is refused. Keep the earlier backup to roll back
to the old version: after the migration, the old binary cannot read the new
secrets. Do not run the migration while the application is running.
