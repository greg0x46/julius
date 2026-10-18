# Goals

> Living reference — what exists and works today. Keep it updated when the
> feature changes. Not a domain engine (see "Not engines" in
> `.specs/motores-de-dominio.md`).

## What it is

A goal is a target amount (and optional date) over one **global metric**. It
stores only the target; the current value and progress are recomputed on every
read from the engine that owns the figure, never persisted, and a goal never
adds to any total.

Goals replaced the old investment "objetivos" (grouping of positions): there is
no per-position or per-account goal, and positions carry no goal reference.

## State vs. ideal

Only the metric `invested_value` is accepted. The schema CHECK already admits
`cash_balance` and `net_worth` (SQLite cannot widen a CHECK without rebuilding
the table), but the domain rejects them until they are implemented. Not
implemented: other metrics, projecting portfolio appreciation toward the target
date, and a history/chart of progress.

Goals are a separate package rather than a `payables.Kind`: a payable is paid
down by *flows* linked to real transactions; a goal measures a *stock* owned by
another engine. See `motores-de-dominio.md` §4.

## Backend

`internal/goals` — CRUD plus the computed read model.

- `invested_value`: `networth.InvestmentValue` (the `InvestmentBalance` line of
  `networth.Compute`, without valuing cash or cards) — the market value
  of every investment, gains and losses included. It is the investments line of
  net worth, so linked custody cash (already bank cash) is not counted twice and
  foreign holdings are valued in BRL with the day's rate; a missing rate on a
  foreign holding makes the read fail instead of omitting it, but unrelated
  foreign cash without a rate does not.
- `progress = current_value / target_amount`; it may exceed 1 once reached.
- Target must be strictly positive; name required. Everything is in BRL and is
  **not** converted to the reference currency.
- A list reads the metric once, so all goals in a response agree.
- Create and update commit only after computing the response progress; a
  valuation failure leaves the stored goal unchanged.

Table `goals` (migration `0005_goals`, both dialects): `id`, `name`, `metric`,
`target_amount`, `target_date`, `notes`, `created_at`, `updated_at`. The same
migration drops `investment_portfolios` and `investment_position_portfolios`;
no data is carried over.

## HTTP

`GET /api/goals`, `POST /api/goals`, `PUT /api/goals/{id}`,
`DELETE /api/goals/{id}`. A goal DTO carries `id`, `name`, `metric`,
`target_amount`, `target_date`, `notes`, `current_value`, `progress`,
`currency_code`, `created_at`, `updated_at` (money as decimal strings).
Problems: `invalid-goal-input` (400), `goal-not-found` (404),
`exchange-unavailable` / `goals-unavailable` (503).

## Frontend

Page **Metas** at `/metas`: list of goals with a progress bar, current value and
target, create/edit/delete. All HTTP goes through `src/api`; the page shows the
backend `current_value` and `progress` as-is.
Completion requires `progress >= 1` using exact decimal comparison. The whole
percentage and bar stay at most 99% below that boundary; reached goals fill the
bar while their displayed percentage may exceed 100%.
