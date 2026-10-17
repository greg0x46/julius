package httpapi_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/greg0x46/julius/internal/db"
)

func TestGetNetWorthComputesBreakdownFromLiveData(t *testing.T) {
	srv, conn := newTestServer(t)
	now := db.FormatTime(time.Now())
	sourceID, syncRunID, rawImportID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := conn.Exec(query, args...); err != nil {
			t.Fatalf("exec %q: %v", query, err)
		}
	}
	exec(`INSERT INTO data_sources (id, provider, external_item_id, created_at, updated_at)
		VALUES (?, 'pluggy', ?, ?, ?)`, sourceID, sourceID, now, now)
	exec(`INSERT INTO sync_runs (id, source_id, status, started_at, finished_at)
		VALUES (?, ?, 'completed', ?, ?)`, syncRunID, sourceID, now, now)
	exec(`INSERT INTO raw_imports (
			id, sync_run_id, source_id, scope, page_sequence, request_attempt,
			request_method, request_path, http_status, response_headers, payload,
			payload_sha256, received_at
		) VALUES (?, ?, ?, 'accounts', 1, 1, 'GET', '/x', 200, '{}', x'00', 'sha', ?)`,
		rawImportID, syncRunID, sourceID, now)

	cashID := uuid.NewString()
	exec(`INSERT INTO financial_accounts (
			id, source_id, external_id, account_type, balance, currency_code,
			current_raw_import_id, normalized_hash, created_at, updated_at
		) VALUES (?, ?, ?, NULL, '1000.00', 'BRL', ?, 'hash', ?, ?)`,
		cashID, sourceID, cashID, rawImportID, now, now)

	// The CREDIT account's raw provider balance (300.00) is deliberately
	// irrelevant — net worth's credit_card_balance comes from
	// transactions.CreditCardTransactionTotal's current bill-cycle transaction
	// total (0, since no financial_bills rows exist here), the same figure
	// the homepage's "Dívida total" widget uses. See internal/networth's
	// TestComputeCreditCardMatchesCurrentBillCycleTransactions for the
	// full-cycle case.
	creditID := uuid.NewString()
	exec(`INSERT INTO financial_accounts (
			id, source_id, external_id, account_type, balance, currency_code,
			current_raw_import_id, normalized_hash, created_at, updated_at
		) VALUES (?, ?, ?, 'CREDIT', '300.00', 'BRL', ?, 'hash', ?, ?)`,
		creditID, sourceID, creditID, rawImportID, now, now)

	investmentID := uuid.NewString()
	exec(`INSERT INTO financial_investments (
			id, source_id, external_id, balance, currency_code,
			current_raw_import_id, normalized_hash, created_at, updated_at
		) VALUES (?, ?, ?, '500.00', 'BRL', ?, 'hash', ?, ?)`,
		investmentID, sourceID, investmentID, rawImportID, now, now)

	exec(`INSERT INTO payables (id, kind, name, total_amount, starting_settled_amount, created_at, updated_at)
		VALUES (?, 'debt', 'Empréstimo', '200.00', '0', ?, ?)`, uuid.NewString(), now, now)
	exec(`INSERT INTO payables (id, kind, name, total_amount, starting_settled_amount, created_at, updated_at)
		VALUES (?, 'receivable', 'A receber', '150.00', '0', ?, ?)`, uuid.NewString(), now, now)

	resp, err := testGet(t, srv.URL+"/api/net-worth")
	if err != nil {
		t.Fatalf("GET /api/net-worth: %v", err)
	}
	var got map[string]any
	decodeJSON(t, resp, &got)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	latest, ok := got["latest"].(map[string]any)
	if !ok {
		t.Fatalf("latest = %+v, want an object", got["latest"])
	}
	// assets = 1000 (cash) + 500 (investments) = 1500 — the 150 receivable
	// payable above never enters this model at all, a future possibility
	// rather than a realized asset (see internal/networth's Breakdown doc
	// comment); liabilities = 0 (credit card — no bill cycle to compute
	// from) + 200 (debt) = 200; net worth = 1300
	if latest["cash_balance"] != "1000.00" || latest["investment_balance"] != "500.00" ||
		latest["credit_card_balance"] != "0" || latest["payables_debt"] != "200.00" {
		t.Errorf("latest breakdown = %+v", latest)
	}
	if _, hasReceivable := latest["payables_receivable"]; hasReceivable {
		t.Errorf("latest breakdown = %+v, want no payables_receivable field", latest)
	}
	if latest["total_assets"] != "1500.00" || latest["total_liabilities"] != "200.00" || latest["net_worth"] != "1300.00" {
		t.Errorf("latest totals = %+v", latest)
	}

	series, ok := got["series"].([]any)
	if !ok || len(series) != 1 {
		t.Fatalf("series = %+v, want exactly one snapshot (today's)", got["series"])
	}
}

func TestGetNetWorthIsIdempotentWithinTheSameDay(t *testing.T) {
	srv, _ := newTestServer(t)

	first, err := testGet(t, srv.URL+"/api/net-worth")
	if err != nil {
		t.Fatalf("GET /api/net-worth (first): %v", err)
	}
	var firstBody map[string]any
	decodeJSON(t, first, &firstBody)

	second, err := testGet(t, srv.URL+"/api/net-worth")
	if err != nil {
		t.Fatalf("GET /api/net-worth (second): %v", err)
	}
	var secondBody map[string]any
	decodeJSON(t, second, &secondBody)

	firstSeries, _ := firstBody["series"].([]any)
	secondSeries, _ := secondBody["series"].([]any)
	if len(firstSeries) != 1 || len(secondSeries) != 1 {
		t.Fatalf("expected exactly one snapshot per call within the same day, got %d then %d", len(firstSeries), len(secondSeries))
	}
}
