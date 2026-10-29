# `.specs/` — navigation guide

Entry point for anyone (human or LLM) who needs to understand the domain
before changing code. Agent rules and commands: [`/AGENTS.md`](../AGENTS.md).

## Reading order

1. [Project README](../README.md).
2. [Development references](../docs/development/README.md): architecture,
   financial invariants, monetary precision, API contracts and context decisions.
3. The relevant `contextos/<context>/reference.md` below for current feature
   packages, routes and screens. Update it with behavior changes during migration.

The financial references and shared decision audit have moved; their old paths
remain forwarding pages. This directory retains feature inventories, historical
schema conversion evidence and explicitly unfinished proposals until separate
migration/cleanup work. Do not treat proposed behavior as implemented.

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
| `patrimonio-liquido` | Wealth (patrimônio; folder name is historical) | `networth` |
| `metas` | Goals (target vs. a global metric) | `goals` |
| `investimentos` | Investments, quotes, reconciliation | `investments`, `quotes`, `marketdata` |
| `configuracoes-autenticacao` | Settings, authentication, HTTP access log | `auth`, `settings`, `httplog` |

## Shared contracts

- [API contracts](../docs/development/api-contracts.md): wire semantics,
  calculation ownership and shared Go/TypeScript fixtures.
- [Architecture](../docs/development/architecture.md): domain boundaries,
  database decoding and cross-cutting principles.
- [Monetary precision](../docs/development/monetary-precision.md): exact input,
  operation-specific rounding and native-currency display.
- [Schema inventory](reorganizacao-schema/inventario.md): historical snapshots
  and conversion mapping. The current storage and conversion contract belongs to
  the [migration reference](../internal/db/migrations/README.md).
- [Statement import extensions](importacao-extratos.md) and
  [spending-cap backlog](backlog/teto-gasto-diario.md) retain proposals, including
  superseded ideas; use context references for implemented behavior.

## Shared frontend building blocks

Not tied to a single context:

- `frontend/src/components/shared/` — `Money`, `StatusTag`, `RecordMenu`,
  `BottomSheet`, `PanelStack`, `ConditionListEditor`; `components/AsyncState.tsx`
  for loading/error/empty states.
- `frontend/src/theme/tokens.json` — single source of design tokens; `npm run gen:tokens` generates `src/styles/tokens.css` and `src/theme/tokens.ts` (do not edit). Colour roles have light and dark values (dark is neutral, GitHub-style greys); ink follows the theme. The app bar is `--color-app-bar` with a 1px primary line. The header switch (`app/theme.ts`) sets `data-theme` and saves the choice in `localStorage`; without one the theme follows the OS. Generated CSS keeps native controls in the same colour scheme, including saved overrides of the OS theme. The demo notice uses themed surface and border roles. Incoming money is green, outgoing money and negative results are neutral text with an explicit sign, and red is only for errors, overdue items and negative account balances.
  Scales: type `--fs-caption/body/row/title/hero` (12/14/16/20/32px) with `--lh-*`, radius `--radius-sm/md/lg/full`, spacing `--space-1..7`. CSS must use them for `font-size` and `border-radius` (guard: `src/styles/scales.test.ts`).
- `frontend/src/components/filters/filterUrl.ts` — filter ⇄ `URLSearchParams`
  (transaction filters, scenario selection).
- `frontend/src/presentation/money.ts` — money formatting/arithmetic on decimal
  strings.
- `frontend/src/components/ui/` — local HTML/CSS React controls, SVG icons,
  semantic tables, native dialog panels and browser popovers. `styles/ui.css`
  defines their shared appearance; page styles build on local `ui-*` classes.
- `components/ui/LinePlot.tsx` draws Home/net-worth series in responsive SVG;
  category breakdown uses SVG rings. Exact decimal strings remain the source
  for tooltips and financial labels; numeric conversion is only for geometry.
- `presentation/calendar.ts` owns the frontend's small local calendar helper.
  Forms use native date inputs; date-only strings stay local calendar days,
  while timestamps follow the browser timezone. React Router, TanStack Query
  and the existing API transport retain routing, caching and session handling.

Tests: Vitest + Testing Library, colocated `*.test.ts(x)`. Vitest runs two
projects with 2 workers: `dom` (jsdom + `src/test/setup.dom.ts`) for
`*.test.tsx` and the `.ts` files listed in `domTsTests` in `vite.config.ts`, and
`node` (no DOM) for the other `*.test.ts`. A new `.ts` test that needs the DOM
must be added to `domTsTests` (or become `.tsx`); details in
`frontend/TESTING.md`. Playwright financial journeys run with `npm run test:e2e`
against disposable SQLite databases; install Chromium first as described there.

## Workflow for new work

- **New feature with design decisions worth recording:** create
  `.specs/<feature>.md` (split into `.specs/<feature>/mN-*.md` for independent
  milestones). State at the top whether it is implemented.
- **When a feature ships (new or changed):** update or create the context's
  `reference.md` in the same PR, delete the design spec that planned it, and
  update the "Features" section of `/README.md` if it is user-visible.
- **Small, local change inside a documented context:** just update the existing
  `reference.md`.
