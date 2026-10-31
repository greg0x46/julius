package httpapi_test

import (
	"net/http"
	"testing"
	"time"
)

const categorySupermercadoID = "000433b6-3094-5a9c-87df-465b70574a4b"
const categorySalarioID = "3c5a9586-2a11-556d-b014-692ed51c3997"

func TestHistoricalTimelineReturnsAValidLowestPoint(t *testing.T) {
	srv, _ := newTestServer(t)
	for _, window := range []struct{ from, to string }{
		{"2026-08-01", "2026-08-31"},
		{"2025-01-01", "2025-12-31"},
		{"2026-08-10", "2026-08-10"},
	} {
		t.Run(window.from+"/"+window.to, func(t *testing.T) {
			resp := doJSON(t, http.MethodGet, srv.URL+"/api/timeline?reference_date=2026-09-08&from="+window.from+"&to="+window.to+"", nil)
			if resp.StatusCode != 200 {
				t.Fatalf("status = %d", resp.StatusCode)
			}
			var body map[string]any
			decodeJSON(t, resp, &body)
			base := body["base"].(map[string]any)
			lowest := base["lowest_balance"].(map[string]any)
			if lowest["date"] != window.from || lowest["lowest_tier"] != "realizado" || lowest["balance"] != "0" {
				t.Fatalf("invalid historical minimum: %+v", lowest)
			}
			if base["first_negative"] != nil {
				t.Fatalf("historical window has a future warning: %v", base["first_negative"])
			}
		})
	}
}

func TestTimelineOverHTTP(t *testing.T) {
	srv, conn := newTestServer(t)

	groceriesID := insertTransaction(t, conn)
	setCardDebtTransaction(t, conn, groceriesID, time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC), "100.00", "DEBIT", nil)
	resp := doJSON(t, http.MethodPut, srv.URL+"/api/transactions/"+groceriesID+"/category",
		map[string]any{"category_id": categorySupermercadoID})
	if resp.StatusCode != 200 {
		t.Fatalf("set category status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	salaryID := insertTransaction(t, conn)
	setCardDebtTransaction(t, conn, salaryID, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), "2000.00", "CREDIT", nil)
	resp = doJSON(t, http.MethodPut, srv.URL+"/api/transactions/"+salaryID+"/category",
		map[string]any{"category_id": categorySalarioID})
	if resp.StatusCode != 200 {
		t.Fatalf("set category status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	var accountID string
	if err := conn.QueryRow(`SELECT account_id FROM financial_transactions WHERE id = ?`, groceriesID).Scan(&accountID); err != nil {
		t.Fatalf("account id: %v", err)
	}
	if _, err := conn.Exec(`UPDATE financial_accounts SET balance = '5000.00' WHERE id = ?`, accountID); err != nil {
		t.Fatalf("set balance: %v", err)
	}

	resp = doJSON(t, http.MethodGet,
		srv.URL+"/api/timeline?reference_date=2026-08-15&from=2026-08-01&to=2026-08-31&timezone=UTC", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("get timeline status = %d, want 200", resp.StatusCode)
	}
	var body map[string]any
	decodeJSON(t, resp, &body)

	base := body["base"].(map[string]any)
	if base["starting_balance"] != "5000.00" {
		t.Errorf("starting_balance = %v, want 5000.00", base["starting_balance"])
	}
	entries := base["entries"].([]any)
	if len(entries) != 2 {
		t.Fatalf("entries = %+v, want 2", entries)
	}

	// Reconciliation: period_totals must agree with the real Entries —
	// no presentation layer is allowed to recompute this independently.
	periodTotals := body["period_totals"].(map[string]any)
	if periodTotals["income"] != "2000.00" || periodTotals["expense"] != "100.00" || periodTotals["result"] != "1900.00" {
		t.Errorf("period_totals = %+v", periodTotals)
	}
}

func TestTimelineWithScenarioIDsReturnsSimulationAndImpacts(t *testing.T) {
	srv, conn := newTestServer(t)

	txID := insertTransaction(t, conn)
	// Pin occurred_at before reference_date, the way every other timeline
	// test here does: insertTransaction defaults to time.Now(), and a real
	// transaction landing after the reference date would shift the base
	// series' final balance, making the scenario no longer "the only source
	// of difference" this test asserts on.
	setCardDebtTransaction(t, conn, txID, time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC), "-42.00", "DEBIT", nil)
	var accountID string
	if err := conn.QueryRow(`SELECT account_id FROM financial_transactions WHERE id = ?`, txID).Scan(&accountID); err != nil {
		t.Fatalf("account id: %v", err)
	}
	if _, err := conn.Exec(`UPDATE financial_accounts SET balance = '5000.00' WHERE id = ?`, accountID); err != nil {
		t.Fatalf("set balance: %v", err)
	}

	resp := doJSON(t, http.MethodPost, srv.URL+"/api/scenarios", map[string]any{"name": "Viagem"})
	if resp.StatusCode != 201 {
		t.Fatalf("create scenario status = %d, want 201", resp.StatusCode)
	}
	var scenario map[string]any
	decodeJSON(t, resp, &scenario)
	scenarioID := scenario["id"].(string)

	resp = doJSON(t, http.MethodPost, srv.URL+"/api/scenarios/"+scenarioID+"/transactions", map[string]any{
		"description": "Passagem", "amount": "-300.00", "projected_at": "2026-08-20",
	})
	if resp.StatusCode != 201 {
		t.Fatalf("create scenario transaction status = %d, want 201", resp.StatusCode)
	}
	resp.Body.Close()

	// Without scenario_ids: simulation is null, no impacts.
	resp = doJSON(t, http.MethodGet,
		srv.URL+"/api/timeline?reference_date=2026-08-15&from=2026-08-01&to=2026-08-31", nil)
	var withoutScenario map[string]any
	decodeJSON(t, resp, &withoutScenario)
	if withoutScenario["simulation"] != nil {
		t.Errorf("simulation = %v, want nil", withoutScenario["simulation"])
	}
	if impacts := withoutScenario["scenario_impacts"].([]any); len(impacts) != 0 {
		t.Errorf("scenario_impacts = %+v, want empty", impacts)
	}

	// With scenario_ids: simulation reflects the scenario, and the impact
	// equals simulation.final_balance - base.final_balance (-300 here,
	// since the scenario is the only source of difference).
	resp = doJSON(t, http.MethodGet,
		srv.URL+"/api/timeline?reference_date=2026-08-15&from=2026-08-01&to=2026-08-31&scenario_ids="+scenarioID, nil)
	var withScenario map[string]any
	decodeJSON(t, resp, &withScenario)
	simulation := withScenario["simulation"].(map[string]any)
	simulationPoints := simulation["points"].([]any)
	basePoints := withScenario["base"].(map[string]any)["points"].([]any)
	simulationFinal := simulationPoints[len(simulationPoints)-1].(map[string]any)["balance"].(string)
	baseFinal := basePoints[len(basePoints)-1].(map[string]any)["balance"].(string)
	if simulationFinal != "4700.00" || baseFinal != "5000.00" {
		t.Errorf("simulationFinal = %q, baseFinal = %q, want 4700.00 and 5000.00", simulationFinal, baseFinal)
	}

	impacts := withScenario["scenario_impacts"].([]any)
	if len(impacts) != 1 {
		t.Fatalf("scenario_impacts = %+v, want 1", impacts)
	}
	impact := impacts[0].(map[string]any)
	if impact["scenario_id"] != scenarioID || impact["delta"] != "-300.00" {
		t.Errorf("impact = %+v, want delta -300.00", impact)
	}
}

func TestTimelineRejectsMissingParams(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := doJSON(t, http.MethodGet, srv.URL+"/api/timeline", nil)
	if resp.StatusCode != 422 {
		t.Fatalf("status = %d, want 422", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestTimelineRejectsToBeforeFrom(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := doJSON(t, http.MethodGet,
		srv.URL+"/api/timeline?reference_date=2026-08-15&from=2026-08-31&to=2026-08-01", nil)
	if resp.StatusCode != 422 {
		t.Fatalf("status = %d, want 422", resp.StatusCode)
	}
	resp.Body.Close()
}

// The Home dashboard's "Todo Período" option asks for these bounds instead
// of guessing a start date or requesting a decade of days to be safe.
func TestTimelineDataRangeSpansOldestTransactionToLastPlannedInstallment(t *testing.T) {
	srv, conn := newTestServer(t)

	txID := insertTransaction(t, conn)
	setCardDebtTransaction(t, conn, txID, time.Date(2024, 3, 7, 0, 0, 0, 0, time.UTC), "-42.00", "DEBIT", nil)

	resp := doJSON(t, http.MethodGet, srv.URL+"/api/timeline/range?timezone=UTC", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("get range status = %d, want 200", resp.StatusCode)
	}
	var body map[string]any
	decodeJSON(t, resp, &body)
	if body["from"] != "2024-03-07" {
		t.Errorf("from = %v, want 2024-03-07 (the oldest transaction's day)", body["from"])
	}
	// No planned installment yet, so `to` is the current month's end — never
	// a past date, whatever the transaction history looks like.
	now := time.Now().UTC()
	monthEnd := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, -1).Format("2006-01-02")
	if body["to"] != monthEnd {
		t.Errorf("to = %v, want %s (this month's end)", body["to"], monthEnd)
	}

	resp = doJSON(t, http.MethodPost, srv.URL+"/api/scenarios", map[string]any{"name": "Notebook"})
	if resp.StatusCode != 201 {
		t.Fatalf("create scenario status = %d, want 201", resp.StatusCode)
	}
	var scenario map[string]any
	decodeJSON(t, resp, &scenario)
	lastInstallment := time.Date(now.Year()+2, 6, 15, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	resp = doJSON(t, http.MethodPost, srv.URL+"/api/scenarios/"+scenario["id"].(string)+"/transactions", map[string]any{
		"description": "Parcela final", "amount": "-300.00", "projected_at": lastInstallment,
	})
	if resp.StatusCode != 201 {
		t.Fatalf("create scenario transaction status = %d, want 201", resp.StatusCode)
	}
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/timeline/range?timezone=UTC", nil)
	decodeJSON(t, resp, &body)
	if body["to"] != lastInstallment {
		t.Errorf("to = %v, want %s (the last planned installment)", body["to"], lastInstallment)
	}
	if body["from"] != "2024-03-07" {
		t.Errorf("from = %v, want 2024-03-07", body["from"])
	}
}

func TestTimelineTimezoneControlsDayAndRejectsInvalidZone(t *testing.T) {
	srv, conn := newTestServer(t)
	id := insertTransaction(t, conn)
	setCardDebtTransaction(t, conn, id, time.Date(2026, 3, 16, 1, 0, 0, 0, time.UTC), "10", "DEBIT", nil)
	resp := doJSON(t, http.MethodPut, srv.URL+"/api/transactions/"+id+"/category", map[string]any{"category_id": categorySupermercadoID})
	resp.Body.Close()
	for _, tc := range []struct {
		zone  string
		count int
	}{{"America%2FSao_Paulo", 1}, {"UTC", 0}} {
		resp = doJSON(t, http.MethodGet, srv.URL+"/api/timeline?reference_date=2026-03-16&from=2026-03-15&to=2026-03-15&timezone="+tc.zone, nil)
		if resp.StatusCode != 200 {
			t.Fatalf("zone %s status = %d", tc.zone, resp.StatusCode)
		}
		var body map[string]any
		decodeJSON(t, resp, &body)
		entries := body["base"].(map[string]any)["entries"].([]any)
		if len(entries) != tc.count {
			t.Fatalf("zone %s entries = %v", tc.zone, entries)
		}
	}
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/timeline?reference_date=2026-03-16&from=2026-03-15&to=2026-03-15&timezone=invalid", nil)
	defer resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("invalid timezone status = %d", resp.StatusCode)
	}
}

func TestTimelineDataRangeUsesReportingTimezone(t *testing.T) {
	srv, conn := newTestServer(t)
	id := insertTransaction(t, conn)
	setCardDebtTransaction(t, conn, id, time.Date(2026, 3, 16, 1, 0, 0, 0, time.UTC), "10", "DEBIT", nil)
	for _, tc := range []struct{ zone, from string }{
		{"America%2FSao_Paulo", "2026-03-15"},
		{"UTC", "2026-03-16"},
		{"", "2026-03-15"}, // the configured zone defaults to America/Sao_Paulo
	} {
		resp := doJSON(t, http.MethodGet, srv.URL+"/api/timeline/range?timezone="+tc.zone, nil)
		if resp.StatusCode != 200 {
			t.Fatalf("zone %s status = %d", tc.zone, resp.StatusCode)
		}
		var body map[string]any
		decodeJSON(t, resp, &body)
		if body["from"] != tc.from {
			t.Errorf("zone %s from = %v, want %s", tc.zone, body["from"], tc.from)
		}
	}
	resp := doJSON(t, http.MethodGet, srv.URL+"/api/timeline/range?timezone=invalid", nil)
	defer resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("invalid timezone status = %d", resp.StatusCode)
	}
}
