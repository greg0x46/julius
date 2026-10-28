package httpapi_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/httpapi"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/payables"
	"github.com/greg0x46/julius/internal/scenarios"
	"github.com/greg0x46/julius/internal/transactions"
	"github.com/shopspring/decimal"
)

func mcpAccountData(currency, opening string) map[string]any {
	return map[string]any{"name": "Synthetic cash", "currency_code": currency, "opening_balance": opening, "archived": false}
}
func mcpMovement(account, amount string) map[string]any {
	return map[string]any{"account_id": account, "description": "Synthetic movement", "amount": amount, "occurred_at": "2026-10-01"}
}
func mcpBalance(t *testing.T, conn *sql.DB, account, want string) {
	t.Helper()
	value, err := transactions.AccountBalance(context.Background(), conn, account)
	if value != nil && *value != want {
		t.Fatalf("balance: %s, want %s", *value, want)
	}
	if err != nil || value == nil {
		t.Fatalf("balance: %v, want %s: %v", value, want, err)
	}
}
func mcpCount(t *testing.T, conn *sql.DB, table string, want int) {
	t.Helper()
	var count int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != want {
		t.Fatalf("%s: %d want %d: %v", table, count, want, err)
	}
}
func mcpRejected(t *testing.T, conn *sql.DB, op, id string, data any) {
	t.Helper()
	raw, _ := json.Marshal(data)
	if _, err := httpapi.PreviewMCPWrite(context.Background(), conn, httpapi.MCPWriteRequest{Operation: op, ID: id, Data: raw}); err == nil {
		t.Fatalf("accepted invalid %s: %s", op, raw)
	}
}

func TestMCPAccountsNativeBalancesAndOwnership(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		data := mcpAccountData("USD", "100.1234567890123456789")
		id := planningResultID(t, applyWrite(t, conn, writePreview(t, conn, "create_manual_account", "", data)))
		data["name"] = "Synthetic dollars"
		applyWrite(t, conn, writePreview(t, conn, "update_manual_account", id, data))
		mcpBalance(t, conn, id, "100.1234567890123456789")
		mcpRejected(t, conn, "update_manual_account", id, map[string]any{"name": "Synthetic rename"})
		p := writePreview(t, conn, "adjust_manual_account", id, map[string]any{"amount": "-0.0000000000000000001", "description": "Synthetic correction"})
		applyWrite(t, conn, p)
		mcpBalance(t, conn, id, "100.1234567890123456788")
		if _, err := httpapi.ExecuteMCPWrite(ctx, conn, p); !errors.Is(err, httpapi.ErrMCPWriteConflict) {
			t.Fatalf("replayed adjustment: %v", err)
		}
		mcpCount(t, conn, "account_balance_adjustments", 1)
		data["currency_code"] = "BRL"
		mcpRejected(t, conn, "update_manual_account", id, data)
		data["currency_code"] = "USD"
		data["opening_balance"] = "0"
		mcpRejected(t, conn, "update_manual_account", id, data)
		data["opening_balance"] = "100.1234567890123456789"
		data["archived"] = true
		applyWrite(t, conn, writePreview(t, conn, "update_manual_account", id, data))
		mcpRejected(t, conn, "adjust_manual_account", id, map[string]any{"amount": "1", "description": "Synthetic"})
		data["archived"] = false
		applyWrite(t, conn, writePreview(t, conn, "update_manual_account", id, data))
		mcpBalance(t, conn, id, "100.1234567890123456788")
		// Provider balances remain authoritative and cannot be adjusted or edited.
		synced := mcpSyncedAccount(t, conn, "777")
		id = synced
		mcpRejected(t, conn, "update_manual_account", id, data)
		mcpRejected(t, conn, "adjust_manual_account", id, map[string]any{"amount": "1", "description": "Synthetic"})
		mcpBalance(t, conn, id, "777")
		for _, bad := range []map[string]any{
			mcpAccountData("XXX", "0"), mcpAccountData("usd", "0"), mcpAccountData("FAKE", "0"), mcpAccountData("BRL", "1e9"),
			{"name": "Synthetic", "opening_balance": 1}, {"name": "Synthetic", "provider_secret": "blocked"},
		} {
			mcpRejected(t, conn, "create_manual_account", "", bad)
		}
		mcpRejected(t, conn, "adjust_manual_account", uuid.NewString(), map[string]any{"amount": "1", "description": "Synthetic"})
		for _, bad := range []map[string]any{
			{"amount": "0", "description": "Synthetic"},
			{"amount": "1e9", "description": "Synthetic"},
			{"amount": 1, "description": "Synthetic"},
			{"amount": "1", "description": " "},
		} {
			mcpRejected(t, conn, "adjust_manual_account", synced, bad)
		}

		mcpCount(t, conn, "financial_accounts", 2)
	})
}

func TestMCPTransfersPreserveCashAndReportOnlyFees(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		source := planningResultID(t, applyWrite(t, conn, writePreview(t, conn, "create_manual_account", "", mcpAccountData("BRL", "1000"))))
		dest := planningResultID(t, applyWrite(t, conn, writePreview(t, conn, "create_manual_account", "", mcpAccountData("USD", "0"))))
		data := map[string]any{"source": mcpMovement(source, "-540.1234567890123456789"), "destination": mcpMovement(dest, "100.1234567890123456789"), "fee": mcpMovement(source, "-2.0000000000000000001")}
		p := writePreview(t, conn, "create_transfer", "", data)
		mcpCount(t, conn, "financial_transactions", 0)
		result := applyWrite(t, conn, p)
		var transfer transactions.AccountTransfer
		if err := json.Unmarshal(result.Data, &transfer); err != nil || transfer.Fee == nil {
			t.Fatalf("transfer: %s %v", result.Data, err)
		}
		mcpBalance(t, conn, source, "457.8765432109876543210")
		mcpBalance(t, conn, dest, "100.1234567890123456789")
		for _, id := range []string{transfer.Source.TransactionID, transfer.Destination.TransactionID} {
			item, found, err := transactions.GetItem(ctx, conn, id)
			if err != nil || !found || item.TotalsEligibility.Included || item.TotalsEligibility.Reason == nil || *item.TotalsEligibility.Reason != "linked_transfer" {
				t.Fatalf("principal counted: %+v %v", item, err)
			}
		}
		fee, found, err := transactions.GetItem(ctx, conn, transfer.Fee.TransactionID)
		if err != nil || !found || !fee.TotalsEligibility.Included || fee.Amount == nil || *fee.Amount != "-2.0000000000000000001" {
			t.Fatalf("fee: %+v %v", fee, err)
		}

		report, err := transactions.Query(ctx, conn, transactions.QueryRequest{Timezone: "UTC", GroupBy: money.GroupNone, Page: 1, PageSize: 20})
		if err != nil || len(report.Totals) != 1 || report.Totals[0].CurrencyCode != "BRL" || !decimal.RequireFromString(report.Totals[0].Inflow).IsZero() || report.Totals[0].Outflow != "2.0000000000000000001" {
			t.Fatalf("principal leaked into reporting: %+v %v", report.Totals, err)
		}
		if _, err = httpapi.ExecuteMCPWrite(ctx, conn, p); !errors.Is(err, httpapi.ErrMCPWriteConflict) {
			t.Fatalf("replayed manual transfer: %v", err)
		}
		mcpCount(t, conn, "financial_transactions", 3)
		linked := map[string]any{"source_transaction_id": transfer.Source.TransactionID, "destination_transaction_id": transfer.Destination.TransactionID, "fee_transaction_id": transfer.Fee.TransactionID}
		mcpRejected(t, conn, "create_transfer", "", linked)
		applyWrite(t, conn, writePreview(t, conn, "delete_transfer", transfer.ID, map[string]any{}))
		mcpCount(t, conn, "transfer_links", 0)
		mcpCount(t, conn, "financial_transactions", 3)
		mcpBalance(t, conn, source, "457.8765432109876543210")
		fee, _, err = transactions.GetItem(ctx, conn, transfer.Source.TransactionID)
		if err != nil || !fee.TotalsEligibility.Included {
			t.Fatalf("unlink failed to restore reporting: %+v %v", fee, err)
		}
		// Linking synced movements annotates them without changing provider cash.
		synced := mcpSyncedAccount(t, conn, "999")
		planningExec(t, conn, `UPDATE financial_transactions SET account_id=? WHERE account_id=?`, synced, source)
		source = synced
		applyWrite(t, conn, writePreview(t, conn, "create_transfer", "", linked))
		mcpBalance(t, conn, source, "999")
		mcpCount(t, conn, "financial_transactions", 3)
	})
}

func TestMCPPayablesLinksAndPlanCleanup(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		account := planningCash(t, conn, "BRL", "100")
		for _, kind := range []payables.Kind{payables.KindDebt, payables.KindReceivable} {
			data := map[string]any{"name": "Synthetic obligation", "kind": string(kind), "total_amount": "50.1234567890123456789", "initial_remaining_amount": "40.1234567890123456789"}
			id := planningResultID(t, applyWrite(t, conn, writePreview(t, conn, "create_payable", "", data)))
			planKind := scenarios.KindDebtPlan
			amount := decimal.RequireFromString("-10.1234567890123456789")
			if kind == payables.KindReceivable {
				planKind = scenarios.KindReceivablePlan
				amount = amount.Neg()
			}
			plan, err := scenarios.CreateScenario(ctx, conn, planKind, "Synthetic plan", &id)
			if err != nil {
				t.Fatal(err)
			}
			txID, err := transactions.CreateManual(ctx, conn, transactions.ManualInput{AccountID: account, Description: "Synthetic payment", Amount: amount, OccurredAt: time.Now()})
			if err != nil {
				t.Fatal(err)
			}
			linkData := map[string]any{"payable_id": id, "transaction_id": txID}
			linkID := planningResultID(t, applyWrite(t, conn, writePreview(t, conn, "create_payable_link", "", linkData)))
			mcpRejected(t, conn, "create_payable_link", "", linkData)
			mcpRejected(t, conn, "delete_payable_link", linkID, map[string]any{"payable_id": uuid.NewString()})
			item, err := payables.Get(ctx, conn, id)
			if err != nil {
				t.Fatal(err)
			}
			summary, err := payables.Summarize(ctx, conn, item)
			if err != nil || summary.Settled.String() != "20.1234567890123456789" || summary.Remaining.String() != "30" {
				t.Fatalf("summary: %+v %v", summary, err)
			}
			links, err := scenarios.ListRealizations(ctx, conn, plan.ID)
			if err != nil || len(links) != 1 {
				t.Fatalf("settlement missing: %+v %v", links, err)
			}
			applyWrite(t, conn, writePreview(t, conn, "update_payable", id, map[string]any{"name": "Synthetic updated", "total_amount": "60.1234567890123456789"}))
			applyWrite(t, conn, writePreview(t, conn, "delete_payable_link", linkID, map[string]any{"payable_id": id}))
			links, err = scenarios.ListRealizations(ctx, conn, plan.ID)
			if err != nil || len(links) != 0 {
				t.Fatalf("orphan settlement: %+v %v", links, err)
			}
			applyWrite(t, conn, writePreview(t, conn, "create_payable_link", "", linkData))
			p := writePreview(t, conn, "delete_payable", id, map[string]any{})
			if p.AffectedCount < 3 {
				t.Fatalf("missing cascade IDs: %+v", p)
			}
			applyWrite(t, conn, p)
			if _, err = scenarios.GetScenario(ctx, conn, plan.ID); !errors.Is(err, scenarios.ErrScenarioNotFound) {
				t.Fatalf("orphan plan: %v", err)
			}
			if _, found, err := transactions.GetItem(ctx, conn, txID); err != nil || !found {
				t.Fatalf("real payment deleted: %v", err)
			}
		}
		mcpCount(t, conn, "payable_transaction_links", 0)
		mcpCount(t, conn, "scenario_realizations", 0)
	})
}

func TestMCPAccountWritesRejectInvalidLinksAndRollback(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		source := planningCash(t, conn, "BRL", "1000")
		dest := planningCash(t, conn, "BRL", "0")
		foreign := planningCash(t, conn, "USD", "0")
		for _, data := range []map[string]any{
			{"source": mcpMovement(source, "-10"), "destination": mcpMovement(source, "10")},
			{"source": mcpMovement(source, "10"), "destination": mcpMovement(dest, "-10")},
			{"source": mcpMovement(source, "-10"), "destination": mcpMovement(dest, "9")},
			{"source": mcpMovement(source, "-10"), "destination": mcpMovement(dest, "10"), "fee": mcpMovement(foreign, "-1")},
			{"source": mcpMovement(source, "-10"), "destination": mcpMovement(dest, "10"), "fee": mcpMovement(source, "1")},
			{"source": mcpMovement(source, "-1e1"), "destination": mcpMovement(dest, "10")},
			{"source": mcpMovement(source, "-10"), "destination": mcpMovement(dest, "10"), "source_transaction_id": uuid.NewString()},
		} {
			mcpRejected(t, conn, "create_transfer", "", data)
		}
		data := map[string]any{"source": mcpMovement(source, "-10"), "destination": mcpMovement(dest, "10")}
		p := writePreview(t, conn, "create_transfer", "", data)
		drop := dbtest.FailInserts(t, conn, "transfer_transaction_links", "NEW.role='destination'")
		if _, err := httpapi.ExecuteMCPWrite(ctx, conn, p); err == nil {
			t.Fatal("failure not injected")
		}
		mcpCount(t, conn, "financial_transactions", 0)
		mcpCount(t, conn, "transfer_links", 0)
		mcpCount(t, conn, "transfer_transaction_links", 0)
		mcpBalance(t, conn, source, "1000")
		mcpBalance(t, conn, dest, "0")
		drop()
		applyWrite(t, conn, p)
		debt := planningResultID(t, applyWrite(t, conn, writePreview(t, conn, "create_payable", "", map[string]any{"name": "Synthetic debt", "kind": "debt", "total_amount": "50"})))
		plan, err := scenarios.CreateScenario(ctx, conn, scenarios.KindDebtPlan, "Synthetic", &debt)
		if err != nil {
			t.Fatal(err)
		}
		create := func(account, amount string) string {
			id, err := transactions.CreateManual(ctx, conn, transactions.ManualInput{AccountID: account, Description: "Synthetic", Amount: decimal.RequireFromString(amount), OccurredAt: time.Now()})
			if err != nil {
				t.Fatal(err)
			}
			return id
		}
		for _, id := range []string{create(dest, "10"), create(foreign, "-10"), uuid.NewString()} {
			mcpRejected(t, conn, "create_payable_link", "", map[string]any{"payable_id": debt, "transaction_id": id})
		}
		ignored := create(source, "-10")
		if _, err = transactions.SetInclusion(ctx, conn, ignored, money.Ignored, transactions.InclusionOriginManual, nil, nil, nil); err != nil {
			t.Fatal(err)
		}
		mcpRejected(t, conn, "create_payable_link", "", map[string]any{"payable_id": debt, "transaction_id": ignored})
		payment := create(source, "-10")
		p = writePreview(t, conn, "create_payable_link", "", map[string]any{"payable_id": debt, "transaction_id": payment})
		drop = dbtest.FailInserts(t, conn, "scenario_realizations", "")
		if _, err = httpapi.ExecuteMCPWrite(ctx, conn, p); err == nil {
			t.Fatal("settlement failure not injected")
		}
		mcpCount(t, conn, "payable_transaction_links", 0)
		drop()
		applyWrite(t, conn, p)
		p = writePreview(t, conn, "delete_payable", debt, map[string]any{})
		_, err = scenarios.CreateScenarioTransaction(ctx, conn, plan.ID, "Synthetic new installment", decimal.NewFromInt(3), time.Now(), nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = httpapi.ExecuteMCPWrite(ctx, conn, p); !errors.Is(err, httpapi.ErrMCPWriteConflict) {
			t.Fatalf("stale cascade accepted: %v", err)
		}
		for _, bad := range []map[string]any{
			{"name": "Synthetic", "kind": "unknown", "total_amount": "10"},
			{"name": "Synthetic", "kind": "debt", "total_amount": 10},
			{"name": "Synthetic", "kind": "debt", "total_amount": "1e9"},
			{"name": "Synthetic", "kind": "debt", "total_amount": "10", "initial_remaining_amount": "11"},
			{"name": "Synthetic", "kind": "debt", "total_amount": "10", "initial_remaining_amount": "-1"},
		} {
			mcpRejected(t, conn, "create_payable", "", bad)
		}
	})
}

func TestMCPAccountsApprovalAndMutationShareTransaction(t *testing.T) {
	ctx := context.Background()
	conn := dbtest.OpenSQLite(t)
	account := planningCash(t, conn, "BRL", "100")
	p := writePreview(t, conn, "adjust_manual_account", account, map[string]any{"amount": "0.01", "description": "Synthetic"})
	var seq int
	var name, path string
	if err := conn.QueryRow("PRAGMA database_list").Scan(&seq, &name, &path); err != nil {
		t.Fatal(err)
	}
	interceptor := &planningAtomicConnector{backend: conn.Driver(), path: path}
	guarded := sql.OpenDB(interceptor)
	guarded.SetMaxOpenConns(1)
	defer guarded.Close()
	if result, err := httpapi.ExecuteMCPWrite(ctx, guarded, p); err != nil || result.Status >= 400 {
		t.Fatalf("atomic execution: %+v %v", result, err)
	}
	if interceptor.begins != 1 {
		t.Fatalf("expected single transaction: %d", interceptor.begins)
	}
	mcpBalance(t, conn, account, "100.01")
	// An account change between review and approval invalidates the adjustment.
	p = writePreview(t, conn, "adjust_manual_account", account, map[string]any{"amount": "1", "description": "Synthetic"})

	planningExec(t, conn, `UPDATE financial_accounts SET name='Changed' WHERE id=?`, account)
	if _, err := httpapi.ExecuteMCPWrite(ctx, conn, p); !errors.Is(err, httpapi.ErrMCPWriteConflict) {
		t.Fatalf("stale account: %v", err)
	}
}

func mcpSyncedAccount(t *testing.T, conn *sql.DB, balance string) string {
	t.Helper()
	seed := accountSeed{uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()}
	now := db.FormatTime(time.Now())
	planningExec(t, conn, `INSERT INTO data_sources(id,provider,external_item_id,created_at,updated_at) VALUES(?,'pluggy',?,?,?)`, seed.sourceID, seed.sourceID, now, now)
	planningExec(t, conn, `INSERT INTO sync_runs(id,source_id,status,started_at,finished_at) VALUES(?,?,'completed',?,?)`, seed.syncRunID, seed.sourceID, now, now)
	planningExec(t, conn, `INSERT INTO raw_imports(id,sync_run_id,source_id,scope,page_sequence,request_attempt,request_method,request_path,http_status,response_headers,payload,payload_sha256,received_at) VALUES(?,?,?,'accounts',1,1,'GET','/x',200,'{}',?,'sha',?)`, seed.rawImportID, seed.syncRunID, seed.sourceID, []byte{}, now)
	planningExec(t, conn, `INSERT INTO financial_accounts(id,source_id,external_id,name,account_type,currency_code,balance,current_raw_import_id,normalized_hash,created_at,updated_at) VALUES(?,?,?,'Synthetic synced','BANK','BRL',?,?,'hash',?,?)`, seed.accountID, seed.sourceID, seed.accountID, balance, seed.rawImportID, now, now)
	return seed.accountID
}
