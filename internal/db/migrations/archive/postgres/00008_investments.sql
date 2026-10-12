-- +goose Up

-- Investments (holdings) and their transaction history, mirroring
-- financial_accounts/financial_transactions: same current_raw_import_id +
-- normalized_hash change-detection pattern.
CREATE TABLE financial_investments (
    id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL REFERENCES data_sources (id),
    external_id TEXT NOT NULL,
    investment_type TEXT,
    subtype TEXT,
    name TEXT,
    balance TEXT,
    currency_code TEXT,
    number TEXT,
    owner TEXT,
    tax_number TEXT,
    due_date TEXT,
    issuer TEXT,
    issuer_code TEXT,
    rate TEXT,
    rate_type TEXT,
    fixed_annual_rate TEXT,
    annual_rate TEXT,
    last_twelve_months_rate TEXT,
    quantity TEXT,
    value TEXT,
    amount TEXT,
    amount_profit TEXT,
    amount_withdrawal TEXT,
    as_of_date TEXT,
    provider_updated_at TEXT,
    isin TEXT,
    code TEXT,
    provider_status TEXT,
    current_raw_import_id TEXT NOT NULL REFERENCES raw_imports (id),
    normalized_hash TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (source_id, external_id)
);

CREATE TABLE financial_investment_transactions (
    id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL REFERENCES data_sources (id),
    investment_id TEXT NOT NULL REFERENCES financial_investments (id),
    external_id TEXT NOT NULL,
    movement_type TEXT,
    quantity TEXT,
    value TEXT,
    amount TEXT,
    occurred_at TEXT,
    trade_date TEXT,
    current_raw_import_id TEXT NOT NULL REFERENCES raw_imports (id),
    normalized_hash TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (source_id, external_id)
);

CREATE INDEX ix_financial_investment_transactions_investment_id ON financial_investment_transactions (investment_id);
-- See the NULLS LAST note on ix_financial_transactions_occurred_at_id_desc
-- in 00001_core_sync.sql — same reasoning applies here.
CREATE INDEX ix_financial_investment_transactions_occurred_at_id_desc ON financial_investment_transactions (
    occurred_at DESC NULLS LAST, id DESC
);

-- sync_failures.investment_id and normalization_events.investment_id /
-- investment_transaction_id are declared without a FK in 00001_core_sync.sql
-- because financial_investments/financial_investment_transactions don't
-- exist yet at that point — added here now that they do.
ALTER TABLE sync_failures
    ADD CONSTRAINT sync_failures_investment_id_fkey
    FOREIGN KEY (investment_id) REFERENCES financial_investments (id);

ALTER TABLE normalization_events
    ADD CONSTRAINT normalization_events_investment_id_fkey
    FOREIGN KEY (investment_id) REFERENCES financial_investments (id);

ALTER TABLE normalization_events
    ADD CONSTRAINT normalization_events_investment_transaction_id_fkey
    FOREIGN KEY (investment_transaction_id) REFERENCES financial_investment_transactions (id);

-- +goose Down

ALTER TABLE normalization_events DROP CONSTRAINT normalization_events_investment_transaction_id_fkey;
ALTER TABLE normalization_events DROP CONSTRAINT normalization_events_investment_id_fkey;
ALTER TABLE sync_failures DROP CONSTRAINT sync_failures_investment_id_fkey;

DROP TABLE financial_investment_transactions;
DROP TABLE financial_investments;
