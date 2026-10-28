package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/greg0x46/julius/internal/syncsvc"
)

// MCPRequestSync reuses the REST enqueue policy without exposing arbitrary routes.
func MCPRequestSync(ctx context.Context, conn *sql.DB, sourceID string) MCPWriteResult {
	body, _ := json.Marshal(createSyncRunRequest{SourceID: sourceID})
	r, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/", bytes.NewReader(body))
	w := &mcpWriteResponse{header: make(http.Header), status: http.StatusOK}
	handleCreateSyncRun(conn)(w, r)
	return MCPWriteResult{Status: w.status, Data: append(json.RawMessage(nil), w.Bytes()...)}
}

// MCPReadSync exposes bounded history and canonical failure messages only.
func MCPReadSync(ctx context.Context, conn *sql.DB, id string, limit, offset int) (any, error) {
	if id == "" {
		rows, err := conn.QueryContext(ctx, syncRunSelect+` WHERE sr.run_type='sync' ORDER BY sr.started_at DESC,sr.id DESC LIMIT ? OFFSET ?`, limit, offset)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		runs := []syncRunDTO{}
		for rows.Next() {
			run, err := scanSyncRun(rows)
			if err != nil {
				return nil, err
			}
			runs = append(runs, run)
		}
		return runs, rows.Err()
	}
	run, err := loadSyncRun(ctx, conn, id)
	if err != nil {
		return nil, err
	}
	// File imports are not provider synchronization runs.
	var kind string
	if err = conn.QueryRowContext(ctx, `SELECT run_type FROM sync_runs WHERE id=?`, id).Scan(&kind); err != nil {
		return nil, err
	}
	if kind != "sync" {
		return nil, sql.ErrNoRows
	}
	rows, err := conn.QueryContext(ctx, `SELECT stage,error_code FROM sync_failures WHERE sync_run_id=? ORDER BY created_at,id LIMIT ? OFFSET ?`, id, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type failure struct {
		Stage   string `json:"stage"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	failures := []failure{}
	for rows.Next() {
		var f failure
		if err = rows.Scan(&f.Stage, &f.Code); err != nil {
			return nil, err
		}
		f.Message = syncsvc.SafeMessage(f.Code)
		failures = append(failures, f)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return struct {
		Run      syncRunDTO `json:"run"`
		Failures []failure  `json:"failures"`
		Offset   int        `json:"offset"`
		Limit    int        `json:"limit"`
	}{run, failures, offset, limit}, nil
}

// MCPOperationData keeps transport output bounded and database errors private.
func MCPOperationData(value any, err error) (MCPWriteResult, error) {
	if errors.Is(err, sql.ErrNoRows) {
		return MCPWriteResult{}, errors.New("synchronization not found")
	}
	if err != nil {
		return MCPWriteResult{}, errors.New("operations unavailable; retry later")
	}
	data, err := json.Marshal(value)
	if err != nil || len(data) > mcpMaxOutput {
		return MCPWriteResult{}, errors.New("operations output exceeds 256 KiB; reduce limit")
	}
	return MCPWriteResult{Status: 200, Data: data}, nil
}
