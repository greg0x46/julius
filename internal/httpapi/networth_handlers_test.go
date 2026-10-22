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

	// A credit card is credit, not wealth: whatever it owes stays out.
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

	// Neither a debt nor a receivable is realized: both stay out.
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
	// wealth = 1000 (cash) + 500 (investments); card and debt do not count.
	if latest["cash_balance"] != "1000.00" || latest["investment_balance"] != "500.00" || latest["total"] != "1500.00" {
		t.Errorf("latest = %+v", latest)
	}
	for _, gone := range []string{"credit_card_balance", "payables_debt", "total_assets", "total_liabilities", "net_worth"} {
		if _, present := latest[gone]; present {
			t.Errorf("latest carries %s, but wealth has no liabilities: %+v", gone, latest)
		}
	}

	series, ok := got["series"].([]any)
	if !ok || len(series) != 1 {
		t.Fatalf("series = %+v, want exactly one point (today's)", got["series"])
	}
}

func TestGetNetWorthFollowsTheDataOnEveryRead(t *testing.T) {
	srv, conn := newTestServer(t)
	read := func() (string, int) {
		t.Helper()
		resp, err := testGet(t, srv.URL+"/api/net-worth")
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		decodeJSON(t, resp, &got)
		series, _ := got["series"].([]any)
		total, _ := got["latest"].(map[string]any)["total"].(string)
		return total, len(series)
	}
	if total, points := read(); total != "0" || points != 1 {
		t.Fatalf("empty book: total = %q over %d points, want 0 over today only", total, points)
	}
	now := db.FormatTime(time.Now())
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := conn.Exec(query, args...); err != nil {
			t.Fatalf("exec %q: %v", query, err)
		}
	}
	// A dated cash movement gives the history somewhere to start.
	exec(`INSERT INTO financial_accounts(id,origin,name,account_type,currency_code,opening_balance,created_at,updated_at) VALUES('bank','manual','Cash','BANK','BRL','0',?,?)`, now, now)
	exec(`INSERT INTO account_balance_adjustments(id,account_id,amount,description,created_at) VALUES('mark','bank','0','Marco',?)`, db.FormatTime(time.Now().AddDate(0, 0, -3)))
	exec(`INSERT INTO investment_accounts (id,name,kind,currency_code,created_at,updated_at) VALUES ('manual','Manual','manual','BRL',?,?)`, now, now)
	exec(`INSERT INTO investment_operations (id,account_id,kind,occurred_on,amount,created_at,updated_at) VALUES ('opening','manual','initial_balance','2026-09-01','700',?,?)`, now, now)
	if total, points := read(); total != "700" || points < 4 {
		t.Fatalf("after a deposit: total = %q over %d points, want 700 and history back to the first cash movement (nothing is cached or stored)", total, points)
	}
}

func TestGetNetWorthValuesForeignCashAndRecordsTheRates(t *testing.T) {
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
	if latest["valuation_status"] != "complete" {
		t.Fatalf("latest valuation = %+v", latest)
	}
	usd := latest["valuation"].(map[string]any)["rates"].(map[string]any)["USD"].(map[string]any)
	assertMoney(t, "recorded rate", usd["value"], "5")
	if usd["source"] != "synthetic" {
		t.Fatalf("recorded rate lost its source: %+v", usd)
	}
}

// TestGetNetWorthRevaluesEachPastDayInTheReferenceCurrency: history is derived
// in BRL and shown in the chosen currency at each day's own rate, with that
// day's provenance.
func TestGetNetWorthRevaluesEachPastDayInTheReferenceCurrency(t *testing.T) {
	srv, conn := newTestServer(t)
	now := time.Now().UTC() // rates exist per UTC day, whatever the server's zone
	stamp := db.FormatTime(now)
	today, yesterday := now.Format("2006-01-02"), now.AddDate(0, 0, -1).Format("2006-01-02")
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := conn.Exec(query, args...); err != nil {
			t.Fatalf("exec %q: %v", query, err)
		}
	}
	exec(`INSERT INTO financial_accounts(id,origin,name,account_type,currency_code,opening_balance,created_at,updated_at) VALUES('manual','manual','Cash','BANK','BRL','400',?,?)`, db.FormatTime(now.AddDate(0, 0, -1)), stamp)
	exec(`INSERT INTO account_balance_adjustments(id,account_id,amount,description,created_at) VALUES('adjust','manual','0',?,?)`, "Marco", db.FormatTime(now.AddDate(0, 0, -1)))
	for date, rate := range map[string]string{today: "0.2", yesterday: "0.25"} {
		exec(`INSERT INTO exchange_rates (base_currency, quote_currency, as_of, effective_on, rate, source, fetched_at) VALUES ('BRL','USD',?,?,?,'synthetic',?)`, date, date, rate, stamp)
	}
	investmentCall(t, srv, http.MethodPut, "/api/preferences", map[string]any{"transactions_period_basis": "occurred_at", "reference_currency": "USD"}, 200)

	worth := investmentCall(t, srv, http.MethodGet, "/api/net-worth", nil, 200)
	series := worth["series"].([]any)
	byDay := map[string]map[string]any{}
	for _, row := range series {
		point := row.(map[string]any)
		byDay[point["date"].(string)[:10]] = point
	}
	past := byDay[yesterday]
	if past == nil {
		t.Fatalf("no point for %s in %d points", yesterday, len(series))
	}
	assertMoney(t, "yesterday at yesterday's rate", past["total"], "100")
	// Today is valued on the latest day a rate can exist, whatever the server's zone.
	assertMoney(t, "today at today's rate", worth["latest"].(map[string]any)["total"], "80")
	if past["currency_code"] != "USD" || past["fx"].(map[string]any)["source"] != "synthetic" {
		t.Fatalf("lost FX provenance: %+v", past)
	}
}
