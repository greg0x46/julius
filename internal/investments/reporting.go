package investments

import (
	"context"
	"database/sql"
	"fmt"
	"sort"

	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/shopspring/decimal"
)

// ReconciledTransactionAmounts returns only the portions of bank
// transactions that represent a transfer into or out of an investment
// custody account. Fees, taxes and income deliberately stay in the ordinary
// financial report; hiding those would erase real income/cost rather than
// prevent a duplicate transfer.
func ReconciledTransactionAmounts(ctx context.Context, q Querier) (map[string]decimal.Decimal, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT r.financial_transaction_id, r.amount
		FROM investment_reconciliations r
		JOIN investment_operations o ON o.id = r.operation_id
		WHERE r.financial_transaction_id IS NOT NULL
		  AND o.kind IN ('deposit', 'withdrawal')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]decimal.Decimal{}
	for rows.Next() {
		var id, raw string
		if err := rows.Scan(dbtypes.ID(&id), dbtypes.Decimal(&raw)); err != nil {
			return nil, err
		}
		amount, err := decimal.NewFromString(raw)
		if err != nil {
			return nil, fmt.Errorf("parse reconciliation amount: %w", err)
		}
		result[id] = result[id].Add(amount.Abs())
	}
	return result, rows.Err()
}

// ReconciledTransactionAmount is the single-line counterpart of
// ReconciledTransactionAmounts, for callers that already know which bank
// transaction they are looking at and must not pay for the whole table.
func ReconciledTransactionAmount(ctx context.Context, q Querier, financialTransactionID string) (decimal.Decimal, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT r.amount
		FROM investment_reconciliations r
		JOIN investment_operations o ON o.id = r.operation_id
		WHERE r.financial_transaction_id = ?
		  AND o.kind IN ('deposit', 'withdrawal')`, financialTransactionID)
	if err != nil {
		return decimal.Zero, err
	}
	defer rows.Close()
	total := decimal.Zero
	for rows.Next() {
		var raw string
		if err := rows.Scan(dbtypes.Decimal(&raw)); err != nil {
			return decimal.Zero, err
		}
		amount, err := decimal.NewFromString(raw)
		if err != nil {
			return decimal.Zero, fmt.Errorf("parse reconciliation amount: %w", err)
		}
		total = total.Add(amount.Abs())
	}
	return total, rows.Err()
}

// ManualNetWorth is the manual investment asset contribution: manual
// holdings at their current value (the latest quote of an asset priced from a
// market, or the latest manual valuation, or their replayed cost basis) plus
// book cash only when that cash is not already represented by a linked
// provider account. It intentionally does not include provider holdings.
func ManualNetWorth(ctx context.Context, q Querier) (decimal.Decimal, error) {
	accounts, err := ListAccounts(ctx, q)
	if err != nil {
		return decimal.Zero, err
	}
	positions, err := ListPositions(ctx, q, PositionFilter{})
	if err != nil {
		return decimal.Zero, err
	}
	total := decimal.Zero
	for _, position := range positions {
		if position.Source == PositionSourceManual {
			total = total.Add(position.CurrentValue)
		}
	}
	for _, account := range accounts {
		if account.Kind == AccountKindManual && account.FinancialAccountID == nil {
			total = total.Add(account.CashBalance)
		}
	}
	return total, nil
}

// ManualReportingEntries exposes dated manual investment flows without
// pretending they move the application's bank cash curve. Direct operations
// only emit their still-unlinked portion; a partial reconciliation therefore
// leaves an equally partial reporting entry. Buy/sell operations emit their
// fee and tax components as separate dated costs because their principal is
// an exchange of cash for an asset, not spending.
func ManualReportingEntries(ctx context.Context, q Querier) ([]ManualReportingEntry, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT o.id, o.account_id, o.kind, o.occurred_on, o.amount, o.fees, o.taxes,
		       r.financial_transaction_id, r.amount
		FROM investment_operations o
		LEFT JOIN investment_reconciliations r ON r.operation_id = o.id
		WHERE o.source = 'manual'
		ORDER BY o.occurred_on, o.id, r.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type reportOperation struct {
		id, accountID, kindRaw, occurredRaw, amountRaw, feesRaw, taxesRaw string
	}
	operations := []reportOperation{}
	seen := map[string]bool{}
	linkedByOperation := map[string]decimal.Decimal{}
	for rows.Next() {
		var op reportOperation
		var financialTransactionID, linkedRaw sql.NullString
		if err := rows.Scan(dbtypes.ID(&op.id), dbtypes.ID(&op.accountID), &op.kindRaw, dbtypes.Date(&op.occurredRaw), dbtypes.Decimal(&op.amountRaw), dbtypes.Decimal(&op.feesRaw), dbtypes.Decimal(&op.taxesRaw),
			dbtypes.ID(&financialTransactionID), dbtypes.Decimal(&linkedRaw)); err != nil {
			return nil, err
		}
		if !seen[op.id] {
			operations = append(operations, op)
			seen[op.id] = true
		}
		if financialTransactionID.Valid && linkedRaw.Valid {
			linked, err := decimal.NewFromString(linkedRaw.String)
			if err != nil {
				return nil, err
			}
			linkedByOperation[op.id] = linkedByOperation[op.id].Add(linked)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	entries := []ManualReportingEntry{}
	for _, raw := range operations {
		occurredOn, err := parseDate(raw.occurredRaw)
		if err != nil {
			return nil, err
		}
		parse := func(raw string) (decimal.Decimal, error) { return decimal.NewFromString(raw) }
		amount, err := parse(raw.amountRaw)
		if err != nil {
			return nil, err
		}
		fees, err := parse(raw.feesRaw)
		if err != nil {
			return nil, err
		}
		taxes, err := parse(raw.taxesRaw)
		if err != nil {
			return nil, err
		}
		linked := linkedByOperation[raw.id]
		kind := OperationKind(raw.kindRaw)
		add := func(entryID string, entryKind OperationKind, signed decimal.Decimal) {
			if !signed.IsZero() {
				entries = append(entries, ManualReportingEntry{ID: entryID, AccountID: raw.accountID, Kind: entryKind, OccurredOn: occurredOn, Amount: signed})
			}
		}
		switch kind {
		case OperationDeposit, OperationIncome:
			remaining := amount.Sub(linked)
			if remaining.IsPositive() {
				add(raw.id, kind, remaining)
			}
		case OperationWithdrawal, OperationFee, OperationTax:
			remaining := amount.Sub(linked)
			if remaining.IsPositive() {
				add(raw.id, kind, remaining.Neg())
			}
		case OperationBuy, OperationSell:
			// A trade itself changes asset composition. Its explicit costs are
			// reportable, independent of the trade principal.
			add(raw.id+":fee", OperationFee, fees.Neg())
			add(raw.id+":tax", OperationTax, taxes.Neg())
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if !entries[i].OccurredOn.Equal(entries[j].OccurredOn) {
			return entries[i].OccurredOn.Before(entries[j].OccurredOn)
		}
		return entries[i].ID < entries[j].ID
	})
	return entries, nil
}

func ManualOperationReporting(ctx context.Context, q Querier) (ManualReporting, error) {
	entries, err := ManualReportingEntries(ctx, q)
	if err != nil {
		return ManualReporting{}, err
	}
	var result ManualReporting
	for _, entry := range entries {
		switch entry.Kind {
		case OperationIncome:
			result.Income = result.Income.Add(entry.Amount)
		case OperationFee:
			result.Fees = result.Fees.Add(entry.Amount.Abs())
		case OperationTax:
			result.Taxes = result.Taxes.Add(entry.Amount.Abs())
		}
	}
	return result, nil
}
