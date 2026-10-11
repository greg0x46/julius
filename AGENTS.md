# AGENTS.md

Julius: self-hosted personal finance tracker for a single owner. One Go binary
(`net/http`, no framework) with the React/TypeScript frontend embedded. SQLite
by default, Postgres optional. Bank data comes from Pluggy (Open Finance) or
manual/CSV entries. User-facing docs: [`README.md`](README.md). Formerly named
Contadinho: the GitHub repo and some local paths still say `contadinho`.

Keep comments brief; don’t restate what the code already makes clear.

## Read before changing domain code

1. [`.specs/README.md`](.specs/README.md) — doc map and doc conventions.
2. [`.specs/motores-de-dominio.md`](.specs/motores-de-dominio.md) — domain
   engines and cross-cutting principles (the "why").
3. [`.specs/invariantes-financeiros.md`](.specs/invariantes-financeiros.md) —
   money rules, owning module and pinning test. Touching a rule means updating
   its row; a test cited there must keep existing.
4. `.specs/contextos/<context>/reference.md` — current behavior of the area you
   touch: packages, routes, pages. Source of truth for "is this implemented?".

If a reference disagrees with the code, the code wins: fix the doc in the same
change.

## Layout

```
cmd/julius/                entry point + subcommands (auth, seed)
cmd/migrate-to-postgres/   one-shot SQLite → Postgres copier
internal/
  db/                      connections/pool, goose migrations (sqlite/ and postgres/), dbtest/
  httpapi/                 handlers, routing (server.go), DTOs, Problem errors
  money/ dates/            pure rules: classification/eligibility; calendar day
  transactions/ ledger/    ledger queries/mutations; multi-engine flows in one DB tx
  categories/ rules/ automation/   categorization, matching core, rule engine
  payables/ scenarios/ projections/ recurrences/ timeline/   debts, hypotheticals, projection
  networth/ investments/ quotes/ marketdata/                 net worth, investments, prices
  pluggy/ syncsvc/ worker/ datasources/ statementimport/     ingestion
  auth/ settings/          owner account + sessions; encrypted secrets
  invariants/              cross-engine tests pinning the financial rules
  webui/                   embeds frontend/dist (build tag embedded_frontend)
frontend/src/              api/ hooks/ pages/ components/ presentation/ theme/ test/
.specs/                    domain docs (see above)
```

## Commands

```sh
./dev.sh                   # backend :8000 + Vite :5173, hot reload
./build.sh                 # frontend build → embed → ./julius

go build ./... && go vet ./... && go test -p 1 -count=1 ./...
cd frontend && npm run lint && npm run typecheck && npm test -- --run
```

CI (`.github/workflows/ci.yml`) runs exactly these, plus `govulncheck`,
`npm audit` and an embedded-frontend build. Run them before declaring done.

- `npm test` watches by default; pass `--run` in scripts/agents. Vitest has a
  `dom` and a `node` project: a new `.ts` test that needs the DOM must be added
  to `domTsTests` in `frontend/vite.config.ts` (or be `.tsx`). See
  `frontend/TESTING.md`.
- Postgres tests skip unless `JULIUS_TEST_POSTGRES_DSN` points to a
  **disposable** database (they reset its `public` schema) — hence `-p 1`.
- `npm run test:e2e` is scaffolded but has no config or tests; ignore it.
- The server refuses to start without `JULIUS_PUBLIC_URL` and exactly one of
  `JULIUS_MASTER_KEY` / `JULIUS_MASTER_KEY_FILE`, and without an account
  (`go run ./cmd/julius auth init`). See README "Authentication".
- In dev, open exactly the `Frontend:` URL `dev.sh` prints
  (`http://127.0.0.1:5173`). Another host gives 403 `invalid-origin` on login
  and every write. Local overrides go in `.env.local` (git-ignored).

## Conventions

**Backend**
- Never persist derived values (payable settled/remaining/status, reconciliation
  state, position value from quotes); recompute on read.
- Plan/annotation tables never hold hard FKs to real transactions; links go
  through explicit link tables.
- Ingestion (sync, import) never rewrites user decisions (manual category and
  inclusion, payable links, scenario realizations). Provider updates keep the
  entry `id`.
- Hypotheticals (scenarios) are opt-in per request (`scenario_id(s)`), never a
  global mode.
- Money is exact decimal (`shopspring/decimal`, strings in JSON); no `float`.
- Calendar days: use `internal/dates` (`Day`, `DaysInMonth`), not local copies.
- Anything that writes several engines' data (new transaction → learned
  category → rules) runs in one DB transaction; pass the `Querier`, don't open
  a nested one. Reuse `internal/ledger`.
- Errors to clients go through `writeProblem` (`internal/httpapi/problem.go`).
  DB failures → 503 without internals; unauthenticated → 401.
- Routes use Go 1.22+ `ServeMux` patterns (`internal/httpapi/server.go`).
  Conflicting literal/wildcard segments **panic at startup**, not at request
  time — run the `internal/httpapi` tests after adding routes.
- HTTP writes require `Origin` = configured origin and header
  `X-Julius-Request: 1`. Responses under `/api` are `no-store`. Retry-safe
  `POST /api/transactions` takes an `Idempotency-Key` header.
- Totals eligibility lives in `money.Eligibility`; its check order is
  load-bearing (see engines doc §1). Don't reorder it.

**Migrations** (`internal/db/migrations/`): two independent goose sequences.
Numbers differ per dialect; match by name suffix, create both files when the
change applies to both. See that folder's README. SQLite uses a single
connection; Postgres pool is configurable via `JULIUS_DB_*`.

**Frontend**: Ant Design + TanStack Query. All HTTP goes through
`src/api/transport.ts`/`client.ts` (CSRF header, 401 handling); don't call
`fetch` directly. Money is formatted/computed on decimal strings in
`src/presentation/money.ts`. UI copy is Brazilian Portuguese; code, comments
and docs are English. Tests are colocated `*.test.ts(x)`.

**Tests**: Go `testing`, colocated `_test.go`. Use synthetic data only — never
real statements, balances, or Pluggy payloads.

## Git, releases, docs

- Conventional Commits (`feat:`, `fix:`, `refactor:`, `docs:`…). A push to
  `main` that passes CI **publishes a release**: `feat` → minor, `fix`/`perf` →
  patch, `!`/`BREAKING CHANGE` → major. Preview: `scripts/next-version.sh`.
- Prefer branch + PR; `git fetch` before pushing to `main`, other sessions may
  push too.
- Finishing a feature or behavior change: update the matching
  `.specs/contextos/<context>/reference.md` in the same PR (and README
  "Features" if user-visible). Delete a design spec once it is implemented.
- Never commit: `*.db*`, built binaries in the repo root, `.env.local`,
  `deploy.sh`, keys, real financial data. `.claude/` is local state.

## Gotchas

- `.specs/` folder and file names are Portuguese (`contextos`, `pendencias`…)
  and are referenced from Go comments and a test; don't rename without updating
  them. The glossary is in `.specs/README.md`.
- `internal/payables` replaced `internal/debts`/`receivables` (gone).
  Recurring commitments are stored as `Scenario{Kind: recurring}` plus
  `scenario_recurring_schedules`; `internal/recurrences` owns occurrences and
  reconciliation.
- Manual transactions never change account balances (`CashOnHand`, net worth
  backfill filter `origin = 'synced'`); only credit-card bills count them.
- Manual transactions are soft-deleted (`deleted_at`); category/inclusion event
  tables are append-only with `ON DELETE RESTRICT`.
- Sync runs are claimed atomically with a heartbeat lease, so several instances
  can share one Postgres; SQLite supports a single process only.
