package transactions

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/greg0x46/julius/internal/db"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/greg0x46/julius/internal/money"
	"github.com/shopspring/decimal"
)

var ErrAccountNotManual = errors.New("account is not manual")
var ErrAccountHasMovements = errors.New("account currency and opening balance cannot change after movements")
var ErrAccountNotActiveManual = errors.New("adjustments require an active manual account")

// ManualAccountInput contains validated account fields. The caller owns the transaction.
type ManualAccountInput struct {
	Name           string
	CurrencyCode   string
	OpeningBalance decimal.Decimal
	Archived       bool
}

func WriteManualAccount(ctx context.Context, q Querier, id string, in ManualAccountInput, create bool) (string, error) {
	var err error

	now := db.FormatTime(time.Now())
	openingRaw := money.CanonicalDecimal(in.OpeningBalance)
	var archived any
	if in.Archived {
		archived = now
	}
	if create {
		id = uuid.NewString()
		_, err = q.ExecContext(ctx, `INSERT INTO financial_accounts
   (id, origin, name, account_type, currency_code, opening_balance, archived_at, created_at, updated_at)
   VALUES (?, 'manual', ?, 'BANK', ?, ?, ?, ?, ?)`, id, in.Name, in.CurrencyCode, openingRaw, archived, now, now)
	} else {
		// Take the same row lock as manual transaction creation before checking currency.
		result, lockErr := q.ExecContext(ctx, `UPDATE financial_accounts SET updated_at = updated_at WHERE id = ?`, id)
		if lockErr != nil {
			return "", lockErr
		}
		n, lockErr := result.RowsAffected()
		if lockErr != nil {
			return "", lockErr
		}
		if n == 0 {
			return "", ErrAccountNotFound
		}
		var origin, oldCurrency string
		var oldOpening decimal.Decimal
		var hasMovements bool
		err = q.QueryRowContext(ctx, `SELECT origin, COALESCE(currency_code, ''), COALESCE(opening_balance, '0'),
    EXISTS(SELECT 1 FROM financial_transactions WHERE account_id = ?) OR
    EXISTS(SELECT 1 FROM account_balance_adjustments WHERE account_id = ?)
    FROM financial_accounts WHERE id = ?`, id, id, id).Scan(&origin, &oldCurrency, dbtypes.Decimal(&oldOpening), dbtypes.Bool(&hasMovements))
		if err != nil {
			return "", err
		}
		if origin != "manual" {
			return "", ErrAccountNotManual
		}
		if hasMovements && (oldCurrency != in.CurrencyCode || !oldOpening.Equal(in.OpeningBalance)) {
			return "", ErrAccountHasMovements
		}
		_, err = q.ExecContext(ctx, `UPDATE financial_accounts SET name = ?, currency_code = ?, opening_balance = ?, archived_at = ?, updated_at = ? WHERE id = ?`, in.Name, in.CurrencyCode, openingRaw, archived, now, id)
	}
	if err != nil {
		return "", err
	}
	return id, nil
}

func AdjustManualAccount(ctx context.Context, q Querier, id string, amount decimal.Decimal, description string) error {
	result, err := q.ExecContext(ctx, `UPDATE financial_accounts SET updated_at = ? WHERE id = ? AND origin = 'manual' AND archived_at IS NULL`, db.FormatTime(time.Now()), id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		var exists bool
		err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM financial_accounts WHERE id = ?)`, id).Scan(dbtypes.Bool(&exists))
		if err != nil {
			return err
		}
		if !exists {
			return ErrAccountNotFound
		}
		return ErrAccountNotActiveManual
	}
	_, err = q.ExecContext(ctx, `INSERT INTO account_balance_adjustments (id, account_id, amount, description, created_at) VALUES (?, ?, ?, ?, ?)`, uuid.NewString(), id, money.CanonicalDecimal(amount), description, db.FormatTime(time.Now()))
	if err != nil {
		return err
	}
	return nil
}
