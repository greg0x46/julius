package httpapi_test

import (
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

// The key lists below are the frontend's, copied field for field from
// frontend/src/api/contracts.ts. Its parsers reject a record with an extra or
// a missing key, so a response that drifts from these lists breaks every
// investment screen at once — asserting the key set here is what turns that
// into a failing test instead of a blank page.
var (
	investmentAccountResponseKeys = []string{
		"id", "name", "kind", "currency_code", "source_id", "source_display_name",
		"financial_account_id", "active", "cash_balance", "created_at", "updated_at",
	}
	investmentPortfolioResponseKeys = []string{
		"id", "name", "target_amount", "target_date", "notes", "current_value",
		"progress", "created_at", "updated_at",
	}
	investmentPositionResponseKeys = []string{
		"id", "source", "account_id", "asset_id", "portfolio_id", "name", "ticker", "asset_type",
		"quantity", "average_cost", "current_value", "current_unit_price", "valued_on",
		"currency_code", "closed", "linked_investment_id", "notes", "valuation_basis",
	}
	investmentOperationResponseKeys = []string{
		"id", "account_id", "position_id", "transfer_id", "kind", "occurred_on", "amount", "quantity",
		"unit_price", "fees", "taxes", "notes", "source", "is_editable", "created_at", "updated_at",
	}
	investmentReconciliationResponseKeys = []string{
		"id", "operation_id", "financial_transaction_id",
		"financial_investment_transaction_id", "amount", "created_at",
	}
	investmentSummaryResponseKeys = []string{
		"currency_code", "total_value", "manual_value", "synced_value", "cash_balance",
		"unrealized_gain", "portfolios", "accounts",
	}
	investmentSummaryPortfolioKeys = []string{"portfolio_id", "name", "current_value", "target_amount", "progress"}
	investmentSummaryAccountKeys   = []string{"account_id", "name", "kind", "current_value", "cash_balance"}
)

func assertInvestmentKeys(t *testing.T, label string, item map[string]any, want []string) {
	t.Helper()
	got := make([]string, 0, len(item))
	for key := range item {
		got = append(got, key)
	}
	sort.Strings(got)
	expected := append([]string(nil), want...)
	sort.Strings(expected)
	if strings.Join(got, ",") != strings.Join(expected, ",") {
		t.Errorf("%s keys = %v, want %v", label, got, expected)
	}
}

// investmentCall sends one request and insists on the status the frontend
// client waits for: it treats any other status as a failure, so a handler
// answering 200 where the client expects 201 cannot pass.
func investmentCall(t *testing.T, srv *httptest.Server, method, path string, body any, want int) map[string]any {
	t.Helper()
	resp := doJSON(t, method, srv.URL+path, body)
	if resp.StatusCode != want {
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("%s %s status = %d, want %d: %s", method, path, resp.StatusCode, want, raw)
	}
	if want == http.StatusNoContent {
		resp.Body.Close()
		return nil
	}
	var item map[string]any
	decodeJSON(t, resp, &item)
	return item
}

// investmentProblem asserts a refusal and returns its problem slug, so each
// conflict can be told apart from the others by more than its status.
func investmentProblem(t *testing.T, srv *httptest.Server, method, path string, body any, wantStatus int) string {
	t.Helper()
	resp := doJSON(t, method, srv.URL+path, body)
	var problem map[string]any
	decodeJSON(t, resp, &problem)
	if resp.StatusCode != wantStatus {
		t.Fatalf("%s %s status = %d, want %d: %+v", method, path, resp.StatusCode, wantStatus, problem)
	}
	kind, _ := problem["type"].(string)
	return strings.TrimPrefix(kind, "/problems/")
}

func investmentItems(t *testing.T, srv *httptest.Server, path string) []map[string]any {
	t.Helper()
	resp := doJSON(t, http.MethodGet, srv.URL+path, nil)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("GET %s status = %d, want 200", path, resp.StatusCode)
	}
	var payload struct {
		Items []map[string]any `json:"items"`
	}
	decodeJSON(t, resp, &payload)
	return payload.Items
}

func investmentRawBody(t *testing.T, srv *httptest.Server, path string) string {
	t.Helper()
	resp := doJSON(t, http.MethodGet, srv.URL+path, nil)
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return strings.TrimSpace(string(raw))
}

func createCustodyAccount(t *testing.T, srv *httptest.Server, name string) map[string]any {
	t.Helper()
	// Exactly the body investmentPortfolio.ts sends, kind included.
	return investmentCall(t, srv, http.MethodPost, "/api/investment-accounts", map[string]any{
		"name": name, "currency_code": "BRL", "financial_account_id": nil, "kind": "manual",
	}, http.StatusCreated)
}

func investmentID(t *testing.T, item map[string]any) string {
	t.Helper()
	id, _ := item["id"].(string)
	if id == "" {
		t.Fatalf("missing id in %+v", item)
	}
	return id
}

func assertMoney(t *testing.T, label string, got any, want string) {
	t.Helper()
	text, ok := got.(string)
	if !ok {
		t.Fatalf("%s = %v, want decimal text", label, got)
	}
	gotValue, err := decimal.NewFromString(text)
	if err != nil {
		t.Fatalf("%s = %q, not a decimal: %v", label, text, err)
	}
	wantValue, err := decimal.NewFromString(want)
	if err != nil {
		t.Fatalf("bad expectation %q: %v", want, err)
	}
	if !gotValue.Equal(wantValue) {
		t.Errorf("%s = %s, want %s", label, text, want)
	}
}

// integratedAccountID returns the local grouping row a connection gets. The
// id is deterministic and is not a uuid on purpose.
func integratedAccountID(t *testing.T, conn *sql.DB, investmentIdentifier string) string {
	t.Helper()
	var sourceID string
	if err := conn.QueryRow(`SELECT source_id FROM financial_investments WHERE id = ?`, investmentIdentifier).Scan(&sourceID); err != nil {
		t.Fatalf("lookup source_id: %v", err)
	}
	return "integrated:" + sourceID
}

func TestInvestmentAccountLifecycleOverHTTP(t *testing.T) {
	srv, conn := newTestServer(t)

	if body := investmentRawBody(t, srv, "/api/investment-accounts"); body != `{"items":[]}` {
		t.Errorf("empty account list = %s, want an items envelope with an empty array", body)
	}

	created := createCustodyAccount(t, srv, "Corretora XP")
	assertInvestmentKeys(t, "account", created, investmentAccountResponseKeys)
	id := investmentID(t, created)
	if created["kind"] != "manual" || created["currency_code"] != "BRL" || created["active"] != true {
		t.Errorf("created account = %+v", created)
	}
	if created["source_id"] != nil || created["source_display_name"] != nil {
		t.Errorf("a manual account has no connection: %+v", created)
	}
	assertMoney(t, "cash_balance", created["cash_balance"], "0")

	items := investmentItems(t, srv, "/api/investment-accounts")
	if len(items) != 1 || items[0]["id"] != id {
		t.Fatalf("list = %+v, want the account just created", items)
	}

	// The edit form sends only the name and the activity flag: an omitted
	// financial_account_id must keep the current link, not clear it.
	bank := insertAccount(t, conn, "BANK", "Conta corrente", nil, nil)
	linked := investmentCall(t, srv, http.MethodPut, "/api/investment-accounts/"+id,
		map[string]any{"name": "Corretora XP", "financial_account_id": bank.accountID}, http.StatusOK)
	if linked["financial_account_id"] != bank.accountID {
		t.Fatalf("link not persisted: %+v", linked)
	}
	renamed := investmentCall(t, srv, http.MethodPut, "/api/investment-accounts/"+id,
		map[string]any{"name": "Corretora principal", "active": false}, http.StatusOK)
	if renamed["name"] != "Corretora principal" || renamed["active"] != false {
		t.Errorf("update = %+v, want the new name and active=false", renamed)
	}
	if renamed["financial_account_id"] != bank.accountID {
		t.Errorf("an omitted financial_account_id unlinked the bank account: %+v", renamed)
	}
	// The flag is stored by the same domain write as the name, so it has to
	// survive a fresh read and come back when the form re-enables the account.
	if items := investmentItems(t, srv, "/api/investment-accounts"); len(items) != 1 || items[0]["active"] != false {
		t.Errorf("list after deactivation = %+v, want active=false persisted", items)
	}
	reactivated := investmentCall(t, srv, http.MethodPut, "/api/investment-accounts/"+id,
		map[string]any{"name": "Corretora principal", "active": true}, http.StatusOK)
	if reactivated["active"] != true || reactivated["financial_account_id"] != bank.accountID {
		t.Errorf("reactivation = %+v, want active=true with the link kept", reactivated)
	}
	// An explicit null is the only way the form unlinks the bank account.
	unlinked := investmentCall(t, srv, http.MethodPut, "/api/investment-accounts/"+id,
		map[string]any{"name": "Corretora principal", "active": true, "financial_account_id": nil}, http.StatusOK)
	if unlinked["financial_account_id"] != nil {
		t.Errorf("an explicit null financial_account_id kept the link: %+v", unlinked)
	}
	if items := investmentItems(t, srv, "/api/investment-accounts"); len(items) != 1 || items[0]["financial_account_id"] != nil {
		t.Errorf("list after unlink = %+v, want financial_account_id=null persisted", items)
	}

	if slug := investmentProblem(t, srv, http.MethodPut, "/api/investment-accounts/"+uuidLike,
		map[string]any{"name": "Sem conta"}, http.StatusNotFound); slug != "investment-account-not-found" {
		t.Errorf("unknown account slug = %q", slug)
	}

	// An account still holding a position refuses deletion, with its own slug.
	position := investmentCall(t, srv, http.MethodPost, "/api/investment-positions", map[string]any{
		"account_id": id, "name": "Tesouro Selic", "asset_type": "Renda fixa",
	}, http.StatusCreated)
	if slug := investmentProblem(t, srv, http.MethodDelete, "/api/investment-accounts/"+id, nil, http.StatusConflict); slug != "investment-account-has-positions" {
		t.Errorf("delete with positions slug = %q", slug)
	}
	investmentCall(t, srv, http.MethodDelete, "/api/investment-positions/"+investmentID(t, position), nil, http.StatusNoContent)
	investmentCall(t, srv, http.MethodDelete, "/api/investment-accounts/"+id, nil, http.StatusNoContent)
	if items := investmentItems(t, srv, "/api/investment-accounts"); len(items) != 0 {
		t.Errorf("after delete list = %+v, want empty", items)
	}
}

const uuidLike = "11111111-1111-4111-8111-111111111111"

func TestInvestmentPortfolioLifecycleOverHTTP(t *testing.T) {
	srv, _ := newTestServer(t)

	if body := investmentRawBody(t, srv, "/api/investment-portfolios"); body != `{"items":[]}` {
		t.Errorf("empty portfolio list = %s, want an items envelope with an empty array", body)
	}

	created := investmentCall(t, srv, http.MethodPost, "/api/investment-portfolios", map[string]any{
		"name": "Reserva de emergência", "target_amount": "30000.00",
		"target_date": "2027-12-31", "notes": "seis meses de custo",
	}, http.StatusCreated)
	assertInvestmentKeys(t, "portfolio", created, investmentPortfolioResponseKeys)
	id := investmentID(t, created)
	assertMoney(t, "target_amount", created["target_amount"], "30000.00")
	if created["target_date"] != "2027-12-31" {
		t.Errorf("target_date = %v, want a plain YYYY-MM-DD", created["target_date"])
	}
	assertMoney(t, "current_value", created["current_value"], "0")
	if created["progress"] == nil {
		t.Errorf("progress = nil, want 0 against a known target")
	}

	updated := investmentCall(t, srv, http.MethodPut, "/api/investment-portfolios/"+id, map[string]any{
		"name": "Reserva", "target_amount": nil, "target_date": nil, "notes": nil,
	}, http.StatusOK)
	if updated["name"] != "Reserva" || updated["target_amount"] != nil || updated["target_date"] != nil || updated["notes"] != nil {
		t.Errorf("update = %+v", updated)
	}
	if updated["progress"] != nil {
		t.Errorf("progress = %v, want nil without a target", updated["progress"])
	}

	if slug := investmentProblem(t, srv, http.MethodPost, "/api/investment-portfolios",
		map[string]any{"name": "", "target_amount": nil, "target_date": nil, "notes": nil},
		http.StatusBadRequest); slug != "invalid-investment-input" {
		t.Errorf("empty name slug = %q", slug)
	}
	if slug := investmentProblem(t, srv, http.MethodPost, "/api/investment-portfolios",
		map[string]any{"name": "x", "target_amount": "30000", "target_date": "31/12/2027", "notes": nil},
		http.StatusBadRequest); slug != "invalid-investment-input" {
		t.Errorf("bad date slug = %q", slug)
	}
	if slug := investmentProblem(t, srv, http.MethodPost, "/api/investment-portfolios",
		map[string]any{"name": "x", "target_amount": "trinta mil", "target_date": nil, "notes": nil},
		http.StatusBadRequest); slug != "invalid-investment-input" {
		t.Errorf("bad decimal slug = %q", slug)
	}

	investmentCall(t, srv, http.MethodDelete, "/api/investment-portfolios/"+id, nil, http.StatusNoContent)
	if slug := investmentProblem(t, srv, http.MethodDelete, "/api/investment-portfolios/"+id, nil, http.StatusNotFound); slug != "investment-portfolio-not-found" {
		t.Errorf("second delete slug = %q", slug)
	}
}

func TestInvestmentPositionLifecycleOverHTTP(t *testing.T) {
	srv, _ := newTestServer(t)
	account := investmentID(t, createCustodyAccount(t, srv, "Corretora"))
	other := investmentID(t, createCustodyAccount(t, srv, "Banco"))
	goal := investmentID(t, investmentCall(t, srv, http.MethodPost, "/api/investment-portfolios", map[string]any{
		"name": "Aposentadoria", "target_amount": "1000.00", "target_date": nil, "notes": nil,
	}, http.StatusCreated))

	if body := investmentRawBody(t, srv, "/api/investment-positions"); body != `{"items":[]}` {
		t.Errorf("empty position list = %s", body)
	}

	created := investmentCall(t, srv, http.MethodPost, "/api/investment-positions", map[string]any{
		"account_id": account, "name": "ITSA4", "ticker": "ITSA4", "asset_type": "Ação",
		"portfolio_id": goal, "initial_quantity": "10", "initial_unit_cost": "9.50",
		"occurred_on": "2026-01-05", "notes": "posição inicial",
	}, http.StatusCreated)
	assertInvestmentKeys(t, "position", created, investmentPositionResponseKeys)
	id := investmentID(t, created)
	if created["source"] != "manual" || created["currency_code"] != "BRL" || created["closed"] != false {
		t.Errorf("created position = %+v", created)
	}
	assertMoney(t, "quantity", created["quantity"], "10")
	assertMoney(t, "average_cost", created["average_cost"], "9.50")
	assertMoney(t, "current_value", created["current_value"], "95.00")
	if created["valuation_basis"] != "cost_basis" {
		t.Errorf("valuation_basis = %v, want cost_basis without a valuation", created["valuation_basis"])
	}
	if created["portfolio_id"] != goal {
		t.Errorf("portfolio_id = %v, want the goal it was created under", created["portfolio_id"])
	}

	fetched := investmentCall(t, srv, http.MethodGet, "/api/investment-positions/"+id, nil, http.StatusOK)
	if fetched["id"] != id {
		t.Errorf("get = %+v", fetched)
	}
	assertInvestmentKeys(t, "position (get)", fetched, investmentPositionResponseKeys)

	investmentCall(t, srv, http.MethodPost, "/api/investment-positions", map[string]any{
		"account_id": other, "name": "CDB", "asset_type": "Renda fixa",
	}, http.StatusCreated)

	if items := investmentItems(t, srv, "/api/investment-positions"); len(items) != 2 {
		t.Fatalf("unfiltered list = %d positions, want 2", len(items))
	}
	filtered := investmentItems(t, srv, "/api/investment-positions?account_id="+account)
	if len(filtered) != 1 || filtered[0]["id"] != id {
		t.Errorf("account filter = %+v, want only the position of that account", filtered)
	}
	byGoal := investmentItems(t, srv, "/api/investment-positions?portfolio_id="+goal)
	if len(byGoal) != 1 || byGoal[0]["id"] != id {
		t.Errorf("portfolio filter = %+v", byGoal)
	}

	// Selling the whole holding closes it, and a closed holding only shows up
	// when the caller asks for it.
	investmentCall(t, srv, http.MethodPost, "/api/investment-operations", map[string]any{
		"account_id": account, "position_id": id, "kind": "sell",
		"occurred_on": "2026-02-01", "amount": "120.00", "quantity": "10",
	}, http.StatusCreated)
	if items := investmentItems(t, srv, "/api/investment-positions?account_id="+account); len(items) != 0 {
		t.Errorf("closed position still listed: %+v", items)
	}
	closed := investmentItems(t, srv, "/api/investment-positions?account_id="+account+"&include_closed=true")
	if len(closed) != 1 || closed[0]["closed"] != true {
		t.Errorf("include_closed = %+v, want the closed position", closed)
	}

	updated := investmentCall(t, srv, http.MethodPut, "/api/investment-positions/"+id, map[string]any{
		"name": "Itaúsa", "ticker": nil, "asset_type": "Ação", "portfolio_id": nil, "notes": nil,
	}, http.StatusOK)
	if updated["name"] != "Itaúsa" || updated["ticker"] != nil || updated["portfolio_id"] != nil {
		t.Errorf("update = %+v", updated)
	}

	// The holding still carries the sale, so it cannot be dropped silently.
	if slug := investmentProblem(t, srv, http.MethodDelete, "/api/investment-positions/"+id, nil, http.StatusConflict); slug != "investment-position-has-operations" {
		t.Errorf("delete with operations slug = %q", slug)
	}
	if slug := investmentProblem(t, srv, http.MethodGet, "/api/investment-positions/"+uuidLike, nil, http.StatusNotFound); slug != "investment-position-not-found" {
		t.Errorf("unknown position slug = %q", slug)
	}
}

func TestInvestmentOperationLifecycleOverHTTP(t *testing.T) {
	srv, conn := newTestServer(t)
	account := investmentID(t, createCustodyAccount(t, srv, "Corretora"))
	other := investmentID(t, createCustodyAccount(t, srv, "Banco"))

	if body := investmentRawBody(t, srv, "/api/investment-operations"); body != `{"items":[]}` {
		t.Errorf("empty operation list = %s", body)
	}

	created := investmentCall(t, srv, http.MethodPost, "/api/investment-operations", map[string]any{
		"account_id": account, "position_id": nil, "kind": "deposit",
		"occurred_on": "2026-01-10", "amount": "1000.00", "quantity": nil,
		"unit_price": nil, "fees": nil, "taxes": nil, "notes": "aporte",
	}, http.StatusCreated)
	assertInvestmentKeys(t, "operation", created, investmentOperationResponseKeys)
	id := investmentID(t, created)
	if created["kind"] != "deposit" || created["occurred_on"] != "2026-01-10" {
		t.Errorf("created operation = %+v", created)
	}
	if created["source"] != "manual" || created["is_editable"] != true {
		t.Errorf("source/is_editable = %v/%v, want the domain's manual answer", created["source"], created["is_editable"])
	}
	assertMoney(t, "amount", created["amount"], "1000.00")
	assertMoney(t, "fees", created["fees"], "0")

	investmentCall(t, srv, http.MethodPost, "/api/investment-operations", map[string]any{
		"account_id": other, "position_id": nil, "kind": "deposit",
		"occurred_on": "2026-03-10", "amount": "50.00",
	}, http.StatusCreated)

	if items := investmentItems(t, srv, "/api/investment-operations"); len(items) != 2 {
		t.Fatalf("unfiltered list = %d operations, want 2", len(items))
	}
	byAccount := investmentItems(t, srv, "/api/investment-operations?account_id="+account)
	if len(byAccount) != 1 || byAccount[0]["id"] != id {
		t.Errorf("account filter = %+v", byAccount)
	}
	byRange := investmentItems(t, srv, "/api/investment-operations?from=2026-02-01&to=2026-12-31")
	if len(byRange) != 1 || byRange[0]["account_id"] != other {
		t.Errorf("date filter = %+v, want only the March operation", byRange)
	}
	if slug := investmentProblem(t, srv, http.MethodGet, "/api/investment-operations?from=10/01/2026", nil, http.StatusBadRequest); slug != "invalid-investment-input" {
		t.Errorf("bad from slug = %q", slug)
	}

	updated := investmentCall(t, srv, http.MethodPut, "/api/investment-operations/"+id, map[string]any{
		"account_id": account, "position_id": nil, "kind": "deposit",
		"occurred_on": "2026-01-11", "amount": "1200.00",
	}, http.StatusOK)
	assertMoney(t, "amount", updated["amount"], "1200.00")
	if updated["occurred_on"] != "2026-01-11" {
		t.Errorf("occurred_on = %v", updated["occurred_on"])
	}

	// A withdrawal bigger than the cash is refused by the replay, and the
	// refusal has to leave the book exactly as it was.
	before := countInvestmentOperations(t, conn)
	slug := investmentProblem(t, srv, http.MethodPost, "/api/investment-operations", map[string]any{
		"account_id": account, "position_id": nil, "kind": "withdrawal",
		"occurred_on": "2026-01-12", "amount": "5000.00",
	}, http.StatusConflict)
	if slug != "investment-negative-cash" {
		t.Errorf("negative cash slug = %q", slug)
	}
	if after := countInvestmentOperations(t, conn); after != before {
		t.Errorf("rejected withdrawal persisted %d operation(s)", after-before)
	}

	// The same guard protects a correction: raising a withdrawal beyond the
	// available cash must not be applied either.
	withdrawal := investmentID(t, investmentCall(t, srv, http.MethodPost, "/api/investment-operations", map[string]any{
		"account_id": account, "position_id": nil, "kind": "withdrawal",
		"occurred_on": "2026-01-12", "amount": "100.00",
	}, http.StatusCreated))
	if slug := investmentProblem(t, srv, http.MethodPut, "/api/investment-operations/"+withdrawal, map[string]any{
		"account_id": account, "position_id": nil, "kind": "withdrawal",
		"occurred_on": "2026-01-12", "amount": "9000.00",
	}, http.StatusConflict); slug != "investment-negative-cash" {
		t.Errorf("negative cash on update slug = %q", slug)
	}
	for _, item := range investmentItems(t, srv, "/api/investment-operations?account_id="+account) {
		if item["id"] == withdrawal {
			assertMoney(t, "withdrawal amount after refused update", item["amount"], "100.00")
		}
	}

	investmentCall(t, srv, http.MethodDelete, "/api/investment-operations/"+withdrawal, nil, http.StatusNoContent)
	if slug := investmentProblem(t, srv, http.MethodDelete, "/api/investment-operations/"+withdrawal, nil, http.StatusNotFound); slug != "investment-operation-not-found" {
		t.Errorf("second delete slug = %q", slug)
	}

	// An unknown kind is invalid input, not a server failure.
	if slug := investmentProblem(t, srv, http.MethodPost, "/api/investment-operations", map[string]any{
		"account_id": account, "position_id": nil, "kind": "resgate",
		"occurred_on": "2026-01-12", "amount": "10.00",
	}, http.StatusBadRequest); slug != "invalid-investment-input" {
		t.Errorf("unknown kind slug = %q", slug)
	}
}

func countInvestmentOperations(t *testing.T, conn *sql.DB) int {
	t.Helper()
	var count int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM investment_operations`).Scan(&count); err != nil {
		t.Fatalf("count operations: %v", err)
	}
	return count
}

func TestInvestmentReconciliationOverHTTP(t *testing.T) {
	srv, conn := newTestServer(t)
	account := investmentID(t, createCustodyAccount(t, srv, "Corretora"))
	bankLine := insertTransaction(t, conn)
	holding := insertInvestment(t, conn)
	insertInvestmentTransaction(t, conn, holding, "BUY", "42.00")
	var importedMovement string
	if err := conn.QueryRow(`SELECT id FROM financial_investment_transactions WHERE investment_id = ?`, holding).
		Scan(&importedMovement); err != nil {
		t.Fatalf("lookup imported movement: %v", err)
	}

	deposit := investmentID(t, investmentCall(t, srv, http.MethodPost, "/api/investment-operations", map[string]any{
		"account_id": account, "position_id": nil, "kind": "deposit",
		"occurred_on": "2026-01-10", "amount": "42.00",
	}, http.StatusCreated))

	if body := investmentRawBody(t, srv, "/api/investment-reconciliations"); body != `{"items":[]}` {
		t.Errorf("empty reconciliation list = %s", body)
	}

	// The bank line and the imported movement describe the same event, so
	// they belong in one row: the cap counts every link of an operation
	// together, and two full-amount rows would over-allocate it.
	created := investmentCall(t, srv, http.MethodPost, "/api/investment-reconciliations", map[string]any{
		"operation_id": deposit, "financial_transaction_id": bankLine,
		"financial_investment_transaction_id": importedMovement, "amount": "42.00",
	}, http.StatusCreated)
	assertInvestmentKeys(t, "reconciliation", created, investmentReconciliationResponseKeys)
	if created["financial_transaction_id"] != bankLine || created["financial_investment_transaction_id"] != importedMovement {
		t.Errorf("both ids should ride in one row: %+v", created)
	}
	assertMoney(t, "amount", created["amount"], "42.00")

	links := investmentItems(t, srv, "/api/investment-reconciliations?operation_id="+deposit)
	if len(links) != 1 {
		t.Fatalf("filtered list = %+v, want the single link", links)
	}
	if other := investmentItems(t, srv, "/api/investment-reconciliations?operation_id="+uuidLike); len(other) != 0 {
		t.Errorf("filter by another operation = %+v, want empty", other)
	}

	// Linking the same bank line to the same operation twice is a duplicate,
	// not a split.
	if slug := investmentProblem(t, srv, http.MethodPost, "/api/investment-reconciliations", map[string]any{
		"operation_id": deposit, "financial_transaction_id": bankLine, "amount": "1.00",
	}, http.StatusConflict); slug != "investment-reconciliation-duplicate" {
		t.Errorf("duplicate slug = %q", slug)
	}

	// A conciliated operation cannot be deleted before the link is undone.
	if slug := investmentProblem(t, srv, http.MethodDelete, "/api/investment-operations/"+deposit, nil, http.StatusConflict); slug != "investment-operation-has-reconciliations" {
		t.Errorf("delete linked operation slug = %q", slug)
	}

	// Allocating more of a bank line than it is worth is refused, even though
	// the operation itself is big enough to carry the parcel.
	spare := insertTransaction(t, conn)
	bigDeposit := investmentID(t, investmentCall(t, srv, http.MethodPost, "/api/investment-operations", map[string]any{
		"account_id": account, "position_id": nil, "kind": "deposit",
		"occurred_on": "2026-01-15", "amount": "100.00",
	}, http.StatusCreated))
	if slug := investmentProblem(t, srv, http.MethodPost, "/api/investment-reconciliations", map[string]any{
		"operation_id": bigDeposit, "financial_transaction_id": spare, "amount": "50.00",
	}, http.StatusConflict); slug != "investment-reconciliation-conflict" {
		t.Errorf("over-allocation slug = %q", slug)
	}
	var persisted int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM investment_reconciliations WHERE operation_id = ?`, bigDeposit).Scan(&persisted); err != nil {
		t.Fatalf("count links: %v", err)
	}
	if persisted != 0 {
		t.Errorf("refused link persisted %d row(s)", persisted)
	}

	// A buy never crosses the bank boundary, so no bank line can explain it.
	position := investmentID(t, investmentCall(t, srv, http.MethodPost, "/api/investment-positions", map[string]any{
		"account_id": account, "name": "CDB", "asset_type": "Renda fixa",
	}, http.StatusCreated))
	buy := investmentID(t, investmentCall(t, srv, http.MethodPost, "/api/investment-operations", map[string]any{
		"account_id": account, "position_id": position, "kind": "buy",
		"occurred_on": "2026-01-20", "amount": "40.00", "quantity": "1",
	}, http.StatusCreated))
	if slug := investmentProblem(t, srv, http.MethodPost, "/api/investment-reconciliations", map[string]any{
		"operation_id": buy, "financial_transaction_id": spare, "amount": "10.00",
	}, http.StatusConflict); slug != "investment-invalid-reconciliation-link" {
		t.Errorf("non-reconcilable kind slug = %q", slug)
	}

	investmentCall(t, srv, http.MethodDelete, "/api/investment-reconciliations/"+investmentID(t, created), nil, http.StatusNoContent)
	if slug := investmentProblem(t, srv, http.MethodDelete, "/api/investment-reconciliations/"+investmentID(t, created), nil, http.StatusNotFound); slug != "investment-reconciliation-not-found" {
		t.Errorf("second delete slug = %q", slug)
	}
	// With the link gone the operation is ordinary again.
	investmentCall(t, srv, http.MethodDelete, "/api/investment-operations/"+deposit, nil, http.StatusNoContent)
}

func TestIntegratedInvestmentRecordsAreReadOnlyOverHTTP(t *testing.T) {
	srv, conn := newTestServer(t)
	holding := insertInvestment(t, conn)
	grouping := integratedAccountID(t, conn, holding)

	accounts := investmentItems(t, srv, "/api/investment-accounts")
	if len(accounts) != 1 {
		t.Fatalf("accounts = %+v, want the connection's grouping", accounts)
	}
	integrated := accounts[0]
	assertInvestmentKeys(t, "integrated account", integrated, investmentAccountResponseKeys)
	if integrated["id"] != grouping {
		t.Fatalf("id = %v, want the deterministic %q", integrated["id"], grouping)
	}
	if integrated["kind"] != "integrated" || integrated["currency_code"] != "BRL" {
		t.Errorf("integrated account = %+v", integrated)
	}
	if integrated["source_display_name"] != "Banco Exemplo" {
		t.Errorf("source_display_name = %v, want the connection's name", integrated["source_display_name"])
	}
	assertMoney(t, "cash_balance", integrated["cash_balance"], "0")

	escaped := strings.ReplaceAll(grouping, ":", "%3A")

	// Renaming the connection's grouping is refused; linking its custody cash
	// to a bank account is a local choice and goes through.
	if slug := investmentProblem(t, srv, http.MethodPut, "/api/investment-accounts/"+escaped,
		map[string]any{"name": "Minha corretora"}, http.StatusConflict); slug != "investment-integrated-read-only" {
		t.Errorf("rename slug = %q", slug)
	}
	bank := insertAccount(t, conn, "BANK", "Conta corrente", nil, nil)
	linked := investmentCall(t, srv, http.MethodPut, "/api/investment-accounts/"+escaped,
		map[string]any{"financial_account_id": bank.accountID}, http.StatusOK)
	if linked["financial_account_id"] != bank.accountID || linked["name"] != integrated["name"] {
		t.Errorf("link change = %+v", linked)
	}
	// Switching the grouping off is refused as a whole: the unlink sent in
	// the same request must not go through either.
	if slug := investmentProblem(t, srv, http.MethodPut, "/api/investment-accounts/"+escaped,
		map[string]any{"active": false, "financial_account_id": nil}, http.StatusConflict); slug != "investment-integrated-read-only" {
		t.Errorf("deactivate slug = %q", slug)
	}
	if accounts := investmentItems(t, srv, "/api/investment-accounts"); len(accounts) != 1 ||
		accounts[0]["active"] != true || accounts[0]["financial_account_id"] != bank.accountID {
		t.Errorf("refused deactivation changed the grouping: %+v", accounts)
	}
	if slug := investmentProblem(t, srv, http.MethodDelete, "/api/investment-accounts/"+escaped, nil, http.StatusConflict); slug != "investment-integrated-read-only" {
		t.Errorf("delete slug = %q", slug)
	}

	// The provider holding shows up as a read-only position whose id is the
	// provider's own, and it accepts a goal and nothing else.
	positions := investmentItems(t, srv, "/api/investment-positions")
	if len(positions) != 1 || positions[0]["id"] != holding {
		t.Fatalf("positions = %+v, want the synced holding", positions)
	}
	synced := positions[0]
	assertInvestmentKeys(t, "synced position", synced, investmentPositionResponseKeys)
	if synced["source"] != "synced" || synced["valuation_basis"] != "provider_balance" {
		t.Errorf("synced position = %+v", synced)
	}
	if synced["linked_investment_id"] != holding {
		t.Errorf("linked_investment_id = %v, want the provider id", synced["linked_investment_id"])
	}
	assertMoney(t, "current_value", synced["current_value"], "1000.50")

	goal := investmentID(t, investmentCall(t, srv, http.MethodPost, "/api/investment-portfolios", map[string]any{
		"name": "Longo prazo", "target_amount": "2000.00", "target_date": nil, "notes": nil,
	}, http.StatusCreated))
	assigned := investmentCall(t, srv, http.MethodPut, "/api/investment-positions/"+holding,
		map[string]any{"portfolio_id": goal}, http.StatusOK)
	if assigned["portfolio_id"] != goal {
		t.Errorf("goal assignment = %+v", assigned)
	}
	if slug := investmentProblem(t, srv, http.MethodPut, "/api/investment-positions/"+holding,
		map[string]any{"name": "Fundo renomeado", "ticker": nil, "asset_type": "Fundo", "portfolio_id": goal, "notes": nil},
		http.StatusConflict); slug != "investment-integrated-read-only" {
		t.Errorf("synced rename slug = %q", slug)
	}

	// No local operation can apply to a provider holding.
	manual := investmentID(t, createCustodyAccount(t, srv, "Corretora"))
	if slug := investmentProblem(t, srv, http.MethodPost, "/api/investment-operations", map[string]any{
		"account_id": manual, "position_id": holding, "kind": "buy",
		"occurred_on": "2026-01-10", "amount": "10.00", "quantity": "1",
	}, http.StatusConflict); slug != "investment-not-manual" {
		t.Errorf("operation on synced holding slug = %q", slug)
	}
}

func TestManualHoldingInIntegratedAccountOverHTTP(t *testing.T) {
	srv, conn := newTestServer(t)
	holding := insertInvestment(t, conn) // provider holding worth 1000.50
	grouping := integratedAccountID(t, conn, holding)
	investmentItems(t, srv, "/api/investment-accounts")

	created := investmentCall(t, srv, http.MethodPost, "/api/investment-positions", map[string]any{
		"account_id": grouping, "name": "Bitcoin", "ticker": "BTC", "asset_type": "Criptoativo",
		"initial_quantity": "0.5", "initial_unit_cost": "1000.00", "occurred_on": "2026-01-05",
	}, http.StatusCreated)
	id := investmentID(t, created)
	if created["source"] != "manual" || created["account_id"] != grouping {
		t.Errorf("created = %+v", created)
	}
	assertMoney(t, "current_value", created["current_value"], "500.00")

	investmentCall(t, srv, http.MethodPost, "/api/investment-operations", map[string]any{
		"account_id": grouping, "position_id": id, "kind": "buy",
		"occurred_on": "2026-01-10", "amount": "600.00", "quantity": "0.5",
	}, http.StatusCreated)
	updated := investmentCall(t, srv, http.MethodPut, "/api/investment-positions/"+id, map[string]any{
		"name": "Bitcoin", "ticker": "BTC", "asset_type": "Criptoativo", "portfolio_id": nil, "notes": "carteira fria",
	}, http.StatusOK)
	if updated["notes"] != "carteira fria" {
		t.Errorf("update = %+v", updated)
	}

	summary := investmentCall(t, srv, http.MethodGet, "/api/investment-summary", nil, http.StatusOK)
	assertMoney(t, "manual_value", summary["manual_value"], "1100.00")
	assertMoney(t, "synced_value", summary["synced_value"], "1000.50")
	assertMoney(t, "total_value", summary["total_value"], "2100.50")
	accounts, _ := summary["accounts"].([]any)
	if len(accounts) != 1 {
		t.Fatalf("accounts = %+v, want only the grouping", summary["accounts"])
	}
	row, _ := accounts[0].(map[string]any)
	assertMoney(t, "grouping current_value", row["current_value"], "2100.50")
	assertMoney(t, "grouping cash_balance", row["cash_balance"], "0")

	// The provider holding next to it stays read-only.
	if slug := investmentProblem(t, srv, http.MethodPut, "/api/investment-positions/"+holding,
		map[string]any{"name": "Fundo renomeado", "ticker": nil, "asset_type": "Fundo", "portfolio_id": nil, "notes": nil},
		http.StatusConflict); slug != "investment-integrated-read-only" {
		t.Errorf("synced rename slug = %q", slug)
	}
	if slug := investmentProblem(t, srv, http.MethodDelete, "/api/investment-positions/"+holding, nil,
		http.StatusConflict); slug != "investment-integrated-read-only" {
		t.Errorf("synced delete slug = %q", slug)
	}

	operations := investmentItems(t, srv, "/api/investment-operations?position_id="+id)
	for i := len(operations) - 1; i >= 0; i-- {
		investmentCall(t, srv, http.MethodDelete, "/api/investment-operations/"+investmentID(t, operations[i]), nil, http.StatusNoContent)
	}
	investmentCall(t, srv, http.MethodDelete, "/api/investment-positions/"+id, nil, http.StatusNoContent)
	if positions := investmentItems(t, srv, "/api/investment-positions"); len(positions) != 1 || positions[0]["id"] != holding {
		t.Errorf("positions after delete = %+v", positions)
	}
}

func TestInvestmentSummaryOverHTTP(t *testing.T) {
	srv, conn := newTestServer(t)
	holding := insertInvestment(t, conn) // provider holding worth 1000.50
	account := investmentID(t, createCustodyAccount(t, srv, "Corretora"))

	summary := investmentCall(t, srv, http.MethodGet, "/api/investment-summary", nil, http.StatusOK)
	assertInvestmentKeys(t, "summary", summary, investmentSummaryResponseKeys)
	if summary["currency_code"] != "BRL" {
		t.Errorf("currency_code = %v", summary["currency_code"])
	}
	assertMoney(t, "synced_value", summary["synced_value"], "1000.50")
	assertMoney(t, "manual_value", summary["manual_value"], "0")
	assertMoney(t, "total_value", summary["total_value"], "1000.50")

	investmentCall(t, srv, http.MethodPost, "/api/investment-operations", map[string]any{
		"account_id": account, "position_id": nil, "kind": "deposit",
		"occurred_on": "2026-01-10", "amount": "1000.00",
	}, http.StatusCreated)
	investmentCall(t, srv, http.MethodPost, "/api/investment-positions", map[string]any{
		"account_id": account, "name": "ITSA4", "asset_type": "Ação",
		"initial_quantity": "10", "initial_unit_cost": "50.00", "occurred_on": "2026-01-11",
	}, http.StatusCreated)

	summary = investmentCall(t, srv, http.MethodGet, "/api/investment-summary", nil, http.StatusOK)
	assertMoney(t, "manual_value", summary["manual_value"], "500.00")
	assertMoney(t, "synced_value", summary["synced_value"], "1000.50")
	// Manual cash that no bank account represents is part of the patrimony;
	// the provider grouping adds no cash of its own.
	assertMoney(t, "cash_balance", summary["cash_balance"], "1000.00")
	assertMoney(t, "total_value", summary["total_value"], "2500.50")
	assertMoney(t, "unrealized_gain", summary["unrealized_gain"], "0")

	accounts, ok := summary["accounts"].([]any)
	if !ok || len(accounts) != 2 {
		t.Fatalf("accounts = %+v, want the custody account and the grouping", summary["accounts"])
	}
	byID := map[string]map[string]any{}
	for _, entry := range accounts {
		row, _ := entry.(map[string]any)
		assertInvestmentKeys(t, "summary account", row, investmentSummaryAccountKeys)
		id, _ := row["account_id"].(string)
		byID[id] = row
	}
	manual := byID[account]
	if manual == nil {
		t.Fatalf("custody account missing from %+v", byID)
	}
	assertMoney(t, "custody current_value", manual["current_value"], "500.00")
	assertMoney(t, "custody cash_balance", manual["cash_balance"], "1000.00")
	grouping := byID[integratedAccountID(t, conn, holding)]
	if grouping == nil {
		t.Fatalf("integrated grouping missing from %+v", byID)
	}
	assertMoney(t, "grouping current_value", grouping["current_value"], "1000.50")
	assertMoney(t, "grouping cash_balance", grouping["cash_balance"], "0")

	portfolios, ok := summary["portfolios"].([]any)
	if !ok || len(portfolios) != 1 {
		t.Fatalf("portfolios = %+v, want the single unassigned bucket", summary["portfolios"])
	}
	unassigned, _ := portfolios[0].(map[string]any)
	assertInvestmentKeys(t, "summary portfolio", unassigned, investmentSummaryPortfolioKeys)
	if unassigned["portfolio_id"] != nil {
		t.Errorf("portfolio_id = %v, want nil for holdings with no goal", unassigned["portfolio_id"])
	}
	assertMoney(t, "unassigned current_value", unassigned["current_value"], "1500.50")
}

func TestCompoundInvestmentPurchaseOverHTTP(t *testing.T) {
	srv, conn := newTestServer(t)
	account := investmentID(t, createCustodyAccount(t, srv, "Corretora"))
	position := investmentID(t, investmentCall(t, srv, http.MethodPost, "/api/investment-positions", map[string]any{"account_id": account, "name": "Ações", "asset_type": "Ações"}, http.StatusCreated))
	bank := insertTransaction(t, conn)
	body := map[string]any{"operations": []map[string]any{
		{"account_id": account, "kind": "deposit", "occurred_on": "2026-01-10", "amount": "42"},
		{"account_id": account, "position_id": position, "kind": "buy", "occurred_on": "2026-01-10", "quantity": "3", "unit_price": "14"},
	}, "reconciliation": map[string]any{"financial_transaction_id": bank, "amount": "42"}}
	investmentCall(t, srv, http.MethodPost, "/api/investment-operations/batch", body, http.StatusCreated)
	holding := investmentCall(t, srv, http.MethodGet, "/api/investment-positions/"+position, nil, http.StatusOK)
	assertMoney(t, "value", holding["current_value"], "42")
	assertMoney(t, "cost", holding["average_cost"], "14")
	before := countInvestmentOperations(t, conn)
	investmentCall(t, srv, http.MethodPost, "/api/investment-operations/batch", body, http.StatusConflict)
	if countInvestmentOperations(t, conn) != before {
		t.Fatal("failed link left an orphan operation")
	}
}

func TestProviderMovementReconciliationOverHTTP(t *testing.T) {
	srv, conn := newTestServer(t)
	bankLine := insertTransaction(t, conn)
	holding := insertInvestment(t, conn)
	insertInvestmentTransaction(t, conn, holding, "BUY", "42.00")
	var importedMovement string
	if err := conn.QueryRow(`SELECT id FROM financial_investment_transactions WHERE investment_id = ?`, holding).
		Scan(&importedMovement); err != nil {
		t.Fatalf("lookup imported movement: %v", err)
	}

	created := investmentCall(t, srv, http.MethodPost, "/api/investment-reconciliations", map[string]any{
		"financial_transaction_id": bankLine, "financial_investment_transaction_id": importedMovement, "amount": "42.00",
	}, http.StatusCreated)
	pivotID, _ := created["operation_id"].(string)
	if pivotID == "" {
		t.Fatalf("derived pivot missing from response: %+v", created)
	}

	operations := investmentItems(t, srv, "/api/investment-operations?source=synced")
	if len(operations) != 1 || operations[0]["id"] != pivotID {
		t.Fatalf("synced operations = %+v", operations)
	}
	pivot := operations[0]
	if pivot["kind"] != "deposit" || pivot["is_editable"] != false || pivot["position_id"] != nil {
		t.Errorf("derived pivot = %+v", pivot)
	}
	assertMoney(t, "pivot amount", pivot["amount"], "42.00")

	// Provider evidence is never edited or deleted by hand.
	if slug := investmentProblem(t, srv, http.MethodDelete, "/api/investment-operations/"+pivotID, nil, http.StatusConflict); slug == "" {
		t.Errorf("deleting the derived pivot should be refused")
	}

	linkID, _ := created["id"].(string)
	investmentCall(t, srv, http.MethodDelete, "/api/investment-reconciliations/"+linkID, nil, http.StatusNoContent)
	if remaining := investmentItems(t, srv, "/api/investment-operations?source=synced"); len(remaining) != 0 {
		t.Errorf("orphan pivot survived unlink: %+v", remaining)
	}

	// Without a movement there is nothing to derive a pivot from.
	if slug := investmentProblem(t, srv, http.MethodPost, "/api/investment-reconciliations", map[string]any{
		"financial_transaction_id": bankLine, "amount": "1.00",
	}, http.StatusConflict); slug != "investment-invalid-reconciliation-link" {
		t.Errorf("no pivot slug = %q", slug)
	}
}

// TestInvestmentValuesCarryNoDecimalTailOverHTTP reads the JSON text itself:
// money.CanonicalDecimal preserves scale, so a value derived from a unit cost
// of 100 ÷ 3 would reach the API as "99.9999999999999999" and assertMoney,
// which compares numerically, would still pass.
func TestInvestmentValuesCarryNoDecimalTailOverHTTP(t *testing.T) {
	srv, _ := newTestServer(t)
	account := investmentID(t, createCustodyAccount(t, srv, "Corretora"))
	exact := func(label string, got any, want string) {
		t.Helper()
		if got != want {
			t.Errorf("%s = %#v, want exactly %q", label, got, want)
		}
	}
	thirds := investmentCall(t, srv, http.MethodPost, "/api/investment-positions", map[string]any{
		"account_id": account, "name": "Fundo", "asset_type": "Fundo",
		"initial_quantity": "3", "initial_value": "100", "occurred_on": "2026-01-05",
	}, http.StatusCreated)
	exact("current_value at cost", thirds["current_value"], "100")
	investmentCall(t, srv, http.MethodPost, "/api/investment-operations", map[string]any{
		"account_id": account, "position_id": investmentID(t, thirds), "kind": "valuation",
		"occurred_on": "2026-01-10", "amount": "1000",
	}, http.StatusCreated)
	valued := investmentCall(t, srv, http.MethodGet, "/api/investment-positions/"+investmentID(t, thirds), nil, http.StatusOK)
	exact("current_value valued", valued["current_value"], "1000")

	hundreds := investmentCall(t, srv, http.MethodPost, "/api/investment-positions", map[string]any{
		"account_id": account, "name": "Ações", "asset_type": "Ação",
		"initial_quantity": "3", "initial_unit_cost": "100", "occurred_on": "2026-01-05",
	}, http.StatusCreated)
	exact("current_value 3 × 100", hundreds["current_value"], "300")

	summary := investmentCall(t, srv, http.MethodGet, "/api/investment-summary", nil, http.StatusOK)
	exact("manual_value", summary["manual_value"], "1300")
	exact("total_value", summary["total_value"], "1300")
	exact("unrealized_gain", summary["unrealized_gain"], "900")
}

// TestInvestmentIdentityConflictsOverHTTP pins the two UNIQUE columns the
// domain checks ahead of the database to their 409 slugs: a bank account
// already backing a custody, and an asset identity another position holds.
func TestInvestmentIdentityConflictsOverHTTP(t *testing.T) {
	srv, conn := newTestServer(t)
	first := investmentID(t, createCustodyAccount(t, srv, "Corretora"))
	second := investmentID(t, createCustodyAccount(t, srv, "Banco"))
	bank := insertAccount(t, conn, "BANK", "Conta corrente", nil, nil)
	investmentCall(t, srv, http.MethodPut, "/api/investment-accounts/"+first,
		map[string]any{"name": "Corretora", "financial_account_id": bank.accountID}, http.StatusOK)
	if slug := investmentProblem(t, srv, http.MethodPut, "/api/investment-accounts/"+second,
		map[string]any{"name": "Banco", "financial_account_id": bank.accountID}, http.StatusConflict); slug != "investment-financial-account-linked" {
		t.Errorf("second custody on the same bank account slug = %q", slug)
	}
	if slug := investmentProblem(t, srv, http.MethodPost, "/api/investment-accounts", map[string]any{
		"name": "Terceira", "currency_code": "BRL", "financial_account_id": bank.accountID, "kind": "manual",
	}, http.StatusConflict); slug != "investment-financial-account-linked" {
		t.Errorf("create custody on the same bank account slug = %q", slug)
	}

	vale := investmentID(t, investmentCall(t, srv, http.MethodPost, "/api/investment-positions", map[string]any{
		"account_id": first, "name": "Vale", "ticker": "VALE3", "asset_type": "Ação",
	}, http.StatusCreated))
	investmentCall(t, srv, http.MethodPost, "/api/investment-positions", map[string]any{
		"account_id": first, "name": "Petrobras", "ticker": "PETR4", "asset_type": "Ação",
	}, http.StatusCreated)
	if slug := investmentProblem(t, srv, http.MethodPut, "/api/investment-positions/"+vale, map[string]any{
		"name": "Vale", "ticker": "PETR4", "asset_type": "Ação", "portfolio_id": nil, "notes": nil,
	}, http.StatusConflict); slug != "investment-asset-already-exists" {
		t.Errorf("rename onto another asset slug = %q", slug)
	}
}
