// Package syncsvc ports app/sync/raw_imports.py, app/sync/service.py, and
// app/sync/recovery.py: persisting raw provider responses, upserting
// accounts/transactions with change-detection hashing, and recovering sync
// runs an earlier process left in_progress.
package syncsvc

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/pluggy"
)

// allowedResponseHeaders and allowedQueryParameters mirror raw_imports.py's
// allowlists: only what's useful for debugging a sync (never anything that
// could carry a credential or session token) is persisted.
var (
	allowedResponseHeaders = map[string]bool{
		"content-type": true, "date": true, "etag": true,
		"ratelimit-limit": true, "ratelimit-remaining": true, "ratelimit-reset": true,
		"retry-after": true, "x-request-id": true,
	}
	allowedQueryParameters = map[string]bool{"accountid": true, "after": true, "itemid": true}
)

func sanitizeRequestPath(rawPath string) string {
	parsed, err := url.Parse(rawPath)
	if err != nil {
		return "/"
	}
	query := parsed.Query()
	safe := url.Values{}
	for key, values := range query {
		if allowedQueryParameters[strings.ToLower(key)] {
			for _, v := range values {
				safe.Add(key, v)
			}
		}
	}
	path := parsed.Path
	if path == "" {
		path = "/"
	}
	if len(safe) == 0 {
		return path
	}
	return path + "?" + safe.Encode()
}

func allowlistedHeaders(headers map[string]string) map[string]string {
	out := make(map[string]string, len(headers))
	for name, value := range headers {
		lower := strings.ToLower(name)
		if allowedResponseHeaders[lower] {
			out[lower] = value
		}
	}
	return out
}

// RawImportWriter mirrors RawImportWriter, implementing
// pluggy.RawEnvelopeWriter.
type RawImportWriter struct {
	DB        *sql.DB
	SyncRunID string
	SourceID  string
	WorkerID  string
}

func (w *RawImportWriter) Write(envelope pluggy.RawResponseEnvelope) (string, error) {
	headers, err := json.Marshal(allowlistedHeaders(envelope.Headers))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(envelope.Content)
	id := uuid.NewString()
	receivedAt := envelope.ReceivedAt
	if receivedAt.IsZero() {
		receivedAt = time.Now().UTC()
	}
	ctx := context.Background()
	tx, err := w.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if w.WorkerID != "" {
		result, err := tx.ExecContext(ctx,
			`UPDATE sync_runs SET heartbeat_at = ? WHERE id = ? AND status = 'in_progress' AND worker_id = ?`,
			db.FormatTime(time.Now()), w.SyncRunID, w.WorkerID)
		if err != nil {
			return "", err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return "", err
		}
		if affected == 0 {
			return "", ErrClaimLost
		}
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO raw_imports (
			id, sync_run_id, source_id, scope, external_account_id, page_sequence,
			request_attempt, request_method, request_path, http_status,
			response_headers, payload, payload_sha256, received_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, w.SyncRunID, w.SourceID, string(envelope.Scope), envelope.ExternalAccountID,
		envelope.PageSequence, envelope.RequestAttempt, strings.ToUpper(envelope.Method),
		sanitizeRequestPath(envelope.Path), envelope.StatusCode, string(headers),
		envelope.Content, hex.EncodeToString(sum[:]), db.FormatTime(receivedAt),
	)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return id, nil
}
