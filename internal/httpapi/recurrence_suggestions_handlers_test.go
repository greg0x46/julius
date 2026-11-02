package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/dates"
	"github.com/greg0x46/julius/internal/transactions"
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

func TestRecurrenceSuggestionReviewBulkAndDismiss(t *testing.T) {
	srv, conn := newTestServer(t)
	ctx := context.Background()
	account, err := transactions.WriteManualAccount(ctx, conn, "", transactions.ManualAccountInput{Name: "Synthetic", CurrencyCode: "BRL"}, true)
	if err != nil {
		t.Fatal(err)
	}
	today := dates.Day(time.Now().UTC())
	anchor := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
	for _, name := range []string{"Internet", "Music", "Insurance"} {
		for n := -5; n <= 0; n++ {
			if _, err := transactions.CreateManual(ctx, conn, transactions.ManualInput{AccountID: account, Description: name, Amount: decimal.NewFromInt(-100), OccurredAt: dates.AddMonths(anchor, n)}); err != nil {
				t.Fatal(err)
			}
		}
	}
	response := doJSON(t, http.MethodGet, srv.URL+"/api/recurrence-suggestions", nil)
	var candidates []map[string]any
	decodeJSON(t, response, &candidates)
	if len(candidates) != 3 {
		t.Fatal(candidates)
	}
	response = doJSON(t, http.MethodPost, srv.URL+"/api/categories", map[string]any{"name": "Synthetic", "kind": "expense", "icon": "home", "color": "#495057"})
	if response.StatusCode != 201 {
		t.Fatalf("category: %d", response.StatusCode)
	}
	var category map[string]any
	decodeJSON(t, response, &category)
	items := []map[string]any{}
	for _, c := range candidates {
		draft := c["draft"].(map[string]any)
		draft["category_id"] = category["id"]
		item := map[string]any{"key": c["key"], "draft": draft}
		response = doJSON(t, http.MethodPost, srv.URL+"/api/recurrence-suggestions/review", item)
		if response.StatusCode != 200 {
			var problem map[string]any
			decodeJSON(t, response, &problem)
			t.Fatalf("review %d: %+v draft %+v", response.StatusCode, problem, draft)
		}
		var review map[string]any
		decodeJSON(t, response, &review)
		if len(review["history"].([]any)) != 18 {
			t.Fatal(review)
		}
		item["token"], item["conditions"], item["acknowledge"] = review["token"], review["conditions"], true
		items = append(items, item)
	}
	// Each item commits independently; a missing category does not block other items.
	items[1]["draft"].(map[string]any)["category_id"] = ""
	response = doJSON(t, http.MethodPost, srv.URL+"/api/recurrence-suggestions/accept", map[string]any{"items": items[:2]})
	var results []map[string]any
	decodeJSON(t, response, &results)
	if len(results) != 2 || results[0]["status"] != float64(201) || results[1]["status"] != float64(422) {
		t.Fatal(results)
	}
	response = doJSON(t, http.MethodPost, srv.URL+"/api/recurrence-suggestions/accept", map[string]any{"items": items[:1]})
	var retry []map[string]any
	decodeJSON(t, response, &retry)
	if retry[0]["scenario_id"] != results[0]["scenario_id"] {
		t.Fatal(retry)
	}
	response = doJSON(t, http.MethodPost, srv.URL+"/api/recurrence-suggestions/dismiss", map[string]any{"key": items[2]["key"]})
	response.Body.Close()
	if response.StatusCode != 204 {
		t.Fatal(response.StatusCode)
	}
	response = doJSON(t, http.MethodGet, srv.URL+"/api/recurrence-suggestions", nil)
	decodeJSON(t, response, &candidates)
	if len(candidates) != 1 {
		t.Fatal(candidates)
	}
}
