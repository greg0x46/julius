package main

import (
	"fmt"
	"time"

	"github.com/greg0x46/julius/internal/db/types"
	"github.com/greg0x46/julius/internal/investments"
)

// Logical types from the native baseline; opaque IDs and payloads copy unchanged.
var columnKinds = map[string]string{
	"transfer_links.id":                                              "uuid",
	"transfer_links.created_at":                                      "timestamptz",
	"transfer_transaction_links.transaction_id":                      "uuid",
	"transfer_transaction_links.transfer_id":                         "uuid",
	"account_balance_adjustments.account_id":                         "uuid",
	"account_balance_adjustments.amount":                             "numeric",
	"account_balance_adjustments.created_at":                         "timestamptz",
	"account_balance_adjustments.id":                                 "uuid",
	"app_auth.created_at":                                            "timestamptz",
	"app_auth.updated_at":                                            "timestamptz",
	"auth_owner.created_at":                                          "timestamptz",
	"auth_sessions.created_at":                                       "timestamptz",
	"auth_sessions.expires_at":                                       "timestamptz",
	"auth_sessions.last_seen_at":                                     "timestamptz",
	"automation_rule_actions.category_id":                            "uuid",
	"automation_rule_actions.id":                                     "uuid",
	"automation_rule_actions.rule_id":                                "uuid",
	"automation_rule_actions.scenario_id":                            "uuid",
	"automation_rule_conditions.id":                                  "uuid",
	"automation_rule_conditions.rule_id":                             "uuid",
	"automation_rules.created_at":                                    "timestamptz",
	"automation_rules.id":                                            "uuid",
	"automation_rules.is_active":                                     "boolean",
	"automation_rules.updated_at":                                    "timestamptz",
	"categories.created_at":                                          "timestamptz",
	"categories.id":                                                  "uuid",
	"categories.is_active":                                           "boolean",
	"categories.updated_at":                                          "timestamptz",
	"data_sources.created_at":                                        "timestamptz",
	"data_sources.id":                                                "uuid",
	"data_sources.is_active":                                         "boolean",
	"data_sources.updated_at":                                        "timestamptz",
	"exchange_rates.as_of":                                           "date",
	"exchange_rates.effective_on":                                    "date",
	"exchange_rates.fetched_at":                                      "timestamptz",
	"exchange_rates.rate":                                            "numeric",
	"financial_accounts.archived_at":                                 "timestamptz",
	"financial_accounts.available_credit_limit":                      "numeric",
	"financial_accounts.balance":                                     "numeric",
	"financial_accounts.balance_as_of":                               "timestamptz",
	"financial_accounts.balance_close_date":                          "date",
	"financial_accounts.balance_due_date":                            "date",
	"financial_accounts.created_at":                                  "timestamptz",
	"financial_accounts.credit_limit":                                "numeric",
	"financial_accounts.current_raw_import_id":                       "uuid",
	"financial_accounts.id":                                          "uuid",
	"financial_accounts.opening_balance":                             "numeric",
	"financial_accounts.provider_updated_at":                         "timestamptz",
	"financial_accounts.source_id":                                   "uuid",
	"financial_accounts.updated_at":                                  "timestamptz",
	"financial_bills.account_id":                                     "uuid",
	"financial_bills.closing_date":                                   "date",
	"financial_bills.created_at":                                     "timestamptz",
	"financial_bills.current_raw_import_id":                          "uuid",
	"financial_bills.due_date":                                       "date",
	"financial_bills.id":                                             "uuid",
	"financial_bills.minimum_payment_amount":                         "numeric",
	"financial_bills.source_id":                                      "uuid",
	"financial_bills.total_amount":                                   "numeric",
	"financial_bills.updated_at":                                     "timestamptz",
	"financial_investment_transactions.amount":                       "numeric",
	"financial_investment_transactions.created_at":                   "timestamptz",
	"financial_investment_transactions.current_raw_import_id":        "uuid",
	"financial_investment_transactions.id":                           "uuid",
	"financial_investment_transactions.investment_id":                "uuid",
	"financial_investment_transactions.occurred_at":                  "timestamptz",
	"financial_investment_transactions.quantity":                     "numeric",
	"financial_investment_transactions.source_id":                    "uuid",
	"financial_investment_transactions.trade_date":                   "date",
	"financial_investment_transactions.updated_at":                   "timestamptz",
	"financial_investment_transactions.value":                        "numeric",
	"financial_investments.amount":                                   "numeric",
	"financial_investments.amount_original":                          "numeric",
	"financial_investments.amount_profit":                            "numeric",
	"financial_investments.amount_withdrawal":                        "numeric",
	"financial_investments.annual_rate":                              "numeric",
	"financial_investments.as_of_date":                               "timestamptz",
	"financial_investments.balance":                                  "numeric",
	"financial_investments.created_at":                               "timestamptz",
	"financial_investments.current_raw_import_id":                    "uuid",
	"financial_investments.due_date":                                 "date",
	"financial_investments.fixed_annual_rate":                        "numeric",
	"financial_investments.id":                                       "uuid",
	"financial_investments.issue_date":                               "date",
	"financial_investments.last_twelve_months_rate":                  "numeric",
	"financial_investments.provider_updated_at":                      "timestamptz",
	"financial_investments.purchase_date":                            "date",
	"financial_investments.quantity":                                 "numeric",
	"financial_investments.rate":                                     "numeric",
	"financial_investments.source_id":                                "uuid",
	"financial_investments.taxes":                                    "numeric",
	"financial_investments.taxes2":                                   "numeric",
	"financial_investments.updated_at":                               "timestamptz",
	"financial_investments.value":                                    "numeric",
	"financial_transactions.account_id":                              "uuid",
	"financial_transactions.amount":                                  "numeric",
	"financial_transactions.amount_in_account_currency":              "numeric",
	"financial_transactions.balance_after":                           "numeric",
	"financial_transactions.created_at":                              "timestamptz",
	"financial_transactions.current_raw_import_id":                   "uuid",
	"financial_transactions.deleted_at":                              "timestamptz",
	"financial_transactions.id":                                      "uuid",
	"financial_transactions.occurred_at":                             "timestamptz",
	"financial_transactions.provider_created_at":                     "timestamptz",
	"financial_transactions.provider_updated_at":                     "timestamptz",
	"financial_transactions.source_id":                               "uuid",
	"financial_transactions.updated_at":                              "timestamptz",
	"investment_accounts.created_at":                                 "timestamptz",
	"investment_accounts.financial_account_id":                       "uuid",
	"investment_accounts.is_active":                                  "boolean",
	"investment_accounts.source_id":                                  "uuid",
	"investment_accounts.updated_at":                                 "timestamptz",
	"investment_asset_quote_coverage.asset_id":                       "uuid",
	"investment_asset_quote_coverage.covered_from":                   "date",
	"investment_asset_quote_coverage.covered_to":                     "date",
	"investment_asset_quote_coverage.updated_at":                     "timestamptz",
	"investment_asset_quotes.asset_id":                               "uuid",
	"investment_asset_quotes.created_at":                             "timestamptz",
	"investment_asset_quotes.price":                                  "numeric",
	"investment_asset_quotes.quoted_on":                              "date",
	"investment_asset_quotes.raw_import_id":                          "uuid",
	"investment_asset_quotes.updated_at":                             "timestamptz",
	"investment_assets.created_at":                                   "timestamptz",
	"investment_assets.id":                                           "uuid",
	"investment_assets.updated_at":                                   "timestamptz",
	"investment_operations.amount":                                   "numeric",
	"investment_operations.created_at":                               "timestamptz",
	"investment_operations.fees":                                     "numeric",
	"investment_operations.id":                                       "uuid",
	"investment_operations.occurred_on":                              "date",
	"investment_operations.position_id":                              "uuid",
	"investment_operations.quantity":                                 "numeric",
	"investment_operations.taxes":                                    "numeric",
	"investment_operations.transfer_id":                              "uuid",
	"investment_operations.unit_price":                               "numeric",
	"investment_operations.updated_at":                               "timestamptz",
	"investment_portfolios.created_at":                               "timestamptz",
	"investment_portfolios.id":                                       "uuid",
	"investment_portfolios.target_amount":                            "numeric",
	"investment_portfolios.target_date":                              "date",
	"investment_portfolios.updated_at":                               "timestamptz",
	"investment_position_portfolios.created_at":                      "timestamptz",
	"investment_position_portfolios.financial_investment_id":         "uuid",
	"investment_position_portfolios.manual_position_id":              "uuid",
	"investment_position_portfolios.portfolio_id":                    "uuid",
	"investment_position_portfolios.updated_at":                      "timestamptz",
	"investment_positions.asset_id":                                  "uuid",
	"investment_positions.created_at":                                "timestamptz",
	"investment_positions.id":                                        "uuid",
	"investment_positions.updated_at":                                "timestamptz",
	"investment_reconciliations.amount":                              "numeric",
	"investment_reconciliations.created_at":                          "timestamptz",
	"investment_reconciliations.financial_investment_transaction_id": "uuid",
	"investment_reconciliations.financial_transaction_id":            "uuid",
	"investment_reconciliations.id":                                  "uuid",
	"investment_reconciliations.operation_id":                        "uuid",
	"manual_transaction_idempotency.created_at":                      "timestamptz",
	"manual_transaction_idempotency.transaction_id":                  "uuid",
	"net_worth_snapshots.captured_at":                                "date",
	"net_worth_snapshots.cash_balance":                               "numeric",
	"net_worth_snapshots.created_at":                                 "timestamptz",
	"net_worth_snapshots.credit_card_balance":                        "numeric",
	"net_worth_snapshots.id":                                         "uuid",
	"net_worth_snapshots.investment_balance":                         "numeric",
	"net_worth_snapshots.is_backfilled":                              "boolean",
	"net_worth_snapshots.net_worth":                                  "numeric",
	"net_worth_snapshots.payables_debt":                              "numeric",
	"net_worth_snapshots.total_assets":                               "numeric",
	"net_worth_snapshots.total_liabilities":                          "numeric",
	"net_worth_snapshots.updated_at":                                 "timestamptz",
	"net_worth_snapshots.valued_at":                                  "timestamptz",
	"normalization_events.account_id":                                "uuid",
	"normalization_events.bill_id":                                   "uuid",
	"normalization_events.created_at":                                "timestamptz",
	"normalization_events.id":                                        "uuid",
	"normalization_events.investment_id":                             "uuid",
	"normalization_events.investment_transaction_id":                 "uuid",
	"normalization_events.raw_import_id":                             "uuid",
	"normalization_events.sync_run_id":                               "uuid",
	"normalization_events.transaction_id":                            "uuid",
	"payable_transaction_links.id":                                   "uuid",
	"payable_transaction_links.linked_amount":                        "numeric",
	"payable_transaction_links.linked_at":                            "timestamptz",
	"payable_transaction_links.payable_id":                           "uuid",
	"payable_transaction_links.transaction_id":                       "uuid",
	"payables.created_at":                                            "timestamptz",
	"payables.id":                                                    "uuid",
	"payables.starting_settled_amount":                               "numeric",
	"payables.total_amount":                                          "numeric",
	"payables.updated_at":                                            "timestamptz",
	"raw_imports.id":                                                 "uuid",
	"raw_imports.received_at":                                        "timestamptz",
	"raw_imports.source_id":                                          "uuid",
	"raw_imports.sync_run_id":                                        "uuid",
	"scenario_realizations.allocated_amount":                         "numeric",
	"scenario_realizations.created_at":                               "timestamptz",
	"scenario_realizations.id":                                       "uuid",
	"scenario_realizations.linked_amount":                            "numeric",
	"scenario_realizations.occurrence_date":                          "date",
	"scenario_realizations.scenario_id":                              "uuid",
	"scenario_realizations.scenario_transaction_id":                  "uuid",
	"scenario_realizations.transaction_id":                           "uuid",
	"scenario_recurring_schedules.account_id":                        "uuid",
	"scenario_recurring_schedules.amount":                            "numeric",
	"scenario_recurring_schedules.category_id":                       "uuid",
	"scenario_recurring_schedules.end_date":                          "date",
	"scenario_recurring_schedules.scenario_id":                       "uuid",
	"scenario_recurring_schedules.start_date":                        "date",
	"scenario_transactions.amount":                                   "numeric",
	"scenario_transactions.created_at":                               "timestamptz",
	"scenario_transactions.id":                                       "uuid",
	"scenario_transactions.projected_at":                             "date",
	"scenario_transactions.scenario_id":                              "uuid",
	"scenario_transactions.updated_at":                               "timestamptz",
	"scenarios.created_at":                                           "timestamptz",
	"scenarios.id":                                                   "uuid",
	"scenarios.is_accounting_source":                                 "boolean",
	"scenarios.is_active":                                            "boolean",
	"scenarios.payable_id":                                           "uuid",
	"scenarios.updated_at":                                           "timestamptz",
	"settings.is_encrypted":                                          "boolean",
	"settings.updated_at":                                            "timestamptz",
	"statement_imports.account_id":                                   "uuid",
	"statement_imports.created_at":                                   "timestamptz",
	"statement_imports.id":                                           "uuid",
	"statement_imports.source_id":                                    "uuid",
	"statement_imports.sync_run_id":                                  "uuid",
	"sync_failures.account_id":                                       "uuid",
	"sync_failures.bill_id":                                          "uuid",
	"sync_failures.created_at":                                       "timestamptz",
	"sync_failures.id":                                               "uuid",
	"sync_failures.investment_id":                                    "uuid",
	"sync_failures.raw_import_id":                                    "uuid",
	"sync_failures.sync_run_id":                                      "uuid",
	"sync_runs.finished_at":                                          "timestamptz",
	"sync_runs.heartbeat_at":                                         "timestamptz",
	"sync_runs.id":                                                   "uuid",
	"sync_runs.source_id":                                            "uuid",
	"sync_runs.started_at":                                           "timestamptz",
	"transaction_category_decisions.category_id":                     "uuid",
	"transaction_category_decisions.changed_at":                      "timestamptz",
	"transaction_category_decisions.transaction_id":                  "uuid",
	"transaction_category_events.changed_at":                         "timestamptz",
	"transaction_category_events.id":                                 "uuid",
	"transaction_category_events.transaction_id":                     "uuid",
	"transaction_inclusion_decisions.changed_at":                     "timestamptz",
	"transaction_inclusion_decisions.rule_id":                        "uuid",
	"transaction_inclusion_decisions.transaction_id":                 "uuid",
	"transaction_inclusion_events.changed_at":                        "timestamptz",
	"transaction_inclusion_events.id":                                "uuid",
	"transaction_inclusion_events.transaction_id":                    "uuid",
}

var legacyDates = map[string]string{
	"financial_accounts.balance_close_date":        "utc",
	"financial_accounts.balance_due_date":          "utc",
	"financial_bills.due_date":                     "utc",
	"financial_bills.closing_date":                 "utc",
	"financial_investments.due_date":               "provider",
	"financial_investments.issue_date":             "provider",
	"financial_investments.purchase_date":          "provider",
	"financial_investment_transactions.trade_date": "movement",
}

func convert(key string, value any, postgres bool) (any, error) {
	if value == nil {
		return nil, nil
	}
	if key == "auth_owner.created_at" || key == "auth_sessions.created_at" || key == "auth_sessions.last_seen_at" || key == "auth_sessions.expires_at" {
		seconds, ok := value.(int64)
		if !ok {
			return nil, fmt.Errorf("expected integer Unix seconds")
		}
		stamp := time.Unix(seconds, 0).UTC()
		if stamp.Year() < 1 || stamp.Year() > 9999 {
			return nil, fmt.Errorf("timestamp out of range")
		}
		return stamp.Format(types.TimeLayout), nil
	}
	if dayKind, ok := legacyDates[key]; ok {
		stamp, err := types.ParseTime(value)
		if err != nil {
			return nil, err
		}
		switch dayKind {
		case "provider":
			stamp = investments.ProviderDay(stamp)
		case "movement":
			stamp = investments.MovementDay(stamp)
		}
		return stamp.Format(types.DateLayout), nil
	}
	switch columnKinds[key] {
	case "numeric":
		d, err := types.ParseDecimal(value)
		if err != nil {
			return nil, err
		}
		return d.String(), nil
	case "timestamptz":
		stamp, err := types.ParseTime(value)
		if err != nil {
			return nil, err
		}
		return stamp.Format(types.TimeLayout), nil
	case "date":
		day, err := types.ParseDate(value)
		if err != nil {
			return nil, err
		}
		return day.Format(types.DateLayout), nil
	case "boolean":
		n, ok := value.(int64)
		if !ok || (n != 0 && n != 1) {
			return nil, fmt.Errorf("expected integer flag 0 or 1")
		}
		if postgres {
			return n == 1, nil
		}
		return n, nil
	case "uuid":
		if !postgres {
			return value, nil
		}
		id, err := types.ParseUUID(value)
		if err != nil {
			return nil, err
		}
		return id.String(), nil
	}
	return value, nil
}
