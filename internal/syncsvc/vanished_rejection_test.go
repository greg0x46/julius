package syncsvc_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/pluggy"
	"github.com/greg0x46/julius/internal/transactions"
)

func TestVanishedPendingIsKeptWhenSuccessorIngestionFails(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		f := newVanishFixture(t, conn)
		f.listing(t, record("pending-1", "PENDING", "6772.00", salaryDay))
		pendingID := f.internalID(t, "pending-1")
		commitment := f.reconcile(t, "pending-1")

		runID, rawID := uuid.NewString(), uuid.NewString()
		if _, err := conn.Exec(`INSERT INTO sync_runs (id, source_id, status, started_at) VALUES (?, ?, 'in_progress', ?)`,
			runID, f.sourceID, db.FormatTime(time.Now())); err != nil {
			t.Fatal(err)
		}
		insertRawImport(t, conn, rawID, runID, f.sourceID)
		f.provider.transactionPages["acc-1"] = []pluggy.TransactionsPage{{
			RawImportID: rawID,
			Transactions: []pluggy.TransactionSnapshot{
				record("posted-1", "POSTED", "6772.00", salaryDay),
			},
		}, {
			RawImportID:  rawID,
			Transactions: []pluggy.TransactionSnapshot{record("other", "POSTED", "10.00", salaryDay)},
		}}
		service := newVanishService(f, runID)
		service.OnTransactionUpserted = func(ctx context.Context, tx transactions.Querier, transactionID, accountID string) error {
			var externalID string
			if err := tx.QueryRowContext(ctx, `SELECT external_id FROM financial_transactions WHERE id = ?`, transactionID).Scan(&externalID); err != nil {
				return err
			}
			if externalID == "posted-1" {
				return errors.New("synthetic hook failure")
			}
			return nil
		}
		if err := service.Execute(context.Background()); err != nil {
			t.Fatal(err)
		}

		assertRetryRun(t, conn, runID, "completed_with_failures", 1, 0, 1)
		if f.removedAt(t, "pending-1") {
			t.Fatal("pending row removed despite rejected successor")
		}
		if linked := f.linkedTo(t, commitment); linked == nil || *linked != pendingID {
			t.Fatalf("recurrence link = %v, want pending row %s", linked, pendingID)
		}
		if f.removedAt(t, "other") {
			t.Fatal("successful neighboring transaction is not active")
		}

		// A complete retry can move the preserved link to the successor.
		f.listing(t, record("posted-1", "POSTED", "6772.00", salaryDay), record("other", "POSTED", "10.00", salaryDay))
		if !f.removedAt(t, "pending-1") {
			t.Fatal("pending row was not removed after successful retry")
		}
		if linked := f.linkedTo(t, commitment); linked == nil || *linked != f.internalID(t, "posted-1") {
			t.Fatalf("recurrence link = %v, want posted successor", linked)
		}
	})
}
