# Goals

> Living reference — what exists and works today. Keep it updated when the
> feature changes. Not a domain engine (see "Not engines" in
> `docs/development/architecture.md`).

## What it is

A goal is a target amount (and optional date) over one **global metric**. It
stores only the target; the current value and progress are recomputed on every
read from the engine that owns the figure, never persisted, and a goal never
adds to any total.

Goals replaced the old investment "objetivos" (grouping of positions): there is
no per-position or per-account goal, and positions carry no goal reference.

Goal labels, controls and projections follow the browser language selector.
See the [frontend reference](../frontend.md) for overall translation coverage.

## State vs. ideal

Only the metric `invested_value` is accepted. The schema CHECK already admits
`cash_balance` and `net_worth` (SQLite cannot widen a CHECK without rebuilding
the table), but the domain rejects them until they are implemented. Not
implemented: other metrics and a history/chart of progress.

Goals are a separate package rather than a `payables.Kind`: a payable is paid
down by *flows* linked to real transactions; a goal measures a *stock* owned by
another engine. See `docs/development/architecture.md` (Domain engines).

## Backend

`internal/goals` — CRUD plus the computed read model.

- `invested_value`: `networth.InvestmentValue` (the `InvestmentBalance` line of
  `networth.Compute`, without valuing cash) — the market value
  of every investment, gains and losses included. It is the investments line of
  wealth, so linked custody cash (already bank cash) is not counted twice and
  foreign holdings are valued in BRL with the day's rate; a missing rate on a
  foreign holding makes the read fail instead of omitting it, but unrelated
  foreign cash without a rate does not.
- `progress = current_value / target_amount`; it may exceed 1 once reached.
- Target must be strictly positive; name required. Everything is in BRL and is
  **not** converted to the reference currency.
- A list reads the metric once, so all goals in a response agree.
- Create and update commit only after computing the response progress; a
  valuation failure leaves the stored goal unchanged.

### Projection

A goal with a **future** `target_date` carries a `projection` (null without a
date, or when the date is today or past). It is computed on read, never stored,
and assumes no new contributions other than the one it solves for.

- **Monthly return.** The average appreciation or depreciation of the
  investments: `investments.BuildPortfolio` over the trailing 12 months ending
  today (Modified Dietz, so contributions and withdrawals are not counted as
  yield). Dietz averages capital over the whole window, so when the portfolio
  held capital for only part of it the rate is rescaled to the history held
  (from `PortfolioPosition.CapitalSince`). That period return is compounded to a
  monthly rate: `(1 + R)^(30.4375 / days) − 1`. Negative when the portfolio lost
  value. The same rate applies to every goal in a response (measured once).
- **Not enough history.** Under 30 days of capital, no measurable position, or a
  total loss gives `monthly_return: null`; the projection then assumes **zero**
  appreciation (the contribution is the gap split evenly) and the UI says so.
  `return_partial` flags positions left out of the return as unavailable.
- **Horizon.** `months_remaining` counts calendar months from today to the
  target date, a partial last month counting as one; horizons over 1200 months
  yield no projection.
- **Projected value** = `current_value × (1 + r)^n`; `projected_progress` is it
  over the target (it may exceed 1).
- **Required contribution** is the end-of-month deposit `C` such that
  `current_value × (1 + r)^n + C × ((1 + r)^n − 1) / r = target` (`gap / n` when
  `r` is zero). Exact decimal, not rounded; the UI rounds on display.
- **Status**: `reached` (current value already at or above the target),
  `on_track` (appreciation alone reaches it; contribution `0`), `behind`.

The return is the portfolio's historical average and the contribution assumes
it repeats; it is an estimate, not a promise, and the card says so. It measures
investments with a period yield, so it can differ from `invested_value` (which
also covers positions without history).

Table `goals` (migration `0005_goals`, both dialects): `id`, `name`, `metric`,
`target_amount`, `target_date`, `notes`, `created_at`, `updated_at`. The same
migration drops `investment_portfolios` and `investment_position_portfolios`;
no data is carried over.

## HTTP

`GET /api/goals`, `POST /api/goals`, `PUT /api/goals/{id}`,
`DELETE /api/goals/{id}`. A goal DTO carries `id`, `name`, `metric`,
`target_amount`, `target_date`, `notes`, `current_value`, `progress`,
`projection`, `currency_code`, `created_at`, `updated_at` (money as decimal
strings). `projection` is null or `{status, months_remaining, monthly_return,
return_from, return_partial, projected_value, projected_progress,
required_monthly_contribution}`; `monthly_return` and `return_from` are null
without enough history.
Problems: `invalid-goal-input` (400), `goal-not-found` (404),
`exchange-unavailable` / `goals-unavailable` (503).

## Frontend

Page **Metas** at `/metas`: a two-column grid (one on phone) of goal cards,
open goals first and reached ones last, each group by ascending target amount.
A card shows icon, name, metric and "···" menu; progress ring, current value, target and a status tag; target date with
months left, or "Sem data-alvo definida". A dashed "Adicionar nova meta"
tile ends the grid; create/edit/delete. All HTTP goes through `src/api`; the page shows the
backend `current_value`, `progress` and `projection` as-is. A dated goal shows
the monthly contribution needed ("Nenhum" when appreciation alone is enough)
plus the average monthly return and the projected value (and percent of the
target) on the date with no new contributions.
Completion requires `progress >= 1` using exact decimal comparison. The whole
percentage and ring stay at most 99% below that boundary; reached goals fill the
ring while their displayed percentage may exceed 100%.

## MCP reads

`list_goals` paginates `goals.List` using the REST goal formatter. Current
value, progress, target-date projection, required monthly contribution and
partial/missing return history come from the same engines. Goals stay BRL and
exclude hypothetical scenarios; the disclosed assumptions retain zero
appreciation when no monthly return can be measured.

Tool inputs, output bounds and dev validation: [MCP tool reference](../mcp-tools.md).

## MCP writes

`create_goal`, `update_goal` and `delete_goal` require `mcp:write` plus fresh
owner approval in Julius. Creation/editing use the existing goal request
conversion, `goals.Create`/`Update` and REST formatter; updates replace fields.
Positive exact BRL targets and calendar dates are validated before preview.
Progress and target-date projections stay derived from realized investments.
Deleting a goal removes only its target. Stored before-state is rechecked before
application; unknown IDs fail safely. Notes are bounded to 4,000 characters.

Synthetic lifecycle, exact amount, derived progress, stale/invalid input and
OAuth/approval tests: `internal/httpapi/mcp_planning_writes_test.go` and
`internal/mcp/planning_writes_test.go` (SQLite and Postgres).
