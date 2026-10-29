package ledger

import (
	"context"
	"database/sql"
	"errors"
	"github.com/greg0x46/julius/internal/transactions"
)

func (s *Service) LinkTransfer(ctx context.Context, in transactions.TransferInput) (*transactions.AccountTransfer, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	id, err := transactions.CreateTransfer(ctx, tx, in)
	if err != nil {
		return nil, err
	}
	result, err := transactions.GetTransfer(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) CreateManualTransfer(ctx context.Context, source, destination transactions.ManualInput, fee *transactions.ManualInput) (*transactions.AccountTransfer, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	result, err := CreateManualTransferWithQuerier(ctx, tx, source, destination, fee, s.OnIgnored)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

// CreateManualTransferWithQuerier applies all movements and decisions in the caller's transaction.
func CreateManualTransferWithQuerier(ctx context.Context, q transactions.Querier, source, destination transactions.ManualInput, fee *transactions.ManualInput, onIgnored transactions.OnIgnoredHook) (*transactions.AccountTransfer, error) {
	if source.AccountID == destination.AccountID || !source.Amount.IsNegative() || !destination.Amount.IsPositive() || source.OccurredAt.IsZero() || destination.OccurredAt.IsZero() {
		return nil, transactions.ErrInvalidTransfer
	}
	inputs := []transactions.ManualInput{source, destination}
	if fee != nil {
		inputs = append(inputs, *fee)
	}
	ids := make([]string, 0, len(inputs))
	for _, in := range inputs {
		var origin string
		if err := q.QueryRowContext(ctx, `SELECT origin FROM financial_accounts WHERE id = ?`, in.AccountID).Scan(&origin); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, transactions.ErrAccountNotFound
			}
			return nil, err
		}
		if origin != "manual" {
			return nil, transactions.ErrInvalidTransfer
		}
		id, err := transactions.CreateManual(ctx, q, in)
		if err != nil {
			return nil, err
		}
		if err := ApplyNewTransactionDecisions(ctx, q, id, nil, onIgnored); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	in := transactions.TransferInput{SourceID: ids[0], DestinationID: ids[1]}
	if fee != nil {
		in.FeeID = ids[2]
	}
	id, err := transactions.CreateTransfer(ctx, q, in)
	if err != nil {
		return nil, err
	}
	result, err := transactions.GetTransfer(ctx, q, id)
	if err != nil {
		return nil, err
	}
	return result, nil
}
