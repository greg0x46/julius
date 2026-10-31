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
  strings. Currency amounts (`formatMoney`, `formatBRL`) keep the Brazilian
  format ("R$ 1.234,56") in every UI language on purpose; only chart axis
  scales (`formatAxisMoney`) and decimals/percentages follow the language.
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


## Translation catalogs

`frontend/src/i18n/messages.ts` combines the Portuguese (`pt-BR`, default) and
English (`en`) catalogs. Messages live in one file per area under
`frontend/src/i18n/catalogs/` (common, settings, accounts, transactions, ...),
each with both languages side by side. A key belongs to exactly one area; the
catalog test checks matching keys and placeholders per area. `I18nProvider` wraps the application;
`useTranslation()` returns `t(key)`, `language` and `setLanguage(language)`.
Keys are checked by TypeScript. The catalog test checks matching keys.
Add messages to both languages of the right area and call `t` during rendering for reactive text.

The header selector updates the document `lang` and saves `julius.language`
in browser storage. Other tabs follow storage changes. Invalid or unavailable
storage defaults to Portuguese; switching still works when saving is blocked.
This preference belongs to the browser, independently of authentication.

Every page and shared control is translated: navigation, login, settings,
Home, accounts, transactions, payables, recurring commitments, automations,
scenarios, investments, goals, net worth and statement import. Code outside
React (API client errors) uses `apiMessage(key)` from `i18n/apiMessage.ts`,
which reads the selected language when the error is created. Presentation
helpers take an optional `language` argument that defaults to `pt-BR`. Server
messages (problem details, import warnings, asset class labels) and the user's
own data stay as the server or the user wrote them. 
Date and time formats come from `frontend/src/i18n/dateFormats.ts`, which
defines each language's conventions (Intl locale, day/month order, short month
names) and cached `Intl` formatters. Use `presentation/dates.ts` (`formatDay`,
`formatDateOnly`, `formatCompactDay`, ...) and pass the `language` from
`useTranslation()`; do not build locale-specific date strings or call
`Intl.DateTimeFormat` with a hard-coded locale. Adding a language means adding
its entry there. Language does not change financial reference
currency, stored data or API values.
