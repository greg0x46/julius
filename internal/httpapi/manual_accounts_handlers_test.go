package httpapi_test

import (
	"net/http"
	"testing"
)

func TestManualAccountLifecycle(t *testing.T) {
	srv, conn := newTestServer(t)
	create := func(currency string) map[string]any {
		resp := doJSON(t, "POST", srv.URL+"/api/accounts", map[string]any{"name": "Manual " + currency, "currency_code": currency, "opening_balance": "100"})
		if resp.StatusCode != 201 {
			t.Fatalf("create %s: %d", currency, resp.StatusCode)
		}
		var got map[string]any
		decodeJSON(t, resp, &got)
		return got
	}
	usd := create("USD")
	create("EUR")
	nativeOnly := create("XTS")
	if nativeOnly["currency_code"] != "XTS" || nativeOnly["balance"] != "100" {
		t.Fatalf("native-only account: %+v", nativeOnly)
	}
	id := usd["id"].(string)
	for _, table := range []string{"data_sources", "raw_imports"} {
		var n int
		if err := conn.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("fake identity in %s: %d %v", table, n, err)
		}
	}
	if usd["external_id"] != nil || usd["balance"] != "100" || usd["origin"] != "manual" {
		t.Fatalf("account: %+v", usd)
	}
	write := map[string]any{"account_id": id, "description": "USD movement", "amount": "10", "occurred_at": "2026-10-01"}
	resp := doJSON(t, "POST", srv.URL+"/api/transactions", write)
	if resp.StatusCode != 201 {
		t.Fatalf("transaction: %d", resp.StatusCode)
	}
	var movement map[string]any
	decodeJSON(t, resp, &movement)
	txID := movement["id"].(string)
	assertBalance := func(want string) {
		t.Helper()
		got := getAccountJSON(t, srv.URL, id)
		if got["balance"] != want {
			t.Fatalf("balance: %+v, want %s", got, want)
		}
	}
	assertBalance("110")
	resp = doJSON(t, "PUT", srv.URL+"/api/accounts/"+id, map[string]any{"name": "Changed", "currency_code": "EUR", "opening_balance": "100"})
	if resp.StatusCode != 409 {
		t.Fatalf("currency change: %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = doJSON(t, "PUT", srv.URL+"/api/transactions/"+txID+"/inclusion", map[string]any{"state": "ignored"})
	resp.Body.Close()
	assertBalance("110")
	write["amount"] = "-20"
	resp = doJSON(t, "PUT", srv.URL+"/api/transactions/"+txID, write)
	if resp.StatusCode != 200 {
		t.Fatalf("edit movement: %d", resp.StatusCode)
	}
	resp.Body.Close()
	assertBalance("80")
	resp = doJSON(t, "POST", srv.URL+"/api/accounts/"+id+"/adjustments", map[string]any{"amount": "5", "description": "Correction"})
	if resp.StatusCode != 200 {
		t.Fatalf("adjust: %d", resp.StatusCode)
	}
	resp.Body.Close()
	assertBalance("85")
	resp = doJSON(t, "DELETE", srv.URL+"/api/transactions/"+txID, nil)
	if resp.StatusCode != 204 {
		t.Fatalf("delete: %d", resp.StatusCode)
	}
	resp.Body.Close()
	assertBalance("105")
	req := map[string]any{"name": "Archived USD", "currency_code": "USD", "opening_balance": "100", "archived": true}
	resp = doJSON(t, "PUT", srv.URL+"/api/accounts/"+id, req)
	if resp.StatusCode != 200 {
		t.Fatalf("archive: %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = doJSON(t, "POST", srv.URL+"/api/transactions", write)
	if resp.StatusCode == 201 {
		t.Fatal("archived account accepted movement")
	}
	resp.Body.Close()
	assertBalance("105")
	req["archived"] = false
	resp = doJSON(t, "PUT", srv.URL+"/api/accounts/"+id, req)
	if resp.StatusCode != 200 {
		t.Fatalf("reactivate: %d", resp.StatusCode)
	}
	resp.Body.Close()
	synced := insertAccount(t, conn, "BANK", "Synced", strPtr("500"), nil)
	resp = doJSON(t, "PUT", srv.URL+"/api/accounts/"+synced.accountID, req)
	if resp.StatusCode != 409 {
		t.Fatalf("synced edit: %d", resp.StatusCode)
	}
	resp.Body.Close()
	if got := getAccountJSON(t, srv.URL, synced.accountID); got["balance"] != "500" {
		t.Fatalf("synced balance: %+v", got)
	}
	resp = doJSON(t, "POST", srv.URL+"/api/accounts", map[string]any{"name": "Bad", "currency_code": "ZZZ"})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("invalid currency: %d", resp.StatusCode)
	}
	resp.Body.Close()
}
