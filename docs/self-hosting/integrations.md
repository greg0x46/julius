# Integration schedules and market data

### Scheduled sync

Since the API routes require a browser session, an external cron can no longer
trigger `POST /api/sync-runs`. Instead, set `JULIUS_SYNC_SCHEDULE` so the
process itself queues a sync of every active connection once a day:

```sh
export JULIUS_SYNC_SCHEDULE="06:00"                  # process local time
export JULIUS_SYNC_SCHEDULE="06:00 America/Sao_Paulo" # or with an explicit IANA time zone
```

If the process is down at that time, the pending sync is queued as soon as it
starts; a connection that already synced (including manually) after that day's
time is not repeated. Empty disables it.

### Automatic quotes

Investment assets with automatic quotes enabled have the price of their manual
positions fetched automatically and stored in the asset's price series. The
position value is computed on every read (quantity × latest price); a manual
valuation dated on or after the latest price takes precedence over it. Fetching
is enabled by default at 19:00 America/Sao_Paulo. The backend persists
`enabled`, `time` and `timezone` and applies changes without a restart.
Startup and re-enabling fetch missing prices and history; disabling also stops
backfill. The active spot batch may finish; disabling prevents the following
history stage and later work cycles. Providers are tried in order,
with automatic fallback; `JULIUS_QUOTES_PROVIDERS` sets the order
(comma-separated; an omitted provider is disabled; an unknown name prevents
startup):

```sh
export JULIUS_QUOTES_SCHEDULE="19:00 America/Sao_Paulo" # first initialization only
export JULIUS_QUOTES_PROVIDERS="yahoo,brapi,coingecko" # default
```

An authenticated owner can read `GET /api/settings/quotes` and partially update
`PUT /api/settings/quotes`, with the session cookie, configured `Origin` and
`X-Julius-Request: 1`. Example JSON to change the schedule:

```json
{"enabled": true, "time": "20:30", "timezone": "America/Sao_Paulo"}
```

Send `{"enabled": false}` to disable it. Omitted fields are preserved;
`brapi_token` stays write-only, and an explicit empty string clears it.
`time` must be strict HH:MM and `timezone` must be loadable. Saved settings
survive restarts and take precedence over `JULIUS_QUOTES_SCHEDULE`, which
initializes only an absent configuration (HH:MM alone keeps process-local time).
An unset or empty environment uses the enabled default. API changes wake the
scheduler immediately; other persisted changes are checked every minute.

By default Yahoo Finance is the primary source for both markets; brapi is the
B3 fallback and CoinGecko the crypto one. The optional brapi token is
configured in the UI. Details in the
[investments reference](../development/contexts/investments.md#automatic-quotes).

Assets are registered with a financial class (fixed income, variable income,
multi-market, currency, crypto assets or other) and an instrument type. The
quote market is derived from the type; fixed-income and crypto ETFs can use B3
prices.

On every application startup, after database migrations, Julius inserts missing
entries from a curated starter catalog of 27 well-known assets, including BTC,
ETH, SOL, USDC and B3 instruments, and logs the number inserted. Existing assets,
classification and quotation preferences are preserved. No positions, balances,
operations, transactions or price requests are created by seeding.

The manual command remains available:

```sh
go run ./cmd/julius seed investment-assets
```

The command uses `JULIUS_DB` or accepts `-db path-or-DSN`. It can be run
again without duplicating or overwriting entries, and it does not create
positions or balances.
