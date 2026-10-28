package httpapi_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/greg0x46/julius/internal/automation"
	"github.com/greg0x46/julius/internal/categories"
	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/httpapi"
	"github.com/greg0x46/julius/internal/ledger"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/transactions"
	"github.com/shopspring/decimal"
)

func writePreview(t *testing.T, conn *sql.DB, operation, id string, data any) httpapi.MCPWritePreview {
	t.Helper()
	raw, _ := json.Marshal(data)
	out, err := httpapi.PreviewMCPWrite(context.Background(), conn, httpapi.MCPWriteRequest{Operation: operation, ID: id, Data: raw})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func applyWrite(t *testing.T, conn *sql.DB, p httpapi.MCPWritePreview) httpapi.MCPWriteResult {
	t.Helper()
	result, err := httpapi.ExecuteMCPWrite(context.Background(), conn, p)
	if err != nil || result.Status >= 400 {
		t.Fatalf("write %s: %+v %v", p.Request.Operation, result, err)
	}
	return result
}
func TestMCPWriteInstallmentPreviewAndRollback(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		account := planningCash(t, conn, "BRL", "100")
		cat, err := categories.Create(ctx, conn, "Synthetic transfer", money.Transfer, "swap", "#2a78d6")
		if err != nil {
			t.Fatal(err)
		}
		ids := []string{}
		for n := 1; n <= 2; n++ {
			id, err := transactions.CreateManual(ctx, conn, transactions.ManualInput{AccountID: account, Description: fmt.Sprintf("Synthetic purchase %d/2", n), Amount: decimal.RequireFromString("-10"), OccurredAt: time.Date(2026, time.October, n, 12, 0, 0, 0, time.UTC)})
			if err != nil {
				t.Fatal(err)
			}
			planningExec(t, conn, `UPDATE financial_transactions SET credit_card_metadata=? WHERE id=?`, fmt.Sprintf(`{"installmentNumber":%d,"totalInstallments":2,"cardNumber":"1234"}`, n), id)
			ids = append(ids, id)
		}
		p := writePreview(t, conn, "assign_transaction_category", ids[0], map[string]any{"category_id": cat.ID})
		if p.AffectedCount != 2 || len(p.Before) != 3 {
			t.Fatalf("inaccurate preview: %+v", p)
		}
		drop := dbtest.FailInserts(t, conn, "transaction_category_events", `NEW.transaction_id='`+p.AffectedIDs[1]+`'`)
		if _, err = httpapi.ExecuteMCPWrite(ctx, conn, p); err == nil {
			t.Fatal("injected failure not reported")
		}
		var count int
		_ = conn.QueryRow(`SELECT COUNT(*) FROM transaction_category_decisions`).Scan(&count)
		if count != 0 {
			t.Fatal("partial installment categorization")
		}
		drop()
		applyWrite(t, conn, p)
		for _, id := range ids {
			item, _, err := transactions.GetItem(ctx, conn, id)
			if err != nil || item.InternalCategory == nil || item.InternalCategory.ID != cat.ID || item.TotalsEligibility.Included {
				t.Fatalf("category/invariant differs: %+v %v", item, err)
			}
		}
		if _, err = httpapi.ExecuteMCPWrite(ctx, conn, p); !errors.Is(err, httpapi.ErrMCPWriteConflict) {
			t.Fatalf("stale bulk accepted: %v", err)
		}
		// A new sibling after preview must not silently enter the approved cascade.
		p = writePreview(t, conn, "assign_transaction_category", ids[0], map[string]any{"category_id": cat.ID})
		planningExec(t, conn, `UPDATE financial_transactions SET description='Other purchase' WHERE id=?`, ids[1])
		if _, err = httpapi.ExecuteMCPWrite(ctx, conn, p); !errors.Is(err, httpapi.ErrMCPWriteConflict) {
			t.Fatalf("changed membership accepted: %v", err)
		}
	})
}
func TestMCPWriteCatalogRulesAndInclusion(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		account := planningCash(t, conn, "BRL", "100")
		result := applyWrite(t, conn, writePreview(t, conn, "create_category", "", map[string]any{"name": "Synthetic groceries", "kind": "expense", "icon": "coffee", "color": "#2a78d6"}))
		var cat struct{ ID string }
		_ = json.Unmarshal(result.Data, &cat)
		result = applyWrite(t, conn, writePreview(t, conn, "update_category", cat.ID, map[string]any{"name": "Synthetic food"}))
		if !strings.Contains(string(result.Data), "Synthetic food") {
			t.Fatal("category update failed")
		}
		data := map[string]any{"name": "Synthetic rule", "is_active": true, "logic_operator": "and", "conditions": []map[string]any{{"field": "description", "operator": "contains", "value": "Synthetic"}}, "actions": []map[string]any{{"type": "set_category", "category_id": cat.ID}}}
		result = applyWrite(t, conn, writePreview(t, conn, "create_automation_rule", "", data))
		var rule struct{ Rule struct{ ID string } }
		_ = json.Unmarshal(result.Data, &rule)
		svc := ledger.Service{DB: conn, OnIgnored: ledger.OnIgnored}
		item, err := svc.CreateManual(ctx, transactions.ManualInput{AccountID: account, Description: "Synthetic market", Amount: decimal.RequireFromString("-12.345"), OccurredAt: time.Now()}, nil)
		if err != nil || item.InternalCategory == nil || item.InternalCategory.ID != cat.ID {
			t.Fatalf("automation not shared: %+v %v", item, err)
		}
		p := writePreview(t, conn, "set_transaction_inclusion", item.ID, map[string]any{"state": "ignored"})
		applyWrite(t, conn, p)
		current, _, err := transactions.GetItem(ctx, conn, item.ID)
		if err != nil || current.Inclusion.State != money.Ignored {
			t.Fatal("ignore not applied")
		}
		balance, err := transactions.AccountBalance(ctx, conn, account)
		if err != nil || *balance != "87.655" {
			t.Fatalf("ignore changed cash: %v %v", balance, err)
		}
		applyWrite(t, conn, writePreview(t, conn, "set_transaction_inclusion", item.ID, map[string]any{"state": "considered"}))
		data["is_active"] = false
		applyWrite(t, conn, writePreview(t, conn, "update_automation_rule", rule.Rule.ID, data))
		updated, err := automation.Get(ctx, conn, rule.Rule.ID)
		if err != nil || updated.IsActive {
			t.Fatal("rule update failed")
		}
		applyWrite(t, conn, writePreview(t, conn, "delete_automation_rule", rule.Rule.ID, map[string]any{}))
		if _, err = automation.Get(ctx, conn, rule.Rule.ID); !errors.Is(err, automation.ErrNotFound) {
			t.Fatal("rule not deleted")
		}
	})
}
func TestMCPWriteRejectsInvalidInputsAndSyncedEditing(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		account := planningCash(t, conn, "BRL", "100")
		for _, test := range []struct{ operation, id, key, data string }{
			{"create_manual_transaction", "", "valid-key", `{"account_id":"` + account + `","description":"Synthetic","amount":1.1,"occurred_at":"2026-10-01"}`},
			{"create_manual_transaction", "", "valid-key", `{"account_id":"` + account + `","description":"Synthetic","amount":"1e4","occurred_at":"2026-10-01"}`},
			{"create_manual_transaction", "", "valid-key", `{"account_id":"` + account + `","description":"Synthetic","amount":"1","occurred_at":"2026-02-30"}`},
			{"create_manual_transaction", "", "", `{"account_id":"` + account + `","description":"Synthetic","amount":"1","occurred_at":"2026-10-01"}`},
			{"delete_manual_transaction", "bad-id", "", `{}`},
			{"delete_manual_transaction", uuid.NewString(), "", `{"force":true}`},
			{"set_transaction_inclusion", uuid.NewString(), "", `{"state":"unknown"}`},
			{"assign_transaction_category", uuid.NewString(), "", `{"category_id":"bad"}`},
			{"create_automation_rule", "", "", `{"name":"rule","apply_retroactively":true}`},
			{"http_proxy", "", "", `{"url":"/api/settings"}`},
		} {
			if _, err := httpapi.PreviewMCPWrite(ctx, conn, httpapi.MCPWriteRequest{Operation: test.operation, ID: test.id, Data: json.RawMessage(test.data), IdempotencyKey: test.key}); err == nil {
				t.Fatalf("invalid accepted: %+v", test)
			}
		}
		// Both dialects enforce the same provider-origin restriction.
		seed := accountSeed{uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()}
		now := db.FormatTime(time.Now())
		planningExec(t, conn, `INSERT INTO data_sources(id,provider,external_item_id,created_at,updated_at) VALUES(?,'pluggy',?,?,?)`, seed.sourceID, seed.sourceID, now, now)
		planningExec(t, conn, `INSERT INTO sync_runs(id,source_id,status,started_at,finished_at) VALUES(?,?,'completed',?,?)`, seed.syncRunID, seed.sourceID, now, now)
		planningExec(t, conn, `INSERT INTO raw_imports(id,sync_run_id,source_id,scope,page_sequence,request_attempt,request_method,request_path,http_status,response_headers,payload,payload_sha256,received_at) VALUES(?,?,?,'accounts',1,1,'GET','/x',200,'{}',?,'sha',?)`, seed.rawImportID, seed.syncRunID, seed.sourceID, []byte{}, now)
		planningExec(t, conn, `INSERT INTO financial_accounts(id,source_id,external_id,currency_code,current_raw_import_id,normalized_hash,created_at,updated_at) VALUES(?,?,?,'BRL',?,'hash',?,?)`, seed.accountID, seed.sourceID, seed.accountID, seed.rawImportID, now, now)
		insertAccountTransaction(t, conn, seed, nil, time.Now())
		var id string
		if err := conn.QueryRow(`SELECT id FROM financial_transactions WHERE account_id=?`, seed.accountID).Scan(&id); err != nil {
			t.Fatal(err)
		}
		for _, op := range []string{"update_manual_transaction", "delete_manual_transaction"} {
			data := json.RawMessage(`{}`)
			if op == "update_manual_transaction" {
				data = json.RawMessage(`{"account_id":"` + account + `","description":"Synthetic","amount":"1","occurred_at":"2026-10-01"}`)
			}
			if _, err := httpapi.PreviewMCPWrite(ctx, conn, httpapi.MCPWriteRequest{Operation: op, ID: id, Data: data}); !errors.Is(err, transactions.ErrNotManual) {
				t.Fatalf("synced %s: %v", op, err)
			}
		}
	})
}

func TestMCPWriteTransferCleanupPreview(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		source := planningCash(t, conn, "BRL", "100")
		destination := planningCash(t, conn, "BRL", "0")
		svc := ledger.Service{DB: conn, OnIgnored: ledger.OnIgnored}
		debit, err := svc.CreateManual(ctx, transactions.ManualInput{AccountID: source, Description: "Synthetic outgoing", Amount: decimal.RequireFromString("-10.0001"), OccurredAt: time.Now()}, nil)
		if err != nil {
			t.Fatal(err)
		}
		credit, err := svc.CreateManual(ctx, transactions.ManualInput{AccountID: destination, Description: "Synthetic incoming", Amount: decimal.RequireFromString("10.0001"), OccurredAt: time.Now()}, nil)
		if err != nil {
			t.Fatal(err)
		}
		link, err := transactions.CreateTransfer(ctx, conn, transactions.TransferInput{SourceID: debit.ID, DestinationID: credit.ID})
		if err != nil {
			t.Fatal(err)
		}
		considered := writePreview(t, conn, "set_transaction_inclusion", debit.ID, map[string]any{"state": "considered"})
		if considered.AffectedCount != 1 {
			t.Fatal("considering incorrectly previews transfer cleanup")
		}
		deletion := writePreview(t, conn, "delete_manual_transaction", debit.ID, map[string]any{})
		if deletion.AffectedCount != 2 {
			t.Fatal("deleting omitted transfer counterpart")
		}
		p := writePreview(t, conn, "set_transaction_inclusion", debit.ID, map[string]any{"state": "ignored"})
		raw, _ := json.Marshal(p)
		if p.AffectedCount != 2 || !strings.Contains(string(raw), credit.ID) || !strings.Contains(string(raw), link) || !strings.Contains(string(raw), "10.0001") {
			t.Fatalf("transfer effects missing: %s", raw)
		}
		applyWrite(t, conn, p)
		transfer, err := transactions.GetTransferForTransaction(ctx, conn, credit.ID)
		if err != nil || transfer != nil {
			t.Fatal("transfer not removed")
		}
		item, found, err := transactions.GetItem(ctx, conn, credit.ID)
		if err != nil || !found || !item.TotalsEligibility.Included {
			t.Fatalf("counterpart not preserved: %+v %v", item, err)
		}
	})
}
