package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/transactions"
)

func TestTransactionReconciliationRetainsTransferCategoryPayment(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		request := func(handler http.HandlerFunc, method, body, id string) *httptest.ResponseRecorder {
			t.Helper()
			r := httptest.NewRequest(method, "/", bytes.NewBufferString(body))
			r.SetPathValue("id", id)
			r.SetPathValue("date", "2026-02-05")
			w := httptest.NewRecorder()
			handler(w, r)
			return w
		}
		decode := func(w *httptest.ResponseRecorder, want int, dst any) {
			t.Helper()
			if w.Code != want {
				t.Fatalf("status = %d: %s, want %d", w.Code, w.Body.String(), want)
			}
			if err := json.Unmarshal(w.Body.Bytes(), dst); err != nil {
				t.Fatal(err)
			}
		}
		w := request(handleWriteManualAccount(conn, true), "POST", `{"name":"Synthetic account","currency_code":"BRL","opening_balance":"0"}`, "")
		var account accountDTO
		decode(w, 201, &account)
		occurredAt := time.Date(2026, 2, 6, 0, 0, 0, 0, time.UTC)
		txID, err := transactions.CreateManual(context.Background(), conn, transactions.ManualInput{
			AccountID: account.ID, Description: "Synthetic payment", Amount: decimal.NewFromInt(-1490), OccurredAt: occurredAt,
		})
		if err != nil {
			t.Fatal(err)
		}
		var expenseCategory, transferCategory string
		if err := conn.QueryRow(`SELECT id FROM categories WHERE kind = 'expense' LIMIT 1`).Scan(&expenseCategory); err != nil {
			t.Fatal(err)
		}
		if err := conn.QueryRow(`SELECT id FROM categories WHERE kind = 'transfer' LIMIT 1`).Scan(&transferCategory); err != nil {
			t.Fatal(err)
		}
		w = request(handleCreateRecurringCommitment(conn), "POST", `{"name":"Synthetic commitment","kind":"expense","amount":"1500","category_id":"`+expenseCategory+`","cadence":"monthly","day_of_month":5,"start_date":"2026-01-01","is_active":true}`, "")
		var commitment recurringCommitmentDTO
		decode(w, 201, &commitment)
		w = request(handleSetTransactionCategory(conn), "PUT", `{"category_id":"`+transferCategory+`"}`, txID)
		if w.Code != 200 {
			t.Fatalf("set transfer category: %d %s", w.Code, w.Body.String())
		}

		read := func() transactionReconciliationDTO {
			t.Helper()
			w := request(handleGetTransactionReconciliation(conn), "GET", "", txID)
			var result transactionReconciliationDTO
			decode(w, 200, &result)
			return result
		}
		before := read()
		found := false
		for _, option := range before.Options {
			if option.CommitmentID == commitment.ID && option.OccurrenceDate == "2026-02-05" {
				found = true
			}
		}
		if !found {
			t.Error("transfer payment options omit February's occurrence")
		}
		w = request(handlePutRecurrenceReconciliation(conn), "PUT", `{"state":"linked","transaction_id":"`+txID+`"}`, commitment.ID)
		if w.Code != 200 {
			t.Fatalf("link transfer payment: %d %s", w.Code, w.Body.String())
		}
		after := read()
		if after.Current == nil || after.Current.CommitmentID != commitment.ID || after.Current.OccurrenceDate != "2026-02-05" || after.Current.Origin != "manual" {
			t.Errorf("saved reconciliation missing from transfer payment: %+v", after)
		}
		w = request(handleCreateAutomationRule(conn), "POST", `{"name":"Synthetic reconciliation","is_active":true,"logic_operator":"and","conditions":[{"field":"description","operator":"equals","value":"Synthetic payment"}],"actions":[{"type":"reconcile","scenario_id":"`+commitment.ID+`"}]}`, "")
		if w.Code != 201 {
			t.Fatalf("create reconciliation rule: %d %s", w.Code, w.Body.String())
		}
		w = request(handleDeleteRecurrenceReconciliation(conn), "DELETE", "", commitment.ID)
		if w.Code != 204 {
			t.Fatalf("restore automatic reconciliation: %d %s", w.Code, w.Body.String())
		}
		automatic := read()
		if automatic.Current == nil || automatic.Current.CommitmentID != commitment.ID || automatic.Current.OccurrenceDate != "2026-02-05" || automatic.Current.Origin != "rule" {
			t.Errorf("automatic reconciliation missing from transfer payment: %+v", automatic)
		}
		w = request(handleSetTransactionInclusion(conn), "PUT", `{"state":"ignored"}`, txID)
		if w.Code != 200 {
			t.Fatalf("ignore transfer payment: %d %s", w.Code, w.Body.String())
		}
		ignored := read()
		if ignored.Current != nil || len(ignored.Options) != 0 {
			t.Errorf("ignored transfer payment offers reconciliation: %+v", ignored)
		}
	})
}
