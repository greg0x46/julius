package investments_test

import (
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/investments"
)

func valuesOn(t *testing.T, f *ledgerFixture, days ...time.Time) []investments.DayValues {
	t.Helper()
	values, err := investments.ValuesOn(f.ctx, f.conn, days)
	if err != nil {
		t.Fatalf("ValuesOn: %v", err)
	}
	if len(values) != len(days) {
		t.Fatalf("ValuesOn returned %d days, want %d", len(values), len(days))
	}
	return values
}

func wantBRL(t *testing.T, label string, got investments.DayValues, want string) {
	t.Helper()
	if !got.Complete || !got.Balances.Get("BRL").Equal(dec(want)) {
		t.Errorf("%s = %s (complete %v), want %s", label, got.Balances.Get("BRL"), got.Complete, want)
	}
}

// TestValuesOnFollowsTheLedgerDayByDay: each day is the book as it stood then
// (cost until a valuation, cash after a deposit), and the last day agrees with
// today's figure because both apply the same rules.
func TestValuesOnFollowsTheLedgerDayByDay(t *testing.T) {
	f := newLedgerFixture(t)
	position := f.addPosition(f.custodyID, "Ações")
	f.deposit("1000", day(1))
	f.create(investments.OperationInput{
		AccountID: f.custodyID, PositionID: &position, Kind: investments.OperationBuy,
		OccurredOn: day(2), Amount: dec("600"), Quantity: decRef("6"), UnitPrice: decRef("100"),
	})
	f.create(investments.OperationInput{
		AccountID: f.custodyID, PositionID: &position, Kind: investments.OperationValuation,
		OccurredOn: day(10), Amount: dec("660"),
	})

	got := valuesOn(t, f, day(1), day(2), day(9), day(10), day(12))
	wantBRL(t, "deposit day (cash)", got[0], "1000")
	wantBRL(t, "buy day (cost 600 + cash 400)", got[1], "1000")
	wantBRL(t, "carried at cost", got[2], "1000")
	wantBRL(t, "valuation day (660 + cash 400)", got[3], "1060")
	wantBRL(t, "after the valuation", got[4], "1060")

	today, err := investments.ManualNetWorth(f.ctx, f.conn)
	if err != nil {
		t.Fatal(err)
	}
	if !got[4].Balances.Get("BRL").Equal(today) {
		t.Errorf("last day %s differs from ManualNetWorth %s", got[4].Balances.Get("BRL"), today)
	}

	// Nothing is stored: a corrected valuation moves every day after it.
	f.exec(`UPDATE investment_operations SET amount = '700' WHERE kind = 'valuation'`)
	corrected := valuesOn(t, f, day(9), day(12))
	wantBRL(t, "before the valuation is untouched", corrected[0], "1000")
	wantBRL(t, "after the corrected valuation", corrected[1], "1100")
}

func TestValuesOnPricesAQuotedAssetWithTheLatestQuoteOfThatDay(t *testing.T) {
	f := newLedgerFixture(t)
	position := f.addPosition(f.custodyID, "Ações")
	asset := f.quoteFrom(position, "b3", "ACAO3")
	f.deposit("1000", day(1))
	f.create(investments.OperationInput{
		AccountID: f.custodyID, PositionID: &position, Kind: investments.OperationBuy,
		OccurredOn: day(2), Amount: dec("600"), Quantity: decRef("6"), UnitPrice: decRef("100"),
	})
	f.quote(asset, "yahoo", day(3), "110")
	f.quote(asset, "yahoo", day(5), "120")

	got := valuesOn(t, f, day(2), day(3), day(4), day(5))
	wantBRL(t, "before any quote (cost 600 + cash 400)", got[0], "1000")
	wantBRL(t, "6 x 110 + cash 400", got[1], "1060")
	wantBRL(t, "the quote carries to the next day", got[2], "1060")
	wantBRL(t, "6 x 120 + cash 400", got[3], "1120")
}

// A custody linked to a bank account has no cash of its own: that cash is the
// bank account's balance, already counted as cash.
func TestValuesOnCountsLinkedCustodyCashOnlyAsBankCash(t *testing.T) {
	f := newLedgerFixture(t)
	f.deposit("1000", day(1))
	wantBRL(t, "unlinked", valuesOn(t, f, day(1))[0], "1000")
	f.exec(`UPDATE investment_accounts SET financial_account_id = ? WHERE id = ?`, f.bankID, f.custodyID)
	wantBRL(t, "linked", valuesOn(t, f, day(1))[0], "0")
}

func TestValuesOnPricesProviderHoldingsAndFlagsDaysWithoutAPrice(t *testing.T) {
	f := newLedgerFixture(t)
	holding := lca()
	id := f.addHolding(holding)
	f.exec(`UPDATE financial_investments SET balance = '258.6' WHERE id = ?`, id)
	f.recordQuote(holding, "2026-08-31T06:25:30Z", "256.29")
	f.recordQuote(holding, "2026-09-28T00:03:22Z", "258.6")

	got := valuesOn(t, f, calendarDay("2024-09-01"), calendarDay("2026-07-01"), calendarDay("2026-08-31"), calendarDay("2026-09-20"), calendarDay("2026-09-28"))
	wantBRL(t, "before the purchase", got[0], "0")
	wantBRL(t, "carried at the purchase price before the first sync", got[1], "200")
	wantBRL(t, "quote day", got[2], "256.29")
	wantBRL(t, "carried until the next quote", got[3], "256.29")
	wantBRL(t, "latest quote", got[4], "258.6")
}

// TestValuesOnAgreesWithTodayAcrossSalesAndClosedPositions: partial and total
// sales, and a closed position, follow the same rules as today's figure.
func TestValuesOnAgreesWithTodayAcrossSalesAndClosedPositions(t *testing.T) {
	f := newLedgerFixture(t)
	position := f.addPosition(f.custodyID, "Ações")
	f.deposit("1000", day(1))
	f.create(investments.OperationInput{
		AccountID: f.custodyID, PositionID: &position, Kind: investments.OperationBuy,
		OccurredOn: day(2), Amount: dec("600"), Quantity: decRef("6"), UnitPrice: decRef("100"),
	})
	f.create(investments.OperationInput{
		AccountID: f.custodyID, PositionID: &position, Kind: investments.OperationSell,
		OccurredOn: day(4), Amount: dec("300"), Quantity: decRef("2"), UnitPrice: decRef("150"),
	})
	got := valuesOn(t, f, day(3), day(4))
	wantBRL(t, "before the sale (600 cost + 400 cash)", got[0], "1000")
	wantBRL(t, "after selling 2 of 6 (400 cost + 700 cash)", got[1], "1100")

	f.create(investments.OperationInput{
		AccountID: f.custodyID, PositionID: &position, Kind: investments.OperationSell,
		OccurredOn: day(6), Amount: dec("720"), Quantity: decRef("4"), UnitPrice: decRef("180"),
	})
	closed := valuesOn(t, f, day(5), day(6))
	wantBRL(t, "still holding 4", closed[0], "1100")
	wantBRL(t, "closed position, only cash (1420)", closed[1], "1420")
	today, err := investments.ManualNetWorth(f.ctx, f.conn)
	if err != nil {
		t.Fatal(err)
	}
	if !closed[1].Balances.Get("BRL").Equal(today) {
		t.Errorf("last day %s differs from ManualNetWorth %s", closed[1].Balances.Get("BRL"), today)
	}
}

// TestValuesOnIgnoresOperationsDatedAfterTheDay: a future-dated operation is
// not in the book yet.
func TestValuesOnIgnoresOperationsDatedAfterTheDay(t *testing.T) {
	f := newLedgerFixture(t)
	f.deposit("100", day(1))
	f.deposit("50", day(20))
	got := valuesOn(t, f, day(5), day(20))
	wantBRL(t, "before the later deposit", got[0], "100")
	wantBRL(t, "on it", got[1], "150")
}

// A manual holding inside an integrated custody counts like any other, with no
// custody cash of its own.
func TestValuesOnCountsAManualHoldingInAnIntegratedAccount(t *testing.T) {
	f := newLedgerFixture(t)
	f.addHolding(equity("1")) // a provider row with no balance is not part of the book
	if err := investments.EnsureIntegratedAccounts(f.ctx, f.conn); err != nil {
		t.Fatal(err)
	}
	if _, err := investments.CreatePosition(f.ctx, f.conn, investments.PositionInput{
		AccountID: "integrated:" + f.sourceID, Name: "Bitcoin", AssetType: "Criptoativo",
		InitialQuantity: dec("0.5"), InitialUnitCost: dec("1000"), OccurredOn: day(1),
	}); err != nil {
		t.Fatal(err)
	}
	got := valuesOn(t, f, day(0), day(2))
	wantBRL(t, "before it was opened", got[0], "0")
	wantBRL(t, "0.5 x 1000", got[1], "500")
	today, err := investments.ManualNetWorth(f.ctx, f.conn)
	if err != nil {
		t.Fatal(err)
	}
	if !got[1].Balances.Get("BRL").Equal(today) {
		t.Errorf("%s differs from ManualNetWorth %s", got[1].Balances.Get("BRL"), today)
	}
}

// A provider holding in another currency stays native: the caller converts it
// with the day's own rate.
func TestValuesOnKeepsAForeignProviderHoldingNative(t *testing.T) {
	f := newLedgerFixture(t)
	holding := lca()
	holding.CurrencyCode = strRef("USD")
	id := f.addHolding(holding)
	f.exec(`UPDATE financial_investments SET balance = '258.6', currency_code = 'USD' WHERE id = ?`, id)
	f.recordQuote(holding, "2026-09-28T00:03:22Z", "258.6")
	got := valuesOn(t, f, calendarDay("2026-09-28"))[0]
	if !got.Complete || !got.Balances.Get("USD").Equal(dec("258.6")) || !got.Balances.Get("BRL").IsZero() {
		t.Errorf("got %+v, want 258.6 USD and no BRL", got)
	}
}
