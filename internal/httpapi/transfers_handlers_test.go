package httpapi_test

import (
	"github.com/greg0x46/julius/internal/transactions"
	"net/http"
	"testing"
)

func TestTransferHTTPCreateReadUnlinkAndValidation(t *testing.T) {
	srv, _ := newTestServer(t)
	createAccount := func(currency string) string {
		resp := doJSON(t, "POST", srv.URL+"/api/accounts", map[string]any{"name": "Synthetic " + currency, "currency_code": currency, "opening_balance": "1000"})
		if resp.StatusCode != 201 {
			t.Fatalf("account status: %d", resp.StatusCode)
		}
		var account map[string]any
		decodeJSON(t, resp, &account)
		return account["id"].(string)
	}
	source, destination := createAccount("BRL"), createAccount("USD")
	movement := func(account, amount string) map[string]any {
		return map[string]any{"account_id": account, "amount": amount, "description": "Synthetic transfer", "occurred_at": "2026-03-01"}
	}
	resp := doJSON(t, "POST", srv.URL+"/api/transfers", map[string]any{"source": movement(source, "-540"), "destination": movement(destination, "100"), "fee": movement(source, "-10")})
	if resp.StatusCode != 201 {
		t.Fatalf("create status: %d", resp.StatusCode)
	}
	var linked transactions.AccountTransfer
	decodeJSON(t, resp, &linked)
	if linked.EffectiveRate != "5.4" || linked.Fee == nil {
		t.Fatalf("transfer %+v", linked)
	}
	read := doJSON(t, "GET", srv.URL+"/api/transactions/"+linked.Source.TransactionID+"/transfer", nil)
	if read.StatusCode != 200 || read.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("read status %d cache %s", read.StatusCode, read.Header.Get("Cache-Control"))
	}
	var found transactions.AccountTransfer
	decodeJSON(t, read, &found)
	if found.ID != linked.ID {
		t.Fatal("different transfer")
	}
	duplicate := doJSON(t, "POST", srv.URL+"/api/transfers", map[string]any{"source_transaction_id": linked.Source.TransactionID, "destination_transaction_id": linked.Destination.TransactionID})
	duplicate.Body.Close()
	if duplicate.StatusCode != 409 {
		t.Fatalf("duplicate status: %d", duplicate.StatusCode)
	}
	invalid := doJSON(t, "POST", srv.URL+"/api/transfers", map[string]any{"source": movement(source, "-540"), "destination": movement(source, "100")})
	invalid.Body.Close()
	if invalid.StatusCode != 422 {
		t.Fatalf("invalid status: %d", invalid.StatusCode)
	}
	removed := doJSON(t, "DELETE", srv.URL+"/api/transfers/"+linked.ID, nil)
	removed.Body.Close()
	if removed.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status: %d", removed.StatusCode)
	}
	read = doJSON(t, "GET", srv.URL+"/api/transactions/"+linked.Destination.TransactionID+"/transfer", nil)
	var absent *transactions.AccountTransfer
	decodeJSON(t, read, &absent)
	if absent != nil {
		t.Fatal("link remains")
	}
	removed = doJSON(t, "DELETE", srv.URL+"/api/transfers/"+linked.ID, nil)
	removed.Body.Close()
	if removed.StatusCode != 404 {
		t.Fatalf("missing status: %d", removed.StatusCode)
	}
}
