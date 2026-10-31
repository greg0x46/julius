package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/greg0x46/julius/internal/scenarios"
)

func TestScenarioGenerationLimitsReturnValidationErrors(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := doJSON(t, http.MethodPost, srv.URL+"/api/payables", map[string]any{
		"kind": "debt", "name": "Synthetic payable", "total_amount": "9999999999999",
	})
	var payable map[string]any
	decodeJSON(t, resp, &payable)
	newScenario := func(t *testing.T) string {
		t.Helper()
		resp := doJSON(t, http.MethodPost, srv.URL+"/api/payables/"+payable["id"].(string)+"/scenarios", map[string]any{"name": "Synthetic plan"})
		var scenario map[string]any
		decodeJSON(t, resp, &scenario)
		return srv.URL + "/api/scenarios/" + scenario["id"].(string)
	}
	assertLimitProblem := func(t *testing.T, resp *http.Response) {
		t.Helper()
		if resp.StatusCode != http.StatusUnprocessableEntity {
			resp.Body.Close()
			t.Fatalf("status=%d, want 422", resp.StatusCode)
		}
		var problem map[string]any
		decodeJSON(t, resp, &problem)
		if problem["detail"] != "O plano não pode gerar mais de 10.000 parcelas por operação." {
			t.Fatalf("unexpected validation problem: %+v", problem)
		}
	}
	for _, tc := range []struct {
		name, path string
		body       map[string]any
	}{
		{"count", "/generate-installments", map[string]any{"cadence": "mensal", "months": scenarios.MaxInstallments + 1}},
		{"amount", "/generate-installments", map[string]any{"cadence": "mensal", "installment_amount": "1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			url := newScenario(t)
			assertLimitProblem(t, doJSON(t, http.MethodPost, url+tc.path, tc.body))
		})
	}
	url := newScenario(t)
	resp = doJSON(t, http.MethodPost, url+"/transactions", map[string]any{
		"description": "Preserved installment", "amount": "1", "projected_at": "2026-09-01",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create installment: status=%d", resp.StatusCode)
	}
	var original map[string]any
	decodeJSON(t, resp, &original)
	assertLimitProblem(t, doJSON(t, http.MethodPost, url+"/readjust", map[string]any{"strategy": "abater_do_final"}))
	resp = doJSON(t, http.MethodGet, url, nil)
	var stored map[string]any
	decodeJSON(t, resp, &stored)
	installments := stored["transactions"].([]any)
	if len(installments) != 1 {
		t.Fatalf("stored installments after rejection = %v", installments)
	}
	installment := installments[0].(map[string]any)
	for _, field := range []string{"id", "description", "amount", "projected_at"} {
		if installment[field] != original[field] {
			t.Errorf("stored %s=%v, want %v", field, installment[field], original[field])
		}
	}
}
