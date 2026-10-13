package transactions

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/greg0x46/julius/internal/db"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/greg0x46/julius/internal/money"
	"github.com/shopspring/decimal"
)

var ErrInvalidTransfer = errors.New("invalid transfer movements")
var ErrTransferLinked = errors.New("transaction already linked to a transfer")
var ErrTransferNotFound = errors.New("transfer not found")

type TransferInput struct {
	SourceID      string
	DestinationID string
	FeeID         string
}

type TransferMovement struct {
	TransactionID string  `json:"transaction_id"`
	AccountID     string  `json:"account_id"`
	AccountName   string  `json:"account_name"`
	Amount        *string `json:"amount"`
	CurrencyCode  string  `json:"currency_code"`
}

type AccountTransfer struct {
	ID          string           `json:"id"`
	Source      TransferMovement `json:"source"`
	Destination TransferMovement `json:"destination"`
	// Source currency units per one destination currency unit, rounded to 16 places.
	EffectiveRate string            `json:"effective_rate"`
	Fee           *TransferMovement `json:"fee"`
}

func RequireNoTransferLink(ctx context.Context, q Querier, id string) error {
	var linked bool
	if err := q.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM transfer_transaction_links WHERE transaction_id = ?)`, id).Scan(dbtypes.Bool(&linked)); err != nil {
		return err
	}
	if linked {
		return ErrTransferLinked
	}
	return nil
}

func UnlinkTransferForTransaction(ctx context.Context, q Querier, id string) error {
	_, err := q.ExecContext(ctx, `DELETE FROM transfer_links WHERE id IN (SELECT transfer_id FROM transfer_transaction_links WHERE transaction_id = ?)`, id)
	return err
}

func DeleteTransfer(ctx context.Context, q Querier, id string) error {
	result, err := q.ExecContext(ctx, `DELETE FROM transfer_links WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrTransferNotFound
	}
	return nil
}

func transferMovement(ctx context.Context, q Querier, id string) (TransferMovement, error) {
	var m TransferMovement
	var name sql.NullString
	var amount *decimal.Decimal
	var currency, status, movement sql.NullString
	var accountType sql.NullString
	var inclusion string
	err := q.QueryRowContext(ctx, `SELECT ft.id, ft.account_id, fa.name, ft.amount_in_account_currency,
 fa.currency_code, ft.provider_status, ft.movement_type, fa.account_type, COALESCE(tid.state, 'considered')
 FROM financial_transactions ft JOIN financial_accounts fa ON fa.id = ft.account_id
 LEFT JOIN transaction_inclusion_decisions tid ON tid.transaction_id = ft.id
 WHERE ft.id = ? AND ft.deleted_at IS NULL`, id).Scan(dbtypes.ID(&m.TransactionID), dbtypes.ID(&m.AccountID), &name, dbtypes.Decimal(&amount), &currency, &status, &movement, &accountType, &inclusion)
	if errors.Is(err, sql.ErrNoRows) {
		return m, ErrTransactionNotFound
	}
	if err != nil {
		return m, err
	}
	if amount == nil || amount.IsZero() || len(currency.String) != 3 || accountType.String == "CREDIT" || inclusion == "ignored" || (status.String != "POSTED" && status.String != "PENDING") {
		return m, ErrInvalidTransfer
	}
	if (amount.IsNegative() && movement.String != "DEBIT") || (amount.IsPositive() && movement.String != "CREDIT") {
		return m, ErrInvalidTransfer
	}
	nativeAmount := money.CanonicalDecimal(*amount)
	m.AccountName, m.Amount, m.CurrencyCode = name.String, &nativeAmount, currency.String
	return m, nil
}

// CreateTransfer annotates existing owner-account movements. It never changes their rows or decisions.
// The caller owns the database transaction; the unique transaction key also guards concurrent pairing.
func CreateTransfer(ctx context.Context, q Querier, in TransferInput) (string, error) {
	if in.SourceID == "" || in.DestinationID == "" || in.SourceID == in.DestinationID || (in.FeeID != "" && (in.FeeID == in.SourceID || in.FeeID == in.DestinationID)) {
		return "", ErrInvalidTransfer
	}
	ids := []string{in.SourceID, in.DestinationID}
	if in.FeeID != "" {
		ids = append(ids, in.FeeID)
	}
	lockIDs := append([]string(nil), ids...)
	sort.Strings(lockIDs)
	// Share the bank-row lock with investment reconciliation before checking links.
	for _, id := range lockIDs {
		if _, err := q.ExecContext(ctx, `UPDATE financial_transactions SET id = id WHERE id = ?`, id); err != nil {
			return "", err
		}
	}
	for _, id := range ids {
		if err := RequireNoTransferLink(ctx, q, id); err != nil {
			return "", err
		}
		if err := requireNoInvestmentLinks(ctx, q, id); err != nil {
			return "", err
		}
	}
	source, err := transferMovement(ctx, q, in.SourceID)
	if err != nil {
		return "", err
	}
	destination, err := transferMovement(ctx, q, in.DestinationID)
	if err != nil {
		return "", err
	}
	sent, received := decimal.RequireFromString(*source.Amount), decimal.RequireFromString(*destination.Amount)
	if source.AccountID == destination.AccountID || !sent.IsNegative() || !received.IsPositive() || (source.CurrencyCode == destination.CurrencyCode && !sent.Abs().Equal(received)) {
		return "", ErrInvalidTransfer
	}
	if in.FeeID != "" {
		fee, err := transferMovement(ctx, q, in.FeeID)
		if err != nil {
			return "", err
		}
		item, found, err := GetItem(ctx, q, in.FeeID)
		if err != nil {
			return "", err
		}
		if !found || !decimal.RequireFromString(*fee.Amount).IsNegative() || !item.TotalsEligibility.Included || (fee.AccountID != source.AccountID && fee.AccountID != destination.AccountID) {
			return "", ErrInvalidTransfer
		}
	}
	id := uuid.NewString()
	if _, err := q.ExecContext(ctx, `INSERT INTO transfer_links (id, created_at) VALUES (?, ?)`, id, db.FormatTime(time.Now())); err != nil {
		return "", err
	}
	roles := []string{"source", "destination", "fee"}
	for i, movementID := range ids {
		result, err := q.ExecContext(ctx, `INSERT INTO transfer_transaction_links (transaction_id, transfer_id, role) VALUES (?, ?, ?) ON CONFLICT (transaction_id) DO NOTHING`, movementID, id, roles[i])
		if err != nil {
			return "", err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return "", err
		}
		if n == 0 {
			return "", ErrTransferLinked
		}
	}
	return id, nil
}

func GetTransferForTransaction(ctx context.Context, q Querier, transactionID string) (*AccountTransfer, error) {
	var id string
	err := q.QueryRowContext(ctx, `SELECT transfer_id FROM transfer_transaction_links WHERE transaction_id = ?`, transactionID).Scan(dbtypes.ID(&id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return GetTransfer(ctx, q, id)
}

// Read native amounts again so provider corrections are reflected without rewriting provider data.
func GetTransfer(ctx context.Context, q Querier, id string) (*AccountTransfer, error) {
	rows, err := q.QueryContext(ctx, `SELECT tl.role, ft.id, ft.account_id, fa.name, ft.amount_in_account_currency, fa.currency_code
 FROM transfer_transaction_links tl JOIN financial_transactions ft ON ft.id = tl.transaction_id
 JOIN financial_accounts fa ON fa.id = ft.account_id WHERE tl.transfer_id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := &AccountTransfer{ID: id}
	for rows.Next() {
		var m TransferMovement
		var role string
		var name, currency sql.NullString
		if err := rows.Scan(&role, dbtypes.ID(&m.TransactionID), dbtypes.ID(&m.AccountID), &name, dbtypes.Decimal(&m.Amount), &currency); err != nil {
			return nil, err
		}
		m.AccountName, m.CurrencyCode = name.String, currency.String
		switch role {
		case "source":
			result.Source = m
		case "destination":
			result.Destination = m
		case "fee":
			result.Fee = &m
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if result.Source.TransactionID == "" || result.Destination.TransactionID == "" {
		return nil, ErrTransferNotFound
	}
	if result.Source.Amount == nil || result.Destination.Amount == nil {
		return result, nil
	}
	sent, e1 := decimal.NewFromString(*result.Source.Amount)
	received, e2 := decimal.NewFromString(*result.Destination.Amount)
	if e1 == nil && e2 == nil && !received.IsZero() {
		result.EffectiveRate = sent.Abs().DivRound(received.Abs(), 16).String()
	}
	return result, nil
}
