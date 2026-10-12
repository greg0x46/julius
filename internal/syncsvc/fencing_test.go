package syncsvc_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/pluggy"
	"github.com/greg0x46/julius/internal/syncsvc"
)

func countFailures(t *testing.T, conn *sql.DB, syncRunID string) int {
	t.Helper()
	var n int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM sync_failures WHERE sync_run_id = ?`, syncRunID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestExecuteCompletionIsFencedByOwner(t *testing.T) {
	conn := newTestConn(t)
	ctx := context.Background()
	sourceID, syncRunID := newSyncRun(t, conn)
	claimRun(t, conn, syncRunID, "owner", time.Now())
	provider := &fakeProvider{source: defaultSource()}

	intruder := &syncsvc.Service{DB: conn, Provider: provider, SyncRunID: syncRunID, SourceID: sourceID, WorkerID: "intruder"}
	if err := intruder.Execute(ctx); !errors.Is(err, syncsvc.ErrClaimLost) {
		t.Fatalf("intruder err = %v, want ErrClaimLost", err)
	}
	if status, _ := runStatus(t, conn, syncRunID); status != "in_progress" {
		t.Fatalf("status after intruder = %s, want in_progress", status)
	}

	owner := &syncsvc.Service{DB: conn, Provider: provider, SyncRunID: syncRunID, SourceID: sourceID, WorkerID: "owner"}
	if err := owner.Execute(ctx); err != nil {
		t.Fatalf("owner: %v", err)
	}
	if status, _ := runStatus(t, conn, syncRunID); status != "completed" {
		t.Fatalf("status = %s, want completed", status)
	}

	// A second completion attempt on a finished run is a no-op.
	failing := &fakeProvider{sourceErr: &pluggy.ProviderError{Code: "provider_unavailable", Stage: pluggy.StageItem}}
	again := &syncsvc.Service{DB: conn, Provider: failing, SyncRunID: syncRunID, SourceID: sourceID, WorkerID: "owner"}
	if err := again.Execute(ctx); err != nil {
		t.Fatalf("repeat: %v", err)
	}
	if status, _ := runStatus(t, conn, syncRunID); status != "completed" {
		t.Errorf("status after repeat = %s, want completed", status)
	}
	if n := countFailures(t, conn, syncRunID); n != 0 {
		t.Errorf("sync_failures = %d, want 0", n)
	}
}

func TestExecuteGeneralFailureIsFencedByOwner(t *testing.T) {
	conn := newTestConn(t)
	ctx := context.Background()
	sourceID, syncRunID := newSyncRun(t, conn)
	claimRun(t, conn, syncRunID, "new-owner", time.Now())
	provider := &fakeProvider{sourceErr: &pluggy.ProviderError{Code: "provider_unavailable", Stage: pluggy.StageItem}}

	stale := &syncsvc.Service{DB: conn, Provider: provider, SyncRunID: syncRunID, SourceID: sourceID, WorkerID: "stale-owner"}
	if err := stale.Execute(ctx); !errors.Is(err, syncsvc.ErrClaimLost) {
		t.Fatalf("stale owner err = %v, want ErrClaimLost", err)
	}
	if status, _ := runStatus(t, conn, syncRunID); status != "in_progress" {
		t.Fatalf("status = %s, want in_progress", status)
	}
	if n := countFailures(t, conn, syncRunID); n != 0 {
		t.Fatalf("stale owner left %d sync_failures, want 0", n)
	}

	owner := &syncsvc.Service{DB: conn, Provider: provider, SyncRunID: syncRunID, SourceID: sourceID, WorkerID: "new-owner"}
	if err := owner.Execute(ctx); err != nil {
		t.Fatalf("owner: %v", err)
	}
	if status, code := runStatus(t, conn, syncRunID); status != "failed" || code.String != "provider_unavailable" {
		t.Errorf("status=%s code=%v, want failed/provider_unavailable", status, code)
	}
	if n := countFailures(t, conn, syncRunID); n != 1 {
		t.Errorf("sync_failures = %d, want 1", n)
	}
}

// stealingProvider hands the claim to another worker right before accounts
// are fetched, modelling a lease that expired mid-run.
type stealingProvider struct {
	*fakeProvider
	steal func()
}

func (p *stealingProvider) GetAccounts(ctx context.Context) (pluggy.AccountsPage, error) {
	p.steal()
	return p.fakeProvider.GetAccounts(ctx)
}

func TestExecuteDoesNotCompleteAStolenClaim(t *testing.T) {
	conn := newTestConn(t)
	sourceID, syncRunID := newSyncRun(t, conn)
	claimRun(t, conn, syncRunID, "owner", time.Now())
	provider := &stealingProvider{fakeProvider: &fakeProvider{source: defaultSource()}, steal: func() {
		if _, err := conn.Exec(`UPDATE sync_runs SET worker_id = 'thief' WHERE id = ?`, syncRunID); err != nil {
			t.Error(err)
		}
	}}

	service := &syncsvc.Service{DB: conn, Provider: provider, SyncRunID: syncRunID, SourceID: sourceID, WorkerID: "owner"}
	if err := service.Execute(context.Background()); !errors.Is(err, syncsvc.ErrClaimLost) {
		t.Fatalf("err = %v, want ErrClaimLost", err)
	}
	if status, _ := runStatus(t, conn, syncRunID); status != "in_progress" {
		t.Errorf("status = %s, want in_progress (left to the new owner)", status)
	}
}

func TestStaleClaimCannotOverwriteAccountAfterTakeover(t *testing.T) {
	conn := newTestConn(t)
	sourceID, syncRunID := newSyncRun(t, conn)
	claimRun(t, conn, syncRunID, "owner", time.Now())
	insertRawImport(t, conn, "old-raw", syncRunID, sourceID)
	insertRawImport(t, conn, "stale-raw", syncRunID, sourceID)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := conn.Exec(`INSERT INTO financial_accounts
		(id, source_id, external_id, balance, current_raw_import_id, normalized_hash, created_at, updated_at)
		VALUES ('account-1', ?, 'bank-account', '500.00', 'old-raw', 'old-hash', ?, ?)`, sourceID, now, now); err != nil {
		t.Fatal(err)
	}
	provider := &stealingProvider{fakeProvider: &fakeProvider{
		source: defaultSource(),
		accountsPage: pluggy.AccountsPage{
			RawImportID: "stale-raw",
			Accounts:    []pluggy.AccountSnapshot{{ExternalID: "bank-account", Balance: amountP("100.00")}},
		},
	}, steal: func() {
		if _, err := conn.Exec(`UPDATE sync_runs SET worker_id = 'new-owner' WHERE id = ?`, syncRunID); err != nil {
			t.Error(err)
		}
	}}
	service := &syncsvc.Service{DB: conn, Provider: provider, SyncRunID: syncRunID, SourceID: sourceID, WorkerID: "owner"}
	if err := service.Execute(context.Background()); !errors.Is(err, syncsvc.ErrClaimLost) {
		t.Fatalf("Execute error = %v, want ErrClaimLost", err)
	}
	var balance, rawID string
	if err := conn.QueryRow(`SELECT balance, current_raw_import_id FROM financial_accounts WHERE id = 'account-1'`).Scan(&balance, &rawID); err != nil {
		t.Fatal(err)
	}
	if balance != "500.00" || rawID != "old-raw" {
		t.Fatalf("stale worker changed account balance=%s raw_import=%s", balance, rawID)
	}
}
