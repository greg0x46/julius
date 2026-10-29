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
