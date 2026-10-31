package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/greg0x46/julius/internal/categories"
	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/scenarios"
	"github.com/greg0x46/julius/internal/transactions"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func (f *writeFixture) applyPlanning(t *testing.T, tool string, args map[string]any) string {
	t.Helper()
	out := f.call(t, tool, args)
	if out.State != "pending" || out.Preview == nil {
		t.Fatalf("no preview: %+v", out)
	}
	f.approve(t, out)
	result := f.result(t, out.RequestID)
	if result.State != "applied" || result.Result == nil {
		t.Fatalf("%s: %+v", tool, result)
	}
	if result.Result.Status == 204 {
		return ""
	}
	var item struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(result.Result.Data, &item); err != nil || item.ID == "" {
		t.Fatalf("result: %s %v", result.Result.Data, err)
	}
	return item.ID
}
func TestMCPPlanningWriteToolsLifecycle(t *testing.T) {
	for _, family := range []string{"goal", "scenario", "planned_transaction", "recurring_commitment"} {
		t.Run(family, func(t *testing.T) {
			dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
				f := newWriteFixture(t, conn)
				args := map[string]any{}
				var data map[string]any
				switch family {
				case "goal":
					data = map[string]any{"name": "Synthetic goal", "metric": "invested_value", "target_amount": "10000.001", "target_date": "2028-12-01", "notes": "Synthetic notes"}
				case "scenario":
					data = map[string]any{"name": "Synthetic vehicle"}
				case "planned_transaction":
					parent, err := scenarios.CreateScenario(context.Background(), conn, scenarios.KindStandalone, "Synthetic vehicle", nil)
					if err != nil {
						t.Fatal(err)
					}
					args["scenario_id"] = parent.ID
					data = map[string]any{"description": "Synthetic car purchase", "amount": "-50.001", "projected_at": "2026-12-01"}
				case "recurring_commitment":
					cat, err := categories.Create(context.Background(), conn, "Synthetic category", money.Expense, "coffee", "#2a78d6")
					if err != nil {
						t.Fatal(err)
					}
					data = map[string]any{"name": "Synthetic rent", "kind": "expense", "amount": "10.001", "category_id": cat.ID, "cadence": "monthly", "day_of_month": 10, "start_date": "2026-01-01", "is_active": true}
				}
				args["data"] = data
				id := f.applyPlanning(t, "create_"+family, args)
				args["id"] = id
				switch family {
				case "goal":
					data["target_amount"] = "20000.0001"
				case "scenario":
					data["name"] = "Synthetic car"
				case "planned_transaction":
					data["amount"] = "-60.001"
				case "recurring_commitment":
					data["day_of_month"] = 15
					data["is_active"] = false
				}
				f.applyPlanning(t, "update_"+family, args)
				args["data"] = map[string]any{}
				f.applyPlanning(t, "delete_"+family, args)
				balance, err := transactions.AccountBalance(context.Background(), conn, f.account)
				if err != nil || balance == nil || *balance != "100" {
					t.Fatalf("planning changed cash: %v %v", balance, err)
				}
			})
		})
	}
}
func TestMCPPlanningWriteAuthorizationAndDestruction(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		f := newWriteFixture(t, conn)
		tools, err := f.session.ListTools(context.Background(), &sdk.ListToolsParams{})
		if err != nil {
			t.Fatal(err)
		}
		for _, tool := range tools.Tools {
			if !strings.HasSuffix(tool.Name, "goal") && !strings.HasSuffix(tool.Name, "scenario") && !strings.HasSuffix(tool.Name, "planned_transaction") && !strings.HasSuffix(tool.Name, "recurring_commitment") {
				continue
			}
			if tool.Annotations == nil || tool.Annotations.ReadOnlyHint {
				t.Fatalf("write marked read-only: %s", tool.Name)
			}
			data := map[string]any{}
			if strings.HasSuffix(tool.Name, "scenario") && !strings.HasPrefix(tool.Name, "delete_") {
				data["name"] = "Synthetic"
			}
			if strings.HasSuffix(tool.Name, "goal") && !strings.HasPrefix(tool.Name, "delete_") {
				data = map[string]any{"name": "Synthetic", "metric": "invested_value", "target_amount": "1"}
			}
			if strings.HasSuffix(tool.Name, "planned_transaction") && !strings.HasPrefix(tool.Name, "delete_") {
				data = map[string]any{"description": "Synthetic", "amount": "-1", "projected_at": "2026-01-01"}
			}
			if strings.HasSuffix(tool.Name, "recurring_commitment") && !strings.HasPrefix(tool.Name, "delete_") {
				data = map[string]any{"name": "Synthetic", "kind": "expense", "amount": "1", "category_id": uuid.NewString(), "cadence": "monthly", "day_of_month": 1, "start_date": "2026-01-01", "is_active": true}
			}
			args := map[string]any{"data": data}
			if !strings.HasPrefix(tool.Name, "create_") {
				args["id"] = uuid.NewString()
			}
			if strings.HasSuffix(tool.Name, "planned_transaction") {
				args["scenario_id"] = uuid.NewString()
			}
			raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": tool.Name, "arguments": args}})
			if w := request(t, f.handler, "", "", string(raw)); w.Code != 401 {
				t.Fatalf("anonymous %s: %d", tool.Name, w.Code)
			}
			if w := request(t, f.handler, f.readToken, "", string(raw)); !strings.Contains(w.Body.String(), "mcp:write scope required") {
				t.Fatalf("read scope %s: %s", tool.Name, w.Body)
			}
		}
		id := f.applyPlanning(t, "create_scenario", map[string]any{"data": map[string]any{"name": "Synthetic vehicle"}})
		pending := f.call(t, "delete_scenario", map[string]any{"id": id, "data": map[string]any{}})
		var count int
		if err := conn.QueryRow(`SELECT COUNT(*) FROM scenarios WHERE id=?`, id).Scan(&count); err != nil || count != 1 {
			t.Fatalf("prepare deleted: %d %v", count, err)
		}
		u, _ := url.Parse(pending.ApprovalURL)
		w := oauthRequest(f.handler, "GET", u.RequestURI(), "", nil, "")
		if !strings.Contains(w.Body.String(), "Excluir cenário") || !strings.Contains(w.Body.String(), "lançamentos reais") {
			t.Fatalf("missing consequences: %s", w.Body)
		}
		cookie := w.Result().Cookies()[0]
		form := url.Values{"request": {pending.RequestID}, "decision": {"deny"}}
		w = oauthRequest(f.handler, "POST", "/oauth/write", form.Encode(), cookie, testConfig.PublicURL)
		if w.Code != 200 || f.result(t, pending.RequestID).State != "denied" {
			t.Fatalf("denial: %d", w.Code)
		}
		// A denied deletion can never be approved by replaying the same request.
		form.Set("decision", "allow")
		form.Set("email", "owner@example.com")
		form.Set("password", "synthetic-password")
		oauthRequest(f.handler, "POST", "/oauth/write", form.Encode(), cookie, testConfig.PublicURL)
		if err := conn.QueryRow(`SELECT COUNT(*) FROM scenarios WHERE id=?`, id).Scan(&count); err != nil || count != 1 {
			t.Fatalf("denied deletion ran: %d %v", count, err)
		}
		f.applyPlanning(t, "delete_scenario", map[string]any{"id": id, "data": map[string]any{}})
	})
}

func TestMCPPlanningWriteRejectsInvalidContracts(t *testing.T) {
	conn := dbtest.OpenSQLite(t)
	f := newWriteFixture(t, conn)
	for _, tc := range []struct {
		tool string
		args map[string]any
	}{
		{"create_goal", map[string]any{"data": map[string]any{"name": "Synthetic", "metric": "invested_value", "target_amount": 1}}},
		{"create_scenario", map[string]any{"data": map[string]any{"name": "Synthetic", "is_active": true}}},
		{"create_planned_transaction", map[string]any{"data": map[string]any{"description": "Synthetic", "amount": "-1", "projected_at": "2026-01-01"}}},
		{"delete_recurring_commitment", map[string]any{"id": uuid.NewString(), "data": map[string]any{}}},
	} {
		result, err := f.session.CallTool(context.Background(), &sdk.CallToolParams{Name: tc.tool, Arguments: tc.args})
		if err == nil && !result.IsError {
			t.Fatalf("invalid contract accepted: %s", tc.tool)
		}
	}
}

func TestMCPPlanningWriteConsentDisclosure(t *testing.T) {
	conn := dbtest.OpenSQLite(t)
	store, _ := setup(t, conn)
	flow := newFlow(t, store)
	flow.params.Set("scope", mcpScope+" "+writeScope)
	body := flow.begin(t).Body.String()
	for _, text := range []string{"metas, cenários e compromissos recorrentes", "Cada alteração financeira exige sua aprovação no Julius."} {
		if !strings.Contains(body, text) {
			t.Fatalf("missing write disclosure %q", text)
		}
	}
}
