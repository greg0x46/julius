package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/greg0x46/julius/internal/datasources"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/greg0x46/julius/internal/syncsvc"
)

var resultMessages = map[string]string{
	"in_progress":             "Synchronization is in progress.",
	"completed":               "Synchronization completed successfully.",
	"completed_with_failures": "Synchronization completed with retained partial failures.",
	"failed":                  "Synchronization failed; previously imported data was preserved.",
}

type syncRunDTO struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	// SourceID and SourceName say which connection the run refreshed — with
	// several banks configured, a run is meaningless without it.
	SourceID             string     `json:"source_id"`
	SourceName           string     `json:"source_name"`
	StartedAt            time.Time  `json:"started_at"`
	FinishedAt           *time.Time `json:"finished_at"`
	AccountsProcessed    int        `json:"accounts_processed"`
	TransactionsInserted int        `json:"transactions_inserted"`
	TransactionsUpdated  int        `json:"transactions_updated"`
	ResultMessage        string     `json:"result_message"`
}

type syncFailureDTO struct {
	Stage                 string    `json:"stage"`
	Code                  string    `json:"code"`
	Message               string    `json:"message"`
	ExternalAccountID     *string   `json:"external_account_id"`
	ExternalTransactionID *string   `json:"external_transaction_id"`
	OccurredAt            time.Time `json:"occurred_at"`
}

// syncRunSelect joins the connection in so a run carries the name of what it
// refreshed — with several banks configured, a run is meaningless without it.
const syncRunSelect = `
	SELECT sr.id, sr.status, sr.source_id, ` + connectionNameColumn + `,
	       sr.started_at, sr.finished_at,
	       sr.accounts_processed, sr.transactions_inserted, sr.transactions_updated
	FROM sync_runs sr JOIN data_sources ds ON ds.id = sr.source_id`

type syncRunRowScanner interface {
	Scan(dest ...any) error
}

func scanSyncRun(row syncRunRowScanner) (syncRunDTO, error) {
	var (
		d             syncRunDTO
		startedAtRaw  string
		finishedAtRaw sql.NullString
	)
	if err := row.Scan(dbtypes.ID(&d.ID), &d.Status, dbtypes.ID(&d.SourceID), &d.SourceName, dbtypes.Time(&startedAtRaw), dbtypes.Time(&finishedAtRaw),
		&d.AccountsProcessed, &d.TransactionsInserted, &d.TransactionsUpdated); err != nil {
		return syncRunDTO{}, err
	}
	started, err := dbtypes.ParseTime(startedAtRaw)
	if err != nil {
		return syncRunDTO{}, err
	}
	d.StartedAt = started
	if d.FinishedAt, err = dbtypes.ParseNullTime(finishedAtRaw); err != nil {
		return syncRunDTO{}, err
	}
	d.ResultMessage = resultMessages[d.Status]
	return d, nil
}

func loadSyncRun(ctx context.Context, conn *sql.DB, id string) (syncRunDTO, error) {
	return scanSyncRun(conn.QueryRowContext(ctx, syncRunSelect+` WHERE sr.id = ?`, id))
}

type createSyncRunRequest struct {
	// SourceID names a single connection to refresh; empty (or an absent
	// body) means every active one.
	SourceID string `json:"source_id"`
}

// createSyncRunResponse reports both what started and how many connections
// were targeted, so a caller can tell a partial start (requested > len(runs))
// apart from a complete one instead of the two looking identical.
type createSyncRunResponse struct {
	Runs      []syncRunDTO `json:"runs"`
	Requested int          `json:"requested"`
}

// handleCreateSyncRun enqueues one run per connection, so "Sincronizar agora"
// keeps meaning "refresh everything" now that there can be more than one bank.
//
// Partial success is the rule, not an accident. A connection already syncing
// is skipped rather than failing the request — one busy bank must not stop the
// others from refreshing — and a connection whose insert fails outright is
// skipped the same way. That matters because the inserts are separate
// statements: aborting mid-loop and answering 503 would leave the runs already
// inserted in_progress, picked up by the worker, with the client told the
// whole request failed. Wrapping the loop in one transaction is not the answer
// either: a unique violation is the *expected* outcome for a busy connection,
// and on Postgres that would poison the surrounding transaction and roll back
// the connections that started perfectly well.
//
// So the response reports what actually started, plus how many connections
// were targeted (createSyncRunResponse.Requested), so the caller can tell a
// partial start apart from a complete one. Only when nothing started at all
// does this fail, and then it distinguishes the two reasons: 503 if any
// connection errored, 409 if every one of them was simply already syncing —
// which is exactly the single-connection case the frontend's conflict notice
// was written for, and where active_sync_run_id still points somewhere useful.
func handleCreateSyncRun(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createSyncRunRequest
		if err := decodeStrict(r, &req); err != nil && !errors.Is(err, io.EOF) {
			writeProblem(w, 422, "invalid-sync-run", "Solicitação inválida", "Revise os campos enviados.")
			return
		}

		targets, ok := syncRunTargets(w, r, conn, req.SourceID)
		if !ok {
			return
		}

		created := []syncRunDTO{}
		var active sql.NullString
		failed, busy := 0, 0
		for _, target := range targets {
			runID, isBusy, err := syncsvc.Enqueue(r.Context(), conn, target.ID, time.Now())
			if isBusy {
				// uq_sync_runs_active_source rejected it: this connection is
				// mid-sync. Remember the first one so a request that starts
				// nothing can still point at something to watch.
				if !active.Valid {
					_ = conn.QueryRowContext(r.Context(),
						`SELECT id FROM sync_runs WHERE source_id = ? AND status = 'in_progress'`, target.ID,
					).Scan(dbtypes.ID(&active))
				}
				busy++
				continue
			}
			if err != nil {
				log.Printf("sync_run_enqueue_failed source_id=%s source=%q: %v", target.ID, target.Name(), err)
				failed++
				continue
			}
			run, err := loadSyncRun(r.Context(), conn, runID)
			if err != nil {
				// The run is inserted and the worker will pick it up; only the
				// read-back failed. Counting it as failed rather than dropping
				// the request keeps the other connections' runs reportable, and
				// GET /api/sync-runs will show this one on the next poll.
				failed++
				continue
			}
			created = append(created, run)
		}

		if len(created) == 0 {
			if failed > 0 {
				writeProblem(w, 503, "sync-run-unavailable", "Sincronização temporariamente indisponível", "Tente novamente em instantes.")
				return
			}
			body := map[string]any{
				"type": "/problems/sync-already-running", "title": "Synchronization already in progress",
				"status": 409, "detail": "Wait for the active synchronization to finish.",
			}
			if active.Valid {
				body["active_sync_run_id"] = active.String
			}
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(409)
			_ = json.NewEncoder(w).Encode(body)
			return
		}
		if len(created) == 1 {
			w.Header().Set("Location", "/api/sync-runs/"+created[0].ID)
		}
		// requested lets the caller tell a partial start (fewer runs than
		// connections targeted) apart from a complete one — a bare array of
		// what started is otherwise indistinguishable from "that is all there
		// was". The individual failures are still logged below for operators.
		if failed > 0 || busy > 0 {
			log.Printf("sync_run_create_partial started=%d busy=%d failed=%d of=%d",
				len(created), busy, failed, len(targets))
		}
		writeJSON(w, http.StatusAccepted, createSyncRunResponse{Runs: created, Requested: len(targets)})
	}
}

// syncRunTargets resolves which connections a create request covers, writing
// the problem response itself and reporting ok=false when it did.
func syncRunTargets(w http.ResponseWriter, r *http.Request, conn *sql.DB, sourceID string) ([]datasources.DataSource, bool) {
	if sourceID != "" {
		source, err := datasources.Get(r.Context(), conn, sourceID)
		if errors.Is(err, datasources.ErrNotFound) {
			writeProblem(w, 404, "data-source-not-found", "Conexão não encontrada", "")
			return nil, false
		}
		if err != nil {
			writeProblem(w, 503, "sync-run-unavailable", "Sincronização temporariamente indisponível", "Tente novamente em instantes.")
			return nil, false
		}
		if source.Provider != datasources.ProviderPluggy {
			writeProblem(w, 409, "data-source-not-syncable", "Fonte sem sincronização",
				"Esta fonte é importada por arquivo e não pode ser sincronizada com a Pluggy.")
			return nil, false
		}
		if !source.IsActive {
			writeProblem(w, 409, "data-source-inactive", "Conexão desativada",
				"Reative a conexão antes de sincronizá-la.")
			return nil, false
		}
		return []datasources.DataSource{source}, true
	}

	sources, err := datasources.ListActive(r.Context(), conn)
	if err != nil {
		writeProblem(w, 503, "sync-run-unavailable", "Sincronização temporariamente indisponível", "Tente novamente em instantes.")
		return nil, false
	}
	if len(sources) == 0 {
		writeProblem(w, 409, "not-configured", "Nenhuma conexão ativa",
			"Cadastre uma conexão do Pluggy antes de sincronizar.")
		return nil, false
	}
	return sources, true
}

// handleListSyncRuns returns the most recent runs, newest first.
//
// A row that fails to scan or parse fails the whole request. The previous
// shape skipped it and served the rest, which is the wrong trade for a
// history: a silently shorter list is indistinguishable from "that sync never
// happened", and the timestamps here are written by this app in a fixed
// format, so a parse failure means the database is corrupt rather than
// merely surprising. 503 says so, and matches what GET /api/sync-runs/{id}
// has always done with the same row.
func handleListSyncRuns(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 20
		if v := r.URL.Query().Get("limit"); v != "" {
			if parsed, err := parsePositiveInt(v); err == nil && parsed >= 1 && parsed <= 100 {
				limit = parsed
			}
		}
		rows, err := conn.QueryContext(r.Context(),
			syncRunSelect+` WHERE sr.run_type = 'sync' ORDER BY sr.started_at DESC, sr.id DESC LIMIT ?`, limit)
		if err != nil {
			writeProblem(w, 503, "sync-run-unavailable", "Sincronização temporariamente indisponível", "Tente novamente em instantes.")
			return
		}
		defer rows.Close()

		result := []syncRunDTO{}
		for rows.Next() {
			run, err := scanSyncRun(rows)
			if err != nil {
				writeProblem(w, 503, "sync-run-unavailable", "Sincronização temporariamente indisponível", "Tente novamente em instantes.")
				return
			}
			result = append(result, run)
		}
		if rows.Err() != nil {
			writeProblem(w, 503, "sync-run-unavailable", "Sincronização temporariamente indisponível", "Tente novamente em instantes.")
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func parsePositiveInt(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errBadInt
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

var errBadInt = errors.New("not a positive integer")

type syncRunDetailDTO struct {
	syncRunDTO
	Failures []syncFailureDTO `json:"failures"`
}

func handleGetSyncRun(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		run, err := loadSyncRun(r.Context(), conn, id)
		if errors.Is(err, sql.ErrNoRows) {
			writeProblem(w, 404, "sync-run-not-found", "Synchronization not found", "")
			return
		}
		if err != nil {
			writeProblem(w, 503, "sync-run-unavailable", "Sincronização temporariamente indisponível", "Tente novamente em instantes.")
			return
		}

		rows, err := conn.QueryContext(r.Context(), `
			SELECT stage, error_code, safe_message, external_account_id, external_transaction_id, created_at
			FROM sync_failures WHERE sync_run_id = ? ORDER BY created_at`, id)
		if err != nil {
			writeProblem(w, 503, "sync-run-unavailable", "Sincronização temporariamente indisponível", "Tente novamente em instantes.")
			return
		}
		defer rows.Close()

		failures := []syncFailureDTO{}
		for rows.Next() {
			var f syncFailureDTO
			var occurredAtRaw string
			if err := rows.Scan(&f.Stage, &f.Code, &f.Message, dbtypes.ID(&f.ExternalAccountID), dbtypes.ID(&f.ExternalTransactionID), dbtypes.Time(&occurredAtRaw)); err != nil {
				writeProblem(w, 503, "sync-run-unavailable", "Sincronização temporariamente indisponível", "Tente novamente em instantes.")
				return
			}
			if f.OccurredAt, err = dbtypes.ParseTime(occurredAtRaw); err != nil {
				writeProblem(w, 503, "sync-run-unavailable", "Sincronização temporariamente indisponível", "Tente novamente em instantes.")
				return
			}
			failures = append(failures, f)
		}
		if rows.Err() != nil {
			writeProblem(w, 503, "sync-run-unavailable", "Sincronização temporariamente indisponível", "Tente novamente em instantes.")
			return
		}
		writeJSON(w, http.StatusOK, syncRunDetailDTO{syncRunDTO: run, Failures: failures})
	}
}
