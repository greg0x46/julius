-- Final legacy schema after Postgres migration 00046_manual_bank_accounts.
-- Schema only; pg_dump after internal/db.Open on a disposable database.
--
-- PostgreSQL database dump
--



SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET transaction_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: reject_append_only_write(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.reject_append_only_write() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION '% is append-only', TG_TABLE_NAME;
END;
$$;


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: account_balance_adjustments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.account_balance_adjustments (
    id text NOT NULL,
    account_id text NOT NULL,
    amount text NOT NULL,
    description text NOT NULL,
    created_at text NOT NULL
);


--
-- Name: app_auth; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.app_auth (
    id integer NOT NULL,
    salt text NOT NULL,
    verifier text NOT NULL,
    created_at text NOT NULL,
    updated_at text NOT NULL,
    CONSTRAINT app_auth_id_check CHECK ((id = 1))
);


--
-- Name: auth_owner; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.auth_owner (
    id integer NOT NULL,
    email text NOT NULL,
    password_hash text NOT NULL,
    revision integer DEFAULT 1 NOT NULL,
    created_at bigint NOT NULL,
    CONSTRAINT auth_owner_id_check CHECK ((id = 1))
);


--
-- Name: auth_sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.auth_sessions (
    token_hash text NOT NULL,
    owner_id integer NOT NULL,
    revision integer NOT NULL,
    created_at bigint NOT NULL,
    last_seen_at bigint NOT NULL,
    expires_at bigint NOT NULL
);


--
-- Name: automation_rule_actions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.automation_rule_actions (
    id text NOT NULL,
    rule_id text NOT NULL,
    action_type text NOT NULL,
    scenario_id text,
    category_id text,
    "position" integer NOT NULL,
    CONSTRAINT automation_rule_actions_new_action_type_check1 CHECK ((action_type = ANY (ARRAY['ignore'::text, 'reconcile'::text, 'set_category'::text]))),
    CONSTRAINT automation_rule_actions_new_position_check1 CHECK (("position" >= 0)),
    CONSTRAINT automation_rule_actions_target_check CHECK ((((action_type = 'ignore'::text) AND (scenario_id IS NULL) AND (category_id IS NULL)) OR ((action_type = 'reconcile'::text) AND (scenario_id IS NOT NULL) AND (category_id IS NULL)) OR ((action_type = 'set_category'::text) AND (scenario_id IS NULL) AND (category_id IS NOT NULL))))
);


--
-- Name: automation_rule_conditions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.automation_rule_conditions (
    id text NOT NULL,
    rule_id text NOT NULL,
    field text NOT NULL,
    operator text NOT NULL,
    value text NOT NULL,
    "position" integer NOT NULL,
    CONSTRAINT automation_rule_conditions_new_field_check1 CHECK ((field = ANY (ARRAY['description'::text, 'card'::text, 'account'::text, 'amount'::text, 'day_of_month'::text]))),
    CONSTRAINT automation_rule_conditions_new_operator_check1 CHECK ((operator = ANY (ARRAY['contains'::text, 'equals'::text, 'within_percent'::text, 'day_range'::text]))),
    CONSTRAINT automation_rule_conditions_new_position_check1 CHECK (("position" >= 0)),
    CONSTRAINT automation_rule_conditions_new_value_check1 CHECK ((TRIM(BOTH FROM value) <> ''::text))
);


--
-- Name: automation_rules; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.automation_rules (
    id text NOT NULL,
    name text NOT NULL,
    is_active integer DEFAULT 1 NOT NULL,
    logic_operator text NOT NULL,
    created_at text NOT NULL,
    updated_at text NOT NULL,
    CONSTRAINT automation_rules_is_active_check CHECK ((is_active = ANY (ARRAY[0, 1]))),
    CONSTRAINT automation_rules_logic_operator_check CHECK ((logic_operator = ANY (ARRAY['and'::text, 'or'::text]))),
    CONSTRAINT automation_rules_name_check CHECK ((TRIM(BOTH FROM name) <> ''::text))
);


--
-- Name: categories; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.categories (
    id text NOT NULL,
    name text NOT NULL,
    kind text NOT NULL,
    is_active integer DEFAULT 1 NOT NULL,
    created_at text NOT NULL,
    updated_at text NOT NULL,
    icon text DEFAULT 'ellipsis'::text NOT NULL,
    color text DEFAULT '#6c757d'::text NOT NULL,
    CONSTRAINT categories_is_active_check CHECK ((is_active = ANY (ARRAY[0, 1]))),
    CONSTRAINT categories_kind_check CHECK ((kind = ANY (ARRAY['expense'::text, 'income'::text, 'transfer'::text])))
);


--
-- Name: data_sources; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.data_sources (
    id text NOT NULL,
    provider text NOT NULL,
    external_item_id text NOT NULL,
    display_name text,
    created_at text NOT NULL,
    updated_at text NOT NULL,
    is_active integer DEFAULT 1 NOT NULL,
    label text,
    CONSTRAINT data_sources_is_active_check CHECK ((is_active = ANY (ARRAY[0, 1])))
);


--
-- Name: exchange_rates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.exchange_rates (
    base_currency text NOT NULL,
    quote_currency text NOT NULL,
    as_of text NOT NULL,
    effective_on text NOT NULL,
    rate text NOT NULL,
    source text NOT NULL,
    fetched_at text NOT NULL
);


--
-- Name: financial_accounts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.financial_accounts (
    id text NOT NULL,
    source_id text,
    external_id text,
    institution text,
    name text,
    number text,
    account_type text,
    account_subtype text,
    balance text,
    credit_limit text,
    currency_code text,
    provider_updated_at text,
    current_raw_import_id text,
    normalized_hash text,
    created_at text NOT NULL,
    updated_at text NOT NULL,
    available_credit_limit text,
    balance_close_date text,
    balance_due_date text,
    manual_closing_day integer,
    balance_as_of text,
    origin text DEFAULT 'synced'::text NOT NULL,
    opening_balance text,
    archived_at text,
    CONSTRAINT financial_accounts_manual_closing_day_check CHECK (((manual_closing_day >= 1) AND (manual_closing_day <= 31))),
    CONSTRAINT financial_accounts_origin_check CHECK ((origin = ANY (ARRAY['synced'::text, 'manual'::text]))),
    CONSTRAINT financial_accounts_origin_identity CHECK ((((origin = 'synced'::text) AND (source_id IS NOT NULL) AND (external_id IS NOT NULL) AND (current_raw_import_id IS NOT NULL) AND (normalized_hash IS NOT NULL) AND (opening_balance IS NULL)) OR ((origin = 'manual'::text) AND (source_id IS NULL) AND (external_id IS NULL) AND (current_raw_import_id IS NULL) AND (normalized_hash IS NULL) AND (account_type IS NOT NULL) AND (account_type = 'BANK'::text) AND (currency_code IS NOT NULL) AND (opening_balance IS NOT NULL) AND (balance IS NULL))))
);


--
-- Name: financial_bills; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.financial_bills (
    id text NOT NULL,
    source_id text NOT NULL,
    account_id text NOT NULL,
    external_id text NOT NULL,
    due_date text,
    closing_date text,
    total_amount text,
    currency_code text,
    minimum_payment_amount text,
    current_raw_import_id text NOT NULL,
    normalized_hash text NOT NULL,
    created_at text NOT NULL,
    updated_at text NOT NULL
);


--
-- Name: financial_investment_transactions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.financial_investment_transactions (
    id text NOT NULL,
    source_id text NOT NULL,
    investment_id text NOT NULL,
    external_id text NOT NULL,
    movement_type text,
    quantity text,
    value text,
    amount text,
    occurred_at text,
    trade_date text,
    current_raw_import_id text NOT NULL,
    normalized_hash text NOT NULL,
    created_at text NOT NULL,
    updated_at text NOT NULL,
    direction text,
    CONSTRAINT financial_investment_transactions_direction_check CHECK ((direction = ANY (ARRAY['inflow'::text, 'outflow'::text])))
);


--
-- Name: financial_investments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.financial_investments (
    id text NOT NULL,
    source_id text NOT NULL,
    external_id text NOT NULL,
    investment_type text,
    subtype text,
    name text,
    balance text,
    currency_code text,
    number text,
    owner text,
    tax_number text,
    due_date text,
    issuer text,
    issuer_code text,
    rate text,
    rate_type text,
    fixed_annual_rate text,
    annual_rate text,
    last_twelve_months_rate text,
    quantity text,
    value text,
    amount text,
    amount_profit text,
    amount_withdrawal text,
    as_of_date text,
    provider_updated_at text,
    isin text,
    code text,
    provider_status text,
    current_raw_import_id text NOT NULL,
    normalized_hash text NOT NULL,
    created_at text NOT NULL,
    updated_at text NOT NULL,
    amount_original text,
    taxes text,
    taxes2 text,
    issue_date text,
    purchase_date text,
    issuer_cnpj text
);


--
-- Name: financial_transactions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.financial_transactions (
    id text NOT NULL,
    source_id text,
    account_id text NOT NULL,
    external_id text,
    description text,
    description_raw text,
    amount text,
    amount_in_account_currency text,
    balance_after text,
    currency_code text,
    occurred_at text,
    provider_status text,
    movement_type text,
    source_category text,
    source_category_id text,
    provider_code text,
    payment_data text,
    credit_card_metadata text,
    merchant text,
    operation_type text,
    operation_type_additional_info text,
    operation_category text,
    provider_id text,
    provider_order integer,
    provider_created_at text,
    provider_updated_at text,
    current_raw_import_id text,
    normalized_hash text,
    created_at text NOT NULL,
    updated_at text NOT NULL,
    origin text DEFAULT 'synced'::text NOT NULL,
    deleted_at text,
    CONSTRAINT chk_financial_transactions_origin CHECK ((((origin = 'synced'::text) AND (source_id IS NOT NULL) AND (external_id IS NOT NULL) AND (current_raw_import_id IS NOT NULL) AND (normalized_hash IS NOT NULL)) OR ((origin = 'manual'::text) AND (source_id IS NULL) AND (external_id IS NULL) AND (current_raw_import_id IS NULL) AND (normalized_hash IS NULL))))
);


--
-- Name: goose_db_version; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.goose_db_version (
    id integer NOT NULL,
    version_id bigint NOT NULL,
    is_applied boolean NOT NULL,
    tstamp timestamp without time zone DEFAULT now() NOT NULL
);


--
-- Name: goose_db_version_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.goose_db_version ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.goose_db_version_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: investment_accounts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.investment_accounts (
    id text NOT NULL,
    name text NOT NULL,
    kind text NOT NULL,
    currency_code text,
    source_id text,
    financial_account_id text,
    is_active boolean DEFAULT true NOT NULL,
    created_at text NOT NULL,
    updated_at text NOT NULL,
    CONSTRAINT investment_accounts_check CHECK ((((kind = 'manual'::text) AND (source_id IS NULL) AND (currency_code = 'BRL'::text)) OR ((kind = 'integrated'::text) AND (source_id IS NOT NULL)))),
    CONSTRAINT investment_accounts_kind_check CHECK ((kind = ANY (ARRAY['manual'::text, 'integrated'::text])))
);


--
-- Name: investment_asset_quote_coverage; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.investment_asset_quote_coverage (
    asset_id text NOT NULL,
    source text NOT NULL,
    symbol text NOT NULL,
    covered_from text NOT NULL,
    covered_to text NOT NULL,
    updated_at text NOT NULL
);


--
-- Name: investment_asset_quotes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.investment_asset_quotes (
    asset_id text NOT NULL,
    quoted_on text NOT NULL,
    price text NOT NULL,
    source text NOT NULL,
    raw_import_id text,
    created_at text NOT NULL,
    updated_at text NOT NULL,
    origin text DEFAULT 'market'::text NOT NULL,
    CONSTRAINT investment_asset_quotes_origin_check CHECK ((origin = ANY (ARRAY['sync'::text, 'issue'::text, 'market'::text])))
);


--
-- Name: investment_assets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.investment_assets (
    id text NOT NULL,
    canonical_key text NOT NULL,
    name text NOT NULL,
    ticker text,
    asset_type text NOT NULL,
    currency_code text DEFAULT 'BRL'::text NOT NULL,
    created_at text NOT NULL,
    updated_at text NOT NULL,
    quote_source text,
    quote_symbol text,
    asset_class text DEFAULT 'other'::text NOT NULL,
    CONSTRAINT investment_assets_asset_class_check CHECK ((asset_class = ANY (ARRAY['fixed_income'::text, 'variable_income'::text, 'multimarket'::text, 'currency'::text, 'crypto'::text, 'other'::text])))
);


--
-- Name: investment_operations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.investment_operations (
    id text NOT NULL,
    account_id text NOT NULL,
    position_id text,
    kind text NOT NULL,
    occurred_on text NOT NULL,
    amount text NOT NULL,
    quantity text,
    unit_price text,
    fees text DEFAULT '0'::text NOT NULL,
    taxes text DEFAULT '0'::text NOT NULL,
    notes text,
    source text DEFAULT 'manual'::text NOT NULL,
    transfer_id text,
    created_at text NOT NULL,
    updated_at text NOT NULL,
    CONSTRAINT investment_operations_kind_check CHECK ((kind = ANY (ARRAY['initial_balance'::text, 'deposit'::text, 'withdrawal'::text, 'buy'::text, 'sell'::text, 'income'::text, 'fee'::text, 'tax'::text, 'valuation'::text, 'transfer_out'::text, 'transfer_in'::text]))),
    CONSTRAINT investment_operations_source_check CHECK ((source = ANY (ARRAY['manual'::text, 'synced'::text])))
);


--
-- Name: investment_portfolios; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.investment_portfolios (
    id text NOT NULL,
    name text NOT NULL,
    target_amount text,
    target_date text,
    notes text,
    created_at text NOT NULL,
    updated_at text NOT NULL
);


--
-- Name: investment_position_portfolios; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.investment_position_portfolios (
    manual_position_id text,
    financial_investment_id text,
    portfolio_id text NOT NULL,
    created_at text NOT NULL,
    updated_at text NOT NULL,
    CONSTRAINT investment_position_portfolios_check CHECK ((((manual_position_id IS NOT NULL) AND (financial_investment_id IS NULL)) OR ((manual_position_id IS NULL) AND (financial_investment_id IS NOT NULL))))
);


--
-- Name: investment_positions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.investment_positions (
    id text NOT NULL,
    account_id text NOT NULL,
    asset_id text NOT NULL,
    notes text,
    created_at text NOT NULL,
    updated_at text NOT NULL
);


--
-- Name: investment_reconciliations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.investment_reconciliations (
    id text NOT NULL,
    operation_id text NOT NULL,
    financial_transaction_id text,
    financial_investment_transaction_id text,
    amount text NOT NULL,
    created_at text NOT NULL,
    CONSTRAINT investment_reconciliations_check CHECK (((financial_transaction_id IS NOT NULL) OR (financial_investment_transaction_id IS NOT NULL)))
);


--
-- Name: manual_transaction_idempotency; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.manual_transaction_idempotency (
    owner_id integer NOT NULL,
    idempotency_key text NOT NULL,
    request_hash text NOT NULL,
    transaction_id text,
    created_at text NOT NULL
);


--
-- Name: net_worth_snapshots; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.net_worth_snapshots (
    id text NOT NULL,
    captured_at text NOT NULL,
    total_assets text NOT NULL,
    total_liabilities text NOT NULL,
    net_worth text NOT NULL,
    cash_balance text NOT NULL,
    investment_balance text NOT NULL,
    credit_card_balance text NOT NULL,
    payables_debt text NOT NULL,
    created_at text NOT NULL,
    updated_at text NOT NULL,
    is_backfilled integer DEFAULT 0 NOT NULL,
    CONSTRAINT net_worth_snapshots_is_backfilled_check CHECK ((is_backfilled = ANY (ARRAY[0, 1])))
);


--
-- Name: normalization_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.normalization_events (
    id text NOT NULL,
    sync_run_id text NOT NULL,
    raw_import_id text NOT NULL,
    entity_type text NOT NULL,
    account_id text,
    transaction_id text,
    investment_id text,
    investment_transaction_id text,
    external_id text,
    outcome text NOT NULL,
    normalized_hash text,
    created_at text NOT NULL,
    bill_id text,
    CONSTRAINT normalization_events_check CHECK ((((outcome = 'rejected'::text) AND (account_id IS NULL) AND (transaction_id IS NULL) AND (investment_id IS NULL) AND (investment_transaction_id IS NULL) AND (bill_id IS NULL)) OR ((outcome <> 'rejected'::text) AND (((((((account_id IS NOT NULL))::integer + ((transaction_id IS NOT NULL))::integer) + ((investment_id IS NOT NULL))::integer) + ((investment_transaction_id IS NOT NULL))::integer) + ((bill_id IS NOT NULL))::integer) = 1)))),
    CONSTRAINT normalization_events_entity_type_check CHECK ((entity_type = ANY (ARRAY['account'::text, 'transaction'::text, 'investment'::text, 'investment_transaction'::text, 'bill'::text]))),
    CONSTRAINT normalization_events_outcome_check CHECK ((outcome = ANY (ARRAY['inserted'::text, 'updated'::text, 'unchanged'::text, 'rejected'::text])))
);


--
-- Name: payable_transaction_links; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.payable_transaction_links (
    id text NOT NULL,
    payable_id text NOT NULL,
    transaction_id text NOT NULL,
    linked_amount text NOT NULL,
    linked_at text NOT NULL
);


--
-- Name: payables; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.payables (
    id text NOT NULL,
    kind text NOT NULL,
    name text NOT NULL,
    total_amount text NOT NULL,
    starting_settled_amount text DEFAULT '0'::text NOT NULL,
    created_at text NOT NULL,
    updated_at text NOT NULL,
    CONSTRAINT payables_kind_check CHECK ((kind = ANY (ARRAY['debt'::text, 'receivable'::text]))),
    CONSTRAINT payables_starting_settled_amount_check CHECK (((starting_settled_amount)::real >= (0)::double precision)),
    CONSTRAINT payables_total_amount_check CHECK (((total_amount)::real > (0)::double precision))
);


--
-- Name: raw_imports; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.raw_imports (
    id text NOT NULL,
    sync_run_id text NOT NULL,
    source_id text NOT NULL,
    scope text NOT NULL,
    external_account_id text,
    page_sequence integer NOT NULL,
    request_attempt integer NOT NULL,
    request_method text,
    request_path text,
    http_status integer,
    response_headers text,
    payload bytea NOT NULL,
    payload_sha256 text NOT NULL,
    received_at text NOT NULL,
    CONSTRAINT raw_imports_http_fields_check CHECK (((scope = 'file'::text) OR ((request_method IS NOT NULL) AND (request_path IS NOT NULL) AND (http_status IS NOT NULL) AND (response_headers IS NOT NULL)))),
    CONSTRAINT raw_imports_page_sequence_check CHECK ((page_sequence >= 1)),
    CONSTRAINT raw_imports_request_attempt_check CHECK ((request_attempt >= 1)),
    CONSTRAINT raw_imports_scope_check CHECK ((scope = ANY (ARRAY['item'::text, 'accounts'::text, 'transactions'::text, 'investments'::text, 'investment_transactions'::text, 'bills'::text, 'file'::text])))
);


--
-- Name: scenario_realizations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.scenario_realizations (
    id text NOT NULL,
    scenario_id text NOT NULL,
    scenario_transaction_id text,
    occurrence_date text,
    transaction_id text,
    relation_type text NOT NULL,
    state text NOT NULL,
    origin text NOT NULL,
    allocated_amount text,
    linked_amount text,
    created_at text NOT NULL,
    CONSTRAINT scenario_realizations_check CHECK ((((state = 'linked'::text) AND (transaction_id IS NOT NULL)) OR ((state = 'detached'::text) AND (transaction_id IS NULL)))),
    CONSTRAINT scenario_realizations_check1 CHECK ((((relation_type = 'settlement'::text) AND (scenario_transaction_id IS NULL) AND (occurrence_date IS NULL) AND (state = 'linked'::text) AND (linked_amount IS NOT NULL)) OR ((relation_type = 'allocation'::text) AND (scenario_transaction_id IS NOT NULL) AND (occurrence_date IS NULL) AND (state = 'linked'::text) AND (allocated_amount IS NOT NULL)) OR ((relation_type = 'reconciliation'::text) AND (scenario_transaction_id IS NULL) AND (occurrence_date IS NOT NULL)))),
    CONSTRAINT scenario_realizations_relation_type_check CHECK ((relation_type = ANY (ARRAY['settlement'::text, 'allocation'::text, 'reconciliation'::text]))),
    CONSTRAINT scenario_realizations_state_check CHECK ((state = ANY (ARRAY['linked'::text, 'detached'::text])))
);


--
-- Name: scenario_recurring_schedules; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.scenario_recurring_schedules (
    scenario_id text NOT NULL,
    cashflow_kind text NOT NULL,
    amount text NOT NULL,
    category_id text,
    account_id text,
    cadence text NOT NULL,
    day_of_month integer NOT NULL,
    month_of_year integer,
    start_date text NOT NULL,
    end_date text,
    CONSTRAINT scenario_recurring_schedules_amount_check CHECK (((amount)::numeric > (0)::numeric)),
    CONSTRAINT scenario_recurring_schedules_cadence_check CHECK ((cadence = ANY (ARRAY['monthly'::text, 'annual'::text]))),
    CONSTRAINT scenario_recurring_schedules_cashflow_kind_check CHECK ((cashflow_kind = ANY (ARRAY['income'::text, 'expense'::text]))),
    CONSTRAINT scenario_recurring_schedules_check CHECK (((cadence <> 'annual'::text) OR (month_of_year IS NOT NULL))),
    CONSTRAINT scenario_recurring_schedules_day_of_month_check CHECK (((day_of_month >= 1) AND (day_of_month <= 31))),
    CONSTRAINT scenario_recurring_schedules_month_of_year_check CHECK (((month_of_year >= 1) AND (month_of_year <= 12)))
);


--
-- Name: scenario_transactions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.scenario_transactions (
    id text NOT NULL,
    scenario_id text NOT NULL,
    description text NOT NULL,
    amount text NOT NULL,
    projected_at text NOT NULL,
    category text,
    created_at text NOT NULL,
    updated_at text NOT NULL
);


--
-- Name: scenarios; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.scenarios (
    id text NOT NULL,
    kind text NOT NULL,
    name text NOT NULL,
    created_at text NOT NULL,
    updated_at text NOT NULL,
    payable_id text,
    is_active integer DEFAULT 1 NOT NULL,
    is_accounting_source integer DEFAULT 0 NOT NULL,
    CONSTRAINT scenarios_accounting_source_check CHECK (((is_accounting_source = 0) OR (kind = ANY (ARRAY['debt_plan'::text, 'receivable_plan'::text])))),
    CONSTRAINT scenarios_is_accounting_source_check CHECK ((is_accounting_source = ANY (ARRAY[0, 1]))),
    CONSTRAINT scenarios_is_active_check CHECK ((is_active = ANY (ARRAY[0, 1]))),
    CONSTRAINT scenarios_kind_check CHECK ((kind = ANY (ARRAY['debt_plan'::text, 'receivable_plan'::text, 'standalone'::text, 'recurring'::text]))),
    CONSTRAINT scenarios_payable_id_check CHECK ((((kind = ANY (ARRAY['standalone'::text, 'recurring'::text])) AND (payable_id IS NULL)) OR ((kind = ANY (ARRAY['debt_plan'::text, 'receivable_plan'::text])) AND (payable_id IS NOT NULL))))
);


--
-- Name: settings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.settings (
    key text NOT NULL,
    value text,
    is_encrypted integer DEFAULT 0 NOT NULL,
    updated_at text NOT NULL,
    CONSTRAINT settings_is_encrypted_check CHECK ((is_encrypted = ANY (ARRAY[0, 1])))
);


--
-- Name: statement_imports; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.statement_imports (
    id text NOT NULL,
    sync_run_id text NOT NULL,
    source_id text NOT NULL,
    account_id text NOT NULL,
    format text NOT NULL,
    adapter_version text NOT NULL,
    filename text NOT NULL,
    file_sha256 text NOT NULL,
    rows_total integer NOT NULL,
    rows_invalid integer DEFAULT 0 NOT NULL,
    rows_duplicate integer DEFAULT 0 NOT NULL,
    created_at text NOT NULL,
    CONSTRAINT statement_imports_rows_duplicate_check CHECK ((rows_duplicate >= 0)),
    CONSTRAINT statement_imports_rows_invalid_check CHECK ((rows_invalid >= 0)),
    CONSTRAINT statement_imports_rows_total_check CHECK ((rows_total >= 0))
);


--
-- Name: sync_failures; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sync_failures (
    id text NOT NULL,
    sync_run_id text NOT NULL,
    raw_import_id text,
    account_id text,
    external_account_id text,
    external_transaction_id text,
    investment_id text,
    external_investment_id text,
    stage text NOT NULL,
    error_code text NOT NULL,
    safe_message text NOT NULL,
    created_at text NOT NULL,
    bill_id text,
    external_bill_id text,
    CONSTRAINT sync_failures_stage_check CHECK ((stage = ANY (ARRAY['auth'::text, 'item'::text, 'accounts'::text, 'account'::text, 'transactions'::text, 'investments'::text, 'investment'::text, 'investment_transactions'::text, 'bills'::text, 'normalize'::text, 'interrupted'::text, 'worker_unavailable'::text])))
);


--
-- Name: sync_runs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sync_runs (
    id text NOT NULL,
    source_id text NOT NULL,
    status text DEFAULT 'in_progress'::text NOT NULL,
    started_at text NOT NULL,
    finished_at text,
    heartbeat_at text,
    worker_id text,
    accounts_processed integer DEFAULT 0 NOT NULL,
    transactions_inserted integer DEFAULT 0 NOT NULL,
    transactions_updated integer DEFAULT 0 NOT NULL,
    investments_processed integer DEFAULT 0 NOT NULL,
    investment_transactions_inserted integer DEFAULT 0 NOT NULL,
    investment_transactions_updated integer DEFAULT 0 NOT NULL,
    general_error_code text,
    general_error_message text,
    bills_processed integer DEFAULT 0 NOT NULL,
    run_type text DEFAULT 'sync'::text NOT NULL,
    CONSTRAINT sync_runs_accounts_processed_check CHECK ((accounts_processed >= 0)),
    CONSTRAINT sync_runs_bills_processed_check CHECK ((bills_processed >= 0)),
    CONSTRAINT sync_runs_check CHECK ((((status = 'in_progress'::text) AND (finished_at IS NULL)) OR ((status <> 'in_progress'::text) AND (finished_at IS NOT NULL)))),
    CONSTRAINT sync_runs_run_type_check CHECK ((run_type = ANY (ARRAY['sync'::text, 'file_import'::text]))),
    CONSTRAINT sync_runs_status_check CHECK ((status = ANY (ARRAY['in_progress'::text, 'completed'::text, 'completed_with_failures'::text, 'failed'::text]))),
    CONSTRAINT sync_runs_transactions_inserted_check CHECK ((transactions_inserted >= 0)),
    CONSTRAINT sync_runs_transactions_updated_check CHECK ((transactions_updated >= 0))
);


--
-- Name: transaction_category_decisions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.transaction_category_decisions (
    transaction_id text NOT NULL,
    category_id text NOT NULL,
    revision integer NOT NULL,
    changed_at text NOT NULL,
    origin text NOT NULL,
    CONSTRAINT transaction_category_decisions_new_origin_check1 CHECK ((origin = ANY (ARRAY['manual'::text, 'automatic'::text, 'rule'::text, 'learned'::text]))),
    CONSTRAINT transaction_category_decisions_new_revision_check1 CHECK ((revision >= 1))
);


--
-- Name: transaction_category_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.transaction_category_events (
    id text NOT NULL,
    transaction_id text NOT NULL,
    revision integer NOT NULL,
    previous_category_id text,
    resulting_category_id text NOT NULL,
    changed_at text NOT NULL,
    origin text NOT NULL,
    CONSTRAINT transaction_category_events_new_origin_check1 CHECK ((origin = ANY (ARRAY['manual'::text, 'automatic'::text, 'rule'::text, 'learned'::text]))),
    CONSTRAINT transaction_category_events_new_revision_check1 CHECK ((revision >= 1))
);


--
-- Name: transaction_inclusion_decisions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.transaction_inclusion_decisions (
    transaction_id text NOT NULL,
    state text NOT NULL,
    revision integer NOT NULL,
    changed_at text NOT NULL,
    origin text NOT NULL,
    rule_id text,
    rule_name text,
    CONSTRAINT transaction_inclusion_decisions_check CHECK (((origin = 'rule'::text) OR ((rule_id IS NULL) AND (rule_name IS NULL)))),
    CONSTRAINT transaction_inclusion_decisions_origin_check CHECK ((origin = ANY (ARRAY['manual'::text, 'rule'::text]))),
    CONSTRAINT transaction_inclusion_decisions_revision_check CHECK ((revision >= 1)),
    CONSTRAINT transaction_inclusion_decisions_state_check CHECK ((state = ANY (ARRAY['considered'::text, 'ignored'::text])))
);


--
-- Name: transaction_inclusion_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.transaction_inclusion_events (
    id text NOT NULL,
    transaction_id text NOT NULL,
    revision integer NOT NULL,
    previous_state text NOT NULL,
    resulting_state text NOT NULL,
    changed_at text NOT NULL,
    origin text NOT NULL,
    rule_id text,
    rule_name text,
    CONSTRAINT transaction_inclusion_events_check CHECK ((previous_state <> resulting_state)),
    CONSTRAINT transaction_inclusion_events_check1 CHECK (((origin = 'rule'::text) OR ((rule_id IS NULL) AND (rule_name IS NULL)))),
    CONSTRAINT transaction_inclusion_events_origin_check CHECK ((origin = ANY (ARRAY['manual'::text, 'rule'::text]))),
    CONSTRAINT transaction_inclusion_events_previous_state_check CHECK ((previous_state = ANY (ARRAY['considered'::text, 'ignored'::text]))),
    CONSTRAINT transaction_inclusion_events_resulting_state_check CHECK ((resulting_state = ANY (ARRAY['considered'::text, 'ignored'::text]))),
    CONSTRAINT transaction_inclusion_events_revision_check CHECK ((revision >= 1))
);


--
-- Name: account_balance_adjustments account_balance_adjustments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_balance_adjustments
    ADD CONSTRAINT account_balance_adjustments_pkey PRIMARY KEY (id);


--
-- Name: app_auth app_auth_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.app_auth
    ADD CONSTRAINT app_auth_pkey PRIMARY KEY (id);


--
-- Name: auth_owner auth_owner_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.auth_owner
    ADD CONSTRAINT auth_owner_pkey PRIMARY KEY (id);


--
-- Name: auth_sessions auth_sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.auth_sessions
    ADD CONSTRAINT auth_sessions_pkey PRIMARY KEY (token_hash);


--
-- Name: automation_rule_actions automation_rule_actions_new_pkey1; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.automation_rule_actions
    ADD CONSTRAINT automation_rule_actions_new_pkey1 PRIMARY KEY (id);


--
-- Name: automation_rule_actions automation_rule_actions_new_rule_id_position_key1; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.automation_rule_actions
    ADD CONSTRAINT automation_rule_actions_new_rule_id_position_key1 UNIQUE (rule_id, "position");


--
-- Name: automation_rule_conditions automation_rule_conditions_new_pkey1; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.automation_rule_conditions
    ADD CONSTRAINT automation_rule_conditions_new_pkey1 PRIMARY KEY (id);


--
-- Name: automation_rule_conditions automation_rule_conditions_new_rule_id_position_key1; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.automation_rule_conditions
    ADD CONSTRAINT automation_rule_conditions_new_rule_id_position_key1 UNIQUE (rule_id, "position");


--
-- Name: automation_rules automation_rules_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.automation_rules
    ADD CONSTRAINT automation_rules_pkey PRIMARY KEY (id);


--
-- Name: categories categories_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.categories
    ADD CONSTRAINT categories_pkey PRIMARY KEY (id);


--
-- Name: data_sources data_sources_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.data_sources
    ADD CONSTRAINT data_sources_pkey PRIMARY KEY (id);


--
-- Name: data_sources data_sources_provider_external_item_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.data_sources
    ADD CONSTRAINT data_sources_provider_external_item_id_key UNIQUE (provider, external_item_id);


--
-- Name: exchange_rates exchange_rates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.exchange_rates
    ADD CONSTRAINT exchange_rates_pkey PRIMARY KEY (base_currency, quote_currency, as_of);


--
-- Name: financial_accounts financial_accounts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.financial_accounts
    ADD CONSTRAINT financial_accounts_pkey PRIMARY KEY (id);


--
-- Name: financial_accounts financial_accounts_source_id_external_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.financial_accounts
    ADD CONSTRAINT financial_accounts_source_id_external_id_key UNIQUE (source_id, external_id);


--
-- Name: financial_bills financial_bills_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.financial_bills
    ADD CONSTRAINT financial_bills_pkey PRIMARY KEY (id);


--
-- Name: financial_bills financial_bills_source_id_external_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.financial_bills
    ADD CONSTRAINT financial_bills_source_id_external_id_key UNIQUE (source_id, external_id);


--
-- Name: financial_investment_transactions financial_investment_transactions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.financial_investment_transactions
    ADD CONSTRAINT financial_investment_transactions_pkey PRIMARY KEY (id);


--
-- Name: financial_investment_transactions financial_investment_transactions_source_id_external_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.financial_investment_transactions
    ADD CONSTRAINT financial_investment_transactions_source_id_external_id_key UNIQUE (source_id, external_id);


--
-- Name: financial_investments financial_investments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.financial_investments
    ADD CONSTRAINT financial_investments_pkey PRIMARY KEY (id);


--
-- Name: financial_investments financial_investments_source_id_external_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.financial_investments
    ADD CONSTRAINT financial_investments_source_id_external_id_key UNIQUE (source_id, external_id);


--
-- Name: financial_transactions financial_transactions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.financial_transactions
    ADD CONSTRAINT financial_transactions_pkey PRIMARY KEY (id);


--
-- Name: financial_transactions financial_transactions_source_id_external_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.financial_transactions
    ADD CONSTRAINT financial_transactions_source_id_external_id_key UNIQUE (source_id, external_id);


--
-- Name: goose_db_version goose_db_version_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.goose_db_version
    ADD CONSTRAINT goose_db_version_pkey PRIMARY KEY (id);


--
-- Name: investment_accounts investment_accounts_financial_account_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.investment_accounts
    ADD CONSTRAINT investment_accounts_financial_account_id_key UNIQUE (financial_account_id);


--
-- Name: investment_accounts investment_accounts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.investment_accounts
    ADD CONSTRAINT investment_accounts_pkey PRIMARY KEY (id);


--
-- Name: investment_accounts investment_accounts_source_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.investment_accounts
    ADD CONSTRAINT investment_accounts_source_id_key UNIQUE (source_id);


--
-- Name: investment_asset_quote_coverage investment_asset_quote_coverage_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.investment_asset_quote_coverage
    ADD CONSTRAINT investment_asset_quote_coverage_pkey PRIMARY KEY (asset_id);


--
-- Name: investment_asset_quotes investment_asset_quotes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.investment_asset_quotes
    ADD CONSTRAINT investment_asset_quotes_pkey PRIMARY KEY (asset_id, quoted_on);


--
-- Name: investment_assets investment_assets_canonical_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.investment_assets
    ADD CONSTRAINT investment_assets_canonical_key_key UNIQUE (canonical_key);


--
-- Name: investment_assets investment_assets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.investment_assets
    ADD CONSTRAINT investment_assets_pkey PRIMARY KEY (id);


--
-- Name: investment_operations investment_operations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.investment_operations
    ADD CONSTRAINT investment_operations_pkey PRIMARY KEY (id);


--
-- Name: investment_portfolios investment_portfolios_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.investment_portfolios
    ADD CONSTRAINT investment_portfolios_pkey PRIMARY KEY (id);


--
-- Name: investment_position_portfolios investment_position_portfolios_financial_investment_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.investment_position_portfolios
    ADD CONSTRAINT investment_position_portfolios_financial_investment_id_key UNIQUE (financial_investment_id);


--
-- Name: investment_position_portfolios investment_position_portfolios_manual_position_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.investment_position_portfolios
    ADD CONSTRAINT investment_position_portfolios_manual_position_id_key UNIQUE (manual_position_id);


--
-- Name: investment_positions investment_positions_account_id_asset_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.investment_positions
    ADD CONSTRAINT investment_positions_account_id_asset_id_key UNIQUE (account_id, asset_id);


--
-- Name: investment_positions investment_positions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.investment_positions
    ADD CONSTRAINT investment_positions_pkey PRIMARY KEY (id);


--
-- Name: investment_reconciliations investment_reconciliations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.investment_reconciliations
    ADD CONSTRAINT investment_reconciliations_pkey PRIMARY KEY (id);


--
-- Name: manual_transaction_idempotency manual_transaction_idempotency_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.manual_transaction_idempotency
    ADD CONSTRAINT manual_transaction_idempotency_pkey PRIMARY KEY (owner_id, idempotency_key);


--
-- Name: net_worth_snapshots net_worth_snapshots_captured_at_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.net_worth_snapshots
    ADD CONSTRAINT net_worth_snapshots_captured_at_key UNIQUE (captured_at);


--
-- Name: net_worth_snapshots net_worth_snapshots_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.net_worth_snapshots
    ADD CONSTRAINT net_worth_snapshots_pkey PRIMARY KEY (id);


--
-- Name: normalization_events normalization_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.normalization_events
    ADD CONSTRAINT normalization_events_pkey PRIMARY KEY (id);


--
-- Name: payable_transaction_links payable_transaction_links_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payable_transaction_links
    ADD CONSTRAINT payable_transaction_links_pkey PRIMARY KEY (id);


--
-- Name: payable_transaction_links payable_transaction_links_transaction_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payable_transaction_links
    ADD CONSTRAINT payable_transaction_links_transaction_id_key UNIQUE (transaction_id);


--
-- Name: payables payables_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payables
    ADD CONSTRAINT payables_pkey PRIMARY KEY (id);


--
-- Name: raw_imports raw_imports_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.raw_imports
    ADD CONSTRAINT raw_imports_pkey PRIMARY KEY (id);


--
-- Name: scenario_realizations scenario_realizations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scenario_realizations
    ADD CONSTRAINT scenario_realizations_pkey PRIMARY KEY (id);


--
-- Name: scenario_recurring_schedules scenario_recurring_schedules_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scenario_recurring_schedules
    ADD CONSTRAINT scenario_recurring_schedules_pkey PRIMARY KEY (scenario_id);


--
-- Name: scenario_transactions scenario_transactions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scenario_transactions
    ADD CONSTRAINT scenario_transactions_pkey PRIMARY KEY (id);


--
-- Name: scenarios scenarios_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scenarios
    ADD CONSTRAINT scenarios_pkey PRIMARY KEY (id);


--
-- Name: settings settings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.settings
    ADD CONSTRAINT settings_pkey PRIMARY KEY (key);


--
-- Name: statement_imports statement_imports_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.statement_imports
    ADD CONSTRAINT statement_imports_pkey PRIMARY KEY (id);


--
-- Name: statement_imports statement_imports_sync_run_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.statement_imports
    ADD CONSTRAINT statement_imports_sync_run_id_key UNIQUE (sync_run_id);


--
-- Name: sync_failures sync_failures_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sync_failures
    ADD CONSTRAINT sync_failures_pkey PRIMARY KEY (id);


--
-- Name: sync_runs sync_runs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sync_runs
    ADD CONSTRAINT sync_runs_pkey PRIMARY KEY (id);


--
-- Name: transaction_category_decisions transaction_category_decisions_new_pkey1; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.transaction_category_decisions
    ADD CONSTRAINT transaction_category_decisions_new_pkey1 PRIMARY KEY (transaction_id);


--
-- Name: transaction_category_events transaction_category_events_new_pkey1; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.transaction_category_events
    ADD CONSTRAINT transaction_category_events_new_pkey1 PRIMARY KEY (id);


--
-- Name: transaction_category_events transaction_category_events_new_transaction_id_revision_key1; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.transaction_category_events
    ADD CONSTRAINT transaction_category_events_new_transaction_id_revision_key1 UNIQUE (transaction_id, revision);


--
-- Name: transaction_inclusion_decisions transaction_inclusion_decisions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.transaction_inclusion_decisions
    ADD CONSTRAINT transaction_inclusion_decisions_pkey PRIMARY KEY (transaction_id);


--
-- Name: transaction_inclusion_events transaction_inclusion_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.transaction_inclusion_events
    ADD CONSTRAINT transaction_inclusion_events_pkey PRIMARY KEY (id);


--
-- Name: transaction_inclusion_events transaction_inclusion_events_transaction_id_revision_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.transaction_inclusion_events
    ADD CONSTRAINT transaction_inclusion_events_transaction_id_revision_key UNIQUE (transaction_id, revision);


--
-- Name: auth_sessions_expiry; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX auth_sessions_expiry ON public.auth_sessions USING btree (expires_at);


--
-- Name: idx_automation_rule_actions_category_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_automation_rule_actions_category_id ON public.automation_rule_actions USING btree (category_id);


--
-- Name: idx_automation_rule_actions_scenario_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_automation_rule_actions_scenario_id ON public.automation_rule_actions USING btree (scenario_id);


--
-- Name: idx_scenario_realizations_scenario_occurrence; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_scenario_realizations_scenario_occurrence ON public.scenario_realizations USING btree (scenario_id, occurrence_date);


--
-- Name: idx_scenario_realizations_transaction_event; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_scenario_realizations_transaction_event ON public.scenario_realizations USING btree (scenario_transaction_id, transaction_id);


--
-- Name: idx_scenario_realizations_transaction_settlement_reconciliation; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_scenario_realizations_transaction_settlement_reconciliation ON public.scenario_realizations USING btree (transaction_id) WHERE (relation_type = ANY (ARRAY['settlement'::text, 'reconciliation'::text]));


--
-- Name: idx_scenario_recurring_schedules_account_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_scenario_recurring_schedules_account_id ON public.scenario_recurring_schedules USING btree (account_id);


--
-- Name: idx_scenario_recurring_schedules_category_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_scenario_recurring_schedules_category_id ON public.scenario_recurring_schedules USING btree (category_id);


--
-- Name: idx_scenario_transactions_scenario_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_scenario_transactions_scenario_id ON public.scenario_transactions USING btree (scenario_id);


--
-- Name: idx_scenarios_payable_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_scenarios_payable_id ON public.scenarios USING btree (payable_id);


--
-- Name: ix_account_balance_adjustments_account; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_account_balance_adjustments_account ON public.account_balance_adjustments USING btree (account_id);


--
-- Name: ix_financial_bills_account_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_financial_bills_account_id ON public.financial_bills USING btree (account_id);


--
-- Name: ix_financial_investment_transactions_investment_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_financial_investment_transactions_investment_id ON public.financial_investment_transactions USING btree (investment_id);


--
-- Name: ix_financial_investment_transactions_occurred_at_id_desc; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_financial_investment_transactions_occurred_at_id_desc ON public.financial_investment_transactions USING btree (occurred_at DESC NULLS LAST, id DESC);


--
-- Name: ix_financial_transactions_account_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_financial_transactions_account_id ON public.financial_transactions USING btree (account_id);


--
-- Name: ix_financial_transactions_occurred_at_id_desc; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_financial_transactions_occurred_at_id_desc ON public.financial_transactions USING btree (occurred_at DESC NULLS LAST, id DESC);


--
-- Name: ix_investment_operations_account_date; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_investment_operations_account_date ON public.investment_operations USING btree (account_id, occurred_on, created_at, id);


--
-- Name: ix_investment_operations_position_date; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_investment_operations_position_date ON public.investment_operations USING btree (position_id, occurred_on, created_at, id);


--
-- Name: ix_investment_operations_transfer_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_investment_operations_transfer_id ON public.investment_operations USING btree (transfer_id);


--
-- Name: ix_investment_position_portfolios_portfolio_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_investment_position_portfolios_portfolio_id ON public.investment_position_portfolios USING btree (portfolio_id);


--
-- Name: ix_investment_positions_account_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_investment_positions_account_id ON public.investment_positions USING btree (account_id);


--
-- Name: ix_investment_positions_asset_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_investment_positions_asset_id ON public.investment_positions USING btree (asset_id);


--
-- Name: ix_investment_reconciliations_operation_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_investment_reconciliations_operation_id ON public.investment_reconciliations USING btree (operation_id);


--
-- Name: ix_statement_imports_account_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_statement_imports_account_created ON public.statement_imports USING btree (account_id, created_at DESC);


--
-- Name: ix_transaction_category_decisions_category_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_transaction_category_decisions_category_id ON public.transaction_category_decisions USING btree (category_id);


--
-- Name: uq_automation_rule_actions_one_reconcile_per_scenario; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_automation_rule_actions_one_reconcile_per_scenario ON public.automation_rule_actions USING btree (scenario_id) WHERE ((action_type = 'reconcile'::text) AND (scenario_id IS NOT NULL));


--
-- Name: uq_investment_reconciliation_operation_financial_investment_tra; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_investment_reconciliation_operation_financial_investment_tra ON public.investment_reconciliations USING btree (operation_id, financial_investment_transaction_id) WHERE (financial_investment_transaction_id IS NOT NULL);


--
-- Name: uq_investment_reconciliation_operation_financial_transaction; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_investment_reconciliation_operation_financial_transaction ON public.investment_reconciliations USING btree (operation_id, financial_transaction_id) WHERE (financial_transaction_id IS NOT NULL);


--
-- Name: uq_raw_imports_identity; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_raw_imports_identity ON public.raw_imports USING btree (sync_run_id, scope, COALESCE(external_account_id, ''::text), page_sequence, request_attempt);


--
-- Name: uq_scenario_realizations_one_complete_event_transaction; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_scenario_realizations_one_complete_event_transaction ON public.scenario_realizations USING btree (transaction_id) WHERE ((transaction_id IS NOT NULL) AND (relation_type = ANY (ARRAY['settlement'::text, 'reconciliation'::text])) AND (state = 'linked'::text));


--
-- Name: uq_scenario_realizations_reconciliation_event; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_scenario_realizations_reconciliation_event ON public.scenario_realizations USING btree (scenario_id, occurrence_date) WHERE (relation_type = 'reconciliation'::text);


--
-- Name: uq_scenarios_accounting_source; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_scenarios_accounting_source ON public.scenarios USING btree (payable_id) WHERE (is_accounting_source = 1);


--
-- Name: uq_sync_runs_active_source; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_sync_runs_active_source ON public.sync_runs USING btree (source_id) WHERE (status = 'in_progress'::text);


--
-- Name: normalization_events normalization_events_reject_delete; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER normalization_events_reject_delete BEFORE DELETE ON public.normalization_events FOR EACH ROW EXECUTE FUNCTION public.reject_append_only_write();


--
-- Name: normalization_events normalization_events_reject_update; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER normalization_events_reject_update BEFORE UPDATE ON public.normalization_events FOR EACH ROW EXECUTE FUNCTION public.reject_append_only_write();


--
-- Name: raw_imports raw_imports_reject_delete; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER raw_imports_reject_delete BEFORE DELETE ON public.raw_imports FOR EACH ROW EXECUTE FUNCTION public.reject_append_only_write();


--
-- Name: raw_imports raw_imports_reject_update; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER raw_imports_reject_update BEFORE UPDATE ON public.raw_imports FOR EACH ROW EXECUTE FUNCTION public.reject_append_only_write();


--
-- Name: transaction_category_events transaction_category_events_reject_delete; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER transaction_category_events_reject_delete BEFORE DELETE ON public.transaction_category_events FOR EACH ROW EXECUTE FUNCTION public.reject_append_only_write();


--
-- Name: transaction_category_events transaction_category_events_reject_update; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER transaction_category_events_reject_update BEFORE UPDATE ON public.transaction_category_events FOR EACH ROW EXECUTE FUNCTION public.reject_append_only_write();


--
-- Name: transaction_inclusion_events transaction_inclusion_events_reject_delete; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER transaction_inclusion_events_reject_delete BEFORE DELETE ON public.transaction_inclusion_events FOR EACH ROW EXECUTE FUNCTION public.reject_append_only_write();


--
-- Name: transaction_inclusion_events transaction_inclusion_events_reject_update; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER transaction_inclusion_events_reject_update BEFORE UPDATE ON public.transaction_inclusion_events FOR EACH ROW EXECUTE FUNCTION public.reject_append_only_write();


--
-- Name: account_balance_adjustments account_balance_adjustments_account_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_balance_adjustments
    ADD CONSTRAINT account_balance_adjustments_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.financial_accounts(id) ON DELETE RESTRICT;


--
-- Name: auth_sessions auth_sessions_owner_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.auth_sessions
    ADD CONSTRAINT auth_sessions_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES public.auth_owner(id);


--
-- Name: automation_rule_actions automation_rule_actions_new_category_id_fkey1; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.automation_rule_actions
    ADD CONSTRAINT automation_rule_actions_new_category_id_fkey1 FOREIGN KEY (category_id) REFERENCES public.categories(id) ON DELETE RESTRICT;


--
-- Name: automation_rule_actions automation_rule_actions_new_rule_id_fkey1; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.automation_rule_actions
    ADD CONSTRAINT automation_rule_actions_new_rule_id_fkey1 FOREIGN KEY (rule_id) REFERENCES public.automation_rules(id) ON DELETE CASCADE;


--
-- Name: automation_rule_actions automation_rule_actions_new_scenario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.automation_rule_actions
    ADD CONSTRAINT automation_rule_actions_new_scenario_id_fkey FOREIGN KEY (scenario_id) REFERENCES public.scenarios(id) ON DELETE RESTRICT;


--
-- Name: automation_rule_conditions automation_rule_conditions_new_rule_id_fkey1; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.automation_rule_conditions
    ADD CONSTRAINT automation_rule_conditions_new_rule_id_fkey1 FOREIGN KEY (rule_id) REFERENCES public.automation_rules(id) ON DELETE CASCADE;


--
-- Name: financial_accounts financial_accounts_current_raw_import_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.financial_accounts
    ADD CONSTRAINT financial_accounts_current_raw_import_id_fkey FOREIGN KEY (current_raw_import_id) REFERENCES public.raw_imports(id);


--
-- Name: financial_accounts financial_accounts_source_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.financial_accounts
    ADD CONSTRAINT financial_accounts_source_id_fkey FOREIGN KEY (source_id) REFERENCES public.data_sources(id);


--
-- Name: financial_bills financial_bills_account_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.financial_bills
    ADD CONSTRAINT financial_bills_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.financial_accounts(id);


--
-- Name: financial_bills financial_bills_current_raw_import_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.financial_bills
    ADD CONSTRAINT financial_bills_current_raw_import_id_fkey FOREIGN KEY (current_raw_import_id) REFERENCES public.raw_imports(id);


--
-- Name: financial_bills financial_bills_source_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.financial_bills
    ADD CONSTRAINT financial_bills_source_id_fkey FOREIGN KEY (source_id) REFERENCES public.data_sources(id);


--
-- Name: financial_investment_transactions financial_investment_transactions_current_raw_import_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.financial_investment_transactions
    ADD CONSTRAINT financial_investment_transactions_current_raw_import_id_fkey FOREIGN KEY (current_raw_import_id) REFERENCES public.raw_imports(id);


--
-- Name: financial_investment_transactions financial_investment_transactions_investment_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.financial_investment_transactions
    ADD CONSTRAINT financial_investment_transactions_investment_id_fkey FOREIGN KEY (investment_id) REFERENCES public.financial_investments(id);


--
-- Name: financial_investment_transactions financial_investment_transactions_source_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.financial_investment_transactions
    ADD CONSTRAINT financial_investment_transactions_source_id_fkey FOREIGN KEY (source_id) REFERENCES public.data_sources(id);


--
-- Name: financial_investments financial_investments_current_raw_import_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.financial_investments
    ADD CONSTRAINT financial_investments_current_raw_import_id_fkey FOREIGN KEY (current_raw_import_id) REFERENCES public.raw_imports(id);


--
-- Name: financial_investments financial_investments_source_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.financial_investments
    ADD CONSTRAINT financial_investments_source_id_fkey FOREIGN KEY (source_id) REFERENCES public.data_sources(id);


--
-- Name: financial_transactions financial_transactions_account_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.financial_transactions
    ADD CONSTRAINT financial_transactions_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.financial_accounts(id);


--
-- Name: financial_transactions financial_transactions_current_raw_import_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.financial_transactions
    ADD CONSTRAINT financial_transactions_current_raw_import_id_fkey FOREIGN KEY (current_raw_import_id) REFERENCES public.raw_imports(id);


--
-- Name: financial_transactions financial_transactions_source_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.financial_transactions
    ADD CONSTRAINT financial_transactions_source_id_fkey FOREIGN KEY (source_id) REFERENCES public.data_sources(id);


--
-- Name: investment_accounts investment_accounts_financial_account_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.investment_accounts
    ADD CONSTRAINT investment_accounts_financial_account_id_fkey FOREIGN KEY (financial_account_id) REFERENCES public.financial_accounts(id) ON DELETE SET NULL;


--
-- Name: investment_accounts investment_accounts_source_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.investment_accounts
    ADD CONSTRAINT investment_accounts_source_id_fkey FOREIGN KEY (source_id) REFERENCES public.data_sources(id) ON DELETE CASCADE;


--
-- Name: investment_asset_quote_coverage investment_asset_quote_coverage_asset_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.investment_asset_quote_coverage
    ADD CONSTRAINT investment_asset_quote_coverage_asset_id_fkey FOREIGN KEY (asset_id) REFERENCES public.investment_assets(id) ON DELETE CASCADE;


--
-- Name: investment_asset_quotes investment_asset_quotes_asset_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.investment_asset_quotes
    ADD CONSTRAINT investment_asset_quotes_asset_id_fkey FOREIGN KEY (asset_id) REFERENCES public.investment_assets(id) ON DELETE CASCADE;


--
-- Name: investment_asset_quotes investment_asset_quotes_raw_import_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.investment_asset_quotes
    ADD CONSTRAINT investment_asset_quotes_raw_import_id_fkey FOREIGN KEY (raw_import_id) REFERENCES public.raw_imports(id);


--
-- Name: investment_operations investment_operations_account_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.investment_operations
    ADD CONSTRAINT investment_operations_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.investment_accounts(id) ON DELETE RESTRICT;


--
-- Name: investment_operations investment_operations_position_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.investment_operations
    ADD CONSTRAINT investment_operations_position_id_fkey FOREIGN KEY (position_id) REFERENCES public.investment_positions(id) ON DELETE RESTRICT;


--
-- Name: investment_position_portfolios investment_position_portfolios_financial_investment_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.investment_position_portfolios
    ADD CONSTRAINT investment_position_portfolios_financial_investment_id_fkey FOREIGN KEY (financial_investment_id) REFERENCES public.financial_investments(id) ON DELETE CASCADE;


--
-- Name: investment_position_portfolios investment_position_portfolios_manual_position_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.investment_position_portfolios
    ADD CONSTRAINT investment_position_portfolios_manual_position_id_fkey FOREIGN KEY (manual_position_id) REFERENCES public.investment_positions(id) ON DELETE CASCADE;


--
-- Name: investment_position_portfolios investment_position_portfolios_portfolio_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.investment_position_portfolios
    ADD CONSTRAINT investment_position_portfolios_portfolio_id_fkey FOREIGN KEY (portfolio_id) REFERENCES public.investment_portfolios(id) ON DELETE CASCADE;


--
-- Name: investment_positions investment_positions_account_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.investment_positions
    ADD CONSTRAINT investment_positions_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.investment_accounts(id) ON DELETE RESTRICT;


--
-- Name: investment_positions investment_positions_asset_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.investment_positions
    ADD CONSTRAINT investment_positions_asset_id_fkey FOREIGN KEY (asset_id) REFERENCES public.investment_assets(id) ON DELETE RESTRICT;


--
-- Name: investment_reconciliations investment_reconciliations_financial_investment_transactio_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.investment_reconciliations
    ADD CONSTRAINT investment_reconciliations_financial_investment_transactio_fkey FOREIGN KEY (financial_investment_transaction_id) REFERENCES public.financial_investment_transactions(id) ON DELETE CASCADE;


--
-- Name: investment_reconciliations investment_reconciliations_financial_transaction_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.investment_reconciliations
    ADD CONSTRAINT investment_reconciliations_financial_transaction_id_fkey FOREIGN KEY (financial_transaction_id) REFERENCES public.financial_transactions(id) ON DELETE CASCADE;


--
-- Name: investment_reconciliations investment_reconciliations_operation_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.investment_reconciliations
    ADD CONSTRAINT investment_reconciliations_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES public.investment_operations(id) ON DELETE CASCADE;


--
-- Name: manual_transaction_idempotency manual_transaction_idempotency_transaction_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.manual_transaction_idempotency
    ADD CONSTRAINT manual_transaction_idempotency_transaction_id_fkey FOREIGN KEY (transaction_id) REFERENCES public.financial_transactions(id);


--
-- Name: normalization_events normalization_events_account_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.normalization_events
    ADD CONSTRAINT normalization_events_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.financial_accounts(id);


--
-- Name: normalization_events normalization_events_bill_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.normalization_events
    ADD CONSTRAINT normalization_events_bill_id_fkey FOREIGN KEY (bill_id) REFERENCES public.financial_bills(id);


--
-- Name: normalization_events normalization_events_investment_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.normalization_events
    ADD CONSTRAINT normalization_events_investment_id_fkey FOREIGN KEY (investment_id) REFERENCES public.financial_investments(id);


--
-- Name: normalization_events normalization_events_investment_transaction_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.normalization_events
    ADD CONSTRAINT normalization_events_investment_transaction_id_fkey FOREIGN KEY (investment_transaction_id) REFERENCES public.financial_investment_transactions(id);


--
-- Name: normalization_events normalization_events_raw_import_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.normalization_events
    ADD CONSTRAINT normalization_events_raw_import_id_fkey FOREIGN KEY (raw_import_id) REFERENCES public.raw_imports(id);


--
-- Name: normalization_events normalization_events_sync_run_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.normalization_events
    ADD CONSTRAINT normalization_events_sync_run_id_fkey FOREIGN KEY (sync_run_id) REFERENCES public.sync_runs(id);


--
-- Name: normalization_events normalization_events_transaction_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.normalization_events
    ADD CONSTRAINT normalization_events_transaction_id_fkey FOREIGN KEY (transaction_id) REFERENCES public.financial_transactions(id);


--
-- Name: payable_transaction_links payable_transaction_links_payable_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payable_transaction_links
    ADD CONSTRAINT payable_transaction_links_payable_id_fkey FOREIGN KEY (payable_id) REFERENCES public.payables(id) ON DELETE CASCADE;


--
-- Name: payable_transaction_links payable_transaction_links_transaction_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payable_transaction_links
    ADD CONSTRAINT payable_transaction_links_transaction_id_fkey FOREIGN KEY (transaction_id) REFERENCES public.financial_transactions(id) ON DELETE RESTRICT;


--
-- Name: raw_imports raw_imports_source_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.raw_imports
    ADD CONSTRAINT raw_imports_source_id_fkey FOREIGN KEY (source_id) REFERENCES public.data_sources(id);


--
-- Name: raw_imports raw_imports_sync_run_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.raw_imports
    ADD CONSTRAINT raw_imports_sync_run_id_fkey FOREIGN KEY (sync_run_id) REFERENCES public.sync_runs(id);


--
-- Name: scenario_realizations scenario_realizations_scenario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scenario_realizations
    ADD CONSTRAINT scenario_realizations_scenario_id_fkey FOREIGN KEY (scenario_id) REFERENCES public.scenarios(id) ON DELETE CASCADE;


--
-- Name: scenario_realizations scenario_realizations_scenario_transaction_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scenario_realizations
    ADD CONSTRAINT scenario_realizations_scenario_transaction_id_fkey FOREIGN KEY (scenario_transaction_id) REFERENCES public.scenario_transactions(id) ON DELETE CASCADE;


--
-- Name: scenario_realizations scenario_realizations_transaction_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scenario_realizations
    ADD CONSTRAINT scenario_realizations_transaction_id_fkey FOREIGN KEY (transaction_id) REFERENCES public.financial_transactions(id) ON DELETE CASCADE;


--
-- Name: scenario_recurring_schedules scenario_recurring_schedules_account_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scenario_recurring_schedules
    ADD CONSTRAINT scenario_recurring_schedules_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.financial_accounts(id) ON DELETE RESTRICT;


--
-- Name: scenario_recurring_schedules scenario_recurring_schedules_category_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scenario_recurring_schedules
    ADD CONSTRAINT scenario_recurring_schedules_category_id_fkey FOREIGN KEY (category_id) REFERENCES public.categories(id) ON DELETE RESTRICT;


--
-- Name: scenario_recurring_schedules scenario_recurring_schedules_scenario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scenario_recurring_schedules
    ADD CONSTRAINT scenario_recurring_schedules_scenario_id_fkey FOREIGN KEY (scenario_id) REFERENCES public.scenarios(id) ON DELETE CASCADE;


--
-- Name: scenario_transactions scenario_transactions_scenario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scenario_transactions
    ADD CONSTRAINT scenario_transactions_scenario_id_fkey FOREIGN KEY (scenario_id) REFERENCES public.scenarios(id) ON DELETE CASCADE;


--
-- Name: scenarios scenarios_payable_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scenarios
    ADD CONSTRAINT scenarios_payable_id_fkey FOREIGN KEY (payable_id) REFERENCES public.payables(id) ON DELETE CASCADE;


--
-- Name: statement_imports statement_imports_account_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.statement_imports
    ADD CONSTRAINT statement_imports_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.financial_accounts(id);


--
-- Name: statement_imports statement_imports_source_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.statement_imports
    ADD CONSTRAINT statement_imports_source_id_fkey FOREIGN KEY (source_id) REFERENCES public.data_sources(id);


--
-- Name: statement_imports statement_imports_sync_run_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.statement_imports
    ADD CONSTRAINT statement_imports_sync_run_id_fkey FOREIGN KEY (sync_run_id) REFERENCES public.sync_runs(id);


--
-- Name: sync_failures sync_failures_account_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sync_failures
    ADD CONSTRAINT sync_failures_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.financial_accounts(id);


--
-- Name: sync_failures sync_failures_bill_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sync_failures
    ADD CONSTRAINT sync_failures_bill_id_fkey FOREIGN KEY (bill_id) REFERENCES public.financial_bills(id);


--
-- Name: sync_failures sync_failures_investment_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sync_failures
    ADD CONSTRAINT sync_failures_investment_id_fkey FOREIGN KEY (investment_id) REFERENCES public.financial_investments(id);


--
-- Name: sync_failures sync_failures_raw_import_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sync_failures
    ADD CONSTRAINT sync_failures_raw_import_id_fkey FOREIGN KEY (raw_import_id) REFERENCES public.raw_imports(id);


--
-- Name: sync_failures sync_failures_sync_run_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sync_failures
    ADD CONSTRAINT sync_failures_sync_run_id_fkey FOREIGN KEY (sync_run_id) REFERENCES public.sync_runs(id);


--
-- Name: sync_runs sync_runs_source_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sync_runs
    ADD CONSTRAINT sync_runs_source_id_fkey FOREIGN KEY (source_id) REFERENCES public.data_sources(id);


--
-- Name: transaction_category_decisions transaction_category_decisions_new_category_id_fkey1; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.transaction_category_decisions
    ADD CONSTRAINT transaction_category_decisions_new_category_id_fkey1 FOREIGN KEY (category_id) REFERENCES public.categories(id) ON DELETE RESTRICT;


--
-- Name: transaction_category_decisions transaction_category_decisions_new_transaction_id_fkey1; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.transaction_category_decisions
    ADD CONSTRAINT transaction_category_decisions_new_transaction_id_fkey1 FOREIGN KEY (transaction_id) REFERENCES public.financial_transactions(id) ON DELETE RESTRICT;


--
-- Name: transaction_category_events transaction_category_events_new_transaction_id_fkey1; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.transaction_category_events
    ADD CONSTRAINT transaction_category_events_new_transaction_id_fkey1 FOREIGN KEY (transaction_id) REFERENCES public.financial_transactions(id) ON DELETE RESTRICT;


--
-- Name: transaction_inclusion_decisions transaction_inclusion_decisions_rule_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.transaction_inclusion_decisions
    ADD CONSTRAINT transaction_inclusion_decisions_rule_id_fkey FOREIGN KEY (rule_id) REFERENCES public.automation_rules(id) ON DELETE SET NULL;


--
-- Name: transaction_inclusion_decisions transaction_inclusion_decisions_transaction_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.transaction_inclusion_decisions
    ADD CONSTRAINT transaction_inclusion_decisions_transaction_id_fkey FOREIGN KEY (transaction_id) REFERENCES public.financial_transactions(id) ON DELETE RESTRICT;


--
-- Name: transaction_inclusion_events transaction_inclusion_events_transaction_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.transaction_inclusion_events
    ADD CONSTRAINT transaction_inclusion_events_transaction_id_fkey FOREIGN KEY (transaction_id) REFERENCES public.financial_transactions(id) ON DELETE RESTRICT;


--
-- PostgreSQL database dump complete
--


