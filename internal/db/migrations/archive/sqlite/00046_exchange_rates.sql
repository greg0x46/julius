-- Decimal rates cached per requested day; effective_on records the provider date.

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
