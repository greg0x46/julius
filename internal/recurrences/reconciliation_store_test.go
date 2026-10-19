package recurrences_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/recurrences"
)

// seedTransaction inserts the minimum chain of rows a
// financial_transactions row needs (data source, sync run, raw import,
// account), so the FK on scenario_realizations.transaction_id holds.
// The ids are deterministic per connection, so repeat calls only add the
// transaction.
func seedTransaction(t *testing.T, conn *sql.DB, id string) string {
	t.Helper()
	ctx := context.Background()
	now := db.FormatTime(time.Now())
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := conn.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("exec %q: %v", query, err)
		}
	}
	exec(`INSERT OR IGNORE INTO data_sources (id, provider, external_item_id, created_at, updated_at)
		VALUES ('src-1', 'pluggy', 'item-1', ?, ?)`, now, now)
	exec(`INSERT OR IGNORE INTO sync_runs (id, source_id, status, started_at, finished_at)
		VALUES ('run-1', 'src-1', 'completed', ?, ?)`, now, now)
	exec(`INSERT OR IGNORE INTO raw_imports (
			id, sync_run_id, source_id, scope, page_sequence, request_attempt,
			request_method, request_path, http_status, response_headers, payload,
			payload_sha256, received_at
		) VALUES ('raw-1', 'run-1', 'src-1', 'transactions', 1, 1, 'GET', '/x', 200, '{}', x'00', 'sha', ?)`, now)
	exec(`INSERT OR IGNORE INTO financial_accounts (
			id, source_id, external_id, currency_code, current_raw_import_id, normalized_hash, created_at, updated_at
		) VALUES ('acct-1', 'src-1', 'ext-acct-1', 'BRL', 'raw-1', 'hash', ?, ?)`, now, now)
	exec(`INSERT INTO financial_transactions (
			id, source_id, account_id, external_id, description, amount, amount_in_account_currency,
			currency_code, occurred_at, provider_status, movement_type, current_raw_import_id,
			normalized_hash, created_at, updated_at
		) VALUES (?, 'src-1', 'acct-1', ?, 'ALUGUEL', '-1500.00', '-1500.00', 'BRL', ?, 'POSTED', 'DEBIT', 'raw-1', 'hash', ?, ?)`,
		id, "ext-"+id, now, now, now)
	return id
}

func newCommitment(t *testing.T, conn *sql.DB) recurrences.RecurringCommitment {
	t.Helper()
	category := newCategory(t, conn)
	commitment, err := recurrences.Create(context.Background(), conn, testWrite(t, category.ID))
	if err != nil {
		t.Fatalf("recurrences.Create: %v", err)
	}
	return commitment
}

func TestPutOverrideRoundTripsALink(t *testing.T) {
	conn := newTestDB(t)
	ctx := context.Background()
	commitment := newCommitment(t, conn)
	txID := seedTransaction(t, conn, "tx-1")

	stored, err := recurrences.PutOverride(ctx, conn, commitment.ID, date(t, "2026-02-05"), recurrences.StateLinked, &txID)
	if err != nil {
		t.Fatalf("PutOverride: %v", err)
	}
	if stored.State != recurrences.StateLinked || stored.TransactionID == nil || *stored.TransactionID != txID {
		t.Fatalf("unexpected stored override: %+v", stored)
	}

	fetched, found, err := recurrences.GetOverride(ctx, conn, commitment.ID, date(t, "2026-02-05"))
	if err != nil || !found {
		t.Fatalf("GetOverride: found=%v err=%v", found, err)
	}
	if fetched.ID != stored.ID || !fetched.OccurrenceDate.Equal(date(t, "2026-02-05")) {
		t.Errorf("round-tripped override differs: %+v", fetched)
	}
}

// The three UI actions (link, detach, re-link) all write the same row, so a
// second Put must replace rather than collide with the first.
func TestPutOverrideReplacesTheDecisionOnTheSameOccurrence(t *testing.T) {
	conn := newTestDB(t)
	ctx := context.Background()
	commitment := newCommitment(t, conn)
	txID := seedTransaction(t, conn, "tx-1")
	occurrence := date(t, "2026-02-05")

	if _, err := recurrences.PutOverride(ctx, conn, commitment.ID, occurrence, recurrences.StateLinked, &txID); err != nil {
		t.Fatalf("first PutOverride: %v", err)
	}
	if _, err := recurrences.PutOverride(ctx, conn, commitment.ID, occurrence, recurrences.StateDetached, nil); err != nil {
		t.Fatalf("second PutOverride: %v", err)
	}

	fetched, found, err := recurrences.GetOverride(ctx, conn, commitment.ID, occurrence)
	if err != nil || !found {
		t.Fatalf("GetOverride: found=%v err=%v", found, err)
	}
	if fetched.State != recurrences.StateDetached || fetched.TransactionID != nil {
		t.Errorf("expected the detach to replace the link, got %+v", fetched)
	}
	// The transaction it used to hold must be free again.
	linked, err := recurrences.LinkedTransactionIDs(ctx, conn)
	if err != nil {
		t.Fatalf("LinkedTransactionIDs: %v", err)
	}
	if linked[txID] {
		t.Error("the replaced link must release its transaction")
	}
}

func TestPutOverrideRejectsATransactionAnotherOccurrenceClaims(t *testing.T) {
	conn := newTestDB(t)
	ctx := context.Background()
	commitment := newCommitment(t, conn)
	txID := seedTransaction(t, conn, "tx-1")

	if _, err := recurrences.PutOverride(ctx, conn, commitment.ID, date(t, "2026-02-05"), recurrences.StateLinked, &txID); err != nil {
		t.Fatalf("first PutOverride: %v", err)
	}
	_, err := recurrences.PutOverride(ctx, conn, commitment.ID, date(t, "2026-03-05"), recurrences.StateLinked, &txID)
	if !errors.Is(err, recurrences.ErrTransactionAlreadyReconciled) {
		t.Fatalf("err = %v, want ErrTransactionAlreadyReconciled", err)
	}
}

// Detached rows carry no transaction_id, so the UNIQUE on that column must
// not stop a user detaching more than one occurrence.
func TestPutOverrideAllowsManyDetachedOccurrences(t *testing.T) {
	conn := newTestDB(t)
	ctx := context.Background()
	commitment := newCommitment(t, conn)

	for _, day := range []string{"2026-02-05", "2026-03-05", "2026-04-05"} {
		if _, err := recurrences.PutOverride(ctx, conn, commitment.ID, date(t, day), recurrences.StateDetached, nil); err != nil {
			t.Fatalf("PutOverride %s: %v", day, err)
		}
	}
	overrides, err := recurrences.ListOverrides(ctx, conn, []string{commitment.ID}, date(t, "2026-01-01"), date(t, "2026-12-31"))
	if err != nil {
		t.Fatalf("ListOverrides: %v", err)
	}
	if len(overrides[commitment.ID]) != 3 {
		t.Errorf("got %d detached overrides, want 3", len(overrides[commitment.ID]))
	}
}

func TestListOverridesScopesToTheRequestedWindow(t *testing.T) {
	conn := newTestDB(t)
	ctx := context.Background()
	commitment := newCommitment(t, conn)

	for _, day := range []string{"2025-12-05", "2026-02-05", "2026-06-05"} {
		if _, err := recurrences.PutOverride(ctx, conn, commitment.ID, date(t, day), recurrences.StateDetached, nil); err != nil {
			t.Fatalf("PutOverride %s: %v", day, err)
		}
	}
	overrides, err := recurrences.ListOverrides(ctx, conn, []string{commitment.ID}, date(t, "2026-01-01"), date(t, "2026-03-31"))
	if err != nil {
		t.Fatalf("ListOverrides: %v", err)
	}
	if len(overrides[commitment.ID]) != 1 {
		t.Fatalf("got %d overrides in window, want 1", len(overrides[commitment.ID]))
	}
	if got := overrides[commitment.ID][0].OccurrenceDate.Format("2006-01-02"); got != "2026-02-05" {
		t.Errorf("in-window override = %s, want 2026-02-05", got)
	}
}

func TestListOverridesWithNoCommitmentsSkipsTheQuery(t *testing.T) {
	conn := newTestDB(t)
	overrides, err := recurrences.ListOverrides(context.Background(), conn, nil, date(t, "2026-01-01"), date(t, "2026-12-31"))
	if err != nil {
		t.Fatalf("ListOverrides: %v", err)
	}
	if len(overrides) != 0 {
		t.Errorf("got %d entries, want none", len(overrides))
	}
}

func TestDeleteOverrideReportsAMissingDecision(t *testing.T) {
	conn := newTestDB(t)
	ctx := context.Background()
	commitment := newCommitment(t, conn)

	err := recurrences.DeleteOverride(ctx, conn, commitment.ID, date(t, "2026-02-05"))
	if !errors.Is(err, recurrences.ErrOverrideNotFound) {
		t.Fatalf("err = %v, want ErrOverrideNotFound", err)
	}
}

func TestDeleteOverrideHandsTheOccurrenceBackToTheRule(t *testing.T) {
	conn := newTestDB(t)
	ctx := context.Background()
	commitment := newCommitment(t, conn)
	occurrence := date(t, "2026-02-05")

	if _, err := recurrences.PutOverride(ctx, conn, commitment.ID, occurrence, recurrences.StateDetached, nil); err != nil {
		t.Fatalf("PutOverride: %v", err)
	}
	if err := recurrences.DeleteOverride(ctx, conn, commitment.ID, occurrence); err != nil {
		t.Fatalf("DeleteOverride: %v", err)
	}
	if _, found, err := recurrences.GetOverride(ctx, conn, commitment.ID, occurrence); err != nil || found {
		t.Errorf("override survived the delete: found=%v err=%v", found, err)
	}
}

func TestOverrideForTransactionFindsTheOccurrenceItSettles(t *testing.T) {
	conn := newTestDB(t)
	ctx := context.Background()
	commitment := newCommitment(t, conn)
	txID := seedTransaction(t, conn, "tx-1")

	if _, err := recurrences.PutOverride(ctx, conn, commitment.ID, date(t, "2026-02-05"), recurrences.StateLinked, &txID); err != nil {
		t.Fatalf("PutOverride: %v", err)
	}
	found, ok, err := recurrences.OverrideForTransaction(ctx, conn, txID)
	if err != nil || !ok {
		t.Fatalf("OverrideForTransaction: ok=%v err=%v", ok, err)
	}
	if found.ScenarioID != commitment.ID {
		t.Errorf("scenario = %s, want %s", found.ScenarioID, commitment.ID)
	}
}

// A transaction the user excludes from the totals can't be what carries a
// commitment's money — see ledger.OnIgnored.
func TestUnlinkIfPresentDropsOnlyTheLink(t *testing.T) {
	conn := newTestDB(t)
	ctx := context.Background()
	commitment := newCommitment(t, conn)
	txID := seedTransaction(t, conn, "tx-1")

	if _, err := recurrences.PutOverride(ctx, conn, commitment.ID, date(t, "2026-02-05"), recurrences.StateLinked, &txID); err != nil {
		t.Fatalf("PutOverride link: %v", err)
	}
	if _, err := recurrences.PutOverride(ctx, conn, commitment.ID, date(t, "2026-03-05"), recurrences.StateDetached, nil); err != nil {
		t.Fatalf("PutOverride detach: %v", err)
	}
	if err := recurrences.UnlinkIfPresent(ctx, conn, txID); err != nil {
		t.Fatalf("UnlinkIfPresent: %v", err)
	}

	if _, found, _ := recurrences.GetOverride(ctx, conn, commitment.ID, date(t, "2026-02-05")); found {
		t.Error("the link should be gone")
	}
	if _, found, _ := recurrences.GetOverride(ctx, conn, commitment.ID, date(t, "2026-03-05")); !found {
		t.Error("an unrelated detached decision must survive")
	}
}

// The override is an annotation on the commitment; deleting the commitment
// takes it along (ON DELETE CASCADE), unlike a payable link's RESTRICT.
func TestDeletingACommitmentCascadesItsOverrides(t *testing.T) {
	conn := newTestDB(t)
	ctx := context.Background()
	commitment := newCommitment(t, conn)

	scenarioID := commitment.ID
	if _, err := recurrences.PutOverride(ctx, conn, scenarioID, date(t, "2026-02-05"), recurrences.StateDetached, nil); err != nil {
		t.Fatalf("PutOverride: %v", err)
	}
	if err := recurrences.Delete(ctx, conn, commitment.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	var remaining int
	if err := conn.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM scenario_realizations WHERE relation_type = 'reconciliation' AND scenario_id = ?`, scenarioID,
	).Scan(&remaining); err != nil {
		t.Fatalf("count: %v", err)
	}
	if remaining != 0 {
		t.Errorf("%d overrides survived the cascade", remaining)
	}
}
