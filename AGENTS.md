# AGENTS.md

Julius: self-hosted personal finance tracker for a single owner. One Go binary
(`net/http`, no framework) with the React/TypeScript frontend embedded. SQLite
by default, Postgres optional. Bank data comes from Pluggy (Open Finance) or
manual/CSV entries. User-facing docs: [`docs/README.md`](docs/README.md). Formerly named
Contadinho: the GitHub repo and some local paths still say `contadinho`.

Keep comments brief; don’t restate what the code already makes clear.

## Read before changing domain code

1. [`docs/development/README.md`](docs/development/README.md) — doc map and doc conventions.
2. [`docs/development/architecture.md`](docs/development/architecture.md) — domain
   engines and cross-cutting principles (the "why").
3. [`docs/development/financial-invariants.md`](docs/development/financial-invariants.md) —
   money rules, owning module and pinning test. Touching a rule means updating
   its row; a test cited there must keep existing.
4. `docs/development/contexts/<context>.md` — current behavior of the area you
   touch: packages, routes, pages. Source of truth for "is this implemented?".

If a reference disagrees with the code, the code wins: fix the doc in the same
change.

## Layout

```
cmd/julius/                entry point + subcommands (auth, seed, demo)
cmd/migrate-legacy/        one-shot legacy SQLite → SQLite/Postgres converter
internal/
  db/                      connections/pool, goose migrations (sqlite/ and postgres/), dbtest/
  httpapi/                 handlers, routing (server.go), DTOs, Problem errors
  httplog/                 HTTP access log, request ids, security events; one middleware, controlled values only
  money/ dates/            pure rules: classification/eligibility; calendar day
  transactions/ ledger/    ledger queries/mutations; multi-engine flows in one DB tx
  categories/ rules/ automation/   categorization, matching core, rule engine
  payables/ scenarios/ projections/ recurrences/ timeline/   debts, hypotheticals, projection
  goals/                   targets over a global metric; progress recomputed on read
  networth/ investments/ quotes/ marketdata/                 wealth (cash + investments), investments, prices
  pluggy/ syncsvc/ worker/ datasources/ statementimport/     ingestion
  auth/ settings/          owner account + sessions; encrypted secrets
  demo/                    synthetic data for the public demo (`julius demo seed`, `JULIUS_DEMO`)
  invariants/              cross-engine tests pinning the financial rules
  webui/                   embeds frontend/dist (build tag embedded_frontend)
frontend/src/              api/ hooks/ pages/ components/ presentation/ theme/ test/
docs/development/          architecture, financial rules and API contracts
docs/guides/               user guides
docs/self-hosting/         installation and operations
```

## Commands

```sh
./dev.sh                   # backend :8000 + Vite :5173, hot reload
./build.sh                 # frontend build → embed → ./julius

go build ./... && go vet ./... && go test -p 1 -count=1 ./...
cd frontend && npm run lint && npm run typecheck && npm test -- --run
```

CI (`.github/workflows/ci.yml`) runs exactly these, plus `govulncheck`,
`npm audit`, an embedded-frontend build, Playwright financial journeys and
`scripts/tests/run.sh` (shell tests of the release scripts). Run them before declaring done; run
`scripts/tests/run.sh` whenever you touch `scripts/`, `CHANGELOG.md` or the
workflow.

- `npm test` watches by default; pass `--run` in scripts/agents. Vitest has a
  `dom` and a `node` project: a new `.ts` test that needs the DOM must be added
  to `domTsTests` in `frontend/vite.config.ts` (or be `.tsx`). See
  `frontend/TESTING.md`.
- Postgres tests skip unless `JULIUS_TEST_POSTGRES_DSN` points to a
  **disposable** database (they reset its `public` schema) — hence `-p 1`.
- `npm run test:e2e` builds the app and runs Playwright against disposable SQLite
  databases. Install Chromium first; see `frontend/TESTING.md`. No live provider
  credentials or existing dev instance are needed.
- The server refuses to start without `JULIUS_PUBLIC_URL` and exactly one of
  `JULIUS_MASTER_KEY` / `JULIUS_MASTER_KEY_FILE`, and without an account
  (`go run ./cmd/julius auth init`). See `docs/self-hosting/security.md`.
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
- Every request is logged by the one `internal/httplog` middleware; new routes
  appear on their own. A handler that rejects something calls
  `httplog.SetError` (`writeProblem` does it for REST) with literals (never request text, `err.Error()`, bodies,
  queries or secrets); new events need an entry in `httplog.eventLevels`.
- Routes use Go 1.22+ `ServeMux` patterns (`internal/httpapi/server.go`).
  Conflicting literal/wildcard segments **panic at startup**, not at request
  time — run the `internal/httpapi` tests after adding routes.
- HTTP writes require `Origin` = configured origin and header
  `X-Julius-Request: 1`. Responses under `/api` are `no-store`. Retry-safe
  `POST /api/transactions` takes an `Idempotency-Key` header.
- Totals eligibility lives in `money.Eligibility`; its check order is
  load-bearing (see architecture "Ledger, reporting and cash"). Don't reorder it.

**Migrations** (`internal/db/migrations/`): one numbered goose sequence with
a file per dialect. Both start at `0001_initial.sql`; use the same number and
name suffix in SQLite and Postgres. Legacy migrations are preserved in `archive/`
and never run by the application. See that folder's README. SQLite uses a single
connection; Postgres pool is configurable via `JULIUS_DB_*`.

Because every merge into `stage` is applied to the persistent dev database
(never restored or refreshed automatically) and only later reaches production:
- Migrations are forward-only. Never edit or renumber a migration that is
  already on `stage` or `main`; add a new one.
- Keep a migration safe to run once on data that already exists (backfill,
  defaults, no destructive change without a deliberate plan), and test it on
  both dialects when it applies to both.
- Don't depend on production data or credentials existing in dev; use
  synthetic data in tests and in the *How to validate in dev* steps.

**Frontend**: React + TanStack Query, with local HTML/CSS/SVG components in
`src/components/ui/` (no external UI or chart library). All HTTP goes through
`src/api/transport.ts`/`client.ts` (CSRF header, 401 handling); don't call
`fetch` directly. Money is formatted/computed on decimal strings in
`src/presentation/money.ts`. UI copy is Brazilian Portuguese; code, comments
and docs are English. Tests are colocated `*.test.ts(x)`.

**Tests**: Go `testing`, colocated `_test.go`. Use synthetic data only — never
real statements, balances, or Pluggy payloads.

## Git, releases, docs

**Branches.** `stage` is the rolling integration branch; `main` is production.
- Open every implementation PR **against `stage`** (agents included), from a
  branch like `agent/<issue>-<slug>`. Don't open feature PRs against `main`.
- The PR **title** must be a Conventional Commit (`feat:`, `fix:`, `perf:`,
  `refactor:`, `docs:`, `!`/`BREAKING CHANGE` for breaking). PRs into `stage`
  are **squashed**, one commit per PR whose subject ends in `(#PR)`; that
  subject is what the changelog and the version are built from.
- Fill in the PR template (`.github/pull_request_template.md`): Summary,
  *How to validate in dev* (where, prerequisites/data, numbered steps, expected
  result, limitations), Tests, Related issue. Use `Refs #N`, never `Closes #N`:
  the issue is closed by the release PR. The owner checks the change in the dev
  instance, which serves `stage`, using only those steps.
- `git fetch` before pushing; other sessions may push too. Never push directly
  to `main` or `stage`.

**Releases.** Only a push to `main` can publish; pushes and PRs on `stage`
run the same CI but never release or tag. Promotion is automatic up to the
release PR; only the owner merges it.
- A release candidate is a frozen `release/vX.Y.Z` branch cut by automation
  from one `stage` SHA whose Backend (Go) and Frontend (Node) checks passed. It
  adds exactly one commit, `docs(changelog): vX.Y.Z`, and is promoted with a PR
  `chore(release): vX.Y.Z` into `main` that lists the original PRs and carries
  the `Closes #N` lines. Agents never merge it and never push to the branch.
  Later `stage` commits wait for the next release. If `main` advances before it
  is merged, the candidate is closed, its branch is archived as
  `stale/release-vX.Y.Z-<sha7>` and a new one is cut.
- The PR is merged by the owner with a **merge commit, never squash or
  rebase** (squashing would erase the per-PR commits the version is computed
  from). The ruleset on `main` allows only that method, requires the Backend
  and Frontend checks to be up to date with `main`, and forbids force-push and
  deletion. GitHub deletes `release/vX.Y.Z` when the PR is merged.
- A release is *published* when CI on `main` creates the tag and the GitHub
  release; it is *deployed* only when the environment that installs it says so.
  Don't treat one as the other.
- After publication, `main` is merged back into `stage` (merge commit
  `chore(release): merge main into stage after vX.Y.Z`; never rebase or
  force-push `stage`). No new candidate is cut while `main` has commits that
  `stage` lacks.
- `scripts/next-version.sh` is the only place that decides versions: `feat` →
  minor, `fix`/`perf` → patch, `!`/`BREAKING CHANGE` → major (minor on 0.x);
  everything else, and merge commits, release nothing. Preview with
  `scripts/next-version.sh` and `scripts/next-version.sh --notes`. A revert
  cancels the change it reverts when both are in the release; a revert it
  cannot interpret is listed under "Needs review" for a human.
- `CHANGELOG.md` is the single changelog and is written **only when a release
  is prepared**: `scripts/update-changelog.sh vX.Y.Z [--date YYYY-MM-DD]`
  prepends one section (idempotent; fails if `vX.Y.Z` is not what
  `next-version.sh` computes). Don't edit it in feature PRs and don't generate
  a changelog elsewhere. On `main`, the release job publishes the version
  `next-version.sh` computes with that version's committed `CHANGELOG.md`
  section as the release notes, and **fails** if the section is missing or is
  not the newest. If it fails, fix it with a `docs(changelog)` change to
  `main` (run `scripts/update-changelog.sh` there); the next run publishes.
- `scripts/changelog-section.sh vX.Y.Z` prints one section (what CI uses).

**Dependabot.** Version updates target `stage` (`.github/dependabot.yml`):
Go modules and npm production dependencies as `fix(deps)` (patch release),
workflow actions as `build(ci)`, npm dev dependencies as `build(deps-dev)`
(no release). *Security* updates are not redirected by `target-branch` and
still open against `main`. Controlled procedure: prefer applying the same bump
on `stage` (retarget the Dependabot PR to `stage`, or make an equivalent
`fix(deps)` PR) and let it ship with the next promotion. Only for an urgent
fix merge it into `main` directly (with a merge commit, the only method the
ruleset allows, once its checks are up to date), then fix the release path:
the `main` release job needs a `docs(changelog)` change on `main` (run
`scripts/update-changelog.sh` there), and `main` must be merged into `stage`
(no rebase, no force-push) before a new candidate can be cut. An open release
PR becomes stale once `main` advances and is closed and re-cut by the
promotion automation.

**Docs and hygiene.**
- Write documentation in English only. Use direct, short sentences and
  task-oriented headings; give prerequisites, steps and expected results when
  explaining a task.
- Document implemented behavior only in user guides and references. Keep
  proposals in explicitly labeled design specs or backlog documents.
- Keep one source for each topic; link to it instead of duplicating content.
- Keep [`docs/README.md`](docs/README.md) and its links accurate as pages are
  added or moved. Create directories only when they contain useful pages;
  don't add placeholder pages.
- Keep unfinished proposals in `docs/development/proposals/`, explicitly labeled.
- Finishing a feature or behavior change: update the matching
  `docs/development/contexts/<context>.md` in the same PR (and README
  "Features" if user-visible). Delete a design spec once it is implemented.
- Never commit: `*.db*`, built binaries in the repo root, `.env.local`,
  `deploy.sh`, keys, real financial data, or host-specific details (domains,
  ports, local paths). `.claude/` is local state.

## Gotchas

- `internal/payables` replaced `internal/debts`/`receivables` (gone).
  Recurring commitments are stored as `Scenario{Kind: recurring}` plus
  `scenario_recurring_schedules`; `internal/recurrences` owns occurrences and
  reconciliation.
- Manual transactions on synced accounts never change provider balances;
  credit-card bills count them. Manual bank accounts derive native balance from
  opening balance + posted undeleted movements + explicit adjustment deltas.
  BRL cash aggregates exclude foreign currencies without rates.
- Manual transactions are soft-deleted (`deleted_at`); category/inclusion event
  tables are append-only with `ON DELETE RESTRICT`.
- Sync runs are claimed atomically with a heartbeat lease, so several instances
  can share one Postgres; SQLite supports a single process only.
