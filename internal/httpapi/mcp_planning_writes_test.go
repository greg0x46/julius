package httpapi_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/greg0x46/julius/internal/automation"
	"github.com/greg0x46/julius/internal/categories"
	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/goals"
	"github.com/greg0x46/julius/internal/httpapi"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/recurrences"
	"github.com/greg0x46/julius/internal/scenarios"
	"github.com/greg0x46/julius/internal/transactions"
	"github.com/shopspring/decimal"
)

func plannedPreview(t *testing.T, conn *sql.DB, op, parent, id string, data any) httpapi.MCPWritePreview {
	t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	p, err := httpapi.PreviewMCPWrite(context.Background(), conn, httpapi.MCPWriteRequest{Operation: op, ID: id, ScenarioID: parent, Data: raw})
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func planningResultID(t *testing.T, result httpapi.MCPWriteResult) string {
	t.Helper()
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(result.Data, &out); err != nil || out.ID == "" {
		t.Fatalf("result: %s %v", result.Data, err)
	}
	return out.ID
}
func TestMCPPlanningWriteScenarioLifecycle(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		account := planningCash(t, conn, "BRL", "100000")
		result := applyWrite(t, conn, writePreview(t, conn, "create_scenario", "", map[string]any{"name": "Synthetic vehicle"}))
		id := planningResultID(t, result)
		p := writePreview(t, conn, "update_scenario", id, map[string]any{"name": "Synthetic car"})
		applyWrite(t, conn, p)
		s, err := scenarios.GetScenario(ctx, conn, id)
		if err != nil || s.Name != "Synthetic car" || s.IsActive || s.Kind != scenarios.KindStandalone {
			t.Fatalf("scenario: %+v %v", s, err)
		}
		if _, err = httpapi.ExecuteMCPWrite(ctx, conn, p); !errors.Is(err, httpapi.ErrMCPWriteConflict) {
			t.Fatalf("stale rename: %v", err)
		}
		data := map[string]any{"description": "Synthetic purchase", "amount": "-20000.1234", "projected_at": "2026-11-10"}
		result = applyWrite(t, conn, plannedPreview(t, conn, "create_planned_transaction", id, "", data))
		plannedID := planningResultID(t, result)
		data["amount"] = "-25000.1234"
		applyWrite(t, conn, plannedPreview(t, conn, "update_planned_transaction", id, plannedID, data))
		session := planningSession(t, conn)
		query := map[string]any{"from": "2026-11-01", "to": "2026-11-30", "reference_date": "2026-11-01"}
		base := planningCall(t, session, "get_timeline", query)
		query["scenario_ids"] = []string{id}
		simulated := planningCall(t, session, "get_timeline", query)
		if reflect.DeepEqual(base["period_totals"], simulated["period_totals"]) {
			t.Fatal("scenario had no simulated effect")
		}
		balance, err := transactions.AccountBalance(ctx, conn, account)
		if err != nil || balance == nil || *balance != "100000" {
			t.Fatalf("simulation moved cash: %v %v", balance, err)
		}
		realID, err := transactions.CreateManual(ctx, conn, transactions.ManualInput{AccountID: account, Description: "Synthetic real purchase", Amount: decimal.RequireFromString("-25000.1234"), OccurredAt: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)})
		if err != nil {
			t.Fatal(err)
		}
		_, err = scenarios.RealizeEvent(ctx, conn, id, "scenario:"+id+":transaction:"+plannedID, scenarios.RealizationWrite{State: "linked", TransactionID: &realID, AllocatedAmount: decimal.RequireFromString("25000.1234")})
		if err != nil {
			t.Fatal(err)
		}
		p = writePreview(t, conn, "delete_scenario", id, map[string]any{})
		if p.AffectedCount != 2 {
			t.Fatalf("cascade preview: %+v", p)
		}
		applyWrite(t, conn, p)
		if _, err = scenarios.GetScenarioTransaction(ctx, conn, plannedID); !errors.Is(err, scenarios.ErrTransactionNotFound) {
			t.Fatalf("planned transaction survived: %v", err)
		}
		real, found, err := transactions.GetItem(ctx, conn, realID)
		if err != nil || !found || (real.Amount == nil || *real.Amount != "-25000.1234") {
			t.Fatalf("real transaction altered: %+v %v", real, err)
		}
		// Individual deletes use the same parent ownership checks.
		id = planningResultID(t, applyWrite(t, conn, writePreview(t, conn, "create_scenario", "", map[string]any{"name": "Synthetic trip"})))
		plannedID = planningResultID(t, applyWrite(t, conn, plannedPreview(t, conn, "create_planned_transaction", id, "", data)))
		applyWrite(t, conn, plannedPreview(t, conn, "delete_planned_transaction", id, plannedID, map[string]any{}))
		if _, err = scenarios.GetScenarioTransaction(ctx, conn, plannedID); !errors.Is(err, scenarios.ErrTransactionNotFound) {
			t.Fatalf("individual delete failed: %v", err)
		}
	})
}
func TestMCPPlanningWriteGoalsAndRecurrences(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		data := map[string]any{"name": "Synthetic retirement", "metric": "invested_value", "target_amount": "50000.1234", "target_date": time.Now().AddDate(2, 0, 0).Format(time.DateOnly)}
		id := planningResultID(t, applyWrite(t, conn, writePreview(t, conn, "create_goal", "", data)))
		data["target_amount"] = "75000.1234"
		result := applyWrite(t, conn, writePreview(t, conn, "update_goal", id, data))
		g, err := goals.Get(ctx, conn, id)
		if err != nil || !g.TargetAmount.Equal(decimal.RequireFromString(data["target_amount"].(string))) || g.Projection == nil {
			t.Fatalf("goal: %+v %v", g, err)
		}
		var response map[string]any
		_ = json.Unmarshal(result.Data, &response)
		if !decimal.RequireFromString(response["current_value"].(string)).Equal(g.CurrentValue) || !decimal.RequireFromString(response["progress"].(string)).Equal(g.Progress) {
			t.Fatalf("goal derived values differ: %s", result.Data)
		}
		applyWrite(t, conn, writePreview(t, conn, "delete_goal", id, map[string]any{}))
		if _, err = goals.Get(ctx, conn, id); !errors.Is(err, goals.ErrNotFound) {
			t.Fatal("goal survived")
		}
		cat, err := categories.Create(ctx, conn, "Synthetic rent", money.Expense, "coffee", "#2a78d6")
		if err != nil {
			t.Fatal(err)
		}
		data = map[string]any{"name": "Synthetic rent", "kind": "expense", "amount": "1500.1234", "category_id": cat.ID, "cadence": "monthly", "day_of_month": 31, "start_date": "2026-01-01", "is_active": true}
		id = planningResultID(t, applyWrite(t, conn, writePreview(t, conn, "create_recurring_commitment", "", data)))
		c, err := recurrences.Get(ctx, conn, id)
		feb := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
		occurrences := c.Occurrences(feb, feb.AddDate(0, 1, 0).AddDate(0, 0, -1))
		if err != nil || len(occurrences) != 1 || occurrences[0].Date.Day() != 28 {
			t.Fatalf("calendar differs: %+v %v", occurrences, err)
		}
		_, err = scenarios.RealizeEvent(ctx, conn, id, "scenario:"+id+":occurrence:2026-02-28", scenarios.RealizationWrite{State: "detached"})
		if err != nil {
			t.Fatal(err)
		}
		p := writePreview(t, conn, "delete_recurring_commitment", id, map[string]any{})
		data["day_of_month"] = 15
		data["amount"] = "1600.001"
		applyWrite(t, conn, writePreview(t, conn, "update_recurring_commitment", id, data))
		if _, err = httpapi.ExecuteMCPWrite(ctx, conn, p); !errors.Is(err, httpapi.ErrMCPWriteConflict) {
			t.Fatalf("stale schedule accepted: %v", err)
		}
		links, err := scenarios.ListRealizations(ctx, conn, id)
		if err != nil || len(links) != 0 {
			t.Fatalf("orphan decisions remain: %+v %v", links, err)
		}
		c, err = recurrences.Get(ctx, conn, id)
		if err != nil || c.DayOfMonth != 15 || c.Amount.String() != "1600.001" {
			t.Fatalf("update: %+v %v", c, err)
		}
		rule, err := automation.Create(ctx, conn, automation.Write{Name: "Synthetic reconcile", IsActive: true, LogicOperator: "and", Conditions: []automation.Condition{{Field: "description", Operator: "contains", Value: "Synthetic"}}, Actions: []automation.ActionWrite{{Type: "reconcile", ScenarioID: &id}}})
		if err != nil {
			t.Fatal(err)
		}
		raw := json.RawMessage(`{}`)
		if _, err = httpapi.PreviewMCPWrite(ctx, conn, httpapi.MCPWriteRequest{Operation: "delete_recurring_commitment", ID: id, Data: raw}); !errors.Is(err, recurrences.ErrLinkedToAutomationRule) {
			t.Fatalf("linked recurrence deletion: %v", err)
		}
		if err = automation.Delete(ctx, conn, rule.ID); err != nil {
			t.Fatal(err)
		}
		applyWrite(t, conn, writePreview(t, conn, "delete_recurring_commitment", id, map[string]any{}))
		if _, err = recurrences.Get(ctx, conn, id); !errors.Is(err, recurrences.ErrNotFound) {
			t.Fatal("recurrence survived")
		}
	})
}
func TestMCPPlanningWriteValidationAndScope(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		s, err := scenarios.CreateScenario(ctx, conn, scenarios.KindStandalone, "Synthetic", nil)
		if err != nil {
			t.Fatal(err)
		}
		other, err := scenarios.CreateScenario(ctx, conn, scenarios.KindStandalone, "Other", nil)
		if err != nil {
			t.Fatal(err)
		}
		st, err := scenarios.CreateScenarioTransaction(ctx, conn, s.ID, "Synthetic", decimal.NewFromInt(-10), time.Now(), nil)
		if err != nil {
			t.Fatal(err)
		}
		recurring, err := scenarios.CreateScenario(ctx, conn, scenarios.KindRecurring, "Synthetic recurring", nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, in := range []httpapi.MCPWriteRequest{
			{Operation: "create_goal", Data: json.RawMessage(`{"name":"Synthetic","metric":"invested_value","target_amount":"-1"}`)},
			{Operation: "create_goal", Data: json.RawMessage(`{"name":"Synthetic","metric":"unknown","target_amount":"1"}`)},
			{Operation: "create_goal", Data: json.RawMessage(`{"name":"Synthetic","metric":"invested_value","target_amount":"1","target_date":"2026-02-30"}`)},
			{Operation: "create_goal", Data: json.RawMessage(`{"name":"Synthetic","metric":"invested_value","target_amount":1}`)},
			{Operation: "delete_goal", ID: uuid.NewString(), Data: json.RawMessage(`{}`)},
			{Operation: "delete_scenario", ID: recurring.ID, Data: json.RawMessage(`{}`)},
			{Operation: "update_scenario", ID: s.ID, Data: json.RawMessage(`{"name":"New","is_active":true}`)},
			{Operation: "create_planned_transaction", ScenarioID: s.ID, Data: json.RawMessage(`{"description":"Synthetic","amount":"0","projected_at":"2026-01-01"}`)},
			{Operation: "create_planned_transaction", ScenarioID: s.ID, Data: json.RawMessage(`{"description":"Synthetic","amount":"1e9","projected_at":"2026-01-01"}`)},
			{Operation: "create_planned_transaction", ScenarioID: s.ID, Data: json.RawMessage(`{"description":"Synthetic","amount":1,"projected_at":"2026-01-01"}`)},
			{Operation: "create_planned_transaction", ScenarioID: s.ID, Data: json.RawMessage(`{"description":"Synthetic","amount":"1","projected_at":"2026-02-30"}`)},
			{Operation: "create_planned_transaction", ScenarioID: recurring.ID, Data: json.RawMessage(`{"description":"Synthetic","amount":"1","projected_at":"2026-01-01"}`)},
			{Operation: "delete_planned_transaction", ScenarioID: other.ID, ID: st.ID, Data: json.RawMessage(`{}`)},
			{Operation: "delete_planned_transaction", ScenarioID: s.ID, ID: uuid.NewString(), Data: json.RawMessage(`{}`)},
			{Operation: "delete_scenario", ID: s.ID, Data: json.RawMessage(`{"force":true}`)},
		} {
			if _, err := httpapi.PreviewMCPWrite(ctx, conn, in); err == nil {
				t.Fatalf("invalid accepted: %+v", in)
			}
		}
		p := writePreview(t, conn, "delete_scenario", s.ID, map[string]any{})
		_, err = scenarios.CreateScenarioTransaction(ctx, conn, s.ID, "Another", decimal.NewFromInt(-1), time.Now(), nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = httpapi.ExecuteMCPWrite(ctx, conn, p); !errors.Is(err, httpapi.ErrMCPWriteConflict) {
			t.Fatalf("stale cascade accepted: %v", err)
		}
	})
}

func TestMCPPlanningWriteInvalidRecurrence(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		cat, err := categories.Create(ctx, conn, "Synthetic rent", money.Expense, "coffee", "#2a78d6")
		if err != nil {
			t.Fatal(err)
		}
		for _, bad := range []struct {
			field string
			value any
		}{
			{"amount", "0"}, {"amount", "-1"}, {"amount", 1}, {"amount", "1e2"},
			{"start_date", "2026-02-30"}, {"end_date", "2025-12-31"},
			{"cadence", "weekly"}, {"day_of_month", 0}, {"day_of_month", 32},
			{"kind", "unknown"}, {"category_id", uuid.NewString()}, {"account_id", uuid.NewString()},
		} {
			data := map[string]any{"name": "Synthetic rent", "kind": "expense", "amount": "100.001", "category_id": cat.ID, "cadence": "monthly", "day_of_month": 1, "start_date": "2026-01-01", "is_active": true}
			data[bad.field] = bad.value
			raw, _ := json.Marshal(data)
			if _, err := httpapi.PreviewMCPWrite(ctx, conn, httpapi.MCPWriteRequest{Operation: "create_recurring_commitment", Data: raw}); err == nil {
				t.Fatalf("invalid recurrence accepted: %+v", bad)
			}
		}
	})
}
