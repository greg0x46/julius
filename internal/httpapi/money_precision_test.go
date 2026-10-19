package httpapi_test

import (
	"net/http"
	"testing"
)

func TestAPIMoneyPrecisionRoundTrip(t *testing.T) {
	srv, conn := newTestServer(t)
	seed := insertAccount(t, conn, "BANK", "Synthetic precision account", nil, nil)
	for _, amount := range []string{"0.000000000000000001", "-1.005", "9007199254740993.123456789012345678", "-9007199254740993.123456789012345678"} {
		t.Run(amount, func(t *testing.T) {
			body := map[string]any{"account_id": seed.accountID, "description": "Precision", "amount": amount, "occurred_at": "2026-02-05"}
			resp := doJSON(t, http.MethodPost, srv.URL+"/api/transactions", body)
			if resp.StatusCode != http.StatusCreated {
				t.Fatalf("create status = %d", resp.StatusCode)
			}
			var created map[string]any
			decodeJSON(t, resp, &created)
			if created["amount"] != amount {
				t.Fatalf("create amount = %v, want %s", created["amount"], amount)
			}
			url := srv.URL + "/api/transactions/" + created["id"].(string)
			resp = doJSON(t, http.MethodPost, srv.URL+"/api/transactions/query", map[string]any{"timezone": "UTC", "group_by": "none", "page": 1, "page_size": 100})
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("read status = %d", resp.StatusCode)
			}
			var query struct {
				Items []map[string]any `json:"items"`
			}
			decodeJSON(t, resp, &query)
			var read map[string]any
			for _, item := range query.Items {
				if item["id"] == created["id"] {
					read = item
					break
				}
			}
			if read["amount"] != amount {
				t.Fatalf("read amount = %v, want %s", read["amount"], amount)
			}
			body["amount"] = read["amount"]
			resp = doJSON(t, http.MethodPut, url, body)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("update status = %d", resp.StatusCode)
			}
			var updated map[string]any
			decodeJSON(t, resp, &updated)
			if updated["amount"] != amount {
				t.Fatalf("update amount = %v, want %s", updated["amount"], amount)
			}
		})
	}
}

func TestRecurringAPIMoneyPrecision(t *testing.T) {
	f := newReconciliationFixture(t)
	txID := f.addTransaction("-1.005", "2026-02-05")
	for _, amount := range []string{"0.000000000000000001", "1.005", "9007199254740993.123456789012345678"} {
		t.Run(amount, func(t *testing.T) {
			body := map[string]any{
				"name": "Precision", "kind": "expense", "amount": amount, "category_id": f.categoryID,
				"cadence": "monthly", "day_of_month": 5, "start_date": "2026-01-01", "is_active": true,
			}
			resp := doJSON(t, http.MethodPut, f.url+"/api/recurring-scenarios/"+f.id, body)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("update status = %d", resp.StatusCode)
			}
			var updated map[string]any
			decodeJSON(t, resp, &updated)
			if updated["amount"] != amount {
				t.Fatalf("update amount = %v, want %s", updated["amount"], amount)
			}
			resp = doJSON(t, http.MethodGet, f.url+"/api/recurring-scenarios", nil)
			var commitments []map[string]any
			decodeJSON(t, resp, &commitments)
			if len(commitments) != 1 || commitments[0]["amount"] != amount {
				t.Fatalf("list = %v, want amount %s", commitments, amount)
			}
			resp = doJSON(t, http.MethodGet, f.occurrencesURL("2026-02-01", "2026-02-28"), nil)
			var occurrences []map[string]any
			decodeJSON(t, resp, &occurrences)
			if len(occurrences) != 1 || occurrences[0]["expected_amount"] != amount {
				t.Fatalf("occurrences = %v, want amount %s", occurrences, amount)
			}
			resp = doJSON(t, http.MethodGet, f.url+"/api/transactions/"+txID+"/reconciliation", nil)
			var reconciliation struct {
				Options []struct {
					ExpectedAmount string `json:"expected_amount"`
				} `json:"options"`
			}
			decodeJSON(t, resp, &reconciliation)
			if len(reconciliation.Options) == 0 {
				t.Fatal("no reconciliation options")
			}
			for _, option := range reconciliation.Options {
				if option.ExpectedAmount != amount {
					t.Errorf("option amount = %s, want %s", option.ExpectedAmount, amount)
				}
			}
		})
	}
}

func TestTransactionQueryRejectsHugePaging(t *testing.T) {
	srv, _ := newTestServer(t)
	for _, body := range []map[string]any{
		{"timezone": "UTC", "group_by": "none", "page": int64(9223372036854775807), "page_size": 50},
		{"timezone": "UTC", "group_by": "none", "page": 2, "page_size": int64(9223372036854775807)},
	} {
		resp := doJSON(t, http.MethodPost, srv.URL+"/api/transactions/query", body)
		resp.Body.Close()
		if resp.StatusCode != 422 {
			t.Fatalf("%v: status = %d, want 422", body, resp.StatusCode)
		}
	}
}
