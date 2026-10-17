-- +goose Up
-- Goals replace the investment grouping (investment_portfolios and its position
-- links): a goal now tracks one global metric against a target instead of a
-- hand-picked set of positions. The metric CHECK matches the SQLite one, which
-- already lists the metrics the domain will add later; only the metrics
-- implemented in internal/goals are accepted on write.
DROP TABLE investment_position_portfolios;
DROP TABLE investment_portfolios;

CREATE TABLE goals (
    id            UUID NOT NULL PRIMARY KEY,
    name          TEXT NOT NULL,
    metric        TEXT NOT NULL CHECK (metric IN ('invested_value', 'cash_balance', 'net_worth')),
    target_amount NUMERIC NOT NULL CHECK (target_amount > 0),
    target_date   DATE,
    notes         TEXT,
    created_at    TIMESTAMPTZ NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL
,
    CHECK (target_amount NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric))
);

-- +goose Down
DROP TABLE goals;

CREATE TABLE investment_portfolios (
    id UUID NOT NULL PRIMARY KEY,
    name TEXT NOT NULL,
    target_amount NUMERIC,
    target_date DATE,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
,
    CHECK (target_amount NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric))
);

CREATE TABLE investment_position_portfolios (
    manual_position_id UUID UNIQUE REFERENCES investment_positions (id) ON DELETE CASCADE,
    financial_investment_id UUID UNIQUE REFERENCES financial_investments (id) ON DELETE CASCADE,
    portfolio_id UUID NOT NULL REFERENCES investment_portfolios (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CHECK (
        (manual_position_id IS NOT NULL AND financial_investment_id IS NULL) OR
        (manual_position_id IS NULL AND financial_investment_id IS NOT NULL)
    )
);

CREATE INDEX ix_investment_position_portfolios_portfolio_id ON investment_position_portfolios (portfolio_id);
