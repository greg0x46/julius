package httpapi_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/categories"
	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/money"
)

func TestReferenceCurrencyValuesReportsWithoutChangingRecords(t *testing.T) {
	srv, conn := newTestServer(t)
	holding := insertInvestment(t, conn)
	account := investmentCall(t, srv, http.MethodPost, "/api/accounts", map[string]any{"name": "Conta de teste", "currency_code": "BRL", "opening_balance": "500"}, 201)
	now := time.Now().UTC()
	day := now.Format("2006-01-02")
	category, err := categories.Create(context.Background(), conn, "Mercado sintético", money.Expense, "", "")
	if err != nil {
		t.Fatal(err)
	}
	transaction := investmentCall(t, srv, http.MethodPost, "/api/transactions", map[string]any{"account_id": account["id"], "description": "Compra sintética", "amount": "-50", "occurred_at": day, "category_id": category.ID}, 201)
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
	for date, rate := range map[string]string{day: "0.2", yesterday: "0.25"} {
		_, err := conn.Exec(`INSERT INTO exchange_rates (base_currency, quote_currency, as_of, effective_on, rate, source, fetched_at) VALUES ('BRL','USD',?,?,?,?,?)`, date, date, rate, "synthetic", db.FormatTime(now))
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, code := range []string{"BRL", "USD", "BRL"} {
		investmentCall(t, srv, http.MethodPut, "/api/preferences", map[string]any{"transactions_period_basis": "occurred_at", "reference_currency": code}, 200)
		summary := investmentCall(t, srv, http.MethodGet, "/api/investment-summary", nil, 200)
		expected := "1000.50"
		cash := "450"
		expense := "50"
		if code == "USD" {
			expected = "200.10"
			cash = "90"
			expense = "10"
		}
		assertMoney(t, "investment total", summary["total_value"], expected)
		if summary["currency_code"] != code {
			t.Fatalf("summary currency = %v", summary["currency_code"])
		}
		timeline := investmentCall(t, srv, http.MethodGet, "/api/timeline?reference_date="+day+"&from="+day+"&to="+day, nil, 200)
		assertMoney(t, "timeline cash", timeline["base"].(map[string]any)["starting_balance"], cash)
		assertMoney(t, "timeline expense", timeline["period_totals"].(map[string]any)["expense"], expense)
		for _, route := range []string{"/api/transactions/spending-by-category?timezone=UTC", "/api/transactions/category-breakdown?timezone=UTC&classification=outflow&period=all", "/api/transactions/category-breakdown?timezone=UTC&classification=outflow&month=" + day[:7]} {
			report := investmentCall(t, srv, http.MethodGet, route, nil, 200)
			if report["currency_code"] != code {
				t.Fatalf("category currency = %v", report["currency_code"])
			}
			assertMoney(t, "category expense", report["total"], expense)
		}
		query := investmentCall(t, srv, http.MethodPost, "/api/transactions/query", map[string]any{"timezone": "UTC", "group_by": "day", "page": 1, "page_size": 50, "filters": map[string]any{}}, 200)
		if query["currency_code"] != code || query["fx"].(map[string]any)["source"] == "" {
			t.Fatalf("query valuation = %+v", query)
		}
		report := query["reporting"].(map[string]any)["totals"].(map[string]any)
		if report["currency_code"] != code {
			t.Fatalf("query report currency = %+v", report)
		}
		assertMoney(t, "query expense", report["outflow"], expense)
		assertMoney(t, "native query expense", query["totals"].([]any)[0].(map[string]any)["outflow"], "50")
		assertMoney(t, "group expense", query["groups"].([]any)[0].(map[string]any)["reporting"].(map[string]any)["totals"].(map[string]any)["outflow"], expense)
		item := query["items"].([]any)[0].(map[string]any)
		assertMoney(t, "native transaction", item["amount"], "-50")
		assertMoney(t, "native effective money", item["effective_money"].(map[string]any)["value"], "-50")
		assertMoney(t, "native reporting amount", item["reportable_amount"], "50")
		worth := investmentCall(t, srv, http.MethodGet, "/api/net-worth", nil, 200)
		latest := worth["latest"].(map[string]any)
		if latest["currency_code"] != code {
			t.Fatalf("wealth currency = %+v", latest)
		}
		assertMoney(t, "wealth cash", latest["cash_balance"], cash)
	}
	var native, currency string
	if err := conn.QueryRow(`SELECT balance,currency_code FROM financial_investments WHERE id=?`, holding).Scan(&native, &currency); err != nil {
		t.Fatal(err)
	}
	if native != "1000.50" || currency != "BRL" {
		t.Fatalf("holding changed: %s %s", native, currency)
	}
	original := investmentCall(t, srv, http.MethodGet, "/api/accounts/"+account["id"].(string), nil, 200)
	assertMoney(t, "original balance", original["balance"], "450")
	if err := conn.QueryRow(`SELECT amount,currency_code FROM financial_transactions WHERE id=?`, transaction["id"]).Scan(&native, &currency); err != nil {
		t.Fatal(err)
	}
	if native != "-50" || currency != "BRL" {
		t.Fatalf("transaction changed: %s %s", native, currency)
	}
	for _, invalid := range []any{nil, 123, "", "usd", "ZZZ", "XXX", " USD"} {
		investmentCall(t, srv, http.MethodPut, "/api/preferences", map[string]any{"transactions_period_basis": "paid_at", "reference_currency": invalid}, 422)
	}
	preferences := investmentCall(t, srv, http.MethodGet, "/api/preferences", nil, 200)
	if preferences["transactions_period_basis"] != "occurred_at" || preferences["reference_currency"] != "BRL" {
		t.Fatalf("invalid request changed settings: %+v", preferences)
	}
}
