package syncsvc

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"time"

	"github.com/google/uuid"

	"github.com/greg0x46/julius/internal/db"
)

const (
	// DefaultHeartbeatInterval is how often a worker refreshes heartbeat_at
	// on the run it owns.
	DefaultHeartbeatInterval = 15 * time.Second
	// DefaultLeaseTimeout is how long a claimed run may go without a
	// heartbeat before RecoverStaleRuns treats its owner as dead.
	DefaultLeaseTimeout = 90 * time.Second
)

// ErrClaimLost is returned when a worker tries to finish a run it no longer
// owns: the run is still in_progress but worker_id names someone else.
var ErrClaimLost = errors.New("sync run claim lost")

// errRunFinished means the run already reached a terminal status, which
// callers treat as an idempotent no-op.
var errRunFinished = errors.New("sync run already finished")

// fencedTerminalUpdate applies "UPDATE sync_runs SET <set>" to an in_progress
// run, restricted to workerID when it is non-empty. It returns
// errRunFinished if the run is already terminal and ErrClaimLost if another
// worker owns it.
func fencedTerminalUpdate(ctx context.Context, tx *sql.Tx, syncRunID, workerID, set string, args ...any) error {
	query := `UPDATE sync_runs SET ` + set + ` WHERE id = ? AND status = 'in_progress'`
	args = append(args, syncRunID)
	if workerID != "" {
		query += ` AND worker_id = ?`
		args = append(args, workerID)
	}
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 1 {
		return nil
	}
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM sync_runs WHERE id = ?`, syncRunID).Scan(&status); err != nil {
		return err
	}
	if status != "in_progress" {
		return errRunFinished
	}
	return ErrClaimLost
}

type staleRun struct {
	id, workerID, seenAt string
}

// RecoverStaleRuns marks as failed/interrupted every claimed Pluggy run whose
// owner has not heartbeated (falling back to started_at) within lease of now.
// Each run is failed by compare-and-set on (worker_id, last heartbeat), so
// concurrent recoverers never both fail it and a heartbeat that lands first
// keeps the run alive. Unclaimed runs are pending work and are left for any
// worker to claim. Recovered runs are not retried: the daily schedule or a
// manual sync enqueues a fresh one.
func RecoverStaleRuns(ctx context.Context, conn *sql.DB, now time.Time, lease time.Duration) ([]string, error) {
	if lease <= 0 {
		lease = DefaultLeaseTimeout
	}
	rows, err := conn.QueryContext(ctx, `
		SELECT sr.id, sr.worker_id, COALESCE(sr.heartbeat_at, sr.started_at) FROM sync_runs sr
		JOIN data_sources ds ON ds.id = sr.source_id
		WHERE sr.status = 'in_progress' AND sr.run_type = 'sync' AND ds.provider = 'pluggy'
		  AND sr.worker_id IS NOT NULL AND COALESCE(sr.heartbeat_at, sr.started_at) < ?
		ORDER BY sr.started_at, sr.id`,
		db.FormatTime(now.Add(-lease)),
	)
	if err != nil {
		return nil, err
	}
	var candidates []staleRun
	for rows.Next() {
		var run staleRun
		if err := rows.Scan(&run.id, &run.workerID, &run.seenAt); err != nil {
			rows.Close()
			return nil, err
		}
		candidates = append(candidates, run)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	var recovered []string
	for _, run := range candidates {
		ok, err := recoverRun(ctx, conn, run, now)
		if err != nil {
			return recovered, err
		}
		if ok {
			recovered = append(recovered, run.id)
		}
	}
	return recovered, nil
}

func recoverRun(ctx context.Context, conn *sql.DB, run staleRun, now time.Time) (bool, error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	nowText := db.FormatTime(now)
	message := SafeMessage("interrupted")
	result, err := tx.ExecContext(ctx, `
		UPDATE sync_runs SET status = 'failed', finished_at = ?, general_error_code = 'interrupted', general_error_message = ?
		WHERE id = ? AND status = 'in_progress' AND worker_id = ? AND COALESCE(heartbeat_at, started_at) = ?`,
		nowText, message, run.id, run.workerID, run.seenAt,
	)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected != 1 {
		return false, nil
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO sync_failures (id, sync_run_id, stage, error_code, safe_message, created_at)
		VALUES (?, ?, 'interrupted', 'interrupted', ?, ?)`,
		uuid.NewString(), run.id, message, nowText,
	); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// FailRun marks syncRunID failed with a general, item-stage error, mirroring
// failGeneral but callable before a Service exists for the run, e.g. when its
// connection was deactivated after the claim. workerID fences the update to
// the run's owner (empty skips the check). A no-op if the run is no longer
// in_progress; ErrClaimLost if another worker owns it.
func FailRun(ctx context.Context, conn *sql.DB, syncRunID, workerID, errorCode string) error {
	log.Printf("sync_run_general_failure run_id=%s stage=item code=%s", syncRunID, errorCode)
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := db.FormatTime(time.Now())
	message := SafeMessage(errorCode)
	err = fencedTerminalUpdate(ctx, tx, syncRunID, workerID,
		`status = 'failed', finished_at = ?, general_error_code = ?, general_error_message = ?`,
		now, errorCode, message)
	if errors.Is(err, errRunFinished) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO sync_failures (id, sync_run_id, stage, error_code, safe_message, created_at)
		VALUES (?, ?, 'item', ?, ?, ?)`,
		uuid.NewString(), syncRunID, errorCode, message, now,
	); err != nil {
		return err
	}
	return tx.Commit()
}
