package investments_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/exchange"
	"github.com/greg0x46/julius/internal/investments"
	"github.com/greg0x46/julius/internal/valuation"
)

func (f *ledgerFixture) portfolio(query investments.PortfolioQuery) investments.Portfolio {
	f.t.Helper()
	got, err := investments.BuildPortfolio(f.ctx, f.conn, query)
	if err != nil {
		f.t.Fatalf("BuildPortfolio: %v", err)
	}
	return got
}

func period(from *time.Time, to time.Time) investments.PortfolioQuery {
	return investments.PortfolioQuery{From: from, To: to}
}

func dayRef(n int) *time.Time { d := day(n); return &d }

func (f *ledgerFixture) value(positionID string, on time.Time, amount string) {
	f.t.Helper()
	f.create(investments.OperationInput{AccountID: f.custodyID, PositionID: &positionID, Kind: investments.OperationValuation, OccurredOn: on, Amount: dec(amount)})
}

func (f *ledgerFixture) sell(positionID string, on time.Time, amount, quantity string) {
	f.t.Helper()
	f.create(investments.OperationInput{AccountID: f.custodyID, PositionID: &positionID, Kind: investments.OperationSell, OccurredOn: on, Amount: dec(amount), Quantity: decRef(quantity)})
}

func positionsOf(p investments.Portfolio) map[string]investments.PortfolioPosition {
	found := map[string]investments.PortfolioPosition{}
	for _, group := range p.Groups {
		for _, position := range group.Positions {
			found[position.Name] = position
		}
	}
	return found
}

func sameDecimal(t *testing.T, label string, got decimal.Decimal, want string) {
	t.Helper()
	if !got.Equal(dec(want)) {
		t.Fatalf("%s = %s, want %s", label, got, want)
	}
}

func sameRate(t *testing.T, label string, got *decimal.Decimal, want string) {
	t.Helper()
	if want == "" {
		if got != nil {
			t.Fatalf("%s = %s, want none", label, got)
		}
		return
	}
	if got == nil || !got.Equal(dec(want)) {
		t.Fatalf("%s = %v, want %s", label, got, want)
	}
}

// reconciles checks V_end = V_start + contributions − withdrawals + yield.
func reconciles(t *testing.T, label string, start, end, contributions, withdrawals, yield decimal.Decimal) {
	t.Helper()
	if got := start.Add(contributions).Sub(withdrawals).Add(yield); !got.Equal(end) {
		t.Fatalf("%s: %s + %s − %s + %s = %s, want end value %s", label, start, contributions, withdrawals, yield, got, end)
	}
}

func assertReconciles(t *testing.T, p investments.Portfolio) {
	t.Helper()
	totals := p.Totals
	reconciles(t, "total", totals.StartValue, totals.EndValue, totals.Contributions, totals.Withdrawals, totals.Yield)
	for _, group := range p.Groups {
		g := group.Totals
		reconciles(t, "group "+group.Key, g.StartValue, g.EndValue, g.Contributions, g.Withdrawals, g.Yield)
		for _, position := range group.Positions {
			if position.UnavailableReason != "" {
				continue
			}
			reconciles(t, "position "+position.Name, position.StartValue, position.EndValue, position.Contributions, position.Withdrawals, position.Yield)
		}
	}
}

// The same economic story — 10 units for 1000 on the 1st, priced 105 on the
// 5th, 110 on the 20th, half sold for 600 on the 25th at 120 — told by a
// manual holding (typed valuations) and by a synced one (price series and
// movements) must give the same period.
func manualAndSyncedStory(f *ledgerFixture) {
	f.t.Helper()
	manual := f.addPosition(f.custodyID, "Manual")
	f.deposit("2000", day(1))
	f.buy(manual, day(1), "1000", "10")
	f.value(manual, day(5), "1050")
	f.value(manual, day(20), "1100")
	f.sell(manual, day(25), "600", "5")
	f.value(manual, day(25), "600")

	holding := equity("5")
	holding.Name, holding.Code = strRef("Synced"), strRef("SYNC11")
	id := f.addHolding(holding)
	f.addMovement(id, "inflow", "1000", "10", day(1))
	f.addMovement(id, "outflow", "600", "5", day(25))
	f.recordQuote(holding, "2026-09-05T12:00:00Z", "525")
	f.recordQuote(holding, "2026-09-20T12:00:00Z", "550")
	f.recordQuote(holding, "2026-09-25T12:00:00Z", "600")
}

func TestPortfolioManualAndSyncedFollowTheSameMethodology(t *testing.T) {
	f := newLedgerFixture(t)
	manualAndSyncedStory(f)
	manual, synced := investments.PositionSourceManual, investments.PositionSourceSynced

	for _, scenario := range []struct {
		name                          string
		from                          *time.Time
		start, end, contributions     string
		withdrawals, yield, base, pct string
	}{
		// Since inception: 1000 in on the 1st, 600 out on the 25th. The flows
		// weigh 27 and 3 of the 28 days: (1000×27 − 600×3) / 28 = 900.
		{"inception", nil, "0", "600", "1000", "600", "200", "900", "0.2222222222"},
		// From the 10th the opening is the 9th's value, 1050, kept by the
		// quote and the valuation of the 5th: (1050×19 − 600×3) / 19.
		{"from the 10th", dayRef(10), "1050", "600", "0", "600", "150", "955.2631578947", "0.1570247934"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			query := period(scenario.from, day(28))
			query.Source = &manual
			m := f.portfolio(query)
			query.Source = &synced
			s := f.portfolio(query)
			for label, got := range map[string]investments.PortfolioTotals{"manual": m.Totals, "synced": s.Totals} {
				sameDecimal(t, label+" start", got.StartValue, scenario.start)
				sameDecimal(t, label+" end", got.EndValue, scenario.end)
				sameDecimal(t, label+" contributions", got.Contributions, scenario.contributions)
				sameDecimal(t, label+" withdrawals", got.Withdrawals, scenario.withdrawals)
				sameDecimal(t, label+" yield", got.Yield, scenario.yield)
				sameDecimal(t, label+" capital base", got.CapitalBase, scenario.base)
				sameRate(t, label+" rate", got.YieldRate, scenario.pct)
				if got.PositionsTotal != 1 || got.Partial() {
					t.Fatalf("%s counts = %+v", label, got)
				}
			}
			assertReconciles(t, m)
			assertReconciles(t, s)
		})
	}

	// The cost of what is held differs by design: the ledger's basis for a
	// manual holding (half of 1000), the net money put into a synced one.
	query := period(nil, day(28))
	both := positionsOf(f.portfolio(query))
	sameDecimal(t, "manual invested", *both["Manual"].Invested, "500")
	sameDecimal(t, "synced invested", *both["Synced"].Invested, "400")
	sameDecimal(t, "synced quantity", *both["Synced"].Quantity, "5")
}

func TestPortfolioIsAsOfTheEndOfThePeriod(t *testing.T) {
	f := newLedgerFixture(t)
	closedLater := f.addPosition(f.custodyID, "Closed later")
	f.deposit("5000", day(1))
	f.buy(closedLater, day(1), "1000", "10")
	f.value(closedLater, day(2), "1000")
	f.sell(closedLater, day(20), "1100", "10")
	boughtLater := f.addPosition(f.custodyID, "Bought later")
	f.buy(boughtLater, day(15), "500", "5")
	f.value(boughtLater, day(15), "500")

	// On the 12th the first is still held and the second does not exist yet,
	// although today it is the other way round.
	early := f.portfolio(period(dayRef(3), day(12)))
	found := positionsOf(early)
	if len(found) != 1 || found["Closed later"].ClosedAsOfTo {
		t.Fatalf("early positions = %+v", found)
	}
	held := found["Closed later"]
	sameDecimal(t, "quantity as of the 12th", *held.Quantity, "10")
	sameDecimal(t, "end value as of the 12th", held.EndValue, "1000")
	sameDecimal(t, "invested as of the 12th", *held.Invested, "1000")
	sameDecimal(t, "early yield", early.Totals.Yield, "0")

	// The same holds since inception.
	if found := positionsOf(f.portfolio(period(nil, day(12)))); len(found) != 1 || found["Bought later"].ID != "" {
		t.Fatalf("since inception, early positions = %+v", found)
	}

	// On the 28th the first was sold for more than it cost: it is worth
	// nothing, still part of the numbers, and its result is the realized gain.
	late := f.portfolio(period(dayRef(3), day(28)))
	found = positionsOf(late)
	closed := found["Closed later"]
	if !closed.ClosedAsOfTo || len(found) != 2 {
		t.Fatalf("late positions = %+v", found)
	}
	sameDecimal(t, "closed end value", closed.EndValue, "0")
	sameDecimal(t, "closed start value", closed.StartValue, "1000")
	sameDecimal(t, "closed withdrawals", closed.Withdrawals, "1100")
	sameDecimal(t, "closed realized yield", closed.Yield, "100")
	sameDecimal(t, "closed invested", *closed.Invested, "0")
	assertReconciles(t, late)

	// A holding that closed before the period and did nothing in it is not
	// reported.
	if after := positionsOf(f.portfolio(period(dayRef(22), day(28)))); len(after) != 1 || after["Bought later"].ID == "" {
		t.Fatalf("positions after the sale = %+v", after)
	}

	// Since inception its yield is the monthly movements' realized gain.
	inception := positionsOf(f.portfolio(period(nil, day(28))))["Closed later"]
	sameDecimal(t, "yield since inception", inception.Yield, "100")
}

func TestPortfolioContributionsWithdrawalsAndTransfersAreNotYield(t *testing.T) {
	f := newLedgerFixture(t)
	destination, err := investments.CreateAccount(f.ctx, f.conn, investments.AccountInput{Name: "Carteira"})
	if err != nil {
		t.Fatal(err)
	}
	ticker := "ABCD3"
	source, err := investments.CreatePosition(f.ctx, f.conn, investments.PositionInput{AccountID: f.custodyID, Name: "Ações", Ticker: &ticker, AssetType: "Ação"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := investments.CreatePosition(f.ctx, f.conn, investments.PositionInput{AccountID: destination.ID, AssetID: source.AssetID, Name: "Ações", AssetType: "Ação"})
	if err != nil {
		t.Fatal(err)
	}
	f.deposit("5000", day(1))
	f.buy(source.ID, day(1), "1000", "10")
	f.value(source.ID, day(2), "1000")
	// A contribution priced at cost, then a custody transfer priced at cost:
	// nothing was earned, however much money moved.
	f.buy(source.ID, day(5), "500", "5")
	f.value(source.ID, day(5), "1500")
	if _, err := investments.CreateTransfer(f.ctx, f.conn, investments.TransferInput{SourcePositionID: source.ID, DestinationPositionID: target.ID, Quantity: dec("15"), OccurredOn: day(10)}); err != nil {
		t.Fatalf("CreateTransfer: %v", err)
	}
	f.create(investments.OperationInput{AccountID: destination.ID, PositionID: &target.ID, Kind: investments.OperationValuation, OccurredOn: day(10), Amount: dec("1500")})
	f.create(investments.OperationInput{AccountID: destination.ID, PositionID: &target.ID, Kind: investments.OperationSell, OccurredOn: day(20), Amount: dec("300"), Quantity: decRef("3")})

	query := period(dayRef(3), day(28))
	query.GroupBy = investments.PortfolioGroupAccount
	got := f.portfolio(query)
	assertReconciles(t, got)
	if len(got.Groups) != 2 {
		t.Fatalf("groups = %+v", got.Groups)
	}
	// 500 contributed, 1500 left the first account and entered the second, 300
	// was sold at the valuation it carried (3 of 15 units of 1500 = 300).
	sameDecimal(t, "contributions", got.Totals.Contributions, "2000")
	sameDecimal(t, "withdrawals", got.Totals.Withdrawals, "1800")
	sameDecimal(t, "yield", got.Totals.Yield, "0")
}

func TestPortfolioTotalsDoNotDependOnGroupingOrDisplayFilter(t *testing.T) {
	f := newLedgerFixture(t)
	manualAndSyncedStory(f)
	other, err := investments.CreateAccount(f.ctx, f.conn, investments.AccountInput{Name: "Outra"})
	if err != nil {
		t.Fatal(err)
	}
	thirdID := "00000000-0000-4000-8000-000000000042"
	f.exec(`INSERT INTO investment_assets (id, canonical_key, name, asset_type, asset_class, created_at, updated_at)
		VALUES (?, 'name:CDB:Fixed', 'Fixed', 'CDB', 'fixed_income', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`, thirdID)
	positionID := "00000000-0000-4000-8000-000000000043"
	f.exec(`INSERT INTO investment_positions (id, account_id, asset_id, created_at, updated_at) VALUES (?, ?, ?, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`, positionID, other.ID, thirdID)
	f.create(investments.OperationInput{AccountID: other.ID, Kind: investments.OperationDeposit, OccurredOn: day(1), Amount: dec("300")})
	f.create(investments.OperationInput{AccountID: other.ID, PositionID: &positionID, Kind: investments.OperationBuy, OccurredOn: day(3), Amount: dec("300"), Quantity: decRef("3")})
	f.create(investments.OperationInput{AccountID: other.ID, PositionID: &positionID, Kind: investments.OperationValuation, OccurredOn: day(3), Amount: dec("300")})
	f.create(investments.OperationInput{AccountID: other.ID, PositionID: &positionID, Kind: investments.OperationValuation, OccurredOn: day(15), Amount: dec("330")})

	base := period(dayRef(10), day(28))
	var reference investments.PortfolioTotals
	for i, query := range []investments.PortfolioQuery{
		base,
		{From: base.From, To: base.To, GroupBy: investments.PortfolioGroupAccount},
		{From: base.From, To: base.To, GroupBy: investments.PortfolioGroupClass},
		{From: base.From, To: base.To, GroupBy: investments.PortfolioGroupAccount, AccountID: &other.ID},
	} {
		got := f.portfolio(query)
		assertReconciles(t, got)
		if i == 0 {
			reference = got.Totals
			// 150 earned by the two stories plus 30 by the third: summed
			// components, not an average of the three percentages.
			sameDecimal(t, "yield", got.Totals.Yield, "330")
			continue
		}
		if got.Totals.Yield.String() != reference.Yield.String() || got.Totals.EndValue.String() != reference.EndValue.String() ||
			got.Totals.CapitalBase.String() != reference.CapitalBase.String() || !got.Totals.YieldRate.Equal(*reference.YieldRate) {
			t.Fatalf("query %d: totals %+v differ from %+v", i, got.Totals, reference)
		}
		var sum decimal.Decimal
		for _, group := range got.Groups {
			sum = sum.Add(group.Totals.EndValue)
			if group.Share == nil {
				t.Fatalf("group %s has no share", group.Key)
			}
		}
		if query.AccountID == nil && !sum.Equal(got.Totals.EndValue) {
			t.Fatalf("groups add up to %s, total is %s", sum, got.Totals.EndValue)
		}
	}

	byClass := f.portfolio(investments.PortfolioQuery{From: base.From, To: base.To, GroupBy: investments.PortfolioGroupClass})
	keys := map[string]bool{}
	for _, group := range byClass.Groups {
		keys[group.Key] = true
	}
	if len(byClass.Groups) != 3 || !keys["other"] || !keys["fixed_income"] || !keys["variable_income"] {
		t.Fatalf("class groups = %v", keys)
	}
	filtered := f.portfolio(investments.PortfolioQuery{From: base.From, To: base.To, GroupBy: investments.PortfolioGroupAccount, AccountID: &other.ID})
	if len(filtered.Groups) != 1 || filtered.Groups[0].AccountID == nil || *filtered.Groups[0].AccountID != other.ID {
		t.Fatalf("filtered groups = %+v", filtered.Groups)
	}
	if share := filtered.Groups[0].Share; share == nil || !share.LessThan(dec("1")) {
		t.Fatalf("a filtered group's share is of the whole portfolio, got %v", share)
	}
}

func TestPortfolioPercentageIsDietzOverSummedComponents(t *testing.T) {
	f := newLedgerFixture(t)
	big := f.addPosition(f.custodyID, "Big")
	small := f.addPosition(f.custodyID, "Small")
	f.deposit("5000", day(1))
	f.buy(big, day(1), "1000", "10")
	f.value(big, day(1), "1000")
	f.value(big, day(15), "1100")
	f.buy(small, day(1), "100", "1")
	f.value(small, day(1), "100")
	f.value(small, day(15), "150")

	got := f.portfolio(period(dayRef(2), day(30)))
	found := positionsOf(got)
	sameRate(t, "big", found["Big"].YieldRate, "0.1")
	sameRate(t, "small", found["Small"].YieldRate, "0.5")
	// 150 earned on 1100 of capital — not the 0.3 average of 0.1 and 0.5.
	sameRate(t, "total", got.Totals.YieldRate, "0.1363636364")
	sameDecimal(t, "capital", got.Totals.CapitalBase, "1100")
}

func TestPortfolioWeighsAContributionByTheDaysItWasInvested(t *testing.T) {
	f := newLedgerFixture(t)
	id := f.addPosition(f.custodyID, "Ações")
	f.deposit("5000", day(1))
	f.buy(id, day(1), "1000", "10")
	f.value(id, day(1), "1000")
	f.buy(id, day(16), "1000", "10")
	f.value(id, day(30), "2100")

	// From the 2nd to the 30th: opening 1000 on 29 days, plus 1000 on the 16th
	// with 14 days left: (1000×29 + 1000×14) / 29 = 1482.7586206897.
	got := f.portfolio(period(dayRef(2), day(30)))
	sameDecimal(t, "yield", got.Totals.Yield, "100")
	sameDecimal(t, "contributions", got.Totals.Contributions, "1000")
	sameDecimal(t, "capital", got.Totals.CapitalBase, "1482.7586206897")
	sameRate(t, "rate", got.Totals.YieldRate, "0.0674418605")
	assertReconciles(t, got)
}

func TestPortfolioCountsFeesAndTaxesOnce(t *testing.T) {
	f := newLedgerFixture(t)
	id := f.addPosition(f.custodyID, "Ações")
	f.deposit("5000", day(1))
	f.buyWithCosts(f.custodyID, id, day(2), "600", "6", "12", "6") // cost basis 618
	f.value(id, day(3), "650")
	f.sellWithCosts(f.custodyID, id, day(10), "700", "6", "3", "1") // nets 696

	open := positionsOf(f.portfolio(period(nil, day(5))))["Ações"]
	// Money in is the amount plus its costs; the valuation is not touched by them.
	sameDecimal(t, "contributions", open.Contributions, "618")
	sameDecimal(t, "yield", open.Yield, "32")
	sameDecimal(t, "costs", *open.Costs, "18")
	sameDecimal(t, "invested", *open.Invested, "618")

	closed := positionsOf(f.portfolio(period(nil, day(28))))["Ações"]
	sameDecimal(t, "withdrawals", closed.Withdrawals, "696")
	// 696 net proceeds against 618 paid, exactly the realized gain; the 22 in
	// fees and taxes are informational.
	sameDecimal(t, "realized yield", closed.Yield, "78")
	sameDecimal(t, "costs", *closed.Costs, "22")
	gain := f.monthlyMovements(nil)["2026-09"].RealizedGain
	sameDecimal(t, "realized gain", gain, "78")

	// Costs outside the period are not reported for it.
	later := positionsOf(f.portfolio(period(dayRef(8), day(28))))["Ações"]
	sameDecimal(t, "costs in the second week", *later.Costs, "4")
}

// An unavailable position is still closed when the provider says nothing is
// held, so the "show closed" switch can hide it; an unknown quantity says
// nothing and the position stays listed.
func TestPortfolioMarksAnUnavailablePositionClosedOnlyWhenNothingIsHeld(t *testing.T) {
	f := newLedgerFixture(t)
	manualAndSyncedStory(f)
	gone := equity("0")
	gone.Name, gone.Code = strRef("Gone"), strRef("GONE11")
	f.addHolding(gone)
	held := equity("10")
	held.Name, held.Code = strRef("Held"), strRef("HELD11")
	f.addHolding(held)
	unknown := equity("10")
	unknown.Name, unknown.Code, unknown.Quantity = strRef("Unknown"), strRef("UNKN11"), nil
	f.addHolding(unknown)

	found := positionsOf(f.portfolio(period(nil, day(28))))
	for _, name := range []string{"Gone", "Held", "Unknown"} {
		if found[name].UnavailableReason == "" {
			t.Fatalf("%s should be unavailable: %+v", name, found[name])
		}
	}
	if !found["Gone"].ClosedAsOfTo {
		t.Fatalf("a position known to hold nothing must be closed: %+v", found["Gone"])
	}
	if found["Held"].ClosedAsOfTo || found["Unknown"].ClosedAsOfTo {
		t.Fatalf("held = %v, unknown quantity = %v", found["Held"].ClosedAsOfTo, found["Unknown"].ClosedAsOfTo)
	}
}

func TestPortfolioNeverTurnsUnknownIntoZero(t *testing.T) {
	f := newLedgerFixture(t)
	manualAndSyncedStory(f)
	orphan := equity("10")
	orphan.Name, orphan.Code = strRef("Orphan"), strRef("ORPH11")
	f.addHolding(orphan)
	unpriced := equity("10")
	unpriced.Name, unpriced.Code = strRef("Unpriced"), strRef("UNPR11")
	unpricedID := f.addHolding(unpriced)
	f.addMovement(unpricedID, "inflow", "1000", "10", day(1))
	f.recordQuote(orphan, "2026-09-02T12:00:00Z", "1000")
	f.recordQuote(orphan, "2026-09-26T12:00:00Z", "1200")

	got := f.portfolio(period(nil, day(28)))
	found := positionsOf(got)
	if found["Orphan"].UnavailableReason != investments.YieldReasonNoHistory || found["Unpriced"].UnavailableReason != investments.YieldReasonNoPrice {
		t.Fatalf("reasons: orphan %q, unpriced %q", found["Orphan"].UnavailableReason, found["Unpriced"].UnavailableReason)
	}
	if found["Orphan"].Native != nil || found["Orphan"].Invested != nil || found["Orphan"].InvestedUnavailableReason != investments.YieldReasonNoHistory {
		t.Fatalf("an unavailable position carries figures: %+v", found["Orphan"])
	}
	if got.Totals.PositionsTotal != 4 || got.Totals.PositionsUnavailable != 2 || !got.Totals.Partial() {
		t.Fatalf("counts = %+v", got.Totals)
	}
	// Only the two available positions are summed — in numerator and base.
	sameDecimal(t, "yield", got.Totals.Yield, "400")
	sameDecimal(t, "end value", got.Totals.EndValue, "1200")
	assertReconciles(t, got)

	// With an explicit start the orphan is computable from the price series
	// alone, although nothing says what it cost.
	from := f.portfolio(period(dayRef(5), day(28)))
	orphanPeriod := positionsOf(from)["Orphan"]
	if orphanPeriod.UnavailableReason != "" || orphanPeriod.Invested != nil || orphanPeriod.InvestedUnavailableReason != investments.YieldReasonNoHistory {
		t.Fatalf("orphan with a start = %+v", orphanPeriod)
	}
	sameDecimal(t, "orphan yield", orphanPeriod.Yield, "200")
	if orphanPeriod.HistoryComplete || from.Totals.InvestedUnknown != 1 {
		t.Fatalf("history complete %v, unknown invested %d", orphanPeriod.HistoryComplete, from.Totals.InvestedUnknown)
	}
	assertReconciles(t, from)
}

func TestPortfolioConvertsForeignPositionsConsistently(t *testing.T) {
	f := newLedgerFixture(t)
	holding := equity("10")
	holding.Name, holding.Code = strRef("Global"), strRef("GLBL")
	id := f.addHolding(holding)
	f.exec(`UPDATE financial_investments SET currency_code = 'USD' WHERE id = ?`, id)
	holding.CurrencyCode = strRef("USD")
	f.addMovement(id, "inflow", "100", "10", day(1))
	f.recordQuote(holding, "2026-09-20T12:00:00Z", "110")

	valued := func(rate string) *valuation.Valuation {
		return valuation.New(exchange.New(f.conn, usdProvider{rate}), day(28))
	}
	// Without a rate (asked before any is stored) the position is unavailable,
	// not worth zero reais.
	missing, err := investments.BuildPortfolioValued(f.ctx, f.conn, period(nil, day(28)), valued(""))
	if err != nil {
		t.Fatal(err)
	}
	if got := positionsOf(missing)["Global"]; got.UnavailableReason != investments.YieldReasonNoRate {
		t.Fatalf("without a rate: %+v", got)
	}
	if missing.Totals.PositionsUnavailable != 1 || !missing.Totals.EndValue.IsZero() {
		t.Fatalf("totals without a rate = %+v", missing.Totals)
	}

	got, err := investments.BuildPortfolioValued(f.ctx, f.conn, period(nil, day(28)), valued("5"))
	if err != nil {
		t.Fatal(err)
	}
	position := positionsOf(got)["Global"]
	// 100 USD in, worth 110 USD: 10 USD earned, 50 BRL at the one rate used.
	sameDecimal(t, "end", got.Totals.EndValue, "550")
	sameDecimal(t, "contributions", got.Totals.Contributions, "500")
	sameDecimal(t, "yield", got.Totals.Yield, "50")
	sameDecimal(t, "native end", position.Native.EndValue, "110")
	sameDecimal(t, "native yield", position.Native.Yield, "10")
	sameDecimal(t, "invested", *position.Invested, "500")
	sameDecimal(t, "native invested", *position.Native.Invested, "100")
	if position.CurrencyCode != "USD" || got.Valuation.Rates["USD"].Value.String() != "5" {
		t.Fatalf("position %+v, valuation %+v", position, got.Valuation)
	}
	assertReconciles(t, got)

}

func TestPortfolioKeepsTheProvidersFiguresForConferenceOnly(t *testing.T) {
	f := newLedgerFixture(t)
	holding := equity("10")
	id := f.addHolding(holding)
	f.exec(`UPDATE financial_investments SET amount_profit = '999', taxes = '4.5', taxes2 = '0.5', as_of_date = ? WHERE id = ?`, "2026-09-20T12:00:00Z", id)
	f.addMovement(id, "inflow", "1000", "10", day(1))
	f.recordQuote(holding, "2026-09-20T12:00:00Z", "1100")

	position := positionsOf(f.portfolio(period(nil, day(28))))["HGLG11"]
	sameDecimal(t, "computed yield", position.Yield, "100")
	if position.ReportedProfit == nil || position.ReportedProfit.String() != "999" {
		t.Fatalf("reported profit = %v", position.ReportedProfit)
	}
	// The yield is gross: the provisioned IR/IOF is reported, never deducted.
	if position.ProvisionedTaxes == nil || position.ProvisionedTaxes.String() != "5" {
		t.Fatalf("provisioned taxes = %v", position.ProvisionedTaxes)
	}
	// A snapshot taken after the period says nothing about it.
	before := positionsOf(f.portfolio(period(nil, day(15))))["HGLG11"]
	if before.ProvisionedTaxes != nil {
		t.Fatalf("provisioned taxes before the snapshot = %v", before.ProvisionedTaxes)
	}
}

func TestPortfolioRejectsAnInvertedPeriodAndUnknownGrouping(t *testing.T) {
	f := newLedgerFixture(t)
	if _, err := investments.BuildPortfolio(f.ctx, f.conn, period(dayRef(20), day(10))); err == nil {
		t.Fatal("from after to was accepted")
	}
	query := period(nil, day(10))
	query.GroupBy = "institution"
	if _, err := investments.BuildPortfolio(f.ctx, f.conn, query); err == nil {
		t.Fatal("unknown grouping was accepted")
	}
	empty := f.portfolio(period(nil, day(10)))
	if len(empty.Groups) != 0 || empty.Totals.PositionsTotal != 0 || empty.Totals.YieldRate != nil {
		t.Fatalf("empty portfolio = %+v", empty)
	}
}

type queryCounter struct {
	investments.Querier
	counts map[string]int
}

func (c *queryCounter) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	for _, table := range []string{"FROM investment_asset_quotes", "FROM financial_investment_transactions", "FROM financial_investments"} {
		if strings.Contains(query, table) {
			c.counts[table]++
		}
	}
	return c.Querier.QueryContext(ctx, query, args...)
}

// Sanity bound, not a benchmark: holdings of one asset share one price-series
// load, and the provider's holdings and movements are read in bulk.
func TestPortfolioReadsSharedDataOncePerRequest(t *testing.T) {
	f := newLedgerFixture(t)
	twoLotsOfOneStock(f)
	for i := 0; i < 3; i++ {
		other := equity("2")
		other.Name, other.Code = strRef("Other "+string(rune('A'+i))), strRef("OTH"+string(rune('A'+i))+"11")
		id := f.addHolding(other)
		f.addMovement(id, "inflow", "200", "2", day(1))
		f.recordQuote(other, "2026-09-20T12:00:00Z", "220")
	}
	counter := &queryCounter{Querier: f.conn, counts: map[string]int{}}
	got, err := investments.BuildPortfolio(f.ctx, counter, period(nil, day(28)))
	if err != nil {
		t.Fatal(err)
	}
	if got.Totals.PositionsTotal != 5 || got.Totals.PositionsUnavailable != 0 {
		t.Fatalf("totals = %+v", got.Totals)
	}
	// Four assets, five holdings: one series per asset.
	if counter.counts["FROM investment_asset_quotes"] != 4 {
		t.Fatalf("price series loaded %d times for 4 assets", counter.counts["FROM investment_asset_quotes"])
	}
	if counter.counts["FROM financial_investment_transactions"] != 1 {
		t.Fatalf("movements read %d times", counter.counts["FROM financial_investment_transactions"])
	}
}

// Held and closed split the totals: what the positions still held earned, and
// the realized result of the ones closed as of `to`. Both reconcile, they add
// up to the totals, and each rate divides its own summed components.
func TestPortfolioSplitsHeldFromClosedAndSharesAddUp(t *testing.T) {
	f := newLedgerFixture(t)
	closed := f.addPosition(f.custodyID, "Closed")
	heldA := f.addPosition(f.custodyID, "Held A")
	heldB := f.addPosition(f.custodyID, "Held B")
	f.deposit("5000", day(1))
	f.buy(closed, day(1), "1000", "10")
	f.value(closed, day(2), "1000")
	f.sell(closed, day(20), "1100", "10")
	f.buy(heldA, day(1), "500", "5")
	f.value(heldA, day(2), "500")
	f.value(heldA, day(28), "600")
	f.buy(heldB, day(1), "300", "3")
	f.value(heldB, day(2), "300")
	f.value(heldB, day(28), "400")

	got := f.portfolio(period(dayRef(3), day(28)))
	assertReconciles(t, got)
	totals := got.Totals
	for label, part := range map[string]investments.PortfolioPart{"held": totals.Held, "closed": totals.Closed} {
		reconciles(t, label, part.StartValue, part.EndValue, part.Contributions, part.Withdrawals, part.Yield)
	}
	sameDecimal(t, "held start", totals.Held.StartValue, "800")
	sameDecimal(t, "held end", totals.Held.EndValue, "1000")
	sameDecimal(t, "held yield", totals.Held.Yield, "200")
	sameRate(t, "held rate", totals.Held.YieldRate, "0.25")
	sameDecimal(t, "closed start", totals.Closed.StartValue, "1000")
	sameDecimal(t, "closed end", totals.Closed.EndValue, "0")
	sameDecimal(t, "closed withdrawals", totals.Closed.Withdrawals, "1100")
	sameDecimal(t, "closed yield", totals.Closed.Yield, "100")
	sameDecimal(t, "closed capital", totals.Closed.CapitalBase, "661.5384615385")
	sameRate(t, "closed rate", totals.Closed.YieldRate, "0.1511627907")

	// held + closed = totals, field by field; the rate is not their average.
	for label, pair := range map[string][2]decimal.Decimal{
		"start":         {totals.Held.StartValue.Add(totals.Closed.StartValue), totals.StartValue},
		"end":           {totals.Held.EndValue.Add(totals.Closed.EndValue), totals.EndValue},
		"contributions": {totals.Held.Contributions.Add(totals.Closed.Contributions), totals.Contributions},
		"withdrawals":   {totals.Held.Withdrawals.Add(totals.Closed.Withdrawals), totals.Withdrawals},
		"yield":         {totals.Held.Yield.Add(totals.Closed.Yield), totals.Yield},
		"capital":       {totals.Held.CapitalBase.Add(totals.Closed.CapitalBase), totals.CapitalBase},
	} {
		if !pair[0].Equal(pair[1]) {
			t.Fatalf("held + closed %s = %s, total %s", label, pair[0], pair[1])
		}
	}
	sameDecimal(t, "capital", totals.CapitalBase, "1461.5384615385")
	sameRate(t, "total rate", totals.YieldRate, "0.2052631579")

	// Shares of the portfolio's end value; the closed position has none left.
	found := positionsOf(got)
	sameRate(t, "held A share", found["Held A"].Share, "0.6")
	sameRate(t, "held B share", found["Held B"].Share, "0.4")
	sameRate(t, "closed share", found["Closed"].Share, "0")
	sameRate(t, "group share", got.Groups[0].Share, "1")
	if !found["Closed"].ClosedAsOfTo || found["Held A"].ClosedAsOfTo {
		t.Fatalf("closed flags: %+v", found)
	}

	// Within a group the part still reconciles, and an unavailable position
	// has no share.
	f.addHolding(equity("10"))
	withOrphan := f.portfolio(period(nil, day(28)))
	for _, group := range withOrphan.Groups {
		reconciles(t, "held "+group.Key, group.Totals.Held.StartValue, group.Totals.Held.EndValue, group.Totals.Held.Contributions, group.Totals.Held.Withdrawals, group.Totals.Held.Yield)
	}
	if orphan := positionsOf(withOrphan)["HGLG11"]; orphan.UnavailableReason == "" || orphan.Share != nil {
		t.Fatalf("orphan = %+v", orphan)
	}
}
