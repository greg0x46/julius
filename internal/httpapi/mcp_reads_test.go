package httpapi_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/auth"
	"github.com/greg0x46/julius/internal/db"
	juliusmcp "github.com/greg0x46/julius/internal/mcp"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type mcpReadTransport struct{ token string }

func (b mcpReadTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

func financialMCPSession(t *testing.T, conn *sql.DB) *sdk.ClientSession {
	t.Helper()
	store := auth.NewStore(conn)
	token := "synthetic-read-token"
	sum := sha256.Sum256([]byte(token))
	if _, err := conn.Exec(`INSERT INTO oauth_clients(id,name,redirect_uris) VALUES('reads-client','Synthetic','[]')`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO mcp_authorizations(id,client_id,scope,approved_scope,revision,created_at,expires_at) SELECT 'reads-connection','reads-client','mcp:access','mcp:access',revision,?,? FROM auth_owner WHERE id=1`,
		db.FormatTime(time.Now()), db.FormatTime(time.Now().Add(time.Hour))); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO oauth_grants(token_hash,kind,client_id,redirect_uri,resource,scope,challenge,state,csrf_hash,revision,expires_at,authorization_id) SELECT ?,'access','reads-client','https://client.example/cb',?,'mcp:access','','','',revision,?,'reads-connection' FROM auth_owner WHERE id=1`,
		hex.EncodeToString(sum[:]), testOrigin+"/mcp", db.FormatTime(time.Now().Add(time.Hour))); err != nil {
		t.Fatal(err)
	}
	handler, err := juliusmcp.NewHandler(true, auth.Config{PublicURL: testOrigin}, store)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := sdk.NewClient(&sdk.Implementation{Name: "financial-contract-client", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &sdk.StreamableClientTransport{Endpoint: server.URL + "/mcp", HTTPClient: &http.Client{Transport: mcpReadTransport{token}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

type financialMCPResult struct {
	Status     int `json:"status"`
	Data       any `json:"data"`
	Pagination *struct {
		Page       int `json:"page"`
		PageSize   int `json:"page_size"`
		TotalItems int `json:"total_items"`
		TotalPages int `json:"total_pages"`
	} `json:"pagination"`
}

func callFinancialMCP(t *testing.T, session *sdk.ClientSession, name string, args any) (*sdk.CallToolResult, financialMCPResult) {
	t.Helper()
	result, err := session.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	var body financialMCPResult
	if result.StructuredContent != nil {
		data, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &body); err != nil {
			t.Fatal(err)
		}
	}
	return result, body
}

// Request timestamps may differ between calls; all financial fields must agree.
func stableFinancialPayload(value any) {
	switch v := value.(type) {
	case map[string]any:
		delete(v, "confirmed_at")
		if fx, ok := v["fx"].(map[string]any); ok {
			delete(fx, "fetched_at")
		}
		for _, child := range v {
			stableFinancialPayload(child)
		}
	case []any:
		for _, child := range v {
			stableFinancialPayload(child)
		}
	}
}

func compareFinancialMCP(t *testing.T, session *sdk.ClientSession, name string, args any, method, endpoint string, body any) financialMCPResult {
	t.Helper()
	result, got := callFinancialMCP(t, session, name, args)
	response := doJSON(t, method, endpoint, body)
	var want any
	decodeJSON(t, response, &want)
	stableFinancialPayload(want)
	stableFinancialPayload(got.Data)
	if result.IsError != (response.StatusCode >= 400) || got.Status != response.StatusCode || !reflect.DeepEqual(got.Data, want) {
		t.Fatalf("%s differs from REST: MCP error=%v payload=%+v; REST status=%d payload=%+v; content=%+v", name, result.IsError, got, response.StatusCode, want, result.Content)
	}
	return got
}

func TestMCPFinancialReadsMatchHTTPFixtures(t *testing.T) {
	rest, conn := newTestServer(t)
	pending := insertTransaction(t, conn)
	if _, err := conn.Exec(`UPDATE financial_transactions SET provider_status = 'PENDING', amount = '-9007199254740993.123456789012345678', amount_in_account_currency = '-9007199254740993.123456789012345678' WHERE id = ?`, pending); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`UPDATE financial_accounts SET account_type = 'CREDIT', balance = '9007199254740993.123456789012345678', credit_limit = '10000000000000000' WHERE id = (SELECT account_id FROM financial_transactions WHERE id = ?)`, pending); err != nil {
		t.Fatal(err)
	}
	ignored := insertTransaction(t, conn)
	response := doJSON(t, "PUT", rest.URL+"/api/transactions/"+ignored+"/inclusion", map[string]any{"state": "ignored"})
	if response.StatusCode != 200 {
		t.Fatal(response.StatusCode)
	}
	response.Body.Close()
	transfer := insertTransaction(t, conn)
	var transferCategory string
	if err := conn.QueryRow(`SELECT id FROM categories WHERE kind = 'transfer' LIMIT 1`).Scan(&transferCategory); err != nil {
		t.Fatal(err)
	}
	response = doJSON(t, "PUT", rest.URL+"/api/transactions/"+transfer+"/category", map[string]any{"category_id": transferCategory})
	if response.StatusCode != 200 {
		t.Fatal(response.StatusCode)
	}
	response.Body.Close()
	card := insertAccount(t, conn, "CREDIT", "Synthetic card", nil, strPtr("5000.00"))
	insertBill(t, conn, card, time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC), "1234.567890123456789012")
	session := financialMCPSession(t, conn)
	tools, err := session.ListTools(context.Background(), &sdk.ListToolsParams{})
	if err != nil || len(tools.Tools) != 61 {
		t.Fatalf("discovery: %+v %v", tools, err)
	}
	financial := map[string]bool{"list_accounts": true, "list_card_bills": true, "search_transactions": true, "list_categories": true, "spending_by_category": true}
	found := 0
	for _, tool := range tools.Tools {
		if !financial[tool.Name] {
			continue
		}
		found++
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Fatalf("tool not read-only: %+v", tool)
		}
	}
	if found != len(financial) {
		t.Fatalf("financial read tools discovered: %d", found)
	}
	compareFinancialMCP(t, session, "list_accounts", map[string]any{}, "GET", rest.URL+"/api/accounts", nil)
	compareFinancialMCP(t, session, "list_categories", map[string]any{}, "GET", rest.URL+"/api/categories", nil)
	compareFinancialMCP(t, session, "list_card_bills", map[string]any{"account_id": card.accountID}, "GET", rest.URL+"/api/accounts/"+card.accountID+"/bills", nil)
	args := map[string]any{"timezone": "UTC", "group_by": "month", "page": 1, "page_size": 50}
	query := compareFinancialMCP(t, session, "search_transactions", args, "POST", rest.URL+"/api/transactions/query", args)
	items := query.Data.(map[string]any)["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("omitted transactions: %+v", items)
	}
	reasons := map[string]any{}
	for _, entry := range items {
		item := entry.(map[string]any)
		eligibility := item["totals_eligibility"].(map[string]any)
		reasons[item["id"].(string)] = eligibility["reason"]
		if item["id"] == pending && (item["amount"] != "-9007199254740993.123456789012345678" || eligibility["included"] != true) {
			t.Fatalf("pending precision or eligibility: %+v", item)
		}
	}
	if reasons[ignored] != "ignored" || reasons[transfer] != "transfer_category" {
		t.Fatalf("reasons: %+v", reasons)
	}
	filters := map[string]any{"timezone": "UTC", "group_by": "none", "filters": map[string]any{"description": "Mercado", "provider_statuses": []string{"PENDING"}, "classification": "outflow", "credit_card": true, "date_from": "2000-01-01", "date_to": "2099-12-31"}}
	compareFinancialMCP(t, session, "search_transactions", filters, "POST", rest.URL+"/api/transactions/query", filters)
	compareFinancialMCP(t, session, "spending_by_category", map[string]any{"timezone": "UTC", "period": "all", "classification": "outflow"}, "GET", rest.URL+"/api/transactions/category-breakdown?timezone=UTC&period=all&classification=outflow", nil)
	pageResult, page := callFinancialMCP(t, session, "list_accounts", map[string]any{"page": 2, "page_size": 1})
	if pageResult.IsError || page.Pagination == nil || page.Pagination.TotalItems != 4 || len(page.Data.([]any)) != 1 || page.Pagination.Page != 2 {
		t.Fatalf("page: %+v %+v", pageResult, page)
	}
}

func TestMCPReportingRetainsMissingFXAndNativeMoney(t *testing.T) {
	unavailableReportingFXProvider(t)
	rest, conn := newTestServer(t)
	id := insertTransaction(t, conn)
	if _, err := conn.Exec(`UPDATE financial_transactions SET amount = '-10', amount_in_account_currency = NULL, currency_code = 'USD', occurred_at = '2099-03-15T12:00:00.000000000Z' WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	session := financialMCPSession(t, conn)
	report := compareFinancialMCP(t, session, "spending_by_category", map[string]any{"timezone": "UTC", "month": "2099-03", "classification": "outflow"}, "GET", rest.URL+"/api/transactions/category-breakdown?timezone=UTC&month=2099-03&classification=outflow", nil)
	data := report.Data.(map[string]any)
	if data["total"] != nil || data["reporting"].(map[string]any)["missing"] != float64(1) || data["totals"].([]any)[0].(map[string]any)["currency_code"] != "USD" {
		t.Fatalf("missing FX guessed or dropped: %+v", data)
	}
	local := insertTransaction(t, conn)
	if _, err := conn.Exec(`UPDATE financial_transactions SET occurred_at = '2099-03-15T12:00:00.000000000Z' WHERE id = ?`, local); err != nil {
		t.Fatal(err)
	}
	partial := compareFinancialMCP(t, session, "spending_by_category", map[string]any{"timezone": "UTC", "date_from": "2099-03-01", "date_to": "2099-03-31", "classification": "outflow"}, "GET", rest.URL+"/api/transactions/category-breakdown?timezone=UTC&date_from=2099-03-01&date_to=2099-03-31&classification=outflow", nil)
	partialData := partial.Data.(map[string]any)
	if partialData["total"] != nil || partialData["reporting"].(map[string]any)["status"] != "incomplete" || len(partialData["totals"].([]any)) != 2 {
		t.Fatalf("partial report: %+v", partialData)
	}
	args := map[string]any{"timezone": "UTC", "group_by": "none"}
	compareFinancialMCP(t, session, "search_transactions", args, "POST", rest.URL+"/api/transactions/query", args)
}

func TestMCPEmptyAndInvalidReads(t *testing.T) {
	rest, conn := newTestServer(t)
	session := financialMCPSession(t, conn)
	compareFinancialMCP(t, session, "list_accounts", map[string]any{}, "GET", rest.URL+"/api/accounts", nil)
	card := insertAccount(t, conn, "CREDIT", "Empty card", nil, nil)
	compareFinancialMCP(t, session, "list_card_bills", map[string]any{"account_id": card.accountID}, "GET", rest.URL+"/api/accounts/"+card.accountID+"/bills", nil)
	if _, err := conn.Exec(`DELETE FROM categories`); err != nil {
		t.Fatal(err)
	}
	compareFinancialMCP(t, session, "list_categories", map[string]any{}, "GET", rest.URL+"/api/categories", nil)
	_, beyond := callFinancialMCP(t, session, "list_accounts", map[string]any{"page": 1000000})
	if beyond.Pagination == nil || beyond.Pagination.TotalItems != 1 || len(beyond.Data.([]any)) != 0 {
		t.Fatalf("out-of-range page: %+v", beyond)
	}

	args := map[string]any{"timezone": "UTC", "group_by": "none"}
	compareFinancialMCP(t, session, "search_transactions", args, "POST", rest.URL+"/api/transactions/query", args)
	compareFinancialMCP(t, session, "list_card_bills", map[string]any{"account_id": "missing"}, "GET", rest.URL+"/api/accounts/missing/bills", nil)
	for _, tc := range []struct {
		name string
		args any
	}{
		{"list_accounts", map[string]any{"page_size": 201}},
		{"list_accounts", map[string]any{"page": -1}},
		{"list_categories", map[string]any{"page": 1000001}},
		{"list_card_bills", map[string]any{"account_id": ""}},
		{"list_categories", map[string]any{"credential": true}},
		{"search_transactions", map[string]any{"timezone": "invalid"}},
		{"search_transactions", map[string]any{"timezone": "UTC", "group_by": "invalid"}},
		{"search_transactions", map[string]any{"timezone": "UTC", "group_by": "none", "filters": map[string]any{"date_from": "2026-01-01"}}},
		{"search_transactions", map[string]any{"timezone": "UTC", "group_by": "none", "filters": map[string]any{"amount_min": "1.00", "amount_max": "0.99"}}},
		{"search_transactions", map[string]any{"timezone": "UTC", "group_by": "none", "filters": map[string]any{"classification": "invalid"}}},
		{"spending_by_category", map[string]any{"timezone": "UTC", "month": "2026-01", "period": "all", "classification": "outflow"}},
		{"spending_by_category", map[string]any{"timezone": "UTC", "date_from": "2026-02-30", "date_to": "2026-03-01", "classification": "outflow"}},
	} {
		result, _ := callFinancialMCP(t, session, tc.name, tc.args)
		if !result.IsError {
			t.Fatalf("accepted %s %+v: %+v", tc.name, tc.args, result)
		}
	}
}
