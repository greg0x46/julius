package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/dates"
	"github.com/greg0x46/julius/internal/transactions"
	"github.com/shopspring/decimal"
)

func TestRecurrenceSuggestionsHTTP(t *testing.T) {
	srv, conn := newTestServer(t)
	read := func() []map[string]any {
		t.Helper()
		response := doJSON(t, http.MethodGet, srv.URL+"/api/recurrence-suggestions", nil)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("status %d", response.StatusCode)
		}
		if response.Header.Get("Cache-Control") != "no-store" {
			t.Fatal("missing no-store")
		}
		var suggestions []map[string]any
		decodeJSON(t, response, &suggestions)
		if suggestions == nil {
			t.Fatal("expected array")
		}
		return suggestions
	}
	if len(read()) != 0 {
		t.Fatal("nonempty initial discovery")
	}
	ctx := context.Background()
	account, err := transactions.WriteManualAccount(ctx, conn, "", transactions.ManualAccountInput{Name: "Synthetic", CurrencyCode: "BRL"}, true)
	if err != nil {
		t.Fatal(err)
	}
	today := dates.Day(time.Now().UTC())
	ids := []string{}
	for n := -5; n <= 0; n++ {
		id, err := transactions.CreateManual(ctx, conn, transactions.ManualInput{AccountID: account, Description: "Synthetic subscription", Amount: decimal.RequireFromString("-100.1234"), OccurredAt: dates.AddMonths(today, n)})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	got := read()
	if len(got) != 1 {
		t.Fatalf("suggestions: %+v", got)
	}
	draft := got[0]["draft"].(map[string]any)
	if draft["amount"] != "100.1234" || draft["account_id"] != account || draft["cadence"] != "monthly" || draft["kind"] != "expense" || got[0]["currency_code"] != "BRL" || got[0]["amount_kind"] != "fixed" {
		t.Fatalf("draft: %+v", got)
	}
	evidence, _ := json.Marshal(got[0]["transaction_ids"])
	expected, _ := json.Marshal(ids)
	if string(evidence) != string(expected) {
		t.Fatalf("evidence: %s", evidence)
	}
	if !reflect.DeepEqual(got, read()) {
		t.Fatal("nondeterministic API")
	}
	// Drafts with no category explicitly need owner input before creation.
	if draft["category_id"] != "" {
		t.Fatal("invented category")
	}
	var count int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM scenarios`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("discovery persisted a scenario: %d, %v", count, err)
	}
	conn.Close()
	response := doJSON(t, http.MethodGet, srv.URL+"/api/recurrence-suggestions", nil)
	defer response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("database failure status: %d", response.StatusCode)
	}
}
