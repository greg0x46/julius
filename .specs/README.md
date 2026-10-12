# `.specs/` — navigation guide

Entry point for anyone (human or LLM) who needs to understand the domain
before changing code. Agent rules and commands: [`/AGENTS.md`](../AGENTS.md).

## Reading order

1. [`/README.md`](../README.md) — what Julius is, how to run it, stack.
2. This file — map of the folder.
3. [`motores-de-dominio.md`](motores-de-dominio.md) — reusable domain
   abstractions ("domain engines") and the principles connecting them.
4. [`invariantes-financeiros.md`](invariantes-financeiros.md) — the core
   financial rules as they are today, with the owning module and the test that
   pins each one.
5. `contextos/<context>/reference.md` — what exists and works today, feature by
   feature. Start with the context you will touch.
6. A design spec for that context, if one exists (linked from its
   `reference.md`). Currently only [`importacao-extratos.md`](importacao-extratos.md).

## Doc types

- **`motores-de-dominio.md`** — architecture, not features. Describes the domain
  at the ideal level; where code diverges, the divergence is stated explicitly.
- **`invariantes-financeiros.md`** — cross-cutting, living matrix: balance,
  income/expense, transfer, obligations, investments, hypotheticals and
  reprocessing — each rule's current behavior, owning module and pinning test
  (cross-engine tests live in `internal/invariants`). Found-but-undecided
  divergences are listed at its end. Update in the same PR that changes any of
  these rules.
- **`contextos/<context>/reference.md`** — living docs of the *current, literal*
  state: packages, HTTP routes, frontend pages. The source of truth for "is
  this implemented?". Update in the same PR as the change; a stale reference
  is worse than none.
- **`<feature>.md`** (optionally `<feature>/mN-*.md`) — design specs for work
  in progress: motivation, rejected alternatives, decisions, milestone plan.
  State at the top whether it is implemented. When the feature ships, move the
  relevant content into the context's `reference.md` and delete the spec, so
  two documents never describe the same state.
- **`backlog/`** — ideas, explicitly not to be implemented yet.

## Context glossary

Folder names are Portuguese and referenced from Go comments; they are not
renamed. Frontend routes and UI labels are also Portuguese.

| Folder | Topic | Main packages |
|---|---|---|
| `sincronizacao-open-banking` | Pluggy sync, connections | `pluggy`, `syncsvc`, `worker`, `datasources` |
| `importacao-extratos` | CSV statement import | `statementimport` |
| `transacoes` | Ledger, manual entries, accounts/cards | `transactions`, `money`, `ledger` |
| `categorias` | Categories, categorization origin/precedence | `categories` |
| `automacao` | Automation rules | `rules`, `automation` |
| `pendencias` | Payables (debts, receivables) | `payables` |
| `cenarios` | Scenarios (hypothetical layer) | `scenarios`, `projections` |
| `recorrencias` | Recurring commitments + reconciliation | `recurrences` |
| `home` | Home dashboard, projection | `timeline` |
| `patrimonio-liquido` | Net worth | `networth` |
| `investimentos` | Investments, quotes, reconciliation | `investments`, `quotes`, `marketdata` |
| `configuracoes-autenticacao` | Settings, authentication | `auth`, `settings` |

## Shared database read boundary

`internal/db/types` decodes timestamps, calendar dates, booleans, identifiers
and exact decimals returned by SQLite and Postgres. Stores and SQL-backed HTTP
handlers use its scan adapters, including nullable columns; native driver values
are normalized there before domain logic runs. Opaque provider and legacy IDs
remain intact; `ParseUUID` is available for UUID values. Decimal reads accept
text and integers and reject floating-point values. Postgres uses native exact
numeric, temporal, UUID and boolean types; SQLite keeps text decimals, UTC
instants, calendar dates and integer flags. Both start at `0001_initial.sql`.
`db.Open` rejects the old schema before running migrations; the archived history
is embedded only in historical tests. See
[`migrations/README.md`](../internal/db/migrations/README.md) for the contract.

## Cross-cutting design work

- [`reorganizacao-schema/inventario.md`](reorganizacao-schema/inventario.md) —
  final legacy schema snapshots, complete column inventory, data/auth scope
  decisions and the proposed changed-column conversion map. The native baseline
  is implemented; the legacy conversion tool remains unimplemented.

## Shared frontend building blocks

Not tied to a single context:

- `frontend/src/components/shared/` — `Money`, `StatusTag`, `RecordMenu`,
  `BottomSheet`, `PanelStack`, `ConditionListEditor`; `components/AsyncState.tsx`
  for loading/error/empty states.
- `frontend/src/theme/tokens.ts` — colors, font, spacing, antd theme.
- `frontend/src/components/filters/filterUrl.ts` — filter ⇄ `URLSearchParams`
  (transaction filters, scenario selection).
- `frontend/src/presentation/money.ts` — money formatting/arithmetic on decimal
  strings.
- Recharts is the chart library (Home projection, net worth).

Tests: Vitest + Testing Library, colocated `*.test.ts(x)`. Vitest runs two
projects with 2 workers: `dom` (jsdom + `src/test/setup.dom.ts`) for
`*.test.tsx` and the `.ts` files listed in `domTsTests` in `vite.config.ts`, and
`node` (no DOM) for the other `*.test.ts`. A new `.ts` test that needs the DOM
must be added to `domTsTests` (or become `.tsx`); details in
`frontend/TESTING.md`. Playwright is scaffolded (`test:e2e`) without config or
tests — don't expect it to work.

## Workflow for new work

- **New feature with design decisions worth recording:** create
  `.specs/<feature>.md` (split into `.specs/<feature>/mN-*.md` for independent
  milestones). State at the top whether it is implemented.
- **When a feature ships (new or changed):** update or create the context's
  `reference.md` in the same PR, delete the design spec that planned it, and
  update the "Features" section of `/README.md` if it is user-visible.
- **Small, local change inside a documented context:** just update the existing
  `reference.md`.
