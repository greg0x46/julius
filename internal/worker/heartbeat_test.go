package worker

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/db"
)

func TestHeartbeatAdvancesAndCancelsWhenTheClaimIsLost(t *testing.T) {
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	old := db.FormatTime(time.Now().Add(-time.Hour))
	if _, err := conn.Exec(`INSERT INTO data_sources (id, provider, external_item_id, created_at, updated_at)
		VALUES ('source-1', 'pluggy', 'item-1', ?, ?)`, old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO sync_runs (id, source_id, status, started_at, worker_id, heartbeat_at)
		VALUES ('run-1', 'source-1', 'in_progress', ?, 'owner', ?)`, old, old); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	lost := make(chan struct{})
	go heartbeat(ctx, conn, "run-1", "owner", 10*time.Millisecond, func() { close(lost) })

	deadline := time.Now().Add(5 * time.Second)
	for {
		var heartbeatAt string
		if err := conn.QueryRow(`SELECT heartbeat_at FROM sync_runs WHERE id = 'run-1'`).Scan(&heartbeatAt); err != nil {
			t.Fatal(err)
		}
		if heartbeatAt > old {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("heartbeat_at never advanced")
		}
		time.Sleep(5 * time.Millisecond)
	}

	if _, err := conn.Exec(`UPDATE sync_runs SET worker_id = 'thief' WHERE id = 'run-1'`); err != nil {
		t.Fatal(err)
	}
	select {
	case <-lost:
	case <-time.After(5 * time.Second):
		t.Fatal("onLost not called after the claim moved to another worker")
	}
}
