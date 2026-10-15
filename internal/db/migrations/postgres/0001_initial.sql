-- +goose Up

CREATE TABLE app_auth (
    id INTEGER NOT NULL PRIMARY KEY CHECK (id = 1),
    salt TEXT NOT NULL,
    verifier TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE auth_owner (
    id INTEGER NOT NULL PRIMARY KEY CHECK (id = 1),
    email TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    revision INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE automation_rules (
    id UUID NOT NULL PRIMARY KEY,
    name TEXT NOT NULL CHECK (trim(name) <> ''),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    logic_operator TEXT NOT NULL CHECK (logic_operator IN ('and', 'or')),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE categories (
    id UUID NOT NULL PRIMARY KEY,
    name TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('expense', 'income', 'transfer')),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
, icon TEXT NOT NULL DEFAULT 'ellipsis', color TEXT NOT NULL DEFAULT '#6c757d');

CREATE TABLE data_sources (
    id UUID NOT NULL PRIMARY KEY,
    provider TEXT NOT NULL,
    external_item_id TEXT NOT NULL,
    display_name TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL, is_active BOOLEAN NOT NULL DEFAULT TRUE, label TEXT,
    UNIQUE (provider, external_item_id)
);

CREATE TABLE exchange_rates (
    base_currency TEXT NOT NULL,
    quote_currency TEXT NOT NULL,
    as_of DATE NOT NULL,
    effective_on DATE NOT NULL,
    rate NUMERIC NOT NULL,
    source TEXT NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (base_currency, quote_currency, as_of)
,
    CHECK (base_currency ~ '^[A-Z]{3}$'),
    CHECK (quote_currency ~ '^[A-Z]{3}$'),
    CHECK (rate NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric))
);

CREATE TABLE investment_assets (
    id UUID NOT NULL PRIMARY KEY,
    canonical_key TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    ticker TEXT,
    asset_type TEXT NOT NULL,
    currency_code TEXT NOT NULL DEFAULT 'BRL',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
, quote_source TEXT, quote_symbol TEXT, asset_class TEXT NOT NULL DEFAULT 'other'
    CHECK (asset_class IN ('fixed_income', 'variable_income', 'multimarket', 'currency', 'crypto', 'other')),
    CHECK (currency_code ~ '^[A-Z]{3}$')
);

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

CREATE TABLE net_worth_snapshots (
    id                    UUID NOT NULL PRIMARY KEY,
    captured_at           DATE NOT NULL UNIQUE,
    total_assets          NUMERIC NOT NULL,
    total_liabilities     NUMERIC NOT NULL,
    net_worth             NUMERIC NOT NULL,
    cash_balance          NUMERIC NOT NULL,
    investment_balance    NUMERIC NOT NULL,
    credit_card_balance   NUMERIC NOT NULL,
    payables_debt         NUMERIC NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL,
    updated_at            TIMESTAMPTZ NOT NULL
, is_backfilled BOOLEAN NOT NULL DEFAULT FALSE,
    CHECK (total_assets NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (total_liabilities NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (net_worth NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (cash_balance NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (investment_balance NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (credit_card_balance NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (payables_debt NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric))
);

CREATE TABLE payables (
    id                      UUID NOT NULL PRIMARY KEY,
    kind                    TEXT NOT NULL CHECK (kind IN ('debt', 'receivable')),
    name                    TEXT NOT NULL,
    total_amount            NUMERIC NOT NULL CHECK (total_amount > 0),
    starting_settled_amount NUMERIC NOT NULL DEFAULT '0' CHECK (starting_settled_amount >= 0),
    created_at              TIMESTAMPTZ NOT NULL,
    updated_at              TIMESTAMPTZ NOT NULL
,
    CHECK (total_amount NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (starting_settled_amount NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric))
);

CREATE TABLE settings (
    key TEXT NOT NULL PRIMARY KEY,
    value TEXT,
    is_encrypted BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE auth_sessions (
    token_hash TEXT NOT NULL PRIMARY KEY,
    owner_id INTEGER NOT NULL REFERENCES auth_owner (id) ON DELETE RESTRICT,
    revision INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE automation_rule_conditions (
    id       UUID NOT NULL PRIMARY KEY,
    rule_id  UUID NOT NULL REFERENCES automation_rules (id) ON DELETE CASCADE,
    field    TEXT NOT NULL CHECK (field IN ('description', 'card', 'account', 'amount', 'day_of_month')),
    operator TEXT NOT NULL CHECK (operator IN ('contains', 'equals', 'within_percent', 'day_range')),
    value    TEXT NOT NULL CHECK (trim(value) <> ''),
    position INTEGER NOT NULL CHECK (position >= 0),
    UNIQUE (rule_id, position)
);

CREATE TABLE investment_asset_quote_coverage (
    asset_id UUID NOT NULL PRIMARY KEY REFERENCES investment_assets (id) ON DELETE CASCADE,
    source TEXT NOT NULL,
    symbol TEXT NOT NULL,
    covered_from DATE NOT NULL,
    covered_to DATE NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE scenarios (
    id                   UUID NOT NULL PRIMARY KEY,
    kind                 TEXT NOT NULL CHECK (kind IN ('debt_plan', 'receivable_plan', 'standalone', 'recurring')),
    name                 TEXT NOT NULL,
    payable_id           UUID REFERENCES payables (id) ON DELETE CASCADE,
    is_active            BOOLEAN NOT NULL DEFAULT TRUE,
    is_accounting_source BOOLEAN NOT NULL DEFAULT FALSE,
    created_at           TIMESTAMPTZ NOT NULL,
    updated_at           TIMESTAMPTZ NOT NULL,
    CHECK (is_accounting_source = FALSE OR kind IN ('debt_plan', 'receivable_plan')),
    CHECK (
        (kind IN ('standalone', 'recurring') AND payable_id IS NULL) OR
        (kind IN ('debt_plan', 'receivable_plan') AND payable_id IS NOT NULL)
    )
);

CREATE TABLE sync_runs (
    id UUID NOT NULL PRIMARY KEY,
    source_id UUID NOT NULL REFERENCES data_sources (id) ON DELETE RESTRICT,
    status TEXT NOT NULL DEFAULT 'in_progress'
        CHECK (status IN ('in_progress', 'completed', 'completed_with_failures', 'failed')),
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ,
    heartbeat_at TIMESTAMPTZ,
    worker_id TEXT,
    accounts_processed INTEGER NOT NULL DEFAULT 0 CHECK (accounts_processed >= 0),
    transactions_inserted INTEGER NOT NULL DEFAULT 0 CHECK (transactions_inserted >= 0),
    transactions_updated INTEGER NOT NULL DEFAULT 0 CHECK (transactions_updated >= 0),
    general_error_code TEXT,
    general_error_message TEXT, investments_processed INTEGER NOT NULL DEFAULT 0, investment_transactions_inserted INTEGER NOT NULL DEFAULT 0, investment_transactions_updated INTEGER NOT NULL DEFAULT 0, bills_processed INTEGER NOT NULL DEFAULT 0, run_type TEXT NOT NULL DEFAULT 'sync'
    CHECK (run_type IN ('sync', 'file_import')),
    CHECK (
        (status = 'in_progress' AND finished_at IS NULL) OR
        (status <> 'in_progress' AND finished_at IS NOT NULL)
    )
);

CREATE TABLE automation_rule_actions (
    id          UUID NOT NULL PRIMARY KEY,
    rule_id     UUID NOT NULL REFERENCES automation_rules (id) ON DELETE CASCADE,
    action_type TEXT NOT NULL CHECK (action_type IN ('ignore', 'reconcile', 'set_category')),
    scenario_id UUID REFERENCES scenarios (id) ON DELETE RESTRICT,
    category_id UUID REFERENCES categories (id) ON DELETE RESTRICT,
    position    INTEGER NOT NULL CHECK (position >= 0),
    UNIQUE (rule_id, position),
    CHECK (
        (action_type = 'ignore' AND scenario_id IS NULL AND category_id IS NULL) OR
        (action_type = 'reconcile' AND scenario_id IS NOT NULL AND category_id IS NULL) OR
        (action_type = 'set_category' AND scenario_id IS NULL AND category_id IS NOT NULL)
    )
);

CREATE TABLE raw_imports (
    id UUID NOT NULL PRIMARY KEY,
    sync_run_id UUID NOT NULL REFERENCES sync_runs (id) ON DELETE RESTRICT,
    source_id UUID NOT NULL REFERENCES data_sources (id) ON DELETE RESTRICT,
    scope TEXT NOT NULL CHECK (scope IN ('item', 'accounts', 'transactions', 'investments', 'investment_transactions', 'bills', 'file')),
    external_account_id TEXT,
    page_sequence INTEGER NOT NULL CHECK (page_sequence >= 1),
    request_attempt INTEGER NOT NULL CHECK (request_attempt >= 1),
    request_method TEXT,
    request_path TEXT,
    http_status INTEGER,
    response_headers TEXT,
    payload BYTEA NOT NULL,
    payload_sha256 TEXT NOT NULL,
    received_at TIMESTAMPTZ NOT NULL,
    CHECK (scope = 'file' OR
        (request_method IS NOT NULL AND request_path IS NOT NULL
         AND http_status IS NOT NULL AND response_headers IS NOT NULL))
);

CREATE TABLE scenario_transactions (
    id           UUID NOT NULL PRIMARY KEY,
    scenario_id  UUID NOT NULL REFERENCES scenarios (id) ON DELETE CASCADE,
    description  TEXT NOT NULL,
    amount       NUMERIC NOT NULL,
    projected_at DATE NOT NULL,
    category     TEXT,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL
,
    CHECK (amount NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric))
);

CREATE TABLE financial_accounts (
    id UUID NOT NULL PRIMARY KEY,
    source_id UUID REFERENCES data_sources (id) ON DELETE RESTRICT,
    external_id TEXT,
    institution TEXT,
    name TEXT,
    number TEXT,
    account_type TEXT,
    account_subtype TEXT,
    balance NUMERIC,
    credit_limit NUMERIC,
    currency_code TEXT,
    provider_updated_at TIMESTAMPTZ,
    current_raw_import_id UUID REFERENCES raw_imports (id) ON DELETE RESTRICT,
    normalized_hash TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    available_credit_limit NUMERIC,
    balance_close_date DATE,
    balance_due_date DATE,
    manual_closing_day INTEGER CHECK (manual_closing_day BETWEEN 1 AND 31),
    balance_as_of TIMESTAMPTZ,
    origin TEXT NOT NULL DEFAULT 'synced' CHECK (origin IN ('synced', 'manual')),
    opening_balance NUMERIC,
    archived_at TIMESTAMPTZ,
    UNIQUE (source_id, external_id),
    CHECK (
        (origin = 'synced' AND source_id IS NOT NULL AND external_id IS NOT NULL
            AND current_raw_import_id IS NOT NULL AND normalized_hash IS NOT NULL AND opening_balance IS NULL) OR
        (origin = 'manual' AND source_id IS NULL AND external_id IS NULL
            AND current_raw_import_id IS NULL AND normalized_hash IS NULL
            AND account_type IS NOT NULL AND account_type = 'BANK' AND currency_code IS NOT NULL
            AND opening_balance IS NOT NULL AND balance IS NULL)
    )
,
    CHECK (balance NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (credit_limit NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (currency_code ~ '^[A-Z]{3}$'),
    CHECK (available_credit_limit NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (opening_balance NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric))
);

CREATE TABLE financial_investments (
    id UUID NOT NULL PRIMARY KEY,
    source_id UUID NOT NULL REFERENCES data_sources (id) ON DELETE RESTRICT,
    external_id TEXT NOT NULL,
    investment_type TEXT,
    subtype TEXT,
    name TEXT,
    balance NUMERIC,
    currency_code TEXT,
    number TEXT,
    owner TEXT,
    tax_number TEXT,
    due_date DATE,
    issuer TEXT,
    issuer_code TEXT,
    rate NUMERIC,
    rate_type TEXT,
    fixed_annual_rate NUMERIC,
    annual_rate NUMERIC,
    last_twelve_months_rate NUMERIC,
    quantity NUMERIC,
    value NUMERIC,
    amount NUMERIC,
    amount_profit NUMERIC,
    amount_withdrawal NUMERIC,
    as_of_date TIMESTAMPTZ,
    provider_updated_at TIMESTAMPTZ,
    isin TEXT,
    code TEXT,
    provider_status TEXT,
    current_raw_import_id UUID NOT NULL REFERENCES raw_imports (id) ON DELETE RESTRICT,
    normalized_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL, amount_original NUMERIC, taxes NUMERIC, taxes2 NUMERIC, issue_date DATE, purchase_date DATE, issuer_cnpj TEXT,
    UNIQUE (source_id, external_id)
,
    CHECK (balance NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (currency_code ~ '^[A-Z]{3}$'),
    CHECK (rate NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (fixed_annual_rate NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (annual_rate NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (last_twelve_months_rate NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (quantity NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (value NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (amount NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (amount_profit NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (amount_withdrawal NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (amount_original NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (taxes NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (taxes2 NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric))
);

CREATE TABLE investment_asset_quotes (
    asset_id UUID NOT NULL REFERENCES investment_assets (id) ON DELETE CASCADE,
    quoted_on DATE NOT NULL,
    price NUMERIC NOT NULL,
    source TEXT NOT NULL,
    raw_import_id UUID REFERENCES raw_imports (id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL, origin TEXT NOT NULL DEFAULT 'market'
    CHECK (origin IN ('sync', 'issue', 'market')),
    PRIMARY KEY (asset_id, quoted_on)
,
    CHECK (price NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric))
);

CREATE TABLE account_balance_adjustments (
    id UUID NOT NULL PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES financial_accounts(id) ON DELETE RESTRICT,
    amount NUMERIC NOT NULL,
    description TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
,
    CHECK (amount NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric))
);

CREATE TABLE financial_bills (
    id UUID NOT NULL PRIMARY KEY,
    source_id UUID NOT NULL REFERENCES data_sources (id) ON DELETE RESTRICT,
    account_id UUID NOT NULL REFERENCES financial_accounts (id) ON DELETE RESTRICT,
    external_id TEXT NOT NULL,
    due_date DATE,
    closing_date DATE,
    total_amount NUMERIC,
    currency_code TEXT,
    minimum_payment_amount NUMERIC,
    current_raw_import_id UUID NOT NULL REFERENCES raw_imports (id) ON DELETE RESTRICT,
    normalized_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (source_id, external_id)
,
    CHECK (total_amount NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (currency_code ~ '^[A-Z]{3}$'),
    CHECK (minimum_payment_amount NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric))
);

CREATE TABLE financial_investment_transactions (
    id UUID NOT NULL PRIMARY KEY,
    source_id UUID NOT NULL REFERENCES data_sources (id) ON DELETE RESTRICT,
    investment_id UUID NOT NULL REFERENCES financial_investments (id) ON DELETE RESTRICT,
    external_id TEXT NOT NULL,
    movement_type TEXT,
    quantity NUMERIC,
    value NUMERIC,
    amount NUMERIC,
    occurred_at TIMESTAMPTZ,
    trade_date DATE,
    current_raw_import_id UUID NOT NULL REFERENCES raw_imports (id) ON DELETE RESTRICT,
    normalized_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL, direction TEXT
    CHECK (direction IN ('inflow', 'outflow')),
    UNIQUE (source_id, external_id)
,
    CHECK (quantity NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (value NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (amount NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric))
);

CREATE TABLE financial_transactions (
    id UUID NOT NULL PRIMARY KEY,
    source_id UUID REFERENCES data_sources (id) ON DELETE RESTRICT,
    account_id UUID NOT NULL REFERENCES financial_accounts (id) ON DELETE RESTRICT,
    external_id TEXT,
    description TEXT,
    description_raw TEXT,
    amount NUMERIC,
    amount_in_account_currency NUMERIC,
    balance_after NUMERIC,
    currency_code TEXT,
    occurred_at TIMESTAMPTZ,
    provider_status TEXT,
    movement_type TEXT,
    source_category TEXT,
    source_category_id TEXT,
    provider_code TEXT,
    payment_data TEXT,
    credit_card_metadata TEXT,
    merchant TEXT,
    operation_type TEXT,
    operation_type_additional_info TEXT,
    operation_category TEXT,
    provider_id TEXT,
    provider_order INTEGER,
    provider_created_at TIMESTAMPTZ,
    provider_updated_at TIMESTAMPTZ,
    current_raw_import_id UUID REFERENCES raw_imports (id) ON DELETE RESTRICT,
    normalized_hash TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    origin TEXT NOT NULL DEFAULT 'synced' CHECK (origin IN ('synced', 'manual')),
    deleted_at TIMESTAMPTZ,
    UNIQUE (source_id, external_id),
    CHECK (
        (origin = 'synced' AND source_id IS NOT NULL AND external_id IS NOT NULL
            AND current_raw_import_id IS NOT NULL AND normalized_hash IS NOT NULL) OR
        (origin = 'manual' AND source_id IS NULL AND external_id IS NULL
            AND current_raw_import_id IS NULL AND normalized_hash IS NULL)
    )
,
    CHECK (amount NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (amount_in_account_currency NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (balance_after NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (currency_code ~ '^[A-Z]{3}$')
);

CREATE TABLE investment_accounts (
    id TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('manual', 'integrated')),
    currency_code TEXT,
    source_id UUID UNIQUE REFERENCES data_sources (id) ON DELETE CASCADE,
    financial_account_id UUID UNIQUE REFERENCES financial_accounts (id) ON DELETE SET NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CHECK (
        (kind = 'manual' AND source_id IS NULL AND currency_code = 'BRL') OR
        (kind = 'integrated' AND source_id IS NOT NULL)
    )
,
    CHECK (currency_code ~ '^[A-Z]{3}$')
);

CREATE TABLE scenario_recurring_schedules (
    scenario_id    UUID NOT NULL PRIMARY KEY REFERENCES scenarios (id) ON DELETE CASCADE,
    cashflow_kind  TEXT NOT NULL CHECK (cashflow_kind IN ('income', 'expense')),
    amount         NUMERIC NOT NULL CHECK (amount > 0),
    category_id    UUID REFERENCES categories (id) ON DELETE RESTRICT,
    account_id     UUID REFERENCES financial_accounts (id) ON DELETE RESTRICT,
    cadence        TEXT NOT NULL CHECK (cadence IN ('monthly', 'annual')),
    day_of_month   INTEGER NOT NULL CHECK (day_of_month BETWEEN 1 AND 31),
    month_of_year  INTEGER CHECK (month_of_year BETWEEN 1 AND 12),
    start_date     DATE NOT NULL,
    end_date       DATE,
    CHECK (cadence != 'annual' OR month_of_year IS NOT NULL)
,
    CHECK (amount NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric))
);

CREATE TABLE statement_imports (
    id UUID NOT NULL PRIMARY KEY,
    sync_run_id UUID NOT NULL UNIQUE REFERENCES sync_runs (id) ON DELETE RESTRICT,
    source_id UUID NOT NULL REFERENCES data_sources (id) ON DELETE RESTRICT,
    account_id UUID NOT NULL REFERENCES financial_accounts (id) ON DELETE RESTRICT,
    format TEXT NOT NULL,
    adapter_version TEXT NOT NULL,
    filename TEXT NOT NULL,
    file_sha256 TEXT NOT NULL,
    rows_total INTEGER NOT NULL CHECK (rows_total >= 0),
    rows_invalid INTEGER NOT NULL DEFAULT 0 CHECK (rows_invalid >= 0),
    rows_duplicate INTEGER NOT NULL DEFAULT 0 CHECK (rows_duplicate >= 0),
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE investment_positions (
    id UUID NOT NULL PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES investment_accounts (id) ON DELETE RESTRICT,
    asset_id UUID NOT NULL REFERENCES investment_assets (id) ON DELETE RESTRICT,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (account_id, asset_id)
);

CREATE TABLE manual_transaction_idempotency (
    owner_id INTEGER NOT NULL,
    idempotency_key TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    transaction_id UUID REFERENCES financial_transactions (id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (owner_id, idempotency_key)
);

CREATE TABLE normalization_events (
    id UUID NOT NULL PRIMARY KEY,
    sync_run_id UUID NOT NULL REFERENCES sync_runs (id) ON DELETE RESTRICT,
    raw_import_id UUID NOT NULL REFERENCES raw_imports (id) ON DELETE RESTRICT,
    entity_type TEXT NOT NULL CHECK (entity_type IN ('account', 'transaction', 'investment', 'investment_transaction', 'bill')),
    account_id UUID REFERENCES financial_accounts (id) ON DELETE RESTRICT,
    transaction_id UUID REFERENCES financial_transactions (id) ON DELETE RESTRICT,
    investment_id UUID REFERENCES financial_investments (id) ON DELETE RESTRICT,
    investment_transaction_id UUID REFERENCES financial_investment_transactions (id) ON DELETE RESTRICT,
    bill_id UUID REFERENCES financial_bills (id) ON DELETE RESTRICT,
    external_id TEXT,
    outcome TEXT NOT NULL CHECK (outcome IN ('inserted', 'updated', 'unchanged', 'rejected')),
    normalized_hash TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    CHECK (
        (outcome = 'rejected' AND account_id IS NULL AND transaction_id IS NULL
            AND investment_id IS NULL AND investment_transaction_id IS NULL AND bill_id IS NULL) OR
        (outcome <> 'rejected' AND (CAST((account_id IS NOT NULL) AS INTEGER) + CAST((transaction_id IS NOT NULL) AS INTEGER)
            + CAST((investment_id IS NOT NULL) AS INTEGER) + CAST((investment_transaction_id IS NOT NULL) AS INTEGER) + CAST((bill_id IS NOT NULL) AS INTEGER)) = 1)
    )
);

CREATE TABLE payable_transaction_links (
    id             UUID NOT NULL PRIMARY KEY,
    payable_id     UUID NOT NULL REFERENCES payables (id) ON DELETE CASCADE,
    transaction_id UUID NOT NULL UNIQUE REFERENCES financial_transactions (id) ON DELETE RESTRICT,
    linked_amount  NUMERIC NOT NULL,
    linked_at      TIMESTAMPTZ NOT NULL
,
    CHECK (linked_amount NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric))
);

CREATE TABLE scenario_realizations (
    id                      UUID NOT NULL PRIMARY KEY,
    scenario_id             UUID NOT NULL REFERENCES scenarios (id) ON DELETE CASCADE,
    scenario_transaction_id UUID REFERENCES scenario_transactions (id) ON DELETE CASCADE,
    occurrence_date         DATE,
    transaction_id          UUID REFERENCES financial_transactions (id) ON DELETE CASCADE,
    relation_type           TEXT NOT NULL CHECK (relation_type IN ('settlement', 'allocation', 'reconciliation')),
    state                   TEXT NOT NULL CHECK (state IN ('linked', 'detached')),
    origin                  TEXT NOT NULL,
    allocated_amount        NUMERIC,
    linked_amount           NUMERIC,
    created_at              TIMESTAMPTZ NOT NULL,
    CHECK ((state = 'linked' AND transaction_id IS NOT NULL) OR
           (state = 'detached' AND transaction_id IS NULL)),
    CHECK (
        (relation_type = 'settlement' AND scenario_transaction_id IS NULL AND occurrence_date IS NULL AND state = 'linked' AND linked_amount IS NOT NULL) OR
        (relation_type = 'allocation' AND scenario_transaction_id IS NOT NULL AND occurrence_date IS NULL AND state = 'linked' AND allocated_amount IS NOT NULL) OR
        (relation_type = 'reconciliation' AND scenario_transaction_id IS NULL AND occurrence_date IS NOT NULL)
    )
,
    CHECK (allocated_amount NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (linked_amount NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric))
);

CREATE TABLE sync_failures (
    id UUID NOT NULL PRIMARY KEY,
    sync_run_id UUID NOT NULL REFERENCES sync_runs (id) ON DELETE RESTRICT,
    raw_import_id UUID REFERENCES raw_imports (id) ON DELETE RESTRICT,
    account_id UUID REFERENCES financial_accounts (id) ON DELETE RESTRICT,
    external_account_id TEXT,
    external_transaction_id TEXT,
    investment_id UUID REFERENCES financial_investments (id) ON DELETE RESTRICT,
    external_investment_id TEXT,
    bill_id UUID REFERENCES financial_bills (id) ON DELETE RESTRICT,
    external_bill_id TEXT,
    stage TEXT NOT NULL CHECK (stage IN (
        'auth', 'item', 'accounts', 'account', 'transactions',
        'investments', 'investment', 'investment_transactions',
        'bills', 'normalize', 'interrupted', 'worker_unavailable'
    )),
    error_code TEXT NOT NULL,
    safe_message TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE transaction_category_decisions (
    transaction_id UUID NOT NULL PRIMARY KEY REFERENCES financial_transactions (id) ON DELETE RESTRICT,
    category_id UUID NOT NULL REFERENCES categories (id) ON DELETE RESTRICT,
    revision INTEGER NOT NULL CHECK (revision >= 1),
    changed_at TIMESTAMPTZ NOT NULL,
    origin TEXT NOT NULL CHECK (origin IN ('manual', 'automatic', 'rule', 'learned'))
);

CREATE TABLE transaction_category_events (
    id UUID NOT NULL PRIMARY KEY,
    transaction_id UUID NOT NULL REFERENCES financial_transactions (id) ON DELETE RESTRICT,
    revision INTEGER NOT NULL CHECK (revision >= 1),
    previous_category_id TEXT,
    resulting_category_id TEXT NOT NULL,
    changed_at TIMESTAMPTZ NOT NULL,
    origin TEXT NOT NULL CHECK (origin IN ('manual', 'automatic', 'rule', 'learned')),
    UNIQUE (transaction_id, revision)
);

CREATE TABLE transaction_inclusion_decisions (
    transaction_id UUID NOT NULL PRIMARY KEY REFERENCES financial_transactions (id) ON DELETE RESTRICT,
    state TEXT NOT NULL CHECK (state IN ('considered', 'ignored')),
    revision INTEGER NOT NULL CHECK (revision >= 1),
    changed_at TIMESTAMPTZ NOT NULL,
    origin TEXT NOT NULL CHECK (origin IN ('manual', 'rule')),
    rule_id UUID REFERENCES automation_rules (id) ON DELETE SET NULL,
    rule_name TEXT,
    CHECK (origin = 'rule' OR (rule_id IS NULL AND rule_name IS NULL))
);

CREATE TABLE transaction_inclusion_events (
    id UUID NOT NULL PRIMARY KEY,
    transaction_id UUID NOT NULL REFERENCES financial_transactions (id) ON DELETE RESTRICT,
    revision INTEGER NOT NULL CHECK (revision >= 1),
    previous_state TEXT NOT NULL CHECK (previous_state IN ('considered', 'ignored')),
    resulting_state TEXT NOT NULL CHECK (resulting_state IN ('considered', 'ignored')),
    changed_at TIMESTAMPTZ NOT NULL,
    origin TEXT NOT NULL CHECK (origin IN ('manual', 'rule')),
    -- No FK on rule_id: this table is append-only (see triggers below), and a
    -- deleted rule must not force an UPDATE of historical rows to null it out.
    -- rule_name is the durable audit snapshot; rule_id is a best-effort reference.
    rule_id TEXT,
    rule_name TEXT,
    CHECK (previous_state <> resulting_state),
    CHECK (origin = 'rule' OR (rule_id IS NULL AND rule_name IS NULL)),
    UNIQUE (transaction_id, revision)
);

CREATE TABLE investment_operations (
    id UUID NOT NULL PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES investment_accounts (id) ON DELETE RESTRICT,
    position_id UUID REFERENCES investment_positions (id) ON DELETE RESTRICT,
    kind TEXT NOT NULL CHECK (kind IN (
        'initial_balance', 'deposit', 'withdrawal', 'buy', 'sell', 'income', 'fee', 'tax', 'valuation',
        'transfer_out', 'transfer_in'
    )),
    occurred_on DATE NOT NULL,
    amount NUMERIC NOT NULL,
    quantity NUMERIC,
    unit_price NUMERIC,
    fees NUMERIC NOT NULL DEFAULT '0',
    taxes NUMERIC NOT NULL DEFAULT '0',
    notes TEXT,
    source TEXT NOT NULL DEFAULT 'manual' CHECK (source IN ('manual', 'synced')),
    transfer_id UUID,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
,
    CHECK (amount NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (quantity NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (unit_price NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (fees NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (taxes NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric))
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

CREATE TABLE investment_reconciliations (
    id UUID NOT NULL PRIMARY KEY,
    operation_id UUID NOT NULL REFERENCES investment_operations (id) ON DELETE CASCADE,
    financial_transaction_id UUID REFERENCES financial_transactions (id) ON DELETE CASCADE,
    financial_investment_transaction_id UUID REFERENCES financial_investment_transactions (id) ON DELETE CASCADE,
    amount NUMERIC NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    CHECK (financial_transaction_id IS NOT NULL OR financial_investment_transaction_id IS NOT NULL)
,
    CHECK (amount NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric))
);

CREATE INDEX auth_sessions_expiry ON auth_sessions(expires_at);

CREATE INDEX idx_automation_rule_actions_category_id ON automation_rule_actions (category_id);

CREATE INDEX idx_automation_rule_actions_scenario_id ON automation_rule_actions (scenario_id);

CREATE INDEX idx_scenario_realizations_scenario_occurrence
    ON scenario_realizations (scenario_id, occurrence_date);

CREATE INDEX idx_scenario_realizations_transaction_event
    ON scenario_realizations (scenario_transaction_id, transaction_id);

CREATE INDEX idx_scenario_realizations_transaction_settlement_reconciliation
    ON scenario_realizations (transaction_id)
    WHERE relation_type IN ('settlement', 'reconciliation');

CREATE INDEX idx_scenario_recurring_schedules_account_id
    ON scenario_recurring_schedules (account_id);

CREATE INDEX idx_scenario_recurring_schedules_category_id
    ON scenario_recurring_schedules (category_id);

CREATE INDEX idx_scenario_transactions_scenario_id ON scenario_transactions (scenario_id);

CREATE INDEX idx_scenarios_payable_id ON scenarios (payable_id);

CREATE INDEX ix_account_balance_adjustments_account ON account_balance_adjustments(account_id);

CREATE INDEX ix_financial_bills_account_id ON financial_bills (account_id);

CREATE INDEX ix_financial_investment_transactions_investment_id ON financial_investment_transactions (investment_id);

CREATE INDEX ix_financial_investment_transactions_occurred_at_id_desc ON financial_investment_transactions (
    occurred_at DESC NULLS LAST, id DESC
);

CREATE INDEX ix_financial_transactions_account_id ON financial_transactions (account_id);

CREATE INDEX ix_financial_transactions_occurred_at_id_desc ON financial_transactions (
    occurred_at DESC NULLS LAST, id DESC
);

CREATE INDEX ix_investment_operations_account_date ON investment_operations (account_id, occurred_on, created_at, id);

CREATE INDEX ix_investment_operations_position_date ON investment_operations (position_id, occurred_on, created_at, id);

CREATE INDEX ix_investment_operations_transfer_id ON investment_operations (transfer_id);

CREATE INDEX ix_investment_position_portfolios_portfolio_id ON investment_position_portfolios (portfolio_id);

CREATE INDEX ix_investment_positions_account_id ON investment_positions (account_id);

CREATE INDEX ix_investment_positions_asset_id ON investment_positions (asset_id);

CREATE INDEX ix_investment_reconciliations_operation_id ON investment_reconciliations (operation_id);

CREATE INDEX ix_statement_imports_account_created ON statement_imports (account_id, created_at DESC);

CREATE INDEX ix_transaction_category_decisions_category_id ON transaction_category_decisions (category_id);

CREATE UNIQUE INDEX uq_automation_rule_actions_one_reconcile_per_scenario
    ON automation_rule_actions (scenario_id)
    WHERE action_type = 'reconcile' AND scenario_id IS NOT NULL;

CREATE UNIQUE INDEX uq_investment_reconciliation_operation_financial_investment_transaction
    ON investment_reconciliations (operation_id, financial_investment_transaction_id)
    WHERE financial_investment_transaction_id IS NOT NULL;

CREATE UNIQUE INDEX uq_investment_reconciliation_operation_financial_transaction
    ON investment_reconciliations (operation_id, financial_transaction_id)
    WHERE financial_transaction_id IS NOT NULL;

CREATE UNIQUE INDEX uq_raw_imports_identity ON raw_imports (
    sync_run_id, scope, COALESCE(external_account_id, ''), page_sequence, request_attempt
);

CREATE UNIQUE INDEX uq_scenario_realizations_one_complete_event_transaction
    ON scenario_realizations (transaction_id)
    WHERE transaction_id IS NOT NULL
      AND relation_type IN ('settlement', 'reconciliation')
      AND state = 'linked';

CREATE UNIQUE INDEX uq_scenario_realizations_reconciliation_event
    ON scenario_realizations (scenario_id, occurrence_date)
    WHERE relation_type = 'reconciliation';

CREATE UNIQUE INDEX uq_scenarios_accounting_source
    ON scenarios (payable_id)
    WHERE is_accounting_source = TRUE;

CREATE UNIQUE INDEX uq_sync_runs_active_source ON sync_runs (source_id)
    WHERE status = 'in_progress';

-- +goose StatementBegin
CREATE FUNCTION reject_append_only_write() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION '% is append-only', TG_TABLE_NAME;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER normalization_events_reject_delete BEFORE DELETE ON normalization_events
FOR EACH ROW EXECUTE FUNCTION reject_append_only_write();

CREATE TRIGGER normalization_events_reject_update BEFORE UPDATE ON normalization_events
FOR EACH ROW EXECUTE FUNCTION reject_append_only_write();

CREATE TRIGGER raw_imports_reject_delete BEFORE DELETE ON raw_imports
FOR EACH ROW EXECUTE FUNCTION reject_append_only_write();

CREATE TRIGGER raw_imports_reject_update BEFORE UPDATE ON raw_imports
FOR EACH ROW EXECUTE FUNCTION reject_append_only_write();

CREATE TRIGGER transaction_category_events_reject_delete BEFORE DELETE ON transaction_category_events
FOR EACH ROW EXECUTE FUNCTION reject_append_only_write();

CREATE TRIGGER transaction_category_events_reject_update BEFORE UPDATE ON transaction_category_events
FOR EACH ROW EXECUTE FUNCTION reject_append_only_write();

CREATE TRIGGER transaction_inclusion_events_reject_delete BEFORE DELETE ON transaction_inclusion_events
FOR EACH ROW EXECUTE FUNCTION reject_append_only_write();

CREATE TRIGGER transaction_inclusion_events_reject_update BEFORE UPDATE ON transaction_inclusion_events
FOR EACH ROW EXECUTE FUNCTION reject_append_only_write();

INSERT INTO categories (id, name, kind, is_active, created_at, updated_at, icon, color) VALUES
    ('000433b6-3094-5a9c-87df-465b70574a4b', 'Supermercado', 'expense', TRUE, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z', 'shopping-cart', '#2a78d6'),
    ('11d50e25-577d-5780-bbae-35a6b14c7d01', 'Compras', 'expense', TRUE, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z', 'shopping', '#e64980'),
    ('12cdb9e7-3f28-5fcf-a675-a2195a732bf1', 'Alimentação', 'expense', TRUE, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z', 'coffee', '#eb6834'),
    ('15278c27-bfce-5ab7-bbe5-eb7351425226', 'Família', 'expense', TRUE, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z', 'team', '#4c6ef5'),
    ('17f56080-837c-5ebb-a828-93fcb891bb8f', 'Saúde', 'expense', TRUE, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z', 'medicine', '#37b24d'),
    ('1dddd1b3-1a1e-5c05-9a75-6359c68ce466', 'Serviços digitais', 'expense', TRUE, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z', 'wifi', '#099268'),
    ('2231c10d-ff72-59af-9513-72516f9f452e', 'Impostos e taxas', 'expense', TRUE, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z', 'percentage', '#495057'),
    ('223e55fe-b31e-539d-83aa-0b22427fc17f', 'Seguros', 'expense', TRUE, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z', 'safety-certificate', '#1baf7a'),
    ('3b734076-b597-545b-b302-47061c2278bf', 'Empréstimos', 'expense', TRUE, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z', 'bank', '#d64545'),
    ('3c5a9586-2a11-556d-b014-692ed51c3997', 'Salário', 'income', TRUE, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z', 'money-collect', '#1baf7a'),
    ('51ed2111-26c8-5a0f-bffb-42aa00b5a0ce', 'Educação', 'expense', TRUE, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z', 'book', '#7c5cbf'),
    ('533d9187-99b6-542b-a2f3-6eb9cbb299ce', 'Transferência entre Contas Próprias', 'transfer', TRUE, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z', 'swap', '#7c5cbf'),
    ('659c118e-42ad-5427-898e-cea0153525ed', 'Lazer', 'expense', TRUE, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z', 'smile', '#eda100'),
    ('6885e5af-bcc8-5dec-b6a2-45033d62376b', 'Outras receitas', 'income', TRUE, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z', 'plus-circle', '#6c757d'),
    ('6c5acb3a-477c-5ea0-a3f0-a036298d603a', 'Pets', 'expense', TRUE, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z', 'heart', '#f76707'),
    ('78bc85d7-21fc-5965-908f-a707208e0b1b', 'Rendimentos', 'income', TRUE, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z', 'rise', '#37b24d'),
    ('7f185706-e8f2-5c1c-893a-150a93cfa133', 'Cuidados pessoais', 'expense', TRUE, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z', 'scissor', '#e87ba4'),
    ('99f20e22-cd96-5081-afd0-068a203c5fde', 'Transporte', 'expense', TRUE, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z', 'car', '#17a2b8'),
    ('a5318bcc-de1c-52c3-889b-2142e9c02408', 'Viagens', 'expense', TRUE, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z', 'global', '#2a78d6'),
    ('a5632bd8-442b-5a7c-9375-f05a2550f73f', 'Presentes e doações', 'expense', TRUE, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z', 'gift', '#e87ba4'),
    ('aec4034c-f5a2-59a0-a1f9-1d737e4f1f3f', 'Serviços', 'expense', TRUE, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z', 'tool', '#6c757d'),
    ('b70b2537-8d98-55f3-930a-fd5d9b40cb2b', 'Investimentos', 'expense', TRUE, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z', 'line-chart', '#099268'),
    ('b74c367a-9409-516c-b548-00a7207f67bd', 'Outros', 'expense', TRUE, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z', 'ellipsis', '#6c757d'),
    ('bbdc84e0-788a-59f7-a9f4-b2143172e53b', 'Moradia', 'expense', TRUE, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z', 'home', '#b8860b'),
    ('be4336bf-2090-58cd-babe-c7755e035619', 'Reembolsos', 'income', TRUE, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z', 'rollback', '#17a2b8'),
    ('c814e5a7-d199-5455-93ef-4c6369ce0e0a', 'Trabalho autônomo', 'income', TRUE, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z', 'laptop', '#4c6ef5'),
    ('c86337c1-3771-5917-aa53-5540e8d937bb', 'Benefícios', 'income', TRUE, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z', 'trophy', '#eda100'),
    ('d7ea08b8-b80b-5e93-8352-9354ac7fe6d8', 'Empréstimos recebidos', 'income', TRUE, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z', 'wallet', '#b8860b'),
    ('dd10c680-fb35-4457-8595-4e51c8d279a7', 'Pagamento de Fatura de Cartão', 'transfer', TRUE, '2026-09-03T00:00:00.000000000Z', '2026-09-03T00:00:00.000000000Z', 'credit-card', '#7c5cbf');


CREATE TABLE julius_schema_version (
    id INTEGER NOT NULL PRIMARY KEY CHECK (id = 1),
    generation INTEGER NOT NULL CHECK (generation = 1)
);
INSERT INTO julius_schema_version (id, generation) VALUES (1, 1);

-- +goose Down

DROP TABLE investment_reconciliations CASCADE;
DROP TABLE investment_position_portfolios CASCADE;
DROP TABLE investment_operations CASCADE;
DROP TABLE transaction_inclusion_events CASCADE;
DROP TABLE transaction_inclusion_decisions CASCADE;
DROP TABLE transaction_category_events CASCADE;
DROP TABLE transaction_category_decisions CASCADE;
DROP TABLE sync_failures CASCADE;
DROP TABLE scenario_realizations CASCADE;
DROP TABLE payable_transaction_links CASCADE;
DROP TABLE normalization_events CASCADE;
DROP TABLE manual_transaction_idempotency CASCADE;
DROP TABLE investment_positions CASCADE;
DROP TABLE statement_imports CASCADE;
DROP TABLE scenario_recurring_schedules CASCADE;
DROP TABLE investment_accounts CASCADE;
DROP TABLE financial_transactions CASCADE;
DROP TABLE financial_investment_transactions CASCADE;
DROP TABLE financial_bills CASCADE;
DROP TABLE account_balance_adjustments CASCADE;
DROP TABLE investment_asset_quotes CASCADE;
DROP TABLE financial_investments CASCADE;
DROP TABLE financial_accounts CASCADE;
DROP TABLE scenario_transactions CASCADE;
DROP TABLE raw_imports CASCADE;
DROP TABLE automation_rule_actions CASCADE;
DROP TABLE sync_runs CASCADE;
DROP TABLE scenarios CASCADE;
DROP TABLE investment_asset_quote_coverage CASCADE;
DROP TABLE automation_rule_conditions CASCADE;
DROP TABLE auth_sessions CASCADE;
DROP TABLE settings CASCADE;
DROP TABLE payables CASCADE;
DROP TABLE net_worth_snapshots CASCADE;
DROP TABLE investment_portfolios CASCADE;
DROP TABLE investment_assets CASCADE;
DROP TABLE exchange_rates CASCADE;
DROP TABLE data_sources CASCADE;
DROP TABLE categories CASCADE;
DROP TABLE automation_rules CASCADE;
DROP TABLE auth_owner CASCADE;
DROP TABLE app_auth CASCADE;
DROP FUNCTION reject_append_only_write();
DROP TABLE julius_schema_version;
