package ledger

import (
	"context"
	"database/sql"
	"errors"

	"github.com/greg0x46/julius/internal/categories"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/transactions"
)

// OnSyncedRemoved is the cleanup for "a synced transaction the provider
// withdrew is about to be marked deleted" (transactions.RemovedHook). With a
// successor — the posted row that replaced it under a new external id — what
// the user attached moves there, so replacing a pending entry by its posted
// one never undoes a manual category, an ignore, a recurrence reconciliation, a
// payable link, a transfer pairing or an investment reconciliation. Whatever
// is left (everything, without a successor) is detached like for an ignored
// transaction: a deleted row must not keep carrying a commitment's money.
func OnSyncedRemoved(ctx context.Context, q transactions.Querier, removedID string, successorID *string) error {
	if successorID != nil {
		if err := moveDecisions(ctx, q, removedID, *successorID); err != nil {
			return err
		}
		if err := moveLinks(ctx, q, removedID, *successorID); err != nil {
			return err
		}
	}
	if err := transactions.UnlinkTransferForTransaction(ctx, q, removedID); err != nil {
		return err
	}
	return OnIgnored(ctx, q, removedID)
}

// moveDecisions copies the removed row's manual category and manual inclusion
// to the successor, unless the successor already carries its own manual one:
// a user decision outranks the successor's automatic ones, never another
// user decision.
func moveDecisions(ctx context.Context, q transactions.Querier, removedID, successorID string) error {
	var categoryID string
	err := q.QueryRowContext(ctx, `SELECT category_id FROM transaction_category_decisions WHERE transaction_id = ? AND origin = 'manual'`, removedID).Scan(dbtypes.ID(&categoryID))
	if err == nil {
		var taken int
		err = q.QueryRowContext(ctx, `SELECT 1 FROM transaction_category_decisions WHERE transaction_id = ? AND origin = 'manual'`, successorID).Scan(&taken)
		if errors.Is(err, sql.ErrNoRows) {
			_, err = categories.AssignManualWithQuerier(ctx, q, successorID, categoryID)
			if errors.Is(err, categories.ErrCategoryInvalid) {
				// The category was deactivated since: the decision stays on the
				// removed row rather than failing the whole removal every sync.
				err = nil
			}
		}
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	var state string
	err = q.QueryRowContext(ctx, `SELECT state FROM transaction_inclusion_decisions WHERE transaction_id = ? AND origin = 'manual'`, removedID).Scan(&state)
	if err == nil {
		var taken int
		err = q.QueryRowContext(ctx, `SELECT 1 FROM transaction_inclusion_decisions WHERE transaction_id = ? AND origin = 'manual'`, successorID).Scan(&taken)
		if errors.Is(err, sql.ErrNoRows) {
			_, err = transactions.SetInclusion(ctx, q, successorID, money.InclusionState(state), transactions.InclusionOriginManual, nil, nil, OnIgnored)
		}
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return nil
}

// moveLinks repoints every link the removed row holds to the successor. A
// table where the successor already has a link of its own is left as is, so
// the removed row's link falls to the detach that follows.
func moveLinks(ctx context.Context, q transactions.Querier, removedID, successorID string) error {
	for _, link := range []struct{ table, column string }{
		{"transfer_transaction_links", "transaction_id"},
		{"payable_transaction_links", "transaction_id"},
		{"scenario_realizations", "transaction_id"},
		{"investment_reconciliations", "financial_transaction_id"},
	} {
		if _, err := q.ExecContext(ctx, `
			UPDATE `+link.table+` SET `+link.column+` = ? WHERE `+link.column+` = ?
			AND NOT EXISTS (SELECT 1 FROM `+link.table+` WHERE `+link.column+` = ?)`,
			successorID, removedID, successorID); err != nil {
			return err
		}
	}
	return nil
}
