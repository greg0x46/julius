package transactions

import (
	"context"
	"database/sql"

	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/greg0x46/julius/internal/money"
	"github.com/shopspring/decimal"
)

// AccountBalance preserves provider balances; manual balances are opening
// balance + posted, undeleted movements + explicit adjustment deltas.
func AccountBalance(ctx context.Context, q Querier, id string) (*string, error) {
	var origin string
	var balance, opening sql.NullString
	if err := q.QueryRowContext(ctx, `SELECT origin, balance, opening_balance FROM financial_accounts WHERE id = ?`, id).Scan(&origin, dbtypes.Decimal(&balance), dbtypes.Decimal(&opening)); err != nil {
		return nil, err
	}
	if origin != "manual" {
		if !balance.Valid {
			return nil, nil
		}
		return &balance.String, nil
	}
	total, err := decimal.NewFromString(opening.String)
	if err != nil {
		return nil, err
	}
	rows, err := q.QueryContext(ctx, `SELECT amount_in_account_currency FROM financial_transactions
 WHERE account_id = ? AND deleted_at IS NULL AND provider_status = 'POSTED'
 UNION ALL SELECT amount FROM account_balance_adjustments WHERE account_id = ?`, id, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(dbtypes.Decimal(&raw)); err != nil {
			return nil, err
		}
		amount, err := decimal.NewFromString(raw)
		if err != nil {
			return nil, err
		}
		total = total.Add(amount)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	value := money.CanonicalDecimal(total)
	return &value, nil
}

// LockAccount serializes account changes with manual transaction writes.
func LockAccount(ctx context.Context, q Querier, id string) error {
	result, err := q.ExecContext(ctx, `UPDATE financial_accounts SET updated_at = updated_at WHERE id = ? AND archived_at IS NULL`, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		var exists bool
		if err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM financial_accounts WHERE id = ?)`, id).Scan(dbtypes.Bool(&exists)); err != nil {
			return err
		}
		if exists {
			return ErrAccountArchived
		}
		return ErrAccountNotFound
	}
	return nil
}
