package httpapi_test

import (
	"net/http"
	"testing"
)

func TestScenarioNestedMutationsRejectUnrelatedParents(t *testing.T) {
	for _, operation := range []string{"update-installment", "delete-installment", "delete-realization"} {
		for _, parent := range []string{"another-scenario", "missing-scenario"} {
			t.Run(operation+"/"+parent, func(t *testing.T) {
				srv, conn := newTestServer(t)
				create := func(path string, body map[string]any) map[string]any {
					t.Helper()
					resp := doJSON(t, http.MethodPost, srv.URL+path, body)
					if resp.StatusCode != http.StatusCreated {
						t.Fatalf("POST %s status = %d, want 201", path, resp.StatusCode)
					}
					var result map[string]any
					decodeJSON(t, resp, &result)
					return result
				}
				debt := create("/api/payables", map[string]any{"kind": "debt", "name": "Synthetic debt", "total_amount": "100.00"})
				scenario := create("/api/payables/"+debt["id"].(string)+"/scenarios", map[string]any{"name": "Original plan"})
				scenarioID := scenario["id"].(string)
				installment := create("/api/scenarios/"+scenarioID+"/transactions", map[string]any{
					"description": "Original installment", "amount": "42.00", "projected_at": "2026-01-01",
				})
				installmentID := installment["id"].(string)
				link := create("/api/payables/"+debt["id"].(string)+"/links", map[string]any{"transaction_id": insertTransaction(t, conn)})
				realized := create("/api/scenarios/"+scenarioID+"/transactions/"+installmentID+"/realizations", map[string]any{
					"payable_link_id": link["id"], "allocated_amount": "42.00",
				})
				realizationID := realized["realizations"].([]any)[0].(map[string]any)["id"].(string)

				parentID := "00000000-0000-0000-0000-000000000000"
				if parent == "another-scenario" {
					other := create("/api/scenarios", map[string]any{"name": "Unrelated scenario"})
					parentID = other["id"].(string)
				}
				path := "/api/scenarios/" + parentID + "/transactions/" + installmentID
				method := http.MethodDelete
				var body any
				if operation == "update-installment" {
					method = http.MethodPut
					body = map[string]any{"description": "Unwanted change", "amount": "99.00", "projected_at": "2026-02-01"}
				} else if operation == "delete-realization" {
					path += "/realizations/" + realizationID
				}
				resp := doJSON(t, method, srv.URL+path, body)
				if resp.StatusCode != http.StatusNotFound {
					t.Errorf("%s status = %d, want 404", method, resp.StatusCode)
				}
				resp.Body.Close()

				resp = doJSON(t, http.MethodGet, srv.URL+"/api/scenarios/"+scenarioID, nil)
				var detail map[string]any
				decodeJSON(t, resp, &detail)
				installments := detail["transactions"].([]any)
				if len(installments) != 1 {
					t.Fatalf("installments after rejected request = %d, want 1", len(installments))
				}
				stored := installments[0].(map[string]any)
				if stored["description"] != "Original installment" || stored["amount"] != "42.00" || stored["projected_at"] != "2026-01-01" {
					t.Errorf("installment changed after rejected request: %+v", stored)
				}
				if stored["status"] != "paga" || len(stored["realizations"].([]any)) != 1 {
					t.Errorf("realization changed after rejected request: %+v", stored)
				}
			})
		}
	}
}
