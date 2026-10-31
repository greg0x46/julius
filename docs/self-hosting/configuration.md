# Configuration

Set environment variables in the shell or service environment that launches
Julius. The server requires a valid public origin, exactly one master-key
source, and an initialized owner account. Follow
[getting started](../getting-started.md) for first-time setup.

## Required settings

| Variable | Value |
| --- | --- |
| `JULIUS_PUBLIC_URL` | Browser-facing origin, such as `https://finance.example.com`, without a path, query, fragment, or trailing slash. HTTP is accepted only for `localhost`, `127.0.0.1`, or `::1`. |
| `JULIUS_MASTER_KEY_FILE` | Path to a readable file containing the Base64-encoded 32-byte master key. Set this **or** `JULIUS_MASTER_KEY`. |
| `JULIUS_MASTER_KEY` | The Base64-encoded 32-byte master key, supplied through your host's secrets mechanism instead of a key file. |

The public URL does not configure the listener or enable TLS. See
[security](security.md) for HTTPS, origin checks, and key handling.

## Database and listener

`-db` accepts a SQLite path or PostgreSQL DSN and overrides `JULIUS_DB`.
Without either, Julius uses `julius.db` in the current directory. If that file
does not exist but `contadinho.db` does, it uses the latter for compatibility.
Prefer an explicit path in a service to avoid opening a new empty database
after a working-directory change.

`-addr` defaults to `localhost:4200`. Bind it to an address reachable by your
reverse proxy; restrict direct access to the backend listener.

Both dialects apply embedded schema migrations automatically when opened,
including by authentication commands. See [upgrading](upgrading.md) before
opening an existing database with a new version.

## PostgreSQL

Provision an empty database and a role that can create and alter its schema.
Set the DSN in a protected service environment; the following is a placeholder
example, not a credential to reuse:

```sh
export JULIUS_DB='postgres://user:password@host:5432/julius?sslmode=require'
./julius auth init
./julius
```

The mandatory origin and key settings above still apply. `postgresql://` is
also accepted. Use your database provider's TLS and certificate settings for
remote connections; do not disable TLS for a remote database. Treat the DSN
as a secret and keep the database off the public internet.

Account data and sessions live in PostgreSQL; the master key stays outside it.
Several instances can share a database: migrations serialize with an advisory
lock (waiting up to about five minutes), and sync runs use heartbeat leases.
For upgrades, stop old instances before starting the new version.

Optional pool settings apply per instance; SQLite ignores them:

| Variable | Default | Accepted value |
| --- | --- | --- |
| `JULIUS_DB_MAX_OPEN_CONNS` | `10` | Integer at least 1. |
| `JULIUS_DB_MAX_IDLE_CONNS` | Effective max open | Integer at least 0, capped at max open. |
| `JULIUS_DB_CONN_MAX_LIFETIME` | `30m` | Non-negative Go duration; `0` means unlimited. |
| `JULIUS_DB_CONN_MAX_IDLE_TIME` | `5m` | Non-negative Go duration; `0` means unlimited. |

Invalid values fail database opening rather than silently using defaults.

## Timezone

Calendar days (what "today" is, when a credit-card cycle closes, which month a
report covers) follow the timezone chosen under **Configurações**, which
defaults to `America/Sao_Paulo`. The server's `TZ` does not affect them, so a
container running in UTC behaves the same as a host in São Paulo. Only the sync
schedule below uses the process zone when you give it no zone.

## Optional integrations and schedules

Pluggy credentials are configured under **Configurações** and stored encrypted
in the database. They are optional for manual accounts and CSV imports.

- `JULIUS_SYNC_SCHEDULE`: daily sync time, such as `06:00 America/Sao_Paulo`.
  `HH:MM` alone uses process-local time; empty disables scheduled sync. A missed
  run is queued on startup unless the connection already synced after that
  day's scheduled time.
- `JULIUS_QUOTES_SCHEDULE`: initial quote schedule, defaulting to
  `19:00 America/Sao_Paulo`, enabled. Saved quote settings take precedence on
  later starts.
- `JULIUS_QUOTES_PROVIDERS`: comma-separated provider order, default
  `yahoo,brapi,coingecko`. Omitted providers are disabled; unknown names prevent
  startup.
- `JULIUS_MCP_ENABLED`: optional MCP integration, disabled by default. See the
  [MCP reference](../development/mcp-tools.md) before enabling it.

Leave `JULIUS_DEMO` unset for a personal installation: demo mode opens access
and disables external integrations. `VITE_*` values are build-time frontend
settings and must never contain secrets.
