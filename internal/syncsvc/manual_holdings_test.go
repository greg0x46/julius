package syncsvc_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/investments"
	"github.com/greg0x46/julius/internal/pluggy"
	"github.com/greg0x46/julius/internal/syncsvc"
)

// A sync rewrites provider holdings and their quotes, never the manual
// holdings the user keeps in the same integrated custody.
func TestExecutePreservesManualHoldingsInIntegratedAccount(t *testing.T) {
	ctx := context.Background()
	conn := newTestConn(t)
	sourceID, syncRunID1 := newSyncRun(t, conn)
	insertRawImport(t, conn, "raw-accounts", syncRunID1, sourceID)
	insertRawImport(t, conn, "raw-investments", syncRunID1, sourceID)
	insertRawImport(t, conn, "raw-invtx", syncRunID1, sourceID)

	day1 := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	snapshot := pluggy.InvestmentSnapshot{
		ExternalID: "inv-1", InvestmentType: strp("EQUITY"), Name: strp("Bitcoin"), Code: strp("BTC"),
		Balance: amountP("400"), Amount: amountP("400"), Quantity: amountP("2"), CurrencyCode: strp("BRL"), AsOfDate: &day1,
	}
	provider := &fakeProvider{
		source:          investmentSafeSource(),
		accountsPage:    pluggy.AccountsPage{RawImportID: "raw-accounts"},
		investmentsPage: pluggy.InvestmentsPage{RawImportID: "raw-investments", Investments: []pluggy.InvestmentSnapshot{snapshot}},
		investmentTransactions: map[string]pluggy.InvestmentTransactionsPage{
			"inv-1": {RawImportID: "raw-invtx"},
		},
	}
	if err := (&syncsvc.Service{DB: conn, Provider: provider, SyncRunID: syncRunID1, SourceID: sourceID}).Execute(ctx); err != nil {
		t.Fatalf("first Execute: %v", err)
	}
	positions, err := investments.ListPositions(ctx, conn, investments.PositionFilter{})
	if err != nil || len(positions) != 1 {
		t.Fatalf("positions: %+v %v", positions, err)
	}
	synced := positions[0]
	integrated := synced.AccountID

	dec := func(s string) decimal.Decimal { return decimal.RequireFromString(s) }
	ptr := func(s string) *decimal.Decimal { v := dec(s); return &v }
	goal, err := investments.CreatePortfolio(ctx, conn, investments.PortfolioInput{Name: "Cripto"})
	if err != nil {
		t.Fatal(err)
	}
	ether, err := investments.CreatePosition(ctx, conn, investments.PositionInput{
		AccountID: integrated, Name: "Ethereum", Ticker: strp("ETH"), AssetType: "Criptoativo", PortfolioID: &goal.ID,
		InitialQuantity: dec("1"), InitialUnitCost: dec("100"), OccurredOn: day1,
	})
	if err != nil {
		t.Fatalf("CreatePosition: %v", err)
	}
	for _, in := range []investments.OperationInput{
		{AccountID: integrated, PositionID: &ether.ID, Kind: investments.OperationBuy, OccurredOn: day1, Amount: dec("120"), Quantity: ptr("1")},
		{AccountID: integrated, PositionID: &ether.ID, Kind: investments.OperationValuation, OccurredOn: day2, Amount: dec("260")},
	} {
		if _, err := investments.CreateOperation(ctx, conn, in); err != nil {
			t.Fatalf("CreateOperation %s: %v", in.Kind, err)
		}
	}
	// A manual holding of the provider's own asset, priced by its quotes.
	if _, err := conn.Exec(`UPDATE investment_assets SET quote_source = 'crypto', quote_symbol = 'BTC' WHERE id = ?`, *synced.AssetID); err != nil {
		t.Fatal(err)
	}
	shared, err := investments.CreatePosition(ctx, conn, investments.PositionInput{
		AccountID: integrated, AssetID: synced.AssetID, Name: synced.Name, AssetType: synced.AssetType,
		InitialQuantity: dec("0.5"), InitialUnitCost: dec("150"), OccurredOn: day1,
	})
	if err != nil {
		t.Fatalf("CreatePosition shared: %v", err)
	}
	if got, _ := investments.GetPosition(ctx, conn, shared.ID); got.ValuationBasis != investments.ValuationBasisMarketQuote || got.CurrentValue.String() != "100" {
		t.Fatalf("shared before sync = %+v", got)
	}
	before, err := investments.GetPosition(ctx, conn, ether.ID)
	if err != nil {
		t.Fatal(err)
	}
	operationsBefore, err := investments.ListOperations(ctx, conn, investments.OperationFilter{AccountID: &integrated})
	if err != nil {
		t.Fatal(err)
	}

	provider.investmentsPage.Investments[0].Balance = amountP("500")
	provider.investmentsPage.Investments[0].Amount = amountP("500")
	provider.investmentsPage.Investments[0].AsOfDate = &day2
	syncRunID2 := uuid.NewString()
	if _, err := conn.Exec(`INSERT INTO sync_runs (id, source_id, status, started_at) VALUES (?, ?, 'in_progress', ?)`,
		syncRunID2, sourceID, db.FormatTime(time.Now())); err != nil {
		t.Fatal(err)
	}
	if err := (&syncsvc.Service{DB: conn, Provider: provider, SyncRunID: syncRunID2, SourceID: sourceID}).Execute(ctx); err != nil {
		t.Fatalf("second Execute: %v", err)
	}
	if _, err := investments.PruneUnusedSyncedAssets(ctx, conn); err != nil {
		t.Fatal(err)
	}

	after, err := investments.GetPosition(ctx, conn, ether.ID)
	if err != nil {
		t.Fatalf("manual holding lost: %v", err)
	}
	if !reflect.DeepEqual(before, after) || after.Source != investments.PositionSourceManual ||
		after.PortfolioID == nil || *after.PortfolioID != goal.ID {
		t.Fatalf("manual holding changed:\nbefore %+v\nafter  %+v", before, after)
	}
	operationsAfter, err := investments.ListOperations(ctx, conn, investments.OperationFilter{AccountID: &integrated})
	if err != nil || !reflect.DeepEqual(operationsBefore, operationsAfter) {
		t.Fatalf("operations changed:\nbefore %+v\nafter  %+v (%v)", operationsBefore, operationsAfter, err)
	}
	if _, err := investments.GetAsset(ctx, conn, *ether.AssetID); err != nil {
		t.Fatalf("manual asset pruned: %v", err)
	}
	if got, _ := investments.GetPosition(ctx, conn, synced.ID); got.CurrentValue.String() != "500" {
		t.Fatalf("provider holding = %+v", got)
	}
	got, err := investments.GetPosition(ctx, conn, shared.ID)
	if err != nil || got.Source != investments.PositionSourceManual || got.Quantity.String() != "0.5" ||
		got.TotalCost == nil || got.TotalCost.String() != "75" {
		t.Fatalf("shared after sync = %+v %v", got, err)
	}
	if got.ValuationBasis != investments.ValuationBasisMarketQuote || got.CurrentValue.String() != "125" {
		t.Fatalf("shared valuation = %+v, want the latest synced quote", got)
	}
}
