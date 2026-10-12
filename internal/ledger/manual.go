package ledger

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/greg0x46/julius/internal/categories"
	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/transactions"
)

// ErrItemUnavailable is returned when a committed lançamento cannot be read back.
var ErrItemUnavailable = errors.New("transaction unavailable after commit")
var ErrIdempotencyConflict = errors.New("idempotency key used with a different request")

// Service owns the lançamento manual flows (see
// .specs/contextos/transacoes/reference.md). Domain errors from the underlying
// packages are returned unchanged so callers can match them with errors.Is.
type Service struct {
	DB        *sql.DB
	OnIgnored transactions.OnIgnoredHook // nil is a valid no-op
}

// CreateManual inserts a lançamento the user authors by hand and runs the same
// automation pass a sync would, so an existing rule categorizes or ignores a
// manual entry exactly like a synced one. An explicit categoryID is applied
// last, so the user's own choice always wins. Everything commits or rolls back
// together.
func (s *Service) CreateManual(ctx context.Context, in transactions.ManualInput, categoryID *string) (transactions.Item, error) {
	return s.CreateManualIdempotent(ctx, in, categoryID, "", 0)
}

// CreateManualIdempotent binds a supplied key to one owner's normalized request.
// The reservation and transaction commit together, so a failed attempt can be retried.
func (s *Service) CreateManualIdempotent(ctx context.Context, in transactions.ManualInput, categoryID *string, key string, ownerID int64) (transactions.Item, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return transactions.Item{}, err
	}
	defer tx.Rollback()
	if key != "" {
		fingerprint, err := manualFingerprint(in, categoryID)
		if err != nil {
			return transactions.Item{}, err
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO manual_transaction_idempotency
			(owner_id, idempotency_key, request_hash, created_at)
			VALUES (?, ?, ?, ?) ON CONFLICT (owner_id, idempotency_key) DO NOTHING`,
			ownerID, key, fingerprint, db.FormatTime(time.Now()))
		if err != nil {
			return transactions.Item{}, err
		}
		inserted, err := result.RowsAffected()
		if err != nil {
			return transactions.Item{}, err
		}
		if inserted == 0 {
			var savedHash string
			var id sql.NullString
			if err := tx.QueryRowContext(ctx, `SELECT request_hash, transaction_id FROM manual_transaction_idempotency
				WHERE owner_id = ? AND idempotency_key = ?`, ownerID, key).Scan(&savedHash, &id); err != nil {
				return transactions.Item{}, err
			}
			if savedHash != fingerprint {
				return transactions.Item{}, ErrIdempotencyConflict
			}
			if !id.Valid {
				return transactions.Item{}, ErrItemUnavailable
			}
			if err := tx.Rollback(); err != nil {
				return transactions.Item{}, err
			}
			return s.item(ctx, id.String)
		}
	}

	id, err := transactions.CreateManual(ctx, tx, in)
	if err != nil {
		return transactions.Item{}, err
	}
	if err := ApplyNewTransactionDecisions(ctx, tx, id, categoryID, s.OnIgnored); err != nil {
		return transactions.Item{}, err
	}
	if key != "" {
		if _, err := tx.ExecContext(ctx, `UPDATE manual_transaction_idempotency SET transaction_id = ?
			WHERE owner_id = ? AND idempotency_key = ?`, id, ownerID, key); err != nil {
			return transactions.Item{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return transactions.Item{}, err
	}
	return s.item(ctx, id)
}

func manualFingerprint(in transactions.ManualInput, categoryID *string) (string, error) {
	request := struct {
		AccountID   string  `json:"account_id"`
		Description string  `json:"description"`
		Amount      string  `json:"amount"`
		OccurredAt  string  `json:"occurred_at"`
		CategoryID  *string `json:"category_id"`
	}{in.AccountID, in.Description, in.Amount.String(), in.OccurredAt.Format("2006-01-02"), categoryID}
	encoded, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// UpdateManual edits a lançamento manual's core fields and, when supplied, its
// category in the same database transaction.
func (s *Service) UpdateManual(ctx context.Context, id string, in transactions.ManualInput, categoryID *string) (transactions.Item, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return transactions.Item{}, err
	}
	defer tx.Rollback()

	if err := transactions.UpdateManual(ctx, tx, id, in); err != nil {
		return transactions.Item{}, err
	}
	if categoryID != nil {
		if _, err := categories.AssignManualWithQuerier(ctx, tx, id, *categoryID); err != nil {
			return transactions.Item{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return transactions.Item{}, err
	}
	return s.item(ctx, id)
}

// DeleteManual soft-deletes a lançamento manual.
func (s *Service) DeleteManual(ctx context.Context, id string) error {
	return transactions.DeleteManual(ctx, s.DB, id, s.OnIgnored)
}

func (s *Service) item(ctx context.Context, id string) (transactions.Item, error) {
	item, found, err := transactions.GetItem(ctx, s.DB, id)
	if err != nil {
		return transactions.Item{}, err
	}
	if !found {
		return transactions.Item{}, ErrItemUnavailable
	}
	return item, nil
}
