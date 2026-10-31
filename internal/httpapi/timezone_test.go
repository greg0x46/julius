package httpapi_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// defaultZone is the zone calendar logic uses until the owner changes it.
func defaultZone(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func setTimezone(t *testing.T, srvURL, zone string) {
	t.Helper()
	resp := doJSON(t, http.MethodPut, srvURL+"/api/preferences", map[string]any{"transactions_period_basis": "occurred_at", "timezone": zone})
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("set timezone %s: status = %d", zone, resp.StatusCode)
	}
}

func TestPreferencesTimezone(t *testing.T) {
	srv, _ := newTestServer(t)

	var got map[string]any
	decodeJSON(t, doJSON(t, http.MethodGet, srv.URL+"/api/preferences", nil), &got)
	if got["timezone"] != "America/Sao_Paulo" {
		t.Fatalf("default timezone = %v", got["timezone"])
	}

	resp := doJSON(t, http.MethodPut, srv.URL+"/api/preferences", map[string]any{"transactions_period_basis": "paid_at", "timezone": "Asia/Tokyo"})
	if resp.StatusCode != 200 {
		t.Fatalf("put status = %d", resp.StatusCode)
	}
	decodeJSON(t, resp, &got)
	if got["timezone"] != "Asia/Tokyo" || got["reference_currency"] != "BRL" {
		t.Fatalf("put response = %v", got)
	}

	// Older clients omit the timezone; the saved one is kept, and the timezone
	// alone never resets the currency.
	resp = doJSON(t, http.MethodPut, srv.URL+"/api/preferences", map[string]any{"transactions_period_basis": "occurred_at"})
	if resp.StatusCode != 200 {
		t.Fatalf("partial put status = %d", resp.StatusCode)
	}
	decodeJSON(t, resp, &got)
	if got["timezone"] != "Asia/Tokyo" {
		t.Fatalf("omitted timezone = %v, want the saved Asia/Tokyo", got["timezone"])
	}
	decodeJSON(t, doJSON(t, http.MethodGet, srv.URL+"/api/preferences", nil), &got)
	if got["timezone"] != "Asia/Tokyo" || got["transactions_period_basis"] != "occurred_at" {
		t.Fatalf("persisted = %v", got)
	}

	for _, invalid := range []any{"", "Local", "Mars/Olympus", " UTC", 5, nil} {
		resp = doJSON(t, http.MethodPut, srv.URL+"/api/preferences", map[string]any{"transactions_period_basis": "occurred_at", "timezone": invalid})
		var problem map[string]any
		if resp.StatusCode != 422 {
			t.Errorf("timezone %v: status = %d, want 422", invalid, resp.StatusCode)
			resp.Body.Close()
			continue
		}
		decodeJSON(t, resp, &problem)
		if problem["detail"] != "Escolha um fuso horário válido." {
			t.Errorf("timezone %v: detail = %v", invalid, problem["detail"])
		}
	}
	decodeJSON(t, doJSON(t, http.MethodGet, srv.URL+"/api/preferences", nil), &got)
	if got["timezone"] != "Asia/Tokyo" {
		t.Fatalf("a rejected update changed the timezone to %v", got["timezone"])
	}
}

// The timezone parameter is optional on every reporting endpoint: omitted, the
// configured zone applies, and an explicit one still wins. The transaction is
// Feb 28 22:00 in Sao Paulo and already Mar 1 in UTC.
func TestOmittedTimezoneFollowsTheConfiguredZone(t *testing.T) {
	srv, conn := newTestServer(t)
	id := insertTransaction(t, conn)
	setCardDebtTransaction(t, conn, id, time.Date(2026, 3, 1, 1, 0, 0, 0, time.UTC), "10", "DEBIT", nil)
	resp := doJSON(t, http.MethodPut, srv.URL+"/api/transactions/"+id+"/category", map[string]any{"category_id": categorySupermercadoID})
	resp.Body.Close()

	timelineEntries := func(query string) int {
		t.Helper()
		body := investmentCall(t, srv, http.MethodGet, "/api/timeline?reference_date=2026-02-28&from=2026-02-28&to=2026-02-28"+query, nil, 200)
		return len(body["base"].(map[string]any)["entries"].([]any))
	}
	rangeFrom := func(query string) string {
		t.Helper()
		return investmentCall(t, srv, http.MethodGet, "/api/timeline/range"+query, nil, 200)["from"].(string)
	}
	breakdown := func(month, query string) decimal.Decimal {
		t.Helper()
		body := investmentCall(t, srv, http.MethodGet, "/api/transactions/category-breakdown?classification=outflow&month="+month+query, nil, 200)
		return decimal.RequireFromString(body["total"].(string))
	}
	queryItems := func(body map[string]any) int {
		t.Helper()
		body["group_by"] = "none"
		body["filters"] = map[string]any{"date_from": "2026-02-28", "date_to": "2026-02-28"}
		return len(investmentCall(t, srv, http.MethodPost, "/api/transactions/query", body, 200)["items"].([]any))
	}

	// Default configuration: America/Sao_Paulo.
	if timelineEntries("") != 1 || rangeFrom("") != "2026-02-28" {
		t.Fatal("timeline without timezone ignored the default zone")
	}
	if !breakdown("2026-02", "").Equal(decimal.NewFromInt(10)) || !breakdown("2026-03", "").IsZero() {
		t.Fatal("category-breakdown without timezone ignored the default zone")
	}
	if queryItems(map[string]any{}) != 1 {
		t.Fatal("transactions query without timezone ignored the default zone")
	}
	// An explicit zone overrides the setting.
	if timelineEntries("&timezone=UTC") != 0 || rangeFrom("?timezone=UTC") != "2026-03-01" || !breakdown("2026-03", "&timezone=UTC").Equal(decimal.NewFromInt(10)) || queryItems(map[string]any{"timezone": "UTC"}) != 0 {
		t.Fatal("an explicit timezone no longer overrides the configured one")
	}

	setTimezone(t, srv.URL, "UTC")
	if timelineEntries("") != 0 || rangeFrom("") != "2026-03-01" {
		t.Fatal("timeline without timezone did not follow the new setting")
	}
	if !breakdown("2026-02", "").IsZero() || !breakdown("2026-03", "").Equal(decimal.NewFromInt(10)) {
		t.Fatal("category-breakdown without timezone did not follow the new setting")
	}
	if queryItems(map[string]any{}) != 0 {
		t.Fatal("transactions query without timezone did not follow the new setting")
	}
	if timelineEntries("&timezone=America%2FSao_Paulo") != 1 {
		t.Fatal("an explicit timezone no longer overrides the configured one")
	}

	// Present but invalid is still rejected; "Local" would mean the host zone.
	for _, zone := range []string{"Mars%2FOlympus", "Local"} {
		for path, want := range map[string]int{
			"/api/timeline?reference_date=2026-02-28&from=2026-02-28&to=2026-02-28&timezone=" + zone:  422,
			"/api/timeline/range?timezone=" + zone:                                                    422,
			"/api/transactions/spending-by-category?timezone=" + zone:                                 400,
			"/api/transactions/category-breakdown?classification=outflow&period=all&timezone=" + zone: 400,
		} {
			resp := doJSON(t, http.MethodGet, srv.URL+path, nil)
			resp.Body.Close()
			if resp.StatusCode != want {
				t.Errorf("%s: status = %d, want %d", path, resp.StatusCode, want)
			}
		}
	}
	for _, zone := range []string{"Mars/Olympus", "Local"} {
		resp := doJSON(t, http.MethodPost, srv.URL+"/api/transactions/query", map[string]any{"timezone": zone})
		resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Errorf("query timezone %q: status = %d, want 400", zone, resp.StatusCode)
		}
	}
}

// "This month" of spending-by-category is read in the configured zone.
// Kiritimati (UTC+14) and Pago Pago (UTC-11) are always on different days.
func TestSpendingByCategoryMonthUsesTheConfiguredZone(t *testing.T) {
	srv, _ := newTestServer(t)
	month := func(zone string) string {
		loc, err := time.LoadLocation(zone)
		if err != nil {
			t.Fatal(err)
		}
		return time.Now().In(loc).Format("2006-01")
	}
	check := func(label, query, zone string) {
		t.Helper()
		before := month(zone)
		got := investmentCall(t, srv, http.MethodGet, "/api/transactions/spending-by-category"+query, nil, 200)["month"]
		if after := month(zone); got != before && got != after {
			t.Errorf("%s: month = %v, want %s", label, got, before)
		}
	}
	check("default", "", "America/Sao_Paulo")
	setTimezone(t, srv.URL, "Pacific/Kiritimati")
	check("configured", "", "Pacific/Kiritimati")
	check("explicit overrides the setting", "?timezone=Pacific%2FPago_Pago", "Pacific/Pago_Pago")
}

// A manual transaction keeps the "noon of the chosen day" semantic in the
// configured zone, whatever the process zone is.
func TestManualTransactionDayIsAnchoredInTheConfiguredZone(t *testing.T) {
	srv, _ := newTestServer(t)
	account := investmentCall(t, srv, http.MethodPost, "/api/accounts", map[string]any{"name": "Conta de teste", "currency_code": "BRL", "opening_balance": "0"}, 201)
	for _, tc := range []struct{ zone, want string }{
		{"America/Sao_Paulo", "2026-03-16T15:00:00Z"},
		{"Asia/Tokyo", "2026-03-16T03:00:00Z"},
		{"Pacific/Kiritimati", "2026-03-15T22:00:00Z"},
		{"UTC", "2026-03-16T12:00:00Z"},
	} {
		setTimezone(t, srv.URL, tc.zone)
		created := investmentCall(t, srv, http.MethodPost, "/api/transactions", map[string]any{"account_id": account["id"], "description": "Teste " + tc.zone, "amount": "-5", "occurred_at": "2026-03-16"}, 201)
		got, err := time.Parse(time.RFC3339, created["occurred_at"].(string))
		if err != nil || got.UTC().Format(time.RFC3339) != tc.want {
			t.Errorf("%s: occurred_at = %v, want %s", tc.zone, created["occurred_at"], tc.want)
		}
		edited := investmentCall(t, srv, http.MethodPut, "/api/transactions/"+created["id"].(string), map[string]any{"account_id": account["id"], "description": "Editado", "amount": "-5", "occurred_at": "2026-03-17"}, 200)
		got, err = time.Parse(time.RFC3339, edited["occurred_at"].(string))
		if want, _ := time.Parse(time.RFC3339, tc.want); err != nil || !got.Equal(want.AddDate(0, 0, 1)) {
			t.Errorf("%s: edited occurred_at = %v, want one day after %s", tc.zone, edited["occurred_at"], tc.want)
		}
	}
}

// The card total reads "today" in the configured zone. Kiritimati (UTC+14) and
// Pago Pago (UTC-11) are on different days at every instant, so the bill that
// closes on Kiritimati's today has closed there and is still open in Pago Pago.
func TestTotalOwedCardCycleFollowsTheConfiguredZone(t *testing.T) {
	srv, conn := newTestServer(t)
	kiritimati, err := time.LoadLocation("Pacific/Kiritimati")
	if err != nil {
		t.Fatal(err)
	}
	y, m, d := time.Now().In(kiritimati).Date()
	closing := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)

	id := insertTransaction(t, conn)
	setCardDebtTransaction(t, conn, id, closing.AddDate(0, 0, -10).Add(12*time.Hour), "-50.00", "DEBIT", nil)
	insertBillForTransaction(t, conn, id, "bill-previous", closing.AddDate(0, 0, -28), closing.AddDate(0, 0, -20))
	insertBillForTransaction(t, conn, id, "bill-closing-on-kiritimati-today", closing, closing.AddDate(0, 0, 8))
	if _, err := conn.Exec(`UPDATE financial_accounts SET account_type = 'CREDIT', balance = '999.00'
		WHERE id = (SELECT account_id FROM financial_transactions WHERE id = ?)`, id); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct{ zone, want string }{
		{"Pacific/Kiritimati", "0"},
		{"Pacific/Pago_Pago", "50.00"},
	} {
		setTimezone(t, srv.URL, tc.zone)
		total := investmentCall(t, srv, http.MethodGet, "/api/payables/total-owed", nil, 200)
		if total["future_installments_total"] != tc.want {
			t.Errorf("%s: future_installments_total = %v, want %s", tc.zone, total["future_installments_total"], tc.want)
		}
	}
}

// A goal's projection counts the months left from today in the configured
// zone: a goal due on Kiritimati's today has nothing left to project there,
// while it is still a month away in Pago Pago.
func TestGoalProjectionFollowsTheConfiguredZone(t *testing.T) {
	srv, _ := newTestServer(t)
	kiritimati, err := time.LoadLocation("Pacific/Kiritimati")
	if err != nil {
		t.Fatal(err)
	}
	target := time.Now().In(kiritimati).Format("2006-01-02")
	investmentCall(t, srv, http.MethodPost, "/api/goals", map[string]any{"name": "Reserva", "metric": "invested_value", "target_amount": "1200", "target_date": target}, 201)
	for _, tc := range []struct {
		zone       string
		projection bool
	}{{"Pacific/Kiritimati", false}, {"Pacific/Pago_Pago", true}} {
		setTimezone(t, srv.URL, tc.zone)
		items := investmentCall(t, srv, http.MethodGet, "/api/goals", nil, 200)["items"].([]any)
		if got := items[0].(map[string]any)["projection"] != nil; got != tc.projection {
			t.Errorf("%s: projection present = %v, want %v", tc.zone, got, tc.projection)
		}
	}
}
