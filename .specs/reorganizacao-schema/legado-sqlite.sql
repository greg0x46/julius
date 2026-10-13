-- Final legacy schema after SQLite migration 00047_manual_bank_accounts.
-- Schema only; generated from internal/db.Open(":memory:").

CREATE TABLE account_balance_adjustments (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES financial_accounts(id) ON DELETE RESTRICT,
    amount TEXT NOT NULL,
    description TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE app_auth (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    salt TEXT NOT NULL,
    verifier TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE auth_owner (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    email TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    revision INTEGER NOT NULL DEFAULT 1,
    created_at BIGINT NOT NULL
);

CREATE TABLE auth_sessions (
    token_hash TEXT PRIMARY KEY,
    owner_id INTEGER NOT NULL REFERENCES auth_owner(id),
    revision INTEGER NOT NULL,
    created_at BIGINT NOT NULL,
    last_seen_at BIGINT NOT NULL,
    expires_at BIGINT NOT NULL
);

CREATE TABLE "automation_rule_actions" (
    id          TEXT PRIMARY KEY,
    rule_id     TEXT NOT NULL REFERENCES automation_rules (id) ON DELETE CASCADE,
    action_type TEXT NOT NULL CHECK (action_type IN ('ignore', 'reconcile', 'set_category')),
    scenario_id TEXT REFERENCES scenarios (id) ON DELETE RESTRICT,
    category_id TEXT REFERENCES categories (id) ON DELETE RESTRICT,
    position    INTEGER NOT NULL CHECK (position >= 0),
    UNIQUE (rule_id, position),
    CHECK (
        (action_type = 'ignore' AND scenario_id IS NULL AND category_id IS NULL) OR
        (action_type = 'reconcile' AND scenario_id IS NOT NULL AND category_id IS NULL) OR
        (action_type = 'set_category' AND scenario_id IS NULL AND category_id IS NOT NULL)
    )
);

CREATE TABLE "automation_rule_conditions" (
    id       TEXT PRIMARY KEY,
    rule_id  TEXT NOT NULL REFERENCES automation_rules (id) ON DELETE CASCADE,
    field    TEXT NOT NULL CHECK (field IN ('description', 'card', 'account', 'amount', 'day_of_month')),
    operator TEXT NOT NULL CHECK (operator IN ('contains', 'equals', 'within_percent', 'day_range')),
    value    TEXT NOT NULL CHECK (trim(value) <> ''),
    position INTEGER NOT NULL CHECK (position >= 0),
    UNIQUE (rule_id, position)
);

CREATE TABLE automation_rules (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL CHECK (trim(name) <> ''),
    is_active INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0, 1)),
    logic_operator TEXT NOT NULL CHECK (logic_operator IN ('and', 'or')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE categories (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('expense', 'income', 'transfer')),
    is_active INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0, 1)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
, icon TEXT NOT NULL DEFAULT 'ellipsis', color TEXT NOT NULL DEFAULT '#6c757d');

CREATE TABLE data_sources (
    id TEXT PRIMARY KEY,
    provider TEXT NOT NULL,
    external_item_id TEXT NOT NULL,
    display_name TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL, is_active INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0, 1)), label TEXT,
    UNIQUE (provider, external_item_id)
);

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

CREATE TABLE "financial_accounts" (
    id TEXT PRIMARY KEY,
    source_id TEXT REFERENCES data_sources (id),
    external_id TEXT,
    institution TEXT,
    name TEXT,
    number TEXT,
    account_type TEXT,
    account_subtype TEXT,
    balance TEXT,
    credit_limit TEXT,
    currency_code TEXT,
    provider_updated_at TEXT,
    current_raw_import_id TEXT REFERENCES raw_imports (id),
    normalized_hash TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    available_credit_limit TEXT,
    balance_close_date TEXT,
    balance_due_date TEXT,
    manual_closing_day INTEGER CHECK (manual_closing_day BETWEEN 1 AND 31),
    balance_as_of TEXT,
    origin TEXT NOT NULL DEFAULT 'synced' CHECK (origin IN ('synced', 'manual')),
    opening_balance TEXT,
    archived_at TEXT,
    UNIQUE (source_id, external_id),
    CHECK (
        (origin = 'synced' AND source_id IS NOT NULL AND external_id IS NOT NULL
            AND current_raw_import_id IS NOT NULL AND normalized_hash IS NOT NULL AND opening_balance IS NULL) OR
        (origin = 'manual' AND source_id IS NULL AND external_id IS NULL
            AND current_raw_import_id IS NULL AND normalized_hash IS NULL
            AND account_type IS NOT NULL AND account_type = 'BANK' AND currency_code IS NOT NULL
            AND opening_balance IS NOT NULL AND balance IS NULL)
    )
);

CREATE TABLE financial_bills (
    id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL REFERENCES data_sources (id),
    account_id TEXT NOT NULL REFERENCES financial_accounts (id),
    external_id TEXT NOT NULL,
    due_date TEXT,
    closing_date TEXT,
    total_amount TEXT,
    currency_code TEXT,
    minimum_payment_amount TEXT,
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
    updated_at TEXT NOT NULL, direction TEXT
    CHECK (direction IN ('inflow', 'outflow')),
    UNIQUE (source_id, external_id)
);

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
    updated_at TEXT NOT NULL, amount_original TEXT, taxes TEXT, taxes2 TEXT, issue_date TEXT, purchase_date TEXT, issuer_cnpj TEXT,
    UNIQUE (source_id, external_id)
);

CREATE TABLE "financial_transactions" (
    id TEXT PRIMARY KEY,
    source_id TEXT REFERENCES data_sources (id),
    account_id TEXT NOT NULL REFERENCES financial_accounts (id),
    external_id TEXT,
    description TEXT,
    description_raw TEXT,
    amount TEXT,
    amount_in_account_currency TEXT,
    balance_after TEXT,
    currency_code TEXT,
    occurred_at TEXT,
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
    provider_created_at TEXT,
    provider_updated_at TEXT,
    current_raw_import_id TEXT REFERENCES raw_imports (id),
    normalized_hash TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    origin TEXT NOT NULL DEFAULT 'synced' CHECK (origin IN ('synced', 'manual')),
    deleted_at TEXT,
    UNIQUE (source_id, external_id),
    CHECK (
        (origin = 'synced' AND source_id IS NOT NULL AND external_id IS NOT NULL
            AND current_raw_import_id IS NOT NULL AND normalized_hash IS NOT NULL) OR
        (origin = 'manual' AND source_id IS NULL AND external_id IS NULL
            AND current_raw_import_id IS NULL AND normalized_hash IS NULL)
    )
);

CREATE TABLE goose_db_version (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		version_id INTEGER NOT NULL,
		is_applied INTEGER NOT NULL,
		tstamp TIMESTAMP DEFAULT (datetime('now'))
	);

CREATE TABLE investment_accounts (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('manual', 'integrated')),
    currency_code TEXT,
    source_id TEXT UNIQUE REFERENCES data_sources (id) ON DELETE CASCADE,
    financial_account_id TEXT UNIQUE REFERENCES financial_accounts (id) ON DELETE SET NULL,
    is_active INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0, 1)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    CHECK (
        (kind = 'manual' AND source_id IS NULL AND currency_code = 'BRL') OR
        (kind = 'integrated' AND source_id IS NOT NULL)
    )
);

CREATE TABLE investment_asset_quote_coverage (
    asset_id TEXT PRIMARY KEY REFERENCES investment_assets (id) ON DELETE CASCADE,
    source TEXT NOT NULL,
    symbol TEXT NOT NULL,
    covered_from TEXT NOT NULL,
    covered_to TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE investment_asset_quotes (
    asset_id TEXT NOT NULL REFERENCES investment_assets (id) ON DELETE CASCADE,
    quoted_on TEXT NOT NULL,
    price TEXT NOT NULL,
    source TEXT NOT NULL,
    raw_import_id TEXT REFERENCES raw_imports (id),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL, origin TEXT NOT NULL DEFAULT 'market'
    CHECK (origin IN ('sync', 'issue', 'market')),
    PRIMARY KEY (asset_id, quoted_on)
);

CREATE TABLE investment_assets (
    id TEXT PRIMARY KEY,
    canonical_key TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    ticker TEXT,
    asset_type TEXT NOT NULL,
    currency_code TEXT NOT NULL DEFAULT 'BRL',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
, quote_source TEXT, quote_symbol TEXT, asset_class TEXT NOT NULL DEFAULT 'other'
    CHECK (asset_class IN ('fixed_income', 'variable_income', 'multimarket', 'currency', 'crypto', 'other')));

CREATE TABLE investment_operations (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES investment_accounts (id) ON DELETE RESTRICT,
    position_id TEXT REFERENCES investment_positions (id) ON DELETE RESTRICT,
    kind TEXT NOT NULL CHECK (kind IN (
        'initial_balance', 'deposit', 'withdrawal', 'buy', 'sell', 'income', 'fee', 'tax', 'valuation',
        'transfer_out', 'transfer_in'
    )),
    occurred_on TEXT NOT NULL,
    amount TEXT NOT NULL,
    quantity TEXT,
    unit_price TEXT,
    fees TEXT NOT NULL DEFAULT '0',
    taxes TEXT NOT NULL DEFAULT '0',
    notes TEXT,
    source TEXT NOT NULL DEFAULT 'manual' CHECK (source IN ('manual', 'synced')),
    transfer_id TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE investment_portfolios (
    id TEXT PRIMARY KEY,
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

CREATE TABLE investment_positions (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES investment_accounts (id) ON DELETE RESTRICT,
    asset_id TEXT NOT NULL REFERENCES investment_assets (id) ON DELETE RESTRICT,
    notes TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (account_id, asset_id)
);

CREATE TABLE investment_reconciliations (
    id TEXT PRIMARY KEY,
    operation_id TEXT NOT NULL REFERENCES investment_operations (id) ON DELETE CASCADE,
    financial_transaction_id TEXT REFERENCES financial_transactions (id) ON DELETE CASCADE,
    financial_investment_transaction_id TEXT REFERENCES financial_investment_transactions (id) ON DELETE CASCADE,
    amount TEXT NOT NULL,
    created_at TEXT NOT NULL,
    CHECK (financial_transaction_id IS NOT NULL OR financial_investment_transaction_id IS NOT NULL)
);

CREATE TABLE manual_transaction_idempotency (
    owner_id INTEGER NOT NULL,
    idempotency_key TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    transaction_id TEXT REFERENCES financial_transactions(id),
    created_at TEXT NOT NULL,
    PRIMARY KEY (owner_id, idempotency_key)
);

CREATE TABLE net_worth_snapshots (
    id                    TEXT PRIMARY KEY,
    captured_at           TEXT NOT NULL UNIQUE,
    total_assets          TEXT NOT NULL,
    total_liabilities     TEXT NOT NULL,
    net_worth             TEXT NOT NULL,
    cash_balance          TEXT NOT NULL,
    investment_balance    TEXT NOT NULL,
    credit_card_balance   TEXT NOT NULL,
    payables_debt         TEXT NOT NULL,
    created_at            TEXT NOT NULL,
    updated_at            TEXT NOT NULL
, is_backfilled INTEGER NOT NULL DEFAULT 0 CHECK (is_backfilled IN (0, 1)));

CREATE TABLE "normalization_events" (
    id TEXT PRIMARY KEY,
    sync_run_id TEXT NOT NULL REFERENCES sync_runs (id),
    raw_import_id TEXT NOT NULL REFERENCES raw_imports (id),
    entity_type TEXT NOT NULL CHECK (entity_type IN ('account', 'transaction', 'investment', 'investment_transaction', 'bill')),
    account_id TEXT REFERENCES financial_accounts (id),
    transaction_id TEXT REFERENCES financial_transactions (id),
    investment_id TEXT REFERENCES financial_investments (id),
    investment_transaction_id TEXT REFERENCES financial_investment_transactions (id),
    bill_id TEXT REFERENCES financial_bills (id),
    external_id TEXT,
    outcome TEXT NOT NULL CHECK (outcome IN ('inserted', 'updated', 'unchanged', 'rejected')),
    normalized_hash TEXT,
    created_at TEXT NOT NULL,
    CHECK (
        (outcome = 'rejected' AND account_id IS NULL AND transaction_id IS NULL
            AND investment_id IS NULL AND investment_transaction_id IS NULL AND bill_id IS NULL) OR
        (outcome <> 'rejected' AND ((account_id IS NOT NULL) + (transaction_id IS NOT NULL)
            + (investment_id IS NOT NULL) + (investment_transaction_id IS NOT NULL) + (bill_id IS NOT NULL)) = 1)
    )
);

CREATE TABLE payable_transaction_links (
    id             TEXT PRIMARY KEY,
    payable_id     TEXT NOT NULL REFERENCES payables (id) ON DELETE CASCADE,
    transaction_id TEXT NOT NULL UNIQUE REFERENCES financial_transactions (id) ON DELETE RESTRICT,
    linked_amount  TEXT NOT NULL,
    linked_at      TEXT NOT NULL
);

CREATE TABLE payables (
    id                      TEXT PRIMARY KEY,
    kind                    TEXT NOT NULL CHECK (kind IN ('debt', 'receivable')),
    name                    TEXT NOT NULL,
    total_amount            TEXT NOT NULL CHECK (CAST(total_amount AS REAL) > 0),
    starting_settled_amount TEXT NOT NULL DEFAULT '0' CHECK (CAST(starting_settled_amount AS REAL) >= 0),
    created_at              TEXT NOT NULL,
    updated_at              TEXT NOT NULL
);

CREATE TABLE "raw_imports" (
    id TEXT PRIMARY KEY,
    sync_run_id TEXT NOT NULL REFERENCES sync_runs (id),
    source_id TEXT NOT NULL REFERENCES data_sources (id),
    scope TEXT NOT NULL CHECK (scope IN ('item', 'accounts', 'transactions', 'investments', 'investment_transactions', 'bills', 'file')),
    external_account_id TEXT,
    page_sequence INTEGER NOT NULL CHECK (page_sequence >= 1),
    request_attempt INTEGER NOT NULL CHECK (request_attempt >= 1),
    request_method TEXT,
    request_path TEXT,
    http_status INTEGER,
    response_headers TEXT,
    payload BLOB NOT NULL,
    payload_sha256 TEXT NOT NULL,
    received_at TEXT NOT NULL,
    CHECK (scope = 'file' OR
        (request_method IS NOT NULL AND request_path IS NOT NULL
         AND http_status IS NOT NULL AND response_headers IS NOT NULL))
);

CREATE TABLE scenario_realizations (
    id                      TEXT PRIMARY KEY,
    scenario_id             TEXT NOT NULL REFERENCES scenarios (id) ON DELETE CASCADE,
    scenario_transaction_id TEXT REFERENCES scenario_transactions (id) ON DELETE CASCADE,
    occurrence_date         TEXT,
    transaction_id          TEXT REFERENCES financial_transactions (id) ON DELETE CASCADE,
    relation_type           TEXT NOT NULL CHECK (relation_type IN ('settlement', 'allocation', 'reconciliation')),
    state                   TEXT NOT NULL CHECK (state IN ('linked', 'detached')),
    origin                  TEXT NOT NULL,
    allocated_amount        TEXT,
    linked_amount           TEXT,
    created_at              TEXT NOT NULL,
    CHECK ((state = 'linked' AND transaction_id IS NOT NULL) OR
           (state = 'detached' AND transaction_id IS NULL)),
    CHECK (
        (relation_type = 'settlement' AND scenario_transaction_id IS NULL AND occurrence_date IS NULL AND state = 'linked' AND linked_amount IS NOT NULL) OR
        (relation_type = 'allocation' AND scenario_transaction_id IS NOT NULL AND occurrence_date IS NULL AND state = 'linked' AND allocated_amount IS NOT NULL) OR
        (relation_type = 'reconciliation' AND scenario_transaction_id IS NULL AND occurrence_date IS NOT NULL)
    )
);

CREATE TABLE scenario_recurring_schedules (
    scenario_id    TEXT PRIMARY KEY REFERENCES scenarios (id) ON DELETE CASCADE,
    cashflow_kind  TEXT NOT NULL CHECK (cashflow_kind IN ('income', 'expense')),
    amount         TEXT NOT NULL CHECK (CAST(amount AS REAL) > 0),
    category_id    TEXT REFERENCES categories (id) ON DELETE RESTRICT,
    account_id     TEXT REFERENCES financial_accounts (id) ON DELETE RESTRICT,
    cadence        TEXT NOT NULL CHECK (cadence IN ('monthly', 'annual')),
    day_of_month   INTEGER NOT NULL CHECK (day_of_month BETWEEN 1 AND 31),
    month_of_year  INTEGER CHECK (month_of_year BETWEEN 1 AND 12),
    start_date     TEXT NOT NULL,
    end_date       TEXT,
    CHECK (cadence != 'annual' OR month_of_year IS NOT NULL)
);

CREATE TABLE "scenario_transactions" (
    id           TEXT PRIMARY KEY,
    scenario_id  TEXT NOT NULL REFERENCES scenarios (id) ON DELETE CASCADE,
    description  TEXT NOT NULL,
    amount       TEXT NOT NULL,
    projected_at TEXT NOT NULL,
    category     TEXT,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);

CREATE TABLE "scenarios" (
    id                   TEXT PRIMARY KEY,
    kind                 TEXT NOT NULL CHECK (kind IN ('debt_plan', 'receivable_plan', 'standalone', 'recurring')),
    name                 TEXT NOT NULL,
    payable_id           TEXT REFERENCES payables (id) ON DELETE CASCADE,
    is_active            INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0, 1)),
    is_accounting_source INTEGER NOT NULL DEFAULT 0 CHECK (is_accounting_source IN (0, 1)),
    created_at           TEXT NOT NULL,
    updated_at           TEXT NOT NULL,
    CHECK (is_accounting_source = 0 OR kind IN ('debt_plan', 'receivable_plan')),
    CHECK (
        (kind IN ('standalone', 'recurring') AND payable_id IS NULL) OR
        (kind IN ('debt_plan', 'receivable_plan') AND payable_id IS NOT NULL)
    )
);

CREATE TABLE settings (
    key TEXT PRIMARY KEY,
    value TEXT,
    is_encrypted INTEGER NOT NULL DEFAULT 0 CHECK (is_encrypted IN (0, 1)),
    updated_at TEXT NOT NULL
);

CREATE TABLE statement_imports (
    id TEXT PRIMARY KEY,
    sync_run_id TEXT NOT NULL UNIQUE REFERENCES sync_runs (id),
    source_id TEXT NOT NULL REFERENCES data_sources (id),
    account_id TEXT NOT NULL REFERENCES financial_accounts (id),
    format TEXT NOT NULL,
    adapter_version TEXT NOT NULL,
    filename TEXT NOT NULL,
    file_sha256 TEXT NOT NULL,
    rows_total INTEGER NOT NULL CHECK (rows_total >= 0),
    rows_invalid INTEGER NOT NULL DEFAULT 0 CHECK (rows_invalid >= 0),
    rows_duplicate INTEGER NOT NULL DEFAULT 0 CHECK (rows_duplicate >= 0),
    created_at TEXT NOT NULL
);

CREATE TABLE "sync_failures" (
    id TEXT PRIMARY KEY,
    sync_run_id TEXT NOT NULL REFERENCES sync_runs (id),
    raw_import_id TEXT REFERENCES raw_imports (id),
    account_id TEXT REFERENCES financial_accounts (id),
    external_account_id TEXT,
    external_transaction_id TEXT,
    investment_id TEXT REFERENCES financial_investments (id),
    external_investment_id TEXT,
    bill_id TEXT REFERENCES financial_bills (id),
    external_bill_id TEXT,
    stage TEXT NOT NULL CHECK (stage IN (
        'auth', 'item', 'accounts', 'account', 'transactions',
        'investments', 'investment', 'investment_transactions',
        'bills', 'normalize', 'interrupted', 'worker_unavailable'
    )),
    error_code TEXT NOT NULL,
    safe_message TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE sync_runs (
    id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL REFERENCES data_sources (id),
    status TEXT NOT NULL DEFAULT 'in_progress'
        CHECK (status IN ('in_progress', 'completed', 'completed_with_failures', 'failed')),
    started_at TEXT NOT NULL,
    finished_at TEXT,
    heartbeat_at TEXT,
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

CREATE TABLE "transaction_category_decisions" (
    transaction_id TEXT PRIMARY KEY REFERENCES financial_transactions (id) ON DELETE RESTRICT,
    category_id TEXT NOT NULL REFERENCES categories (id) ON DELETE RESTRICT,
    revision INTEGER NOT NULL CHECK (revision >= 1),
    changed_at TEXT NOT NULL,
    origin TEXT NOT NULL CHECK (origin IN ('manual', 'automatic', 'rule', 'learned'))
);

CREATE TABLE "transaction_category_events" (
    id TEXT PRIMARY KEY,
    transaction_id TEXT NOT NULL REFERENCES financial_transactions (id) ON DELETE RESTRICT,
    revision INTEGER NOT NULL CHECK (revision >= 1),
    previous_category_id TEXT,
    resulting_category_id TEXT NOT NULL,
    changed_at TEXT NOT NULL,
    origin TEXT NOT NULL CHECK (origin IN ('manual', 'automatic', 'rule', 'learned')),
    UNIQUE (transaction_id, revision)
);

CREATE TABLE transaction_inclusion_decisions (
    transaction_id TEXT PRIMARY KEY REFERENCES financial_transactions (id) ON DELETE RESTRICT,
    state TEXT NOT NULL CHECK (state IN ('considered', 'ignored')),
    revision INTEGER NOT NULL CHECK (revision >= 1),
    changed_at TEXT NOT NULL,
    origin TEXT NOT NULL CHECK (origin IN ('manual', 'rule')),
    rule_id TEXT REFERENCES automation_rules (id) ON DELETE SET NULL,
    rule_name TEXT,
    CHECK (origin = 'rule' OR (rule_id IS NULL AND rule_name IS NULL))
);

CREATE TABLE transaction_inclusion_events (
    id TEXT PRIMARY KEY,
    transaction_id TEXT NOT NULL REFERENCES financial_transactions (id) ON DELETE RESTRICT,
    revision INTEGER NOT NULL CHECK (revision >= 1),
    previous_state TEXT NOT NULL CHECK (previous_state IN ('considered', 'ignored')),
    resulting_state TEXT NOT NULL CHECK (resulting_state IN ('considered', 'ignored')),
    changed_at TEXT NOT NULL,
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
    occurred_at DESC, id DESC
);

CREATE INDEX ix_financial_transactions_account_id ON financial_transactions (account_id);

CREATE INDEX ix_financial_transactions_occurred_at_id_desc ON financial_transactions (
    occurred_at DESC, id DESC
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
    WHERE is_accounting_source = 1;

CREATE UNIQUE INDEX uq_sync_runs_active_source ON sync_runs (source_id)
    WHERE status = 'in_progress';

CREATE TRIGGER normalization_events_reject_delete
BEFORE DELETE ON normalization_events
BEGIN
    SELECT RAISE(ABORT, 'normalization_events is append-only');
END;

CREATE TRIGGER normalization_events_reject_update
BEFORE UPDATE ON normalization_events
BEGIN
    SELECT RAISE(ABORT, 'normalization_events is append-only');
END;

CREATE TRIGGER raw_imports_reject_delete BEFORE DELETE ON raw_imports
BEGIN SELECT RAISE(ABORT, 'raw_imports is append-only'); END;

CREATE TRIGGER raw_imports_reject_update BEFORE UPDATE ON raw_imports
BEGIN SELECT RAISE(ABORT, 'raw_imports is append-only'); END;

CREATE TRIGGER transaction_category_events_reject_delete
BEFORE DELETE ON transaction_category_events
BEGIN
    SELECT RAISE(ABORT, 'transaction_category_events is append-only');
END;

CREATE TRIGGER transaction_category_events_reject_update
BEFORE UPDATE ON transaction_category_events
BEGIN
    SELECT RAISE(ABORT, 'transaction_category_events is append-only');
END;

CREATE TRIGGER transaction_inclusion_events_reject_delete
BEFORE DELETE ON transaction_inclusion_events
BEGIN
    SELECT RAISE(ABORT, 'transaction_inclusion_events is append-only');
END;

CREATE TRIGGER transaction_inclusion_events_reject_update
BEFORE UPDATE ON transaction_inclusion_events
BEGIN
    SELECT RAISE(ABORT, 'transaction_inclusion_events is append-only');
END;
