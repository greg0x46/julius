-- +goose Up
-- Goals replace the investment grouping (investment_portfolios and its position
-- links): a goal now tracks one global metric against a target instead of a
-- hand-picked set of positions. The metric CHECK already lists the metrics the
-- domain will add later, because SQLite cannot widen a CHECK without
-- rebuilding the table; only the metrics implemented in internal/goals are
-- accepted on write.
DROP INDEX ix_investment_position_portfolios_portfolio_id;
DROP TABLE investment_position_portfolios;
DROP TABLE investment_portfolios;

CREATE TABLE goals (
    id            TEXT NOT NULL PRIMARY KEY,
    name          TEXT NOT NULL,
    metric        TEXT NOT NULL CHECK (metric IN ('invested_value', 'cash_balance', 'net_worth')),
    target_amount TEXT NOT NULL CHECK ((target_amount NOT GLOB '*[^0-9.-]*' AND target_amount GLOB '*[0-9]*' AND length(target_amount) - length(replace(target_amount, '.', '')) <= 1 AND instr(substr(target_amount, 2), '-') = 0 AND (substr(target_amount, 1, 1) <> '-' AND ltrim(target_amount, '-0.') <> ''))),
    target_date   TEXT,
    notes         TEXT,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);

-- +goose Down
DROP TABLE goals;

CREATE TABLE investment_portfolios (
    id TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL,
    target_amount TEXT,
    target_date TEXT,
    notes TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE investment_position_portfolios (
    manual_position_id TEXT UNIQUE REFERENCES investment_positions (id) ON DELETE CASCADE,
    financial_investment_id TEXT UNIQUE REFERENCES financial_investments (id) ON DELETE CASCADE,
    portfolio_id TEXT NOT NULL REFERENCES investment_portfolios (id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    CHECK (
        (manual_position_id IS NOT NULL AND financial_investment_id IS NULL) OR
        (manual_position_id IS NULL AND financial_investment_id IS NOT NULL)
    )
);

CREATE INDEX ix_investment_position_portfolios_portfolio_id ON investment_position_portfolios (portfolio_id);
