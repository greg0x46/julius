package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/shopspring/decimal"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/transactions"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPAccountsToolsLifecycleAndApprovalReplay(t *testing.T) {
	for _, family := range []string{"account", "transfer", "payable"} {
		t.Run(family, func(t *testing.T) {
			dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
				f := newWriteFixture(t, conn)
				counts := map[string]int{}
				switch family {
				case "account":
					data := map[string]any{"name": "Synthetic dollars", "currency_code": "USD", "opening_balance": "100.0001", "archived": false}
					pending := f.call(t, "create_manual_account", map[string]any{"data": data})
					var count int
					if err := conn.QueryRow(`SELECT COUNT(*) FROM financial_accounts`).Scan(&count); err != nil || count != 1 {
						t.Fatal("prepare wrote account")
					}
					f.approve(t, pending)
					var account struct{ ID string }
					result := f.result(t, pending.RequestID)
					if result.State != "applied" || result.Result == nil || json.Unmarshal(result.Result.Data, &account) != nil {
						t.Fatalf("account result: %+v", result)
					}
					data["name"] = "Synthetic foreign cash"
					f.applyPlanning(t, "update_manual_account", map[string]any{"id": account.ID, "data": data})
					f.applyPlanning(t, "adjust_manual_account", map[string]any{"id": account.ID, "data": map[string]any{"amount": "0.0001", "description": "Synthetic adjustment"}})
					balance, err := transactions.AccountBalance(context.Background(), conn, account.ID)
					if err != nil || balance == nil || *balance != "100.0002" {
						t.Fatalf("precision: %v %v", balance, err)
					}
					counts = map[string]int{"financial_accounts": 2, "account_balance_adjustments": 1, "financial_transactions": 0}
				case "transfer":
					account := f.applyPlanning(t, "create_manual_account", map[string]any{"data": map[string]any{"name": "Synthetic dollars", "currency_code": "USD"}})
					movement := func(id, amount string) map[string]any {
						return map[string]any{"account_id": id, "description": "Synthetic transfer", "amount": amount, "occurred_at": "2026-10-01"}
					}
					transferID := f.applyPlanning(t, "create_transfer", map[string]any{"data": map[string]any{"source": movement(f.account, "-50"), "destination": movement(account, "10"), "fee": movement(f.account, "-1")}})
					f.applyPlanning(t, "delete_transfer", map[string]any{"id": transferID, "data": map[string]any{}})
					counts = map[string]int{"financial_transactions": 3, "transfer_links": 0, "transfer_transaction_links": 0}
				case "payable":
					payableID := f.applyPlanning(t, "create_payable", map[string]any{"data": map[string]any{"kind": "debt", "name": "Synthetic obligation", "total_amount": "70.0001"}})
					f.applyPlanning(t, "update_payable", map[string]any{"id": payableID, "data": map[string]any{"name": "Synthetic updated", "total_amount": "80.0001"}})
					payment, err := transactions.CreateManual(context.Background(), conn, transactions.ManualInput{AccountID: f.account, Description: "Synthetic payment", Amount: decimal.NewFromInt(-50), OccurredAt: time.Now()})
					if err != nil {
						t.Fatal(err)
					}
					linkID := f.applyPlanning(t, "create_payable_link", map[string]any{"data": map[string]any{"payable_id": payableID, "transaction_id": payment}})
					f.applyPlanning(t, "delete_payable_link", map[string]any{"id": linkID, "data": map[string]any{"payable_id": payableID}})
					f.applyPlanning(t, "delete_payable", map[string]any{"id": payableID, "data": map[string]any{}})
					counts = map[string]int{"financial_transactions": 1, "payables": 0, "payable_transaction_links": 0}
				}
				// Every approval helper replays its POST; no financial effect may be duplicated.
				for table, want := range counts {
					var count int
					if err := conn.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != want {
						t.Fatalf("%s count: %d want %d: %v", table, count, want, err)
					}
				}
			})
		})
	}
}

func TestMCPAccountsToolsRequireWriteScopeAndOwnerConsent(t *testing.T) {
	conn := dbtest.OpenSQLite(t)
	f := newWriteFixture(t, conn)
	cases := map[string]map[string]any{
		"create_manual_account": {"data": map[string]any{"name": "Synthetic"}},
		"update_manual_account": {"id": uuid.NewString(), "data": map[string]any{"name": "Synthetic", "currency_code": "BRL", "opening_balance": "0", "archived": false}},
		"adjust_manual_account": {"id": uuid.NewString(), "data": map[string]any{"amount": "1", "description": "Synthetic"}},
		"create_transfer":       {"data": map[string]any{"source_transaction_id": uuid.NewString(), "destination_transaction_id": uuid.NewString()}},
		"delete_transfer":       {"id": uuid.NewString(), "data": map[string]any{}},
		"create_payable":        {"data": map[string]any{"kind": "debt", "name": "Synthetic", "total_amount": "1"}},
		"update_payable":        {"id": uuid.NewString(), "data": map[string]any{"name": "Synthetic", "total_amount": "1"}},
		"delete_payable":        {"id": uuid.NewString(), "data": map[string]any{}},
		"create_payable_link":   {"data": map[string]any{"payable_id": uuid.NewString(), "transaction_id": uuid.NewString()}},
		"delete_payable_link":   {"id": uuid.NewString(), "data": map[string]any{"payable_id": uuid.NewString()}},
	}
	listed, err := f.session.ListTools(context.Background(), &sdk.ListToolsParams{})
	if err != nil {
		t.Fatal(err)
	}
	for name, args := range cases {
		found := false
		for _, tool := range listed.Tools {
			if tool.Name == name {
				found = true
				if tool.Annotations == nil || tool.Annotations.ReadOnlyHint {
					t.Fatalf("write marked read-only: %s", name)
				}
			}
		}
		if !found {
			t.Fatalf("missing tool: %s", name)
		}
		raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
		if w := request(t, f.handler, "", "", string(raw)); w.Code != 401 {
			t.Fatalf("anonymous %s: %d", name, w.Code)
		}
		if w := request(t, f.handler, f.readToken, "", string(raw)); !strings.Contains(w.Body.String(), "mcp:write scope required") {
			t.Fatalf("read scope %s: %s", name, w.Body)
		}
	}
	pending := f.call(t, "adjust_manual_account", map[string]any{"id": f.account, "data": map[string]any{"amount": "-10", "description": "Synthetic correction"}})
	u, _ := url.Parse(pending.ApprovalURL)
	w := oauthRequest(f.handler, "GET", u.RequestURI(), "", nil, "")
	for _, text := range []string{"Registrar ajuste de saldo", "não entra nas receitas e despesas", "Synthetic correction"} {
		if !strings.Contains(w.Body.String(), text) {
			t.Fatalf("missing consequence %q: %s", text, w.Body)
		}
	}
	cookie := w.Result().Cookies()[0]
	form := url.Values{"request": {pending.RequestID}, "decision": {"allow"}, "email": {"owner@example.com"}, "password": {"synthetic-password"}}
	if w := oauthRequest(f.handler, "POST", "/oauth/write", form.Encode(), cookie, "https://attacker.example"); w.Code != 403 {
		t.Fatal("wrong origin accepted")
	}
	if w := oauthRequest(f.handler, "POST", "/oauth/write", form.Encode(), nil, testConfig.PublicURL); w.Code != 403 {
		t.Fatal("missing CSRF accepted")
	}
	form.Set("password", "wrong")
	if w := oauthRequest(f.handler, "POST", "/oauth/write", form.Encode(), cookie, testConfig.PublicURL); w.Code != 401 {
		t.Fatal("invalid owner accepted")
	}
	form.Set("decision", "deny")
	oauthRequest(f.handler, "POST", "/oauth/write", form.Encode(), cookie, testConfig.PublicURL)
	form.Set("decision", "allow")
	form.Set("password", "synthetic-password")
	oauthRequest(f.handler, "POST", "/oauth/write", form.Encode(), cookie, testConfig.PublicURL)
	if f.result(t, pending.RequestID).State != "denied" {
		t.Fatal("denied operation applied")
	}
	var count int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM account_balance_adjustments`).Scan(&count); err != nil || count != 0 {
		t.Fatal("unapproved adjustment persisted")
	}
	flow := newFlow(t, f.store)
	flow.params.Set("scope", mcpScope+" "+writeScope)
	body := flow.begin(t).Body.String()
	for _, text := range []string{"contas manuais", "ajustes de saldo", "transferências entre contas próprias", "valores a receber e seus vínculos", "Cada alteração financeira exige sua aprovação no Julius."} {
		if !strings.Contains(body, text) {
			t.Fatalf("missing disclosure: %q", text)
		}
	}
}

func TestMCPAccountsToolsRejectInvalidContracts(t *testing.T) {
	conn := dbtest.OpenSQLite(t)
	f := newWriteFixture(t, conn)
	for _, tc := range []struct {
		name string
		data map[string]any
	}{
		{"create_manual_account", map[string]any{"name": "Synthetic", "opening_balance": 1}},
		{"create_manual_account", map[string]any{"name": "Synthetic", "provider_secret": "forbidden"}},
		{"create_payable", map[string]any{"name": "Synthetic", "kind": "debt", "total_amount": 1}},
		{"create_transfer", map[string]any{"source": map[string]any{"account_id": f.account, "description": "Synthetic", "amount": -10, "occurred_at": "2026-10-01"}, "destination": map[string]any{}}},
	} {
		result, err := f.session.CallTool(context.Background(), &sdk.CallToolParams{Name: tc.name, Arguments: map[string]any{"data": tc.data}})
		if err == nil && !result.IsError {
			t.Fatalf("invalid contract accepted: %s", tc.name)
		}
	}
}
