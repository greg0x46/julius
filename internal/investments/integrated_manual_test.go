package investments_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/investments"
)

func (f *bookFixture) integratedAccountID() string {
	f.t.Helper()
	// Listing materializes the integrated grouping, as the UI does before
	// offering it.
	f.positions(investments.PositionFilter{})
	return "integrated:" + f.sourceID
}

func (f *bookFixture) openBitcoin(accountID, quantity, unitCost string) investments.Position {
	f.t.Helper()
	ticker := "BTC"
	position, err := investments.CreatePosition(context.Background(), f.conn, investments.PositionInput{
		AccountID: accountID, Name: "Bitcoin", Ticker: &ticker, AssetType: "Criptoativo",
		InitialQuantity: bookDec(quantity), InitialUnitCost: bookDec(unitCost),
		OccurredOn: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		f.t.Fatalf("CreatePosition: %v", err)
	}
	return position
}

func (f *bookFixture) integratedCash(accountID string) string {
	f.t.Helper()
	account, err := investments.GetAccount(context.Background(), f.conn, accountID)
	if err != nil {
		f.t.Fatalf("GetAccount: %v", err)
	}
	return account.CashBalance.String()
}

func TestManualHoldingInIntegratedAccount(t *testing.T) {
	f := newBookFixture(t)
	ctx := context.Background()
	syncedID := f.addFinancialInvestment("2000", "4", "500")
	integrated := f.integratedAccountID()

	manual := f.openBitcoin(integrated, "0.5", "100000")
	if manual.Source != investments.PositionSourceManual || manual.AccountID != integrated {
		t.Fatalf("manual = %+v", manual)
	}
	if manual.Quantity.String() != "0.5" || bookDecimalString(manual.TotalCost) != "50000" ||
		bookDecimalString(manual.AverageCost) != "100000" || manual.CurrentValue.String() != "50000" {
		t.Fatalf("manual values = %+v", manual)
	}

	list := f.positions(investments.PositionFilter{AccountID: &integrated})
	sources := map[string]investments.PositionSource{}
	for _, p := range list {
		sources[p.ID] = p.Source
	}
	if len(list) != 2 || sources[manual.ID] != investments.PositionSourceManual || sources[syncedID] != investments.PositionSourceSynced {
		t.Fatalf("positions = %+v", list)
	}

	summary := f.summary()
	if summary.ManualValue.String() != "50000" || summary.SyncedValue.String() != "2000" || summary.TotalValue.String() != "52000" {
		t.Fatalf("summary = %+v", summary)
	}
	found := false
	for _, account := range summary.Accounts {
		if account.AccountID == integrated {
			found = true
			if account.CurrentValue.String() != "52000" || !account.CashBalance.IsZero() {
				t.Fatalf("integrated account = %+v", account)
			}
		}
	}
	if !found {
		t.Fatalf("integrated account missing: %+v", summary.Accounts)
	}
	netWorth, err := investments.ManualNetWorth(ctx, f.conn)
	if err != nil || netWorth.String() != "50000" {
		t.Fatalf("ManualNetWorth = %v err=%v", netWorth, err)
	}
}

func TestManualHoldingInIntegratedAccountUsesMarketQuote(t *testing.T) {
	f := newBookFixture(t)
	f.addFinancialInvestment("2000", "4", "500")
	asset := f.quotedAsset("Bitcoin", "BTC")
	position := f.openQuoted(f.integratedAccountID(), asset, "0.5", "100000")
	f.quoteOn(asset.ID, "2026-09-10", "120000")

	got := f.position(position.ID)
	if got.ValuationBasis != investments.ValuationBasisMarketQuote || got.CurrentValue.String() != "60000" ||
		bookDecimalString(got.TotalCost) != "50000" {
		t.Fatalf("quoted = %+v", got)
	}
}

func TestEditAndDeleteManualHoldingLeavesSyncedUntouched(t *testing.T) {
	f := newBookFixture(t)
	ctx := context.Background()
	syncedID := f.addFinancialInvestment("2000", "4", "500")
	integrated := f.integratedAccountID()
	before := f.position(syncedID)

	manual := f.openBitcoin(integrated, "1", "100")
	goal, err := investments.CreatePortfolio(ctx, f.conn, investments.PortfolioInput{Name: "Cripto"})
	if err != nil {
		t.Fatal(err)
	}
	notes := "Carteira fria"
	if _, err := investments.UpdatePosition(ctx, f.conn, manual.ID, investments.PositionUpdate{
		Name: manual.Name, Ticker: manual.Ticker, AssetType: manual.AssetType, PortfolioID: &goal.ID, Notes: &notes,
	}); err != nil {
		t.Fatalf("UpdatePosition: %v", err)
	}

	day := func(d int) time.Time { return time.Date(2026, 9, d, 0, 0, 0, 0, time.UTC) }
	buy, err := investments.CreateOperation(ctx, f.conn, investments.OperationInput{
		AccountID: integrated, PositionID: &manual.ID, Kind: investments.OperationBuy, OccurredOn: day(2),
		Amount: bookDec("150"), Quantity: bookDecPtr("1"), Fees: bookDec("1"),
	})
	if err != nil {
		t.Fatalf("buy: %v", err)
	}
	// The trade is cash-neutral locally: the provider owns the custody cash.
	if cash := f.integratedCash(integrated); cash != "0" {
		t.Fatalf("integrated cash after buy = %s", cash)
	}
	if got := f.position(manual.ID); got.Quantity.String() != "2" || bookDecimalString(got.TotalCost) != "251" {
		t.Fatalf("after buy = %+v", got)
	}
	if _, err := investments.CreateOperation(ctx, f.conn, investments.OperationInput{
		AccountID: integrated, PositionID: &manual.ID, Kind: investments.OperationSell, OccurredOn: day(3),
		Amount: bookDec("1000"), Quantity: bookDecPtr("5"),
	}); !errors.Is(err, investments.ErrNegativePosition) {
		t.Fatalf("oversell = %v", err)
	}
	if _, err := investments.CreateOperation(ctx, f.conn, investments.OperationInput{
		AccountID: integrated, PositionID: &manual.ID, Kind: investments.OperationSell, OccurredOn: day(3),
		Amount: bookDec("100"), Quantity: bookDecPtr("0.5"),
	}); err != nil {
		t.Fatalf("sell: %v", err)
	}
	if _, err := investments.CreateOperation(ctx, f.conn, investments.OperationInput{
		AccountID: integrated, PositionID: &manual.ID, Kind: investments.OperationValuation, OccurredOn: day(4),
		Amount: bookDec("300"),
	}); err != nil {
		t.Fatalf("valuation: %v", err)
	}
	if got := f.position(manual.ID); got.Quantity.String() != "1.5" || got.CurrentValue.String() != "300" ||
		got.ValuationBasis != investments.ValuationBasisManualValuation {
		t.Fatalf("after valuation = %+v", got)
	}
	if _, err := investments.UpdateOperation(ctx, f.conn, buy.ID, investments.OperationInput{
		AccountID: integrated, PositionID: &manual.ID, Kind: investments.OperationBuy, OccurredOn: day(2),
		Amount: bookDec("160"), Quantity: bookDecPtr("1"),
	}); err != nil {
		t.Fatalf("UpdateOperation: %v", err)
	}
	if got := f.position(manual.ID); bookDecimalString(got.TotalCost) != "195" {
		t.Fatalf("after edit = %+v", got)
	}
	if after := f.position(syncedID); !reflect.DeepEqual(before, after) {
		t.Fatalf("synced holding changed:\nbefore %+v\nafter  %+v", before, after)
	}
	if cash := f.integratedCash(integrated); cash != "0" {
		t.Fatalf("integrated cash = %s", cash)
	}

	operations, err := investments.ListOperations(ctx, f.conn, investments.OperationFilter{PositionID: &manual.ID})
	if err != nil {
		t.Fatal(err)
	}
	// Newest first, so no intermediate replay ever sells more than it holds.
	for i := len(operations) - 1; i >= 0; i-- {
		if err := investments.DeleteOperation(ctx, f.conn, operations[i].ID); err != nil {
			t.Fatalf("DeleteOperation %s: %v", operations[i].Kind, err)
		}
	}
	if err := investments.DeletePosition(ctx, f.conn, manual.ID); err != nil {
		t.Fatalf("DeletePosition: %v", err)
	}
	list := f.positions(investments.PositionFilter{AccountID: &integrated})
	if len(list) != 1 || !reflect.DeepEqual(list[0], before) {
		t.Fatalf("after delete = %+v", list)
	}
}

func TestIntegratedAccountOperationRules(t *testing.T) {
	f := newBookFixture(t)
	ctx := context.Background()
	f.addFinancialInvestment("2000", "4", "500")
	integrated := f.integratedAccountID()
	manualAccount := f.addManualAccount("Corretora")
	held := f.openBitcoin(integrated, "1", "100")
	other := f.openBitcoin(manualAccount, "1", "100")
	day := time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)

	if _, err := investments.CreateTransfer(ctx, f.conn, investments.TransferInput{
		SourcePositionID: other.ID, DestinationPositionID: held.ID, Quantity: bookDec("0.5"), OccurredOn: day,
	}); !errors.Is(err, investments.ErrIntegratedReadOnly) {
		t.Fatalf("transfer into integrated = %v", err)
	}
	if _, err := investments.CreateOperation(ctx, f.conn, investments.OperationInput{
		AccountID: integrated, PositionID: &held.ID, Kind: investments.OperationDeposit, OccurredOn: day, Amount: bookDec("10"),
	}); !errors.Is(err, investments.ErrIntegratedReadOnly) {
		t.Fatalf("deposit with position = %v", err)
	}
	if _, err := investments.CreateOperation(ctx, f.conn, investments.OperationInput{
		AccountID: integrated, Kind: investments.OperationInitialBalance, OccurredOn: day, Amount: bookDec("10"),
	}); !errors.Is(err, investments.ErrIntegratedReadOnly) {
		t.Fatalf("cash initial balance = %v", err)
	}
	if _, err := investments.CreateOperation(ctx, f.conn, investments.OperationInput{
		AccountID: integrated, Kind: investments.OperationIncome, OccurredOn: day, Amount: bookDec("10"),
	}); err != nil {
		t.Fatalf("income note: %v", err)
	}
	if got := f.position(held.ID); got.Quantity.String() != "1" || bookDecimalString(got.TotalCost) != "100" {
		t.Fatalf("income moved the holding: %+v", got)
	}
	if cash := f.integratedCash(integrated); cash != "0" {
		t.Fatalf("integrated cash = %s", cash)
	}
}
