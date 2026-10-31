# Shared frontend building blocks

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
  Forms use native date inputs; date-only strings stay calendar days, while
  timestamps, "today" and month boundaries follow the timezone from the
  preferences, never the browser's own. `presentation/timezone.ts` holds the
  zone-aware helpers, `useTimezone()` returns the configured zone, and
  `TimezoneProvider` holds the first paint until the preferences arrive (a
  failed request falls back to the browser zone). Changing the zone drops every
  cached query that depends on it (`discardAfterTimezoneChange`). React Router,
  TanStack Query and the existing API transport retain routing, caching and
  session handling.

Tests: Vitest + Testing Library, colocated `*.test.ts(x)`. Vitest runs two
projects with 2 workers: `dom` (jsdom + `src/test/setup.dom.ts`) for
`*.test.tsx` and the `.ts` files listed in `domTsTests` in `vite.config.ts`, and
`node` (no DOM) for the other `*.test.ts`. A new `.ts` test that needs the DOM
must be added to `domTsTests` (or become `.tsx`); details in
`frontend/TESTING.md`. Playwright financial journeys run with `npm run test:e2e`
against disposable SQLite databases; install Chromium first as described there.

