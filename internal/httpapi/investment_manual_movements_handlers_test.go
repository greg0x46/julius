package httpapi_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/investments"
)

func manualMovementBody(investmentID, kind, day string, extra map[string]any) map[string]any {
	body := map[string]any{"investment_id": investmentID, "kind": kind, "trade_date": day}
	for key, value := range extra {
		body[key] = value
	}
	return body
}

func wantNoYield(t *testing.T, item map[string]any, reason string) {
	t.Helper()
	if item["yield_value"] != nil || item["yield_unavailable_reason"] != reason {
		t.Errorf("%v: yield_value = %v, reason = %v, want none (%s)", item["name"], item["yield_value"], item["yield_unavailable_reason"], reason)
	}
}

func investmentTransactions(t *testing.T, baseURL, investmentID string) []map[string]any {
	t.Helper()
	resp, err := testGet(t, baseURL+"/api/investments/"+investmentID+"/transactions")
	if err != nil {
		t.Fatalf("GET transactions: %v", err)
	}
	var list []map[string]any
	decodeJSON(t, resp, &list)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET transactions status = %d, want 200", resp.StatusCode)
	}
	return list
}

// A holding the provider reports without movements has no history to derive a
// yield from. A purchase the owner records gives it one, and the sync, which
// never touches the table, cannot take it away.
func TestManualMovementGivesProviderHoldingAHistory(t *testing.T) {
	srv, conn := newTestServer(t)
	id := insertSyncedEquity(t, conn, "Ação A", "AAA3", "10", "1100")
	quoteToday(t, conn, "Ação A", "AAA3", "110")
	wantNoYield(t, listInvestmentsByName(t, srv.URL)["Ação A"], "sem_historico")

	created := investmentCall(t, srv, http.MethodPost, "/api/investment-movements",
		manualMovementBody(id, "BUY", "2026-01-05", map[string]any{"quantity": "10", "unit_price": "100"}), http.StatusCreated)
	if created["origin"] != "manual" || created["movement_type"] != "BUY" || created["direction"] != "inflow" {
		t.Fatalf("created = %+v, want a manual inflow BUY", created)
	}
	if created["amount"] != "1000" || created["quantity"] != "10" || created["value"] != "100" {
		t.Fatalf("created amounts = %v/%v/%v, want quantity × unit price", created["amount"], created["quantity"], created["value"])
	}
	wantYield(t, listInvestmentsByName(t, srv.URL)["Ação A"], "100.00", "calculado")

	list := investmentTransactions(t, srv.URL, id)
	if len(list) != 1 || list[0]["origin"] != "manual" || list[0]["id"] != created["id"] {
		t.Fatalf("transactions = %+v, want the manual purchase", list)
	}

	investmentCall(t, srv, http.MethodDelete, "/api/investment-movements/"+created["id"].(string), nil, http.StatusNoContent)
	wantNoYield(t, listInvestmentsByName(t, srv.URL)["Ação A"], "sem_historico")
	if got := len(investmentTransactions(t, srv.URL, id)); got != 0 {
		t.Fatalf("transactions after delete = %d, want 0", got)
	}
}

func TestManualMovementUpdateAndOrdering(t *testing.T) {
	srv, conn := newTestServer(t)
	id := insertSyncedEquity(t, conn, "Ação A", "AAA3", "10", "1100")
	quoteToday(t, conn, "Ação A", "AAA3", "110")
	insertInvestmentMovement(t, conn, id, "INTEREST", "outflow", "5", nil)

	created := investmentCall(t, srv, http.MethodPost, "/api/investment-movements",
		manualMovementBody(id, "BUY", "2026-01-05", map[string]any{"quantity": "10", "amount": "950.50", "notes": "  corretora antiga "}), http.StatusCreated)
	if created["notes"] != "corretora antiga" {
		t.Fatalf("notes = %v, want trimmed text", created["notes"])
	}
	updated := investmentCall(t, srv, http.MethodPut, "/api/investment-movements/"+created["id"].(string),
		manualMovementBody(id, "BUY", "2026-01-06", map[string]any{"quantity": "10", "amount": "1000"}), http.StatusOK)
	if updated["amount"] != "1000" || updated["notes"] != nil {
		t.Fatalf("updated = %+v, want amount 1000 and no notes", updated)
	}

	list := investmentTransactions(t, srv.URL, id)
	if len(list) != 2 {
		t.Fatalf("transactions = %d, want provider and manual", len(list))
	}
	origins := []any{list[0]["origin"], list[1]["origin"]}
	if origins[0] != "provider" || origins[1] != "manual" {
		t.Fatalf("origins = %v, want the provider movement (today) before the manual one (January)", origins)
	}
}

func TestManualMovementRefusals(t *testing.T) {
	srv, conn := newTestServer(t)
	id := insertSyncedEquity(t, conn, "Ação A", "AAA3", "10", "1100")
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	missing := "00000000-0000-4000-8000-000000000000"

	for name, c := range map[string]struct {
		body   map[string]any
		status int
	}{
		"unknown kind":     {manualMovementBody(id, "TAX", "2026-01-05", map[string]any{"amount": "1"}), http.StatusBadRequest},
		"future date":      {manualMovementBody(id, "BUY", tomorrow, map[string]any{"quantity": "1", "amount": "1"}), http.StatusBadRequest},
		"bad date":         {manualMovementBody(id, "BUY", "05/01/2026", map[string]any{"quantity": "1", "amount": "1"}), http.StatusBadRequest},
		"purchase no qty":  {manualMovementBody(id, "BUY", "2026-01-05", map[string]any{"amount": "1"}), http.StatusBadRequest},
		"zero amount":      {manualMovementBody(id, "SELL", "2026-01-05", map[string]any{"quantity": "1", "amount": "0"}), http.StatusBadRequest},
		"negative price":   {manualMovementBody(id, "BUY", "2026-01-05", map[string]any{"quantity": "1", "unit_price": "-1", "amount": "1"}), http.StatusBadRequest},
		"unknown holding":  {manualMovementBody(missing, "BUY", "2026-01-05", map[string]any{"quantity": "1", "amount": "1"}), http.StatusNotFound},
		"unknown property": {manualMovementBody(id, "BUY", "2026-01-05", map[string]any{"quantity": "1", "amount": "1", "origin": "provider"}), http.StatusBadRequest},
	} {
		t.Run(name, func(t *testing.T) {
			investmentProblem(t, srv, http.MethodPost, "/api/investment-movements", c.body, c.status)
		})
	}
	if got := countRows(t, conn, `SELECT COUNT(*) FROM manual_investment_movements`); got != 0 {
		t.Fatalf("refused requests stored %d rows", got)
	}

	investmentProblem(t, srv, http.MethodPut, "/api/investment-movements/"+missing,
		manualMovementBody(id, "BUY", "2026-01-05", map[string]any{"quantity": "1", "amount": "1"}), http.StatusNotFound)
	investmentProblem(t, srv, http.MethodDelete, "/api/investment-movements/"+missing, nil, http.StatusNotFound)

	// A movement stays with the holding it was recorded for.
	other := insertSyncedEquity(t, conn, "Ação B", "BBB3", "5", "650")
	created := investmentCall(t, srv, http.MethodPost, "/api/investment-movements",
		manualMovementBody(id, "BUY", "2026-01-05", map[string]any{"quantity": "1", "amount": "1"}), http.StatusCreated)
	investmentProblem(t, srv, http.MethodPut, "/api/investment-movements/"+created["id"].(string),
		manualMovementBody(other, "BUY", "2026-01-05", map[string]any{"quantity": "1", "amount": "1"}), http.StatusBadRequest)
}

// The market backfill prices an asset from the first day something held it. A
// purchase recorded for a provider holding is such a day; a sale or a provento
// is not.
func TestManualPurchaseMakesTheAssetNeedHistory(t *testing.T) {
	srv, conn := newTestServer(t)
	id := insertSyncedEquity(t, conn, "Ação A", "AAA3", "10", "1100")
	assetID := quoteToday(t, conn, "Ação A", "AAA3", "110")
	ctx := context.Background()

	if from, err := investments.AssetHeldFrom(ctx, conn, assetID); err != nil || from != nil {
		t.Fatalf("held from = %v, %v before any purchase, want nil", from, err)
	}
	investmentCall(t, srv, http.MethodPost, "/api/investment-movements",
		manualMovementBody(id, "BUY", "2026-03-10", map[string]any{"quantity": "4", "amount": "400"}), http.StatusCreated)
	investmentCall(t, srv, http.MethodPost, "/api/investment-movements",
		manualMovementBody(id, "BUY", "2026-02-02", map[string]any{"quantity": "6", "amount": "600"}), http.StatusCreated)
	investmentCall(t, srv, http.MethodPost, "/api/investment-movements",
		manualMovementBody(id, "INTEREST", "2026-01-02", map[string]any{"amount": "3"}), http.StatusCreated)

	from, err := investments.AssetHeldFrom(ctx, conn, assetID)
	if err != nil || from == nil || from.Format("2006-01-02") != "2026-02-02" {
		t.Fatalf("held from = %v, %v, want 2026-02-02", from, err)
	}
}
