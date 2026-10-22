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

func TestEarliestDayIsTheOldestDatedMovement(t *testing.T) {
	f := newLedgerFixture(t)
	if _, ok, err := investments.EarliestDay(f.ctx, f.conn); err != nil || ok {
		t.Fatalf("empty book: ok = %v, err = %v", ok, err)
	}
	f.deposit("10", day(4))
	f.deposit("10", day(2))
	earliest, ok, err := investments.EarliestDay(f.ctx, f.conn)
	if err != nil || !ok || !earliest.Equal(day(2)) {
		t.Fatalf("EarliestDay = %v %v %v, want %v", earliest, ok, err, day(2))
	}
	f.addMovement(f.addHolding(equity("1")), "inflow", "1", "1", day(1))
	if earliest, _, _ = investments.EarliestDay(f.ctx, f.conn); !earliest.Equal(day(1)) {
		t.Fatalf("a provider movement is dated too: %v", earliest)
	}
}

func TestValuesOnFlagsAProviderHoldingThatWasNeverPriced(t *testing.T) {
	f := newLedgerFixture(t)
	id := f.addHolding(equity("5"))
	f.exec(`UPDATE financial_investments SET balance = '500' WHERE id = ?`, id)
	if got := valuesOn(t, f, day(3))[0]; got.Complete {
		t.Errorf("a holding with no price at all is complete: %+v", got)
	}
}
