package httpapi_test

import (
	"net/http"
	"testing"
)

var goalResponseKeys = []string{
	"id", "name", "metric", "target_amount", "target_date", "notes",
	"current_value", "progress", "projection", "monthly_contribution", "currency_code", "created_at", "updated_at",
}

func TestGoalLifecycleOverHTTP(t *testing.T) {
	srv, conn := newTestServer(t)

	if body := investmentRawBody(t, srv, "/api/goals"); body != `{"items":[]}` {
		t.Errorf("empty goal list = %s, want an items envelope with an empty array", body)
	}

	// 1000.50 invested through the provider counts toward the goal.
	insertInvestment(t, conn)

	created := investmentCall(t, srv, http.MethodPost, "/api/goals", map[string]any{
		"name": "Primeiro milhão", "metric": "invested_value", "target_amount": "2001.00",
		"target_date": "2027-12-31", "notes": "longo prazo",
	}, http.StatusCreated)
	assertInvestmentKeys(t, "goal", created, goalResponseKeys)
	id := investmentID(t, created)
	if created["metric"] != "invested_value" || created["currency_code"] != "BRL" || created["notes"] != "longo prazo" {
		t.Errorf("created = %+v", created)
	}
	if created["target_date"] != "2027-12-31" {
		t.Errorf("target_date = %v, want a plain YYYY-MM-DD", created["target_date"])
	}
	assertMoney(t, "target_amount", created["target_amount"], "2001.00")
	assertMoney(t, "current_value", created["current_value"], "1000.50")
	assertMoney(t, "progress", created["progress"], "0.5")

	// A dated goal is projected to its date; with no measurable history the
	// projection assumes no appreciation.
	projection, _ := created["projection"].(map[string]any)
	assertInvestmentKeys(t, "projection", projection, []string{
		"status", "months_remaining", "monthly_return", "return_from", "return_partial",
		"projected_value", "projected_progress", "required_monthly_contribution",
	})
	if projection["status"] != "behind" || projection["monthly_return"] != nil || projection["return_from"] != nil {
		t.Errorf("projection = %+v", projection)
	}
	assertMoney(t, "projected_value", projection["projected_value"], "1000.50")
	assertMoney(t, "projected_progress", projection["projected_progress"], "0.5")

	// A bare JSON number is read from its literal text, and the date is optional.
	second := investmentCall(t, srv, http.MethodPost, "/api/goals", map[string]any{
		"name": "Viagem", "metric": "invested_value", "target_amount": 5000,
	}, http.StatusCreated)
	if second["target_date"] != nil || second["notes"] != nil {
		t.Errorf("optional fields = %+v", second)
	}
	assertMoney(t, "second target", second["target_amount"], "5000")
	if second["projection"] != nil {
		t.Errorf("undated goal projection = %+v, want null", second["projection"])
	}

	items := investmentItems(t, srv, "/api/goals")
	if len(items) != 2 || items[0]["id"] != id {
		t.Fatalf("list = %+v, want both goals in creation order", items)
	}
	assertInvestmentKeys(t, "goal (list)", items[0], goalResponseKeys)

	updated := investmentCall(t, srv, http.MethodPut, "/api/goals/"+id, map[string]any{
		"name": "Reserva", "metric": "invested_value", "target_amount": "1000.50", "target_date": nil, "notes": nil,
	}, http.StatusOK)
	if updated["name"] != "Reserva" || updated["target_date"] != nil || updated["notes"] != nil {
		t.Errorf("update = %+v", updated)
	}
	assertMoney(t, "progress", updated["progress"], "1")

	investmentCall(t, srv, http.MethodDelete, "/api/goals/"+id, nil, http.StatusNoContent)
	if items := investmentItems(t, srv, "/api/goals"); len(items) != 1 {
		t.Errorf("list after delete = %+v", items)
	}
}

func TestGoalRefusalsOverHTTP(t *testing.T) {
	srv, _ := newTestServer(t)
	valid := func(overrides map[string]any) map[string]any {
		body := map[string]any{"name": "x", "metric": "invested_value", "target_amount": "100"}
		for key, value := range overrides {
			body[key] = value
		}
		return body
	}
	for name, body := range map[string]map[string]any{
		"empty name":      valid(map[string]any{"name": ""}),
		"no target":       valid(map[string]any{"target_amount": nil}),
		"zero target":     valid(map[string]any{"target_amount": "0"}),
		"negative target": valid(map[string]any{"target_amount": "-5"}),
		"text target":     valid(map[string]any{"target_amount": "cem"}),
		"bad date":        valid(map[string]any{"target_date": "31/12/2027"}),
		"unknown metric":  valid(map[string]any{"metric": "net_worth"}),
		"missing metric":  valid(map[string]any{"metric": ""}),
		"unknown field":   valid(map[string]any{"portfolio_id": "x"}),
	} {
		t.Run(name, func(t *testing.T) {
			if slug := investmentProblem(t, srv, http.MethodPost, "/api/goals", body, http.StatusBadRequest); slug != "invalid-goal-input" {
				t.Errorf("slug = %q", slug)
			}
		})
	}

	const missing = "/api/goals/11111111-1111-4111-8111-111111111111"
	if slug := investmentProblem(t, srv, http.MethodPut, missing, valid(nil), http.StatusNotFound); slug != "goal-not-found" {
		t.Errorf("update missing slug = %q", slug)
	}
	if slug := investmentProblem(t, srv, http.MethodDelete, missing, nil, http.StatusNotFound); slug != "goal-not-found" {
		t.Errorf("delete missing slug = %q", slug)
	}
	if items := investmentItems(t, srv, "/api/goals"); len(items) != 0 {
		t.Errorf("refused requests persisted goals: %+v", items)
	}
}
