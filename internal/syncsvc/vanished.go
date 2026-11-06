package syncsvc

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/db"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/greg0x46/julius/internal/pluggy"
)

const (
	// successorWindow is how far apart a pending transaction and the posted
	// one that replaced it may be dated: the provider re-dates a settlement.
	successorWindow = 3 * 24 * time.Hour
	// maxRemovalsPerAccount bounds what one sync drops from one account, so a
	// misbehaving listing cannot empty it. Past it nothing is removed.
	maxRemovalsPerAccount = 50
)

// accountListing is what the provider reported for one account in a run.
type accountListing struct {
	seen map[string]bool
	// oldest is the earliest occurred_at reported. Stored rows dated before its
	// day lie outside the window the provider answers for, so their absence
	// says nothing; the day, not the instant, because the provider re-dates a
	// settled entry by hours.
	oldest *time.Time
	// partial: a record was rejected, so seen may lack ids the provider sent.
	partial bool
}

func newAccountListing() *accountListing { return &accountListing{seen: map[string]bool{}} }

func (l *accountListing) add(page pluggy.TransactionsPage) {
	if len(page.Rejections) > 0 {
		l.partial = true
	}
	for _, record := range page.Transactions {
		l.seen[record.ExternalID] = true
		if record.OccurredAt != nil && (l.oldest == nil || record.OccurredAt.Before(*l.oldest)) {
			oldest := *record.OccurredAt
			l.oldest = &oldest
		}
	}
}

// storedTransaction is the part of a synced row that matching needs.
type storedTransaction struct {
	id, externalID, status, movement, currency string
	amount                                     *decimal.Decimal
	occurredAt                                 *time.Time
	createdAt                                  time.Time
}

func (s *Service) storedTransactions(ctx context.Context, accountID string) ([]storedTransaction, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, external_id, provider_status, movement_type, currency_code, amount, occurred_at, created_at
		FROM financial_transactions
		WHERE source_id = ? AND account_id = ? AND origin = 'synced' AND deleted_at IS NULL`,
		s.SourceID, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var stored []storedTransaction
	for rows.Next() {
		var row storedTransaction
		var status, movement, currency sql.NullString
		var created time.Time
		if err := rows.Scan(dbtypes.ID(&row.id), &row.externalID, &status, &movement, &currency,
			dbtypes.Decimal(&row.amount), dbtypes.Time(&row.occurredAt), dbtypes.Time(&created)); err != nil {
			return nil, err
		}
		row.status, row.movement, row.currency, row.createdAt = status.String, movement.String, currency.String, created
		stored = append(stored, row)
	}
	return stored, rows.Err()
}

// reconcileVanishedPending drops the pending transactions the provider stopped
// reporting. A pending entry that settles usually keeps its id, but when the
// provider cannot tell it is the same movement (the date, description or
// amount changed) it withdraws the pending one and reports a new posted one;
// without this the pair counts twice. Only a listing read to its last page
// without a rejected record is trusted, and only PENDING rows are dropped: a
// posted row that went missing may come back, and balances and history lean on
// it, so it is left alone and only logged.
//
// When exactly one posted row that appeared later has the same account,
// direction, amount and (nearly) the same day, it is the successor: the hook
// moves what the user attached to the pending row onto it. Without one (a
// cancelled payment, or an ambiguous pair) the hook detaches it instead.
// Nothing runs without a hook, since a removal that leaves links behind would
// keep a recurrence or payable counted by a transaction that is gone.
func (s *Service) reconcileVanishedPending(ctx context.Context, accountID, externalAccountID string, listing *accountListing) error {
	if s.OnTransactionRemoved == nil || listing.partial || len(listing.seen) == 0 || listing.oldest == nil {
		return nil
	}
	stored, err := s.storedTransactions(ctx, accountID)
	if err != nil {
		return err
	}
	oldest := listing.oldest.UTC()
	windowStart := time.Date(oldest.Year(), oldest.Month(), oldest.Day(), 0, 0, 0, 0, time.UTC)
	var vanished []storedTransaction
	missingPosted := 0
	for _, row := range stored {
		if row.occurredAt == nil || row.occurredAt.Before(windowStart) || listing.seen[row.externalID] {
			continue
		}
		switch row.status {
		case "PENDING":
			vanished = append(vanished, row)
		case "POSTED":
			missingPosted++
		}
	}
	if missingPosted > 0 {
		log.Printf("transactions_missing_from_provider run_id=%s account=%s posted=%d kept", s.SyncRunID, externalAccountID, missingPosted)
	}
	if len(vanished) == 0 {
		return nil
	}
	if len(vanished) > maxRemovalsPerAccount {
		log.Printf("transactions_removal_skipped run_id=%s account=%s vanished=%d limit=%d", s.SyncRunID, externalAccountID, len(vanished), maxRemovalsPerAccount)
		return nil
	}

	successors := matchSuccessors(vanished, stored, listing.seen)
	for _, row := range vanished {
		var successorID *string
		if id, ok := successors[row.id]; ok {
			successorID = &id
		}
		if err := s.removeTransaction(ctx, row.id, successorID); err != nil {
			return err
		}
	}
	log.Printf("transactions_removed run_id=%s account=%s removed=%d with_successor=%d", s.SyncRunID, externalAccountID, len(vanished), len(successors))
	return nil
}

// removeTransaction hands the pending row's user data to the hook and marks
// it deleted, in one transaction.
func (s *Service) removeTransaction(ctx context.Context, id string, successorID *string) error {
	tx, err := s.beginOwnedTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := s.OnTransactionRemoved(ctx, tx, id, successorID); err != nil {
		return err
	}
	now := db.FormatTime(time.Now())
	if _, err := tx.ExecContext(ctx, `
		UPDATE financial_transactions SET deleted_at = ?, updated_at = ?
		WHERE id = ? AND origin = 'synced' AND provider_status = 'PENDING' AND deleted_at IS NULL`,
		now, now, id); err != nil {
		return err
	}
	return tx.Commit()
}

// matchSuccessors pairs each vanished pending row with the posted row that
// replaced it, keyed by the vanished row's id. A pair needs the same movement,
// currency and amount, a day within successorWindow, and a posted row first
// seen after the pending one; it is kept only when neither side has another
// candidate, since a wrong pairing would move one transaction's decisions onto
// an unrelated one.
func matchSuccessors(vanished, stored []storedTransaction, seen map[string]bool) map[string]string {
	candidates := map[string][]string{}
	claims := map[string]int{}
	for _, pending := range vanished {
		for _, posted := range stored {
			if posted.status != "POSTED" || !seen[posted.externalID] || !replaces(posted, pending) {
				continue
			}
			candidates[pending.id] = append(candidates[pending.id], posted.id)
			claims[posted.id]++
		}
	}
	matched := map[string]string{}
	for pendingID, ids := range candidates {
		if len(ids) == 1 && claims[ids[0]] == 1 {
			matched[pendingID] = ids[0]
		}
	}
	return matched
}

func replaces(posted, pending storedTransaction) bool {
	if posted.amount == nil || pending.amount == nil || posted.occurredAt == nil || pending.occurredAt == nil {
		return false
	}
	apart := posted.occurredAt.Sub(*pending.occurredAt)
	if apart < 0 {
		apart = -apart
	}
	return posted.movement == pending.movement && posted.currency == pending.currency &&
		posted.amount.Equal(*pending.amount) && apart <= successorWindow &&
		posted.createdAt.After(pending.createdAt)
}
