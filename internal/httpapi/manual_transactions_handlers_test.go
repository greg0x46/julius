package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/greg0x46/julius/internal/automation"
	"github.com/greg0x46/julius/internal/categories"
	"github.com/greg0x46/julius/internal/money"
)

func TestCreateManualTransactionIdempotentRetry(t *testing.T) {
	srv, conn := newTestServer(t)
	seed := insertAccount(t, conn, "BANK", "Conta corrente", nil, nil)
	key := uuid.NewString()
	create := map[string]any{
		"account_id": seed.accountID, "description": "Feira", "amount": "-42.50", "occurred_at": "2026-03-01",
	}
	request := func(body map[string]any, key string) *http.Response {
		t.Helper()
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/transactions", bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", testOrigin)
		req.Header.Set("X-Julius-Request", "1")
		req.Header.Set("Idempotency-Key", key)
		if token, ok := fixtureTokens.Load(req.URL.Host); ok {
			req.AddCookie(&http.Cookie{Name: "julius_session", Value: token.(string)})
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	first := request(create, key)
	if first.StatusCode != 201 {
		body, _ := io.ReadAll(first.Body)
		t.Fatalf("first status %d: %s", first.StatusCode, body)
	}
	var original map[string]any
	decodeJSON(t, first, &original)
	retry := request(create, key)
	if retry.StatusCode != 201 {
		body, _ := io.ReadAll(retry.Body)
		t.Fatalf("retry status %d: %s", retry.StatusCode, body)
	}
	var repeated map[string]any
	decodeJSON(t, retry, &repeated)
	if repeated["id"] != original["id"] {
		t.Fatalf("retry returned new ID %v instead of %v", repeated["id"], original["id"])
	}
	var count int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM financial_transactions WHERE description = 'Feira'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("transactions = %d, want one", count)
	}

	create["amount"] = "-43.00"
	conflict := request(create, key)
	if conflict.StatusCode != 409 {
		body, _ := io.ReadAll(conflict.Body)
		t.Fatalf("different payload status %d: %s", conflict.StatusCode, body)
	}
	conflict.Body.Close()
	if err := conn.QueryRow(`SELECT COUNT(*) FROM financial_transactions WHERE description = 'Feira'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("transactions after conflict = %d, want one", count)
	}
}

func TestCreateManualIdempotencyKeyRollsBackOnFailure(t *testing.T) {
	srv, conn := newTestServer(t)
	seed := insertAccount(t, conn, "BANK", "Conta corrente", nil, nil)
	key := uuid.NewString()
	body := map[string]any{"account_id": seed.accountID, "description": "Repetir", "amount": "1", "occurred_at": "2026-03-01", "category_id": uuid.NewString()}
	post := func() *http.Response {
		data, _ := json.Marshal(body)
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/transactions", bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", testOrigin)
		req.Header.Set("X-Julius-Request", "1")
		req.Header.Set("Idempotency-Key", key)
		if token, ok := fixtureTokens.Load(req.URL.Host); ok {
			req.AddCookie(&http.Cookie{Name: "julius_session", Value: token.(string)})
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	failed := post()
	failed.Body.Close()
	if failed.StatusCode != 422 {
		t.Fatalf("invalid category status = %d", failed.StatusCode)
	}
	var count int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM manual_transaction_idempotency WHERE idempotency_key = ?`, key).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("failed attempt reserved key")
	}
	delete(body, "category_id")
	retry := post()
	retry.Body.Close()
	if retry.StatusCode != 201 {
		t.Fatalf("corrected retry status = %d", retry.StatusCode)
	}
}

// A rule whose target category was later deactivated is a server-side problem:
// the client never sent a category, so it must not be told its input is invalid.
func TestCreateManualTransactionWithStaleRuleCategoryIs503NotInvalidCategory(t *testing.T) {
	srv, conn := newTestServer(t)
	seed := insertAccount(t, conn, "BANK", "Conta corrente", nil, nil)
	ctx := context.Background()

	category, err := categories.Create(ctx, conn, "Lazer", money.Expense, "", "")
	if err != nil {
		t.Fatalf("categories.Create: %v", err)
	}
	_, err = automation.Create(ctx, conn, automation.Write{
		Name: "streaming", IsActive: true, LogicOperator: automation.LogicOr,
		Conditions: []automation.Condition{{Field: automation.FieldDescription, Operator: automation.OperatorContains, Value: "streaming"}},
		Actions:    []automation.ActionWrite{{Type: automation.ActionSetCategory, CategoryID: &category.ID}},
	})
	if err != nil {
		t.Fatalf("automation.Create: %v", err)
	}
	inactive := false
	if _, err := categories.Update(ctx, conn, category.ID, nil, &inactive, nil, nil); err != nil {
		t.Fatalf("categories.Update: %v", err)
	}

	body := map[string]any{
		"account_id": seed.accountID, "description": "Streaming mensal",
		"amount": "-39.90", "occurred_at": "2026-03-01", "category_id": nil,
	}
	resp := doJSON(t, http.MethodPost, srv.URL+"/api/transactions", body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}

	// An explicit, invalid category is still the client's error.
	body["category_id"] = category.ID
	resp = doJSON(t, http.MethodPost, srv.URL+"/api/transactions", body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("explicit invalid category status = %d, want 422", resp.StatusCode)
	}
}

func TestManualTransactionLifecycleOverHTTP(t *testing.T) {
	srv, conn := newTestServer(t)
	seed := insertAccount(t, conn, "BANK", "Conta corrente", nil, nil)

	create := map[string]any{
		"account_id":  seed.accountID,
		"description": "Almoço em dinheiro",
		"amount":      "-45.50",
		"occurred_at": "2026-03-01",
		"category_id": nil,
	}
	resp := doJSON(t, http.MethodPost, srv.URL+"/api/transactions", create)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", resp.StatusCode)
	}
	var item map[string]any
	decodeJSON(t, resp, &item)
	if item["origin"] != "manual" {
		t.Errorf("origin = %v, want manual", item["origin"])
	}
	if _, ok := item["source_provider"]; !ok || item["source_provider"] != nil {
		t.Errorf("source_provider = %v, want explicit null for manual", item["source_provider"])
	}
	if item["classification"] != "outflow" {
		t.Errorf("classification = %v, want outflow", item["classification"])
	}
	id, _ := item["id"].(string)
	if id == "" {
		t.Fatalf("missing id in response: %+v", item)
	}

	// An unknown account is rejected with 404, not a generic failure.
	badCreate := map[string]any{
		"account_id": "does-not-exist", "description": "x", "amount": "1", "occurred_at": "2026-03-01",
	}
	resp = doJSON(t, http.MethodPost, srv.URL+"/api/transactions", badCreate)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown account create status = %d, want 404", resp.StatusCode)
	}

	// A zero amount is invalid input, not a domain error.
	zeroCreate := map[string]any{
		"account_id": seed.accountID, "description": "x", "amount": "0", "occurred_at": "2026-03-01",
	}
	resp = doJSON(t, http.MethodPost, srv.URL+"/api/transactions", zeroCreate)
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Errorf("zero amount create status = %d, want 422", resp.StatusCode)
	}

	// A failure after the base insert must roll the whole creation back.
	invalidCategoryCreate := map[string]any{
		"account_id": seed.accountID, "description": "não deve persistir", "amount": "1",
		"occurred_at": "2026-03-01", "category_id": uuid.NewString(),
	}
	resp = doJSON(t, http.MethodPost, srv.URL+"/api/transactions", invalidCategoryCreate)
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Errorf("invalid category create status = %d, want 422", resp.StatusCode)
	}
	var leaked int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM financial_transactions WHERE description = 'não deve persistir'`).Scan(&leaked); err != nil {
		t.Fatalf("count rolled-back transaction: %v", err)
	}
	if leaked != 0 {
		t.Errorf("failed create left %d transaction(s), want 0", leaked)
	}

	// Category assignment already goes through the existing, origin-agnostic
	// endpoint and just works.
	var categories []map[string]any
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/categories", nil)
	decodeJSON(t, resp, &categories)
	categoryID := categories[0]["id"].(string)
	updatedCategoryID := categories[1]["id"].(string)
	resp = doJSON(t, http.MethodPut, srv.URL+"/api/transactions/"+id+"/category", map[string]string{"category_id": categoryID})
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("category status = %d, want 200", resp.StatusCode)
	}

	// Editing changes the row.
	update := map[string]any{
		"account_id": seed.accountID, "description": "Almoço corrigido", "amount": "-50.00",
		"occurred_at": "2026-03-02", "category_id": updatedCategoryID,
	}
	resp = doJSON(t, http.MethodPut, srv.URL+"/api/transactions/"+id, update)
	if resp.StatusCode != 200 {
		t.Fatalf("update status = %d, want 200", resp.StatusCode)
	}
	var updated map[string]any
	decodeJSON(t, resp, &updated)
	if updated["description"] != "Almoço corrigido" {
		t.Errorf("description = %v, want Almoço corrigido", updated["description"])
	}
	updatedCategory, _ := updated["internal_category"].(map[string]any)
	if updatedCategory["id"] != updatedCategoryID {
		t.Errorf("category = %v, want %s", updatedCategory["id"], updatedCategoryID)
	}

	// A synced transaction cannot be edited or deleted through these routes.
	syncedID := insertTransaction(t, conn)
	resp = doJSON(t, http.MethodPut, srv.URL+"/api/transactions/"+syncedID, update)
	resp.Body.Close()
	if resp.StatusCode != 409 {
		t.Errorf("edit synced status = %d, want 409", resp.StatusCode)
	}
	resp = doJSON(t, http.MethodDelete, srv.URL+"/api/transactions/"+syncedID, nil)
	resp.Body.Close()
	if resp.StatusCode != 409 {
		t.Errorf("delete synced status = %d, want 409", resp.StatusCode)
	}

	// Deleting the manual one actually removes it.
	resp = doJSON(t, http.MethodDelete, srv.URL+"/api/transactions/"+id, nil)
	if resp.StatusCode != http.StatusNoContent {
		var problem map[string]any
		decodeJSON(t, resp, &problem)
		t.Fatalf("delete status = %d, want 204: %+v", resp.StatusCode, problem)
	}
	resp.Body.Close()
	resp = doJSON(t, http.MethodPut, srv.URL+"/api/transactions/"+id+"/inclusion", map[string]string{"state": "ignored"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("post-delete status = %d, want 404", resp.StatusCode)
	}
}
