package httpapi_test

import (
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/investments"
)

func TestInvestmentHistoryPeriodsAndCoverage(t *testing.T) {
	srv, conn := newTestServer(t)
	today := investments.ProviderDay(time.Now())
	from := today.AddDate(0, -2, 0).Format("2006-01-02")
	// A snapshot alone cannot establish the holding's past performance.
	insertSyncedEquity(t, conn, "Sem historico", "HIST11", "10", "1000")
	got := investmentCall(t, srv, http.MethodGet, "/api/investment-history?from="+from+"&to=9999-12-31", nil, http.StatusOK)
	periods := got["periods"].([]any)
	if len(periods) != 3 {
		t.Fatalf("periods = %d", len(periods))
	}
	if got["baseline"].(map[string]any)["to"] != from {
		t.Fatal("baseline moved from requested start")
	}
	previous := from
	for _, raw := range periods {
		period := raw.(map[string]any)
		if period["from"] != previous {
			t.Fatalf("non-contiguous periods: %+v", period)
		}
		end, err := time.Parse("2006-01-02", period["to"].(string))
		if err != nil {
			t.Fatal(err)
		}
		previous = end.AddDate(0, 0, 1).Format("2006-01-02")
		totals := period["totals"].(map[string]any)
		if totals["partial"] != true {
			t.Fatalf("missing history presented as complete: %+v", period)
		}
	}
	if periods[len(periods)-1].(map[string]any)["to"] != today.Format("2006-01-02") {
		t.Fatal("forecast escaped clamp")
	}
}

func TestInvestmentHistoryRejectsInvalidRanges(t *testing.T) {
	srv, _ := newTestServer(t)
	for _, query := range []string{"", "from=bad&to=2026-01-01", "from=2026-02-01&to=2026-01-01", "from=1900-01-01&to=2026-01-01"} {
		investmentCall(t, srv, http.MethodGet, "/api/investment-history?"+query, nil, http.StatusBadRequest)
	}
}

func TestInvestmentHistoryReusesPeriodCalculation(t *testing.T) {
	srv, conn := newTestServer(t)
	today := investments.ProviderDay(time.Now())
	from := today.Format("2006-01-02")
	id := insertSyncedEquity(t, conn, "Synthetic fund", "YILD11", "10", "1100")
	quantity := "10"
	insertInvestmentMovement(t, conn, id, "BUY", "inflow", "1000", &quantity)
	quoteToday(t, conn, "Synthetic fund", "YILD11", "110")
	history := investmentCall(t, srv, http.MethodGet, "/api/investment-history?from="+from+"&to="+from, nil, http.StatusOK)
	period := investmentCall(t, srv, http.MethodGet, "/api/investment-portfolio?from="+from+"&to="+from, nil, http.StatusOK)
	point := history["periods"].([]any)[0].(map[string]any)
	if !reflect.DeepEqual(point["totals"], period["totals"]) || !reflect.DeepEqual(point["groups"], period["groups"]) {
		t.Fatalf("history differs from portfolio calculation: %+v vs %+v", point, period)
	}
}

func TestInvestmentHistoryUsesEachEndpointsReferenceRate(t *testing.T) {
	srv, conn := newTestServer(t)
	today := investments.ProviderDay(time.Now())
	from := time.Date(today.Year(), today.Month()-1, 1, 0, 0, 0, 0, time.UTC)
	monthEnd := time.Date(today.Year(), today.Month(), 0, 0, 0, 0, 0, time.UTC)
	rates := map[string]string{from.Format("2006-01-02"): "0.2", monthEnd.Format("2006-01-02"): "0.25", today.Format("2006-01-02"): "0.3"}
	for day, rate := range rates {
		if _, err := conn.Exec(`INSERT INTO exchange_rates (base_currency, quote_currency, as_of, effective_on, rate, source, fetched_at) VALUES ('BRL','USD',?,?,?,?,?)`, day, day, rate, "synthetic", db.FormatTime(time.Now())); err != nil {
			t.Fatal(err)
		}
	}
	investmentCall(t, srv, http.MethodPut, "/api/preferences", map[string]any{"transactions_period_basis": "occurred_at", "reference_currency": "USD"}, http.StatusOK)
	got := investmentCall(t, srv, http.MethodGet, "/api/investment-history?from="+from.Format("2006-01-02")+"&to="+today.Format("2006-01-02"), nil, http.StatusOK)
	samples := append([]any{got["baseline"]}, got["periods"].([]any)...)
	for _, raw := range samples {
		sample := raw.(map[string]any)
		fx := sample["fx"].(map[string]any)
		assertMoney(t, "dated rate", fx["value"], rates[sample["to"].(string)])
		if sample["currency_code"] != "USD" || fx["source"] != "synthetic" {
			t.Fatalf("lost valuation provenance: %+v", sample)
		}
	}
}
