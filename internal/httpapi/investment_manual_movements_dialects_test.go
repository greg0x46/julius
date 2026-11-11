package httpapi

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/db/dbtest"
)

// providerEquity stores a synced share with one provider movement dated today.
func providerEquity(t *testing.T, conn *sql.DB) string {
	t.Helper()
	now := db.FormatTime(time.Now())
	sourceID, syncRunID, rawImportID, id := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO data_sources (id, provider, external_item_id, created_at, updated_at) VALUES (?, 'pluggy', 'item-1', ?, ?)`, []any{sourceID, now, now}},
		{`INSERT INTO sync_runs (id, source_id, status, started_at, finished_at) VALUES (?, ?, 'completed', ?, ?)`, []any{syncRunID, sourceID, now, now}},
		{`INSERT INTO raw_imports (id, sync_run_id, source_id, scope, page_sequence, request_attempt, request_method, request_path, http_status, response_headers, payload, payload_sha256, received_at)
			VALUES (?, ?, ?, 'investments', 1, 1, 'GET', '/x', 200, '{}', ?, 'sha', ?)`, []any{rawImportID, syncRunID, sourceID, []byte{0}, now}},
		{`INSERT INTO financial_investments (id, source_id, external_id, name, code, investment_type, currency_code, quantity, balance, current_raw_import_id, normalized_hash, created_at, updated_at)
			VALUES (?, ?, ?, 'Ação A', 'AAA3', 'EQUITY', 'BRL', '10', '1100', ?, 'hash', ?, ?)`, []any{id, sourceID, id, rawImportID, now, now}},
		{`INSERT INTO financial_investment_transactions (id, source_id, investment_id, external_id, movement_type, direction, amount, occurred_at, current_raw_import_id, normalized_hash, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'INTEREST', 'outflow', '0', ?, ?, 'hash', ?, ?)`, []any{uuid.NewString(), sourceID, id, uuid.NewString(), now, rawImportID, now, now}},
	} {
		if _, err := conn.Exec(statement.query, statement.args...); err != nil {
			t.Fatalf("%.40s…: %v", statement.query, err)
		}
	}
	return id
}

func call(t *testing.T, handler http.HandlerFunc, method, body, pathID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/x", bytes.NewReader([]byte(body)))
	if pathID != "" {
		req.SetPathValue("id", pathID)
	}
	w := httptest.NewRecorder()
	handler(w, req)
	return w
}

// Create, list, yield and delete of a recorded movement behave the same on
// SQLite and Postgres: the combined history selects both tables.
func TestManualMovementHandlersOnEveryDialect(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		id := providerEquity(t, conn)

		w := call(t, handleCreateInvestmentMovement(conn), http.MethodPost,
			`{"investment_id":"`+id+`","kind":"BUY","trade_date":"2026-01-05","quantity":"10","unit_price":"100"}`, "")
		if w.Code != http.StatusCreated {
			t.Fatalf("create status %d: %s", w.Code, w.Body)
		}
		var created investmentTransactionDTO
		if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
			t.Fatal(err)
		}
		if created.Origin != "manual" || created.Amount == nil || *created.Amount != "1000" {
			t.Fatalf("created = %+v, want a manual purchase of 1000", created)
		}

		w = call(t, handleListInvestmentTransactions(conn), http.MethodGet, "", id)
		var listed []investmentTransactionDTO
		if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil || len(listed) != 2 {
			t.Fatalf("list status %d body %s err %v, want two movements", w.Code, w.Body, err)
		}
		if listed[0].Origin != "provider" || listed[1].Origin != "manual" {
			t.Fatalf("origins = %s, %s; want today's provider movement before the January purchase", listed[0].Origin, listed[1].Origin)
		}

		w = call(t, handleGetInvestment(conn), http.MethodGet, "", id)
		var item investmentDTO
		if err := json.Unmarshal(w.Body.Bytes(), &item); err != nil {
			t.Fatal(err)
		}
		if item.YieldValue == nil || *item.YieldValue != "100" || item.YieldSource == nil || *item.YieldSource != "calculado" {
			t.Fatalf("yield = %v (%v), want 100 calculado from the recorded purchase", item.YieldValue, item.YieldUnavailableReason)
		}

		if w = call(t, handleDeleteInvestmentMovement(conn), http.MethodDelete, "", created.ID); w.Code != http.StatusNoContent {
			t.Fatalf("delete status %d: %s", w.Code, w.Body)
		}
		w = call(t, handleGetInvestment(conn), http.MethodGet, "", id)
		item = investmentDTO{}
		if err := json.Unmarshal(w.Body.Bytes(), &item); err != nil || item.YieldValue != nil {
			t.Fatalf("yield after delete = %v, %v; want unavailable", item.YieldValue, err)
		}
	})
}
