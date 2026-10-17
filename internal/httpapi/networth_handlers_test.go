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

func TestGetNetWorthValuesForeignCashAndKeepsLegacyAndUnavailableDaysExplicit(t *testing.T) {
	srv, conn := newTestServer(t)
	now := time.Now().UTC()
	stamp := db.FormatTime(now)
	day := now.Format("2006-01-02")
	sourceID, syncRunID, rawImportID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := conn.Exec(query, args...); err != nil {
			t.Fatalf("exec %q: %v", query, err)
		}
	}
	exec(`INSERT INTO data_sources (id, provider, external_item_id, created_at, updated_at) VALUES (?, 'pluggy', ?, ?, ?)`, sourceID, sourceID, stamp, stamp)
	exec(`INSERT INTO sync_runs (id, source_id, status, started_at, finished_at) VALUES (?, ?, 'completed', ?, ?)`, syncRunID, sourceID, stamp, stamp)
	exec(`INSERT INTO raw_imports (id, sync_run_id, source_id, scope, page_sequence, request_attempt, request_method, request_path, http_status, response_headers, payload, payload_sha256, received_at)
		VALUES (?, ?, ?, 'accounts', 1, 1, 'GET', '/x', 200, '{}', x'00', 'sha', ?)`, rawImportID, syncRunID, sourceID, stamp)
	for balance, currency := range map[string]string{"2000": "BRL", "100": "USD"} {
		id := uuid.NewString()
		exec(`INSERT INTO financial_accounts (id, source_id, external_id, account_type, balance, currency_code, current_raw_import_id, normalized_hash, created_at, updated_at)
			VALUES (?, ?, ?, NULL, ?, ?, ?, 'hash', ?, ?)`, id, sourceID, id, balance, currency, rawImportID, stamp, stamp)
	}
	exec(`INSERT INTO exchange_rates (base_currency, quote_currency, as_of, effective_on, rate, source, fetched_at) VALUES ('USD','BRL',?,?,'5','synthetic',?)`, day, day, stamp)
	// An old row from before foreign balances were valued, and a past day that cannot be.
	exec(`INSERT INTO net_worth_snapshots(id,captured_at,total_assets,total_liabilities,net_worth,cash_balance,investment_balance,credit_card_balance,payables_debt,is_backfilled,created_at,updated_at)
		VALUES (?, '2020-01-02','100','0','100','100','0','0','0',FALSE,?,?)`, uuid.NewString(), stamp, stamp)
	exec(`INSERT INTO net_worth_snapshots(id,captured_at,total_assets,total_liabilities,net_worth,cash_balance,investment_balance,credit_card_balance,payables_debt,is_backfilled,created_at,updated_at,valuation_version,valuation_status)
		VALUES (?, '2020-01-03','0','0','0','0','0','0','0',TRUE,?,?,1,'unavailable')`, uuid.NewString(), stamp, stamp)

	resp, err := testGet(t, srv.URL+"/api/net-worth")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	decodeJSON(t, resp, &got)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %+v", resp.StatusCode, got)
	}

	latest := got["latest"].(map[string]any)
	assertMoney(t, "BRL 2,000 + USD 100 at 5", latest["cash_balance"], "2500")
	if latest["valuation_status"] != "complete" || latest["valuation_version"] != float64(1) || latest["stored_currency"] != "BRL" {
		t.Fatalf("latest valuation = %+v", latest)
	}
	usd := latest["valuation"].(map[string]any)["rates"].(map[string]any)["USD"].(map[string]any)
	assertMoney(t, "recorded rate", usd["value"], "5")
	if usd["source"] != "synthetic" {
		t.Fatalf("recorded rate lost its source: %+v", usd)
	}

	byDay := map[string]map[string]any{}
	for _, row := range got["series"].([]any) {
		snapshot := row.(map[string]any)
		byDay[snapshot["captured_at"].(string)[:10]] = snapshot
	}
	legacy := byDay["2020-01-02"]
	if legacy["valuation_status"] != "legacy" || legacy["valuation_version"] != float64(0) || legacy["valuation"] != nil {
		t.Fatalf("legacy row was relabeled: %+v", legacy)
	}
	assertMoney(t, "legacy amount is untouched", legacy["net_worth"], "100")
	unavailable := byDay["2020-01-03"]
	if unavailable["valuation_status"] != "unavailable" {
		t.Fatalf("unavailable row: %+v", unavailable)
	}
	for _, field := range []string{"net_worth", "cash_balance", "total_assets"} {
		if _, present := unavailable[field]; present {
			t.Fatalf("an unavailable day must carry no %s, not a zero: %+v", field, unavailable)
		}
	}
}
