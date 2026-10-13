package httpapi_test

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/db"
)

type reportingTransport func(*http.Request) (*http.Response, error)

func (transport reportingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return transport(req)
}

func unavailableReportingFXProvider(t *testing.T) {
	t.Helper()
	original := http.DefaultTransport
	http.DefaultTransport = reportingTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.Hostname() == "api.frankfurter.dev" {
			return nil, errors.New("FX provider unavailable in test")
		}
		return original.RoundTrip(req)
	})
	t.Cleanup(func() { http.DefaultTransport = original })
}

func TestSpendingByCategoryDisclosesReportingCoverage(t *testing.T) {
	unavailableReportingFXProvider(t)
	for _, status := range []string{"complete", "stale", "unavailable"} {
		t.Run(status, func(t *testing.T) {
			srv, conn := newTestServer(t)
			id := insertTransaction(t, conn)
			if _, err := conn.Exec(`UPDATE financial_transactions SET amount = '-10', amount_in_account_currency = NULL, currency_code = 'USD' WHERE id = ?`, id); err != nil {
				t.Fatal(err)
			}
			if status != "unavailable" {
				day := time.Now().UTC()
				effective := day
				if status == "stale" {
					effective = day.AddDate(0, 0, -10)
				}
				if _, err := conn.Exec(`INSERT INTO exchange_rates (base_currency, quote_currency, as_of, effective_on, rate, source, fetched_at) VALUES ('USD','BRL',?,?,'5','test',?)`, day.Format("2006-01-02"), effective.Format("2006-01-02"), db.FormatTime(day)); err != nil {
					t.Fatal(err)
				}
			}
			resp := doJSON(t, http.MethodGet, srv.URL+"/api/transactions/spending-by-category?timezone=UTC", nil)
			wantStatus := http.StatusOK
			if status == "unavailable" {
				wantStatus = http.StatusServiceUnavailable
			}
			if resp.StatusCode != wantStatus {
				t.Fatalf("status = %d, want %d", resp.StatusCode, wantStatus)
			}
			var body map[string]any
			decodeJSON(t, resp, &body)
			coverage := body["reporting"].(map[string]any)
			if coverage["status"] != status || coverage["eligible"] != float64(1) {
				t.Fatalf("coverage = %+v", coverage)
			}
			raw := body["totals"].([]any)[0].(map[string]any)
			if raw["currency_code"] != "USD" || raw["outflow"] != "10" {
				t.Fatalf("raw totals = %+v", raw)
			}
			if status == "unavailable" {
				if body["type"] != "/problems/reporting-fx-unavailable" || coverage["totals"] != nil || coverage["missing"] != float64(1) {
					t.Fatalf("unavailable response = %+v", body)
				}
			} else if body["total"] != "50" || coverage["converted"] != float64(1) || coverage["totals"].(map[string]any)["outflow"] != "50" {
				t.Fatalf("converted response = %+v", body)
			}
		})
	}
}

func TestSpendingByCategoryProjectedCoverage(t *testing.T) {
	srv, conn := newTestServer(t)
	insertTransaction(t, conn)
	resp := doJSON(t, http.MethodPost, srv.URL+"/api/payables", map[string]any{"kind": "debt", "name": "Debt", "total_amount": "1000"})
	var debt map[string]any
	decodeJSON(t, resp, &debt)
	resp = doJSON(t, http.MethodPost, srv.URL+"/api/payables/"+debt["id"].(string)+"/scenarios", map[string]any{"name": "Plan"})
	var scenario map[string]any
	decodeJSON(t, resp, &scenario)
	scenarioID := scenario["id"].(string)
	day := time.Now().UTC()
	for _, date := range []time.Time{day, day, day.AddDate(0, 1, 0)} {
		resp = doJSON(t, http.MethodPost, srv.URL+"/api/scenarios/"+scenarioID+"/transactions", map[string]any{"description": "Installment", "amount": "10", "projected_at": date.Format("2006-01-02"), "category": "Debt"})
		if resp.StatusCode != http.StatusCreated {
			t.Fatal(resp.StatusCode)
		}
		resp.Body.Close()
	}
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/transactions/spending-by-category?timezone=UTC&scenario_id="+scenarioID, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatal(resp.StatusCode)
	}
	var body map[string]any
	decodeJSON(t, resp, &body)
	coverage := body["reporting"].(map[string]any)
	if body["total"] != "62.00" || coverage["eligible"] != float64(3) || coverage["converted"] != float64(3) || coverage["status"] != "complete" || coverage["totals"].(map[string]any)["outflow"] != "62.00" {
		t.Fatalf("projected response = %+v", body)
	}
	raw := body["totals"].([]any)[0].(map[string]any)
	if raw["currency_code"] != "BRL" || raw["outflow"] != "62.00" || raw["balance"] != "-62.00" {
		t.Fatalf("projected raw totals = %+v", raw)
	}
	items := body["items"].([]any)
	if len(items) != 2 || items[1].(map[string]any)["source"] != "projetado" || items[1].(map[string]any)["amount"] != "20" {
		t.Fatalf("projected items = %+v", items)
	}
}
