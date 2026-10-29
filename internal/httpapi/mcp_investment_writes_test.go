package httpapi_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/httpapi"
	"github.com/greg0x46/julius/internal/investments"
)

func investmentWriteID(t *testing.T, conn *sql.DB, operation, id string, data any) string {
	t.Helper()
	result := applyWrite(t, conn, writePreview(t, conn, operation, id, data))
	if result.Status == 204 {
		return ""
	}
	var item struct{ ID string }
	if err := json.Unmarshal(result.Data, &item); err != nil || item.ID == "" {
		t.Fatalf("result: %s %v", result.Data, err)
	}
	return item.ID
}

func TestMCPInvestmentLedgerRollbackAndConflict(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		account := investmentWriteID(t, conn, "create_investment_account", "", map[string]any{"name": "Synthetic"})
		position := investmentWriteID(t, conn, "create_investment_position", "", map[string]any{"account_id": account, "name": "Synthetic asset", "occurred_on": "2026-09-01"})
		depositData := map[string]any{"account_id": account, "kind": "deposit", "amount": "100", "occurred_on": "2026-09-01"}
		deposit := investmentWriteID(t, conn, "create_investment_operation", "", depositData)
		buyData := map[string]any{"account_id": account, "position_id": position, "kind": "buy", "amount": "90", "quantity": "3", "occurred_on": "2026-09-02"}
		buy := investmentWriteID(t, conn, "create_investment_operation", "", buyData)
		snapshot := func() string {
			ops, err := investments.ListOperations(ctx, conn, investments.OperationFilter{})
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(ops)
			return string(raw)
		}
		before := snapshot()
		for _, tc := range []struct {
			operation, id string
			data          any
			want          error
		}{
			{"delete_investment_operation", deposit, map[string]any{}, investments.ErrNegativeCash},
			{"update_investment_operation", deposit, map[string]any{"account_id": account, "kind": "deposit", "amount": "10", "occurred_on": "2026-09-01"}, investments.ErrNegativeCash},
			{"create_investment_operation", "", map[string]any{"account_id": account, "position_id": position, "kind": "sell", "amount": "150", "quantity": "4", "occurred_on": "2026-09-03"}, investments.ErrNegativePosition},
			{"delete_investment_position", position, map[string]any{}, investments.ErrPositionHasOperations},
			{"delete_investment_account", account, map[string]any{}, investments.ErrAccountHasPositions},
		} {
			p := writePreview(t, conn, tc.operation, tc.id, tc.data)
			if _, err := httpapi.ExecuteMCPWrite(ctx, conn, p); !errors.Is(err, tc.want) {
				t.Fatalf("%s: %v, want %v", tc.operation, err, tc.want)
			}
			if snapshot() != before {
				t.Fatalf("partial write after %s", tc.operation)
			}
		}
		pending := writePreview(t, conn, "delete_investment_operation", buy, map[string]any{})
		buyData["amount"] = "80"
		investmentWriteID(t, conn, "update_investment_operation", buy, buyData)
		if _, err := httpapi.ExecuteMCPWrite(ctx, conn, pending); !errors.Is(err, httpapi.ErrMCPWriteConflict) {
			t.Fatalf("stale approval: %v", err)
		}
		pending = writePreview(t, conn, "create_investment_operation", "", map[string]any{"account_id": account, "kind": "fee", "amount": "1", "occurred_on": "2026-09-04"})
		before = snapshot()
		drop := dbtest.FailInserts(t, conn, "investment_operations", "1=1")
		if _, err := httpapi.ExecuteMCPWrite(ctx, conn, pending); err == nil {
			t.Fatal("injected failure accepted")
		}
		drop()
		if snapshot() != before {
			t.Fatal("failed insert changed ledger")
		}
		applyWrite(t, conn, pending)
		if _, err := httpapi.ExecuteMCPWrite(ctx, conn, pending); !errors.Is(err, httpapi.ErrMCPWriteConflict) {
			t.Fatalf("repeat create duplicated effect: %v", err)
		}
	})
}

func TestMCPInvestmentProviderOwnership(t *testing.T) {
	_, conn := newTestServer(t)
	ctx := context.Background()
	synced := insertInvestment(t, conn)
	integrated := integratedAccountID(t, conn, synced)
	if _, err := investments.ListAccounts(ctx, conn); err != nil {
		t.Fatal(err)
	}
	reject := func(operation, id string, data any) {
		t.Helper()
		raw, _ := json.Marshal(data)
		p, err := httpapi.PreviewMCPWrite(ctx, conn, httpapi.MCPWriteRequest{Operation: operation, ID: id, Data: raw})
		if err == nil {
			_, err = httpapi.ExecuteMCPWrite(ctx, conn, p)
		}
		if err == nil {
			t.Fatalf("provider-owned write accepted: %s", operation)
		}
	}
	reject("update_investment_position", synced, map[string]any{"name": "Synthetic replacement"})
	reject("delete_investment_position", synced, map[string]any{})
	reject("create_investment_operation", "", map[string]any{"account_id": integrated, "position_id": synced, "kind": "buy", "amount": "10", "quantity": "1", "occurred_on": "2026-09-01"})
	reject("update_investment_account", integrated, map[string]any{"name": "Synthetic replacement", "active": false})
	reject("delete_investment_account", integrated, map[string]any{})
	// A local holding in the same custody remains editable and never moves provider cash.
	local := investmentWriteID(t, conn, "create_investment_position", "", map[string]any{"account_id": integrated, "name": "Synthetic local", "occurred_on": "2026-09-01"})
	investmentWriteID(t, conn, "update_investment_position", local, map[string]any{"notes": "Synthetic note"})
	buy := investmentWriteID(t, conn, "create_investment_operation", "", map[string]any{"account_id": integrated, "position_id": local, "kind": "buy", "quantity": "0.000000000000000001", "unit_price": "10", "fees": "0.000000000000000001", "taxes": "0.000000000000000002", "occurred_on": "2026-09-02"})
	pos, err := investments.GetPosition(ctx, conn, local)
	if err != nil || pos.TotalCost == nil || pos.TotalCost.String() != "0.000000000000000013" {
		t.Fatalf("precision: %+v %v", pos, err)
	}
	account, err := investments.GetAccount(ctx, conn, integrated)
	if err != nil || !account.CashBalance.IsZero() {
		t.Fatalf("provider cash changed: %+v %v", account, err)
	}
	provider, err := investments.GetPosition(ctx, conn, synced)
	if err != nil || provider.CurrentValue.String() != "1000.5" {
		t.Fatalf("provider position changed: %+v %v", provider, err)
	}
	// Synced operations are reconciliation pivots and must remain immutable.
	planningExec(t, conn, `UPDATE investment_operations SET source='synced' WHERE id=?`, buy)
	reject("update_investment_operation", buy, map[string]any{"account_id": integrated, "kind": "income", "amount": "1", "occurred_on": "2026-09-02"})
	reject("delete_investment_operation", buy, map[string]any{})
	planningExec(t, conn, `UPDATE investment_operations SET source='manual' WHERE id=?`, buy)
	investmentWriteID(t, conn, "delete_investment_operation", buy, map[string]any{})
	investmentWriteID(t, conn, "delete_investment_position", local, map[string]any{})
}

func TestMCPInvestmentForeignCurrencyPreserved(t *testing.T) {
	_, conn := newTestServer(t)
	ctx := context.Background()
	account := investmentWriteID(t, conn, "create_investment_account", "", map[string]any{"name": "Synthetic"})
	pos := investmentWriteID(t, conn, "create_investment_position", "", map[string]any{"account_id": account, "name": "Synthetic foreign asset", "occurred_on": "2026-09-01"})
	current, err := investments.GetPosition(ctx, conn, pos)
	if err != nil {
		t.Fatal(err)
	}
	investmentWriteID(t, conn, "delete_investment_position", pos, map[string]any{})
	planningExec(t, conn, `UPDATE investment_assets SET currency_code='USD' WHERE id=?`, *current.AssetID)
	preview := writePreview(t, conn, "create_investment_position", "", map[string]any{"account_id": account, "asset_id": *current.AssetID, "occurred_on": "2026-09-01"})
	if _, err = httpapi.ExecuteMCPWrite(ctx, conn, preview); !errors.Is(err, investments.ErrUnsupportedCurrency) {
		t.Fatalf("foreign manual holding accepted: %v", err)
	}
	asset, err := investments.GetAsset(ctx, conn, *current.AssetID)
	if err != nil || asset.CurrencyCode != "USD" {
		t.Fatalf("currency rewritten: %+v %v", asset, err)
	}
}

func TestMCPInvestmentApprovalRecheckIsAtomic(t *testing.T) {
	ctx := context.Background()
	conn := dbtest.OpenSQLite(t)
	account := investmentWriteID(t, conn, "create_investment_account", "", map[string]any{"name": "Synthetic"})
	p := writePreview(t, conn, "create_investment_operation", "", map[string]any{"account_id": account, "kind": "deposit", "amount": "1.000000000000000001", "occurred_on": "2026-09-01"})
	var seq int
	var name, path string
	if err := conn.QueryRowContext(ctx, "PRAGMA database_list").Scan(&seq, &name, &path); err != nil {
		t.Fatal(err)
	}
	interceptor := &planningAtomicConnector{backend: conn.Driver(), path: path}
	guarded := sql.OpenDB(interceptor)
	guarded.SetMaxOpenConns(1)
	defer guarded.Close()
	applyWrite(t, guarded, p)
	if interceptor.begins != 1 {
		t.Fatalf("approval and write used %d transactions", interceptor.begins)
	}
	// A wrong account/position pair cannot leave an operation behind.
	raw, _ := json.Marshal(map[string]any{"account_id": account, "position_id": uuid.NewString(), "kind": "buy", "quantity": "1", "amount": "1", "occurred_on": "2026-09-02"})
	if _, err := httpapi.PreviewMCPWrite(ctx, conn, httpapi.MCPWriteRequest{Operation: "create_investment_operation", Data: raw}); err == nil {
		t.Fatal("missing position accepted")
	}
}
