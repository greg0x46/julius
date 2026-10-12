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
| PUT /api/settings/pluggy | Stores pluggy_client_id/pluggy_client_secret encrypted; optional pluggy_item_id creates a connection in the same transaction; returns saved=true |
| GET /api/settings/quotes | Returns only enabled, time and timezone; never returns the token |
| PUT /api/settings/quotes | Partially updates enabled/time/timezone and optional brapi_token atomically; returns saved=true |
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
The UI keeps the existing token form without schedule controls.
