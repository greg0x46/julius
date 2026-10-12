package transactions

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/money"
)

// ErrAccountNotFound also covers archived accounts, which reject new movements.
var ErrAccountNotFound = errors.New("account not found")
var ErrAccountArchived = errors.New("account archived")

// ErrNotManual is returned by UpdateManual/DeleteManual when transactionID
// names a row whose origin is "synced" — editing or deleting a lançamento
// that came from a provider sync is not something this app lets the user do
// by hand; the provider is the source of truth for that row.
var ErrNotManual = errors.New("transaction is not manual")

var ErrInvestmentLinked = errors.New("transaction has investment reconciliations")

func requireNoInvestmentLinks(ctx context.Context, q Querier, id string) error {
	var linked bool
	if err := q.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM investment_reconciliations WHERE financial_transaction_id = ?)`, id).Scan(&linked); err != nil {
		return err
	}
	if linked {
		return ErrInvestmentLinked
	}
	return nil
}

// ManualInput is what the user supplies to author a lançamento by hand.
// Amount is already signed — positive for money coming in (CREDIT),
// negative for money going out (DEBIT) — mirroring how amount/movement_type
// already relate for a synced row; callers reject a zero amount before it
// reaches here (the HTTP handlers validate it before calling ledger.Service), since money.Eligibility
// would just report it as ReasonZeroValue.
type ManualInput struct {
	AccountID   string
	Description string
	Amount      decimal.Decimal
	OccurredAt  time.Time
}

// CreateManual inserts a new origin='manual' financial_transactions row:
// same entity as a synced row, minus every column that only means something
// for one (source_id/external_id/current_raw_import_id/normalized_hash stay
// NULL — see the CHECK constraint the manual_transactions migration adds).
// provider_status is hardcoded to POSTED: there is no "pending" for
// something the user is asserting already happened. Returns the new row's
// id; the caller (ledger.Service) is responsible for running automation and any
// explicit category choice afterward, exactly like a sync does for its own
// insert — this function only owns the base row.
func CreateManual(ctx context.Context, conn Querier, in ManualInput) (string, error) {
	if err := LockAccount(ctx, conn, in.AccountID); err != nil {
		return "", err
	}
	var currencyCode sql.NullString
	err := conn.QueryRowContext(ctx,
		`SELECT currency_code FROM financial_accounts WHERE id = ?`, in.AccountID,
	).Scan(&currencyCode)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrAccountNotFound
	}
	if err != nil {
		return "", err
	}

	movementType := "CREDIT"
	if in.Amount.IsNegative() {
		movementType = "DEBIT"
	}
	amount := money.CanonicalDecimal(in.Amount)

	id := uuid.NewString()
	now := db.FormatTime(time.Now())
	_, err = conn.ExecContext(ctx, `
		INSERT INTO financial_transactions (
			id, account_id, description, amount, amount_in_account_currency,
			currency_code, occurred_at, provider_status, movement_type,
			origin, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, 'POSTED', ?, 'manual', ?, ?)`,
		id, in.AccountID, in.Description, amount, amount,
		currencyCode, db.FormatTime(in.OccurredAt), movementType, now, now,
	)
	if err != nil {
		return "", err
	}
	return id, nil
}

// requireManual loads origin for transactionID, distinguishing "no such row"
// from "exists but isn't manual" so UpdateManual/DeleteManual can report the
// right sentinel.
func requireManual(ctx context.Context, q Querier, transactionID string) error {
	var origin string
	err := q.QueryRowContext(ctx,
		`SELECT origin FROM financial_transactions WHERE id = ? AND deleted_at IS NULL`, transactionID,
	).Scan(&origin)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrTransactionNotFound
	}
	if err != nil {
		return err
	}
	if origin != "manual" {
		return ErrNotManual
	}
	return nil
}

// UpdateManual overwrites a manual lançamento's core fields — account,
// description, signed amount, date. Category and inclusion keep going
// through their own existing endpoints (categories.AssignManual,
// SetInclusion), which are already agnostic of origin and need no change.
func UpdateManual(ctx context.Context, conn Querier, transactionID string, in ManualInput) error {
	if err := requireManual(ctx, conn, transactionID); err != nil {
		return err
	}
	if err := requireNoInvestmentLinks(ctx, conn, transactionID); err != nil {
		return err
	}

	if err := LockAccount(ctx, conn, in.AccountID); err != nil {
		return err
	}
	var currencyCode sql.NullString
	err := conn.QueryRowContext(ctx,
		`SELECT currency_code FROM financial_accounts WHERE id = ?`, in.AccountID,
	).Scan(&currencyCode)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrAccountNotFound
	}
	if err != nil {
		return err
	}

	movementType := "CREDIT"
	if in.Amount.IsNegative() {
		movementType = "DEBIT"
	}
	amount := money.CanonicalDecimal(in.Amount)

	_, err = conn.ExecContext(ctx, `
		UPDATE financial_transactions SET
			account_id = ?, description = ?, amount = ?, amount_in_account_currency = ?,
			currency_code = ?, occurred_at = ?, movement_type = ?, updated_at = ?
		WHERE id = ? AND origin = 'manual'`,
		in.AccountID, in.Description, amount, amount,
		currencyCode, db.FormatTime(in.OccurredAt), movementType, db.FormatTime(time.Now()),
		transactionID,
	)
	return err
}

// DeleteManual removes a manual lançamento from every view this package
// serves (the ledger, totals, GetItem — see viewSelect's deleted_at filter),
// by marking it deleted rather than physically removing the row.
//
// A hard DELETE is not available here: transaction_category_events and
// transaction_inclusion_events are append-only (their reject_delete
// triggers) and RESTRICT deleting the financial_transactions row they point
// at, so a manual row that was ever categorized or ignored/restored —
// essentially every one worth keeping — could never be hard-deleted. Soft
// deletion sidesteps that entirely: nothing about the row's history needs to
// change, it just stops matching viewSelect's WHERE.
//
// onIgnored runs first, inside the same transaction, to detach whatever a
// real inclusion change would detach (payable links, recurring
// reconciliations — see httpapi.onIgnoredHook) — a soft-deleted transaction
// must not keep counting toward a payable's settled amount or a recurring
// commitment's occurrence.
func DeleteManual(ctx context.Context, conn *sql.DB, transactionID string, onIgnored OnIgnoredHook) error {
	if err := requireManual(ctx, conn, transactionID); err != nil {
		return err
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := requireNoInvestmentLinks(ctx, tx, transactionID); err != nil {
		return err
	}

	if onIgnored != nil {
		if err := onIgnored(ctx, tx, transactionID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE financial_transactions SET deleted_at = ? WHERE id = ? AND origin = 'manual'`,
		db.FormatTime(time.Now()), transactionID,
	); err != nil {
		return err
	}
	return tx.Commit()
}
