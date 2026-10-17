package timeline

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/greg0x46/julius/internal/db"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/greg0x46/julius/internal/exchange"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/reporting"
	"github.com/greg0x46/julius/internal/transactions"
	"github.com/shopspring/decimal"
)

func reportingCashBalance(ctx context.Context, q Querier, accountIDs []string, reference time.Time) (decimal.Decimal, reporting.Summary, error) {
	query := `SELECT id, name, currency_code FROM financial_accounts WHERE (balance IS NOT NULL OR origin = 'manual') AND (account_type IS NULL OR account_type != 'CREDIT')`
	in, args := db.InClause(accountIDs)
	if in != "" {
		query += ` AND id IN (` + in + `)`
	}
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return decimal.Zero, reporting.Summary{}, err
	}
	type accountMoney struct {
		id       string
		name     string
		amount   decimal.Decimal
		currency string
	}
	var accounts []accountMoney
	for rows.Next() {
		var id string
		var name sql.NullString
		var currency sql.NullString
		if err := rows.Scan(dbtypes.ID(&id), &name, &currency); err != nil {
			rows.Close()
			return decimal.Zero, reporting.Summary{}, err
		}
		if !currency.Valid {
			currency.String = reporting.ReferenceCurrency
		}
		accounts = append(accounts, accountMoney{id: id, name: name.String, currency: currency.String})
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
		raw, err := transactions.AccountBalance(ctx, q, account.id)
		if err != nil {
			return decimal.Zero, reporting.Summary{}, err
		}
		if raw == nil {
			continue
		}
		account.amount, err = decimal.NewFromString(*raw)
		if err != nil {
			return decimal.Zero, reporting.Summary{}, fmt.Errorf("parse account balance: %w", err)
		}
		if account.amount.IsZero() {
			continue
		}
		converted := reporting.Convert(ctx, rates, account.amount, account.currency, &reference)
		if converted.Issue != nil {
			converted.Issue.Account = account.name
			if converted.Issue.Account == "" {
				converted.Issue.Account = account.id
			}
		}
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
