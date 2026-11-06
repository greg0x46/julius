package syncsvc_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/categories"
	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/ledger"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/pluggy"
	"github.com/greg0x46/julius/internal/recurrences"
	"github.com/greg0x46/julius/internal/syncsvc"
	"github.com/greg0x46/julius/internal/transactions"
)

// vanishFixture syncs one account over several runs, each run reporting
// exactly the records it is given, as the provider's full listing would.
type vanishFixture struct {
	retryFixture
	removed transactions.RemovedHook
}

func newVanishFixture(t *testing.T, conn *sql.DB) *vanishFixture {
	t.Helper()
	return &vanishFixture{retryFixture: newRetryFixture(t, conn), removed: ledger.OnSyncedRemoved}
}

func (f *vanishFixture) listing(t *testing.T, records ...pluggy.TransactionSnapshot) {
	t.Helper()
	f.page(t, pluggy.TransactionsPage{Transactions: records})
}

func (f *vanishFixture) page(t *testing.T, page pluggy.TransactionsPage) {
	t.Helper()
	// Distinct creation times: a successor is the posted row first seen later.
	time.Sleep(2 * time.Millisecond)
	runID := uuid.NewString()
	if _, err := f.conn.Exec(`INSERT INTO sync_runs (id, source_id, status, started_at) VALUES (?, ?, 'in_progress', ?)`,
		runID, f.sourceID, db.FormatTime(time.Now())); err != nil {
		t.Fatalf("insert sync run: %v", err)
	}
	page.RawImportID = uuid.NewString()
	insertRawImport(t, f.conn, page.RawImportID, runID, f.sourceID)
	f.provider.transactionPages["acc-1"] = []pluggy.TransactionsPage{page}
	service := newVanishService(f, runID)
	if err := service.Execute(context.Background()); err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

func newVanishService(f *vanishFixture, runID string) *syncsvc.Service {
	return &syncsvc.Service{
		DB: f.conn, Provider: f.provider, SyncRunID: runID, SourceID: f.sourceID, OnTransactionRemoved: f.removed,
	}
}

func record(externalID, status, amount string, at time.Time) pluggy.TransactionSnapshot {
	movement := "CREDIT"
	if amount[0] == '-' {
		movement = "DEBIT"
	}
	return pluggy.TransactionSnapshot{
		ExternalID: externalID, ExternalAccountID: "acc-1", Description: strp("Salário " + externalID),
		Amount: amountP(amount), CurrencyCode: strp("BRL"), OccurredAt: &at,
		MovementType: strp(movement), ProviderStatus: strp(status),
	}
}

func (f *vanishFixture) internalID(t *testing.T, externalID string) string {
	t.Helper()
	var id string
	if err := f.conn.QueryRow(`SELECT id FROM financial_transactions WHERE external_id = ?`, externalID).Scan(&id); err != nil {
		t.Fatalf("transaction %s: %v", externalID, err)
	}
	return id
}

func (f *vanishFixture) removedAt(t *testing.T, externalID string) bool {
	t.Helper()
	var removed int
	if err := f.conn.QueryRow(`SELECT CASE WHEN deleted_at IS NULL THEN 0 ELSE 1 END FROM financial_transactions WHERE external_id = ?`, externalID).Scan(&removed); err != nil {
		t.Fatalf("transaction %s: %v", externalID, err)
	}
	return removed == 1
}

func (f *vanishFixture) manualCategory(t *testing.T, externalID string) (categoryID string, ok bool) {
	t.Helper()
	err := f.conn.QueryRow(`SELECT category_id FROM transaction_category_decisions WHERE transaction_id = ? AND origin = 'manual'`, f.internalID(t, externalID)).Scan(&categoryID)
	if err == sql.ErrNoRows {
		return "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return categoryID, true
}

func (f *vanishFixture) incomeCategory(t *testing.T) string {
	t.Helper()
	category, err := categories.Create(context.Background(), f.conn, "Salário", money.Income, "wallet", "#00aa00")
	if err != nil {
		t.Fatalf("categories.Create: %v", err)
	}
	return category.ID
}

// commitment is a recurring commitment whose 2026-11-05 occurrence is
// reconciled with the transaction externalID.
func (f *vanishFixture) reconcile(t *testing.T, externalID string) (commitmentID string) {
	t.Helper()
	ctx := context.Background()
	category, err := categories.Create(ctx, f.conn, "Aluguel", money.Expense, "home", "#ff0000")
	if err != nil {
		t.Fatalf("categories.Create: %v", err)
	}
	commitment, err := recurrences.Create(ctx, f.conn, recurrences.Write{
		Name: "Aluguel", Kind: recurrences.KindExpense, Amount: decimal.RequireFromString("6772.00"), CategoryID: category.ID,
		Cadence: recurrences.CadenceMonthly, DayOfMonth: 5,
		StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), IsActive: true,
	})
	if err != nil {
		t.Fatalf("recurrences.Create: %v", err)
	}
	id := f.internalID(t, externalID)
	if _, err := recurrences.PutOverride(ctx, f.conn, commitment.ID, occurrenceDay, recurrences.StateLinked, &id); err != nil {
		t.Fatalf("PutOverride: %v", err)
	}
	return commitment.ID
}

var (
	salaryDay     = time.Date(2026, 11, 5, 3, 0, 0, 0, time.UTC)
	occurrenceDay = time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
)

func (f *vanishFixture) linkedTo(t *testing.T, commitmentID string) *string {
	t.Helper()
	override, found, err := recurrences.GetOverride(context.Background(), f.conn, commitmentID, occurrenceDay)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		return nil
	}
	return override.TransactionID
}

func TestVanishedPendingHandsItsDecisionsToThePostedSuccessor(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		f := newVanishFixture(t, conn)
		category := f.incomeCategory(t)

		f.listing(t, record("pending-1", "PENDING", "6772.00", salaryDay))
		if _, err := categories.AssignManual(context.Background(), conn, f.internalID(t, "pending-1"), category); err != nil {
			t.Fatalf("AssignManual: %v", err)
		}
		commitment := f.reconcile(t, "pending-1")

		// The provider withdraws the pending credit and reports a posted one.
		f.listing(t, record("posted-1", "POSTED", "6772.00", salaryDay.Add(time.Hour)))

		if !f.removedAt(t, "pending-1") || f.removedAt(t, "posted-1") {
			t.Fatalf("pending removed=%v posted removed=%v, want true/false", f.removedAt(t, "pending-1"), f.removedAt(t, "posted-1"))
		}
		if got, ok := f.manualCategory(t, "posted-1"); !ok || got != category {
			t.Errorf("successor manual category = %q (%v), want %q", got, ok, category)
		}
		linked := f.linkedTo(t, commitment)
		if linked == nil || *linked != f.internalID(t, "posted-1") {
			t.Errorf("occurrence linked to %v, want the successor %s", linked, f.internalID(t, "posted-1"))
		}
	})
}

func TestVanishedPendingStillGoesWhenItsCategoryWasDeactivated(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		f := newVanishFixture(t, conn)
		category := f.incomeCategory(t)
		f.listing(t, record("pending-1", "PENDING", "6772.00", salaryDay))
		if _, err := categories.AssignManual(context.Background(), conn, f.internalID(t, "pending-1"), category); err != nil {
			t.Fatal(err)
		}
		inactive := false
		if _, err := categories.Update(context.Background(), conn, category, nil, &inactive, nil, nil); err != nil {
			t.Fatalf("deactivate category: %v", err)
		}

		f.listing(t, record("posted-1", "POSTED", "6772.00", salaryDay))

		if !f.removedAt(t, "pending-1") {
			t.Error("a category that can no longer be assigned blocked the removal")
		}
		if _, ok := f.manualCategory(t, "posted-1"); ok {
			t.Error("a deactivated category was assigned to the successor")
		}
	})
}

func TestVanishedPendingMovesAManualIgnoreToTheSuccessor(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		f := newVanishFixture(t, conn)
		f.listing(t, record("pending-1", "PENDING", "-50.00", salaryDay))
		if _, err := transactions.SetInclusion(context.Background(), conn, f.internalID(t, "pending-1"), money.Ignored,
			transactions.InclusionOriginManual, nil, nil, ledger.OnIgnored); err != nil {
			t.Fatalf("SetInclusion: %v", err)
		}

		f.listing(t, record("posted-1", "POSTED", "-50.00", salaryDay))

		var state, origin string
		if err := conn.QueryRow(`SELECT state, origin FROM transaction_inclusion_decisions WHERE transaction_id = ?`, f.internalID(t, "posted-1")).Scan(&state, &origin); err != nil {
			t.Fatalf("successor inclusion decision: %v", err)
		}
		if state != "ignored" || origin != "manual" {
			t.Errorf("successor inclusion = %s/%s, want ignored/manual", state, origin)
		}
	})
}

func TestVanishedPendingWithoutSuccessorIsRemovedAndDetached(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		f := newVanishFixture(t, conn)
		f.listing(t, record("pending-1", "PENDING", "6772.00", salaryDay), record("other", "POSTED", "-10.00", salaryDay))
		commitment := f.reconcile(t, "pending-1")

		// A cancelled payment: nothing replaces it.
		f.listing(t, record("other", "POSTED", "-10.00", salaryDay))

		if !f.removedAt(t, "pending-1") {
			t.Fatal("pending row was not removed")
		}
		if linked := f.linkedTo(t, commitment); linked != nil {
			t.Errorf("occurrence still linked to %s, want it released", *linked)
		}
	})
}

func TestVanishedPendingIsKeptWhenSuccessorsAreAmbiguous(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		f := newVanishFixture(t, conn)
		category := f.incomeCategory(t)
		f.listing(t, record("pending-1", "PENDING", "100.00", salaryDay))
		if _, err := categories.AssignManual(context.Background(), conn, f.internalID(t, "pending-1"), category); err != nil {
			t.Fatal(err)
		}

		// Two posted rows could each be the replacement: moving the decision
		// onto the wrong one would be worse than dropping it.
		f.listing(t, record("posted-1", "POSTED", "100.00", salaryDay), record("posted-2", "POSTED", "100.00", salaryDay))

		if !f.removedAt(t, "pending-1") {
			t.Fatal("pending row was not removed")
		}
		for _, ext := range []string{"posted-1", "posted-2"} {
			if _, ok := f.manualCategory(t, ext); ok {
				t.Errorf("%s inherited a manual category from an ambiguous match", ext)
			}
		}
	})
}

func TestVanishedPendingKeepsDifferentAmountsApart(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		f := newVanishFixture(t, conn)
		category := f.incomeCategory(t)
		f.listing(t, record("pending-1", "PENDING", "100.00", salaryDay))
		if _, err := categories.AssignManual(context.Background(), conn, f.internalID(t, "pending-1"), category); err != nil {
			t.Fatal(err)
		}

		f.listing(t, record("posted-1", "POSTED", "100.50", salaryDay))

		if _, ok := f.manualCategory(t, "posted-1"); ok {
			t.Error("a posted row with another amount inherited the decision")
		}
	})
}

func TestPostedRowsMissingFromTheListingAreKept(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		f := newVanishFixture(t, conn)
		f.listing(t, record("posted-1", "POSTED", "-30.00", salaryDay), record("posted-2", "POSTED", "-40.00", salaryDay))

		f.listing(t, record("posted-2", "POSTED", "-40.00", salaryDay))

		if f.removedAt(t, "posted-1") {
			t.Error("a posted row the provider stopped reporting was removed")
		}
	})
}

func TestNothingIsRemovedFromAnUntrustedListing(t *testing.T) {
	rejected := "rejected-1"
	cases := map[string]func(*vanishFixture, *testing.T){
		"empty listing": func(f *vanishFixture, t *testing.T) { f.listing(t) },
		"a rejected record": func(f *vanishFixture, t *testing.T) {
			f.page(t, pluggy.TransactionsPage{
				Transactions: []pluggy.TransactionSnapshot{record("other", "POSTED", "-10.00", salaryDay)},
				Rejections: []pluggy.RejectedRecord{{
					EntityType: "transaction", ExternalID: &rejected, Code: "invalid_provider_payload", SafeMessage: "bad record",
				}},
			})
		},
		"no hook": func(f *vanishFixture, t *testing.T) {
			f.removed = nil
			f.listing(t, record("other", "POSTED", "-10.00", salaryDay))
		},
	}
	for name, run := range cases {
		t.Run(name, func(t *testing.T) {
			dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
				f := newVanishFixture(t, conn)
				f.listing(t, record("pending-1", "PENDING", "6772.00", salaryDay), record("other", "POSTED", "-10.00", salaryDay))

				run(f, t)

				if f.removedAt(t, "pending-1") {
					t.Error("pending row was removed on a listing that cannot show what is gone")
				}
			})
		})
	}
}

func TestPendingOlderThanTheListingWindowIsKept(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		f := newVanishFixture(t, conn)
		old := salaryDay.AddDate(0, -3, 0)
		f.listing(t, record("pending-old", "PENDING", "-20.00", old), record("recent", "POSTED", "-10.00", salaryDay))

		// The provider now answers only for the last weeks, so silence about
		// a row dated before them proves nothing.
		f.listing(t, record("recent", "POSTED", "-10.00", salaryDay))

		if f.removedAt(t, "pending-old") {
			t.Error("a pending row dated before the listed window was removed")
		}
	})
}

func TestRemovedPendingComesBackWhenTheProviderReportsItAgain(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		f := newVanishFixture(t, conn)
		f.listing(t, record("pending-1", "PENDING", "6772.00", salaryDay), record("other", "POSTED", "-10.00", salaryDay))
		f.listing(t, record("other", "POSTED", "-10.00", salaryDay))
		if !f.removedAt(t, "pending-1") {
			t.Fatal("setup: pending row was not removed")
		}

		f.listing(t, record("pending-1", "PENDING", "6772.00", salaryDay), record("other", "POSTED", "-10.00", salaryDay))

		if f.removedAt(t, "pending-1") {
			t.Error("a row the provider reports again stayed deleted")
		}
		var rows int
		if err := conn.QueryRow(`SELECT COUNT(*) FROM financial_transactions WHERE external_id = 'pending-1'`).Scan(&rows); err != nil || rows != 1 {
			t.Errorf("rows for pending-1 = %d (%v), want the original one", rows, err)
		}
	})
}
