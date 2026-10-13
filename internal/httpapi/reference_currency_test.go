package httpapi_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/db"
)

func TestReferenceCurrencyValuesReportsWithoutChangingRecords(t *testing.T) {
	srv, conn := newTestServer(t)
	holding := insertInvestment(t, conn)
	account := investmentCall(t, srv, http.MethodPost, "/api/accounts", map[string]any{"name": "Conta de teste", "currency_code": "BRL", "opening_balance": "500"}, 201)
	now := time.Now().UTC()
	day := now.Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
	for date, rate := range map[string]string{day: "0.2", yesterday: "0.25"} {
		_, err := conn.Exec(`INSERT INTO exchange_rates (base_currency, quote_currency, as_of, effective_on, rate, source, fetched_at) VALUES ('BRL','USD',?,?,?,?,?)`, date, date, rate, "synthetic", db.FormatTime(now))
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err := conn.Exec(`INSERT INTO net_worth_snapshots (id,captured_at,total_assets,total_liabilities,net_worth,cash_balance,investment_balance,credit_card_balance,payables_debt,is_backfilled,created_at,updated_at) VALUES ('historical',?,'400','0','400','400','0','0','0',FALSE,?,?)`, yesterday, db.FormatTime(now), db.FormatTime(now))
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"BRL", "USD", "BRL"} {
		investmentCall(t, srv, http.MethodPut, "/api/preferences", map[string]any{"transactions_period_basis": "occurred_at", "reference_currency": code}, 200)
		summary := investmentCall(t, srv, http.MethodGet, "/api/investment-summary", nil, 200)
		expected := "1000.50"
		cash := "500"
		if code == "USD" {
			expected = "200.10"
			cash = "100"
		}
		assertMoney(t, "investment total", summary["total_value"], expected)
		if summary["currency_code"] != code {
			t.Fatalf("summary currency = %v", summary["currency_code"])
		}
		timeline := investmentCall(t, srv, http.MethodGet, "/api/timeline?reference_date="+day+"&from="+day+"&to="+day, nil, 200)
		assertMoney(t, "timeline cash", timeline["base"].(map[string]any)["starting_balance"], cash)
		worth := investmentCall(t, srv, http.MethodGet, "/api/net-worth", nil, 200)
		latest := worth["latest"].(map[string]any)
		if latest["currency_code"] != code || latest["stored_currency"] != "BRL" {
			t.Fatalf("snapshot currency = %+v", latest)
		}
		assertMoney(t, "net worth cash", latest["cash_balance"], cash)
		if code == "USD" {
			historical := worth["series"].([]any)[0].(map[string]any)
			assertMoney(t, "historical FX", historical["net_worth"], "100")
			if historical["fx"].(map[string]any)["source"] != "synthetic" {
				t.Fatal("lost FX provenance")
			}
		}
	}
	var native, currency string
	if err := conn.QueryRow(`SELECT balance,currency_code FROM financial_investments WHERE id=?`, holding).Scan(&native, &currency); err != nil {
		t.Fatal(err)
	}
	if native != "1000.50" || currency != "BRL" {
		t.Fatalf("holding changed: %s %s", native, currency)
	}
	original := investmentCall(t, srv, http.MethodGet, "/api/accounts/"+account["id"].(string), nil, 200)
	assertMoney(t, "original balance", original["balance"], "500")
	var stored, storedCode string
	if err := conn.QueryRow(`SELECT net_worth,valuation_currency FROM net_worth_snapshots WHERE captured_at=?`, yesterday).Scan(&stored, &storedCode); err != nil {
		t.Fatal(err)
	}
	if stored != "400" || storedCode != "BRL" {
		t.Fatalf("snapshot rewritten: %s %s", stored, storedCode)
	}
	for _, invalid := range []string{"", "usd", "ZZZ", "XXX", " USD"} {
		investmentCall(t, srv, http.MethodPut, "/api/preferences", map[string]any{"transactions_period_basis": "paid_at", "reference_currency": invalid}, 422)
	}
	preferences := investmentCall(t, srv, http.MethodGet, "/api/preferences", nil, 200)
	if preferences["transactions_period_basis"] != "occurred_at" || preferences["reference_currency"] != "BRL" {
		t.Fatalf("invalid request changed settings: %+v", preferences)
	}
}
