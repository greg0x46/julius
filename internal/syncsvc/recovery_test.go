package syncsvc_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/syncsvc"
)

const testLease = time.Minute

func claimRun(t *testing.T, conn *sql.DB, syncRunID, workerID string, heartbeatAt time.Time) {
	t.Helper()
	if _, err := conn.Exec(`UPDATE sync_runs SET worker_id = ?, heartbeat_at = ? WHERE id = ?`,
		workerID, db.FormatTime(heartbeatAt), syncRunID); err != nil {
		t.Fatalf("claim run: %v", err)
	}
}

func runStatus(t *testing.T, conn *sql.DB, syncRunID string) (status string, generalCode sql.NullString) {
	t.Helper()
	if err := conn.QueryRow(`SELECT status, general_error_code FROM sync_runs WHERE id = ?`, syncRunID).Scan(&status, &generalCode); err != nil {
		t.Fatalf("query sync_run: %v", err)
	}
	return status, generalCode
}

func TestRecoverStaleRunsFailsClaimsWithExpiredLease(t *testing.T) {
	conn := newTestConn(t)
	_, syncRunID := newSyncRun(t, conn)
	claimRun(t, conn, syncRunID, "dead-worker", time.Now().Add(-2*testLease))

	recovered, err := syncsvc.RecoverStaleRuns(context.Background(), conn, time.Now(), testLease)
	if err != nil {
		t.Fatalf("RecoverStaleRuns: %v", err)
	}
	if len(recovered) != 1 || recovered[0] != syncRunID {
		t.Fatalf("recovered = %v, want [%s]", recovered, syncRunID)
	}
	if status, code := runStatus(t, conn, syncRunID); status != "failed" || code.String != "interrupted" {
		t.Errorf("status=%s generalCode=%v, want failed/interrupted", status, code)
	}

	var failureCount int
	conn.QueryRow(`SELECT COUNT(*) FROM sync_failures WHERE sync_run_id = ? AND stage = 'interrupted'`, syncRunID).Scan(&failureCount)
	if failureCount != 1 {
		t.Errorf("failureCount = %d, want 1", failureCount)
	}

	again, err := syncsvc.RecoverStaleRuns(context.Background(), conn, time.Now(), testLease)
	if err != nil || len(again) != 0 {
		t.Fatalf("second recovery = %v err=%v, want none", again, err)
	}
}

func TestRecoverStaleRunsLeavesFreshClaimsAlone(t *testing.T) {
	conn := newTestConn(t)
	_, syncRunID := newSyncRun(t, conn)
	claimRun(t, conn, syncRunID, "live-worker", time.Now())

	recovered, err := syncsvc.RecoverStaleRuns(context.Background(), conn, time.Now(), testLease)
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered) != 0 {
		t.Fatalf("recovered = %v, want none", recovered)
	}
	if status, _ := runStatus(t, conn, syncRunID); status != "in_progress" {
		t.Errorf("status = %s, want in_progress", status)
	}
}

func TestRecoverStaleRunsLeavesUnclaimedRunsPending(t *testing.T) {
	conn := newTestConn(t)
	_, syncRunID := newSyncRun(t, conn)
	if _, err := conn.Exec(`UPDATE sync_runs SET started_at = ? WHERE id = ?`,
		db.FormatTime(time.Now().Add(-24*time.Hour)), syncRunID); err != nil {
		t.Fatal(err)
	}

	recovered, err := syncsvc.RecoverStaleRuns(context.Background(), conn, time.Now(), testLease)
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered) != 0 {
		t.Fatalf("recovered = %v, want none", recovered)
	}
	if status, _ := runStatus(t, conn, syncRunID); status != "in_progress" {
		t.Errorf("status = %s, want in_progress (still claimable)", status)
	}
}

func TestRecoverStaleRunsFallsBackToStartedAt(t *testing.T) {
	conn := newTestConn(t)
	_, syncRunID := newSyncRun(t, conn)
	if _, err := conn.Exec(`UPDATE sync_runs SET worker_id = 'dead-worker', heartbeat_at = NULL, started_at = ? WHERE id = ?`,
		db.FormatTime(time.Now().Add(-2*testLease)), syncRunID); err != nil {
		t.Fatal(err)
	}
	recovered, err := syncsvc.RecoverStaleRuns(context.Background(), conn, time.Now(), testLease)
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered) != 1 {
		t.Fatalf("recovered = %v, want the run", recovered)
	}
}

func TestRecoverStaleRunsLeavesFileImportAlone(t *testing.T) {
	conn := newTestConn(t)
	old := db.FormatTime(time.Now().Add(-2 * testLease))
	if _, err := conn.Exec(`INSERT INTO data_sources (id, provider, external_item_id, created_at, updated_at)
		VALUES ('file-source', 'file', 'local-1', ?, ?)`, old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO sync_runs (id, source_id, run_type, status, started_at, worker_id, heartbeat_at)
		VALUES ('file-run', 'file-source', 'file_import', 'in_progress', ?, 'someone', ?)`, old, old); err != nil {
		t.Fatal(err)
	}
	recovered, err := syncsvc.RecoverStaleRuns(context.Background(), conn, time.Now(), testLease)
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered) != 0 {
		t.Fatalf("recovered = %v, want none", recovered)
	}
	if status, _ := runStatus(t, conn, "file-run"); status != "in_progress" {
		t.Fatalf("status = %s, want in_progress", status)
	}
}

func TestRecoverStaleRunsLeavesCompletedRunsAlone(t *testing.T) {
	conn := newTestConn(t)
	_, syncRunID := newSyncRun(t, conn)
	claimRun(t, conn, syncRunID, "dead-worker", time.Now().Add(-2*testLease))
	if _, err := conn.Exec(`UPDATE sync_runs SET status = 'completed', finished_at = ? WHERE id = ?`,
		db.FormatTime(time.Now()), syncRunID); err != nil {
		t.Fatalf("update sync_run: %v", err)
	}

	recovered, err := syncsvc.RecoverStaleRuns(context.Background(), conn, time.Now(), testLease)
	if err != nil {
		t.Fatalf("RecoverStaleRuns: %v", err)
	}
	if len(recovered) != 0 {
		t.Errorf("recovered = %v, want none", recovered)
	}
	if status, _ := runStatus(t, conn, syncRunID); status != "completed" {
		t.Errorf("status = %s, want completed (untouched)", status)
	}
}

func TestRecoverStaleRunsHandlesMultipleRuns(t *testing.T) {
	conn := newTestConn(t)
	now := db.FormatTime(time.Now())
	stale := time.Now().Add(-2 * testLease)

	// A partial unique index only allows one 'in_progress' run per source,
	// so this exercises recovery across two distinct sources instead.
	var runs []string
	for _, item := range []string{"item-1", "item-2"} {
		sourceID, runID := uuid.NewString(), uuid.NewString()
		if _, err := conn.Exec(`INSERT INTO data_sources (id, provider, external_item_id, created_at, updated_at)
			VALUES (?, 'pluggy', ?, ?, ?)`, sourceID, item, now, now); err != nil {
			t.Fatalf("insert data_source: %v", err)
		}
		if _, err := conn.Exec(`INSERT INTO sync_runs (id, source_id, status, started_at) VALUES (?, ?, 'in_progress', ?)`,
			runID, sourceID, now); err != nil {
			t.Fatalf("insert run: %v", err)
		}
		claimRun(t, conn, runID, "dead-worker", stale)
		runs = append(runs, runID)
	}

	recovered, err := syncsvc.RecoverStaleRuns(context.Background(), conn, time.Now(), testLease)
	if err != nil {
		t.Fatalf("RecoverStaleRuns: %v", err)
	}
	if len(recovered) != len(runs) {
		t.Fatalf("recovered = %v, want %v", recovered, runs)
	}
}

func TestFailRunIsFencedByOwner(t *testing.T) {
	conn := newTestConn(t)
	ctx := context.Background()
	_, syncRunID := newSyncRun(t, conn)
	claimRun(t, conn, syncRunID, "owner", time.Now())

	if err := syncsvc.FailRun(ctx, conn, syncRunID, "intruder", "connection_inactive"); err != syncsvc.ErrClaimLost {
		t.Fatalf("non-owner err = %v, want ErrClaimLost", err)
	}
	if status, _ := runStatus(t, conn, syncRunID); status != "in_progress" {
		t.Fatalf("status = %s, want in_progress", status)
	}
	if err := syncsvc.FailRun(ctx, conn, syncRunID, "owner", "connection_inactive"); err != nil {
		t.Fatalf("owner: %v", err)
	}
	if err := syncsvc.FailRun(ctx, conn, syncRunID, "owner", "internal_error"); err != nil {
		t.Fatalf("repeat on a finished run: %v", err)
	}
	if status, code := runStatus(t, conn, syncRunID); status != "failed" || code.String != "connection_inactive" {
		t.Errorf("status=%s code=%v, want failed/connection_inactive", status, code)
	}
	var failures int
	conn.QueryRow(`SELECT COUNT(*) FROM sync_failures WHERE sync_run_id = ?`, syncRunID).Scan(&failures)
	if failures != 1 {
		t.Errorf("sync_failures = %d, want 1", failures)
	}
}
