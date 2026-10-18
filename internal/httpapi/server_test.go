package httpapi_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/uuid"

	"github.com/greg0x46/julius/internal/auth"
	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/httpapi"
	"github.com/greg0x46/julius/internal/marketdata"
	"github.com/greg0x46/julius/internal/settings"
	"net/url"
	"sync"
)

const testPassword = "correct horse battery staple"

// API-domain fixtures authenticate through real persisted sessions. Auth tests use
// their own clients without registering a token in this fixture registry.
var fixtureTokens sync.Map

const testOrigin = "http://localhost:4200"

func newTestServerWithSession(t *testing.T) (*httptest.Server, *sql.DB, *settings.Secrets) {
	t.Helper()
	return newTestServerWithMarket(t, nil)
}
func newTestServerWithMarket(t *testing.T, market *marketdata.Service) (*httptest.Server, *sql.DB, *settings.Secrets) {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	frontend := fstest.MapFS{"index.html": {Data: []byte("<html>spa</html>")}, "assets/app.js": {Data: []byte("console.log(1)")}}
	master := bytes.Repeat([]byte{42}, 32)
	store := auth.NewStore(conn)
	if err := store.Initialize(context.Background(), "owner@example.com", testPassword, master, nil); err != nil {
		t.Fatal(err)
	}
	keys := settings.NewSecrets(master)
	srv := httptest.NewServer(httpapi.NewServer(conn, fs.FS(frontend), keys, auth.Config{PublicURL: testOrigin}, market))
	t.Cleanup(srv.Close)
	return srv, conn, keys
}
func newTestServer(t *testing.T) (*httptest.Server, *sql.DB) {
	t.Helper()
	return loggedIn(t, newTestServerWithSession)
}
func loggedIn(t *testing.T, open func(*testing.T) (*httptest.Server, *sql.DB, *settings.Secrets)) (*httptest.Server, *sql.DB) {
	t.Helper()
	srv, conn, _ := open(t)
	token, err := auth.NewStore(conn).Login(context.Background(), "owner@example.com", testPassword)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(srv.URL)
	fixtureTokens.Store(u.Host, token)
	t.Cleanup(func() { fixtureTokens.Delete(u.Host) })
	return srv, conn
}
func newLockedTestServer(t *testing.T) (*httptest.Server, *sql.DB) {
	srv, conn, _ := newTestServerWithSession(t)
	return srv, conn
}
func testGet(t *testing.T, endpoint string) (*http.Response, error) {
	return doJSON(t, http.MethodGet, endpoint, nil), nil
}

// insertTransaction inserts the minimal sync-schema chain plus one
// financial_transactions row directly, so HTTP tests can exercise
// inclusion/category endpoints without a full sync pipeline (phase 4).
func insertTransaction(t *testing.T, conn *sql.DB) string {
	t.Helper()
	now := db.FormatTime(time.Now())
	sourceID, syncRunID, rawImportID, accountID, txID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := conn.Exec(query, args...); err != nil {
			t.Fatalf("exec %q: %v", query, err)
		}
	}
	exec(`INSERT INTO data_sources (id, provider, external_item_id, created_at, updated_at)
		VALUES (?, 'pluggy', ?, ?, ?)`, sourceID, sourceID, now, now)
	exec(`INSERT INTO sync_runs (id, source_id, status, started_at, finished_at)
		VALUES (?, ?, 'completed', ?, ?)`, syncRunID, sourceID, now, now)
	exec(`INSERT INTO raw_imports (
			id, sync_run_id, source_id, scope, page_sequence, request_attempt,
			request_method, request_path, http_status, response_headers, payload,
			payload_sha256, received_at
		) VALUES (?, ?, ?, 'transactions', 1, 1, 'GET', '/x', 200, '{}', x'00', 'sha', ?)`,
		rawImportID, syncRunID, sourceID, now)
	exec(`INSERT INTO financial_accounts (
			id, source_id, external_id, currency_code, current_raw_import_id, normalized_hash,
			created_at, updated_at
		) VALUES (?, ?, ?, 'BRL', ?, 'hash', ?, ?)`, accountID, sourceID, accountID, rawImportID, now, now)
	exec(`INSERT INTO financial_transactions (
			id, source_id, account_id, external_id, description, amount, amount_in_account_currency,
			currency_code, occurred_at, provider_status, movement_type, current_raw_import_id,
			normalized_hash, created_at, updated_at
		) VALUES (?, ?, ?, ?, 'Mercado', '-42.00', '-42.00', 'BRL', ?, 'POSTED', 'DEBIT', ?, 'hash', ?, ?)`,
		txID, sourceID, accountID, txID, now, rawImportID, now, now)
	return txID
}

func doJSON(t *testing.T, method, url string, body any) *http.Response {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", testOrigin)
	req.Header.Set("X-Julius-Request", "1")
	if token, ok := fixtureTokens.Load(req.URL.Host); ok {
		req.AddCookie(&http.Cookie{Name: "julius_session", Value: token.(string)})
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func decodeJSON(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func TestSyncRunLifecycleOverHTTP(t *testing.T) {
	srv, _ := newTestServer(t)

	// No connection registered yet: creating a run should fail clearly rather
	// than silently creating a data_source with an empty external_item_id.
	resp := doJSON(t, http.MethodPost, srv.URL+"/api/sync-runs", nil)
	resp.Body.Close()
	if resp.StatusCode != 409 {
		t.Fatalf("unconfigured create status = %d, want 409", resp.StatusCode)
	}

	registerConnection(t, srv, "item-1", nil)

	resp = doJSON(t, http.MethodPost, srv.URL+"/api/sync-runs", nil)
	if resp.StatusCode != 202 {
		t.Fatalf("create status = %d, want 202", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc == "" {
		t.Error("expected a Location header when exactly one run was started")
	}
	var createResp struct {
		Runs      []map[string]any `json:"runs"`
		Requested int              `json:"requested"`
	}
	decodeJSON(t, resp, &createResp)
	if len(createResp.Runs) != 1 {
		t.Fatalf("created %d runs, want 1", len(createResp.Runs))
	}
	if createResp.Requested != 1 {
		t.Errorf("requested = %d, want 1", createResp.Requested)
	}
	created := createResp.Runs[0]
	runID := created["id"].(string)
	if created["status"] != "in_progress" {
		t.Errorf("status = %v, want in_progress", created["status"])
	}
	if created["source_name"] != "item-1" {
		t.Errorf("source_name = %v, want the item id a never-synced connection falls back to", created["source_name"])
	}

	// A second run while the first is still in_progress must conflict.
	resp = doJSON(t, http.MethodPost, srv.URL+"/api/sync-runs", nil)
	var conflict map[string]any
	decodeJSON(t, resp, &conflict)
	if resp.StatusCode != 409 || conflict["active_sync_run_id"] != runID {
		t.Errorf("second create: status=%d body=%+v", resp.StatusCode, conflict)
	}

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/sync-runs", nil)
	var list []map[string]any
	decodeJSON(t, resp, &list)
	if len(list) != 1 {
		t.Fatalf("len(list) = %d, want 1", len(list))
	}

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/sync-runs/"+runID, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("get status = %d, want 200", resp.StatusCode)
	}
	var detail map[string]any
	decodeJSON(t, resp, &detail)
	if detail["id"] != runID {
		t.Errorf("detail id = %v, want %s", detail["id"], runID)
	}
	if _, ok := detail["failures"].([]any); !ok {
		t.Errorf("detail should include a failures array, got %+v", detail)
	}

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/sync-runs/does-not-exist", nil)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("unknown run status = %d, want 404", resp.StatusCode)
	}
}

func TestAutomationRuleLifecycleOverHTTP(t *testing.T) {
	srv, conn := newTestServer(t)
	txID := insertTransaction(t, conn)
	if _, err := conn.Exec(`UPDATE financial_transactions SET description = 'Transferencia enviada' WHERE id = ?`, txID); err != nil {
		t.Fatalf("update description: %v", err)
	}

	body := map[string]any{
		"name": "Ignorar transferências", "is_active": true, "logic_operator": "and",
		"conditions":          []map[string]string{{"field": "description", "operator": "contains", "value": "transferencia"}},
		"actions":             []map[string]any{{"type": "ignore"}},
		"apply_retroactively": true,
	}
	resp := doJSON(t, http.MethodPost, srv.URL+"/api/automation-rules", body)
	if resp.StatusCode != 201 {
		t.Fatalf("create status = %d, want 201", resp.StatusCode)
	}
	var created map[string]any
	decodeJSON(t, resp, &created)
	rule := created["rule"].(map[string]any)
	ruleID := rule["id"].(string)
	retro := created["retroactive_apply"].(map[string]any)
	if retro["matched"].(float64) != 1 || retro["ignored"].(float64) != 1 {
		t.Errorf("retroactive_apply = %+v, want matched=1 ignored=1", retro)
	}

	var state string
	conn.QueryRow(`SELECT state FROM transaction_inclusion_decisions WHERE transaction_id = ?`, txID).Scan(&state)
	if state != "ignored" {
		t.Errorf("state = %s, want ignored", state)
	}

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/automation-rules", nil)
	var list []map[string]any
	decodeJSON(t, resp, &list)
	if len(list) != 1 {
		t.Fatalf("len(list) = %d, want 1", len(list))
	}

	resp = doJSON(t, http.MethodPatch, srv.URL+"/api/automation-rules/"+ruleID, map[string]bool{"is_active": false})
	if resp.StatusCode != 200 {
		t.Fatalf("patch status = %d, want 200", resp.StatusCode)
	}
	var patched map[string]any
	decodeJSON(t, resp, &patched)
	if patched["is_active"] != false {
		t.Errorf("is_active = %v, want false", patched["is_active"])
	}

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/automation-rules/condition-options", nil)
	var options map[string]any
	decodeJSON(t, resp, &options)
	if _, ok := options["accounts"]; !ok {
		t.Errorf("expected an accounts key, got %+v", options)
	}

	resp = doJSON(t, http.MethodDelete, srv.URL+"/api/automation-rules/"+ruleID, nil)
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Errorf("delete status = %d, want 204", resp.StatusCode)
	}

	resp = doJSON(t, http.MethodPatch, srv.URL+"/api/automation-rules/"+ruleID, map[string]bool{"is_active": true})
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("patch after delete status = %d, want 404", resp.StatusCode)
	}
}

func TestCreateAutomationRuleRejectsEmptyConditions(t *testing.T) {
	srv, _ := newTestServer(t)
	body := map[string]any{
		"name": "Regra vazia", "is_active": true, "logic_operator": "and",
		"conditions": []map[string]string{}, "actions": []map[string]any{{"type": "ignore"}},
	}
	resp := doJSON(t, http.MethodPost, srv.URL+"/api/automation-rules", body)
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Errorf("status = %d, want 422", resp.StatusCode)
	}
}

func createRecurringCommitmentForAutomation(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	resp := doJSON(t, http.MethodPost, srv.URL+"/api/categories", map[string]any{
		"name": "Aluguel", "kind": "expense", "icon": "home", "color": "#495057",
	})
	var category map[string]any
	decodeJSON(t, resp, &category)
	categoryID := category["id"].(string)

	resp = doJSON(t, http.MethodPost, srv.URL+"/api/recurring-scenarios", map[string]any{
		"name": "Aluguel", "kind": "expense", "amount": "1500.00",
		"category_id": categoryID, "cadence": "monthly", "day_of_month": 5, "start_date": "2026-01-01",
		"is_active": true,
	})
	var commitment map[string]any
	decodeJSON(t, resp, &commitment)
	return commitment["id"].(string)
}

func TestCreateAutomationRuleWithReconcileActionLinksCommitment(t *testing.T) {
	srv, _ := newTestServer(t)
	commitmentID := createRecurringCommitmentForAutomation(t, srv)

	body := map[string]any{
		"name": "Concilia aluguel", "is_active": true, "logic_operator": "and",
		"conditions": []map[string]string{{"field": "amount", "operator": "within_percent", "value": "10"}},
		"actions":    []map[string]any{{"type": "reconcile", "scenario_id": commitmentID}},
	}
	resp := doJSON(t, http.MethodPost, srv.URL+"/api/automation-rules", body)
	if resp.StatusCode != 201 {
		t.Fatalf("create status = %d, want 201", resp.StatusCode)
	}
	var created map[string]any
	decodeJSON(t, resp, &created)
	rule := created["rule"].(map[string]any)
	actions := rule["actions"].([]any)
	if len(actions) != 1 {
		t.Fatalf("actions = %+v, want 1", actions)
	}
	action := actions[0].(map[string]any)
	if action["type"] != "reconcile" || action["scenario_id"] != commitmentID {
		t.Errorf("action = %+v", action)
	}
}

func TestCreateAutomationRuleReconcileActionRejectsUnknownCommitment(t *testing.T) {
	srv, _ := newTestServer(t)
	body := map[string]any{
		"name": "Concilia inexistente", "is_active": true, "logic_operator": "and",
		"conditions": []map[string]string{{"field": "amount", "operator": "within_percent", "value": "10"}},
		"actions":    []map[string]any{{"type": "reconcile", "scenario_id": "does-not-exist"}},
	}
	resp := doJSON(t, http.MethodPost, srv.URL+"/api/automation-rules", body)
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Errorf("status = %d, want 422", resp.StatusCode)
	}
}

func TestDeleteRecurringCommitmentLinkedToRuleReturns409(t *testing.T) {
	srv, _ := newTestServer(t)
	commitmentID := createRecurringCommitmentForAutomation(t, srv)

	body := map[string]any{
		"name": "Concilia aluguel", "is_active": true, "logic_operator": "and",
		"conditions": []map[string]string{{"field": "amount", "operator": "within_percent", "value": "10"}},
		"actions":    []map[string]any{{"type": "reconcile", "scenario_id": commitmentID}},
	}
	resp := doJSON(t, http.MethodPost, srv.URL+"/api/automation-rules", body)
	if resp.StatusCode != 201 {
		t.Fatalf("create rule status = %d, want 201", resp.StatusCode)
	}
	resp.Body.Close()

	resp = doJSON(t, http.MethodDelete, srv.URL+"/api/recurring-scenarios/"+commitmentID, nil)
	resp.Body.Close()
	if resp.StatusCode != 409 {
		t.Errorf("delete status = %d, want 409", resp.StatusCode)
	}
}

func TestDebtLifecycleOverHTTP(t *testing.T) {
	srv, conn := newTestServer(t)

	resp := doJSON(t, http.MethodPost, srv.URL+"/api/payables", map[string]any{"kind": "debt", "name": "Cartão", "total_amount": "1000.00", "initial_remaining_amount": "600.00"})
	if resp.StatusCode != 201 {
		t.Fatalf("create status = %d, want 201", resp.StatusCode)
	}
	var created map[string]any
	decodeJSON(t, resp, &created)
	debtID := created["id"].(string)
	if created["starting_settled_amount"] != "400.00" || created["status"] != "open" || created["link_count"].(float64) != 0 {
		t.Errorf("created = %+v", created)
	}

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/payables", nil)
	var list []map[string]any
	decodeJSON(t, resp, &list)
	if len(list) != 1 {
		t.Fatalf("len(list) = %d, want 1", len(list))
	}

	resp = doJSON(t, http.MethodPut, srv.URL+"/api/payables/"+debtID, map[string]any{"name": "Cartão renomeado", "total_amount": "2000.00"})
	if resp.StatusCode != 200 {
		t.Fatalf("update status = %d, want 200", resp.StatusCode)
	}
	var updated map[string]any
	decodeJSON(t, resp, &updated)
	if updated["name"] != "Cartão renomeado" || updated["total_amount"] != "2000.00" {
		t.Errorf("updated = %+v", updated)
	}

	// Link an eligible outflow transaction.
	txID := insertTransaction(t, conn)
	if _, err := conn.Exec(`UPDATE financial_transactions SET amount = '-150.00', amount_in_account_currency = '-150.00', currency_code = 'BRL' WHERE id = ?`, txID); err != nil {
		t.Fatalf("update transaction: %v", err)
	}
	if _, err := conn.Exec(`UPDATE financial_accounts SET currency_code = 'BRL' WHERE id = (SELECT account_id FROM financial_transactions WHERE id = ?)`, txID); err != nil {
		t.Fatalf("update account: %v", err)
	}

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/payables/eligible-transactions?kind=debt", nil)
	var eligible []map[string]any
	decodeJSON(t, resp, &eligible)
	if len(eligible) != 1 || eligible[0]["id"] != txID {
		t.Fatalf("eligible = %+v, want just %s", eligible, txID)
	}

	resp = doJSON(t, http.MethodPost, srv.URL+"/api/payables/"+debtID+"/links", map[string]string{"transaction_id": txID})
	if resp.StatusCode != 201 {
		t.Fatalf("create link status = %d, want 201", resp.StatusCode)
	}
	var link map[string]any
	decodeJSON(t, resp, &link)
	linkID := link["id"].(string)
	if link["linked_amount"] != "150.00" {
		t.Errorf("linked_amount = %v, want 150.00", link["linked_amount"])
	}

	// Linking the same transaction again must conflict.
	resp = doJSON(t, http.MethodPost, srv.URL+"/api/payables/"+debtID+"/links", map[string]string{"transaction_id": txID})
	resp.Body.Close()
	if resp.StatusCode != 409 {
		t.Errorf("re-link status = %d, want 409", resp.StatusCode)
	}

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/payables/"+debtID, nil)
	var detail map[string]any
	decodeJSON(t, resp, &detail)
	if detail["settled_amount"] != "550.00" { // 400 starting + 150 linked
		t.Errorf("settled_amount = %v, want 550.00", detail["settled_amount"])
	}
	if detail["link_count"].(float64) != 1 {
		t.Errorf("link_count = %v, want 1", detail["link_count"])
	}
	links := detail["links"].([]any)
	if len(links) != 1 {
		t.Fatalf("links = %+v", links)
	}

	// Ignoring the transaction must unlink it (via payables.UnlinkIfPresent).
	resp = doJSON(t, http.MethodPut, srv.URL+"/api/transactions/"+txID+"/inclusion", map[string]string{"state": "ignored"})
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("ignore status = %d, want 200", resp.StatusCode)
	}
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/payables/"+debtID, nil)
	decodeJSON(t, resp, &detail)
	if detail["link_count"].(float64) != 0 {
		t.Errorf("link_count after ignoring = %v, want 0", detail["link_count"])
	}

	resp = doJSON(t, http.MethodDelete, srv.URL+"/api/payables/"+debtID+"/links/"+linkID, nil)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("delete already-removed link status = %d, want 404", resp.StatusCode)
	}

	resp = doJSON(t, http.MethodDelete, srv.URL+"/api/payables/"+debtID, nil)
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Errorf("delete debt status = %d, want 204", resp.StatusCode)
	}
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/payables/"+debtID, nil)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("get deleted debt status = %d, want 404", resp.StatusCode)
	}
}

func TestDebtTotalOwedOverHTTP(t *testing.T) {
	srv, conn := newTestServer(t)

	resp := doJSON(t, http.MethodPost, srv.URL+"/api/payables", map[string]any{"kind": "debt", "name": "Financiamento", "total_amount": "1000.00", "initial_remaining_amount": "600.00"})
	if resp.StatusCode != 201 {
		t.Fatalf("create debt status = %d, want 201", resp.StatusCode)
	}
	resp.Body.Close()

	// "Parcelas futuras (cartão)" is calculated from considered transactions
	// in the current bill cycle. The provider balance is deliberately
	// irrelevant to this indicator.
	creditTxID := insertTransaction(t, conn)
	var creditAccountID string
	if err := conn.QueryRow(`SELECT account_id FROM financial_transactions WHERE id = ?`, creditTxID).Scan(&creditAccountID); err != nil {
		t.Fatalf("account id: %v", err)
	}
	loc := time.Local
	now := time.Now().In(loc)
	currentClosing := time.Date(now.Year(), now.Month(), 2, 0, 0, 0, 0, loc)
	if now.Before(currentClosing) {
		currentClosing = time.Date(now.Year(), now.Month()-1, 2, 0, 0, 0, 0, loc)
	}
	nextClosing := time.Date(currentClosing.Year(), currentClosing.Month()+1, 2, 0, 0, 0, 0, loc)
	setCardDebtTransaction(t, conn, creditTxID, now, "-100.00", "DEBIT", nil)
	// One historical closing establishes the lower bound; the transaction is
	// associated with the next (current-cycle) bill.
	insertBillForTransaction(t, conn, creditTxID, "bill-history", currentClosing, now)
	insertBillForTransaction(t, conn, creditTxID, "bill-current", nextClosing, now)
	if _, err := conn.Exec(`UPDATE financial_accounts SET account_type = 'CREDIT', balance = '5535.84' WHERE id = ?`, creditAccountID); err != nil {
		t.Fatalf("set credit account balance: %v", err)
	}

	// A non-CREDIT account's transactions and balance must not count toward
	// future installments.
	bankTxID := insertTransaction(t, conn)
	var bankAccountID string
	if err := conn.QueryRow(`SELECT account_id FROM financial_transactions WHERE id = ?`, bankTxID).Scan(&bankAccountID); err != nil {
		t.Fatalf("account id: %v", err)
	}
	if _, err := conn.Exec(`UPDATE financial_accounts SET balance = '999999.00' WHERE id = ?`, bankAccountID); err != nil {
		t.Fatalf("set bank account balance: %v", err)
	}

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/payables/total-owed", nil)
	var total map[string]any
	decodeJSON(t, resp, &total)
	if total["remaining_debts_total"] != "600.00" {
		t.Errorf("remaining_debts_total = %v, want 600.00", total["remaining_debts_total"])
	}
	if total["future_installments_total"] != "100.00" {
		t.Errorf("future_installments_total = %v, want 100.00", total["future_installments_total"])
	}
	if total["total_owed"] != "700.00" {
		t.Errorf("total_owed = %v, want 700.00", total["total_owed"])
	}
	if total["currency_code"] != "BRL" {
		t.Errorf("currency_code = %v, want BRL", total["currency_code"])
	}
}

func TestDebtTotalOwedCalculatesConsideredCreditCardTransactionsByBillOrClosingDate(t *testing.T) {
	srv, conn := newTestServer(t)

	loc := time.Local
	now := time.Now().In(loc)
	currentClosing := time.Date(now.Year(), now.Month(), 2, 0, 0, 0, 0, loc)
	if now.Before(currentClosing) {
		currentClosing = time.Date(now.Year(), now.Month()-1, 2, 0, 0, 0, 0, loc)
	}
	previousClosing := time.Date(currentClosing.Year(), currentClosing.Month()-1, 2, 0, 0, 0, 0, loc)
	nextClosing := time.Date(currentClosing.Year(), currentClosing.Month()+1, 2, 0, 0, 0, 0, loc)

	// occurred_at is the provider's posted/launch date, not necessarily the
	// original purchase date. The indicator intentionally classifies this
	// stored provider date against the closing window when no bill is known.
	currentNoBillID := insertTransaction(t, conn)
	setCardDebtTransaction(t, conn, currentNoBillID, time.Date(currentClosing.Year(), currentClosing.Month(), 2, 0, 30, 0, 0, loc), "-100.00", "DEBIT", nil)
	var currentAccountID string
	if err := conn.QueryRow(`SELECT account_id FROM financial_transactions WHERE id = ?`, currentNoBillID).Scan(&currentAccountID); err != nil {
		t.Fatalf("first card account id: %v", err)
	}

	currentForecastOnlyID := insertTransaction(t, conn)
	moveTransactionToAccount(t, conn, currentForecastOnlyID, currentAccountID)
	setCardDebtTransaction(t, conn, currentForecastOnlyID, time.Date(nextClosing.Year(), nextClosing.Month(), 1, 12, 0, 0, 0, loc), "-25.00", "DEBIT", stringPointer(`{"billForecastDate":"1900-01"}`))

	nextClosingID := insertTransaction(t, conn)
	moveTransactionToAccount(t, conn, nextClosingID, currentAccountID)
	setCardDebtTransaction(t, conn, nextClosingID, nextClosing, "-40.00", "DEBIT", nil)

	outsideCycleID := insertTransaction(t, conn)
	moveTransactionToAccount(t, conn, outsideCycleID, currentAccountID)
	setCardDebtTransaction(t, conn, outsideCycleID, time.Date(currentClosing.Year(), currentClosing.Month(), 1, 12, 0, 0, 0, loc), "-30.00", "DEBIT", nil)

	previousBillID := insertTransaction(t, conn)
	moveTransactionToAccount(t, conn, previousBillID, currentAccountID)
	setCardDebtTransaction(t, conn, previousBillID, time.Date(currentClosing.Year(), currentClosing.Month(), 5, 12, 0, 0, 0, loc), "-70.00", "DEBIT", nil)
	insertBillForTransaction(t, conn, previousBillID, "bill-previous", previousClosing,
		time.Date(previousClosing.Year(), previousClosing.Month(), 15, 0, 0, 0, 0, loc))

	currentBillID := insertTransaction(t, conn)
	moveTransactionToAccount(t, conn, currentBillID, currentAccountID)
	// This provider posting date is outside the current cycle, but the known
	// bill is the current-cycle bill and therefore takes precedence. The due
	// date is deliberately different from the closing date and must not
	// classify the transaction.
	setCardDebtTransaction(t, conn, currentBillID, time.Date(currentClosing.Year(), currentClosing.Month(), 1, 12, 0, 0, 0, loc), "-10.00", "DEBIT", nil)
	insertBillForTransaction(t, conn, currentBillID, "bill-current", nextClosing,
		time.Date(currentClosing.Year(), currentClosing.Month(), 1, 0, 0, 0, 0, loc))

	cardBID := insertTransaction(t, conn)
	setCardDebtTransaction(t, conn, cardBID, time.Date(currentClosing.Year(), currentClosing.Month(), 10, 12, 0, 0, 0, loc), "-50.00", "DEBIT", nil)
	insertBillForTransaction(t, conn, cardBID, "bill-card-b", currentClosing,
		time.Date(currentClosing.Year(), currentClosing.Month(), 1, 0, 0, 0, 0, loc))
	if _, err := conn.Exec(`UPDATE financial_transactions SET credit_card_metadata = NULL WHERE id = ?`, cardBID); err != nil {
		t.Fatalf("remove second card bill metadata: %v", err)
	}

	// A payment with a billId must not be subtracted from the cost. It is
	// explicitly ignored and is also a CREDIT, so neither the local decision
	// nor the movement sign can make the widget negative.
	ignoredPaymentID := insertTransaction(t, conn)
	moveTransactionToAccount(t, conn, ignoredPaymentID, currentAccountID)
	setCardDebtTransaction(t, conn, ignoredPaymentID, time.Date(currentClosing.Year(), currentClosing.Month(), 3, 12, 0, 0, 0, loc), "-6133.39", "CREDIT", nil)
	insertBillForTransaction(t, conn, ignoredPaymentID, "bill-payment", currentClosing,
		time.Date(currentClosing.Year(), currentClosing.Month(), 10, 0, 0, 0, 0, loc))
	resp := doJSON(t, http.MethodPut, srv.URL+"/api/transactions/"+ignoredPaymentID+"/inclusion", map[string]string{"state": "ignored"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ignore payment status = %d, want 200", resp.StatusCode)
	}

	// A considered CREDIT (refund/reversal) reduces the cost by its absolute
	// value; this is separate from the ignored bill payment above.
	consideredCreditID := insertTransaction(t, conn)
	moveTransactionToAccount(t, conn, consideredCreditID, currentAccountID)
	setCardDebtTransaction(t, conn, consideredCreditID, time.Date(currentClosing.Year(), currentClosing.Month(), 4, 12, 0, 0, 0, loc), "-12.00", "CREDIT", nil)

	// An ignored debit is also excluded even though it is in the current
	// cycle. This keeps the test focused on the considered set rather than on
	// the provider balance.
	ignoredDebitID := insertTransaction(t, conn)
	moveTransactionToAccount(t, conn, ignoredDebitID, currentAccountID)
	setCardDebtTransaction(t, conn, ignoredDebitID, time.Date(currentClosing.Year(), currentClosing.Month(), 7, 12, 0, 0, 0, loc), "-999.00", "DEBIT", nil)
	resp = doJSON(t, http.MethodPut, srv.URL+"/api/transactions/"+ignoredDebitID+"/inclusion", map[string]string{"state": "ignored"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ignore debit status = %d, want 200", resp.StatusCode)
	}

	var cardBAccountID string
	if err := conn.QueryRow(`SELECT account_id FROM financial_transactions WHERE id = ?`, cardBID).Scan(&cardBAccountID); err != nil {
		t.Fatalf("second card account id: %v", err)
	}
	if _, err := conn.Exec(`UPDATE financial_accounts SET account_type = 'CREDIT', balance = '1000.00' WHERE id = ?`, currentAccountID); err != nil {
		t.Fatalf("set first credit account: %v", err)
	}
	if _, err := conn.Exec(`UPDATE financial_accounts SET account_type = 'CREDIT', balance = '2000.00' WHERE id = ?`, cardBAccountID); err != nil {
		t.Fatalf("set second credit account: %v", err)
	}

	bankID := insertTransaction(t, conn)
	var bankAccountID string
	if err := conn.QueryRow(`SELECT account_id FROM financial_transactions WHERE id = ?`, bankID).Scan(&bankAccountID); err != nil {
		t.Fatalf("bank account id: %v", err)
	}
	if _, err := conn.Exec(`UPDATE financial_accounts SET balance = '999999.00' WHERE id = ?`, bankAccountID); err != nil {
		t.Fatalf("set bank account balance: %v", err)
	}

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/payables/total-owed", nil)
	var total map[string]any
	decodeJSON(t, resp, &total)
	// First card: 100 + 25 + 10 - 12 = 123. The current-cycle payment,
	// ignored debit, previous-bill, no-bill outside-cycle, and next-closing
	// transactions do not count. Second card: 50. The bank account and both
	// provider balances are isolated from the result.
	if total["future_installments_total"] != "173.00" {
		t.Errorf("future_installments_total = %v, want 173.00", total["future_installments_total"])
	}
	if total["total_owed"] != "173.00" {
		t.Errorf("total_owed = %v, want 173.00", total["total_owed"])
	}
}

func TestDebtTotalOwedReturnsZeroWithoutClosingDate(t *testing.T) {
	srv, conn := newTestServer(t)
	loc := time.Local
	now := time.Now().In(loc)
	txID := insertTransaction(t, conn)
	setCardDebtTransaction(t, conn, txID, now, "-40.00", "DEBIT", nil)
	insertBillForTransaction(t, conn, txID, "bill-without-closing",
		time.Date(now.Year(), now.Month(), 2, 0, 0, 0, 0, loc), now)
	if _, err := conn.Exec(`UPDATE financial_bills SET closing_date = NULL WHERE external_id = ?`, "bill-without-closing"); err != nil {
		t.Fatalf("remove closing date: %v", err)
	}

	resp := doJSON(t, http.MethodPut, srv.URL+"/api/transactions/"+txID+"/inclusion", map[string]string{"state": "ignored"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ignore status = %d, want 200", resp.StatusCode)
	}

	var accountID string
	if err := conn.QueryRow(`SELECT account_id FROM financial_transactions WHERE id = ?`, txID).Scan(&accountID); err != nil {
		t.Fatalf("account id: %v", err)
	}
	if _, err := conn.Exec(`UPDATE financial_accounts SET account_type = 'CREDIT', balance = '100.00' WHERE id = ?`, accountID); err != nil {
		t.Fatalf("set credit account: %v", err)
	}

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/payables/total-owed", nil)
	var total map[string]any
	decodeJSON(t, resp, &total)
	if total["future_installments_total"] != "0" {
		t.Errorf("future_installments_total = %v, want 0", total["future_installments_total"])
	}
}

func setCardDebtTransaction(t *testing.T, conn *sql.DB, transactionID string, occurredAt time.Time, amount, movementType string, metadata *string) {
	t.Helper()
	if _, err := conn.Exec(`UPDATE financial_transactions
		SET occurred_at = ?, amount = ?, amount_in_account_currency = ?, movement_type = ?, credit_card_metadata = ?
		WHERE id = ?`, db.FormatTime(occurredAt), amount, amount, movementType, metadata, transactionID); err != nil {
		t.Fatalf("set card transaction %s: %v", transactionID, err)
	}
}

func moveTransactionToAccount(t *testing.T, conn *sql.DB, transactionID, accountID string) {
	t.Helper()
	var sourceID, rawImportID string
	if err := conn.QueryRow(`SELECT source_id, current_raw_import_id FROM financial_accounts WHERE id = ?`, accountID).Scan(&sourceID, &rawImportID); err != nil {
		t.Fatalf("account source for transaction %s: %v", transactionID, err)
	}
	if _, err := conn.Exec(`UPDATE financial_transactions
		SET account_id = ?, source_id = ?, current_raw_import_id = ? WHERE id = ?`,
		accountID, sourceID, rawImportID, transactionID); err != nil {
		t.Fatalf("move transaction %s to account %s: %v", transactionID, accountID, err)
	}
}

func insertBillForTransaction(t *testing.T, conn *sql.DB, transactionID, externalID string, closingDate, dueDate time.Time) {
	t.Helper()
	var accountID, sourceID, rawImportID string
	if err := conn.QueryRow(`
		SELECT fa.id, fa.source_id, fa.current_raw_import_id
		FROM financial_transactions ft
		JOIN financial_accounts fa ON fa.id = ft.account_id
		WHERE ft.id = ?`, transactionID).Scan(&accountID, &sourceID, &rawImportID); err != nil {
		t.Fatalf("transaction account for bill: %v", err)
	}
	billID := uuid.NewString()
	now := db.FormatTime(time.Now())
	providerClosingDate := providerCalendarDate(closingDate)
	providerDueDate := providerCalendarDate(dueDate)
	if _, err := conn.Exec(`INSERT INTO financial_bills (
		id, source_id, account_id, external_id, due_date, closing_date,
		current_raw_import_id, normalized_hash, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, 'hash', ?, ?)`,
		billID, sourceID, accountID, externalID, db.FormatTime(providerDueDate), db.FormatTime(providerClosingDate), rawImportID, now, now); err != nil {
		t.Fatalf("insert bill: %v", err)
	}
	if _, err := conn.Exec(`UPDATE financial_transactions SET credit_card_metadata = ? WHERE id = ?`,
		`{"billId":"`+externalID+`"}`, transactionID); err != nil {
		t.Fatalf("set bill metadata: %v", err)
	}
}

func providerCalendarDate(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func stringPointer(value string) *string {
	return &value
}

func TestSpendingByCategoryOverHTTP(t *testing.T) {
	srv, conn := newTestServer(t)

	resp := doJSON(t, http.MethodPost, srv.URL+"/api/categories", map[string]string{"name": "Mercado", "kind": "expense", "icon": "shopping-cart", "color": "#2a78d6"})
	var category map[string]any
	decodeJSON(t, resp, &category)
	categoryID := category["id"].(string)

	categorizedTxID := insertTransaction(t, conn)
	resp = doJSON(t, http.MethodPut, srv.URL+"/api/transactions/"+categorizedTxID+"/category", map[string]string{"category_id": categoryID})
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("assign category status = %d, want 200", resp.StatusCode)
	}

	uncategorizedTxID := insertTransaction(t, conn)
	if _, err := conn.Exec(`UPDATE financial_transactions SET amount = '-10.00', amount_in_account_currency = '-10.00' WHERE id = ?`, uncategorizedTxID); err != nil {
		t.Fatalf("update transaction: %v", err)
	}

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/transactions/spending-by-category?timezone=UTC", nil)
	var body map[string]any
	decodeJSON(t, resp, &body)
	if body["currency_code"] != "BRL" {
		t.Errorf("currency_code = %v, want BRL", body["currency_code"])
	}
	if body["total"] != "52.00" { // -42.00 categorized + -10.00 uncategorized
		t.Errorf("total = %v, want 52.00", body["total"])
	}
	items := body["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2: %+v", len(items), items)
	}
	first := items[0].(map[string]any)
	if first["category_id"] != categoryID || first["category_name"] != "Mercado" || first["amount"] != "42.00" {
		t.Errorf("items[0] = %+v", first)
	}
	second := items[1].(map[string]any)
	if second["category_id"] != nil || second["category_name"] != "Sem categoria" || second["amount"] != "10.00" {
		t.Errorf("items[1] = %+v", second)
	}

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/transactions/spending-by-category?timezone=Not/AZone", nil)
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("invalid timezone status = %d, want 400", resp.StatusCode)
	}
}

func TestCreateDebtValidation(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := doJSON(t, http.MethodPost, srv.URL+"/api/payables", map[string]any{"kind": "debt", "name": "", "total_amount": "100.00"})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Errorf("empty name status = %d, want 422", resp.StatusCode)
	}
	resp = doJSON(t, http.MethodPost, srv.URL+"/api/payables", map[string]any{"kind": "debt", "name": "x", "total_amount": "-1.00"})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Errorf("non-positive amount status = %d, want 422", resp.StatusCode)
	}
}

func TestHealth(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := doJSON(t, http.MethodGet, srv.URL+"/health", nil)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestSPAFallbackAndAssetsAndUnknownAPI(t *testing.T) {
	srv, _ := newTestServer(t)

	resp := doJSON(t, http.MethodGet, srv.URL+"/transactions", nil)
	resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "text/html; charset=utf-8" {
		t.Errorf("SPA fallback: status=%d content-type=%s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}

	resp = doJSON(t, http.MethodGet, srv.URL+"/assets/app.js", nil)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("asset: status=%d", resp.StatusCode)
	}

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/does-not-exist", nil)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("unknown api: status=%d, want 404", resp.StatusCode)
	}
}

func TestCategoryLifecycleOverHTTP(t *testing.T) {
	srv, _ := newTestServer(t)

	resp := doJSON(t, http.MethodPost, srv.URL+"/api/categories", map[string]string{"name": "Assinaturas", "kind": "expense", "icon": "wifi", "color": "#099268"})
	if resp.StatusCode != 201 {
		t.Fatalf("create status = %d, want 201", resp.StatusCode)
	}
	var created map[string]any
	decodeJSON(t, resp, &created)
	id := created["id"].(string)

	resp = doJSON(t, http.MethodPatch, srv.URL+"/api/categories/"+id, map[string]any{"is_active": false})
	if resp.StatusCode != 200 {
		t.Fatalf("update status = %d, want 200", resp.StatusCode)
	}
	var updated map[string]any
	decodeJSON(t, resp, &updated)
	if updated["is_active"] != false {
		t.Errorf("is_active = %v, want false", updated["is_active"])
	}

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/categories", nil)
	var list []map[string]any
	decodeJSON(t, resp, &list)
	if len(list) != 30 { // 29 seeded + 1 created
		t.Errorf("len(list) = %d, want 30", len(list))
	}
}

func TestTransactionInclusionAndCategoryOverHTTP(t *testing.T) {
	srv, conn := newTestServer(t)
	txID := insertTransaction(t, conn)

	resp := doJSON(t, http.MethodPut, srv.URL+"/api/transactions/"+txID+"/inclusion", map[string]string{"state": "ignored"})
	if resp.StatusCode != 200 {
		t.Fatalf("inclusion status = %d, want 200", resp.StatusCode)
	}
	var inclusion map[string]any
	decodeJSON(t, resp, &inclusion)
	if inclusion["state"] != "ignored" || inclusion["transaction_id"] != txID {
		t.Errorf("inclusion result = %+v", inclusion)
	}

	resp = doJSON(t, http.MethodPut, srv.URL+"/api/transactions/does-not-exist/inclusion", map[string]string{"state": "ignored"})
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("unknown transaction inclusion status = %d, want 404", resp.StatusCode)
	}

	var categories []map[string]any
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/categories", nil)
	decodeJSON(t, resp, &categories)
	categoryID := categories[0]["id"].(string)

	resp = doJSON(t, http.MethodPut, srv.URL+"/api/transactions/"+txID+"/category", map[string]string{"category_id": categoryID})
	if resp.StatusCode != 200 {
		t.Fatalf("category status = %d, want 200", resp.StatusCode)
	}
	var category map[string]any
	decodeJSON(t, resp, &category)
	if category["category_id"] != categoryID || category["origin"] != "manual" {
		t.Errorf("category result = %+v", category)
	}

	resp = doJSON(t, http.MethodPut, srv.URL+"/api/transactions/"+txID+"/category", map[string]string{"category_id": "not-a-uuid"})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Errorf("invalid category status = %d, want 422", resp.StatusCode)
	}
}

func TestIgnoringLinkedBankLineUnlinksInvestmentReconciliation(t *testing.T) {
	srv, conn := newTestServer(t)
	account := investmentID(t, createCustodyAccount(t, srv, "Corretora"))
	bankLine := insertTransaction(t, conn)
	deposit := investmentID(t, investmentCall(t, srv, http.MethodPost, "/api/investment-operations", map[string]any{
		"account_id": account, "position_id": nil, "kind": "deposit",
		"occurred_on": "2026-01-10", "amount": "42.00",
	}, http.StatusCreated))
	investmentCall(t, srv, http.MethodPost, "/api/investment-reconciliations", map[string]any{
		"operation_id": deposit, "financial_transaction_id": bankLine, "amount": "42.00",
	}, http.StatusCreated)

	// Ignoring the line must unlink it (via investments.UnlinkTransactionIfPresent),
	// the same way payables and recurrences let go of it: the aporte is then
	// reported by the operation alone instead of vanishing with the line.
	resp := doJSON(t, http.MethodPut, srv.URL+"/api/transactions/"+bankLine+"/inclusion", map[string]string{"state": "ignored"})
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("ignore status = %d, want 200", resp.StatusCode)
	}
	if links := investmentItems(t, srv, "/api/investment-reconciliations?operation_id="+deposit); len(links) != 0 {
		t.Fatalf("links survived ignoring the bank line: %+v", links)
	}
	// The operation is ordinary again, and the ignored line cannot be linked
	// back while it stays out of the totals.
	if slug := investmentProblem(t, srv, http.MethodPost, "/api/investment-reconciliations", map[string]any{
		"operation_id": deposit, "financial_transaction_id": bankLine, "amount": "42.00",
	}, http.StatusConflict); slug != "investment-invalid-reconciliation-link" {
		t.Errorf("relink ignored line slug = %q", slug)
	}
	investmentCall(t, srv, http.MethodDelete, "/api/investment-operations/"+deposit, nil, http.StatusNoContent)
}

func TestQueryTransactionsOverHTTP(t *testing.T) {
	srv, conn := newTestServer(t)
	txID := insertTransaction(t, conn)

	var categories []map[string]any
	resp := doJSON(t, http.MethodGet, srv.URL+"/api/categories", nil)
	decodeJSON(t, resp, &categories)
	var categoryID string
	for _, c := range categories {
		if c["kind"] == "expense" {
			categoryID = c["id"].(string)
			break
		}
	}
	resp = doJSON(t, http.MethodPut, srv.URL+"/api/transactions/"+txID+"/category", map[string]string{"category_id": categoryID})
	resp.Body.Close()

	body := map[string]any{
		"timezone": "UTC", "group_by": "none", "page": 1, "page_size": 50,
		"filters": map[string]any{
			"date_from": nil, "date_to": nil, "description": nil, "account_id": nil,
			"institution": nil, "category_id": nil, "classification": nil,
			"provider_status": nil, "amount_min": nil, "amount_max": nil, "uncategorized": nil,
		},
	}
	resp = doJSON(t, http.MethodPost, srv.URL+"/api/transactions/query", body)
	if resp.StatusCode != 200 {
		t.Fatalf("query status = %d, want 200", resp.StatusCode)
	}
	var result map[string]any
	decodeJSON(t, resp, &result)
	items, _ := result["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	totals, _ := result["totals"].([]any)
	if len(totals) != 1 {
		t.Fatalf("len(totals) = %d, want 1", len(totals))
	}
	total := totals[0].(map[string]any)
	if total["outflow"] != "42.00" {
		t.Errorf("outflow = %v, want 42.00", total["outflow"])
	}

	body["filters"].(map[string]any)["credit_card"] = true
	for _, accountType := range []string{"BANK", "CREDIT"} {
		if _, err := conn.Exec(`UPDATE financial_accounts SET account_type = ? WHERE id = (SELECT account_id FROM financial_transactions WHERE id = ?)`, accountType, txID); err != nil {
			t.Fatal(err)
		}
		resp = doJSON(t, http.MethodPost, srv.URL+"/api/transactions/query", body)
		if resp.StatusCode != 200 {
			t.Fatalf("credit card query status: %d", resp.StatusCode)
		}
		decodeJSON(t, resp, &result)
		items, _ = result["items"].([]any)
		want := 0
		if accountType == "CREDIT" {
			want = 1
		}
		if len(items) != want {
			t.Fatalf("account type %s: got %d items, want %d", accountType, len(items), want)
		}
	}

	// The legacy singular account_id and the plural account_ids are the same
	// filter; a set that includes the account keeps the transaction, one
	// that does not drops it.
	delete(body["filters"].(map[string]any), "credit_card")
	var accountID string
	if err := conn.QueryRow(`SELECT account_id FROM financial_transactions WHERE id = ?`, txID).Scan(&accountID); err != nil {
		t.Fatal(err)
	}
	for name, filters := range map[string]map[string]any{
		"account_id":            {"account_id": accountID},
		"account_ids":           {"account_ids": []string{"missing", accountID}},
		"account_ids elsewhere": {"account_ids": []string{"missing"}},
	} {
		for k, v := range filters {
			body["filters"].(map[string]any)[k] = v
		}
		resp = doJSON(t, http.MethodPost, srv.URL+"/api/transactions/query", body)
		if resp.StatusCode != 200 {
			t.Fatalf("%s: status %d", name, resp.StatusCode)
		}
		decodeJSON(t, resp, &result)
		items, _ = result["items"].([]any)
		want := 1
		if name == "account_ids elsewhere" {
			want = 0
		}
		if len(items) != want {
			t.Errorf("%s: got %d items, want %d", name, len(items), want)
		}
		for k := range filters {
			body["filters"].(map[string]any)[k] = nil
		}
	}

	// An unknown classification is rejected up front instead of matching
	// nothing.
	body["filters"].(map[string]any)["classification"] = "xpto"
	resp = doJSON(t, http.MethodPost, srv.URL+"/api/transactions/query", body)
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("unknown classification status = %d, want 400", resp.StatusCode)
	}

	badBody := map[string]any{"timezone": "Not/AZone", "group_by": "none", "page": 1, "page_size": 50,
		"filters": map[string]any{
			"date_from": nil, "date_to": nil, "description": nil, "account_id": nil,
			"institution": nil, "category_id": nil, "classification": nil,
			"provider_status": nil, "amount_min": nil, "amount_max": nil, "uncategorized": nil,
		},
	}
	resp = doJSON(t, http.MethodPost, srv.URL+"/api/transactions/query", badBody)
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("invalid timezone status = %d, want 400", resp.StatusCode)
	}
}

func TestCategoryBreakdownOverHTTP(t *testing.T) {
	srv, conn := newTestServer(t)
	for _, entry := range []struct {
		date, amount, movement string
	}{
		{"2024-02-29T15:00:00Z", "100.00", "CREDIT"},
		{"2024-03-01T01:00:00Z", "25.00", "CREDIT"},  // Still February in São Paulo.
		{"2024-03-01T03:00:00Z", "500.00", "CREDIT"}, // March locally.
		{"2024-02-10T15:00:00Z", "-42.00", "DEBIT"},
		{"2023-12-31T23:00:00Z", "70.00", "CREDIT"},
		{"2024-01-01T03:00:00Z", "80.00", "CREDIT"},
	} {
		id := insertTransaction(t, conn)
		date, err := time.Parse(time.RFC3339, entry.date)
		if err != nil {
			t.Fatal(err)
		}
		setCardDebtTransaction(t, conn, id, date, entry.amount, entry.movement, nil)
	}
	for _, tc := range []struct {
		month, direction, total string
	}{
		{"2024-02", "inflow", "125.00"},
		{"2024-02", "outflow", "42.00"},
		{"2024-03", "inflow", "500.00"},
		{"2023-12", "inflow", "70.00"},
		{"2024-01", "inflow", "80.00"},
		{"2024-04", "inflow", "0"},
	} {
		t.Run(tc.month+"/"+tc.direction, func(t *testing.T) {
			resp := doJSON(t, http.MethodGet, srv.URL+"/api/transactions/category-breakdown?timezone=America%2FSao_Paulo&month="+tc.month+"&classification="+tc.direction, nil)
			if resp.StatusCode != 200 {
				t.Fatalf("status = %d", resp.StatusCode)
			}
			var body map[string]any
			decodeJSON(t, resp, &body)
			if body["total"] != tc.total || body["month"] != tc.month || body["classification"] != tc.direction {
				t.Fatalf("unexpected response: %+v", body)
			}
			items := body["items"].([]any)
			if tc.total == "0" {
				if len(items) != 0 {
					t.Fatalf("expected empty items: %+v", items)
				}
			} else if len(items) != 1 || items[0].(map[string]any)["category_name"] != "Sem categoria" {
				t.Fatalf("unexpected categories: %+v", items)
			}
		})
	}
	for _, tc := range []struct {
		query, total string
		from, to     any
	}{
		{"date_from=2024-02-29&date_to=2024-02-29", "125.00", "2024-02-29", "2024-02-29"},
		{"date_from=2024-02-29&date_to=2024-03-01", "625.00", "2024-02-29", "2024-03-01"},
		{"date_from=2024-01-01&date_to=2024-12-31", "705.00", "2024-01-01", "2024-12-31"},
		{"date_from=2099-01-01&date_to=2099-12-31", "0", "2099-01-01", "2099-12-31"},
		{"period=all", "775.00", nil, nil},
	} {
		t.Run(tc.query, func(t *testing.T) {
			resp := doJSON(t, http.MethodGet, srv.URL+"/api/transactions/category-breakdown?timezone=America%2FSao_Paulo&classification=inflow&"+tc.query, nil)
			if resp.StatusCode != 200 {
				t.Fatalf("status = %d", resp.StatusCode)
			}
			var body map[string]any
			decodeJSON(t, resp, &body)
			if body["total"] != tc.total || body["date_from"] != tc.from || body["date_to"] != tc.to {
				t.Fatalf("unexpected response: %+v", body)
			}
			if _, ok := body["month"]; ok {
				t.Fatalf("range response includes month: %+v", body)
			}
		})
	}
	for _, query := range []string{
		"date_from=2024-02-01", "date_to=2024-02-01",
		"date_from=2024-02-30&date_to=2024-03-01",
		"date_from=2024-03-01&date_to=2024-02-01",
		"date_from=0000-01-01&date_to=2024-01-01",
		"month=2024-02&date_from=2024-02-01&date_to=2024-02-29",
		"month=2024-02&period=all", "period=all&date_from=2024-02-01",
		"period=month", "date_from=&date_to=",
	} {
		resp := doJSON(t, http.MethodGet, srv.URL+"/api/transactions/category-breakdown?timezone=UTC&classification=inflow&"+query, nil)
		resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Errorf("%s: status = %d, want 400", query, resp.StatusCode)
		}
	}

	for _, query := range []string{
		"timezone=UTC&month=2024-02",
		"timezone=UTC&classification=inflow",
		"month=2024-02&classification=inflow",
		"timezone=Invalid&month=2024-02&classification=inflow",
		"timezone=UTC&month=2024-13&classification=inflow",
		"timezone=UTC&month=2024-2&classification=inflow",
		"timezone=UTC&month=2024-02&classification=transfer",
	} {
		resp := doJSON(t, http.MethodGet, srv.URL+"/api/transactions/category-breakdown?"+query, nil)
		resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Errorf("%s: status = %d, want 400", query, resp.StatusCode)
		}
	}
}

func TestOversizedJSONBodyIsRejected(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := doJSON(t, "POST", srv.URL+"/api/categories", map[string]string{"name": strings.Repeat("a", 2<<20)})
	resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", resp.StatusCode)
	}
}
