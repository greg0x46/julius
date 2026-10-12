package worker_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/greg0x46/julius/internal/auth"
	"github.com/greg0x46/julius/internal/datasources"
	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/pluggy"
	"github.com/greg0x46/julius/internal/settings"
	"github.com/greg0x46/julius/internal/syncsvc"
	"github.com/greg0x46/julius/internal/worker"
)

const connEmail = "owner@example.com"

func newTestConn(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func insertSourceAndRun(t *testing.T, conn *sql.DB, itemID, status string, startedAt time.Time) (sourceID, runID string) {
	t.Helper()
	sourceID, runID = uuid.NewString(), uuid.NewString()
	now := db.FormatTime(time.Now())
	if _, err := conn.Exec(`INSERT INTO data_sources (id, provider, external_item_id, created_at, updated_at)
		VALUES (?, 'pluggy', ?, ?, ?)`, sourceID, itemID, now, now); err != nil {
		t.Fatalf("insert data_source: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO sync_runs (id, source_id, status, started_at) VALUES (?, ?, ?, ?)`,
		runID, sourceID, status, db.FormatTime(startedAt)); err != nil {
		t.Fatalf("insert sync_run: %v", err)
	}
	return sourceID, runID
}

func TestClaimNextRunPicksOldestUnclaimed(t *testing.T) {
	conn := newTestConn(t)
	_, older := insertSourceAndRun(t, conn, "item-1", "in_progress", time.Now().Add(-time.Hour))
	_, _ = insertSourceAndRun(t, conn, "item-2", "in_progress", time.Now())

	runID, _, ok, err := worker.ClaimNextRun(context.Background(), conn, "worker-1")
	if err != nil {
		t.Fatalf("ClaimNextRun: %v", err)
	}
	if !ok || runID != older {
		t.Errorf("runID = %s, ok=%v, want %s/true", runID, ok, older)
	}

	var workerID sql.NullString
	conn.QueryRow(`SELECT worker_id FROM sync_runs WHERE id = ?`, runID).Scan(&workerID)
	if !workerID.Valid || workerID.String != "worker-1" {
		t.Errorf("worker_id = %v, want worker-1", workerID)
	}
}

func TestClaimNextRunSkipsFileImports(t *testing.T) {
	conn := newTestConn(t)
	ctx := context.Background()
	source, err := datasources.Create(ctx, conn, datasources.ProviderFile, "local-account-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO sync_runs (id, source_id, status, run_type, started_at)
		VALUES ('file-run', ?, 'in_progress', 'file_import', ?)`, source.ID, db.FormatTime(time.Now().Add(-time.Hour))); err != nil {
		t.Fatal(err)
	}
	_, pluggyRun := insertSourceAndRun(t, conn, "item-1", "in_progress", time.Now())
	runID, _, ok, err := worker.ClaimNextRun(ctx, conn, "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || runID != pluggyRun {
		t.Fatalf("claimed %q, ok=%v, want Pluggy run %q", runID, ok, pluggyRun)
	}
}

func TestClaimNextRunSkipsAlreadyClaimedRuns(t *testing.T) {
	conn := newTestConn(t)
	_, runID := insertSourceAndRun(t, conn, "item-1", "in_progress", time.Now())
	if _, err := conn.Exec(`UPDATE sync_runs SET worker_id = 'other-worker' WHERE id = ?`, runID); err != nil {
		t.Fatalf("update: %v", err)
	}

	_, _, ok, err := worker.ClaimNextRun(context.Background(), conn, "worker-1")
	if err != nil {
		t.Fatalf("ClaimNextRun: %v", err)
	}
	if ok {
		t.Error("should not claim a run another worker already holds")
	}
}

func TestClaimNextRunNoneAvailable(t *testing.T) {
	conn := newTestConn(t)
	_, _, ok, err := worker.ClaimNextRun(context.Background(), conn, "worker-1")
	if err != nil {
		t.Fatalf("ClaimNextRun: %v", err)
	}
	if ok {
		t.Error("expected no run to claim on an empty database")
	}
}

// TestProcessClaimUsesTheRunsOwnConnection pins the rule that makes several
// connections safe: the item to fetch comes from the run's source_id, not from
// global config. Reading a single configured item id here would have synced
// one bank's data into another bank's run.
func TestProcessClaimUsesTheRunsOwnConnection(t *testing.T) {
	conn := newTestConn(t)
	insertSourceAndRun(t, conn, "item-first", "in_progress", time.Now().Add(-time.Hour))
	sourceID, runID := insertSourceAndRun(t, conn, "item-second", "in_progress", time.Now())

	var requested []string
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/auth" {
			_, _ = w.Write([]byte(`{"apiKey":"test-key"}`))
			return
		}
		// Anything past auth can fail: this test is about which item was
		// asked for, and the run failing cleanly is a valid outcome.
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer provider.Close()

	ctx := context.Background()
	key := bytes.Repeat([]byte{7}, 32)
	err := auth.NewStore(conn).Initialize(ctx, connEmail, "correct horse battery staple", key, nil)
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	for name, value := range map[string]string{"pluggy.client_id": "cid", "pluggy.client_secret": "csecret"} {
		if err := settings.Set(ctx, conn, name, value, true, key); err != nil {
			t.Fatalf("Set %s: %v", name, err)
		}
	}
	session := settings.NewSecrets(key)

	cfg := worker.Config{Pluggy: pluggy.DefaultConfig()}
	cfg.Pluggy.BaseURL = provider.URL
	cfg.Pluggy.MaxAttempts = 1
	_ = worker.ProcessClaim(ctx, conn, session, cfg, "worker-1", runID, sourceID)

	var asked bool
	for _, uri := range requested {
		if strings.Contains(uri, "item-first") {
			t.Fatalf("asked the provider for another connection's item: %v", requested)
		}
		if uri == "/items/item-second" {
			asked = true
		}
	}
	if !asked {
		t.Errorf("never asked for the run's own item: %v", requested)
	}
}

// TestProcessClaimRejectsAnUnknownConnection guards the other direction: a run
// whose connection vanished must fail loudly instead of syncing whatever item
// happened to be configured.
func TestProcessClaimRejectsAnUnknownConnection(t *testing.T) {
	conn := newTestConn(t)
	_, runID := insertSourceAndRun(t, conn, "item-1", "in_progress", time.Now())

	ctx := context.Background()
	key := bytes.Repeat([]byte{7}, 32)
	err := auth.NewStore(conn).Initialize(ctx, connEmail, "correct horse battery staple", key, nil)
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	for name, value := range map[string]string{"pluggy.client_id": "cid", "pluggy.client_secret": "csecret"} {
		if err := settings.Set(ctx, conn, name, value, true, key); err != nil {
			t.Fatalf("Set %s: %v", name, err)
		}
	}
	session := settings.NewSecrets(key)

	err = worker.ProcessClaim(ctx, conn, session, worker.Config{Pluggy: pluggy.DefaultConfig()},
		"worker-1", runID, "does-not-exist")
	if !errors.Is(err, datasources.ErrNotFound) {
		t.Errorf("err = %v, want datasources.ErrNotFound", err)
	}
}

func setPluggyCredentials(t *testing.T, conn *sql.DB) *settings.Secrets {
	t.Helper()
	ctx := context.Background()
	key := bytes.Repeat([]byte{7}, 32)
	if err := auth.NewStore(conn).Initialize(ctx, connEmail, "correct horse battery staple", key, nil); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	for name, value := range map[string]string{"pluggy.client_id": "cid", "pluggy.client_secret": "csecret"} {
		if err := settings.Set(ctx, conn, name, value, true, key); err != nil {
			t.Fatalf("Set %s: %v", name, err)
		}
	}
	return settings.NewSecrets(key)
}

func TestClaimNextRunIsAtomicAcrossRepeatedClaims(t *testing.T) {
	conn := newTestConn(t)
	_, runID := insertSourceAndRun(t, conn, "item-1", "in_progress", time.Now())
	ctx := context.Background()

	first, _, ok, err := worker.ClaimNextRun(ctx, conn, "worker-1")
	if err != nil || !ok || first != runID {
		t.Fatalf("first claim = %q ok=%v err=%v, want %q", first, ok, err, runID)
	}
	_, _, ok, err = worker.ClaimNextRun(ctx, conn, "worker-2")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("a claimed run was handed out twice")
	}
	var owner string
	var heartbeatAt sql.NullString
	if err := conn.QueryRow(`SELECT worker_id, heartbeat_at FROM sync_runs WHERE id = ?`, runID).Scan(&owner, &heartbeatAt); err != nil {
		t.Fatal(err)
	}
	if owner != "worker-1" || !heartbeatAt.Valid {
		t.Errorf("worker_id=%q heartbeat_at=%v, want worker-1 and a heartbeat", owner, heartbeatAt)
	}
}

func TestProcessClaimFailsInactiveConnectionOnlyForTheOwner(t *testing.T) {
	conn := newTestConn(t)
	sourceID, runID := insertSourceAndRun(t, conn, "item-1", "in_progress", time.Now())
	if _, err := conn.Exec(`UPDATE data_sources SET is_active = 0 WHERE id = ?`, sourceID); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`UPDATE sync_runs SET worker_id = 'owner' WHERE id = ?`, runID); err != nil {
		t.Fatal(err)
	}
	session := setPluggyCredentials(t, conn)
	ctx := context.Background()
	cfg := worker.Config{Pluggy: pluggy.DefaultConfig()}

	if err := worker.ProcessClaim(ctx, conn, session, cfg, "intruder", runID, sourceID); !errors.Is(err, syncsvc.ErrClaimLost) {
		t.Fatalf("non-owner err = %v, want ErrClaimLost", err)
	}
	var status string
	conn.QueryRow(`SELECT status FROM sync_runs WHERE id = ?`, runID).Scan(&status)
	if status != "in_progress" {
		t.Fatalf("status after non-owner = %s, want in_progress", status)
	}

	if err := worker.ProcessClaim(ctx, conn, session, cfg, "owner", runID, sourceID); err != nil {
		t.Fatalf("owner: %v", err)
	}
	var code string
	conn.QueryRow(`SELECT status, general_error_code FROM sync_runs WHERE id = ?`, runID).Scan(&status, &code)
	if status != "failed" || code != "connection_inactive" {
		t.Errorf("status=%s code=%s, want failed/connection_inactive", status, code)
	}
	var failures int
	conn.QueryRow(`SELECT COUNT(*) FROM sync_failures WHERE sync_run_id = ?`, runID).Scan(&failures)
	if failures != 1 {
		t.Errorf("sync_failures = %d, want 1 (none from the non-owner)", failures)
	}
}

// TestRunClaimsAndHeartbeatsPendingRun drives the whole loop on SQLite: a
// pending run gets claimed with worker_id/heartbeat_at and finished by its
// owner, with no external queue involved.
func TestRunClaimsAndHeartbeatsPendingRun(t *testing.T) {
	conn := newTestConn(t)
	_, runID := insertSourceAndRun(t, conn, "item-1", "in_progress", time.Now())
	session := setPluggyCredentials(t, conn)

	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/auth" {
			_, _ = w.Write([]byte(`{"apiKey":"test-key"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer provider.Close()

	cfg := worker.Config{
		PollInterval: 10 * time.Millisecond, HeartbeatInterval: 10 * time.Millisecond,
		LeaseTimeout: time.Minute, Pluggy: pluggy.DefaultConfig(),
	}
	cfg.Pluggy.BaseURL = provider.URL
	cfg.Pluggy.MaxAttempts = 1

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		worker.Run(ctx, conn, session, cfg)
		close(done)
	}()
	defer func() {
		cancel()
		<-done
	}()

	deadline := time.Now().Add(10 * time.Second)
	for {
		var status string
		var owner, heartbeatAt sql.NullString
		if err := conn.QueryRow(`SELECT status, worker_id, heartbeat_at FROM sync_runs WHERE id = ?`, runID).
			Scan(&status, &owner, &heartbeatAt); err != nil {
			t.Fatal(err)
		}
		if status != "in_progress" {
			if !owner.Valid || owner.String == "" || !heartbeatAt.Valid {
				t.Fatalf("finished run has worker_id=%v heartbeat_at=%v, want both set", owner, heartbeatAt)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("run still %s (worker_id=%v) after deadline", status, owner)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRunRecoversStaleClaimWhileProcessingAnotherRun(t *testing.T) {
	conn := newTestConn(t)
	_, activeRunID := insertSourceAndRun(t, conn, "item-active", "in_progress", time.Now())
	session := setPluggyCredentials(t, conn)

	entered := make(chan struct{})
	release := make(chan struct{})
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/auth" {
			_, _ = w.Write([]byte(`{"apiKey":"test-key"}`))
			return
		}
		if r.URL.Path == "/items/item-active" {
			close(entered)
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer provider.Close()
	defer close(release)

	cfg := worker.Config{
		PollInterval: 10 * time.Millisecond, HeartbeatInterval: 10 * time.Millisecond,
		LeaseTimeout: 100 * time.Millisecond, Pluggy: pluggy.DefaultConfig(),
	}
	cfg.Pluggy.BaseURL = provider.URL
	cfg.Pluggy.MaxAttempts = 1
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		worker.Run(ctx, conn, session, cfg)
		close(done)
	}()
	defer func() {
		cancel()
		<-done
	}()

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("active run never reached the provider")
	}

	_, staleRunID := insertSourceAndRun(t, conn, "item-stale", "in_progress", time.Now().Add(-time.Hour))
	if _, err := conn.Exec(`UPDATE sync_runs SET worker_id = 'dead-worker', heartbeat_at = ? WHERE id = ?`,
		db.FormatTime(time.Now().Add(-time.Hour)), staleRunID); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		var staleStatus, activeStatus string
		if err := conn.QueryRow(`SELECT status FROM sync_runs WHERE id = ?`, staleRunID).Scan(&staleStatus); err != nil {
			t.Fatal(err)
		}
		if err := conn.QueryRow(`SELECT status FROM sync_runs WHERE id = ?`, activeRunID).Scan(&activeStatus); err != nil {
			t.Fatal(err)
		}
		if staleStatus == "failed" {
			if activeStatus != "in_progress" {
				t.Fatalf("active run status = %s, want in_progress", activeStatus)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("stale run remained %s while another run was processing", staleStatus)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
