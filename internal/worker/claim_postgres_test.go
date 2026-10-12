package worker_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/syncsvc"
	"github.com/greg0x46/julius/internal/worker"
)

// newPostgresInstances opens two independent pools on one freshly migrated,
// private schema of the Postgres at JULIUS_TEST_POSTGRES_DSN, modelling two
// Julius instances sharing a database. It skips without that variable.
func newPostgresInstances(t *testing.T) (a, b *sql.DB) {
	t.Helper()
	base := os.Getenv("JULIUS_TEST_POSTGRES_DSN")
	if base == "" {
		t.Skip("JULIUS_TEST_POSTGRES_DSN not set")
	}
	raw, err := sql.Open("pgx", base)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { raw.Close() })
	schema := "worker_claim_" + fmt.Sprintf("%x", uuid.New())
	if _, err := raw.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { raw.Exec(`DROP SCHEMA ` + schema + ` CASCADE`) })

	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	open := func() *sql.DB {
		conn, err := db.Open(u.String())
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		t.Cleanup(func() { conn.Close() })
		return conn
	}
	return open(), open()
}

func TestPostgresConcurrentClaimExactlyOneWinner(t *testing.T) {
	a, b := newPostgresInstances(t)
	_, runID := insertSourceAndRun(t, a, "item-1", "in_progress", time.Now())

	const claimants = 8
	start := make(chan struct{})
	winners := make(chan string, claimants)
	var wg sync.WaitGroup
	for i := 0; i < claimants; i++ {
		conn := a
		if i%2 == 1 {
			conn = b
		}
		workerID := fmt.Sprintf("worker-%d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			claimed, _, ok, err := worker.ClaimNextRun(context.Background(), conn, workerID)
			if err != nil {
				t.Errorf("%s: %v", workerID, err)
				return
			}
			if ok {
				if claimed != runID {
					t.Errorf("%s claimed %s, want %s", workerID, claimed, runID)
				}
				winners <- workerID
			}
		}()
	}
	close(start)
	wg.Wait()
	close(winners)

	var won []string
	for w := range winners {
		won = append(won, w)
	}
	if len(won) != 1 {
		t.Fatalf("winners = %v, want exactly one", won)
	}
	var owner string
	if err := b.QueryRow(`SELECT worker_id FROM sync_runs WHERE id = ?`, runID).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if owner != won[0] {
		t.Errorf("worker_id = %s, want winner %s", owner, won[0])
	}
}

func TestPostgresConcurrentClaimsOfManyRunsAreDisjoint(t *testing.T) {
	a, b := newPostgresInstances(t)
	const runs = 20
	want := map[string]bool{}
	base := time.Now().Add(-time.Hour)
	for i := 0; i < runs; i++ {
		_, runID := insertSourceAndRun(t, a, fmt.Sprintf("item-%d", i), "in_progress", base.Add(time.Duration(i)*time.Second))
		want[runID] = true
	}

	var mu sync.Mutex
	claimedBy := map[string][]string{}
	total := 0
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i, conn := range []*sql.DB{a, b} {
		workerID := fmt.Sprintf("worker-%d", i)
		wg.Add(1)
		go func(conn *sql.DB) {
			defer wg.Done()
			<-start
			deadline := time.Now().Add(20 * time.Second)
			for time.Now().Before(deadline) {
				mu.Lock()
				done := total >= runs
				mu.Unlock()
				if done {
					return
				}
				runID, _, ok, err := worker.ClaimNextRun(context.Background(), conn, workerID)
				if err != nil {
					t.Errorf("%s: %v", workerID, err)
					return
				}
				if ok {
					mu.Lock()
					claimedBy[runID] = append(claimedBy[runID], workerID)
					total++
					mu.Unlock()
				}
			}
		}(conn)
	}
	close(start)
	wg.Wait()

	if len(claimedBy) != runs {
		t.Fatalf("claimed %d distinct runs, want %d", len(claimedBy), runs)
	}
	for runID, owners := range claimedBy {
		if !want[runID] {
			t.Errorf("claimed unknown run %s", runID)
		}
		if len(owners) != 1 {
			t.Errorf("run %s claimed by %v, want exactly one worker", runID, owners)
		}
	}
}

func TestPostgresOnlyOwnerFinishesTheRun(t *testing.T) {
	a, b := newPostgresInstances(t)
	ctx := context.Background()
	_, runID := insertSourceAndRun(t, a, "item-1", "in_progress", time.Now())
	if _, _, ok, err := worker.ClaimNextRun(ctx, a, "owner"); err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}

	if err := syncsvc.FailRun(ctx, b, runID, "intruder", "connection_inactive"); !errors.Is(err, syncsvc.ErrClaimLost) {
		t.Fatalf("intruder err = %v, want ErrClaimLost", err)
	}
	var status string
	b.QueryRow(`SELECT status FROM sync_runs WHERE id = ?`, runID).Scan(&status)
	if status != "in_progress" {
		t.Fatalf("status = %s, want in_progress", status)
	}
	if err := syncsvc.FailRun(ctx, a, runID, "owner", "connection_inactive"); err != nil {
		t.Fatalf("owner: %v", err)
	}
	if err := syncsvc.FailRun(ctx, b, runID, "owner", "internal_error"); err != nil {
		t.Fatalf("repeat on a finished run: %v", err)
	}
	var code string
	var failures int
	a.QueryRow(`SELECT status, general_error_code FROM sync_runs WHERE id = ?`, runID).Scan(&status, &code)
	a.QueryRow(`SELECT COUNT(*) FROM sync_failures WHERE sync_run_id = ?`, runID).Scan(&failures)
	if status != "failed" || code != "connection_inactive" || failures != 1 {
		t.Errorf("status=%s code=%s failures=%d, want failed/connection_inactive/1", status, code, failures)
	}
}

func TestPostgresConcurrentRecoveryFailsEachStaleRunOnce(t *testing.T) {
	a, b := newPostgresInstances(t)
	ctx := context.Background()
	const runs = 10
	stale := db.FormatTime(time.Now().Add(-time.Hour))
	var ids []string
	for i := 0; i < runs; i++ {
		_, runID := insertSourceAndRun(t, a, fmt.Sprintf("item-%d", i), "in_progress", time.Now().Add(-2*time.Hour))
		if _, err := a.Exec(`UPDATE sync_runs SET worker_id = 'dead', heartbeat_at = ? WHERE id = ?`, stale, runID); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, runID)
	}

	start := make(chan struct{})
	results := make([][]string, 2)
	var wg sync.WaitGroup
	for i, conn := range []*sql.DB{a, b} {
		wg.Add(1)
		go func(i int, conn *sql.DB) {
			defer wg.Done()
			<-start
			recovered, err := syncsvc.RecoverStaleRuns(ctx, conn, time.Now(), time.Minute)
			if err != nil {
				t.Errorf("recoverer %d: %v", i, err)
			}
			results[i] = recovered
		}(i, conn)
	}
	close(start)
	wg.Wait()

	seen := map[string]int{}
	for _, recovered := range results {
		for _, id := range recovered {
			seen[id]++
		}
	}
	for _, id := range ids {
		if seen[id] != 1 {
			t.Errorf("run %s returned by %d recoverers, want 1", id, seen[id])
		}
		var status, code string
		var failures int
		a.QueryRow(`SELECT status, general_error_code FROM sync_runs WHERE id = ?`, id).Scan(&status, &code)
		a.QueryRow(`SELECT COUNT(*) FROM sync_failures WHERE sync_run_id = ? AND stage = 'interrupted'`, id).Scan(&failures)
		if status != "failed" || code != "interrupted" || failures != 1 {
			t.Errorf("run %s: status=%s code=%s failures=%d, want failed/interrupted/1", id, status, code, failures)
		}
	}
}

func TestPostgresStaleOwnerCannotFinishARecoveredRun(t *testing.T) {
	a, b := newPostgresInstances(t)
	ctx := context.Background()
	_, runID := insertSourceAndRun(t, a, "item-1", "in_progress", time.Now().Add(-2*time.Hour))
	if _, _, ok, err := worker.ClaimNextRun(ctx, a, "zombie"); err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	if _, err := a.Exec(`UPDATE sync_runs SET heartbeat_at = ? WHERE id = ?`, db.FormatTime(time.Now().Add(-time.Hour)), runID); err != nil {
		t.Fatal(err)
	}
	recovered, err := syncsvc.RecoverStaleRuns(ctx, b, time.Now(), time.Minute)
	if err != nil || len(recovered) != 1 {
		t.Fatalf("recovered=%v err=%v, want the run", recovered, err)
	}

	if err := syncsvc.FailRun(ctx, a, runID, "zombie", "internal_error"); err != nil {
		t.Fatalf("zombie FailRun on a recovered run: %v", err)
	}
	var status, code string
	var failures int
	a.QueryRow(`SELECT status, general_error_code FROM sync_runs WHERE id = ?`, runID).Scan(&status, &code)
	a.QueryRow(`SELECT COUNT(*) FROM sync_failures WHERE sync_run_id = ?`, runID).Scan(&failures)
	if status != "failed" || code != "interrupted" || failures != 1 {
		t.Errorf("status=%s code=%s failures=%d, want failed/interrupted/1", status, code, failures)
	}
}
