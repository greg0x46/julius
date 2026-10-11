package timeline

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/exchange"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/reporting"
	"github.com/shopspring/decimal"
)

func reportingCashBalance(ctx context.Context, q Querier, accountIDs []string, reference time.Time) (decimal.Decimal, reporting.Summary, error) {
	query := `SELECT balance, currency_code FROM financial_accounts WHERE balance IS NOT NULL AND (account_type IS NULL OR account_type != 'CREDIT')`
	in, args := db.InClause(accountIDs)
	if in != "" {
		query += ` AND id IN (` + in + `)`
	}
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return decimal.Zero, reporting.Summary{}, err
	}
	type accountMoney struct {
		amount   decimal.Decimal
		currency string
	}
	var accounts []accountMoney
	for rows.Next() {
		var value string
		var currency sql.NullString
		if err := rows.Scan(&value, &currency); err != nil {
			rows.Close()
			return decimal.Zero, reporting.Summary{}, err
		}
		amount, err := decimal.NewFromString(value)
		if err != nil {
			rows.Close()
			return decimal.Zero, reporting.Summary{}, fmt.Errorf("parse account balance %q: %w", value, err)
		}
		if !amount.IsZero() {
			accounts = append(accounts, accountMoney{amount, currency.String})
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return decimal.Zero, reporting.Summary{}, err
	}
	rates := exchange.New(q, nil)
	var coverage reporting.Accumulator
	balance := decimal.Zero
	for _, account := range accounts {
		converted := reporting.Convert(ctx, rates, account.amount, account.currency, &reference)
		direction := money.Inflow
		if account.amount.IsNegative() {
			direction = money.Outflow
		}
		coverage.Add(converted, direction)
		if converted.Amount != nil {
			balance = balance.Add(*converted.Amount)
		}
	}
	summary := coverage.Summary()
	if summary.Missing > 0 {
		return decimal.Zero, summary, &reporting.IncompleteError{Coverage: summary, Totals: []reporting.Totals{}, Detail: "Faltam cotações históricas para converter o saldo das contas."}
	}
	return balance, summary, nil
}
