package httpapi_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/db"
)

func TestHistoricalFXReportingOverHTTP(t *testing.T) {
	srv, conn := newTestServer(t)
	id := insertTransaction(t, conn)
	if _, err := conn.Exec(`UPDATE financial_transactions SET amount = '10', amount_in_account_currency = NULL, currency_code = 'USD', occurred_at = ? WHERE id = ?`, "2026-03-15T12:00:00.000000000Z", id); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO exchange_rates (base_currency, quote_currency, as_of, effective_on, rate, source, fetched_at) VALUES ('USD','BRL','2026-03-15','2026-03-15','5','test',?)`, db.FormatTime(time.Now())); err != nil {
		t.Fatal(err)
	}
	resp := doJSON(t, http.MethodGet, srv.URL+"/api/transactions/category-breakdown?timezone=UTC&month=2026-03&classification=outflow", nil)
	if resp.StatusCode != 200 {
		t.Fatal(resp.StatusCode)
	}
	var category map[string]any
	decodeJSON(t, resp, &category)
	coverage := category["reporting"].(map[string]any)
	if category["total"] != "50" || coverage["status"] != "complete" || category["totals"].([]any)[0].(map[string]any)["currency_code"] != "USD" {
		t.Fatalf("category = %+v", category)
	}
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/timeline?reference_date=2026-03-15&from=2026-03-01&to=2026-03-31", nil)
	if resp.StatusCode != 200 {
		t.Fatal(resp.StatusCode)
	}
	var body map[string]any
	decodeJSON(t, resp, &body)
	period := body["period_totals"].(map[string]any)
	if period["expense"] != "50" || period["reporting"].(map[string]any)["status"] != "complete" {
		t.Fatalf("period = %+v", period)
	}
}

func TestMissingFXReportingOverHTTP(t *testing.T) {
	unavailableReportingFXProvider(t)
	srv, conn := newTestServer(t)
	id := insertTransaction(t, conn)
	if _, err := conn.Exec(`UPDATE financial_transactions SET amount = '10', amount_in_account_currency = NULL, currency_code = 'USD', occurred_at = ? WHERE id = ?`, "2099-03-15T12:00:00.000000000Z", id); err != nil {
		t.Fatal(err)
	}
	resp := doJSON(t, http.MethodGet, srv.URL+"/api/transactions/category-breakdown?timezone=UTC&month=2099-03&classification=outflow", nil)
	if resp.StatusCode != 200 {
		t.Fatal(resp.StatusCode)
	}
	var category map[string]any
	decodeJSON(t, resp, &category)
	if category["total"] != nil || category["reporting"].(map[string]any)["missing"] != float64(1) {
		t.Fatalf("category = %+v", category)
	}
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/timeline?reference_date=2099-03-15&from=2099-03-01&to=2099-03-31", nil)
	if resp.StatusCode != 503 || resp.Header.Get("Content-Type") != "application/problem+json" {
		t.Fatalf("missing timeline = %d, %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	var problem map[string]any
	decodeJSON(t, resp, &problem)
	if problem["type"] != "/problems/reporting-fx-unavailable" || problem["reporting"].(map[string]any)["missing"] != float64(1) {
		t.Fatalf("problem = %+v", problem)
	}
}
