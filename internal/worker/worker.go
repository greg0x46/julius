// Package worker ports worker.py's background sync loop: it repeatedly
// claims the oldest unclaimed run, keeps a heartbeat on it while executing,
// and periodically recovers runs whose owner stopped heartbeating.
//
// Several instances may share one Postgres database: a claim is a single
// conditional UPDATE, so exactly one claimant wins each run, and only the
// owner can finish it (see syncsvc.ErrClaimLost).
package worker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/greg0x46/julius/internal/datasources"
	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/pluggy"
	"github.com/greg0x46/julius/internal/settings"
	"github.com/greg0x46/julius/internal/syncsvc"
)

// Config mirrors the reference's worker-relevant Settings fields. Zero
// durations use the defaults (syncsvc.DefaultHeartbeatInterval,
// syncsvc.DefaultLeaseTimeout).
type Config struct {
	PollInterval          time.Duration
	HeartbeatInterval     time.Duration
	LeaseTimeout          time.Duration
	Pluggy                pluggy.Config
	OnTransactionUpserted syncsvc.TransactionUpsertedHook
}

func (c Config) withDefaults() Config {
	if c.PollInterval <= 0 {
		c.PollInterval = time.Second
	}
	if c.HeartbeatInterval <= 0 {
		c.HeartbeatInterval = syncsvc.DefaultHeartbeatInterval
	}
	if c.LeaseTimeout <= 0 {
		c.LeaseTimeout = syncsvc.DefaultLeaseTimeout
	}
	return c
}

// WorkerID identifies this process for sync_runs.worker_id, the claim owner
// checked by heartbeats and terminal updates.
func WorkerID() string {
	host, _ := os.Hostname()
	return fmt.Sprintf("%s:%d:%s", host, os.Getpid(), uuid.NewString()[:8])
}

const claimAttempts = 3

// ClaimNextRun claims the oldest unclaimed Pluggy run with one conditional
// UPDATE. A claimant that loses the row to a concurrent one gets zero rows
// (Postgres re-checks worker_id IS NULL once the winner commits), so it
// retries a few times to take the next pending run instead of idling.
func ClaimNextRun(ctx context.Context, conn *sql.DB, workerID string) (syncRunID, sourceID string, ok bool, err error) {
	for attempt := 0; attempt < claimAttempts; attempt++ {
		err = conn.QueryRowContext(ctx, `
			UPDATE sync_runs SET worker_id = ?, heartbeat_at = ?
			WHERE id = (
				SELECT sr.id FROM sync_runs sr
				JOIN data_sources ds ON ds.id = sr.source_id
				WHERE sr.status = 'in_progress' AND sr.worker_id IS NULL
				  AND sr.run_type = 'sync' AND ds.provider = 'pluggy'
				ORDER BY sr.started_at, sr.id LIMIT 1
			) AND worker_id IS NULL AND status = 'in_progress'
			RETURNING id, source_id`,
			workerID, db.FormatTime(time.Now()),
		).Scan(&syncRunID, &sourceID)
		if err == nil {
			log.Printf("sync_run_claimed run_id=%s worker_id=%s", syncRunID, workerID)
			return syncRunID, sourceID, true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", "", false, err
		}
		var pending bool
		if err := conn.QueryRowContext(ctx, `
			SELECT EXISTS (SELECT 1 FROM sync_runs sr
				JOIN data_sources ds ON ds.id = sr.source_id
				WHERE sr.status = 'in_progress' AND sr.worker_id IS NULL
				  AND sr.run_type = 'sync' AND ds.provider = 'pluggy')`,
		).Scan(&pending); err != nil {
			return "", "", false, err
		}
		if !pending {
			break
		}
	}
	return "", "", false, nil
}

// heartbeat refreshes heartbeat_at on the owned run every interval until ctx
// ends. When the update matches no row the claim was lost (recovered or
// taken over): onLost is called once and heartbeat returns.
func heartbeat(ctx context.Context, conn *sql.DB, runID, workerID string, interval time.Duration, onLost func()) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		result, err := conn.ExecContext(ctx,
			`UPDATE sync_runs SET heartbeat_at = ? WHERE id = ? AND worker_id = ? AND status = 'in_progress'`,
			db.FormatTime(time.Now()), runID, workerID)
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("sync_run_heartbeat_failed run_id=%s: %v", runID, err)
			}
			continue
		}
		if affected, err := result.RowsAffected(); err == nil && affected == 0 {
			if ctx.Err() == nil {
				log.Printf("sync_run_claim_lost run_id=%s worker_id=%s", runID, workerID)
				onLost()
			}
			return
		}
	}
}

// pluggyCredentials reads the decrypted Pluggy API credentials from settings;
// returns ok=false if the server key or Pluggy credentials are unavailable, in which
// case the caller should wait rather than fail the run.
//
// These are application-wide: every connection is an item under the same
// Pluggy application. Which item a run covers is not read here — it comes
// from the run's own source_id, see ProcessClaim.
func pluggyCredentials(ctx context.Context, conn *sql.DB, secrets *settings.Secrets) (clientID, clientSecret string, ok bool) {
	key, available := secrets.Key()
	if !available {
		return "", "", false
	}
	clientID, found, err := settings.Get(ctx, conn, "pluggy.client_id", key)
	if err != nil || !found {
		return "", "", false
	}
	clientSecret, found, err = settings.Get(ctx, conn, "pluggy.client_secret", key)
	if err != nil || !found {
		return "", "", false
	}
	return clientID, clientSecret, true
}

// ProcessClaim mirrors process_claim: it executes a run workerID claimed,
// heartbeating it meanwhile, and cancels the execution if the claim is lost.
func ProcessClaim(ctx context.Context, conn *sql.DB, secrets *settings.Secrets, cfg Config, workerID, syncRunID, sourceID string) error {
	cfg = cfg.withDefaults()
	runCtx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		heartbeat(runCtx, conn, syncRunID, workerID, cfg.HeartbeatInterval, cancel)
	}()
	defer wg.Wait()
	defer cancel()

	clientID, clientSecret, ok := pluggyCredentials(runCtx, conn, secrets)
	if !ok {
		return fmt.Errorf("pluggy credentials unavailable")
	}
	// The item comes from the connection this run was created for, not from
	// global config: with several connections registered, reading a single
	// configured item id here would sync one bank's data into another's run.
	source, err := datasources.Get(runCtx, conn, sourceID)
	if err != nil {
		return fmt.Errorf("resolve data source %s: %w", sourceID, err)
	}
	if source.Provider != datasources.ProviderPluggy {
		return fmt.Errorf("source %s is not a Pluggy connection", sourceID)
	}
	// The connection can be deactivated after ClaimNextRun claims this run
	// but before we get here — ClaimNextRun only looks at unclaimed rows, so
	// it has no way to see that. Check here, before syncing an item the user
	// no longer wants synced.
	if !source.IsActive {
		return syncsvc.FailRun(runCtx, conn, syncRunID, workerID, "connection_inactive")
	}
	pluggyConfig := cfg.Pluggy
	pluggyConfig.ClientID = clientID
	pluggyConfig.ClientSecret = clientSecret
	pluggyConfig.ItemID = source.ExternalItemID

	writer := &syncsvc.RawImportWriter{DB: conn, SyncRunID: syncRunID, SourceID: sourceID, WorkerID: workerID}
	adapter := pluggy.NewAdapter(pluggyConfig, writer, &http.Client{Timeout: pluggyConfig.ConnectTimeout + pluggyConfig.ReadTimeout})
	service := &syncsvc.Service{
		DB: conn, Provider: adapter, SyncRunID: syncRunID, SourceID: sourceID,
		OnTransactionUpserted: cfg.OnTransactionUpserted, WorkerID: workerID,
	}
	return service.Execute(runCtx)
}

func recoverStale(ctx context.Context, conn *sql.DB, lease time.Duration) {
	recovered, err := syncsvc.RecoverStaleRuns(ctx, conn, time.Now(), lease)
	if err != nil {
		log.Printf("recover_stale_runs failed: %v", err)
	}
	for _, id := range recovered {
		log.Printf("sync_run_recovered run_id=%s", id)
	}
}

// Run mirrors run_worker: loops claiming and executing runs until ctx is
// cancelled. Lease recovery runs independently so a long sync cannot delay it.
func Run(ctx context.Context, conn *sql.DB, secrets *settings.Secrets, cfg Config) {
	cfg = cfg.withDefaults()
	workerID := WorkerID()
	recoveryEvery := cfg.LeaseTimeout / 2
	recoverStale(ctx, conn, cfg.LeaseTimeout)
	recoveryDone := make(chan struct{})
	go func() {
		defer close(recoveryDone)
		ticker := time.NewTicker(recoveryEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				recoverStale(ctx, conn, cfg.LeaseTimeout)
			}
		}
	}()
	defer func() { <-recoveryDone }()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Check credentials before claiming: a run claimed here but blocked
		// before credentials exist would sit claimed until its lease expired.
		// Waiting to claim until we can actually proceed keeps every claimed
		// run either running or finished.
		if _, _, ok := pluggyCredentials(ctx, conn, secrets); !ok {
			sleep(ctx, cfg.PollInterval)
			continue
		}

		syncRunID, sourceID, ok, err := ClaimNextRun(ctx, conn, workerID)
		if err != nil {
			log.Printf("claim_next_run failed: %v", err)
			sleep(ctx, cfg.PollInterval)
			continue
		}
		if !ok {
			sleep(ctx, cfg.PollInterval)
			continue
		}
		if err := ProcessClaim(ctx, conn, secrets, cfg, workerID, syncRunID, sourceID); err != nil {
			log.Printf("sync_run_processing_failed run_id=%s: %v", syncRunID, err)
		}
	}
}

func sleep(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
