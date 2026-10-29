package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/investments"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/shopspring/decimal"
)

func TestMCPInvestmentWriteLifecycle(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		f := newWriteFixture(t, conn)
		ctx := context.Background()
		approvals := 0
		// Restart the transport between groups to keep this lifecycle below
		// the production limit of five owner logins per minute.
		reconnect := func() {
			handler, err := NewHandler(true, testConfig, f.store)
			if err != nil {
				t.Fatal(err)
			}
			srv := httptest.NewServer(handler)
			t.Cleanup(srv.Close)
			client := sdk.NewClient(&sdk.Implementation{Name: "synthetic-investment-writer", Version: "1"}, nil)
			session, err := client.Connect(ctx, &sdk.StreamableClientTransport{Endpoint: srv.URL + "/mcp", HTTPClient: &http.Client{Transport: bearerTransport{f.token}}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { session.Close() })
			f.handler = handler
			f.session = session
		}
		apply := func(tool string, id string, data map[string]any) string {
			if approvals == 4 {
				reconnect()
				approvals = 0
			}
			approvals++
			args := map[string]any{"data": data}
			if id != "" {
				args["id"] = id
			}
			return f.applyPlanning(t, tool, args)
		}
		account := apply("create_investment_account", "", map[string]any{"name": "Synthetic custody"})
		apply("update_investment_account", account, map[string]any{"name": "Synthetic investments", "active": true})
		position := apply("create_investment_position", "", map[string]any{"account_id": account, "name": "Synthetic asset", "asset_type": "Ação", "occurred_on": "2026-09-01"})
		apply("update_investment_position", position, map[string]any{"notes": "Synthetic notes"})
		deposit := apply("create_investment_operation", "", map[string]any{"account_id": account, "kind": "deposit", "occurred_on": "2026-09-01", "amount": "100.000000000000000001"})
		buyData := map[string]any{"account_id": account, "position_id": position, "kind": "buy", "occurred_on": "2026-09-02", "quantity": "3", "unit_price": "10", "fees": "0.1", "taxes": "0.2"}
		buy := apply("create_investment_operation", "", buyData)
		sellData := map[string]any{"account_id": account, "position_id": position, "kind": "sell", "occurred_on": "2026-09-03", "quantity": "1", "unit_price": "12", "fees": "0.2", "taxes": "0.3"}
		sell := apply("create_investment_operation", "", sellData)
		sellData["notes"] = "Synthetic correction"
		apply("update_investment_operation", sell, sellData)
		pos, err := investments.GetPosition(ctx, conn, position)
		if err != nil || pos.Quantity.String() != "2" || pos.TotalCost == nil || pos.TotalCost.String() != "20.2" {
			t.Fatalf("position: %+v %v", pos, err)
		}
		custody, err := investments.GetAccount(ctx, conn, account)
		if err != nil || custody.CashBalance.String() != "81.200000000000000001" {
			t.Fatalf("cash: %+v %v", custody, err)
		}
		valuation := apply("create_investment_operation", "", map[string]any{"account_id": account, "position_id": position, "kind": "valuation", "amount": "24", "occurred_on": "2026-09-04"})
		result, err := f.session.CallTool(ctx, &sdk.CallToolParams{Name: "get_investment_portfolio", Arguments: map[string]any{"to": "2026-09-30", "account_id": account}})
		if err != nil || result.IsError {
			t.Fatalf("portfolio: %+v %v", result, err)
		}
		raw, _ := json.Marshal(result.StructuredContent)
		var portfolio struct {
			Totals struct {
				Yield   string `json:"yield"`
				Costs   string `json:"costs"`
				Partial bool   `json:"partial"`
			}
		}
		if err := json.Unmarshal(raw, &portfolio); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), position) || portfolio.Totals.Partial || !decimal.RequireFromString(portfolio.Totals.Yield).Equal(decimal.RequireFromString("5.2")) || !decimal.RequireFromString(portfolio.Totals.Costs).Equal(decimal.RequireFromString("0.8")) {
			t.Fatalf("portfolio differs: %s", raw)
		}
		// Both independent fees/taxes and trade costs use the ordinary ledger.
		for _, kind := range []string{"income", "fee", "tax"} {
			id := apply("create_investment_operation", "", map[string]any{"account_id": account, "kind": kind, "occurred_on": "2026-09-04", "amount": "0.000000000000000001"})
			op, err := investments.GetOperation(ctx, conn, id)
			if err != nil || op.Amount.String() != "0.000000000000000001" {
				t.Fatalf("precision: %+v %v", op, err)
			}
			apply("delete_investment_operation", id, map[string]any{})
		}
		reconnect()
		approvals = 1
		pending := f.call(t, "delete_investment_operation", map[string]any{"id": sell, "data": map[string]any{}})
		if _, err := investments.GetOperation(ctx, conn, sell); err != nil {
			t.Fatal("preparation deleted operation", err)
		}
		u, _ := url.Parse(pending.ApprovalURL)
		page := oauthRequest(f.handler, "GET", u.RequestURI(), "", nil, "")
		for _, text := range []string{"Excluir operação de investimento", "Preço unitário", "Taxas", "Impostos", "0.3"} {
			if !strings.Contains(page.Body.String(), text) {
				t.Fatalf("missing approval disclosure %q", text)
			}
		}
		f.approve(t, pending) // helper also retries the exact approval POST
		if out := f.result(t, pending.RequestID); out.State != "applied" {
			t.Fatalf("delete: %+v", out)
		}
		apply("delete_investment_operation", valuation, map[string]any{})
		apply("delete_investment_operation", buy, map[string]any{})
		apply("delete_investment_operation", deposit, map[string]any{})
		apply("delete_investment_position", position, map[string]any{})
		apply("delete_investment_account", account, map[string]any{})
	})
}

func TestMCPInvestmentWriteAuthorizationAndContracts(t *testing.T) {
	conn := dbtest.OpenSQLite(t)
	f := newWriteFixture(t, conn)
	tools, err := f.session.ListTools(context.Background(), &sdk.ListToolsParams{})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, tool := range tools.Tools {
		if !strings.Contains(tool.Name, "_investment_") || strings.HasPrefix(tool.Name, "get_") {
			continue
		}
		count++
		if tool.Annotations == nil || tool.Annotations.ReadOnlyHint || tool.InputSchema == nil {
			t.Fatalf("write contract: %+v", tool)
		}
		data := map[string]any{}
		if !strings.HasPrefix(tool.Name, "delete_") {
			switch {
			case strings.HasSuffix(tool.Name, "_account"):
				data["name"] = "Synthetic"
			case strings.HasSuffix(tool.Name, "_position"):
				data = map[string]any{"account_id": uuid.NewString(), "name": "Synthetic", "occurred_on": "2026-09-01"}
				if strings.HasPrefix(tool.Name, "update_") {
					data = map[string]any{"name": "Synthetic"}
				}
			case strings.HasSuffix(tool.Name, "_operation"):
				data = map[string]any{"account_id": uuid.NewString(), "kind": "deposit", "amount": "1", "occurred_on": "2026-09-01"}
			}
		}
		args := map[string]any{"data": data}
		if !strings.HasPrefix(tool.Name, "create_") {
			args["id"] = uuid.NewString()
		}
		raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": tool.Name, "arguments": args}})
		if w := request(t, f.handler, "", "", string(raw)); w.Code != 401 {
			t.Fatalf("anonymous %s: %d", tool.Name, w.Code)
		}
		if w := request(t, f.handler, f.readToken, "", string(raw)); !strings.Contains(w.Body.String(), "mcp:write scope required") {
			t.Fatalf("read scope %s: %s", tool.Name, w.Body)
		}
	}
	if count != 9 {
		t.Fatalf("investment write count: %d", count)
	}
	for _, amount := range []any{1, "1e3", "", strings.Repeat("1", 65)} {
		result, err := f.session.CallTool(context.Background(), &sdk.CallToolParams{Name: "create_investment_operation", Arguments: map[string]any{"data": map[string]any{"account_id": uuid.NewString(), "kind": "deposit", "occurred_on": "2026-09-01", "amount": amount}}})
		if err == nil && !result.IsError {
			t.Fatalf("invalid decimal accepted: %v", amount)
		}
	}
	account := f.applyPlanning(t, "create_investment_account", map[string]any{"data": map[string]any{"name": "Synthetic"}})
	pending := f.call(t, "delete_investment_account", map[string]any{"id": account, "data": map[string]any{}})
	u, _ := url.Parse(pending.ApprovalURL)
	w := oauthRequest(f.handler, "GET", u.RequestURI(), "", nil, "")
	form := url.Values{"request": {pending.RequestID}, "decision": {"deny"}}
	oauthRequest(f.handler, "POST", "/oauth/write", form.Encode(), w.Result().Cookies()[0], testConfig.PublicURL)
	if out := f.result(t, pending.RequestID); out.State != "denied" {
		t.Fatalf("denial: %+v", out)
	}
	if _, err := investments.GetAccount(context.Background(), conn, account); err != nil {
		t.Fatal("denial deleted custody", err)
	}
}

func TestMCPInvestmentConsentDisclosure(t *testing.T) {
	conn := dbtest.OpenSQLite(t)
	store, _ := setup(t, conn)
	flow := newFlow(t, store)
	flow.params.Set("scope", mcpScope+" "+writeScope)
	if !strings.Contains(flow.begin(t).Body.String(), "contas, posições e operações de investimento editáveis") {
		t.Fatal("missing investment consent disclosure")
	}
}
