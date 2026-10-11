-- Exchange rates cached by internal/exchange, so reports can convert between
-- currencies without asking a provider on every read. Original amounts and
-- currencies elsewhere are never rewritten from this table.
--
-- One row per pair per requested day (as_of). effective_on is the day the
-- provider's rate actually refers to: for a weekend or holiday it is the last
-- business day before as_of. rate is a decimal string meaning
-- 1 base_currency = rate quote_currency; fetched_at says whether the row may
-- still change (see internal/exchange).

-- +goose Up

CREATE TABLE exchange_rates (
    base_currency TEXT NOT NULL,
    quote_currency TEXT NOT NULL,
    as_of TEXT NOT NULL,
    effective_on TEXT NOT NULL,
    rate TEXT NOT NULL,
    source TEXT NOT NULL,
    fetched_at TEXT NOT NULL,
    PRIMARY KEY (base_currency, quote_currency, as_of)
);

-- +goose Down

DROP TABLE exchange_rates;
