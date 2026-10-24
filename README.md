# Julius

Julius is a self-hosted personal finance tracker, built for a single
person to use locally or from an online installation. It syncs the activity of
your bank account and credit card through the
[Pluggy](#open-finance--pluggy) Open Finance API — which authorizes you to read
your own financial data, not to manage other people's — and then lets you
categorize transactions, automate recurring categorization with rules, and
track debts (such as installment purchases or loans) against the transactions
that pay them off.

It runs as a single Go binary with the React frontend embedded in it — no
frontend runtime in production. The default database is SQLite, with Postgres
as an option. For online access, the host must provide HTTPS.

## Domain documentation and contributing

Contributions, new features and refactors are welcome. Coding agents: start at
[`AGENTS.md`](AGENTS.md). Before touching an area
of the domain, see [`.specs/README.md`](.specs/README.md) — the navigation hub
of the `.specs/` folder, with the domain architecture
(`motores-de-dominio.md`), the current state of each feature
(`contextos/<context>/reference.md`) and the historical rationale behind design
decisions. When you finish a new feature or change an existing one, update the
matching context's `reference.md` (and the "Features" section below, if it is
relevant enough) in the same PR — stale specs hinder the next contributor as
much as stale code does.

Open pull requests against the **`stage`** branch (not `main`) and fill in the
pull request template, including the *How to validate in dev* steps. See
[Publish and install a version](#publish-and-install-a-version) for how changes
reach a release.

## Why a single binary

Julius packs the application and the frontend into one binary, with
authentication configuration that does not depend on the host:

- **One file to run.** `go build` produces a single executable with the
  frontend already embedded. No Docker, no reverse proxy, no Node runtime in
  production (Node is only needed once, to build the frontend).
- **One file as the database (by default).** With SQLite — the default — data
  and sessions live in the `julius.db` file. Also keep the external
  encryption key when moving or restoring the installation. No database server
  to install or keep running. To run several Julius instances sharing a
  database in the cloud, Postgres is an option — see
  [Running with Postgres](#running-with-postgres).
- **Host-independent configuration.** The account is created from the
  terminal; the public origin and the encryption key come from configuration.
  The app does not depend on cloud SDKs or external authentication services.

## Features

- **Exact money boundaries** — amounts stay decimal strings through entry,
  editing, calculation and HTTP. BRL display rounds to cents with ties away
  from zero; saved fractional cents and large values retain their precision.
  Historical balance minima use exact comparisons to select their value and date.
  The balance chart includes commitments due today in today's end-of-day point;
  **Saldo hoje** shows the current cash balance.
  See the [monetary precision policy](.specs/precisao-monetaria.md).

- **Build identifier** — every signed-in page has a small footer showing the
  release tag, `stage` or `dev`, with a link to the build commit when available.
- **Typed database baseline** — a one-shot legacy converter preserves existing
  data; both dialects share one migration sequence with native exact Postgres
  types. Final legacy Postgres schemas convert at startup; legacy SQLite
  databases need the separate converter. See
  [Database schema generations](#database-schema-generations).
- **Open Banking sync** — a background worker polls Pluggy periodically for
  new transactions and account data, recorded as auditable sync runs (history,
  metrics, failures) instead of a black-box import.
- **Statement import** — upload a Flash CSV, review the per-row preview and
  import it into a file-based account. Re-uploads and overlapping periods are
  automatically deduplicated across files. Balance conflicts show existing and
  incoming values for an explicit per-row choice, with an import history.
- **Transfers between own accounts** — link existing synced/manual outflows and
  inflows, including different currencies, or create both movements together
  in manual accounts. Native amounts and balances stay intact; principal stays
  outside income/expense reports. Details show the effective execution rate
  and an optional separate, explicitly recorded fee. No fee is inferred from
  an FX spread. Unlinking keeps the movements and their prior reporting decisions.
- **Manual bank accounts** — create standalone accounts in an ISO 4217 currency
  (BRL by default), including USD/EUR, without a bank connection. Native balance
  is opening balance + posted movements + explicit adjustments. Edit, archive
  and reactivate accounts in **Contas e cartões**; currency and opening balance
  lock after movements; equivalent decimal representations still allow name
  and archive changes. Foreign amounts stay in their original currency and
  are valued in BRL in Patrimônio and the investment summary with the day's
  exchange rate; a missing rate makes the figure unavailable.
- **Reference currency** — choose the currency for consolidated Home, category,
  investment and patrimônio figures in **Configurações → Geral** (BRL by default).
  Current reports use current FX; the patrimônio history uses each day’s FX.
  Original records remain untouched. Missing rates
  make the affected report unavailable instead of assuming a 1:1 conversion.
- **Transactions** — searchable/filterable transaction list, manual
  inclusion/exclusion (e.g. ignoring a chargeback or a duplicate) and
  categorization. Entries can also be created by hand in an existing account —
  same categorization and automation rules as a synced entry, with editing and
  deletion limited to these manual entries.
- **Multi-currency reporting** — transaction summaries, Home period totals,
  categories and Timeline consolidate eligible flows to BRL using historical
  exchange rates, retaining original values and per-currency drill-down.
  Frankfurter v2 supplies historical blended rates and the account currency list;
  BRL stays available offline. Native-only currencies are allowed with a warning.
  Unsupported currencies, missing history and provider outages show distinct
  feedback without changing native amounts or displaying complete BRL totals.
- **Automation rules** — condition-based rules that categorize new
  transactions automatically and can be applied retroactively to existing ones.
  A single rule can both categorize and reconcile a recurring commitment.
- **Payables (debts and receivables)** — track a real total (debt, installment
  purchase, receivable) and link the transactions that settle it, with
  eligibility rules for which transactions can be linked.
- **Categories** — a user-defined category catalog, with categorization
  history.
- **Recurrences** — register cash-flow commitments known in advance (salary,
  rent, subscriptions), reconciled against the real transactions that settle
  them via an automation rule, even when the transaction is uncategorized or
  has a different category.
- **Scenarios** — user-authored hypothetical projections ("what if I travel",
  "what if I change jobs"), or the installment plan of a debt/receivable —
  they never affect real totals unless explicitly included in a query.
  Fixed-amount installments preserve exact remainders without exceeding the
  chosen installment amount. Generation and readjustment are limited to 10,000
  installments per operation; requests exceeding the limit preserve the plan.
- **Financial report** — a single time navigation (Actual → Today → Base
  Projection → Simulation): how much you will have on a future date, your
  lowest balance until then, and how hypothetical scenarios change that answer.
- **Patrimônio** — what you already have, cash plus investments, today and
  over time. Cards, debts and future bills are not counted, and the history is
  recomputed on every read, so it follows any correction.
- **Goals** — set a target amount (and optional date) for the total market
  value of your investments and follow the progress, recomputed on every read. With a
  date, the goal is projected to it from the average monthly return of your investments and
  shows the monthly contribution needed to reach the target. More metrics (cash balance, patrimônio) are planned.
- **Investments** — a period view of your portfolio (same period selector as Home and Transactions): portfolio value at the end of
  the period, net yield and return, contributions and withdrawals, with positions grouped by account or asset class,
  searchable, and with positions the history cannot cover flagged instead of counted as zero. Integrated and manual investment
  accounts, per-asset operations, manual valuations and optional automatic
  quotes (B3 and crypto), enabled by default with a persistent daily schedule. Contributions and withdrawals reconciled with the
  statement show up apart from spending, keeping their cash effect and avoiding
  double counting in patrimônio. Manual-position returns use the exact recorded
  acquisition cost, rather than reconstructing it from the displayed average cost.
  A curated starter asset catalog is populated automatically on startup.
- **Browser authentication** — email and password login for the owner account,
  persisted sessions, logout and password change. No public sign-up.
- **Secrets encrypted at rest** — Pluggy credentials protected with
  AES-256-GCM by a key independent of the password, supplied by the environment
  or a file. The worker keeps running after restarts without requiring a login.

## Investments by account

In **Investimentos**, pick the period at the top: the summary and the positions
follow it, and the accounts, with their movements, stay below the positions.
Register a manual custody account or use the grouping
created for a connection. Create empty positions for new purchases; enter an
opening balance only for holdings that already existed. Purchases accept
quantity, price, fees and taxes. The "deposit along with the purchase" option
records a contribution or reinvests income in a single action.

In the statement, open the entry and choose **Vincular a investimento** (link
to investment). Confirm the destination and the portion; the rest stays
available for another movement. The imported asset movement can be linked as
well. The **Revisar lançamentos antigos** (review older entries) button opens
the history with no period limit, without reclassifying it automatically.

Integrated accounts keep the balances reported by the institution. Use
**Vincular caixa da corretora** (link brokerage cash) when the imported
financial account already represents the same cash. Manual holdings (such as
crypto that Open Finance does not expose) can be added to an integrated account,
next to the institution's positions, and are never overwritten by a sync. Manual quotes are dated;
corrections to operations recompute subsequent costs and are rejected if they
would produce negative cash or a negative position.

Contracts and limits are in the
[investments reference](.specs/contextos/investimentos/reference.md).

## Goals

In **Metas**, create a goal with a name, a target amount in reais and an
optional date. Today the only metric is **valor investido**: the market value of
all your investments, gains and losses included (the investments line of net
worth). Progress is the current value divided by the target and can pass 100%.
With a future date, the goal also shows a projection: the average monthly
appreciation (or depreciation) of your investments over the last 12 months, the
value and percentage of the target expected on the date without new
contributions, and the monthly contribution needed to reach it. It is an
estimate that assumes the past return repeats. Nothing is stored besides the
target. See the
[goals reference](.specs/contextos/metas/reference.md).

## Tech stack

- **Backend**: Go, `net/http` (stdlib routing, no framework), SQLite via
  [`modernc.org/sqlite`](https://gitlab.com/cznic/sqlite) (no cgo) by default,
  with optional Postgres via [`pgx`](https://github.com/jackc/pgx) — see
  [Running with Postgres](#running-with-postgres) — and migrations via
  [`goose`](https://github.com/pressly/goose) for both dialects.
- **Frontend**: React 19, TypeScript, React Router and TanStack Query. Julius owns
  its HTML/CSS controls, responsive layouts, SVG icons and charts; no external UI,
  chart or date library is shipped. Vite builds the embedded assets.
- **Tests**: Go's standard `testing` package, [Vitest](https://vitest.dev/) +
  Testing Library for components, [Playwright](https://playwright.dev/) for
  end-to-end.

## Getting started

### Prerequisites

- Go 1.26+
- Node.js 24.15+ (or 26+)
- A [Pluggy](https://pluggy.ai) account with a client ID/secret and at least
  one `item_id` for the account you want to sync — see Pluggy's
  [Get your API keys](https://docs.pluggy.ai/docs/get-your-api-keys) guide, and
  the [Open Finance & Pluggy](#open-finance--pluggy) section below. Additional
  banks are registered later, in `/open-banking`, by pasting each one's
  `item_id` — the client ID/secret is the same for all of them.

### Run the production build (single binary)

```sh
./build.sh
```

The frontend bakes build metadata into the binary. Release CI passes the selected
release tag as `VITE_APP_VERSION` and the exact build SHA as `VITE_APP_COMMIT`;
stage CI passes `stage` and its SHA. Local builds on the `stage` branch default
to `stage`; other local builds and the dev server default to `dev`, using Git
HEAD when available. Source archives without Git still show `dev` without a
commit link. These optional build-time overrides also work with `./build.sh`;
the frontend package version is never used as the application version.

Before starting, configure the encryption key and create or migrate the
account. The server refuses to start without prepared authentication or with
an invalid key.

### Publish and install a version

Changes flow `agent/*` or feature branch → **`stage`** → release PR → **`main`**
→ published release → deployment:

1. A pull request into `stage` is squashed into one Conventional Commit whose
   subject ends in `(#PR)`. CI runs on every push and pull request for `stage`
   and `main`, but `stage` never publishes anything. The `stage` build is what
   the development instance runs, so each change is validated there by hand,
   with the *How to validate in dev* steps of its pull request.
2. A release candidate is cut automatically, not by hand. When Backend (Go) and
   Frontend (Node) have passed on the exact `stage` head and there is something
   to release, `release/vX.Y.Z` is cut from that commit. Its only extra commit
   adds the `vX.Y.Z` section to [`CHANGELOG.md`](CHANGELOG.md)
   (`scripts/update-changelog.sh`; the only place the changelog is written), and
   a pull request `chore(release): vX.Y.Z` into `main` lists the original pull
   requests and `Closes #N` for their issues. The candidate is frozen: pull
   requests squashed into `stage` afterwards wait for the next release. If
   `main` moves first, the candidate is closed, its branch is kept as
   `stale/release-vX.Y.Z-<sha7>` and a new one is cut.
3. Only the owner merges that pull request, with a **merge commit**, never
   squash or rebase, so the individual commits stay reachable. The ruleset on
   `main` allows no other method; it also requires a pull request, the
   up-to-date Backend (Go) and Frontend (Node) checks and resolved
   conversations, and forbids force-push and deletion. Merging closes the
   issues. GitHub deletes `release/vX.Y.Z` on merge; the tag is the lasting
   reference.
4. A push to `main` that passes CI publishes a GitHub release with the
   `julius-linux-amd64` binary, `SHA256SUMS` and the provenance attestation
   (`julius-linux-amd64.sigstore.json`), as long as there is something to
   release. The release notes are the committed `CHANGELOG.md` section of that
   version; if the section is missing or is not the newest, the release job
   fails instead of publishing. **Published is not deployed:** installing the
   release is a separate step of your environment, and a release only counts
   as deployed when that step reports it.
5. Once the tag and the GitHub release exist, `main` is merged back into
   `stage` (`chore(release): merge main into stage after vX.Y.Z`, a plain push,
   no rebase or force-push), keeping the pull requests squashed into `stage`
   in the meantime. Until `main` is inside `stage` no new candidate is cut.

The version follows SemVer from the
[Conventional Commits](https://www.conventionalcommits.org/) since the last
tag: `feat` bumps the minor, `fix`/`perf` bump the patch, and a breaking change
(`feat!:` or a `BREAKING CHANGE:` footer) bumps the major (the minor while on
`0.x`). Other types (`docs`, `chore`, `refactor`…) and non-conventional commits
do not create a release. A commit and the revert of that same commit cancel out;
a revert that cannot be matched is listed under "Needs review" in the notes.
To see locally what the next version and notes would be, run
`scripts/next-version.sh` (or `--notes`) after `git fetch --tags`.
`CHANGELOG.md` is written only when a release is prepared and starts after
v0.3.0; the earlier releases are on the GitHub releases page.

To install, download the binary from the release, verify the checksum and run
it with the configuration described in the next sections; how to start it and
keep it running (system service, container, etc.) depends on your environment.

The scripts have shell tests that run against throwaway Git histories:
`scripts/tests/run.sh`.

Dependabot version updates open against `stage`. Security updates still open
against `main`; see "Dependabot" in [`AGENTS.md`](AGENTS.md) for how they are
handled.

**Database.** Every change merged into `stage` is migrated on the persistent
development database before it reaches production, and nothing restores that
database automatically. Migrations must therefore be forward-only (never edit
one that is already merged; add a new one) and safe on existing data. For the numbering rules see
[`internal/db/migrations/README.md`](internal/db/migrations/README.md).

### Authentication and encryption key

Generate a 32-byte Base64 key **once**, in a file outside the repository. The
example refuses to overwrite an existing file:

```sh
mkdir -p "$HOME/.config/julius"
(umask 077; set -C; openssl rand -base64 32 > "$HOME/.config/julius/master.key")
export JULIUS_MASTER_KEY_FILE="$HOME/.config/julius/master.key"
export JULIUS_PUBLIC_URL="http://localhost:4200"
```

Alternatively, provide the Base64 value in `JULIUS_MASTER_KEY` through your
host's secrets mechanism. Set exactly one of the two sources. Do not put the
value in the repository, arguments, logs or `VITE_*` variables.

`JULIUS_PUBLIC_URL` is the origin as seen by the browser, without a path or
trailing slash. For online use, use `https://your-hostname`; HTTP is only
allowed on `localhost`, `127.0.0.1` or `::1`, for development. HTTPS may be
terminated externally; the application does not trust proxy headers to choose
the cookie policy or the allowed origin.

For a new database:

```sh
./julius auth init -db ./julius.db
./julius -db ./julius.db
```

The command asks for an email and a non-empty password in an interactive
terminal, without echoing the password. There are no length or composition
rules: the choice is up to the user. Sign in from the browser and configure the
Pluggy credentials under **Configurações** (settings); the optional Item ID
registers a new connection. Additional connections remain available under
**Open Banking**.

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

Keep a safe copy of the key separate from the database backup. Password
recovery does **not** recover a lost key. Replacing the file with another key
is not rotation: the server will refuse to start. Key rotation is not part of
this version.

To regain access from the terminal while keeping encryption:

```sh
./julius auth reset-password -db ./julius.db
```

This command does not need the key, requires a new password and revokes every
session. Changing the password under **Configurações** requires the current
password and also ends every session. **Sair** (log out) ends only the current
browser session.

Sessions last at most seven days, expire after 24 hours of inactivity, and
survive restarts. Every private access is checked in the backend. Login accepts
up to five attempts per minute per email and thirty per minute per process; the
limit resets with the process. HTTP writes require an `Origin` equal to the
configured origin and `X-Julius-Request: 1`.

Under **Configurações > Acesso**, the owner can disable authentication.
Disabling it requires a valid session; after that, anyone who can reach the
instance can read and change all data without logging in. Re-enabling it is
allowed from open mode and makes subsequent requests require a session
immediately. Origin checks for writes stay active in both modes. The preference
is stored in the database; if it does not exist, protected mode is used, and if
it cannot be read the API becomes unavailable — it never opens because of a
configuration failure.

### Public demo

`JULIUS_DEMO=true` runs the instance as an open showcase of synthetic data:

- There is no login. The API behaves as in open mode whatever the stored
  preference says, and `GET /api/auth/session` adds `"demo": true`, which makes
  the UI show a notice.
- Nothing that changes access or reaches a provider can be written: `/api/auth/*`,
  `/api/settings/*`, `POST /api/sync-runs` and writes to `/api/data-sources`
  answer 403 `demo-mode`. Ordinary data stays editable and the `Origin` check
  stays on. The sync worker and the quote refresh do not run.
- `julius demo seed [-db …]` fills a **fresh** database with a synthetic
  household: accounts, six months of transactions, recurring commitments, debts,
  goals and a small portfolio with prices. Dates are relative to the day it runs.
  It needs the same `JULIUS_PUBLIC_URL` and key variables as the server, creates
  the owner with a random password nobody learns, turns authentication off and
  refuses a database that already has an owner: it never deletes anything.

To reset a demo, start from an empty database (a new SQLite file or a new
Postgres database), seed it and restart the server, for instance daily:

```sh
export JULIUS_MASTER_KEY_FILE=demo.key JULIUS_PUBLIC_URL=https://demo.example.com
julius demo seed -db demo.db
JULIUS_DEMO=true julius -db demo.db -addr 127.0.0.1:4200
```

Give the demo its own key and database; never reuse those of a real instance.

### Database schema generations

This binary initializes a new schema generation on empty databases. Postgres
stores money as exact `NUMERIC`, instants as `TIMESTAMPTZ`, calendar days as
`DATE`, flags as `BOOLEAN`, and UUID identities as `UUID`; SQLite retains exact
text money and UTC timestamps. Both dialects use the same migration numbers.

**Existing Postgres installations:** startup automatically converts the final
legacy schema (version 46) into generation 1 in the same database. Stop old
application instances before upgrading and retain an external database backup.
The database role needs permission to create schemas. A transaction locks the
legacy tables, builds the native baseline, copies and verifies every converted
row, then activates it without changing the DSN. The original tables, migration
history and append-only trigger function remain in a `julius_legacy_<id>` schema;
the startup log identifies it. Concurrent startups serialize, and subsequent
starts skip conversion. Any invalid value, schema mismatch or constraint failure
rolls back the entire operation and prevents startup with the original intact.
Older Postgres schemas must first reach version 46 using the legacy binary.
The backup schema is retained indefinitely; do not delete it before acceptance.

**Existing SQLite installations:** databases from the old migration sequence are
rejected at startup, without applying migrations. Use the one-shot
`cmd/migrate-legacy` tool with a separate target; it replaces
`cmd/migrate-to-postgres`. Keep the old binary, database and external master key.
Do not change the old database's migration version to bypass the guard.

1. Stop Julius and make a consistent SQLite backup. The source must have reached
   the final legacy schema (SQLite migration `00047_manual_bank_accounts`); if
   necessary, upgrade it with the previous binary before taking the backup.
2. Choose a new SQLite path or a fresh Postgres database. Keep both applications
   stopped throughout conversion. The tool initializes the target schema and
   refuses existing application data, except untouched baseline category seeds.
3. Run a dry-run against the backup:

   ```sh
   go run ./cmd/migrate-legacy -sqlite legacy-backup.db -target converted.db -dry-run
   # For Postgres, replace converted.db with a postgres:// DSN.
   ```

   It reports all malformed rows and constraint errors together, with table,
   row identity and value. Require `zero errors` and matching row counts,
   per-account balances and movement sums. Imported data and replaced category
   seeds roll back; the new target schema remains initialized.
4. Repeat the same command without `-dry-run`. Require `copy complete; counts
   and balances match` before starting Julius with the target database and the
   same master key. Check accounts, transactions, decisions and sign-in before
   retiring the previous installation.

The source opens read-only without applying migrations. IDs, encrypted settings,
raw payloads, sessions and user decisions carry over; auth timestamps and calendar
fields follow the documented [conversion contract](.specs/reorganizacao-schema/inventario.md).
Postgres rejects legacy IDs that cannot fit its UUID columns; SQLite preserves
opaque legacy IDs. No rounding or guessed values are used. An interrupted or
failed real copy can leave earlier tables committed: discard only the separate
target and retry with a fresh one. Keep the source untouched as the rollback
backup. This tool is temporary and should be removed after cutover.

### Running with Postgres

SQLite remains the default for local use. To run against a shared Postgres —
for example, several Julius instances pointing at the same cloud database —
pass a `postgres://` (or `postgresql://`) DSN to the `-db` flag instead of a
file path:

```sh
./julius -db "postgres://user:password@host:5432/julius?sslmode=require"
```

The driver is detected automatically from the DSN prefix. Postgres schema
migrations are applied automatically, just like the SQLite ones. Migrations
take a Postgres advisory lock, so several instances can start at once: one
applies the pending migrations, the others wait (up to about 5 minutes) and
then skip them. The `auth init`, `auth migrate` and `auth reset-password`
commands accept the same DSN in the `-db` flag or in `JULIUS_DB`. Sessions and the account live in
the database; the encryption key stays outside it.

The Postgres connection pool can be tuned through environment variables
(SQLite ignores them and always uses a single connection):

| Variable | Default | Meaning |
| --- | --- | --- |
| `JULIUS_DB_MAX_OPEN_CONNS` | `10` | Maximum open connections per instance (integer >= 1). |
| `JULIUS_DB_MAX_IDLE_CONNS` | max open | Idle connections kept for reuse (integer >= 0, capped at max open). |
| `JULIUS_DB_CONN_MAX_LIFETIME` | `30m` | Go duration after which a connection is recycled; `0` = unlimited. |
| `JULIUS_DB_CONN_MAX_IDLE_TIME` | `5m` | Go duration an idle connection is kept; `0` = unlimited. |

An invalid value makes startup (and any `-db` command) fail with an error
naming the variable, instead of silently falling back to the default.

Several instances can share one Postgres database: each sync run is claimed
atomically by exactly one instance, which heartbeats it while running. Only
that owner can persist sync data or finish the run. A separate recovery loop
continues while other runs execute. A run whose owner stops heartbeating for 90
seconds is marked failed (`interrupted`) and not retried; the next scheduled
or manual sync starts a new one. After a crash, an interrupted run is
therefore recovered up to ~90 seconds after restart rather than immediately.
SQLite supports a single process only; do not share a SQLite file between
instances.

#### Documenting the schema with SchemaSpy

`docker-compose.schemaspy.yml` runs [SchemaSpy](https://schemaspy.org/) against
a development Postgres reachable from the machine (`host.docker.internal`) and
writes the HTML to `schemaspy/output/` (a git-ignored directory). It is not the
application's compose file. The password is required; user, database and port
default to `admin`, `julius` and `5432`:

```sh
SCHEMASPY_DB_PASSWORD=... docker compose -f docker-compose.schemaspy.yml up
# optional: SCHEMASPY_DB_USER, SCHEMASPY_DB_NAME, SCHEMASPY_DB_PORT
```

Then open `schemaspy/output/index.html`.

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
[investments reference](.specs/contextos/investimentos/reference.md#automatic-quotes).

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

### Run for local development

After configuring the key and preparing the account with
`go run ./cmd/julius auth init` (or `auth migrate` for an old database),
the script below starts the backend and Vite together. The frontend gets hot
reload and `/api` requests are forwarded to the backend automatically:

```sh
./dev.sh
```

Open the URL the script prints on the `Frontend:` line (`http://127.0.0.1:5173`
by default). The script uses exactly that origin as the default
`JULIUS_PUBLIC_URL`, and the backend strictly compares the browser's
`Origin` header against it: opening `http://localhost:5173` instead, or having
already exported another value in `JULIUS_PUBLIC_URL`, results in a 403
`invalid-origin` on login and on every POST/PUT/DELETE. If you prefer another
host, change `VITE_DEV_HOST` (the script derives the origin from it) instead of
exporting the URL by hand. `Ctrl-C` stops both processes. To override host and
ports:

```sh
JULIUS_DEV_ADDR=localhost:8100 VITE_DEV_HOST=localhost VITE_DEV_PORT=5174 ./dev.sh
```

For machine-specific configuration, create a `.env.local` at the repository
root (ignored by git) with `KEY=value` lines; `dev.sh` loads that file before
starting. E.g. `JULIUS_DB`, `JULIUS_MASTER_KEY_FILE`, ports and, to
reach Vite through another domain, `__VITE_ADDITIONAL_SERVER_ALLOWED_HOSTS=<host>`.

If the dependencies are not installed yet, run `cd frontend && npm install`
once before starting the script.

### Tests and checks

```sh
# backend
go build ./...
go vet ./...
go test ./...

# frontend
cd frontend
npm run lint
npm run typecheck
npm test
```

The Postgres integration tests are skipped unless
`JULIUS_TEST_POSTGRES_DSN` points at a disposable database. They reset its
`public` schema, so run them with `go test -p 1 ./...`, as CI does.

The release scripts (`scripts/next-version.sh`, `update-changelog.sh`,
`changelog-section.sh`) and the CI/Dependabot configuration are covered by
`scripts/tests/run.sh`, which CI also runs.

The frontend suite runs as two Vitest projects (jsdom and plain Node); see
[`frontend/TESTING.md`](frontend/TESTING.md) for the split and worker count.

`npm run test:e2e` runs the financial browser journeys against isolated local
Julius servers and disposable SQLite databases, without Pluggy credentials.
See [`frontend/TESTING.md`](frontend/TESTING.md#financial-browser-journeys-playwright)
for installation, coverage and failure evidence. CI runs the same suite.

## Open Finance & Pluggy

This project only exists thanks to Open Finance regulation in Brazil and to
[Pluggy](https://pluggy.ai), the Open Finance infrastructure provider whose API
Julius integrates with. Pluggy connects to more than 130 Brazilian
financial institutions and, as a Payment Initiation Service Provider (ITP)
regulated by the Central Bank, offers applications like this one a
standardized, authorized way to read account balances, transactions and
investment data on the user's behalf — turning "my data is mine" from a slogan
into something an independent developer can actually build on, without a
bespoke integration with each institution.

All credit to the Pluggy team for this infrastructure and for making Open
Finance access viable for small/independent projects. See
[pluggy.ai](https://pluggy.ai) to learn more about the platform, and
[Get your API keys](https://docs.pluggy.ai/docs/get-your-api-keys) for a
step-by-step guide to obtaining the client ID/secret this project asks for
during setup.

## Project structure

Package map, commands and conventions: [`AGENTS.md`](AGENTS.md). What each area
does today (routes, pages, implementation status): [`.specs/README.md`](.specs/README.md)
and each context's `reference.md` under `.specs/contextos/`.
